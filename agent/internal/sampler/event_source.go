package sampler

import (
	"context"
	"sync"
)

// EventSource is a subscription-driven broadcast source driven by an external
// event producer instead of a fixed tick. It carries the same load-bearing
// guarantee as Source (CLAUDE.md §8): the producer runs only while there is at
// least one subscriber. The first Subscribe starts the producer; the last
// Unsubscribe cancels its context.
//
// M2 uses this for services: one D-Bus signal subscription feeds all watching
// clients, so the UI updates on change rather than on tick and the agent does
// no systemd work while no one is watching services.
type EventSource[T any] struct {
	run func(ctx context.Context, emit func(T))

	mu     sync.Mutex
	subs   map[int]chan T
	nextID int
	cancel context.CancelFunc // cancels the running producer; nil while idle
}

// NewEvent constructs an EventSource. run is invoked when the first subscriber
// arrives and should produce values via emit until ctx is cancelled (which
// happens when the last subscriber leaves). emit never blocks the producer:
// delivery is latest-wins per subscriber.
func NewEvent[T any](run func(ctx context.Context, emit func(T))) *EventSource[T] {
	return &EventSource[T]{run: run, subs: make(map[int]chan T)}
}

// Subscribe registers a subscriber and returns its id and receive channel.
func (s *EventSource[T]) Subscribe() (int, <-chan T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	ch := make(chan T, 1)
	s.subs[id] = ch
	if len(s.subs) == 1 {
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		go s.run(ctx, s.broadcast)
	}
	return id, ch
}

// Unsubscribe removes a subscriber and closes its channel. The last
// Unsubscribe cancels the producer.
func (s *EventSource[T]) Unsubscribe(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.subs[id]
	if !ok {
		return
	}
	delete(s.subs, id)
	close(ch)
	if len(s.subs) == 0 && s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// Subscribers returns the current subscriber count.
func (s *EventSource[T]) Subscribers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

// broadcast fans a produced value out to every subscriber, latest-wins.
func (s *EventSource[T]) broadcast(v T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs {
		trySendLatest(ch, v)
	}
}
