package api

import (
	"errors"
	"net/http"

	"github.com/x2core/mrhandly/agent/internal/audit"
	"github.com/x2core/mrhandly/agent/internal/docker"
	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// requireDocker short-circuits with docker_unavailable when no Docker socket is
// present. Same binary everywhere — the UI hides the tab (docs/ROADMAP.md M3).
func (s *Server) requireDocker(w http.ResponseWriter) bool {
	if s.deps.Docker == nil {
		writeError(w, http.StatusServiceUnavailable, protocol.ErrDockerUnavailable,
			"docker is not available on this host")
		return false
	}
	return true
}

// dockerError maps a client error to an HTTP response. A transport failure
// (socket vanished) degrades to docker_unavailable; a 404 to not_found.
func (s *Server) dockerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, docker.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, protocol.ErrDockerUnavailable,
			"docker socket became unavailable")
	case errors.Is(err, docker.ErrNotFound):
		writeError(w, http.StatusNotFound, protocol.ErrNotFound, "no such container")
	default:
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "docker request failed")
	}
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	if !s.requireDocker(w) {
		return
	}
	list, err := s.deps.Docker.ListContainers(r.Context())
	if err != nil {
		s.dockerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleContainer(w http.ResponseWriter, r *http.Request) {
	if !s.requireDocker(w) {
		return
	}
	c, err := s.deps.Docker.InspectContainer(r.Context(), r.PathValue("id"))
	if err != nil {
		s.dockerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleImages(w http.ResponseWriter, r *http.Request) {
	if !s.requireDocker(w) {
		return
	}
	imgs, err := s.deps.Docker.ListImages(r.Context())
	if err != nil {
		s.dockerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, imgs)
}

// handleContainerAction performs start/stop/restart. The docker_read_only gate
// is enforced here, at the handler boundary; every attempt is audited.
func (s *Server) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireDocker(w) {
		return
	}
	id := r.PathValue("id")
	action := r.PathValue("action")
	actor := remoteHost(r.RemoteAddr)

	do, ok := map[string]func() error{
		"start":   func() error { return s.deps.Docker.StartContainer(r.Context(), id) },
		"stop":    func() error { return s.deps.Docker.StopContainer(r.Context(), id) },
		"restart": func() error { return s.deps.Docker.RestartContainer(r.Context(), id) },
	}[action]
	if !ok {
		writeError(w, http.StatusNotFound, protocol.ErrNotFound, "no such action")
		return
	}

	if !s.deps.DockerWritable {
		s.deps.Audit.Log(actor, "docker."+action, id, audit.ResultForbidden)
		writeError(w, http.StatusForbidden, protocol.ErrDockerReadOnly,
			"docker control is disabled on this host")
		return
	}

	if err := do(); err != nil {
		s.deps.Audit.Log(actor, "docker."+action, id, audit.ResultError)
		s.dockerError(w, err)
		return
	}
	s.deps.Audit.Log(actor, "docker."+action, id, audit.ResultOK)

	if c, err := s.deps.Docker.InspectContainer(r.Context(), id); err == nil {
		writeJSON(w, http.StatusOK, c)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "action": action, "status": "ok"})
}

// handleContainerLogs streams a container's output as SSE. The subprocess-free
// stream is bound to the request context: a disconnect cancels it and the
// underlying connection is closed.
func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireDocker(w) {
		return
	}
	id := r.PathValue("id")
	follow := r.URL.Query().Get("follow") == "1"

	// Start the stream before writing headers so an error is clean JSON.
	ch, err := s.deps.Docker.ContainerLogs(r.Context(), id, follow)
	if err != nil {
		s.dockerError(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "streaming unsupported")
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
