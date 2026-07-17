package api

import (
	"errors"
	"net/http"

	"github.com/x2core/mrhandly/agent/internal/audit"
	"github.com/x2core/mrhandly/agent/internal/protocol"
	"github.com/x2core/mrhandly/agent/internal/systemd"
)

// requireSystemd short-circuits with systemd_unavailable when the host has no
// systemd. The same binary runs everywhere — the agent doesn't care, the UI
// hides the tab (docs/ROADMAP.md M2/M3 pattern).
func (s *Server) requireSystemd(w http.ResponseWriter) bool {
	if s.deps.Services == nil {
		writeError(w, http.StatusServiceUnavailable, protocol.ErrSystemdUnavailable,
			"systemd is not available on this host")
		return false
	}
	return true
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystemd(w) {
		return
	}
	list, err := s.deps.Services.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "failed to list services")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleService(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystemd(w) {
		return
	}
	unit := r.PathValue("unit")
	svc, err := s.deps.Services.Get(r.Context(), unit)
	if err != nil {
		if errors.Is(err, systemd.ErrUnitNotFound) {
			writeError(w, http.StatusNotFound, protocol.ErrNotFound, "no such unit")
			return
		}
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "failed to read unit")
		return
	}
	writeJSON(w, http.StatusOK, svc)
}

// handleServiceAction performs start/stop/restart. The unit allowlist is
// enforced here, at the handler boundary — the D-Bus layer stays pure
// mechanism (CLAUDE.md §4.3). Every attempt is audited.
func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystemd(w) {
		return
	}
	unit := r.PathValue("unit")
	action := r.PathValue("action")
	actor := remoteHost(r.RemoteAddr)

	do, ok := map[string]func() error{
		"start":   func() error { return s.deps.Services.Start(r.Context(), unit) },
		"stop":    func() error { return s.deps.Services.Stop(r.Context(), unit) },
		"restart": func() error { return s.deps.Services.Restart(r.Context(), unit) },
	}[action]
	if !ok {
		writeError(w, http.StatusNotFound, protocol.ErrNotFound, "no such action")
		return
	}

	if !s.deps.Services.Writable(unit) {
		s.deps.Audit.Log(actor, "service."+action, unit, audit.ResultForbidden)
		writeError(w, http.StatusForbidden, protocol.ErrUnitNotAllowed,
			"unit is not in the write allowlist")
		return
	}

	if err := do(); err != nil {
		s.deps.Audit.Log(actor, "service."+action, unit, audit.ResultError)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "action failed")
		return
	}
	s.deps.Audit.Log(actor, "service."+action, unit, audit.ResultOK)

	// Report the current state so the caller sees the result immediately; the
	// stream will carry any later transition.
	if svc, err := s.deps.Services.Get(r.Context(), unit); err == nil {
		writeJSON(w, http.StatusOK, svc)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"unit": unit, "action": action, "status": "ok"})
}

func (s *Server) handleServicesStream(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystemd(w) {
		return
	}
	if s.deps.ServicesStream == nil {
		writeError(w, http.StatusServiceUnavailable, protocol.ErrSystemdUnavailable,
			"service streaming unavailable")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "streaming unsupported")
		return
	}
	setSSEHeaders(w)

	id, ch := s.deps.ServicesStream.Subscribe()
	defer s.deps.ServicesStream.Unsubscribe(id)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case list, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, list)
			flusher.Flush()
		}
	}
}

// handleServiceLogs streams journald logs for a unit as SSE. It validates read
// scope first (an unreadable unit is 404, not a leak), then starts journalctl
// bound to the request context so a disconnect kills it.
func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystemd(w) {
		return
	}
	if s.deps.Journal == nil {
		writeError(w, http.StatusServiceUnavailable, protocol.ErrSystemdUnavailable,
			"log streaming unavailable")
		return
	}
	unit := r.PathValue("unit")
	if _, err := s.deps.Services.Get(r.Context(), unit); err != nil {
		if errors.Is(err, systemd.ErrUnitNotFound) {
			writeError(w, http.StatusNotFound, protocol.ErrNotFound, "no such unit")
			return
		}
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "failed to read unit")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "streaming unsupported")
		return
	}

	follow := r.URL.Query().Get("follow") == "1"
	// Start the subprocess before writing headers so a start error is a clean
	// JSON error, not a half-open stream.
	ch, err := s.deps.Journal.Stream(r.Context(), unit, follow)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "failed to start log stream")
		return
	}

	setSSEHeaders(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, line)
			flusher.Flush()
		}
	}
}
