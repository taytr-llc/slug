//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStableCheapProfileRefusesBeforeStorageOrDisclosure(t *testing.T) {
	pin := readCoreCorpus(t).Pins[0].Pin
	path := filepath.Join(t.TempDir(), "never-created")
	if s, e := Create(path, pin, nil); s != nil || !errors.Is(e, ErrProfile) {
		t.Fatal(s, e)
	}
	if _, e := os.Stat(path); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("cheap creation touched storage", e)
	}
	if s, e := Open(path, pin, nil); s != nil || !errors.Is(e, ErrProfile) {
		t.Fatal(s, e)
	}
	// Even a directly constructed internal handle must refuse before reaching
	// private state or a lease. Public configuration codecs remain structural.
	s := &Store{}
	ctx := context.Background()
	session := Session{}
	calls := []func() error{
		func() error {
			r, e := s.Status(ctx, session)
			if r.Version != 0 {
				t.Fatal("status disclosed")
			}
			return e
		},
		func() error {
			r, e := s.Sync(ctx, session)
			if r.Version != 0 {
				t.Fatal("sync disclosed")
			}
			return e
		},
		func() error {
			r, e := s.Inventory(ctx, session, 0, 8)
			if r.Version != 0 {
				t.Fatal("page disclosed")
			}
			return e
		},
		func() error {
			r, e := s.Roster(ctx, session, 1, 8)
			if r.Version != 0 {
				t.Fatal("roster disclosed")
			}
			return e
		},
		func() error {
			r, e := s.PrepareSlot(ctx, session, SlotRequest{Slot: 1})
			if r.Version != 0 {
				t.Fatal("intent disclosed")
			}
			return e
		},
		func() error {
			r, e := s.ApplySlot(ctx, session, "invalid")
			if r.Version != 0 {
				t.Fatal("mutation disclosed")
			}
			return e
		},
		func() error {
			r, e := s.Operation(ctx, session, "invalid")
			if r.Version != 0 {
				t.Fatal("operation disclosed")
			}
			return e
		},
		func() error {
			r, e := s.Pending(ctx, session)
			if r != nil {
				t.Fatal("pending disclosed")
			}
			return e
		},
		func() error {
			r, e := s.Abandon(ctx, session, "invalid")
			if r.Version != 0 {
				t.Fatal("abandon disclosed")
			}
			return e
		},
	}
	for _, call := range calls {
		if e := call(); !errors.Is(e, ErrProfile) {
			t.Fatal(e)
		}
	}
	raw, e := MarshalPin(pin)
	if e != nil {
		t.Fatal(e)
	}
	if parsed, e := ParsePin(raw); e != nil || parsed != pin {
		t.Fatal(e)
	}
}
