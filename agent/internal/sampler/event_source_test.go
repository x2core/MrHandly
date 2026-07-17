package sampler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventSourceStartsOnFirstSubscriber(t *testing.T) {
	var started int32
	release := make(chan struct{})
	s := NewEvent(func(ctx context.Context, emit func(int)) {
		atomic.AddInt32(&started, 1)
		emit(1)
		<-ctx.Done() // stay alive until the last subscriber leaves
		close(release)
	})

	if atomic.LoadInt32(&started) != 0 {
		t.Fatal("producer started before any subscriber")
	}

	id, ch := s.Subscribe()
	if v := recv(t, ch); v != 1 {
		t.Fatalf("got %d, want emitted 1", v)
	}
	if atomic.LoadInt32(&started) != 1 {
		t.Fatal("producer did not start on first subscribe")
	}

	s.Unsubscribe(id)
	select {
	case <-release:
	case <-time.After(2 * time.Second):
		t.Fatal("producer context was not cancelled on last unsubscribe")
	}
	if s.Subscribers() != 0 {
		t.Fatal("expected 0 subscribers")
	}
}

func TestEventSourceFanOut(t *testing.T) {
	emitCh := make(chan int)
	s := NewEvent(func(ctx context.Context, emit func(int)) {
		for {
			select {
			case <-ctx.Done():
				return
			case v := <-emitCh:
				emit(v)
			}
		}
	})

	_, a := s.Subscribe()
	_, b := s.Subscribe()
	emitCh <- 42
	if va, vb := recv(t, a), recv(t, b); va != 42 || vb != 42 {
		t.Fatalf("fan-out mismatch: a=%d b=%d", va, vb)
	}
}

// TestEventSourceRestarts verifies the producer restarts on a fresh
// subscription after going idle (context per producer lifetime).
func TestEventSourceRestarts(t *testing.T) {
	var starts int32
	s := NewEvent(func(ctx context.Context, emit func(int)) {
		atomic.AddInt32(&starts, 1)
		emit(int(atomic.LoadInt32(&starts)))
		<-ctx.Done()
	})

	id, ch := s.Subscribe()
	recv(t, ch)
	s.Unsubscribe(id)

	id2, ch2 := s.Subscribe()
	if v := recv(t, ch2); v != 2 {
		t.Fatalf("second start emitted %d, want 2", v)
	}
	s.Unsubscribe(id2)

	if got := atomic.LoadInt32(&starts); got != 2 {
		t.Fatalf("producer started %d times, want 2", got)
	}
}
