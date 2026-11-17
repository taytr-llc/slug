// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
)

type cutoffReferenceHistory struct {
	Name, Fixture, Saturday string
	Source                  map[string]struct{ Assignment Assignment }
	Operations              []Operation                   `json:"stable_operations"`
	Expected                struct{ Roster CutoffRoster } `json:"expected_snapshot"`
}
type cutoffReferenceCorpus struct {
	Fixtures []struct {
		Fixture string
		Pin     struct {
			StablePin Pin `json:"stable_pin"`
		} `json:"entry_pin"`
		View struct {
			ViewID     string `json:"view_id"`
			Generation uint64
		} `json:"current_view"`
	}
	Histories []cutoffReferenceHistory
	Cases     []struct {
		Name, BaseHistory, Family string
		Count                     uint32 `json:"expected_operation_count"`
		Operations                []Operation
		Accepted                  bool
		Expected                  CutoffRoster `json:"expected_roster"`
		Patch                     struct {
			ProspectID   string `json:"prospect_id"`
			AcceptedAtMS int64  `json:"accepted_at_ms"`
		} `json:"assignment_patch"`
	} `json:"history_cases"`
	Maximum struct {
		Fixture, Saturday string
		Expected          CutoffRoster `json:"expected_roster"`
	} `json:"maximum_history"`
}

func readCutoffReference(t *testing.T) cutoffReferenceCorpus {
	t.Helper()
	b, e := os.ReadFile("../gymevent/testdata/reference/event_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != "9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a" {
		t.Fatal("frozen oracle changed")
	}
	// Explicit tags for snake_case case fields are retained separately below.
	var raw struct {
		cutoffReferenceCorpus
		Cases []json.RawMessage `json:"history_cases"`
	}
	if json.Unmarshal(b, &raw) != nil {
		t.Fatal("corpus")
	}
	var c cutoffReferenceCorpus
	if json.Unmarshal(b, &c) != nil {
		t.Fatal("corpus")
	}
	for i, b := range raw.Cases {
		var v struct {
			Base string `json:"base_history"`
		}
		json.Unmarshal(b, &v)
		c.Cases[i].BaseHistory = v.Base
	}
	return c
}

