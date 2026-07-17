package docker

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// TestDemuxFraming checks the length-prefixed stdout/stderr framing is split
// correctly and stream types are assigned.
func TestDemuxFraming(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(frame(1, "hello stdout\n"))
	buf.Write(frame(2, "oops stderr\n"))
	buf.Write(frame(1, "two\nlines\n"))

	var got []protocol.ContainerLog
	demux(&buf, false, func(stream string, payload []byte) bool {
		for _, l := range splitLines(payload) {
			got = append(got, protocol.ContainerLog{Stream: stream, Message: l})
		}
		return true
	})

	want := []protocol.ContainerLog{
		{Stream: "stdout", Message: "hello stdout"},
		{Stream: "stderr", Message: "oops stderr"},
		{Stream: "stdout", Message: "two"},
		{Stream: "stdout", Message: "lines"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDemuxTTYRaw(t *testing.T) {
	// TTY streams are not framed — raw newline-delimited text.
	r := bytes.NewBufferString("line a\nline b\n")
	var got []string
	demux(r, true, func(stream string, payload []byte) bool {
		if stream != "stdout" {
			t.Errorf("tty stream = %q, want stdout", stream)
		}
		got = append(got, string(payload))
		return true
	})
	if len(got) != 2 || got[0] != "line a" || got[1] != "line b" {
		t.Fatalf("tty demux = %v", got)
	}
}

func TestSplitTimestamp(t *testing.T) {
	ts, msg := splitTimestamp("2023-11-14T22:13:20.5Z hello world")
	if msg != "hello world" {
		t.Errorf("msg = %q", msg)
	}
	want := time.Date(2023, 11, 14, 22, 13, 20, 500_000_000, time.UTC).UnixMilli()
	if ts != want {
		t.Errorf("ts = %d, want %d", ts, want)
	}

	// No parseable timestamp: whole line is the message.
	ts2, msg2 := splitTimestamp("plain message no ts")
	if ts2 != 0 || msg2 != "plain message no ts" {
		t.Errorf("fallback = %d %q", ts2, msg2)
	}
}

func TestContainerLogsStream(t *testing.T) {
	d := newTestDaemon(t, true)
	d.logStream = append(
		frame(1, "2023-11-14T22:13:20.5Z listening on :80\n"),
		frame(2, "2023-11-14T22:13:21Z a warning\n")...,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := d.client.ContainerLogs(ctx, "abc123def456", false)
	if err != nil {
		t.Fatal(err)
	}

	first := <-ch
	if first.Stream != "stdout" || first.Message != "listening on :80" {
		t.Errorf("first = %+v", first)
	}
	if first.Timestamp == 0 {
		t.Error("expected parsed timestamp")
	}
	second := <-ch
	if second.Stream != "stderr" || second.Message != "a warning" {
		t.Errorf("second = %+v", second)
	}
}

func TestContainerLogsNotFound(t *testing.T) {
	d := newTestDaemon(t, true)
	_, err := d.client.ContainerLogs(context.Background(), "ghost", false)
	if err == nil {
		t.Fatal("expected error for missing container")
	}
}
