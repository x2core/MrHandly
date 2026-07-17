package systemd

import (
	"context"
	"testing"
	"time"

	"github.com/x2core/mrhandly/agent/internal/protocol"
)

func units() []Unit {
	return []Unit{
		{Name: "nginx.service", Description: "web", LoadState: "loaded", ActiveState: "active", SubState: "running"},
		{Name: "sshd.service", Description: "ssh", LoadState: "loaded", ActiveState: "active", SubState: "running"},
		{Name: "cron.timer", Description: "cron", LoadState: "loaded", ActiveState: "active", SubState: "waiting"},
	}
}

// allowWrite/allowRead build predicates from a fixed set.
func allow(set ...string) func(string) bool {
	m := map[string]bool{}
	for _, s := range set {
		m[s] = true
	}
	return func(name string) bool { return m[name] }
}

func allowAll() func(string) bool { return func(string) bool { return true } }

func TestManagerListAppliesReadScopeAndWriteFlag(t *testing.T) {
	f := NewFake(units()...)
	// Read scope: only *.service (drop cron.timer). Write scope: nginx only.
	m := NewManager(f, allow("nginx.service"), func(n string) bool {
		return n == "nginx.service" || n == "sshd.service"
	})

	list, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d services, want 2 (cron.timer filtered out)", len(list))
	}
	if list[0].Name != "nginx.service" || list[1].Name != "sshd.service" {
		t.Fatalf("unexpected order/content: %+v", list)
	}
	if !list[0].Writable {
		t.Error("nginx.service should be writable")
	}
	if list[1].Writable {
		t.Error("sshd.service should not be writable")
	}
}

func TestManagerGetHidesUnreadable(t *testing.T) {
	f := NewFake(units()...)
	m := NewManager(f, allowAll(), allow("nginx.service"))

	if _, err := m.Get(context.Background(), "sshd.service"); err != ErrUnitNotFound {
		t.Errorf("unreadable unit should be ErrUnitNotFound, got %v", err)
	}
	svc, err := m.Get(context.Background(), "nginx.service")
	if err != nil {
		t.Fatal(err)
	}
	if svc.ActiveState != "active" {
		t.Errorf("state = %q", svc.ActiveState)
	}
}

func TestManagerActionsRecorded(t *testing.T) {
	f := NewFake(units()...)
	m := NewManager(f, allowAll(), allowAll())
	ctx := context.Background()

	if err := m.Stop(ctx, "nginx.service"); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx, "nginx.service"); err != nil {
		t.Fatal(err)
	}
	acts := f.Actions()
	if len(acts) != 2 || acts[0] != (Action{"stop", "nginx.service"}) || acts[1] != (Action{"start", "nginx.service"}) {
		t.Fatalf("actions = %+v", acts)
	}
}

// TestProducerEmitsSnapshotThenChanges verifies the services source emits an
// initial snapshot and re-emits on each unit change (event-driven, not tick).
func TestProducerEmitsSnapshotThenChanges(t *testing.T) {
	f := NewFake(units()...)
	m := NewManager(f, allowAll(), allowAll())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	emits := make(chan []protocol.Service, 8)
	go m.Producer()(ctx, func(s []protocol.Service) { emits <- s })

	// Initial snapshot: 3 units.
	first := <-emits
	if len(first) != 3 {
		t.Fatalf("initial snapshot has %d units, want 3", len(first))
	}

	// Drive an external change: nginx goes inactive.
	f.Emit(UnitChange{Unit: Unit{Name: "nginx.service", ActiveState: "inactive", SubState: "dead"}})
	select {
	case next := <-emits:
		var nginx protocol.Service
		for _, s := range next {
			if s.Name == "nginx.service" {
				nginx = s
			}
		}
		if nginx.ActiveState != "inactive" {
			t.Errorf("nginx active = %q, want inactive", nginx.ActiveState)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no re-emit on unit change")
	}
}
