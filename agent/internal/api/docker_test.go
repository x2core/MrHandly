package api

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/x2core/mrhandly/agent/internal/docker"
	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// dockerStub is a minimal Docker Engine over a temp unix socket for api tests.
type dockerStub struct {
	sock string

	mu   sync.Mutex
	reqs []string
	logs []byte
}

func newDockerStub(t *testing.T) *dockerStub {
	t.Helper()
	d := &dockerStub{sock: filepath.Join(t.TempDir(), "d.sock")}
	ln, err := net.Listen("unix", d.sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1.43/containers/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(readFixture(t, "containers.json"))
	})
	mux.HandleFunc("GET /v1.43/images/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(readFixture(t, "images.json"))
	})
	mux.HandleFunc("GET /v1.43/containers/{id}/json", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "ghost" {
			http.Error(w, "no such container", http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"Id":"abc123def456","Name":"/web","Created":"2023-11-14T22:13:20.5Z","State":{"Status":"running"},"Config":{"Image":"nginx:latest","Tty":false}}`))
	})
	mux.HandleFunc("POST /v1.43/containers/{id}/{action}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1.43/containers/{id}/logs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(d.logs)
	})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.reqs = append(d.reqs, r.Method+" "+r.URL.Path)
		d.mu.Unlock()
		mux.ServeHTTP(w, r)
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { _ = srv.Close() })
	return d
}

func (d *dockerStub) recorded() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.reqs...)
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "docker", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func dframe(st byte, s string) []byte {
	h := make([]byte, 8)
	h[0] = st
	binary.BigEndian.PutUint32(h[4:], uint32(len(s)))
	return append(h, []byte(s)...)
}

// withDocker attaches a Docker client (writable per arg) to a base server.
func withDocker(t *testing.T, writable bool) (*Server, *dockerStub, *bytes.Buffer) {
	t.Helper()
	base, _, auditBuf := newTestServer(t)
	stub := newDockerStub(t)
	base.deps.Docker = docker.New(stub.sock, writable)
	base.deps.DockerWritable = writable
	return base, stub, auditBuf
}

func TestContainersList(t *testing.T) {
	s, _, _ := withDocker(t, true)
	rec := do(t, s, "GET", "/v1/docker/containers", "10.44.0.1:5555")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var cs []protocol.Container
	if err := json.Unmarshal(rec.Body.Bytes(), &cs); err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].Name != "web" || !cs[0].Writable {
		t.Fatalf("containers = %+v", cs)
	}
}

func TestImagesList(t *testing.T) {
	s, _, _ := withDocker(t, true)
	rec := do(t, s, "GET", "/v1/docker/images", "10.44.0.1:5555")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var imgs []protocol.Image
	if err := json.Unmarshal(rec.Body.Bytes(), &imgs); err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 2 {
		t.Fatalf("images = %d", len(imgs))
	}
}

func TestContainerActionAllowed(t *testing.T) {
	s, stub, _ := withDocker(t, true)
	rec := do(t, s, "POST", "/v1/docker/containers/abc123def456/restart", "10.44.0.1:5555")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	found := false
	for _, r := range stub.recorded() {
		if r == "POST /v1.43/containers/abc123def456/restart" {
			found = true
		}
	}
	if !found {
		t.Errorf("restart not sent to engine: %v", stub.recorded())
	}
}

func TestContainerActionReadOnly(t *testing.T) {
	s, stub, auditBuf := withDocker(t, false) // docker_read_only
	rec := do(t, s, "POST", "/v1/docker/containers/abc123def456/stop", "10.44.0.1:5555")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var e protocol.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Code != protocol.ErrDockerReadOnly {
		t.Errorf("code = %q, want docker_read_only", e.Code)
	}
	// Read-only must block before reaching the engine.
	for _, r := range stub.recorded() {
		if strings.Contains(r, "/stop") {
			t.Errorf("read-only action leaked to engine: %v", stub.recorded())
		}
	}
	if log := auditBuf.String(); !strings.Contains(log, "docker.stop") || !strings.Contains(log, "forbidden") {
		t.Errorf("blocked docker action not audited: %s", log)
	}
}

func TestContainerActionUnknownVerb(t *testing.T) {
	s, _, _ := withDocker(t, true)
	rec := do(t, s, "POST", "/v1/docker/containers/abc/frobnicate", "10.44.0.1:5555")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDockerUnavailableWhenNil(t *testing.T) {
	s, _, _ := newTestServer(t) // no Docker
	rec := do(t, s, "GET", "/v1/docker/containers", "10.44.0.1:5555")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "docker_unavailable") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestDockerUnavailableWhenSocketDead(t *testing.T) {
	s, _, _ := newTestServer(t)
	s.deps.Docker = docker.New("/no/such/docker.sock", true)
	s.deps.DockerWritable = true
	rec := do(t, s, "GET", "/v1/docker/containers", "10.44.0.1:5555")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "docker_unavailable") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestDockerRoutesGuardedByPeerAllowlist(t *testing.T) {
	s, _, _ := withDocker(t, true)
	rec := do(t, s, "GET", "/v1/docker/containers", "10.44.0.99:5555")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// TestContainerLogsStreamLive exercises GET /v1/docker/containers/:id/logs over
// real HTTP with a framed stub stream.
func TestContainerLogsStreamLive(t *testing.T) {
	s, stub, _ := withDocker(t, true)
	stub.logs = append(
		dframe(1, "2023-11-14T22:13:20.5Z serving\n"),
		dframe(2, "2023-11-14T22:13:21Z warn\n")...,
	)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/docker/containers/abc123def456/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var line protocol.ContainerLog
	if err := json.Unmarshal(readOneSSEEvent(t, resp.Body), &line); err != nil {
		t.Fatal(err)
	}
	if line.Stream != "stdout" || line.Message != "serving" {
		t.Errorf("first log = %+v", line)
	}
}
