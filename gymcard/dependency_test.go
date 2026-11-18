// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"testing"
	"time"
)

func TestCardProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	path := filepath.Join(realTemp(t), "never-created")
	if _, e := Create(context.Background(), path, Pin{}, nil); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
}
func TestCardClosePrecedenceAndCallerCancellation(t *testing.T) {
	requireNormal(t)
	db, e := leveldb.OpenFile(filepath.Join(realTemp(t), "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	s := newStore(db, time.Now)
	defer s.Close()
	work, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, e = s.begin(work); !errors.Is(e, ErrCancelled) {
		t.Fatal(e)
	}
	s.gate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan error, 1)
	go func() {
		_, done, e := s.begin(ctx)
		if done != nil {
			done()
		}
		out <- e
	}()
	cancel()
	select {
	case e := <-out:
		if !errors.Is(e, ErrCancelled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("uncancellable gate")
	}
	<-s.gate
	s.Close()
	if _, _, e = s.begin(context.Background()); !errors.Is(e, ErrClosed) {
		t.Fatal("closed classified cancelled", e)
	}
}
