//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"testing"
	"time"
)

// This is a real local result journal, intentionally without source authority.
// The setter is clock selection only; the pilot test exercises actual fencing.
func TestOwnedResultCalendarClockScope(t *testing.T) {
	ctx := context.Background()
	reference := downstreamRef(t)
	var pin OwnedPin
	if e := json.Unmarshal(reference.Contexts.Pins["result"], &pin); e != nil {
		t.Fatal(e)
	}
	// initialize synchronizes this exact marker and its sibling db directory.
	// Keep both under one retained root, matching the local reopen audit fixture.
	directory := t.TempDir()
	marker := downstreamComposition(t, reference, "result_marker", "v1")
	raw, e := hex.DecodeString(marker.FileHex)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(directory, ownedMarkerName), raw, 0600); e != nil {
		t.Fatal(e)
	}
	db, e := leveldb.OpenFile(filepath.Join(directory, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	s := ownedNewStore(db, func() time.Time { return time.UnixMilli(1795291200100) })
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	s.sourceCreatedAtMS = s.now().UnixMilli() - 1
	if e := s.initialize(ctx, directory, pin); e != nil {
		t.Fatal(e)
	}
	before, e := s.db.Get([]byte("meta"), nil)
	if e != nil {
		t.Fatal(e)
	}
	old := s.now().UnixMilli()
	next := old + 1000
	clock := func() time.Time { return time.UnixMilli(next) }
	foreign := pin
	foreign.Version++
	if e = s.UseOwnedClockForTest(ctx, foreign, clock); !errors.Is(e, ErrConflict) {
		t.Fatal("foreign pin", e)
	}
	if e = s.UseOwnedClockForTest(ctx, pin, nil); !errors.Is(e, ErrInvalid) {
		t.Fatal("nil clock", e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if e = s.UseOwnedClockForTest(cancelled, pin, clock); !errors.Is(e, ErrCancelled) {
		t.Fatal("cancelled setter", e)
	}
	s.readOnly = true
	e = s.UseOwnedClockForTest(ctx, pin, clock)
	s.readOnly = false
	if !errors.Is(e, ErrConflict) {
		t.Fatal("audit owner clock changed", e)
	}
	_, done, e := s.begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	e = s.UseOwnedClockForTest(ctx, pin, clock)
	done()
	if !errors.Is(e, ErrBusy) {
		t.Fatal("admitted action clock changed", e)
	}
	s.mu.Lock()
	s.clockSeen = old + 500
	s.mu.Unlock()
	if e = s.UseOwnedClockForTest(ctx, pin, func() time.Time { return time.UnixMilli(old + 499) }); !errors.Is(e, ErrClock) {
		t.Fatal("observed clock rolled back", e)
	}
	if s.now().UnixMilli() != old {
		t.Fatal("refused setter changed clock")
	}
	if e = s.UseOwnedClockForTest(ctx, pin, clock); e != nil {
		t.Fatal(e)
	}
	if s.now().UnixMilli() != next || s.clockSeen != old+500 {
		t.Fatal("setter observed a new durable clock")
	}
	after, e := s.db.Get([]byte("meta"), nil)
	if e != nil || !bytes.Equal(before, after) || s.meta.Career != nil || s.meta.ResultCount != 0 || len(s.careerIndex) != 0 {
		t.Fatal("clock setter wrote metadata, activation or awards", e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if e = s.UseOwnedClockForTest(ctx, pin, clock); !errors.Is(e, ErrClosed) {
		t.Fatal("closed owner", e)
	}
}
