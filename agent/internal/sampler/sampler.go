// Package sampler provides a subscription-driven broadcast source: it samples
// one data source on a fixed tick and fans each sample out to N subscribers.
//
// The load-bearing property (CLAUDE.md §8): a source with zero subscribers
// does not sample. The sampling goroutine exists only between the first
// Subscribe and the last Unsubscribe. This is what lets the agent idle near
// zero CPU when nobody is watching, and it is why the Processes view (M5) can
// be expensive without costing anything while closed.
package sampler

import (
	"sync"
	"time"
)

// TickerFunc creates a ticker that delivers on the returned channel every d,
// paired with a stop function. It is injectable so tests drive ticks
// deterministically instead of sleeping.
type TickerFunc func(d time.Duration) (<-chan time.Time, func())

func realTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

// Config parameterises a Source.
type Config[T any] struct {
	// Interval is the sampling tick.
	Interval time.Duration
	// Sample produces one sample. It is called at most once per tick,
	// regardless of subscriber count — sample once, fan out.
	Sample func() (T, error)
	// Prime, if set, runs once each time the sampling goroutine (re)starts,
	// before the first tick. Stateful sources (e.g. CPU deltas) use it to
	// reset a baseline after an idle gap.
	Prime func()
	// OnError handles a sampling error; the failed tick is skipped.
	OnError func(error)
	// Ticker overrides the ticker factory (tests). Defaults to time.Ticker.
	Ticker TickerFunc
}

// Source is a running, subscription-driven sampler for values of type T.
type Source[T any] struct {
	interval  time.Duration
	sample    func() (T, error)
	prime     func()
	onErr     func(error)
	newTicker TickerFunc

	mu     sync.Mutex
	subs   map[int]chan T
	nextID int
	stop   func() // stops the running goroutine; nil while idle
}

// New constructs a Source. It does not start sampling; the first Subscribe
// does.
func New[T any](cfg Config[T]) *Source[T] {
	tk := cfg.Ticker
	if tk == nil {
		tk = realTicker
	}
	onErr := cfg.OnError
	if onErr == nil {
		onErr = func(error) {}
	}
	return &Source[T]{
		interval:  cfg.Interval,
		sample:    cfg.Sample,
		prime:     cfg.Prime,
		onErr:     onErr,
		newTicker: tk,
		subs:      make(map[int]chan T),
	}
}

// Subscribe registers a subscriber, returning its id and a receive channel.
// The channel is buffered with latest-wins semantics: a slow reader never
// blocks the sampler and simply skips to the freshest sample. The first
// subscriber starts the sampling goroutine.
func (s *Source[T]) Subscribe() (int, <-chan T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	ch := make(chan T, 1)
	s.subs[id] = ch
	if len(s.subs) == 1 {
		s.startLocked()
	}
	return id, ch
}

// Unsubscribe removes a subscriber and closes its channel. The last
// Unsubscribe stops the sampling goroutine.
func (s *Source[T]) Unsubscribe(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.subs[id]
	if !ok {
		return
	}
	delete(s.subs, id)
	close(ch)
	if len(s.subs) == 0 {
		s.stopLocked()
	}
}

// Subscribers returns the current subscriber count (for tests and diagnostics).
func (s *Source[T]) Subscribers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

func (s *Source[T]) startLocked() {
	tickCh, stopTicker := s.newTicker(s.interval)
	done := make(chan struct{})
	s.stop = func() {
		stopTicker()
		close(done)
	}
	go s.run(tickCh, done)
}

func (s *Source[T]) stopLocked() {
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
}

func (s *Source[T]) run(tick <-chan time.Time, done chan struct{}) {
	if s.prime != nil {
		s.prime()
	}
	for {
		select {
		case <-done:
			return
		case <-tick:
			v, err := s.sample()
			if err != nil {
				s.onErr(err)
				continue
			}
			s.broadcast(v)
		}
	}
}

func (s *Source[T]) broadcast(v T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs {
		trySendLatest(ch, v)
	}
}

// trySendLatest delivers v without blocking, replacing any stale buffered
// value so the subscriber always sees the freshest sample.
func trySendLatest[T any](ch chan T, v T) {
	select {
	case ch <- v:
		return
	default:
	}
	// Buffer full: drop the stale value, then send the fresh one.
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- v:
	default:
	}
}
