package systemd

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
)

// Real D-Bus implementation of Conn. It is pure Go (godbus over the system bus
// unix socket), so CGO stays off (CLAUDE.md §4.5). Runtime behaviour is
// verified on a real Debian host at the end of the milestone — the sandbox has
// no D-Bus — while the Conn contract itself is covered by the fake.

const (
	dbusDest  = "org.freedesktop.systemd1"
	dbusPath  = dbus.ObjectPath("/org/freedesktop/systemd1")
	mgrIface  = "org.freedesktop.systemd1.Manager"
	unitIface = "org.freedesktop.systemd1.Unit"
	propIface = "org.freedesktop.DBus.Properties"
)

type dbusConn struct {
	conn *dbus.Conn
	mgr  dbus.BusObject
}

// NewConn connects to the system bus and returns a live Conn.
func NewConn() (Conn, error) {
	c, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("systemd: connect system bus: %w", err)
	}
	return &dbusConn{conn: c, mgr: c.Object(dbusDest, dbusPath)}, nil
}

func (d *dbusConn) Close() error { return d.conn.Close() }

// dbusUnit mirrors the a(ssssssouso) struct returned by Manager.ListUnits.
type dbusUnit struct {
	Name        string
	Description string
	LoadState   string
	ActiveState string
	SubState    string
	Following   string
	Path        dbus.ObjectPath
	JobID       uint32
	JobType     string
	JobPath     dbus.ObjectPath
}

func (d *dbusConn) ListUnits(ctx context.Context) ([]Unit, error) {
	var raw []dbusUnit
	if err := d.mgr.CallWithContext(ctx, mgrIface+".ListUnits", 0).Store(&raw); err != nil {
		return nil, fmt.Errorf("systemd: ListUnits: %w", err)
	}
	units := make([]Unit, 0, len(raw))
	for _, u := range raw {
		units = append(units, Unit{
			Name:        u.Name,
			Description: u.Description,
			LoadState:   u.LoadState,
			ActiveState: u.ActiveState,
			SubState:    u.SubState,
		})
	}
	return units, nil
}

func (d *dbusConn) GetUnit(ctx context.Context, name string) (Unit, error) {
	var path dbus.ObjectPath
	if err := d.mgr.CallWithContext(ctx, mgrIface+".GetUnit", 0, name).Store(&path); err != nil {
		return Unit{}, ErrUnitNotFound
	}
	return d.unitFromPath(ctx, name, path)
}

// unitFromPath reads the projected properties off a unit object.
func (d *dbusConn) unitFromPath(ctx context.Context, name string, path dbus.ObjectPath) (Unit, error) {
	obj := d.conn.Object(dbusDest, path)
	u := Unit{Name: name}
	get := func(prop string) string {
		v, err := obj.GetProperty(unitIface + "." + prop)
		if err != nil {
			return ""
		}
		s, _ := v.Value().(string)
		return s
	}
	if u.Name == "" {
		u.Name = get("Id")
	}
	u.Description = get("Description")
	u.LoadState = get("LoadState")
	u.ActiveState = get("ActiveState")
	u.SubState = get("SubState")
	return u, nil
}

func (d *dbusConn) StartUnit(ctx context.Context, name string) error {
	return d.job(ctx, "StartUnit", name)
}

func (d *dbusConn) StopUnit(ctx context.Context, name string) error {
	return d.job(ctx, "StopUnit", name)
}

func (d *dbusConn) RestartUnit(ctx context.Context, name string) error {
	return d.job(ctx, "RestartUnit", name)
}

// job enqueues a unit job in "replace" mode. A returned nil means systemd
// accepted the job; completion is observed via the change stream.
func (d *dbusConn) job(ctx context.Context, method, name string) error {
	var jobPath dbus.ObjectPath
	if err := d.mgr.CallWithContext(ctx, mgrIface+"."+method, 0, name, "replace").Store(&jobPath); err != nil {
		return fmt.Errorf("systemd: %s %s: %w", method, name, err)
	}
	return nil
}

func (d *dbusConn) Subscribe(ctx context.Context) (<-chan UnitChange, error) {
	// Ask systemd to emit unit signals (idempotent, ref-counted).
	if call := d.mgr.CallWithContext(ctx, mgrIface+".Subscribe", 0); call.Err != nil {
		return nil, fmt.Errorf("systemd: Subscribe: %w", call.Err)
	}

	// Match on manager add/remove and on per-unit PropertiesChanged. Using
	// signals (not polling ListUnits) is the whole point — the UI updates on
	// change (docs/ROADMAP.md M2).
	if err := d.conn.AddMatchSignalContext(ctx,
		dbus.WithMatchObjectPath(dbusPath),
		dbus.WithMatchInterface(mgrIface),
		dbus.WithMatchMember("UnitNew"),
	); err != nil {
		return nil, err
	}
	if err := d.conn.AddMatchSignalContext(ctx,
		dbus.WithMatchObjectPath(dbusPath),
		dbus.WithMatchInterface(mgrIface),
		dbus.WithMatchMember("UnitRemoved"),
	); err != nil {
		return nil, err
	}
	if err := d.conn.AddMatchSignalContext(ctx,
		dbus.WithMatchInterface(propIface),
		dbus.WithMatchMember("PropertiesChanged"),
	); err != nil {
		return nil, err
	}

	sigCh := make(chan *dbus.Signal, 64)
	d.conn.Signal(sigCh)

	out := make(chan UnitChange, 64)
	go func() {
		defer close(out)
		defer d.conn.RemoveSignal(sigCh)
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-sigCh:
				if !ok {
					return
				}
				d.handleSignal(ctx, sig, out)
			}
		}
	}()
	return out, nil
}

func (d *dbusConn) handleSignal(ctx context.Context, sig *dbus.Signal, out chan<- UnitChange) {
	switch sig.Name {
	case mgrIface + ".UnitNew":
		if len(sig.Body) < 1 {
			return
		}
		name, _ := sig.Body[0].(string)
		if u, err := d.GetUnit(ctx, name); err == nil {
			send(ctx, out, UnitChange{Unit: u})
		}
	case mgrIface + ".UnitRemoved":
		if len(sig.Body) < 1 {
			return
		}
		name, _ := sig.Body[0].(string)
		send(ctx, out, UnitChange{Unit: Unit{Name: name}, Removed: true})
	case propIface + ".PropertiesChanged":
		if len(sig.Body) < 1 {
			return
		}
		iface, _ := sig.Body[0].(string)
		if iface != unitIface {
			return
		}
		if u, err := d.unitFromPath(ctx, "", sig.Path); err == nil && u.Name != "" {
			send(ctx, out, UnitChange{Unit: u})
		}
	}
}

func send(ctx context.Context, out chan<- UnitChange, c UnitChange) {
	select {
	case out <- c:
	case <-ctx.Done():
	}
}
