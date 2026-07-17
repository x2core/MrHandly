package sampler

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tickController is an injectable ticker whose ticks the test drives by hand.
type tickController struct {
	mu     sync.Mutex
	ch     chan time.Time
	starts int
	stops  int
}

func (tc *tickController) factory(time.Duration) (<-chan time.Time, func()) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.starts++
	ch := make(chan time.Time)
	tc.ch = ch
	return ch, func() {
		tc.mu.Lock()
		tc.stops++
		tc.mu.Unlock()
	}
}

// tick delivers one tick and blocks until the run loop receives it.
func (tc *tickController) tick() {
	tc.mu.Lock()
	ch := tc.ch
	tc.mu.Unlock()
	ch <- time.Time{}
}

func recv(t *testing.T, ch <-chan int) int {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sample")
		return 0
	}
}

func TestZeroSubscribersDoNotSample(t *testing.T) {
	tc := &tickController{}
	var samples int32
	s := New(Config[int]{
		Interval: time.Second,
		Ticker:   tc.factory,
		Sample:   func() (int, error) { return int(atomic.AddInt32(&samples, 1)), nil },
	})

	if s.Subscribers() != 0 {
		t.Fatal("expected 0 subscribers at rest")
	}
	// No goroutine should have started, so the ticker factory is untouched.
	tc.mu.Lock()
	starts := tc.starts
	tc.mu.Unlock()
	if starts != 0 {
		t.Fatalf("ticker started %d times with no subscribers, want 0", starts)
	}
	if atomic.LoadInt32(&samples) != 0 {
		t.Fatal("sampled with no subscribers")
	}
}

func TestFanOutSamplesOncePerTick(t *testing.T) {
	tc := &tickController{}
	var samples int32
	s := New(Config[int]{
		Interval: time.Second,
		Ticker:   tc.factory,
		Sample:   func() (int, error) { return int(atomic.AddInt32(&samples, 1)), nil },
	})

	_, a := s.Subscribe()
	_, b := s.Subscribe()
	if s.Subscribers() != 2 {
		t.Fatalf("subscribers = %d, want 2", s.Subscribers())
	}

	tc.tick()
	va, vb := recv(t, a), recv(t, b)
	if va != vb {
		t.Errorf("subscribers saw different values: %d vs %d", va, vb)
	}
	if got := atomic.LoadInt32(&samples); got != 1 {
		t.Errorf("sample called %d times for one tick, want 1 (sample once, fan out)", got)
	}
}

func TestLastUnsubscribeStopsSampling(t *testing.T) {
	tc := &tickController{}
	s := New(Config[int]{
		Interval: time.Second,
		Ticker:   tc.factory,
		Sample:   func() (int, error) { return 1, nil },
	})

	id1, _ := s.Subscribe()
	id2, _ := s.Subscribe()
	s.Unsubscribe(id1)
	if s.Subscribers() != 1 {
		t.Fatal("expected 1 subscriber remaining")
	}
	tc.mu.Lock()
	stopsAfterOne := tc.stops
	tc.mu.Unlock()
	if stopsAfterOne != 0 {
		t.Fatal("ticker stopped while a subscriber remained")
	}

	s.Unsubscribe(id2)
	if s.Subscribers() != 0 {
		t.Fatal("expected 0 subscribers")
	}
	tc.mu.Lock()
	stops := tc.stops
	tc.mu.Unlock()
	if stops != 1 {
		t.Fatalf("ticker stops = %d after last unsubscribe, want 1", stops)
	}
}

func TestPrimeRunsOnEachStart(t *testing.T) {
	tc := &tickController{}
	var primes int32
	s := New(Config[int]{
		Interval: time.Second,
		Ticker:   tc.factory,
		Prime:    func() { atomic.AddInt32(&primes, 1) },
		Sample:   func() (int, error) { return 1, nil },
	})

	id, _ := s.Subscribe()
	tc.tick() // ensure run() has executed prime before we assert
	s.Unsubscribe(id)

	// Re-subscribe: the goroutine restarts and primes again.
	id2, ch := s.Subscribe()
	tc.tick()
	recv(t, ch)
	s.Unsubscribe(id2)

	if got := atomic.LoadInt32(&primes); got != 2 {
		t.Fatalf("prime ran %d times, want 2 (once per (re)start)", got)
	}
}

// TestLatestWins verifies a slow subscriber that never reads always sees the
// freshest sample, and that a full buffer never blocks the sampler. The
// entered/gate handshake makes the interleaving deterministic: each sample
// announces it has begun (entered) and then waits for the test's permission
// (gate). Because the run loop only receives the next tick after finishing the
// previous broadcast, observing sample N+1 begin proves broadcast N completed.
func TestLatestWins(t *testing.T) {
	tc := &tickController{}
	entered := make(chan struct{})
	gate := make(chan struct{})
	var samples int32
	s := New(Config[int]{
		Interval: time.Second,
		Ticker:   tc.factory,
		Sample: func() (int, error) {
			entered <- struct{}{}
			<-gate
			return int(atomic.AddInt32(&samples, 1)), nil
		},
	})

	_, ch := s.Subscribe()

	step := func() {
		go tc.tick()  // run receives the tick, then begins the sample
		<-entered     // proves the previous broadcast finished
		gate <- struct{}{}
	}
	step() // sample 1 broadcast, buffer = 1
	step() // sample 2 broadcast, replaces stale 1, buffer = 2

	// Begin a third tick but do NOT release its sample: the run loop receiving
	// tick 3 proves broadcast 2 completed, and the buffer still holds 2.
	go tc.tick()
	<-entered

	if v := recv(t, ch); v != 2 {
		t.Errorf("slow subscriber got %d, want freshest 2", v)
	}

	gate <- struct{}{} // let the pending sample finish so the goroutine can exit
}
