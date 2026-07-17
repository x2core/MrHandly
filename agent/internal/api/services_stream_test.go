package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/x2core/mrhandly/agent/internal/journal"
	"github.com/x2core/mrhandly/agent/internal/protocol"
	"github.com/x2core/mrhandly/agent/internal/sampler"
	"github.com/x2core/mrhandly/agent/internal/systemd"
)

// TestServicesStreamLive exercises GET /v1/services/stream over real HTTP: the
// initial snapshot, a live re-emit on a unit change, and subscription cleanup
// on disconnect.
func TestServicesStreamLive(t *testing.T) {
	base, _, _ := newTestServer(t)
	fake := systemd.NewFake(units()...)
	mgr := systemd.NewManager(fake, func(string) bool { return true }, func(string) bool { return true })
	base.deps.Services = mgr
	stream := sampler.NewEvent(mgr.Producer())
	base.deps.ServicesStream = stream

	srv := httptest.NewServer(base.Handler())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/v1/services/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var initial []protocol.Service
	if err := json.Unmarshal(readOneSSEEvent(t, resp.Body), &initial); err != nil {
		t.Fatal(err)
	}
	if len(initial) != 2 {
		t.Fatalf("initial snapshot has %d services, want 2", len(initial))
	}

	// External change: nginx goes inactive → the stream re-emits.
	fake.Emit(systemd.UnitChange{Unit: systemd.Unit{Name: "nginx.service", ActiveState: "inactive", SubState: "dead"}})
	var updated []protocol.Service
	if err := json.Unmarshal(readOneSSEEvent(t, resp.Body), &updated); err != nil {
		t.Fatal(err)
	}
	var nginx protocol.Service
	for _, s := range updated {
		if s.Name == "nginx.service" {
			nginx = s
		}
	}
	if nginx.ActiveState != "inactive" {
		t.Errorf("nginx active = %q, want inactive", nginx.ActiveState)
	}

	cancel()
	resp.Body.Close()
	waitFor(t, func() bool { return stream.Subscribers() == 0 }, "services stream cleanup")
}

// TestServiceLogsLive exercises GET /v1/services/:unit/logs over real HTTP with
// a fake journalctl, confirming a log line is parsed and streamed as SSE.
func TestServiceLogsLive(t *testing.T) {
	base, _, _ := newTestServer(t)
	fake := systemd.NewFake(units()...)
	base.deps.Services = systemd.NewManager(fake, func(string) bool { return true }, func(string) bool { return true })

	script := `printf '%s\n' '{"MESSAGE":"listening on :80","PRIORITY":"6","_SYSTEMD_UNIT":"nginx.service","__REALTIME_TIMESTAMP":"5000000"}'; while true; do sleep 1; done`
	base.deps.Journal = journal.New(func(ctx context.Context, _ string, _ bool, _ int) *exec.Cmd {
		return exec.CommandContext(ctx, "/bin/sh", "-c", script)
	})

	srv := httptest.NewServer(base.Handler())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/v1/services/nginx.service/logs?follow=1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var line protocol.LogLine
	if err := json.Unmarshal(readOneSSEEvent(t, resp.Body), &line); err != nil {
		t.Fatal(err)
	}
	if line.Message != "listening on :80" {
		t.Errorf("message = %q", line.Message)
	}
	if line.Unit != "nginx.service" {
		t.Errorf("unit = %q", line.Unit)
	}
	cancel() // disconnect kills the fake journalctl
}

// TestServiceLogsUnreadableHidden confirms logs for a unit outside the read
// scope are 404, not a leak.
func TestServiceLogsUnreadableHidden(t *testing.T) {
	base, _, _ := newTestServer(t)
	fake := systemd.NewFake(units()...)
	base.deps.Services = systemd.NewManager(fake, func(string) bool { return true },
		func(n string) bool { return n == "nginx.service" })
	base.deps.Journal = journal.New(nil)

	rec := do(t, base, "GET", "/v1/services/sshd.service/logs", "10.44.0.1:5555")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unreadable unit", rec.Code)
	}
}
