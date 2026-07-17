package procfs

import "testing"

// BenchmarkStat guards the hottest read (1 Hz aggregate CPU). Steady-state it
// must not allocate — the buffer and PerCPU slice are reused. `make bench` runs
// this in CI as a regression gate (CLAUDE.md §8).
func BenchmarkStat(b *testing.B) {
	r := New(fixtureRoot)
	var s Stat
	if err := r.Stat(&s); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := r.Stat(&s); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProcesses guards the expensive process-table read (~pids × stat).
func BenchmarkProcesses(b *testing.B) {
	r := New(fixtureRoot)
	if _, err := r.Processes(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Processes(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseStat isolates the parser from file I/O.
func BenchmarkParseStat(b *testing.B) {
	r := New(fixtureRoot)
	buf, err := r.slurp(r.statPath)
	if err != nil {
		b.Fatal(err)
	}
	// Copy out of the reusable buffer so the parser reads a stable slice.
	data := append([]byte(nil), buf...)
	var s Stat
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseStat(data, &s)
	}
}
