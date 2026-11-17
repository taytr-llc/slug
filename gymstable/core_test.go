// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type stableCorpus struct {
	Pins []struct {
		Name string
		Pin  Pin
	}
	Cases []struct {
		Fixture    string
		Record     registry.ManifestRecord
		Assignment Assignment
		Conditions []struct {
			AsOf     int64 `json:"as_of_ms"`
			Expected fightercondition.Snapshot
		}
	}
}

func readCoreCorpus(t *testing.T) stableCorpus {
	t.Helper()
	raw, e := os.ReadFile("testdata/reference/stable_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var f stableCorpus
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	return f
}

type coreFixture struct {
	path, sourcePath string
	source           *registry.ManifestStore
	s                *Store
	pin              Pin
	session          Session
	now              int64
	records          []registry.ManifestRecord
}

func sourceRows(t *testing.T, path string, pin Pin, records []registry.ManifestRecord) {
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
func newCoreFixture(t *testing.T, name string, count int) *coreFixture {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-profile verified source")
	}
	f := readCoreCorpus(t)
	x := &coreFixture{}
	for _, p := range f.Pins {
		if p.Name == name {
			x.pin = p.Pin
		}
	}
	for _, c := range f.Cases {
		if c.Fixture == name {
			x.records = append(x.records, c.Record)
		}
	}
	sort.Slice(x.records, func(i, j int) bool { return x.records[i].Sequence < x.records[j].Sequence })
	if count >= 0 {
		x.records = x.records[:count]
	}
	dir := t.TempDir()
	x.path = filepath.Join(dir, "stable")
	x.sourcePath = filepath.Join(dir, "source")
	sourceRows(t, x.sourcePath, x.pin, x.records)
	var e error
	x.source, e = registry.OpenManifestAuthority(x.sourcePath, x.pin.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range x.records {
		x.now = max(x.now, r.Birth.AcceptedAtMS)
	}
	if x.now == 0 {
		x.now = 1794900000000
	}
	x.s, e = create(x.path, x.pin, x.source, func() time.Time { return time.UnixMilli(x.now) }, nil)
	if e != nil {
		t.Fatal(e)
	}
	x.session = fixtureSession(x.pin, 1, strings.Repeat("a", 64))
	t.Cleanup(func() {
		if x.s != nil {
			if e := x.s.Close(); e != nil {
				t.Error(e)
			}
		}
		if x.source != nil {
			if e := x.source.Close(); e != nil {
				t.Error(e)
			}
		}
	})
	return x
}
func fixtureSession(pin Pin, generation uint64, view string) Session {
	return Session{Generation: generation, ViewID: view, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		identity := pin.Identity
		a := gymaccount.Account{ID: pin.AccountID, Generation: generation, Enrollment: &identity}
		return fn(a, func() error { return ctx.Err() })
	}}
}
func (x *coreFixture) reopen(t *testing.T) {
	t.Helper()
	if e := x.s.Close(); e != nil {
		t.Fatal(e)
	}
	var e error
	x.s, e = Open(x.path, x.pin, x.source)
	if e != nil {
		t.Fatal(e)
	}
	x.s.now = func() time.Time { return time.UnixMilli(x.now) }
}
func mustSync(t *testing.T, x *coreFixture) SyncResult {
	t.Helper()
	r, e := x.s.Sync(context.Background(), x.session)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestStableIntegratedConditionBoundaries(t *testing.T) {
	f := readCoreCorpus(t)
	for _, tc := range f.Cases {
		t.Run(tc.Fixture+"/"+string(rune('0'+tc.Record.Sequence)), func(t *testing.T) {
			x := newCoreFixture(t, tc.Fixture, int(tc.Record.Sequence))
			initial, e := x.s.Status(context.Background(), x.session)
			if e != nil || initial.CaughtUp || initial.ObservedSourceCount != tc.Record.Sequence {
				t.Fatal(initial, e)
			}
			synced := mustSync(t, x)
			if len(synced.Items) != int(tc.Record.Sequence) || !synced.Status.CaughtUp {
				t.Fatal(synced.Status)
			}
			cases := append(tc.Conditions[:0:0], tc.Conditions...)
			sort.Slice(cases, func(i, j int) bool { return cases[i].AsOf < cases[j].AsOf })
			for _, c := range cases {
				if c.AsOf < x.now {
					t.Fatal("reference predates source")
				}
				x.now = c.AsOf
				p, e := x.s.Inventory(context.Background(), x.session, tc.Record.Sequence-1, 1)
				if e != nil || len(p.Items) != 1 || p.Items[0].Condition != c.Expected || p.Items[0].Assignment != tc.Assignment {
					t.Fatal("integrated condition", tc.Record.Sequence, c.AsOf, e)
				}
				r := p.Items[0].Receipt()
				if len(r.Proof.MerkleBranch) > 0 {
					r.Proof.MerkleBranch[0] = "mutated"
					if p.Items[0].Receipt().Proof.MerkleBranch[0] == "mutated" {
						t.Fatal("receipt alias")
					}
				}
			}
			x.reopen(t)
			status, e := x.s.Status(context.Background(), x.session)
			if e != nil || status.Checkpoint != tc.Record.Sequence || status.RosterRevision != 0 {
				t.Fatal(status, e)
			}
		})
	}
}
func TestStableRosterReplayViewsAndReopen(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	mustSync(t, x)
	ctx := context.Background()
	id := x.records[0].Verified.ProspectID
	request := SlotRequest{Slot: 1, ProspectID: id}
	intent, e := x.s.PrepareSlot(ctx, x.session, request)
	if e != nil {
		t.Fatal(e)
	}
	again, e := x.s.PrepareSlot(ctx, x.session, request)
	if e != nil || again != intent {
		t.Fatal("prepare replay", e)
	}
	if _, e = x.s.PrepareSlot(ctx, x.session, SlotRequest{Slot: 2}); !errors.Is(e, ErrPending) {
		t.Fatal(e)
	}
	x.reopen(t)
	result, e := x.s.ApplySlot(ctx, x.session, intent.OperationID)
	if e != nil || !result.Receipt.Changed || result.Receipt.Revision != 1 {
		t.Fatal(result, e)
	}
	x.reopen(t)
	replay, e := x.s.ApplySlot(ctx, x.session, intent.OperationID)
	if e != nil || !replay.Replayed || replay.Receipt != result.Receipt {
		t.Fatal(replay, e)
	}
	next := fixtureSession(x.pin, 2, strings.Repeat("b", 64))
	o, e := x.s.Operation(ctx, next, intent.OperationID)
	if e != nil || o.Intent != intent {
		t.Fatal(e)
	}
	if _, e = x.s.ApplySlot(ctx, next, intent.OperationID); !errors.Is(e, ErrStaleView) {
		t.Fatal(e)
	}
	if _, e = x.s.PrepareSlot(ctx, next, SlotRequest{ExpectedRevision: 1, Slot: 2, ProspectID: id}); !errors.Is(e, ErrConflict) {
		t.Fatal("duplicate slot", e)
	}
	noop, e := x.s.PrepareSlot(ctx, next, SlotRequest{ExpectedRevision: 1, Slot: 1, ProspectID: id})
	if e != nil {
		t.Fatal(e)
	}
	n, e := x.s.ApplySlot(ctx, next, noop.OperationID)
	if e != nil || n.Receipt.Changed || n.Receipt.Revision != 1 {
		t.Fatal(n, e)
	}
	p, e := x.s.PrepareSlot(ctx, next, SlotRequest{ExpectedRevision: 1, Slot: 1})
	if e != nil {
		t.Fatal(e)
	}
	old := fixtureSession(x.pin, 3, strings.Repeat("c", 64))
	if _, e = x.s.ApplySlot(ctx, old, p.OperationID); !errors.Is(e, ErrStaleView) {
		t.Fatal(e)
	}
	abandoned, e := x.s.Abandon(ctx, old, p.OperationID)
	if e != nil || abandoned.State != "abandoned" {
		t.Fatal(e)
	}
	x.reopen(t)
	page, e := x.s.Roster(ctx, old, 1, 8)
	if e != nil || len(page.Items) != 1 || page.Items[0].Slot != 1 || page.NextSlot != 9 || page.Status.RosterRevision != 1 {
		t.Fatal(page, e)
	}
	empty, e := x.s.Pending(ctx, old)
	if e != nil || empty != nil {
		t.Fatal(e)
	}
}
func TestStableCancellationAndCloseJoin(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	x.s.proofRead = func() { calls++; cancel() }
	r, e := x.s.Sync(ctx, x.session)
	if !errors.Is(e, ErrCancelled) || r.Version != 0 || calls != 1 || x.s.meta.Checkpoint != 0 {
		t.Fatal(r, e, calls)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	x.s.proofRead = func() { close(entered); <-release }
	work := make(chan error, 1)
	go func() { _, e := x.s.Sync(context.Background(), x.session); work <- e }()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- x.s.Close() }()
	select {
	case e := <-closed:
		t.Fatal("Close returned before real work joined", e)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if e = <-work; !errors.Is(e, ErrCancelled) {
		t.Fatal(e)
	}
	if e = <-closed; e != nil {
		t.Fatal(e)
	}
	x.s = nil
}
func TestStableWriteUnknownAndFinalLeaseError(t *testing.T) {
	for _, stage := range []string{"write", "lease"} {
		t.Run(stage, func(t *testing.T) {
			x := newCoreFixture(t, "zero256", 0)
			ctx := context.Background()
			i, e := x.s.PrepareSlot(ctx, x.session, SlotRequest{Slot: 1})
			if e != nil {
				t.Fatal(e)
			}
			if stage == "write" {
				writer := x.s.writeBatch
				x.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
					if e := writer(b, o); e != nil {
						return e
					}
					return errors.New("lost disk acknowledgement")
				}
			}
			session := x.session
			if stage == "lease" {
				base := session.Lease
				calls := 0
				session.Lease = func(ctx context.Context, c gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
					calls++
					e := base(ctx, c, fn)
					if calls == 2 && e == nil {
						return gymaccount.ErrExpired
					}
					return e
				}
			}
			result, e := x.s.ApplySlot(ctx, session, i.OperationID)
			if !errors.Is(e, ErrOutcomeUnknown) || result.Version != 0 {
				t.Fatal(result, e)
			}
			if stage == "write" {
				if _, e = x.s.Pending(ctx, x.session); !errors.Is(e, ErrPoisoned) {
					t.Fatal(e)
				}
			}
			x.reopen(t)
			o, e := x.s.Operation(ctx, x.session, i.OperationID)
			if e != nil || o.State != "committed" || o.Receipt.Changed {
				t.Fatal(o, e)
			}
		})
	}
}
func TestStableClockAuthorizationAndCodec(t *testing.T) {
	x := newCoreFixture(t, "zero256", 0)
	ctx := context.Background()
	original := x.s.meta.ClockMS
	bad := x.session
	bad.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	x.now++
	if _, e := x.s.Status(ctx, bad); !errors.Is(e, ErrUnauthorized) || x.s.meta.ClockMS != original {
		t.Fatal(e)
	}
	if _, e := x.s.Status(ctx, x.session); e != nil {
		t.Fatal(e)
	}
	x.reopen(t)
	x.now = original
	if _, e := x.s.Status(ctx, x.session); !errors.Is(e, ErrClock) {
		t.Fatal(e)
	}
	raw, _ := MarshalPin(x.pin)
	for _, b := range [][]byte{append(raw, raw...), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1e0`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":null`), 1)} {
		if _, e := ParsePin(b); e == nil {
			t.Fatal("accepted bad JSON")
		}
	}
	request := SlotRequest{Slot: 1}
	r, _ := MarshalSlotRequest(request)
	if parsed, e := ParseSlotRequest(append([]byte(" \n"), r...)); e != nil || parsed != request {
		t.Fatal(e)
	}
}
