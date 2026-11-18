//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type resultFixture struct {
	dir           string
	now           atomic.Int64
	revoked, held atomic.Bool
	inLease       atomic.Int64
	pin           Pin
	record        gymbout.Record
	source        *registry.ManifestStore
	stable        *gymstable.Store
	entry         *gymentry.Store
	event         *gymevent.Store
	seed          *gymseed.Store
	card          *gymcard.Store
	bout          *gymbout.Store
	s             *Store
	session       gymstable.Session
}

func (f *resultFixture) clock() time.Time { return time.UnixMilli(f.now.Load()) }
func (f *resultFixture) call() sourceCall {
	return sourceCall{ctx: context.Background(), bouts: f.bout, cards: f.card, seeds: f.seed, events: f.event, entries: f.entry, session: f.session, date: f.record.Source.Event.Saturday, id: f.record.BoutID}
}
func fixtureJSON(t testing.TB, name string, out any) {
	t.Helper()
	raw, e := os.ReadFile("testdata/reference/inputs/actual47/" + name + ".json")
	if e != nil {
		t.Fatal(e)
	}
	if json.Unmarshal(raw, out) != nil {
		t.Fatal("fixture JSON")
	}
}
func injectRows(t testing.TB, path string, rows map[string][]byte, updates map[string]any) {
	t.Helper()
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	raw, e := db.Get([]byte("meta"), nil)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		t.Fatal("metadata")
	}
	for k, v := range updates {
		b, _ := json.Marshal(v)
		raw = bytes.Replace(raw, append([]byte(`"`+k+`":`), fields[k]...), append([]byte(`"`+k+`":`), b...), 1)
	}
	batch := new(leveldb.Batch)
	batch.Put([]byte("meta"), raw)
	for k, v := range rows {
		batch.Put([]byte(k), v)
	}
	if e = db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
}

