//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// This is a relational pipeline test, not a new expected hash vector. Both
// admissions are newly issued by the same real normal-PoW ManifestStore and
// signed with published fixture keys. The current-parent eligibility guard is
// modeled: this test claims neither live chain finality nor production ownership.
func TestEventSharedSourceScopesNoonSnapshotAcrossRestart(t *testing.T) {
	requireNormal(t)
	ctx := context.Background()
	x := &eventFixture{p: fixturePin(t)}
	dir := realTemp(t)
	x.path = filepath.Join(dir, "events")
	sourcePath := filepath.Join(dir, "issuer")
	clock := func() time.Time { return time.UnixMilli(x.now) }
	base := sharedEventProofSkeleton(t)
	p := x.p.Participants[0].StablePin.Source
	source, e := registry.CreateManifestAuthority(sourcePath, p.Network, p.Genesis, p.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if x.s != nil {
			if e := x.s.Close(); e != nil {
				t.Error(e)
			}
		}
		for i := range x.entry {
			if x.entry[i] != nil {
				if e := x.entry[i].Close(); e != nil {
					t.Error(e)
				}
			}
			if x.stable[i] != nil {
				if e := x.stable[i].Close(); e != nil {
					t.Error(e)
				}
			}
		}
		if source != nil {
			if e := source.Close(); e != nil {
				t.Error(e)
			}
		}
	})
	info, e := source.Info()
	if e != nil {
		t.Fatal(e)
	}
	var records [2]registry.ManifestRecord
	var assignments [2]gymstable.Assignment
	for i := range x.p.Participants {
		ep := &x.p.Participants[i]
		ep.StablePin.Version = gymstable.SharedPinVersion2
		ep.StablePin.Source = info.Pin
		ep.StableStoreID, e = gymstable.ID(ep.StablePin)
		if e != nil {
			t.Fatal(e)
		}
		x.source[i] = source
		records[i] = issueSharedEventRecord(t, source, ep.StablePin, base)
		x.now = max(x.now, records[i].Birth.AcceptedAtMS+1000)
	}
	if _, e = ValidatePin(x.p); e != nil {
		t.Fatal("shared participant pin", e)
	}
	// Choose a future Saturday so this test does not depend on its run date.
	day := time.UnixMilli(x.now).UTC().AddDate(0, 0, 14)
	for day.Weekday() != time.Saturday {
		day = day.AddDate(0, 0, 1)
	}
	saturday := day.Format("2006-01-02")
	for i, ep := range x.p.Participants {
		sp, index := ep.StablePin, i
		x.session[i] = gymstable.Session{Generation: 1, ViewID: strings.Repeat(string(rune('a'+i)), 64), Lease: func(ctx context.Context, _ gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
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
			if e := fn(gymaccount.Account{ID: sp.AccountID, Generation: 1, Enrollment: &identity}, check); e != nil {
				return e
			}
			return check()
		}}
		x.stable[i], e = gymstable.CreateSharedForTestClock(filepath.Join(dir, "stable"+string(rune('0'+i))), sp, source, clock)
		if e != nil {
			t.Fatal(e)
		}
		synced, e := x.stable[i].Sync(ctx, x.session[i])
		if e != nil || len(synced.Items) != 1 || synced.Status.Checkpoint != 2 || !synced.Status.CaughtUp {
			t.Fatalf("gym %d scoped sync: %+v %v", i, synced, e)
		}
		item := synced.Items[0]
		if item.Assignment.Sequence != uint64(i+1) || !equal(item.Receipt(), records[i]) {
			t.Fatal("original shared source sequence or evidence changed")
		}
		assignments[i], x.ids[i] = item.Assignment, item.Assignment.ProspectID
		status, e := x.stable[i].SharedStatus(ctx, x.session[i])
		if e != nil || status.AssignmentCount != 1 || status.Status.Checkpoint != 2 || status.Status.ObservedSourceCount != 2 {
			t.Fatal("global cursor is not a local assignment count", status, e)
		}
		op, e := x.stable[i].PrepareSlot(ctx, x.session[i], gymstable.SlotRequest{Slot: 1, ProspectID: x.ids[i]})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = x.stable[i].ApplySlot(ctx, x.session[i], op.OperationID); e != nil {
			t.Fatal(e)
		}
		x.entry[i], e = gymentry.CreateForTestClock(ctx, filepath.Join(dir, "entry"+string(rune('0'+i))), ep, x.stable[i], clock)
		if e != nil {
			t.Fatal(e)
		}
		ev, e := gymentry.EventForDate(ep, saturday)
		if e != nil {
			t.Fatal(e)
		}
		req := gymentry.Request{Version: 1, EventID: ev.EventID, Saturday: saturday, Action: "replace", ExpectedRosterRevision: 1, ProspectIDs: []string{x.ids[i]}}
		in, e := x.entry[i].Prepare(ctx, x.session[i], req)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = x.entry[i].Apply(ctx, x.session[i], in.OperationID); e != nil {
			t.Fatal(e)
		}
	}
	for i := range x.stable {
		foreign := x.ids[1-i]
		if _, e = x.stable[i].PrepareSlot(ctx, x.session[i], gymstable.SlotRequest{ExpectedRevision: 1, Slot: 2, ProspectID: foreign}); !errors.Is(e, gymstable.ErrNotFound) {
			t.Fatal("foreign assignment entered local roster", e)
		}
		if _, e = x.stable[i].ReadEntrySelection(ctx, x.session[i], []string{foreign}); !errors.Is(e, gymstable.ErrSyncRequired) {
			t.Fatal("foreign assignment entered local event selection", e)
		}
		if _, e = x.stable[i].SharedStatus(ctx, x.session[1-i]); e == nil {
			t.Fatal("foreign account read scoped stable")
		}
	}
	x.s, e = CreateForTestClock(ctx, x.path, x.p, clock)
	if e != nil {
		t.Fatal(e)
	}
	ev, e := EventForDate(x.p, saturday)
	if e != nil {
		t.Fatal(e)
	}
	x.now = ev.CutoffMS + 1000
	var captures [2]ParticipantCapture
	for i := range x.entry {
		out, e := x.s.Capture(ctx, x.entry[i], x.session[i], saturday)
		if e != nil || out.Replayed || len(out.Capture.Snapshot.Members) != 1 {
			t.Fatal("shared noon capture", out, e)
		}
		captures[i] = out.Capture
		got := out.Capture.Snapshot
		sp := x.p.Participants[i].StablePin
		if got.Owner.AccountID != sp.AccountID || got.Owner.GymID != sp.Identity.GymID || got.Owner.AccountAuthorityID != sp.AccountAuthorityID || got.Members[0].Assignment != assignments[i] || got.Members[0].ConditionAtCutoff.AsOfMS != ev.CutoffMS {
			t.Fatal("noon capture changed scoped ownership or source evidence", got)
		}
		if i == 1 && (out.Status.Phase != "snapshotted" || out.Status.SnapshotDigest == "") {
			t.Fatal("shared event did not complete", out.Status)
		}
	}
	original, e := x.s.Read(ctx, x.entry[0], x.session[0], saturday)
	if e != nil || len(original.Input.Participants) != 2 {
		t.Fatal("complete shared snapshot", original, e)
	}
	if _, e = MarshalSnapshotReceipt(x.p, original); e != nil {
		t.Fatal("shared snapshot codec", e)
	}
	// A real post-noon edit must not rewrite either participant's frozen input.
	x.now++
	op, e := x.stable[0].PrepareSlot(ctx, x.session[0], gymstable.SlotRequest{ExpectedRevision: 1, Slot: 1})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.stable[0].ApplySlot(ctx, x.session[0], op.OperationID); e != nil {
		t.Fatal(e)
	}
	// Reopen every source and consumer, retaining original pins and sessions.
	if e = x.s.Close(); e != nil {
		t.Fatal(e)
	}
	x.s = nil
	for i := range x.entry {
		if e = x.entry[i].Close(); e != nil {
			t.Fatal(e)
		}
		x.entry[i] = nil
		if e = x.stable[i].Close(); e != nil {
			t.Fatal(e)
		}
		x.stable[i] = nil
	}
	if e = source.Close(); e != nil {
		t.Fatal(e)
	}
	source = nil
	source, e = registry.OpenManifestAuthority(sourcePath, info.Pin, false)
	if e != nil {
		t.Fatal(e)
	}
	for i, ep := range x.p.Participants {
		x.source[i] = source
		x.stable[i], e = gymstable.OpenSharedForTestClock(filepath.Join(dir, "stable"+string(rune('0'+i))), ep.StablePin, source, clock)
		if e != nil {
			t.Fatal(e)
		}
		x.entry[i], e = gymentry.OpenForTestClock(ctx, filepath.Join(dir, "entry"+string(rune('0'+i))), ep, x.stable[i], clock)
		if e != nil {
			t.Fatal(e)
		}
	}
	x.s, e = OpenForTestClock(ctx, x.path, x.p, clock)
	if e != nil {
		t.Fatal(e)
	}
	for i := range x.entry {
		again, e := x.s.Read(ctx, x.entry[i], x.session[i], saturday)
		if e != nil || !equal(again, original) {
			t.Fatal("restart changed shared noon snapshot", e)
		}
		replay, e := x.s.Capture(ctx, x.entry[i], x.session[i], saturday)
		if e != nil || !replay.Replayed || !equal(replay.Capture, captures[i]) {
			t.Fatal("restart changed participant capture", e)
		}
	}
}

