// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
)

type entryCutoffReferenceHistory struct {
	Name, Fixture, Saturday string
	Source                  map[string]json.RawMessage
	Operations              []Operation                 `json:"entry_operations"`
	Expected                struct{ Entry CutoffEntry } `json:"expected_snapshot"`
}
type entryCutoffReferenceCorpus struct {
	Fixtures []struct {
		Fixture string
		Pin     Pin `json:"entry_pin"`
		View    struct {
			ViewID     string `json:"view_id"`
			Generation uint64
		} `json:"current_view"`
	}
	Histories []entryCutoffReferenceHistory
	Cases     []struct {
		Name, Family string
		Base         string `json:"base_history"`
		Count        uint32 `json:"expected_operation_count"`
		Operations   []Operation
		Accepted     bool
	} `json:"history_cases"`
	Maximum struct {
		Fixture, Saturday string
		Expected          CutoffEntry `json:"expected_entry"`
	} `json:"maximum_history"`
}

func readEntryCutoffReference(t *testing.T) entryCutoffReferenceCorpus {
	t.Helper()
	b, e := os.ReadFile("../gymevent/testdata/reference/event_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != "9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a" {
		t.Fatal("frozen oracle changed")
	}
	var c entryCutoffReferenceCorpus
	if json.Unmarshal(b, &c) != nil {
		t.Fatal("corpus")
	}
	return c
}

