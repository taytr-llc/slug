// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// Expanded records are behavioral public-fixture inputs, not golden outputs.
// Real normal Argon and the published fixture scalar authenticate every row.
func expandCoreFixture(t *testing.T, x *coreFixture, count int) {
	t.Helper()
	raw, e := os.ReadFile("../gymaccount/testdata/enrollment_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var keys struct {
		Cases []struct {
			Private       string `json:"test_only_private_key"`
			Authorization gymauth.Authorization
		}
	}
	if e = json.Unmarshal(raw, &keys); e != nil {
		t.Fatal(e)
	}
	var scalar []byte
	for _, k := range keys.Cases {
		if k.Authorization.PublicKey == x.pin.Identity.GymPublicKey {
			scalar, _ = hex.DecodeString(k.Private)
			break
		}
	}
	if len(scalar) != 32 {
		t.Fatal("published key absent")
	}
	key, _ := btcec.PrivKeyFromBytes(scalar)
	defer key.Zero()
	base := x.records[len(x.records)-1]
	for len(x.records) < count {
		r := base
		r.Sequence = uint64(len(x.records) + 1)
		header, _ := hex.DecodeString(r.Proof.Header)
		binary.LittleEndian.PutUint32(header[76:], uint32(50000+r.Sequence))
		r.Proof.Header = hex.EncodeToString(header)
		r.Verified, e = prospect.Verify(r.Context, r.Proof)
		if e != nil {
			t.Fatal(e)
		}
		r.Authorization, e = gymauth.SignAdmission(r.Context, r.Proof, key)
		if e != nil {
			t.Fatal(e)
		}
		m, e := fightermanifest.GenerateText(1, r.Verified.WorkHash)
		if e != nil {
			t.Fatal(e)
		}
		r.StaticBinding.ManifestDigest, e = fightermanifest.Digest(m)
		if e != nil {
			t.Fatal(e)
		}
		r.StaticBinding.PortraitDigest, e = fighterportrait.Digest(1, m)
		if e != nil {
			t.Fatal(e)
		}
		r.Eligibility = registry.Eligibility{RuleVersion: registry.EligibilityRuleVersion2, Source: "current-parent", Hash: r.Context.ParentHash, Height: r.Context.ParentHeight}
		x.records = append(x.records, r)
	}
	if e = x.s.Close(); e != nil {
		t.Fatal(e)
	}
	if e = x.source.Close(); e != nil {
		t.Fatal(e)
	}
	sourceRows(t, x.sourcePath, x.pin, x.records)
	x.source, e = registry.OpenManifestAuthority(x.sourcePath, x.pin.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	x.s, e = Open(x.path, x.pin, x.source)
	if e != nil {
		t.Fatal(e)
	}
	x.s.now = func() time.Time { return time.UnixMilli(x.now) }
}

func TestStableEightProofCeilingAndTenSlots(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	expandCoreFixture(t, x, 11)
	ctx := context.Background()
	calls := 0
	x.s.proofRead = func() { calls++ }
	first := mustSync(t, x)
	if calls != 8 || len(first.Items) != 8 || first.Status.CaughtUp {
		t.Fatal(calls, first.Status)
	}
	second := mustSync(t, x)
	if calls != 11 || len(second.Items) != 3 || !second.Status.CaughtUp {
		t.Fatal(calls, second.Status)
	}
	for slot := 1; slot <= 10; slot++ {
		i, e := x.s.PrepareSlot(ctx, x.session, SlotRequest{ExpectedRevision: uint64(slot - 1), Slot: uint32(slot), ProspectID: x.records[slot-1].Verified.ProspectID})
		if e != nil {
			t.Fatal(e)
		}
		before := calls
		r, e := x.s.ApplySlot(ctx, x.session, i.OperationID)
		if e != nil || calls != before+1 || r.Receipt.Revision != uint64(slot) {
			t.Fatal(r, e, calls)
		}
	}
	before := calls
	p, e := x.s.Roster(ctx, x.session, 1, 8)
	if e != nil || len(p.Items) != 8 || calls != before+8 {
		t.Fatal(e, calls)
	}
	q, e := x.s.Roster(ctx, x.session, 9, 8)
	if e != nil || len(q.Items) != 2 || q.NextSlot != 0 || q.Status.Slots != p.Status.Slots {
		t.Fatal(e)
	}
	x.reopen(t)
	status, e := x.s.Status(ctx, x.session)
	if e != nil || status.RosterRevision != 10 {
		t.Fatal(status, e)
	}
}

func TestStableSourceGrowthDriftAndForeignGym(t *testing.T) {
	x := newCoreFixture(t, "zero256", 3)
	mustSync(t, x)
	ctx := context.Background()
	i, e := x.s.PrepareSlot(ctx, x.session, SlotRequest{Slot: 1, ProspectID: x.records[0].Verified.ProspectID})
	if e != nil {
		t.Fatal(e)
	}
	expandCoreFixture(t, x, 4)
	replay, e := x.s.PrepareSlot(ctx, x.session, i.Request)
	if e != nil || replay != i {
		t.Fatal("growth changed acknowledged intent", e)
	}
	if _, e = x.s.ApplySlot(ctx, x.session, i.OperationID); !errors.Is(e, ErrSyncRequired) {
		t.Fatal(e)
	}
	mustSync(t, x)
	if _, e = x.s.ApplySlot(ctx, x.session, i.OperationID); e != nil {
		t.Fatal(e)
	}
	// A coherent historical accepted-time rewrite remains a valid source receipt,
	// but must differ from the exact original stable binding when accessed.
	if e = x.s.Close(); e != nil {
		t.Fatal(e)
	}
	if e = x.source.Close(); e != nil {
		t.Fatal(e)
	}
	x.records[0].Birth.AcceptedAtMS++
	sourceRows(t, x.sourcePath, x.pin, x.records)
	x.source, e = registry.OpenManifestAuthority(x.sourcePath, x.pin.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	x.s, e = Open(x.path, x.pin, x.source)
	if e != nil {
		t.Fatal(e)
	}
	x.s.now = func() time.Time { return time.UnixMilli(x.now) }
	if _, e = x.s.Inventory(ctx, x.session, 0, 1); !errors.Is(e, ErrCorrupt) {
		t.Fatal("accepted rewritten source", e)
	}
	other, _ := fixtureIdentityForTest("incrementing256")
	changed := x.pin
	changed.Identity = other
	// Public pin identity may be structurally supported, yet its concrete source
	// cannot assign the first gym's authenticated receipts to that other gym.
	path := x.path + "-foreign"
	foreign, e := create(path, changed, x.source, func() time.Time { return time.UnixMilli(x.now) }, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer foreign.Close()
	if _, e = foreign.Sync(ctx, fixtureSession(changed, 1, strings.Repeat("c", 64))); !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
}

func TestStableOperationCapacityReservation(t *testing.T) {
	x := newCoreFixture(t, "zero256", 0)
	ctx := context.Background()
	// Build the complete canonical history of 4095 abandoned no-op intentions.
	// This tests the actual raw audit and capacity without 8190 redundant fsyncs.
	b := new(leveldb.Batch)
	m := x.s.meta
	for n := 1; n < MaxOperations; n++ {
		id := fmt.Sprintf("%064x", n)
		i := Intent{Version: 1, Sequence: uint64(n), OperationID: id, Generation: 1, ViewID: x.session.ViewID, Request: SlotRequest{Slot: 1}, PreparedAtMS: x.now}
		o := Operation{Version: 1, Intent: i, State: "abandoned", AbandonedAtMS: x.now}
		if e := encodedPut(b, idKey('o', id), o, MaxOperationSize); e != nil {
			t.Fatal(e)
		}
		m.LastOperationID = id
	}
	m.OperationCount = MaxOperations - 1
	if e := x.s.commitLocked(b, m); e != nil {
		t.Fatal(e)
	}
	x.reopen(t)
	x.s.random = bytes.NewReader(bytes.Repeat([]byte{0xfe}, 32))
	i, e := x.s.PrepareSlot(ctx, x.session, SlotRequest{Slot: 1})
	if e != nil || i.Sequence != MaxOperations {
		t.Fatal(i, e)
	}
	if _, e = x.s.ApplySlot(ctx, x.session, i.OperationID); e != nil {
		t.Fatal(e)
	}
	if _, e = x.s.PrepareSlot(ctx, x.session, SlotRequest{Slot: 1}); !errors.Is(e, ErrCapacity) {
		t.Fatal(e)
	}
	if r, e := x.s.ApplySlot(ctx, x.session, i.OperationID); e != nil || !r.Replayed {
		t.Fatal(e)
	}
	if o, e := x.s.Abandon(ctx, x.session, i.OperationID); e != nil || o.State != "committed" {
		t.Fatal(e)
	}
	x.reopen(t)
}

func TestStableRawCorruption(t *testing.T) {
	cases := map[string]func(*Store){
		"missing-roster":     func(s *Store) { _ = s.db.Delete([]byte("roster"), nil) },
		"unknown-row":        func(s *Store) { _ = s.db.Put([]byte("unknown"), []byte("{}"), nil) },
		"oversized-metadata": func(s *Store) { _ = s.db.Put([]byte("meta"), bytes.Repeat([]byte{' '}, MaxMetadataSize+1), nil) },
		"index-value": func(s *Store) {
			_ = s.db.Put(idKey('i', strings.Repeat("a", 64)), bytes.Repeat([]byte{'x'}, 8192), nil)
		},
		"counter-disconnected": func(s *Store) { m := s.meta; m.OperationCount = 1; _ = s.commitLocked(new(leveldb.Batch), m) },
		"roster-without-history": func(s *Store) {
			r := s.roster
			r.Revision = 1
			raw, _ := json.Marshal(r)
			_ = s.db.Put([]byte("roster"), raw, nil)
		},
		"noncanonical": func(s *Store) {
			raw, _ := s.db.Get([]byte("meta"), nil)
			_ = s.db.Put([]byte("meta"), append([]byte(" "), raw...), nil)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			x := newCoreFixture(t, "zero256", 0)
			mutate(x.s)
			if e := x.s.Close(); e != nil {
				t.Fatal(e)
			}
			x.s = nil
			if s, e := Open(x.path, x.pin, x.source); !errors.Is(e, ErrCorrupt) {
				if s != nil {
					s.Close()
				}
				t.Fatal(e)
			}
		})
	}
}

func fixtureIdentityForTest(name string) (gymaccount.Identity, error) {
	return gymaccount.FixtureIdentity(name)
}

// This is a real source append through issuance/admission APIs. The node guard
// is a test-owned current-parent observation; no node canonicality is claimed.
func appendLiveCoreRecord(t *testing.T, x *coreFixture) {
	t.Helper()
	base := x.records[0]
	job, e := x.source.IssueManifestJob(base.Context, x.pin.Identity.GymPublicKey, func() error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	script, e := prospectjob.ManifestCommitmentScript(job)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := hex.DecodeString(base.Proof.Coinbase)
	var tx wire.MsgTx
	if e = tx.Deserialize(bytes.NewReader(raw)); e != nil {
		t.Fatal(e)
	}
	tx.TxOut[len(tx.TxOut)-1].PkScript = script
	var coinbase bytes.Buffer
	if e = tx.SerializeNoWitness(&coinbase); e != nil {
		t.Fatal(e)
	}
	raw, _ = hex.DecodeString(base.Proof.Header)
	var header wire.BlockHeader
	if e = header.Deserialize(bytes.NewReader(raw)); e != nil {
		t.Fatal(e)
	}
	header.MerkleRoot = tx.TxHash()
	var encoded bytes.Buffer
	if e = header.Serialize(&encoded); e != nil {
		t.Fatal(e)
	}
	proof := base.Proof
	proof.JobID = job.ID
	proof.Header = hex.EncodeToString(encoded.Bytes())
	proof.Coinbase = hex.EncodeToString(coinbase.Bytes())
	// Published zero256 fixture key, independently fixed in enrollment_v1.json.
	scalar, _ := hex.DecodeString("9d3035e7440dd3737fe58ffb69ca3576257d424b6e6172b3199a66a8897bddf6")
	key, _ := btcec.PrivKeyFromBytes(scalar)
	defer key.Zero()
	if hex.EncodeToString(key.PubKey().SerializeCompressed()) != x.pin.Identity.GymPublicKey {
		t.Fatal("wrong fixture")
	}
	auth, e := gymauth.SignAdmission(job.Context, proof, key)
	if e != nil {
		t.Fatal(e)
	}
	r, inserted, e := x.source.RecordManifestJobChecked(proof, auth, func(c prospect.Context, p prospect.Proof, v prospect.Verified) (registry.Eligibility, error) {
		return registry.Eligibility{RuleVersion: 2, Source: "current-parent", Hash: c.ParentHash, Height: c.ParentHeight}, nil
	})
	if e != nil || !inserted {
		t.Fatal("actual source append", e)
	}
	x.records = append(x.records, r)
	x.now = max(x.now, r.Birth.AcceptedAtMS)
}

func TestStableReadsRetainObservedSourceGrowth(t *testing.T) {
	for _, method := range []string{"inventory", "roster", "open"} {
		t.Run(method, func(t *testing.T) {
			x := newCoreFixture(t, "zero256", 3)
			mustSync(t, x)
			appendLiveCoreRecord(t, x)
			var status Status
			switch method {
			case "inventory":
				p, e := x.s.Inventory(context.Background(), x.session, 0, 1)
				if e != nil {
					t.Fatal(e)
				}
				status = p.Status
			case "roster":
				p, e := x.s.Roster(context.Background(), x.session, 1, 8)
				if e != nil {
					t.Fatal(e)
				}
				status = p.Status
			case "open":
				x.reopen(t)
				var e error
				status, e = x.s.Status(context.Background(), x.session)
				if e != nil {
					t.Fatal(e)
				}
			}
			if status.Checkpoint != 3 || status.ObservedSourceCount != 4 || status.CaughtUp || status.RosterRevision != 0 {
				t.Fatal(status)
			}
			x.reopen(t)
			if x.s.meta.ObservedSourceCount != 4 {
				t.Fatal("observation lost")
			}
			// A different complete source at the same immutable pin but with the old
			// three-row prefix must now refuse Open, even though checkpoint is three.
			prefix := x.sourcePath + "-rolled-back"
			sourceRows(t, prefix, x.pin, x.records[:3])
			old, e := registry.OpenManifestAuthority(prefix, x.pin.Source, false)
			if e != nil {
				t.Fatal(e)
			}
			defer old.Close()
			if e = x.s.Close(); e != nil {
				t.Fatal(e)
			}
			x.s = nil
			if s, e := Open(x.path, x.pin, old); !errors.Is(e, ErrCorrupt) {
				if s != nil {
					s.Close()
				}
				t.Fatal("source count regression", e)
			}
		})
	}
}