// Copied immutable genuine saved rows are conditional journal inputs here. The
// empty concrete admission stores and modeled lease do not claim new admission,
// account authentication, mining or an actual network publication.
func newResultFixture(t *testing.T) *resultFixture {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal profile")
	}
	ctx := context.Background()
	dir := os.Getenv("SLUG_RESULT_CRASH_ROOT")
	if dir == "" {
		dir = t.TempDir()
	}
	dir, e := filepath.EvalSymlinks(dir)
	if e != nil {
		t.Fatal(e)
	}
	f := &resultFixture{dir: dir, pin: loadReference(t).Pin}
	var bs struct {
		CreatedAtMS       int64 `json:"created_at_ms"`
		SourceCreatedAtMS int64 `json:"source_created_at_ms"`
	}
	fixtureJSON(t, "bout_snapshot", &bs)
	var cs struct {
		CreatedAtMS       int64 `json:"created_at_ms"`
		SourceCreatedAtMS int64 `json:"source_created_at_ms"`
	}
	fixtureJSON(t, "card_snapshot", &cs)
	var seed gymseed.Record
	var card gymcard.Record
	fixtureJSON(t, "seed_record", &seed)
	fixtureJSON(t, "card_record", &card)
	fixtureJSON(t, "bout_record", &f.record)
	ep := f.pin.SourcePin.SourcePin.SourcePin.SourcePin
	part := ep.Participants[0]
	for _, v := range ep.Participants {
		if v.StablePin.AccountID == f.pin.PublisherAccountID {
			part = v
		}
	}
	sp := part.StablePin
	f.now.Store(cs.SourceCreatedAtMS)
	sourceRows(t, filepath.Join(dir, "source"), sp, nil)
	f.source, e = registry.OpenManifestAuthority(filepath.Join(dir, "source"), sp.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	f.stable, e = gymstable.CreateForTestClock(filepath.Join(dir, "stable"), sp, f.source, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.entry, e = gymentry.CreateForTestClock(ctx, filepath.Join(dir, "entry"), part, f.stable, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.event, e = gymevent.CreateForTestClock(ctx, filepath.Join(dir, "event"), ep, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	seedPin := f.pin.SourcePin.SourcePin.SourcePin
	f.seed, e = gymseed.CreateForTestClock(ctx, filepath.Join(dir, "seed"), seedPin, f.event, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.seed.Close()
	raw, e := gymseed.MarshalRecord(seedPin, seed)
	if e != nil {
		t.Fatal(e)
	}
	rows := map[string][]byte{string(key('r', seed.EventKey)): raw}
	if seed.Publication != nil {
		c := seed.Publication.Coin.Outpoint
		k := append([]byte{'u'}, rawID(c.TxID)...)
		k = append(k, make([]byte, 4)...)
		binary.BigEndian.PutUint32(k[33:], c.Index)
		rows[string(k)] = rawID(seed.EventKey)
	}
	injectRows(t, filepath.Join(dir, "seed"), rows, map[string]any{"event_count": 1, "clock_ms": seed.Observation.ObservedAtMS})
	f.seed, e = gymseed.OpenForTestClock(ctx, filepath.Join(dir, "seed"), seedPin, f.event, f.clock)
	if e != nil {
		t.Fatal("seed reopen", e)
	}
	f.now.Store(cs.CreatedAtMS)
	cardPin := f.pin.SourcePin.SourcePin
	f.card, e = gymcard.CreateForTestClock(ctx, filepath.Join(dir, "card"), cardPin, f.seed, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.card.Close()
	raw, e = gymcard.MarshalRecord(cardPin, card)
	if e != nil {
		t.Fatal(e)
	}
	injectRows(t, filepath.Join(dir, "card"), map[string][]byte{string(key('r', card.CardID)): raw}, map[string]any{"event_count": 1, "clock_ms": card.CreatedAtMS})
	f.card, e = gymcard.OpenForTestClock(ctx, filepath.Join(dir, "card"), cardPin, f.seed, f.clock)
	if e != nil {
		t.Fatal("card reopen", e)
	}
	f.now.Store(bs.CreatedAtMS)
	f.bout, e = gymbout.CreateForTestClock(ctx, filepath.Join(dir, "bout"), f.pin.SourcePin, f.card, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.bout.Close()
	// Original frozen bytes, not outputs computed by this implementation.
	raw, e = os.ReadFile("testdata/reference/inputs/actual47/bout_record.json")
	if e != nil {
		t.Fatal(e)
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		t.Fatal("compact")
	}
	raw = compact.Bytes()
	injectRows(t, filepath.Join(dir, "bout"), map[string][]byte{string(key('r', f.record.BoutID)): raw}, map[string]any{"bout_count": 1, "clock_ms": f.record.CreatedAtMS})
	f.bout, e = gymbout.OpenForTestClock(ctx, filepath.Join(dir, "bout"), f.pin.SourcePin, f.card, f.clock)
	if e != nil {
		t.Fatal("bout reopen", e)
	}
	f.now.Store(f.record.CreatedAtMS + 1000)
	f.session = gymstable.Session{Generation: 1, ViewID: strings.Repeat("a", 64), Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if cap != gymaccount.View && cap != gymaccount.ResultPublish {
			return gymaccount.ErrForbidden
		}
		check := func() error {
			if f.revoked.Load() {
				return gymaccount.ErrUnauthorized
			}
			if cap == gymaccount.ResultPublish && f.held.Load() {
				return gymaccount.ErrHeld
			}
			return ctx.Err()
		}
		if e := check(); e != nil {
			return e
		}
		f.inLease.Add(1)
		defer f.inLease.Add(-1)
		a := gymaccount.Account{ID: sp.AccountID, Generation: 1, Enrollment: &sp.Identity}
		if e := fn(a, check); e != nil {
			return e
		}
		return check()
	}}
	f.s, e = create(ctx, filepath.Join(dir, "result"), f.pin, f.bout, f.clock, resultCrashHook())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		for _, x := range []interface{ Close() error }{f.s, f.bout, f.card, f.seed, f.event, f.entry, f.stable, f.source} {
			if x != nil {
				x.Close()
			}
		}
	})
	return f
}
func sourceRows(t *testing.T, path string, pin gymstable.Pin, records []registry.ManifestRecord) {
	t.Helper()
	if e := os.MkdirAll(filepath.Join(path, "db"), 0700); e != nil {
		t.Fatal(e)
	}
	descriptor, e := fighterdescriptor.ResolveCandidate(pin.Source.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	marker := struct {
		Application  string                       `json:"application"`
		Version      uint32                       `json:"version"`
		Network      uint32                       `json:"network"`
		Genesis      string                       `json:"genesis"`
		AuthorityID  string                       `json:"authority_id"`
		DescriptorID string                       `json:"descriptor_id"`
		Descriptor   fighterdescriptor.Descriptor `json:"descriptor"`
	}{"slug-prospect-registry", 5, pin.Source.Network, pin.Source.Genesis, pin.Source.AuthorityID, pin.Source.DescriptorID, descriptor}
	raw, _ := json.Marshal(marker)
	if e = os.WriteFile(filepath.Join(path, "SLUG-REGISTRY.json"), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	b := new(leveldb.Batch)
	jobs := map[string]bool{}
	var clock int64
	for _, r := range records {
		raw, _ = json.Marshal(r)
		b.Put([]byte("record:"+r.Verified.ProspectID), raw)
		k := append([]byte("sequence:"), make([]byte, 8)...)
		binary.BigEndian.PutUint64(k[len("sequence:"):], r.Sequence)
		b.Put(k, []byte(r.Verified.ProspectID))
		raw, _ = json.Marshal(r.Job)
		b.Put([]byte("job:"+r.Job.ID), raw)
		jobs[r.Job.ID] = true
		clock = max(clock, r.Birth.AcceptedAtMS)
	}
	// Structural fixture assembly uses the immutable independent records as input.
	// No new proof/receipt expectation is produced by the implementation under test.
	meta := struct {
		Version      uint32                       `json:"version"`
		Network      uint32                       `json:"network"`
		Genesis      string                       `json:"genesis"`
		AuthorityID  string                       `json:"authority_id"`
		DescriptorID string                       `json:"descriptor_id"`
		Descriptor   fighterdescriptor.Descriptor `json:"descriptor"`
		LastSequence uint64                       `json:"last_sequence"`
		JobCount     uint64                       `json:"job_count"`
		LastClockMS  int64                        `json:"last_clock_ms"`
	}{5, pin.Source.Network, pin.Source.Genesis, pin.Source.AuthorityID, pin.Source.DescriptorID, descriptor, uint64(len(records)), uint64(len(jobs)), clock}
	raw, _ = json.Marshal(meta)
	b.Put([]byte("meta"), raw)
	if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
}
