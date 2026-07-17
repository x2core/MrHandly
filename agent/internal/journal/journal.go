// Package journal streams journald entries by shelling out to
// `journalctl -o json`. That subprocess is the sanctioned path: sd-journal is
// C-only and the on-disk format is internal, not an ABI, so parsing
// /var/log/journal directly is off the table (docs/ROADMAP.md M2).
//
// The one bug this package exists to prevent is a leaked journalctl process
// when a client disconnects. Every stream is tied to a context: cancelling it
// kills the process and the reader goroutine reaps it before closing the
// output channel. The command is injectable so this lifecycle is testable
// without journalctl present (CLAUDE.md §5).
package journal

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"strconv"

	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// DefaultBacklog is the number of historical lines fetched before following.
const DefaultBacklog = 100

// CommandFunc builds the journalctl invocation. It is injectable so tests can
// substitute a fake process. The command MUST be context-bound (built with
// exec.CommandContext) so cancellation kills it.
type CommandFunc func(ctx context.Context, unit string, follow bool, backlog int) *exec.Cmd

// DefaultCommand runs journalctl for a single unit, JSON output, no pager.
func DefaultCommand(ctx context.Context, unit string, follow bool, backlog int) *exec.Cmd {
	args := []string{"-o", "json", "--no-pager", "-u", unit}
	if backlog > 0 {
		args = append(args, "-n", strconv.Itoa(backlog))
	}
	if follow {
		args = append(args, "-f")
	}
	return exec.CommandContext(ctx, "journalctl", args...)
}

// Streamer produces journald line streams.
type Streamer struct {
	command CommandFunc
	backlog int
}

// New returns a Streamer. A nil command uses DefaultCommand.
func New(command CommandFunc) *Streamer {
	if command == nil {
		command = DefaultCommand
	}
	return &Streamer{command: command, backlog: DefaultBacklog}
}

// Stream starts journalctl for unit and returns a channel of parsed lines. The
// channel is closed — and the subprocess reaped — when ctx is cancelled or the
// process exits. The buffer bounds the backlog so a slow consumer applies
// backpressure to journalctl rather than growing memory without limit.
func (s *Streamer) Stream(ctx context.Context, unit string, follow bool) (<-chan protocol.LogLine, error) {
	cmd := s.command(ctx, unit, follow, s.backlog)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	out := make(chan protocol.LogLine, 64)
	go func() {
		defer close(out)
		// Reap the process no matter how we leave: normal EOF, parse loop
		// exit, or context cancellation killing it. This is the anti-orphan
		// guarantee.
		defer func() { _ = cmd.Wait() }()

		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line, ok := parseLine(sc.Bytes())
			if !ok {
				continue
			}
			select {
			case out <- line:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// journalEntry is the subset of journald's JSON fields we project. journald
// encodes every value as a string (or, for non-UTF-8 payloads, an array of
// byte values); RawMessage lets us handle both.
type journalEntry struct {
	Message  json.RawMessage `json:"MESSAGE"`
	Priority string          `json:"PRIORITY"`
	Unit     string          `json:"_SYSTEMD_UNIT"`
	Realtime string          `json:"__REALTIME_TIMESTAMP"`
}

func parseLine(b []byte) (protocol.LogLine, bool) {
	var e journalEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return protocol.LogLine{}, false
	}
	ll := protocol.LogLine{
		Message:  decodeMessage(e.Message),
		Priority: parsePriority(e.Priority),
		Unit:     e.Unit,
	}
	// __REALTIME_TIMESTAMP is microseconds since the epoch.
	if micros, err := strconv.ParseInt(e.Realtime, 10, 64); err == nil {
		ll.Timestamp = micros / 1000
	}
	return ll, true
}

// decodeMessage handles both the string form and journald's byte-array form
// (used when a message is not valid UTF-8).
func decodeMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var bytesArr []byte
	if err := json.Unmarshal(raw, &bytesArr); err == nil {
		return string(bytesArr)
	}
	return ""
}

func parsePriority(s string) int {
	if s == "" {
		return 6 // default: info
	}
	p, err := strconv.Atoi(s)
	if err != nil || p < 0 || p > 7 {
		return 6
	}
	return p
}
