package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeProcTable writes a minimal proc tree: the aggregate /proc/stat plus one
// process whose CPU jiffies (utime) and rss we control.
func writeProcTable(t *testing.T, cpuTotalLine string, pid, utime, rss int) string {
	t.Helper()
	root := t.TempDir()
	must := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(root, "proc", "stat"), cpuTotalLine+"\n")
	// state + 21 tokens; utime at token index 11, rss at index 21.
	stat := fmt.Sprintf("%d (proc) S 1 1 1 0 -1 0 0 0 0 0 %d 0 0 0 20 0 1 0 0 0 %d\n", pid, utime, rss)
	must(filepath.Join(root, "proc", fmt.Sprintf("%d", pid), "stat"), stat)
	must(filepath.Join(root, "proc", fmt.Sprintf("%d", pid), "cmdline"), "")
	return root
}

func TestProcessCollectorDelta(t *testing.T) {
	root := writeProcTable(t, "cpu 100 0 0 900 0 0 0 0", 500, 100, 2000)
	c := NewProcessCollector(root)

	first, err := c.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("got %d procs", len(first))
	}
	if first[0].CPU != 0 {
		t.Errorf("first frame CPU = %f, want 0 (no baseline)", first[0].CPU)
	}
	if first[0].RSS != 2000*uint64(os.Getpagesize()) {
		t.Errorf("rss = %d", first[0].RSS)
	}

	// Advance: total +100 (cpu 175 idle 925), process utime +75 => 75% of machine.
	writeInto := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeInto(filepath.Join(root, "proc", "stat"), "cpu 175 0 0 925 0 0 0 0\n")
	writeInto(filepath.Join(root, "proc", "500", "stat"),
		"500 (proc) S 1 1 1 0 -1 0 0 0 0 0 175 0 0 0 20 0 1 0 0 0 2000\n")

	second, err := c.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if got := second[0].CPU; got < 0.74 || got > 0.76 {
		t.Errorf("delta CPU = %f, want ~0.75", got)
	}
}

func TestProcessCollectorSortsByCPU(t *testing.T) {
	c := NewProcessCollector(fixtureRoot)
	// Two collects so the second has deltas (fixture jiffies are static => 0,
	// but the sort still runs; assert it does not error and returns both pids).
	if _, err := c.Collect(); err != nil {
		t.Fatal(err)
	}
	ps, err := c.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("got %d procs from fixture, want 2", len(ps))
	}
	// Sorted by CPU desc then RSS desc; nginx (rss 2000 pages) outranks app.
	if ps[0].RSS < ps[1].RSS {
		t.Errorf("not sorted by RSS on CPU tie: %+v", ps)
	}
}
