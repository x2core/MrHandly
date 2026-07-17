package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/x2core/mrhandly/agent/internal/protocol"
)

func TestProcessesOneShot(t *testing.T) {
	s, _, _ := newTestServer(t)
	rec := do(t, s, "GET", "/v1/processes", "10.44.0.1:5555")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var ps []protocol.Process
	if err := json.Unmarshal(rec.Body.Bytes(), &ps); err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("got %d processes, want 2", len(ps))
	}
	// nginx has the larger RSS; on a CPU tie it sorts first.
	if ps[0].RSS < ps[1].RSS {
		t.Errorf("not sorted by RSS: %+v", ps)
	}
}

func TestProcessesStreamCleansUp(t *testing.T) {
	s, _, _ := newTestServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/processes/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var ps []protocol.Process
	if err := json.Unmarshal(readOneSSEEvent(t, resp.Body), &ps); err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("stream snapshot has %d processes", len(ps))
	}
	resp.Body.Close()
	waitFor(t, func() bool { return s.deps.Processes.Subscribers() == 0 }, "process stream cleanup")
}
