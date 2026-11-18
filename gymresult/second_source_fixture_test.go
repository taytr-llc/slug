//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"testing"
)

// Adapted from the prior bounded Bout journal fixture. The newly assembled
// later source is a model only, with public fixture certificates and no claimed
// mining/admission. It is not an expected vector or an installed import path.
func modeledSeedFinality(t testing.TB, p gymseed.Pin, r gymseed.Record, at int64) gymseed.Record {
	t.Helper()
	// Only journal model evidence: source members were fully normal-proof verified,
	// but these synthetic headers are not passed to a Publisher or claimed mined.
	did, e := gymseed.DeclarationID(r.Declaration.Declaration)
	if e != nil {
		t.Fatal(e)
	}
	payload := append([]byte("SLUGSEED\x01"), rawID(r.EventKey)...)
	payload = append(payload, rawID(did)...)
	tx := wire.NewMsgTx(2)
	tx.AddTxOut(wire.NewTxOut(0, append([]byte{0x6a, 73}, payload...)))
	var raw bytes.Buffer
	tx.Serialize(&raw)
	digest := sha256.Sum256(raw.Bytes())
	h := wire.BlockHeader{MerkleRoot: tx.TxHash()}
	var hb bytes.Buffer
	h.Serialize(&hb)
	anchor := gymseed.Anchor{Height: 2, Hash: hash("synthetic anchor"), Header: hex.EncodeToString(hb.Bytes()), TxID: tx.TxHash().String(), TxIndex: 0, OutputIndex: 0, Payload: hex.EncodeToString(payload), RawTxDigest: hex.EncodeToString(digest[:]), RawTxSize: uint32(raw.Len()), TxCount: 1, MerkleBranch: []string{}}
	r.Anchor = &anchor
	r.Cursor.Height = 2
	r.Cursor.Hash = anchor.Hash
	r.Cursor.CompleteToHeight = 2
	r.Cursor.CompleteToHash = anchor.Hash
	parent, _ := chainhash.NewHashFromStr(anchor.Hash)
	h = wire.BlockHeader{PrevBlock: *parent}
	hb.Reset()
	h.Serialize(&hb)
	tip := gymseed.Tip{Height: 63, Hash: hash("synthetic tip")}
	target := gymseed.Target{Height: 3, Hash: hash("synthetic target"), Header: hex.EncodeToString(hb.Bytes()), ObservedAtMS: at, ObservedTip: tip}
	r.Target = &target
	th, _ := chainhash.NewHashFromStr(target.Hash)
	genesis, _ := chainhash.NewHashFromStr(p.Genesis)
	cp := chaincfg.SignedCheckpoint{Version: 1, Network: wire.BitcoinNet(p.Network), GenesisHash: *genesis, Height: 3, Hash: *th}
	message, e := cp.SigningBytes()
	if e != nil {
		t.Fatal(e)
	}
	seed := sha256.Sum256([]byte("SLUG candidate seed checkpoint fixture\x00v1\x00"))
	cp.Signature = ed25519.Sign(ed25519.NewKeyFromSeed(seed[:]), message)
	r.Checkpoint = &cp
	r.Finality = &gymseed.Finality{Checkpoint: cp, ObservedAtMS: at, ObservedTip: tip, TargetDescendants: 60}
	r.Observation = gymseed.Observation{ObservedAtMS: at, ChainObservedAtMS: at, NodeState: "online", Tip: &tip, CurrentFinality: true, TargetDescendants: 60}
	if _, e = gymseed.ValidateRecord(p, r); e != nil {
		t.Fatal("synthetic finalized control", e)
	}
	return r
}

