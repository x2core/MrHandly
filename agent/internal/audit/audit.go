// Package audit records security-relevant actions to the agent's own journal
// (stderr → journald) as structured JSON lines. Every write action and every
// rejected request is logged with the actor peer IP, the action, the target,
// and the result (CLAUDE.md §4.7).
//
// The audit log is append-only observability, not application state — it
// carries no logic and makes no decisions.
package audit

import (
	"context"
	"io"
	"log/slog"
)

// Result values for the standard outcomes.
const (
	ResultOK        = "ok"
	ResultForbidden = "forbidden"
	ResultError     = "error"
)

// Logger writes audit events as JSON.
type Logger struct {
	l *slog.Logger
}

// New returns a Logger writing JSON events to w (os.Stderr in production).
func New(w io.Writer) *Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &Logger{l: slog.New(h)}
}

// Log records one audit event. actor is the peer IP, action is the operation
// (e.g. "request", "service.restart"), target is what it acted on (e.g. a
// method+path or a unit name), and result is the outcome.
func (a *Logger) Log(actor, action, target, result string) {
	a.l.LogAttrs(context.Background(), slog.LevelInfo, "audit",
		slog.String("actor", actor),
		slog.String("action", action),
		slog.String("target", target),
		slog.String("result", result),
	)
}
