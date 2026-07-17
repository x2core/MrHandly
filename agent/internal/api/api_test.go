package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x2core/mrhandly/agent/internal/audit"
	"github.com/x2core/mrhandly/agent/internal/config"
	"github.com/x2core/mrhandly/agent/internal/fingerprint"
	"github.com/x2core/mrhandly/agent/internal/metrics"
	"github.com/x2core/mrhandly/agent/internal/protocol"
	"github.com/x2core/mrhandly/agent/internal/sampler"
)

const fixtureRoot = "../../testdata/host-a"

func newTestServer(t *testing.T) (*Server, *sampler.Source[protocol.Metrics], *bytes.Buffer) {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "agent.toml")
	cfgBody := `
interface = "wg0"
subnet = "10.44.0.0/24"
peers = ["10.44.0.1", "127.0.0.1", "::1"]
`
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	fp := fingerprint.Detect(fingerprint.Options{
		Root:    fixtureRoot,
		Arch:    "amd64",
		Systemd: func() bool { return true },
		Docker:  func() bool { return false },
	})

	coll := metrics.NewCollector(fixtureRoot, time.Now)
	src := sampler.New(sampler.Config[protocol.Metrics]{
		Interval: 5 * time.Millisecond,
		Sample:   coll.Collect,
		Prime:    coll.Reset,
	})
	oneShot := func() (protocol.Metrics, error) {
		return metrics.NewCollector(fixtureRoot, time.Now).Collect()
	}

	var auditBuf bytes.Buffer
	s := New(Deps{
		Config:      cfg,
		Fingerprint: fp,
		Version:     "v1.0.0",
		Commit:      "cafe",
		Metrics:     src,
		OneShot:     oneShot,
		Audit:       audit.New(&auditBuf),
	})
	return s, src, &auditBuf
}

func do(t *testing.T, s *Server, method, path, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestInfo(t *testing.T) {
	s, _, _ := newTestServer(t)
	rec := do(t, s, "GET", "/v1/info", "10.44.0.1:5555")
	if rec.Code != 200 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var info protocol.Info
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Protocol != protocol.Version {
		t.Errorf("protocol = %d", info.Protocol)
	}
	if info.Host.Hostname != "lab-02" {
		t.Errorf("hostname = %q", info.Host.Hostname)
	}
	if info.Agent.Version != "v1.0.0" {
		t.Errorf("version = %q", info.Agent.Version)
	}
	if !info.Capabilities.Systemd || info.Capabilities.Docker {
		t.Errorf("caps = %+v", info.Capabilities)
	}
}

func TestMetricsOneShot(t *testing.T) {
	s, _, _ := newTestServer(t)
	rec := do(t, s, "GET", "/v1/metrics", "10.44.0.1:5555")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var m protocol.Metrics
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Memory.Total == 0 {
		t.Error("expected non-zero memory")
	}
	if _, ok := m.Network["eth0"]; !ok {
		t.Error("expected eth0 in network")
	}
}

func TestPeerForbidden(t *testing.T) {
	s, _, auditBuf := newTestServer(t)
	rec := do(t, s, "GET", "/v1/info", "10.44.0.99:5555")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var e protocol.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Code != protocol.ErrPeerForbidden {
		t.Errorf("code = %q, want peer_forbidden", e.Code)
	}
	if !strings.Contains(auditBuf.String(), "10.44.0.99") ||
		!strings.Contains(auditBuf.String(), audit.ResultForbidden) {
		t.Errorf("denial not audited: %s", auditBuf.String())
	}
}

func TestMalformedRemoteAddrForbidden(t *testing.T) {
	s, _, _ := newTestServer(t)
	rec := do(t, s, "GET", "/v1/info", "garbage")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for unparseable RemoteAddr", rec.Code)
	}
}

func TestNotFound(t *testing.T) {
	s, _, _ := newTestServer(t)
	rec := do(t, s, "GET", "/v1/nope", "10.44.0.1:5555")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var e protocol.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Code != protocol.ErrNotFound {
		t.Errorf("code = %q", e.Code)
	}
}

// TestMetricsStream exercises the real SSE path over a live server: it reads
// the immediate snapshot, then confirms the subscription is released when the
// client disconnects (the property that makes subscription-driven sampling
// worthwhile).
func TestMetricsStream(t *testing.T) {
	s, src, _ := newTestServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+"/v1/metrics/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q", ct)
	}

	data := readOneSSEEvent(t, resp.Body)
	var m protocol.Metrics
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("stream event not valid metrics: %v (%s)", err, data)
	}
	if m.Memory.Total == 0 {
		t.Error("stream snapshot has zero memory")
	}

	cancel()
	resp.Body.Close()

	// The disconnect must release the subscription and stop sampling.
	waitFor(t, func() bool { return src.Subscribers() == 0 }, "subscriber cleanup after disconnect")
}

func readOneSSEEvent(t *testing.T, r io.Reader) []byte {
	t.Helper()
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			return []byte(after)
		}
	}
	t.Fatal("no SSE data event received")
	return nil
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
