package api

import (
	"encoding/json"
	"net/http"

	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// handleMetricsStream streams metrics frames as Server-Sent Events. It sends an
// immediate snapshot so the client never stares at an empty panel waiting for
// the first tick, then subscribes to the sampler and forwards each frame.
//
// The subscription is released when the client disconnects (r.Context() is
// cancelled) — this is what makes subscription-driven sampling pay off: close
// the last stream and the sampler stops reading /proc entirely.
func (s *Server) handleMetricsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, "streaming unsupported")
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Immediate snapshot.
	if m, err := s.deps.OneShot(); err == nil {
		writeSSE(w, m)
		flusher.Flush()
	}

	id, ch := s.deps.Metrics.Subscribe()
	defer s.deps.Metrics.Unsubscribe(id)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, m)
			flusher.Flush()
		}
	}
}

// writeSSE writes one SSE "data:" event carrying the JSON encoding of v.
func writeSSE(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n\n"))
}
