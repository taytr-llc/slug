// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
)

func sharedSequences(items []Item) []uint64 {
	seqs := make([]uint64, len(items))
	for i := range items {
		seqs[i] = items[i].Assignment.Sequence
	}
	return seqs
}

func TestSharedStableForeignPagesAndSparseInventory(t *testing.T) {
	x := newSharedFixture(t, "BBBBBBBBABA")
	a, b := x.gyms[0], x.gyms[1]
	ctx := context.Background()
	reads := 0
	a.s.proofRead = func() { reads++ }
	first := mustSync(t, a)
	if reads != 8 || first.Items == nil || len(first.Items) != 0 || first.Status.Checkpoint != 8 || first.Status.ObservedSourceCount != 11 || first.Status.CaughtUp {
		t.Fatalf("foreign-only page: reads=%d result=%+v", reads, first)
	}
	status, e := a.s.SharedStatus(ctx, a.session)
	if e != nil || status.Version != 1 || status.AssignmentCount != 0 || status.Status.Checkpoint != 8 {
		t.Fatal("global progress is not local count", status, e)
	}
	second := mustSync(t, a)
	if reads != 11 || !reflect.DeepEqual(sharedSequences(second.Items), []uint64{9, 11}) || !second.Status.CaughtUp || second.Status.Checkpoint != 11 {
		t.Fatal("second bounded global page", reads, second.Status, sharedSequences(second.Items))
	}
	for _, item := range second.Items {
		if item.Assignment.GymID != a.pin.Identity.GymID || item.record.Authorization.PublicKey != a.pin.Identity.GymPublicKey {
			t.Fatal("foreign assignment disclosed")
		}
	}
	if got := mustSync(t, b); len(got.Items) != 8 || got.Status.Checkpoint != 8 {
		t.Fatal("other gym's first page", got.Status)
	}
	if got := mustSync(t, b); !reflect.DeepEqual(sharedSequences(got.Items), []uint64{10}) || !got.Status.CaughtUp {
		t.Fatal("other gym's sparse suffix", got.Status, sharedSequences(got.Items))
	}
	for _, tc := range []struct {
		after uint64
		want  []uint64
		next  uint64
		more  bool
	}{{0, []uint64{9}, 9, true}, {8, []uint64{9}, 9, true}, {9, []uint64{11}, 11, false}, {10, []uint64{11}, 11, false}, {11, []uint64{}, 11, false}} {
		p, e := a.s.Inventory(ctx, a.session, tc.after, 1)
		if e != nil || !reflect.DeepEqual(sharedSequences(p.Items), tc.want) || p.After != tc.after || p.NextAfter != tc.next || p.HasMore != tc.more || p.Items == nil {
			t.Fatalf("after %d: seq=%v next=%d more=%v err=%v", tc.after, sharedSequences(p.Items), p.NextAfter, p.HasMore, e)
		}
	}
	recent, e := a.s.SharedRecent(ctx, a.session, 1)
	if e != nil || !reflect.DeepEqual(sharedSequences(recent.Items), []uint64{11}) || recent.After != 9 || recent.NextAfter != 11 || recent.HasMore {
		t.Fatal("latest local", recent, e)
	}
	foreign := b.records[0].Verified.ProspectID
	if _, e = a.s.PrepareSlot(ctx, a.session, SlotRequest{Slot: 1, ProspectID: foreign}); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign roster mutation", e)
	}
	if _, e = a.s.ReadEntrySelection(ctx, a.session, []string{foreign}); !errors.Is(e, ErrSyncRequired) {
		t.Fatal("foreign entry disclosure", e)
	}
	if a.s.meta.OperationCount != 0 {
		t.Fatal("foreign request persisted an operation")
	}
	a.now++
	id := a.records[0].Verified.ProspectID
	cutoffAssign(t, a, 0, 1, id)
	a.now++
	roster, e := a.s.Roster(ctx, a.session, 1, 8)
	if e != nil || len(roster.Items) != 1 || roster.Items[0].Item.Assignment.Sequence != 9 || roster.Status.Checkpoint != 11 {
		t.Fatal("sparse roster", e)
	}
	selected, _ := cutoffRead(t, a, a.now, []string{id})
	if len(selected.Members) != 1 || selected.Members[0].Assignment.Sequence != 9 || selected.Roster.Slots[0] != id {
		t.Fatal("sparse cutoff selection", selected)
	}
	reopenSharedFixture(t, a)
	status, e = a.s.SharedStatus(ctx, a.session)
	if e != nil || status.AssignmentCount != 2 || status.Status.Checkpoint != 11 || status.Status.RosterRevision != 1 || !status.Status.CaughtUp {
		t.Fatal("recovered sparse state", status, e)
	}
	again, _ := cutoffRead(t, a, a.now, []string{id})
	if !equalEncoded(selected, again) {
		t.Fatal("restart changed cutoff membership")
	}
}

