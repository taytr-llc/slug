// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// This local fixture exercises admission/storage lifetime only. It deliberately
// has no replay source and cannot mint an owned fighter read or mutate a roster.
func ownedLocalFixture(t *testing.T) (*OwnedStore, Session, gymaccount.Account) {
	t.Helper()
	ref := loadOwnedReference(t)
	p := ref.Contexts["v1/zero256-unawarded/initial"].Pin
	db, e := leveldb.OpenFile(filepath.Join(t.TempDir(), "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	s, e := newOwnedStore(db, p, nil, nil, true, false)
	if e != nil {
		t.Fatal(e)
	}
	id, _ := OwnedID(p)
	s.meta = ownedMetadata{Version: 1, StoreID: id, Pin: p, ClockMS: 1795300000000}
	s.roster = rosterRow{Version: 1}
	s.now = func() time.Time { return time.UnixMilli(s.meta.ClockMS) }
	a := gymaccount.Account{ID: p.Stable.AccountID, Generation: 1, Enrollment: &p.Stable.Identity}
	session := Session{Generation: 1, ViewID: strings.Repeat("a", 64)}
	session.Lease = func(_ context.Context, _ gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		return fn(a, func() error { return nil })
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, session, a
}
func TestOwnedStableAdmissionRejectsBrokenOuterFence(t *testing.T) {
	for _, kind := range []string{"missing", "double", "outer_error", "swallowed_inner"} {
		t.Run(kind, func(t *testing.T) {
			s, session, a := ownedLocalFixture(t)
			denied := errors.New("outer refusal")
			session.Lease = func(_ context.Context, _ gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
				switch kind {
				case "missing":
					return nil
				case "double":
					_ = fn(a, func() error { return nil })
					_ = fn(a, func() error { return nil })
					return nil
				case "outer_error":
					_ = fn(a, func() error { return nil })
					return denied
				default:
					_ = fn(a, func() error { return gymaccount.ErrUnauthorized })
					return nil
				}
			}
			_, done, e := s.begin(context.Background(), session, gymaccount.View)
			if done != nil {
				done()
			}
			if e == nil {
				t.Fatal("broken original fence admitted")
			}
		})
	}
}
func TestOwnedStableCommittedOuterFailureIsUnknown(t *testing.T) {
	s, session, a := ownedLocalFixture(t)
	denied := errors.New("outer refusal after publication")
	session.Lease = func(_ context.Context, _ gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if e := fn(a, func() error { return nil }); e != nil {
			return e
		}
		return denied
	}
	b := new(leveldb.Batch)
	b.Put([]byte("test-commit"), []byte("retained"))
	e := s.commitOwned(context.Background(), session, gymaccount.RosterManage, b, s.meta, nil, nil)
	if !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatalf("publication lost outer fence: %v", e)
	}
	raw, e := s.db.Get([]byte("test-commit"), nil)
	if e != nil || string(raw) != "retained" {
		t.Fatal("test did not reach actual commit")
	}
}
func TestOwnedStableWriteFailurePoisonsOwner(t *testing.T) {
	s, session, _ := ownedLocalFixture(t)
	s.writeBatch = func(*leveldb.Batch, *opt.WriteOptions) error { return errors.New("sync failed") }
	e := s.commitOwned(context.Background(), session, gymaccount.RosterManage, new(leveldb.Batch), s.meta, nil, nil)
	if !errors.Is(e, ErrOutcomeUnknown) || !s.poisoned {
		t.Fatalf("sync uncertainty lost: %v", e)
	}
	want := ErrPoisoned
	if wire.TestPoW {
		// The public runtime admission refuses the cheap profile before testing
		// owner state. The local poisoned-write assertion above still applies.
		want = ErrProfile
	}
	if _, done, e := s.enter(context.Background()); !errors.Is(e, want) {
		if done != nil {
			done()
		}
		t.Fatalf("poisoned owner reused: %v", e)
	}
}
func TestOwnedStableCloseCancelsAndJoinsLane(t *testing.T) {
	s, _, _ := ownedLocalFixture(t)
	ctx, done, e := s.enter(context.Background())
	if wire.TestPoW {
		if done != nil {
			done()
		}
		if !errors.Is(e, ErrProfile) || ctx != nil {
			t.Fatalf("cheap-profile runtime admission: %v", e)
		}
		return
	}
	if e != nil {
		t.Fatal(e)
	}
	var once sync.Once
	release := func() { once.Do(done) }
	defer release()
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("close did not cancel active operation")
	}
	select {
	case e := <-closed:
		t.Fatalf("close returned before lane release: %v", e)
	default:
	}
	release()
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("close did not join")
	}
	if _, done, e = s.enter(context.Background()); !errors.Is(e, ErrClosed) {
		if done != nil {
			done()
		}
		t.Fatal("closed owner admitted")
	}
}
func TestOwnedStableCleanupFailureRetainsOwner(t *testing.T) {
	s, _, _ := ownedLocalFixture(t)
	if e := s.db.Close(); e != nil {
		t.Fatal(e)
	}
	_, e := ownedFail(s, errors.New("construction failed"))
	var cleanup *OwnedCleanupError
	if !errors.As(e, &cleanup) || cleanup.Owner != s || s.Close() == nil {
		t.Fatalf("cleanup ownership/sticky error lost: %v", e)
	}
}
