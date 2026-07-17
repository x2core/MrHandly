package journal

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestParseLine(t *testing.T) {
	raw := `{"MESSAGE":"hello world","PRIORITY":"3","_SYSTEMD_UNIT":"nginx.service","__REALTIME_TIMESTAMP":"1700000000000000"}`
	ll, ok := parseLine([]byte(raw))
	if !ok {
		t.Fatal("parse failed")
	}
	if ll.Message != "hello world" {
		t.Errorf("message = %q", ll.Message)
	}
	if ll.Priority != 3 {
		t.Errorf("priority = %d", ll.Priority)
	}
	if ll.Unit != "nginx.service" {
		t.Errorf("unit = %q", ll.Unit)
	}
	if ll.Timestamp != 1_700_000_000_000 { // micros -> millis
		t.Errorf("timestamp = %d", ll.Timestamp)
	}
}

func TestParseLineDefaultsAndByteMessage(t *testing.T) {
	// No priority => default 6; message as a byte array (non-UTF-8 form).
	raw := `{"MESSAGE":[104,105],"__REALTIME_TIMESTAMP":"1000"}`
	ll, ok := parseLine([]byte(raw))
	if !ok {
		t.Fatal("parse failed")
	}
	if ll.Message != "hi" {
		t.Errorf("message = %q, want hi", ll.Message)
	}
	if ll.Priority != 6 {
		t.Errorf("priority = %d, want default 6", ll.Priority)
	}
}

func TestParseLineRejectsGarbage(t *testing.T) {
	if _, ok := parseLine([]byte("not json")); ok {
		t.Error("expected garbage to be rejected")
	}
}

// shellCommand builds a context-bound /bin/sh command from a script and records
// the *exec.Cmd so the test can inspect the spawned process.
func shellCommand(script string, captured *captured) CommandFunc {
	return func(ctx context.Context, _ string, _ bool, _ int) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
		captured.set(cmd)
		return cmd
	}
}

type captured struct {
	mu  sync.Mutex
	cmd *exec.Cmd
}

func (c *captured) set(cmd *exec.Cmd) { c.mu.Lock(); c.cmd = cmd; c.mu.Unlock() }
func (c *captured) pid() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

func TestStreamParsesAndCloses(t *testing.T) {
	script := `printf '%s\n' '{"MESSAGE":"line one","PRIORITY":"6","_SYSTEMD_UNIT":"nginx.service","__REALTIME_TIMESTAMP":"2000000"}'`
	s := New(shellCommand(script, &captured{}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := s.Stream(ctx, "nginx.service", false)
	if err != nil {
		t.Fatal(err)
	}

	ll, ok := <-ch
	if !ok {
		t.Fatal("expected a line")
	}
	if ll.Message != "line one" {
		t.Errorf("message = %q", ll.Message)
	}
	// The process exits on its own; the channel must then close.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to close after process exit")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after process exit")
	}
}

// TestStreamNoOrphanOnCancel is the anti-orphan guarantee: a follow stream runs
// a long-lived process; cancelling the context must kill and reap it. The
// output channel closes only after cmd.Wait() returns, so a clean close is
// proof the process was reaped, not leaked.
func TestStreamNoOrphanOnCancel(t *testing.T) {
	cap := &captured{}
	// Emit one line, then follow forever.
	script := `printf '%s\n' '{"MESSAGE":"tail","__REALTIME_TIMESTAMP":"3000"}'; while true; do sleep 1; done`
	s := New(shellCommand(script, cap))

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := s.Stream(ctx, "nginx.service", true)
	if err != nil {
		t.Fatal(err)
	}

	if ll, ok := <-ch; !ok || ll.Message != "tail" {
		t.Fatalf("first line = %q ok=%v", ll.Message, ok)
	}
	pid := cap.pid()
	if pid == 0 {
		t.Fatal("no pid captured")
	}

	// Client disconnects.
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			// Drain any buffered line, then require close.
			for range ch {
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("channel did not close after cancel — process leaked")
	}

	// Secondary check: the process should no longer be signalable.
	if err := syscall.Kill(pid, 0); err == nil {
		t.Errorf("process %d still alive after cancel — orphaned", pid)
	}
}