// These are independently modeled journal rows, not admitted records. This
// private projection test never calls a proof-bearing source method.
func installCutoffReference(t *testing.T, p Pin, h cutoffReferenceHistory, ops []Operation, count uint32) *Store {
	t.Helper()
	db, e := leveldb.OpenFile(filepath.Join(t.TempDir(), "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	s := newStore(db, nil)
	t.Cleanup(func() { s.Close() })
	id, _ := ID(p)
	s.meta = metadata{StoreID: id, Pin: p, Checkpoint: uint64(len(h.Source)), ObservedSourceCount: uint64(len(h.Source)), OperationCount: count, ClockMS: h.Expected.Roster.CutoffMS + 10000}
	s.roster = rosterRow{Version: 1}
	b := new(leveldb.Batch)
	for _, x := range h.Source {
		a := x.Assignment
		s.meta.ClockMS = max(s.meta.ClockMS, a.AcceptedAtMS)
		encodedPut(b, seqKey(a.Sequence), a, MaxAssignmentSize)
		idx := make([]byte, 8)
		binary.BigEndian.PutUint64(idx, a.Sequence)
		b.Put(idKey('i', a.ProspectID), idx)
	}
	seenKeys := map[string]bool{}
	for index, o := range ops {
		s.meta.LastOperationID = o.Intent.OperationID
		s.meta.ClockMS = max(s.meta.ClockMS, o.Intent.PreparedAtMS, o.AbandonedAtMS)
		if o.State == "prepared" {
			s.meta.PendingOperationID = o.Intent.OperationID
		}
		if o.Receipt != nil {
			s.meta.RosterRevision = o.Receipt.Revision
			s.roster.Revision = o.Receipt.Revision
			s.roster.Slots = o.Receipt.Slots
			s.meta.ClockMS = max(s.meta.ClockMS, o.Receipt.CommittedAtMS)
		}
		physicalID := o.Intent.OperationID
		if seenKeys[physicalID] {
			// Model rows are a multiset; LevelDB keys are unique. Preserve the
			// duplicate as an extra corrupt crosslinked row, never overwrite it
			// and accidentally turn the negative fixture into a valid journal.
			physicalID = cutoffHash("reference duplicate physical row", cutoffRaw(physicalID), cutoffLE(uint64(index)))
		}
		seenKeys[physicalID] = true
		if e = encodedPut(b, idKey('o', physicalID), o, MaxOperationSize); e != nil {
			t.Fatal(e)
		}
	}
	if e = db.Write(b, nil); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestStableCutoffIndependentHistories(t *testing.T) {
	c := readCutoffReference(t)
	pins := map[string]Pin{}
	histories := map[string]cutoffReferenceHistory{}
	for _, f := range c.Fixtures {
		pins[f.Fixture] = f.Pin.StablePin
	}
	for _, h := range c.Histories {
		histories[h.Name] = h
		t.Run(h.Name, func(t *testing.T) {
			s := installCutoffReference(t, pins[h.Fixture], h, h.Operations, uint32(len(h.Operations)))
			out, e := s.cutoffRoster(context.Background(), h.Expected.Roster.CutoffMS)
			if e != nil || !equalEncoded(out, h.Expected.Roster) {
				t.Fatal("independent cutoff roster", e, out.HistoryDigest, h.Expected.Roster.HistoryDigest)
			}
		})
	}
	n := 0
	for _, v := range c.Cases {
		if v.Family != "stable" {
			continue
		}
		n++
		t.Run(v.Name, func(t *testing.T) {
			h, ok := histories[v.BaseHistory]
			if !ok {
				t.Fatal("base")
			}
			ops := v.Operations
			if ops == nil {
				ops = h.Operations
			}
			if v.Patch.ProspectID != "" {
				x := h.Source[v.Patch.ProspectID]
				x.Assignment.AcceptedAtMS = v.Patch.AcceptedAtMS
				h.Source[v.Patch.ProspectID] = x
			}
			count := v.Count
			if count == 0 {
				count = uint32(len(ops))
			}
			s := installCutoffReference(t, pins[h.Fixture], h, ops, count)
			out, e := s.cutoffRoster(context.Background(), h.Expected.Roster.CutoffMS)
			if (e == nil) != v.Accepted {
				t.Fatal("acceptance", e, v.Accepted)
			}
			if v.Accepted && !equalEncoded(out, v.Expected) {
				t.Fatal("accepted successor changed historical result")
			}
		})
	}
	if n != 12 {
		t.Fatalf("stable history case coverage %d", n)
	}
}
func TestStableCutoffIndependentMaximum4096(t *testing.T) {
	c := readCutoffReference(t)
	var base cutoffReferenceHistory
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
			p = f.Pin.StablePin
			view = f.View.ViewID
			generation = f.View.Generation
		}
	}
	ids := []string{}
	for id := range base.Source {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	target := ids[0]
	when := c.Maximum.Expected.CutoffMS - 1
	ops := make([]Operation, 0, 4096)
	var slots [10]string
	var revision uint64
	for n := uint64(1); n <= 4096; n++ {
		digest := sha256.Sum256([]byte("Set44 modeled public stable op " + view + " " + strconv.FormatUint(n, 10)))
		id := hex.EncodeToString(digest[:])
		q := SlotRequest{revision, 1, target}
		previous := slots[0]
		changed := previous != target
		next := revision
		if changed {
			next++
		}
		slots[0] = target
		r := &MutationReceipt{1, id, generation, view, q, previous, changed, revision, next, slots, when}
		ops = append(ops, Operation{1, Intent{1, n, id, generation, view, q, when}, "committed", r, 0})
		revision = next
	}
	s := installCutoffReference(t, p, base, ops, 4096)
	out, e := s.cutoffRoster(context.Background(), c.Maximum.Expected.CutoffMS)
	if e != nil || !equalEncoded(out, c.Maximum.Expected) {
		t.Fatal(e, out.HistoryDigest, c.Maximum.Expected.HistoryDigest)
	}
}