func modeledSeedRecord(t *testing.T, p gymcard.Pin, template gymevent.SnapshotReceipt, date string) gymseed.Record {
	t.Helper()
	raw, _ := json.Marshal(template)
	var rec gymevent.SnapshotReceipt
	json.Unmarshal(raw, &rec)
	ep := p.SourcePin.SourcePin
	ev, e := gymevent.EventForDate(ep, date)
	if e != nil {
		t.Fatal(e)
	}
	rec.Input.Event = ev
	rec.CompletedAtMS = ev.CutoffMS + 100
	for i := range rec.Input.Participants {
		part := &rec.Input.Participants[i]
		entryEv, e := gymentry.EventForDate(ep.Participants[i], date)
		if e != nil {
			t.Fatal(e)
		}
		entryID, e := gymentry.EntryID(part.Entry.StoreID, entryEv.EventID, part.Owner.GymID)
		if e != nil {
			t.Fatal(e)
		}
		part.Entry.Event = entryEv
		part.Entry.EntryID = entryID
		part.Entry.LatestReceipt.Event = entryEv
		part.Entry.LatestReceipt.EntryID = entryID
		part.Roster.CutoffMS = ev.CutoffMS
		for j := range part.Members {
			m := &part.Members[j]
			manifest, e := fightermanifest.GenerateText(1, m.Assignment.WorkHash)
			if e != nil {
				t.Fatal(e)
			}
			m.ConditionAtCutoff, e = fightercondition.Project(1, manifest.DNA, m.Assignment.BornAtMS, ev.CutoffMS, 0)
			if e != nil {
				t.Fatal(e)
			}
			m.Eligibility = "eligible"
			if m.Slot == 0 {
				m.Eligibility = "not_rostered"
			}
			if m.ConditionAtCutoff.Phase == "retired" {
				m.Eligibility = "retired"
			}
		}
		rec.Captures[i].StableFence.CutoffMS = ev.CutoffMS
		rec.Captures[i].StableFence.ObservedAtMS = rec.CompletedAtMS
		rec.Captures[i].EntryFence.CutoffMS = ev.CutoffMS
		rec.Captures[i].EntryFence.ObservedAtMS = rec.CompletedAtMS
		rec.Captures[i].CapturedAtMS = rec.CompletedAtMS
	}
	rec.SnapshotDigest, e = gymevent.SnapshotDigest(ep, rec.Input)
	if e != nil {
		t.Fatal(e)
	}
	rd, e := gymevent.ReceiptDigest(ep, rec)
	if e != nil {
		t.Fatal(e)
	}
	key, e := gymseed.EventKey(p.SourcePin, ev)
	if e != nil {
		t.Fatal(e)
	}
	sp := p.SourcePin
	decl := gymseed.Declaration{Version: 1, EventKey: key, StoreID: p.SourceStoreID, SourceStoreID: sp.SourceStoreID, Event: ev, SnapshotDigest: rec.SnapshotDigest, ReceiptDigest: rd, PublisherChainID: sp.PublisherChainID, Network: sp.Network, Genesis: sp.Genesis, FloorHeight: 0, FloorHash: sp.Genesis, RulesID: sp.RulesID, PublisherPublicKey: sp.PublisherPublicKey, CheckpointPublicKey: sp.CheckpointPublicKey}
	b, _ := json.Marshal(decl)
	keySeed := sha256.Sum256([]byte("SLUG candidate seed declaration fixture\x00v1\x00"))
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(keySeed[:]), append([]byte("SLUG candidate seed declaration\x00v1\x00"), lp(b)...))
	r := gymseed.Record{Version: 1, StoreID: p.SourceStoreID, EventKey: key, SourceReceipt: rec, Declaration: gymseed.SignedDeclaration{Version: 1, Declaration: decl, Signature: hex.EncodeToString(sig)}, PreparedAtMS: rec.CompletedAtMS, Cursor: gymseed.Cursor{Height: -1, CompleteToHeight: -1, ScanDigest: hash("modeled scan")}, ScanRestarts: []gymseed.ScanRestart{}, Conflicts: []gymseed.Conflict{}, Observation: gymseed.Observation{NodeState: "unobserved"}}
	return modeledSeedFinality(t, sp, r, rec.CompletedAtMS+1)
}

func addModeledSecondBooking(t *testing.T, f *resultFixture) sourceCall {
	t.Helper()
	ctx := context.Background()
	var old gymseed.Record
	fixtureJSON(t, "seed_record", &old)
	p := f.pin.SourcePin.SourcePin
	r := modeledSeedRecord(t, p, old.SourceReceipt, "2026-12-05")
	f.now.Store(r.Finality.ObservedAtMS + 1000)
	f.bout.Close()
	f.card.Close()
	f.seed.Close()
	raw, e := gymseed.MarshalRecord(p.SourcePin, r)
	if e != nil {
		t.Fatal(e)
	}
	injectRows(t, filepath.Join(f.dir, "seed"), map[string][]byte{string(key('r', r.EventKey)): raw}, map[string]any{"event_count": 2, "clock_ms": f.now.Load()})
	f.seed, e = gymseed.OpenForTestClock(ctx, filepath.Join(f.dir, "seed"), p.SourcePin, f.event, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	source := gymseed.CardSource{Version: 1, StoreID: r.StoreID, EventKey: r.EventKey, SourceReceipt: r.SourceReceipt, Declaration: r.Declaration, Anchor: *r.Anchor, Target: *r.Target, Finality: *r.Finality}
	plan, e := gymcard.DerivePlan(p, source)
	if e != nil || len(plan.Bouts) != 1 {
		t.Fatal("second modeled booking", e)
	}
	sd, e := gymseed.CardSourceDigest(p.SourcePin, source)
	if e != nil {
		t.Fatal(e)
	}
	cid, _ := gymcard.ID(p)
	card := gymcard.Record{Version: 1, StoreID: cid, CardID: plan.CardID, Source: source, SourceDigest: sd, Plan: plan, CreatedAtMS: r.Finality.ObservedAtMS + 100}
	raw, e = gymcard.MarshalRecord(p, card)
	if e != nil {
		t.Fatal(e)
	}
	injectRows(t, filepath.Join(f.dir, "card"), map[string][]byte{string(key('r', card.CardID)): raw}, map[string]any{"event_count": 2, "clock_ms": f.now.Load()})
	f.card, e = gymcard.OpenForTestClock(ctx, filepath.Join(f.dir, "card"), p, f.seed, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.bout, e = gymbout.OpenForTestClock(ctx, filepath.Join(f.dir, "bout"), f.pin.SourcePin, f.card, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	b, e := f.bout.Run(ctx, f.card, f.seed, f.event, f.entry, f.session, "2026-12-05", plan.Bouts[0].BoutID)
	if e != nil {
		t.Fatal(e)
	}
	c := f.call()
	c.date = "2026-12-05"
	c.id = b.Record.BoutID
	return c
}
