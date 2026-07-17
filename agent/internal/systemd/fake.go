package systemd

import (
	"context"
	"sync"
)

// Fake is an in-memory Conn for tests. It records write actions, mutates unit
// state on those actions, and fans state changes out to Subscribe channels —
// enough to exercise the services projection, the allowlist boundary, and the
// SSE stream with no D-Bus (CLAUDE.md §5). It is exported so other packages'
// tests (e.g. api) can use it too.
type Fake struct {
	mu       sync.Mutex
	units    map[string]Unit
	order    []string          // preserves ListUnits ordering
	subs     map[int]chan UnitChange
	nextSub  int
	actions  []Action          // recorded write actions
	FailWith error             // if set, write actions return this error
}

// Action is a recorded write against the fake.
type Action struct {
	Verb string // start | stop | restart
	Unit string
}

// NewFake builds a Fake seeded with the given units, in order.
func NewFake(units ...Unit) *Fake {
	f := &Fake{units: make(map[string]Unit), subs: make(map[int]chan UnitChange)}
	for _, u := range units {
		f.units[u.Name] = u
		f.order = append(f.order, u.Name)
	}
	return f
}

// Actions returns a copy of the recorded write actions.
func (f *Fake) Actions() []Action {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Action(nil), f.actions...)
}

func (f *Fake) ListUnits(context.Context) ([]Unit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Unit, 0, len(f.order))
	for _, name := range f.order {
		out = append(out, f.units[name])
	}
	return out, nil
}

func (f *Fake) GetUnit(_ context.Context, name string) (Unit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.units[name]
	if !ok {
		return Unit{}, ErrUnitNotFound
	}
	return u, nil
}

func (f *Fake) StartUnit(_ context.Context, name string) error {
	return f.apply("start", name, "active", "running")
}

func (f *Fake) StopUnit(_ context.Context, name string) error {
	return f.apply("stop", name, "inactive", "dead")
}

func (f *Fake) RestartUnit(_ context.Context, name string) error {
	return f.apply("restart", name, "active", "running")
}

func (f *Fake) apply(verb, name, active, sub string) error {
	f.mu.Lock()
	f.actions = append(f.actions, Action{Verb: verb, Unit: name})
	if f.FailWith != nil {
		f.mu.Unlock()
		return f.FailWith
	}
	u, ok := f.units[name]
	if ok {
		u.ActiveState = active
		u.SubState = sub
		f.units[name] = u
	}
	f.mu.Unlock()
	if ok {
		f.Emit(UnitChange{Unit: u})
	}
	return nil
}

// Emit pushes a change to every active subscriber (used by apply and directly
// by tests to simulate an external state transition).
func (f *Fake) Emit(c UnitChange) {
	f.mu.Lock()
	chans := make([]chan UnitChange, 0, len(f.subs))
	for _, ch := range f.subs {
		chans = append(chans, ch)
	}
	f.mu.Unlock()
	for _, ch := range chans {
		select {
		case ch <- c:
		default:
		}
	}
}

func (f *Fake) Subscribe(ctx context.Context) (<-chan UnitChange, error) {
	f.mu.Lock()
	id := f.nextSub
	f.nextSub++
	ch := make(chan UnitChange, 16)
	f.subs[id] = ch
	f.mu.Unlock()

	go func() {
		<-ctx.Done()
		f.mu.Lock()
		delete(f.subs, id)
		close(ch)
		f.mu.Unlock()
	}()
	return ch, nil
}

func (f *Fake) Close() error { return nil }