func sharedEventProofSkeleton(t *testing.T) registry.ManifestRecord {
	t.Helper()
	raw, e := os.ReadFile("../gymstable/testdata/reference/stable_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "29f6a1302c2e04ade612fe2775c085c65b8bf33587c010060b727833182d2057" {
		t.Fatal("frozen public skeleton corpus changed")
	}
	var corpus struct {
		Cases []struct {
			Fixture string
			Record  registry.ManifestRecord
		}
	}
	if e = json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	for _, c := range corpus.Cases {
		if c.Fixture == "zero256" && c.Record.Sequence == 1 && len(c.Record.Proof.MerkleBranch) == 0 {
			return c.Record
		}
	}
	t.Fatal("coinbase-only public skeleton missing")
	return registry.ManifestRecord{}
}

func issueSharedEventRecord(t *testing.T, source *registry.ManifestStore, pin gymstable.Pin, base registry.ManifestRecord) registry.ManifestRecord {
	t.Helper()
	c := base.Context
	c.ProspectTarget = strings.Repeat("f", 64) // one normal hash, no probabilistic search
	job, e := source.IssueManifestJob(c, pin.Identity.GymPublicKey, func() error { return nil })
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
	proof.JobID, proof.GymID = job.ID, job.GymID
	proof.Header, proof.Coinbase = hex.EncodeToString(encoded.Bytes()), hex.EncodeToString(coinbase.Bytes())
	// Public test-only scalars already published in enrollment_v1.json.
	scalarHex := ""
	if pin.Identity == fixtureIdentity("zero256") {
		scalarHex = "9d3035e7440dd3737fe58ffb69ca3576257d424b6e6172b3199a66a8897bddf6"
	} else if pin.Identity == fixtureIdentity("incrementing256") {
		scalarHex = "948996ecaa9fa9707693267d6d4abefe0b5fb28b6ebae34f542e106244d254e1"
	} else {
		t.Fatal("unknown public fixture identity")
	}
	scalar, _ := hex.DecodeString(scalarHex)
	key, _ := btcec.PrivKeyFromBytes(scalar)
	defer key.Zero()
	if hex.EncodeToString(key.PubKey().SerializeCompressed()) != pin.Identity.GymPublicKey {
		t.Fatal("published fixture key mismatch")
	}
	auth, e := gymauth.SignAdmission(job.Context, proof, key)
	if e != nil {
		t.Fatal(e)
	}
	r, inserted, e := source.RecordManifestJobChecked(proof, auth, func(c prospect.Context, _ prospect.Proof, _ prospect.Verified) (registry.Eligibility, error) {
		return registry.Eligibility{RuleVersion: 2, Source: "current-parent", Hash: c.ParentHash, Height: c.ParentHeight}, nil
	})
	if e != nil || !inserted {
		t.Fatalf("shared issuer admission: inserted=%v err=%v", inserted, e)
	}
	return r
}
