// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOwnedGymRuntimeCloseRetainsFailedBorrow(t *testing.T) {
	life, cancel := context.WithCancel(context.Background())
	failure := errors.New("owned child cleanup uncertain")
	borrow := &poolRuntimeBorrow{account: "original", done: make(chan struct{}), close: func() error { return failure }}
	r := &PoolRuntime{life: life, cancel: cancel, gate: make(chan struct{}, 1), ownedGym: &ownedGymRuntimeResources{}, borrows: map[*poolRuntimeBorrow]bool{borrow: true}}
	done := make(chan error, 1)
	go func() { done <- r.Close() }()
	select {
	case e := <-done:
		if !errors.Is(e, failure) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("Close waited on its cleanup-failed child's retained borrow")
	}
	if !r.borrows[borrow] {
		t.Fatal("failed owner released")
	}
	select {
	case <-borrow.done:
		t.Fatal("failed borrow falsely joined")
	default:
	}
	if !errors.Is(r.Close(), failure) {
		t.Fatal("sticky cleanup error lost")
	}
}
func TestOwnedGymRuntimeCloseJoinsSuccessfulBorrow(t *testing.T) {
	life, cancel := context.WithCancel(context.Background())
	borrow := &poolRuntimeBorrow{account: "original", done: make(chan struct{})}
	borrow.close = func() error { close(borrow.done); return nil }
	r := &PoolRuntime{life: life, cancel: cancel, gate: make(chan struct{}, 1), ownedGym: &ownedGymRuntimeResources{}, borrows: map[*poolRuntimeBorrow]bool{borrow: true}}
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	select {
	case <-life.Done():
	default:
		t.Fatal("runtime lifetime not canceled")
	}
	select {
	case <-borrow.done:
	default:
		t.Fatal("successful child not joined")
	}
}
