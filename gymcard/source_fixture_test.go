//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type eventFixture struct {
	viewOnly bool
	held     bool
	s        *gymevent.Store
	path     string
	p        gymevent.Pin
	now      int64
	stable   [2]*gymstable.Store
	entry    [2]*gymentry.Store
	source   [2]*registry.ManifestStore
	session  [2]gymstable.Session
	revoke   [2]bool
	ids      [2]string
}

func newEventFixture(t *testing.T, withEntries bool) *eventFixture {
	t.Helper()
	requireNormal(t)
	x := &eventFixture{p: eventPin(t), now: 1795000000000}
	dir := os.Getenv("SLUG_CARD_TEST_ROOT")
	if dir == "" {
		dir = realTemp(t)
	}
	x.path = filepath.Join(dir, "events")
	clock := func() time.Time { return time.UnixMilli(x.now) }
	raw, e := os.ReadFile("../gymstable/testdata/reference/stable_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Cases []struct {
			Fixture string
			Record  registry.ManifestRecord
		}
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("records")
	}
	for i, ep := range x.p.Participants {
		sp := ep.StablePin
		var records []registry.ManifestRecord
		for _, c := range corpus.Cases {
			if (c.Fixture == "zero256" && sp.Identity == fixtureIdentity("zero256")) || (c.Fixture == "incrementing256" && sp.Identity == fixtureIdentity("incrementing256")) {
				records = append(records, c.Record)
			}
		}
		sort.Slice(records, func(a, b int) bool { return records[a].Sequence < records[b].Sequence })
		if len(records) < 1 {
			t.Fatal("missing fixture records")
		}
		records = records[:1]
		sourcePath := filepath.Join(dir, "source"+string(rune('0'+i)))
		sourceRows(t, sourcePath, sp, records)
		x.source[i], e = registry.OpenManifestAuthority(sourcePath, sp.Source, false)
		if e != nil {
			t.Fatal(e)
		}
		x.stable[i], e = gymstable.CreateForTestClock(filepath.Join(dir, "stable"+string(rune('0'+i))), sp, x.source[i], clock)
		if e != nil {
			t.Fatal(e)
		}
		index := i
		x.session[i] = gymstable.Session{Generation: 1, ViewID: strings.Repeat(string(rune('a'+i)), 64), Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			if x.viewOnly && cap != gymaccount.View {
				return gymaccount.ErrForbidden
			}
			check := func() error {
				if x.revoke[index] {
					return gymaccount.ErrUnauthorized
				}
				return ctx.Err()
			}
			if e := check(); e != nil {
				return e
			}
			identity := sp.Identity
			account := gymaccount.Account{ID: sp.AccountID, Generation: 1, Enrollment: &identity}
			e := fn(account, check)
			if e != nil {
				return e
			}
			return check()
		}}
		synced, e := x.stable[i].Sync(context.Background(), x.session[i])
		if e != nil || len(synced.Items) != 1 {
			t.Fatal("normal source verification", e)
		}
		x.ids[i] = synced.Items[0].Assignment.ProspectID
		op, e := x.stable[i].PrepareSlot(context.Background(), x.session[i], gymstable.SlotRequest{Slot: 1, ProspectID: x.ids[i]})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = x.stable[i].ApplySlot(context.Background(), x.session[i], op.OperationID); e != nil {
			t.Fatal(e)
		}
		x.entry[i], e = gymentry.CreateForTestClock(context.Background(), filepath.Join(dir, "entry"+string(rune('0'+i))), ep, x.stable[i], clock)
		if e != nil {
			t.Fatal(e)
		}
		if withEntries {
			ev, e := gymentry.EventForDate(ep, "2026-11-21")
			if e != nil {
				t.Fatal(e)
			}
			req := gymentry.Request{Version: 1, EventID: ev.EventID, Saturday: ev.Saturday, Action: "replace", ExpectedRosterRevision: 1, ProspectIDs: []string{x.ids[i]}}
			in, e := x.entry[i].Prepare(context.Background(), x.session[i], req)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = x.entry[i].Apply(context.Background(), x.session[i], in.OperationID); e != nil {
				t.Fatal(e)
			}
		}
	}
	x.s, e = gymevent.CreateForTestClock(context.Background(), x.path, x.p, clock)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if x.s != nil {
			x.s.Close()
		}
		for i := range x.entry {
			if x.entry[i] != nil {
				x.entry[i].Close()
			}
			if x.stable[i] != nil {
				x.stable[i].Close()
			}
			if x.source[i] != nil {
				x.source[i].Close()
			}
		}
	})
	return x
}
func (x *eventFixture) cutoff(t *testing.T) int64 {
	t.Helper()
	ev, e := gymevent.EventForDate(x.p, "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	return ev.CutoffMS
}
func (x *eventFixture) reopen(t *testing.T) {
	t.Helper()
	x.s.Close()
	var e error
	x.s, e = gymevent.OpenForTestClock(context.Background(), x.path, x.p, func() time.Time { return time.UnixMilli(x.now) })
	if e != nil {
		t.Fatal(e)
	}
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

func fixtureIdentity(name string) gymaccount.Identity {
	v, _ := gymaccount.FixtureIdentity(name)
	return v
}
