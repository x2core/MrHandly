// Package systemd projects the host's systemd units over D-Bus. The Conn
// interface is the seam that makes the whole package testable without systemd
// or root (CLAUDE.md §5): the real implementation talks to
// org.freedesktop.systemd1 via godbus, and a fake drives the same contract
// from recorded state in tests.
//
// Unit-allowlist enforcement lives at the API handler boundary, not here — the
// D-Bus layer is mechanism, the allowlist is policy (CLAUDE.md §2, §4.3).
package systemd

import (
	"context"
	"errors"
)

// ErrUnitNotFound is returned by GetUnit when systemd has no such unit.
var ErrUnitNotFound = errors.New("systemd: unit not found")

// Unit is the subset of a systemd unit's state the agent projects.
type Unit struct {
	Name        string
	Description string
	LoadState   string // loaded | not-found | error | masked …
	ActiveState string // active | inactive | failed | activating | deactivating …
	SubState    string // running | dead | exited | listening …
}

// UnitChange is a single unit state transition delivered over Subscribe. When
// Removed is true, only Unit.Name is meaningful.
type UnitChange struct {
	Unit    Unit
	Removed bool
}

// Conn is the agent's view of systemd. Read methods are free; the write
// methods (Start/Stop/Restart) are gated by the unit allowlist at the handler
// boundary before they are ever called.
type Conn interface {
	// ListUnits returns all currently loaded units.
	ListUnits(ctx context.Context) ([]Unit, error)
	// GetUnit returns one unit's state, or ErrUnitNotFound.
	GetUnit(ctx context.Context, name string) (Unit, error)
	// StartUnit enqueues a start job for name.
	StartUnit(ctx context.Context, name string) error
	// StopUnit enqueues a stop job for name.
	StopUnit(ctx context.Context, name string) error
	// RestartUnit enqueues a restart job for name.
	RestartUnit(ctx context.Context, name string) error
	// Subscribe returns a channel of unit state changes driven by D-Bus
	// signals. The channel is closed when ctx is cancelled.
	Subscribe(ctx context.Context) (<-chan UnitChange, error)
	// Close releases the underlying bus connection.
	Close() error
}
