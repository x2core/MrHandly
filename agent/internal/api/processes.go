package api

import (
	"net/http"

	"github.com/x2core/mrhandly/agent/internal/protocol"
)

func (s *Server) handleProcesses(w http.ResponseWriter, _ *http.Request) {
	procs, err := s.deps.ProcessesOneShot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "failed to read processes")
		return
	}
	writeJSON(w, http.StatusOK, procs)
}

// handleProcessesStream streams the process table as SSE. Subscribing starts
// the 2s process sampler; the disconnect below stops it — the whole point of
// the subscription-driven design (CLAUDE.md §8).
func (s *Server) handleProcessesStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "streaming unsupported")
		return
	}
	setSSEHeaders(w)

	if procs, err := s.deps.ProcessesOneShot(); err == nil {
		writeSSE(w, procs)
		flusher.Flush()
	}

	id, ch := s.deps.Processes.Subscribe()
	defer s.deps.Processes.Unsubscribe(id)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case procs, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, procs)
			flusher.Flush()
		}
	}
}
