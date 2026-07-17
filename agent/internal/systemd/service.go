package systemd

import (
	"context"
	"sort"

	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// Manager projects systemd units as protocol.Services, applying the read scope
// (which units are visible) and annotating each with the write scope (whether
// actions are permitted). Both predicates come from config; the Manager only
// applies them — it does not decide policy.
type Manager struct {
	conn     Conn
	writable func(string) bool
	readable func(string) bool
}

// NewManager wraps conn with the read/write allowlist predicates.
func NewManager(conn Conn, writable, readable func(string) bool) *Manager {
	return &Manager{conn: conn, writable: writable, readable: readable}
}

// Writable reports whether write actions are permitted on unit.
func (m *Manager) Writable(unit string) bool { return m.writable(unit) }

// List returns all readable units as sorted Services.
func (m *Manager) List(ctx context.Context) ([]protocol.Service, error) {
	units, err := m.conn.ListUnits(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Service, 0, len(units))
	for _, u := range units {
		if !m.readable(u.Name) {
			continue
		}
		out = append(out, m.toService(u))
	}
	sortServices(out)
	return out, nil
}

// Get returns one readable unit. A unit outside the read scope is reported as
// not found, so the boundary does not leak which units exist.
func (m *Manager) Get(ctx context.Context, name string) (protocol.Service, error) {
	if !m.readable(name) {
		return protocol.Service{}, ErrUnitNotFound
	}
	u, err := m.conn.GetUnit(ctx, name)
	if err != nil {
		return protocol.Service{}, err
	}
	return m.toService(u), nil
}

// Start/Stop/Restart forward to the connection. The allowlist check happens at
// the handler boundary before these are called (CLAUDE.md §4.3).
func (m *Manager) Start(ctx context.Context, name string) error {
	return m.conn.StartUnit(ctx, name)
}
func (m *Manager) Stop(ctx context.Context, name string) error {
	return m.conn.StopUnit(ctx, name)
}
func (m *Manager) Restart(ctx context.Context, name string) error {
	return m.conn.RestartUnit(ctx, name)
}

// Producer returns an event producer for sampler.NewEvent: it lists once,
// emits a snapshot, then re-emits a fresh snapshot on every unit change. Only
// readable units are ever emitted.
func (m *Manager) Producer() func(ctx context.Context, emit func([]protocol.Service)) {
	return func(ctx context.Context, emit func([]protocol.Service)) {
		state := map[string]protocol.Service{}
		if units, err := m.conn.ListUnits(ctx); err == nil {
			for _, u := range units {
				if m.readable(u.Name) {
					state[u.Name] = m.toService(u)
				}
			}
		}
		emit(snapshot(state))

		changes, err := m.conn.Subscribe(ctx)
		if err != nil {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case ch, ok := <-changes:
				if !ok {
					return
				}
				name := ch.Unit.Name
				if ch.Removed || !m.readable(name) {
					delete(state, name)
				} else {
					state[name] = m.toService(ch.Unit)
				}
				emit(snapshot(state))
			}
		}
	}
}

func (m *Manager) toService(u Unit) protocol.Service {
	return protocol.Service{
		Name:        u.Name,
		Description: u.Description,
		LoadState:   u.LoadState,
		ActiveState: u.ActiveState,
		SubState:    u.SubState,
		Writable:    m.writable(u.Name),
	}
}

func snapshot(state map[string]protocol.Service) []protocol.Service {
	out := make([]protocol.Service, 0, len(state))
	for _, s := range state {
		out = append(out, s)
	}
	sortServices(out)
	return out
}

func sortServices(s []protocol.Service) {
	sort.Slice(s, func(i, j int) bool { return s[i].Name < s[j].Name })
}
