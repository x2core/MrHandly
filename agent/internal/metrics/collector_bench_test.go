package metrics

import (
	"testing"
	"time"
)

// BenchmarkCollect measures a full metrics sample — the work done on every
// sampler tick (all six procfs sources + CPU delta + mapping).
func BenchmarkCollect(b *testing.B) {
	c := NewCollector(fixtureRoot, time.Now)
	if _, err := c.Collect(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Collect(); err != nil {
			b.Fatal(err)
		}
	}
}
