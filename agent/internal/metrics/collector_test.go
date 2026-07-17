package metrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/x2core/mrhandly/agent/internal/procfs"
)

const fixtureRoot = "../../testdata/host-a"

func fixedNow() func() time.Time {
	ts := time.Unix(1_700_000_100, 0)
	return func() time.Time { return ts }
}

func TestCollectFixture(t *testing.T) {
	c := NewCollector(fixtureRoot, fixedNow())
	m, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if m.Timestamp != 1_700_000_100*1000 {
		t.Errorf("Timestamp = %d", m.Timestamp)
	}
	// First collect: usage averaged since boot. cpu total busy=180 idle=1030
	// total=1205 -> 1 - 1030/1205 ~= 0.1452.
	if m.CPU.Usage < 0.14 || m.CPU.Usage > 0.15 {
		t.Errorf("since-boot usage = %f, want ~0.145", m.CPU.Usage)
	}
	if len(m.CPU.PerCore) != 2 {
		t.Errorf("per-core len = %d, want 2", len(m.CPU.PerCore))
	}
	if m.Memory.Total != 16384000*1024 {
		t.Errorf("mem total = %d", m.Memory.Total)
	}
	if m.Memory.Used != satSub(16384000*1024, 10240000*1024) {
		t.Errorf("mem used = %d", m.Memory.Used)
	}
	if m.Memory.SwapUsed != satSub(2097152*1024, 2000000*1024) {
		t.Errorf("swap used = %d", m.Memory.SwapUsed)
	}
	if m.Load.One != 0.52 {
		t.Errorf("load one = %f", m.Load.One)
	}
	if m.UptimeSeconds != 123456.78 {
		t.Errorf("uptime = %f", m.UptimeSeconds)
	}
	if _, ok := m.Network["eth0"]; !ok {
		t.Error("eth0 missing from network")
	}
	if _, ok := m.Network["lo"]; ok {
		t.Error("lo should be excluded")
	}
	if _, ok := m.Disk["nvme0n1"]; !ok {
		t.Error("nvme0n1 missing from disk")
	}
}

// writeProcTree writes a minimal proc tree with the given /proc/stat body and
// static values for the other sources, returning the root.
func writeProcTree(t *testing.T, statBody string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"proc/stat":      statBody,
		"proc/meminfo":   "MemTotal: 1000 kB\nMemFree: 400 kB\nMemAvailable: 600 kB\n",
		"proc/loadavg":   "0.10 0.20 0.30 1/100 42\n",
		"proc/uptime":    "1000.00 900.00\n",
		"proc/net/dev":   "h1\nh2\n  eth0: 1 2 0 0 0 0 0 0 3 4 0 0 0 0 0 0\n",
		"proc/diskstats": "   8 0 sda 1 0 2 0 3 0 4 0 0 0 0\n",
	}
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestCollectDelta drives two reads with advancing counters and checks the
// CPU usage is computed over the interval, not since boot.
func TestCollectDelta(t *testing.T) {
	root := writeProcTree(t, "cpu 100 0 0 900 0 0 0 0\n")
	c := NewCollector(root, fixedNow())

	if _, err := c.Collect(); err != nil { // establishes the baseline
		t.Fatal(err)
	}

	// Advance: +75 busy (user), +25 idle => +100 total, 75% busy.
	statPath := filepath.Join(root, "proc", "stat")
	if err := os.WriteFile(statPath, []byte("cpu 175 0 0 925 0 0 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := c.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if got := m.CPU.Usage; got < 0.74 || got > 0.76 {
		t.Errorf("delta usage = %f, want ~0.75", got)
	}
}

// TestResetInvalidatesBaseline verifies Reset forces the next frame back to
// since-boot semantics (no delta spanning an idle gap).
func TestResetInvalidatesBaseline(t *testing.T) {
	root := writeProcTree(t, "cpu 100 0 0 900 0 0 0 0\n")
	c := NewCollector(root, fixedNow())
	if _, err := c.Collect(); err != nil {
		t.Fatal(err)
	}
	c.Reset()
	if c.prevValid {
		t.Fatal("Reset did not invalidate baseline")
	}
	// After reset, a second identical read reports since-boot usage, not 0.
	m, err := c.Collect()
	if err != nil {
		t.Fatal(err)
	}
	want := usageSinceBoot(procfs.CPUTimes{User: 100, Idle: 900})
	if m.CPU.Usage != want {
		t.Errorf("post-reset usage = %f, want since-boot %f", m.CPU.Usage, want)
	}
}
