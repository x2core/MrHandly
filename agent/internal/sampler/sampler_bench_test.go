package sampler

import (
	"testing"
	"time"
)

// BenchmarkBroadcast measures the fan-out cost of one tick to N subscribers
// that are keeping up. This is the sampler-tick hot path the budget cares
// about (CLAUDE.md §8); `make bench` runs it in CI as a regression gate.
func BenchmarkBroadcast(b *testing.B) {
	for _, n := range []int{1, 10, 100} {
		b.Run(subsLabel(n), func(b *testing.B) {
			s := New(Config[int]{Interval: time.Hour, Sample: func() (int, error) { return 1, nil }})
			chans := make([]<-chan int, n)
			for i := 0; i < n; i++ {
				_, chans[i] = s.Subscribe()
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.broadcast(i)
				// Drain so buffers do not stay full (measure steady delivery).
				for _, ch := range chans {
					<-ch
				}
			}
		})
	}
}

func subsLabel(n int) string {
	switch n {
	case 1:
		return "1sub"
	case 10:
		return "10subs"
	default:
		return "100subs"
	}
}
