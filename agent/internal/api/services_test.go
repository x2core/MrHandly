package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/x2core/mrhandly/agent/internal/protocol"
	"github.com/x2core/mrhandly/agent/internal/systemd"
)

func units() []systemd.Unit {
	return []systemd.Unit{
		{Name: "nginx.service", Description: "web", LoadState: "loaded", ActiveState: "active", SubState: "running"},
		{Name: "sshd.service", Description: "ssh", LoadState: "loaded", ActiveState: "active", SubState: "running"},
	}
}

// withServices adds a systemd Manager (backed by a fake) to a base server. Only
// nginx.service is writable; both are readable. Returns the audit buffer too.
func withServices(t *testing.T) (*Server, *systemd.Fake, *bytes.Buffer) {
	t.Helper()
	base, _, auditBuf := newTestServer(t)
	fake := systemd.NewFake(units()...)
	mgr := systemd.NewManager(fake,
		func(n string) bool { return n == "nginx.service" }, // writable
		func(string) bool { return true },                   // readable
	)
	base.deps.Services = mgr
	return base, fake, auditBuf
}

func TestServicesList(t *testing.T) {
	s, _, _ := withServices(t)
	rec := do(t, s, "GET", "/v1/services", "10.44.0.1:5555")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var list []protocol.Service
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d services", len(list))
	}
	if list[0].Name != "nginx.service" || !list[0].Writable {
		t.Errorf("nginx should be first and writable: %+v", list[0])
	}
	if list[1].Writable {
		t.Errorf("sshd should not be writable")
	}
}

func TestServiceGet(t *testing.T) {
	s, _, _ := withServices(t)
	rec := do(t, s, "GET", "/v1/services/sshd.service", "10.44.0.1:5555")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var svc protocol.Service
	if err := json.Unmarshal(rec.Body.Bytes(), &svc); err != nil {
		t.Fatal(err)
	}
	if svc.Name != "sshd.service" {
		t.Errorf("name = %q", svc.Name)
	}
}

func TestServiceGetNotFound(t *testing.T) {
	s, _, _ := withServices(t)
	rec := do(t, s, "GET", "/v1/services/ghost.service", "10.44.0.1:5555")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestServiceActionAllowed(t *testing.T) {
	s, fake, _ := withServices(t)
	rec := do(t, s, "POST", "/v1/services/nginx.service/restart", "10.44.0.1:5555")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	acts := fake.Actions()
	if len(acts) != 1 || acts[0] != (systemd.Action{Verb: "restart", Unit: "nginx.service"}) {
		t.Fatalf("actions = %+v", acts)
	}
}

func TestServiceActionForbiddenByAllowlist(t *testing.T) {
	s, fake, auditBuf := withServices(t)
	rec := do(t, s, "POST", "/v1/services/sshd.service/restart", "10.44.0.1:5555")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var e protocol.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Code != protocol.ErrUnitNotAllowed {
		t.Errorf("code = %q, want unit_not_allowed", e.Code)
	}
	// The write must NOT have reached the D-Bus layer.
	if len(fake.Actions()) != 0 {
		t.Errorf("forbidden action leaked to conn: %+v", fake.Actions())
	}
	// The blocked attempt must be audited.
	if log := auditBuf.String(); !strings.Contains(log, "service.restart") ||
		!strings.Contains(log, "sshd.service") || !strings.Contains(log, "forbidden") {
		t.Errorf("blocked action not audited: %s", log)
	}
}

func TestServiceActionUnknownVerb(t *testing.T) {
	s, _, _ := withServices(t)
	rec := do(t, s, "POST", "/v1/services/nginx.service/frobnicate", "10.44.0.1:5555")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestServicesUnavailableWhenNoSystemd(t *testing.T) {
	// Base server has no Services set → systemd_unavailable.
	s, _, _ := newTestServer(t)
	rec := do(t, s, "GET", "/v1/services", "10.44.0.1:5555")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "systemd_unavailable") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestServicesRoutesStillGuardedByPeerAllowlist(t *testing.T) {
	s, _, _ := withServices(t)
	rec := do(t, s, "GET", "/v1/services", "10.44.0.99:5555")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (peer guard fronts services too)", rec.Code)
	}
}