// Modeled operation rows exercise replay arithmetic and physical crosslinks.
// This private harness does not assert that synthetic inputs were admitted.
func installEntryCutoffReference(t *testing.T, p Pin, h entryCutoffReferenceHistory, ops []Operation, count uint32) *Store {
	t.Helper()
	db, e := leveldb.OpenFile(filepath.Join(t.TempDir(), "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	s := newStore(db, nil)
	t.Cleanup(func() { s.Close() })
	id, _ := ID(p)
	s.meta = metadata{Version: 1, StoreID: id, Pin: p, OperationCount: count, ObservedCheckpoint: uint64(len(h.Source)), ObservedSourceCount: uint64(len(h.Source)), ClockMS: h.Expected.Entry.Event.CutoffMS + 10000}
	rows := map[string]eventRow{}
	b := new(leveldb.Batch)
	seen := map[string]bool{}
	for index, o := range ops {
		i := o.Intent
		s.meta.LastOperationID = i.OperationID
		s.meta.ClockMS = max(s.meta.ClockMS, i.PreparedAtMS, o.AbandonedAtMS)
		s.meta.ObservedRosterRevision = max(s.meta.ObservedRosterRevision, i.Request.ExpectedRosterRevision)
		r, exists := rows[i.Request.EventID]
		if !exists {
			ev, e := EventForDate(p, i.Request.Saturday)
			if e != nil {
				t.Fatal(e)
			}
			entryID, _ := EntryID(id, ev.EventID, p.StablePin.Identity.GymID)
			r = eventRow{Version: 1, Event: ev, EntryID: entryID, CreatedAtMS: i.PreparedAtMS}
		}
		if o.State == "prepared" {
			s.meta.PendingOperationID = i.OperationID
		}
		if o.Receipt != nil {
			r.Revision = o.Receipt.Revision
			r.LatestOperationID = i.OperationID
			s.meta.ClockMS = max(s.meta.ClockMS, o.Receipt.CommittedAtMS)
			s.meta.ObservedCheckpoint = max(s.meta.ObservedCheckpoint, o.Receipt.StableCheckpoint)
			s.meta.ObservedSourceCount = max(s.meta.ObservedSourceCount, o.Receipt.ObservedSourceCount)
			s.meta.ObservedRosterRevision = max(s.meta.ObservedRosterRevision, o.Receipt.RosterRevision)
		}
		rows[i.Request.EventID] = r
		physicalID := i.OperationID
		if seen[physicalID] {
			physicalID = hash("reference duplicate physical row", rawID(physicalID), le64(uint64(index)))
		}
		seen[physicalID] = true
		if e = put(b, key("o", physicalID), o, MaxOperationSize); e != nil {
			t.Fatal(e)
		}
		b.Put(seqKey(i.Sequence), rawID(i.OperationID))
	}
	for id, r := range rows {
		r.Phase = "accepting"
		if s.meta.ClockMS >= r.Event.CutoffMS {
			r.Phase = "closed_pending_lock"
			r.ClosedObservedAtMS = s.meta.ClockMS
		}
		if e = put(b, key("e", id), r, MaxEventSize); e != nil {
			t.Fatal(e)
		}
	}
	s.meta.EventCount = uint32(len(rows))
	if e = db.Write(b, nil); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestEntryCutoffIndependentHistories(t *testing.T) {
	c := readEntryCutoffReference(t)
	pins := map[string]Pin{}
	histories := map[string]entryCutoffReferenceHistory{}
	for _, f := range c.Fixtures {
		pins[f.Fixture] = f.Pin
	}
	for _, h := range c.Histories {
		histories[h.Name] = h
		t.Run(h.Name, func(t *testing.T) {
			s := installEntryCutoffReference(t, pins[h.Fixture], h, h.Operations, uint32(len(h.Operations)))
			out, e := s.cutoffEntry(context.Background(), h.Expected.Entry.Event)
			if e != nil || !equal(out, h.Expected.Entry) {
				t.Fatal("independent entry history", e, out.HistoryDigest, h.Expected.Entry.HistoryDigest)
			}
		})
	}
	n := 0
	for _, v := range c.Cases {
		if v.Family != "entry" {
			continue
		}
		n++
		t.Run(v.Name, func(t *testing.T) {
			h, ok := histories[v.Base]
			if !ok {
				t.Fatal("base")
			}
			count := v.Count
			if count == 0 {
				count = uint32(len(v.Operations))
			}
			s := installEntryCutoffReference(t, pins[h.Fixture], h, v.Operations, count)
			_, e := s.cutoffEntry(context.Background(), h.Expected.Entry.Event)
			if (e == nil) != v.Accepted {
				t.Fatal("acceptance", e, v.Accepted)
			}
		})
	}
	if n != 11 {
		t.Fatalf("entry history case coverage %d", n)
	}
}
func TestEntryCutoffIndependentMaximum4096(t *testing.T) {
	c := readEntryCutoffReference(t)
	var base entryCutoffReferenceHistory
	var p Pin
	var view string
	var generation uint64
	for _, h := range c.Histories {
		if h.Name == c.Maximum.Fixture+"/ordinary" {
			base = h
		}
	}
	for _, f := range c.Fixtures {
		if f.Fixture == c.Maximum.Fixture {
			p = f.Pin
			view = f.View.ViewID
			generation = f.View.Generation
		}
	}
	ev, e := EventForDate(p, c.Maximum.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	sid, _ := ID(p)
	eid, _ := EntryID(sid, ev.EventID, p.StablePin.Identity.GymID)
	when := ev.CutoffMS - 1
	ops := make([]Operation, 0, 4096)
	for n := uint64(1); n <= 4096; n++ {
		h := sha256.Sum256([]byte("Set44 modeled public entry op " + view + " " + strconv.FormatUint(n, 10)))
		id := hex.EncodeToString(h[:])
		q := Request{Version: 1, EventID: ev.EventID, Saturday: ev.Saturday, Action: "withdraw", ExpectedRevision: n - 1, ProspectIDs: []string{}}
		in := Intent{Version: 1, Sequence: n, OperationID: id, Generation: generation, ViewID: view, Request: q, PreparedAtMS: when}
		r := &Receipt{Version: 1, StoreID: sid, EntryID: eid, Event: ev, OperationID: id, OperationSequence: n, Generation: generation, ViewID: view, Action: "withdraw", PreviousRevision: n - 1, Revision: n, StableCheckpoint: uint64(len(base.Source)), ObservedSourceCount: uint64(len(base.Source)), CommittedAtMS: when, Members: []Member{}}
		ops = append(ops, Operation{1, in, "committed", r, 0})
	}
	s := installEntryCutoffReference(t, p, base, ops, 4096)
	out, e := s.cutoffEntry(context.Background(), ev)
	if e != nil || !equal(out, c.Maximum.Expected) {
		t.Fatal(e, out.HistoryDigest, c.Maximum.Expected.HistoryDigest)
	}
}
