// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestStableCancelledCallerDoesNotWaitForCommitMutex(t *testing.T) {
	x := newCoreFixture(t, "zero256", 0)
	x.now++ // Force the live holder to persist an observation under its mutex.
	entered, release := make(chan struct{}), make(chan struct{})
	real := x.s.writeBatch
	var once sync.Once
	x.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
		once.Do(func() { close(entered); <-release })
		return real(b, o)
	}
	holder := make(chan error, 1)
	go func() { _, e := x.s.Status(context.Background(), x.session); holder <- e }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	contender := make(chan error, 1)
	go func() { _, e := x.s.Status(ctx, x.session); contender <- e }()
	var result error
	timely := true
	select {
	case result = <-contender:
	case <-time.After(time.Second):
		timely = false
	}
	close(release)
	holderErr := <-holder
	if !timely {
		result = <-contender
	}
	if holderErr != nil {
		t.Fatal("live holder failed", holderErr)
	}
	if !errors.Is(result, ErrCancelled) {
		t.Fatal("caller cancellation changed", result)
	}
	if !timely {
		t.Fatal("cancelled caller waited for the unrelated synchronous commit mutex")
	}
	if _, e := x.s.Status(context.Background(), x.session); e != nil {
		t.Fatal("gate not released", e)
	}
}