func TestSharedStableRecentUsesLocalTail(t *testing.T) {
	// Cursor expectations are independently frozen in the Python model, not
	// calculated by copying the implementation's paging arithmetic.
	type modeledPage struct {
		After     uint64
		Limit     uint32
		Items     []uint64
		NextAfter uint64 `json:"next_after"`
		HasMore   bool   `json:"has_more"`
	}
	var model struct {
		Owners []string `json:"modeled_owners"`
		Models []struct {
			Owner string
			Sync  []struct {
				Start, Checkpoint uint64
				AssignmentCount   uint64 `json:"assignment_count"`
				Items, Skips      []uint64
			}
			Pages, Recent []modeledPage
		}
	}
	raw, e := os.ReadFile("testdata/shared-reference/shared_v1.json")
	if e != nil || json.Unmarshal(raw, &model) != nil || len(model.Models) != 2 || len(model.Owners) != 12 {
		t.Fatal("independent cursor model missing", e)
	}
	x := newSharedFixture(t, strings.Join(model.Owners, ""))
	ctx := context.Background()
	for _, want := range model.Models {
		g := x.gyms[int(want.Owner[0]-'A')]
		for _, step := range want.Sync {
			if g.s.meta.Checkpoint != step.Start {
				t.Fatal("unexpected sync start", g.s.meta.Checkpoint, step.Start)
			}
			got := mustSync(t, g)
			status, e := g.s.SharedStatus(ctx, g.session)
			if e != nil || got.Status.Checkpoint != step.Checkpoint || status.AssignmentCount != step.AssignmentCount || !reflect.DeepEqual(sharedSequences(got.Items), step.Items) {
				t.Fatal("sync differs from independent model", want.Owner, step, status, e)
			}
			for _, skipped := range step.Skips {
				if exists, e := g.s.db.Has(sharedSkipKey(skipped), nil); e != nil || !exists {
					t.Fatal("modeled foreign row missing witness", skipped, e)
				}
			}
		}
		check := func(got Page, e error, page modeledPage) {
			t.Helper()
			if e != nil || !reflect.DeepEqual(sharedSequences(got.Items), page.Items) || got.After != page.After || got.NextAfter != page.NextAfter || got.HasMore != page.HasMore {
				t.Fatalf("%s cursor mismatch: want=%+v got sequence=%v after=%d next=%d more=%v err=%v", want.Owner, page, sharedSequences(got.Items), got.After, got.NextAfter, got.HasMore, e)
			}
		}
		for _, page := range want.Pages {
			got, e := g.s.Inventory(ctx, g.session, page.After, page.Limit)
			check(got, e, page)
		}
		for _, page := range want.Recent {
			got, e := g.s.SharedRecent(ctx, g.session, page.Limit)
			check(got, e, page)
		}
	}
}

func TestSharedStableCancellationResetAndCommitUncertainty(t *testing.T) {
	for _, mode := range []string{"cancel-foreign", "reset-after-commit", "write-after-commit"} {
		t.Run(mode, func(t *testing.T) {
			x := newSharedFixture(t, "BA")
			a := x.gyms[0]
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			session := a.session
			reads := 0
			a.s.proofRead = func() {
				reads++
				if mode == "cancel-foreign" {
					cancel()
				}
			}
			switch mode {
			case "reset-after-commit":
				revoked := false
				a.s.checkpoint = func(stage string) {
					if stage == "after_sync" {
						revoked = true
					}
				}
				original := session.Lease
				session.Lease = func(c context.Context, capability gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
					e := original(c, capability, fn)
					if revoked {
						return gymaccount.ErrUnauthorized
					}
					return e
				}
			case "write-after-commit":
				a.s.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
					if e := a.s.db.Write(batch, options); e != nil {
						return e
					}
					return errors.New("test: durable write acknowledgment lost")
				}
			}
			result, e := a.s.Sync(ctx, session)
			want := ErrOutcomeUnknown
			checkpoint, local := uint64(2), uint64(1)
			if mode == "cancel-foreign" {
				want, checkpoint, local = ErrCancelled, 0, 0
				if reads != 1 {
					t.Fatal("continued after cancelled foreign row", reads)
				}
			}
			if !errors.Is(e, want) || result.Version != 0 || result.Items != nil {
				t.Fatal("uncertain/cancelled response disclosed state", result, e)
			}
			reopenSharedFixture(t, a)
			status, e := a.s.SharedStatus(context.Background(), a.session)
			if e != nil || status.Status.Checkpoint != checkpoint || status.AssignmentCount != local {
				t.Fatal("atomic cursor/assignment/skip recovery", status, e)
			}
			if mode == "cancel-foreign" {
				mustSync(t, a)
			}
			again := mustSync(t, a)
			if len(again.Items) != 0 || again.Status.Checkpoint != 2 {
				t.Fatal("restart duplicated assignment", again)
			}
		})
	}
}

