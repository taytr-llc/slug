// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"testing"
	"time"
)

func TestOwnedGymRefreshContinuationWithoutIdleTick(t *testing.T) {
	// No idle tick is ever supplied. An unfinished successful replay must be
	// able to continue after yielding, rather than stall on the idle cadence.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if !ownedWaitRefresh(ctx, nil, true) || ctx.Err() != nil {
		t.Fatal("successful incomplete replay waited for an idle tick")
	}
}

func TestOwnedGymRefreshIdleAndCancellation(t *testing.T) {
	t.Run("idle requires tick", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan bool, 1)
		joined := make(chan struct{})
		tick := make(chan time.Time)
		go func() { defer close(joined); done <- ownedWaitRefresh(ctx, tick, false) }()
		defer func() { cancel(); <-joined }()
		select {
		case <-done:
			t.Fatal("idle/error scheduling continued without its tick")
		case tick <- time.Now():
		case <-time.After(2 * time.Second):
			t.Fatal("idle scheduler did not admit its tick")
		}
		select {
		case again := <-done:
			if !again {
				t.Fatal("live idle tick cancelled")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("idle scheduler failed to join")
		}
	})
	for _, continuation := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if ownedWaitRefresh(ctx, nil, continuation) {
			t.Fatal("cancelled original lifecycle obtained another slice", continuation)
		}
	}
	// A dormant idle wait also joins when the original lifetime is cancelled;
	// it does not need a synthetic tick or a replacement context to finish.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	joined := make(chan struct{})
	go func() { defer close(joined); done <- ownedWaitRefresh(ctx, nil, false) }()
	defer func() { cancel(); <-joined }()
	cancel()
	select {
	case again := <-done:
		if again {
			t.Fatal("cancellation resumed dormant work")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled idle scheduler failed to join")
	}
}
