// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The real local journal owner is exercised without a source handle. This
// cannot mint a seed/card/bout/result or claim source authority/finality.
func ownedJournalFixture(t *testing.T) *OwnedStore {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal owner profile; codec/profile tests separate")
	}
	db, e := leveldb.OpenFile(filepath.Join(t.TempDir(), "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	s := ownedNewStore(db, func() time.Time { return time.UnixMilli(1795291200100) })
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func TestOwnedCardCloseJoinsAdmittedJournal(t *testing.T) {
	s := ownedJournalFixture(t)
	work, done, e := s.begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	var once sync.Once
	release := func() { once.Do(done) }
	defer release()
	if _, next, e := s.begin(context.Background()); !errors.Is(e, ErrBusy) {
		if next != nil {
			next()
		}
		t.Fatalf("parallel owner: %v", e)
	}
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case <-work.Done():
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel admitted work")
	}
	select {
	case e := <-closed:
		t.Fatalf("Close returned before admitted work joined: %v", e)
	default:
	}
	release()
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("Close failed to join")
	}
	if _, next, e := s.begin(context.Background()); !errors.Is(e, ErrClosed) {
		if next != nil {
			next()
		}
		t.Fatalf("closed reuse: %v", e)
	}
}
func TestOwnedCardReadOnlyAndPoisonedWrites(t *testing.T) {
	s := ownedJournalFixture(t)
	writes := 0
	s.writeBatch = func(*leveldb.Batch, *opt.WriteOptions) error {
		writes++
		return errors.New("injected sync uncertainty")
	}
	s.readOnly = true
	if e := s.commitLocked(new(leveldb.Batch), s.meta); !errors.Is(e, ErrForbidden) || writes != 0 {
		t.Fatalf("audit owner wrote: %v/%d", e, writes)
	}
	s.readOnly = false
	if e := s.commitLocked(new(leveldb.Batch), s.meta); !errors.Is(e, ErrOutcomeUnknown) || writes != 1 || !s.poisoned {
		t.Fatalf("write uncertainty lost: %v/%d", e, writes)
	}
	if _, done, e := s.begin(context.Background()); !errors.Is(e, ErrPoisoned) {
		if done != nil {
			done()
		}
		t.Fatalf("poisoned owner reused: %v", e)
	}
}
func TestOwnedCardCleanupErrorRetainsOwner(t *testing.T) {
	s := ownedJournalFixture(t)
	if e := s.db.Close(); e != nil {
		t.Fatal(e)
	}
	_, e := ownedFailed(s, ErrCorrupt)
	var cleanup *OwnedCleanupError
	if !errors.As(e, &cleanup) || cleanup.Owner != s || !errors.Is(e, ErrCorrupt) {
		t.Fatalf("failed cleanup owner discarded: %v", e)
	}
	first := s.Close()
	second := s.Close()
	if first == nil || second != first {
		t.Fatalf("cleanup error not sticky: %v/%v", first, second)
	}
}
func TestOwnedCardCanceledAdmission(t *testing.T) {
	s := ownedJournalFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, done, e := s.begin(ctx); !errors.Is(e, ErrCancelled) {
		if done != nil {
			done()
		}
		t.Fatalf("cancelled admission: %v", e)
	}
	if _, done, e := s.begin(context.Background()); e != nil {
		t.Fatal(e)
	} else {
		done()
	}
}

func TestOwnedCardCheapProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal")
	}
	_, e := CreateOwned(context.Background(), t.TempDir(), OwnedPin{}, nil)
	if !errors.Is(e, ErrProfile) {
		t.Fatalf("profile not refused before authority: %v", e)
	}
}

// This audits real local initialization/reopen bytes with an independently
// composed pin/marker. No upstream capability is fabricated or accepted.
func TestOwnedCardLocalJournalReopenAudit(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal owned journal")
	}
	r := downstreamRef(t)
	var p OwnedPin
	if e := json.Unmarshal(r.Contexts.Pins["card"], &p); e != nil {
		t.Fatal(e)
	}
	c := downstreamComposition(t, r, "card_marker", "v1")
	var marker ownedMarker
	if e := json.Unmarshal(c.Value, &marker); e != nil {
		t.Fatal(e)
	}
	d := t.TempDir()
	markerRaw, _ := hex.DecodeString(c.FileHex)
	if e := os.WriteFile(filepath.Join(d, ownedMarkerName), markerRaw, 0600); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(d, "db")
	db, e := leveldb.OpenFile(path, nil)
	if e != nil {
		t.Fatal(e)
	}
	now := int64(1795291199000)
	clock := func() time.Time { return time.UnixMilli(now) }
	s := ownedNewStore(db, clock)
	s.sourceCreatedAtMS = now - 1
	if e = s.initialize(context.Background(), d, p); e != nil {
		_ = s.Close()
		t.Fatal(e)
	}
	before, e := db.Get([]byte("meta"), nil)
	if e != nil {
		_ = s.Close()
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	db, e = leveldb.OpenFile(path, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if e != nil {
		t.Fatal(e)
	}
	reopened := ownedNewStore(db, clock)
	reopened.sourceCreatedAtMS = now - 1
	reopened.readOnly = true
	defer reopened.Close()
	if e = reopened.audit(context.Background(), marker); e != nil {
		t.Fatal(e)
	}
	after, e := db.Get([]byte("meta"), nil)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("read-only audit changed metadata")
	}
	if e = reopened.commitLocked(new(leveldb.Batch), reopened.meta); !errors.Is(e, ErrForbidden) {
		t.Fatalf("audited reader became writer: %v", e)
	}
	wrong := marker
	wrong.StoreID = strings.Repeat("f", 64)
	if e = reopened.audit(context.Background(), wrong); !errors.Is(e, ErrCorrupt) {
		t.Fatalf("foreign marker accepted: %v", e)
	}
}
