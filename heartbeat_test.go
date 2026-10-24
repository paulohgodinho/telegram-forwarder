package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// fakePinger counts calls and returns err (if set) or succeeds.
type fakePinger struct {
	calls int32
	err   error
}

func (f *fakePinger) Ping(_ context.Context) error {
	atomic.AddInt32(&f.calls, 1)
	return f.err
}

func TestStartHeartbeatPingsOnEveryTick(t *testing.T) {
	p := &fakePinger{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startHeartbeat(ctx, p, 5*time.Millisecond)

	deadline := time.After(200 * time.Millisecond)
	for atomic.LoadInt32(&p.calls) < 3 {
		select {
		case <-deadline:
			t.Fatalf("expected at least 3 pings, got %d", atomic.LoadInt32(&p.calls))
		case <-time.After(time.Millisecond):
		}
	}
}

func TestStartHeartbeatStopsWhenContextCanceled(t *testing.T) {
	p := &fakePinger{}
	ctx, cancel := context.WithCancel(context.Background())

	startHeartbeat(ctx, p, 2*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	cancel()

	calls := atomic.LoadInt32(&p.calls)
	time.Sleep(30 * time.Millisecond)
	if got := atomic.LoadInt32(&p.calls); got > calls {
		t.Fatalf("expected no more pings after cancel, had %d then %d", calls, got)
	}
}

func TestPingOnceLogsFailure(t *testing.T) {
	p := &fakePinger{err: errors.New("connection reset")}
	// pingOnce should not panic and should return promptly even on error.
	done := make(chan struct{})
	go func() {
		pingOnce(context.Background(), p)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pingOnce did not return in time")
	}
	if atomic.LoadInt32(&p.calls) != 1 {
		t.Fatalf("expected exactly 1 ping call, got %d", p.calls)
	}
}