func TestSharedStableSourceHealthAndObservedGrowth(t *testing.T) {
	x := newSharedFixture(t, "A")
	a := x.gyms[0]
	mustSync(t, a)
	x.append(t, 'B')
	status, e := a.s.SharedStatus(context.Background(), a.session)
	if e != nil || status.AssignmentCount != 1 || status.Status.Checkpoint != 1 || status.Status.ObservedSourceCount != 2 || status.Status.CaughtUp {
		t.Fatal("foreign growth omitted", status, e)
	}
	reopenSharedFixture(t, a)
	if a.s.meta.ObservedSourceCount != 2 {
		t.Fatal("foreign highwater lost")
	}
	// A complete but older prefix at the same genuine authority pin cannot make
	// the learned foreign row disappear, even though it was not assigned locally.
	oldPath := filepath.Join(t.TempDir(), "old-source")
	sourceRows(t, oldPath, a.pin, x.all[:1])
	old, e := registry.OpenManifestAuthority(oldPath, a.pin.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	defer old.Close()
	if e = a.s.Close(); e != nil {
		t.Fatal(e)
	}
	a.s = nil
	if reopened, e := OpenShared(a.path, a.pin, old); !errors.Is(e, ErrCorrupt) {
		if reopened != nil {
			reopened.Close()
		}
		t.Fatal("observed foreign source rollback accepted", e)
	}
	reopenSharedFixture(t, a)
	if e = a.source.Close(); e != nil {
		t.Fatal(e)
	}
	if p, e := a.s.SharedRecent(context.Background(), a.session, 1); !errors.Is(e, ErrClosed) || p.Version != 0 || p.Items != nil {
		t.Fatal("closed source disclosed cached recent page", e)
	}
	if s, e := a.s.SharedStatus(context.Background(), a.session); !errors.Is(e, ErrClosed) || s.Version != 0 {
		t.Fatal("closed source labeled healthy", e)
	}
}

func TestSharedStableCloseJoinsForeignVerification(t *testing.T) {
	x := newSharedFixture(t, "BA")
	a := x.gyms[0]
	entered, release := make(chan struct{}), make(chan struct{})
	a.s.proofRead = func() { close(entered); <-release }
	syncDone := make(chan error, 1)
	go func() { _, e := a.s.Sync(context.Background(), a.session); syncDone <- e }()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- a.s.Close() }()
	<-a.s.life.Done()
	select {
	case e := <-closed:
		t.Fatal("Close returned while source work was still joined", e)
	default:
	}
	close(release)
	if e := <-syncDone; !errors.Is(e, ErrCancelled) {
		t.Fatal("close did not cancel source read", e)
	}
	if e := <-closed; e != nil {
		t.Fatal(e)
	}
	reopenSharedFixture(t, a)
	status, e := a.s.SharedStatus(context.Background(), a.session)
	if e != nil || status.Status.Checkpoint != 0 || status.AssignmentCount != 0 {
		t.Fatal("close published incomplete foreign skip", status, e)
	}
}

func TestSharedStableNamespaceAndPinIsolation(t *testing.T) {
	x := newSharedFixture(t, "")
	a := x.gyms[0]
	raw, e := os.ReadFile(filepath.Join(a.path, sharedMarkerName))
	if e != nil {
		t.Fatal(e)
	}
	var m marker
	if e = json.Unmarshal(raw, &m); e != nil || m.Application != "slug-candidate-shared-stable" || m.Version != 2 || m.Pin != a.pin {
		t.Fatal("explicit marker", m, e)
	}
	if _, e = os.Stat(filepath.Join(a.path, markerName)); !os.IsNotExist(e) {
		t.Fatal("legacy marker created", e)
	}
	if s, e := Create(filepath.Join(t.TempDir(), "wrong"), a.pin, a.source); !errors.Is(e, ErrInvalid) {
		if s != nil {
			s.Close()
		}
		t.Fatal("legacy constructor accepted shared pin", e)
	}
	legacy := a.pin
	legacy.Version = 1
	if s, e := CreateShared(filepath.Join(t.TempDir(), "wrong"), legacy, a.source); !errors.Is(e, ErrInvalid) {
		if s != nil {
			s.Close()
		}
		t.Fatal("shared constructor accepted legacy pin", e)
	}
	if e = a.s.Close(); e != nil {
		t.Fatal(e)
	}
	a.s = nil
	for _, pin := range []Pin{a.pin, legacy} {
		if s, e := Open(a.path, pin, a.source); e == nil {
			s.Close()
			t.Fatal("legacy open adopted shared namespace")
		}
	}
	reopenSharedFixture(t, a)
	foreignSession := fixtureSession(x.gyms[1].pin, 1, strings.Repeat("a", 64))
	if s, e := a.s.SharedStatus(context.Background(), foreignSession); e == nil || s.Version != 0 {
		t.Fatal("foreign account disclosed shared metadata", e)
	}
}
