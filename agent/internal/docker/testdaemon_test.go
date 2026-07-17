package docker

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// testDaemon is a fake Docker Engine served over a temp unix socket, replaying
// recorded Engine API payloads (CLAUDE.md §5 testability requirement).
type testDaemon struct {
	client *Client

	mu       sync.Mutex
	requests []string // "METHOD path"

	ttyLogs   bool   // inspect reports Tty=true
	logStream []byte // bytes served for the logs endpoint
	failStart bool   // start returns 500
}

func newTestDaemon(t *testing.T, writable bool) *testDaemon {
	t.Helper()
	d := &testDaemon{}

	sock := filepath.Join(t.TempDir(), "docker.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("GET /v1.43/containers/json", func(w http.ResponseWriter, _ *http.Request) {
		serveFixture(t, w, "containers.json")
	})
	mux.HandleFunc("GET /v1.43/images/json", func(w http.ResponseWriter, _ *http.Request) {
		serveFixture(t, w, "images.json")
	})
	mux.HandleFunc("GET /v1.43/containers/{id}/json", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id != "abc123def456" && id != "web" {
			http.Error(w, "no such container", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"Id":%q,"Name":"/web","Created":"2023-11-14T22:13:20.5Z","State":{"Status":"running"},"Config":{"Image":"nginx:latest","Tty":%t}}`, id, d.ttyLogs)
	})
	mux.HandleFunc("POST /v1.43/containers/{id}/start", func(w http.ResponseWriter, _ *http.Request) {
		if d.failStart {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /v1.43/containers/{id}/stop", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /v1.43/containers/{id}/restart", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1.43/containers/{id}/logs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(d.logStream)
	})

	// Record every request before dispatching.
	recorder := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.requests = append(d.requests, r.Method+" "+r.URL.Path)
		d.mu.Unlock()
		mux.ServeHTTP(w, r)
	})

	srv := &http.Server{Handler: recorder}
	go srv.Serve(ln)
	t.Cleanup(func() { _ = srv.Close() })

	d.client = New(sock, writable)
	return d
}

func (d *testDaemon) recorded() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.requests...)
}

func serveFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "docker", name))
	if err != nil {
		t.Fatal(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// frame builds one Docker multiplexed stream frame for the given stream type
// (1=stdout, 2=stderr) and payload.
func frame(streamType byte, payload string) []byte {
	hdr := make([]byte, 8)
	hdr[0] = streamType
	binary.BigEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	return append(hdr, []byte(payload)...)
}
