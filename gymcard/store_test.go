//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type cardFixture struct {
	x        *eventFixture
	seed     *gymseed.Store
	seedPath string
	p        Pin
	path     string
	s        *Store
}

func newCardFixture(t *testing.T) *cardFixture {
	t.Helper()
	x := newEventFixture(t, true)
	ctx := context.Background()
	sp, e := gymseed.CandidatePin(x.p)
	if e != nil {
		t.Fatal(e)
	}
	f := &cardFixture{x: x, seedPath: filepath.Join(filepath.Dir(x.path), "seeds"), path: filepath.Join(filepath.Dir(x.path), "cards")}
	f.seed, e = gymseed.CreateForTestClock(ctx, f.seedPath, sp, x.s, func() time.Time { return time.UnixMilli(x.now) })
	if e != nil {
		t.Fatal(e)
	}
	f.p, e = CandidatePin(sp)
	if e != nil {
		t.Fatal(e)
	}
	x.now = x.cutoff(t) + 1000
	for i := range x.entry {
		if _, e = x.s.Capture(ctx, x.entry[i], x.session[i], "2026-11-21"); e != nil {
			t.Fatal(e)
		}
	}
	r, e := f.seed.Prepare(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	x.now++
	r = syntheticSeedFinality(t, sp, r, x.now)
	f.replaceSeedRecord(t, r)
	f.s, e = create(ctx, f.path, f.p, f.seed, func() time.Time { return time.UnixMilli(x.now) }, cardCrashHook(t))
	if e != nil {
		t.Fatal(e)
	}
	x.viewOnly = true
	t.Cleanup(func() {
		if f.s != nil {
			f.s.Close()
		}
		if f.seed != nil {
			f.seed.Close()
		}
	})
	return f
}
func syntheticSeedFinality(t testing.TB, p gymseed.Pin, r gymseed.Record, at int64) gymseed.Record {
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
func (f *cardFixture) replaceSeedRecord(t *testing.T, r gymseed.Record) {
	t.Helper()
	f.seed.Close()
	db, e := leveldb.OpenFile(filepath.Join(f.seedPath, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := db.Get([]byte("meta"), nil)
	if e != nil {
		t.Fatal(e)
	}
	var meta map[string]json.RawMessage
	json.Unmarshal(raw, &meta)
	meta["clock_ms"], _ = json.Marshal(f.x.now)
	raw, _ = json.Marshal(meta)
	// Preserve canonical field order by replacing only the numeric clock literal
	// in the original metadata bytes. No arbitrary new schema fields are added.
	original, _ := db.Get([]byte("meta"), nil)
	var originalMeta map[string]json.RawMessage
	json.Unmarshal(original, &originalMeta)
	original = bytes.Replace(original, append([]byte(`"clock_ms":`), originalMeta["clock_ms"]...), append([]byte(`"clock_ms":`), meta["clock_ms"]...), 1)
	b := new(leveldb.Batch)
	b.Put([]byte("meta"), original)
	record, e := gymseed.MarshalRecord(f.p.SourcePin, r)
	if e != nil {
		t.Fatal(e)
	}
	b.Put(append([]byte{'r'}, rawID(r.EventKey)...), record)
	if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	f.seed, e = gymseed.OpenForTestClock(context.Background(), f.seedPath, f.p.SourcePin, f.x.s, func() time.Time { return time.UnixMilli(f.x.now) })
	if e != nil {
		t.Fatal(e)
	}
}
func (f *cardFixture) build(t *testing.T) (BuildResult, error) {
	t.Helper()
	return f.s.Build(context.Background(), f.seed, f.x.s, f.x.entry[0], f.x.session[0], "2026-11-21")
}
func TestCardConcreteBuildReplayConflictAndReopen(t *testing.T) {
	f := newCardFixture(t)
	x := f.x
	ctx := context.Background()
	built, e := f.build(t)
	if e != nil {
		t.Fatal(e)
	}
	if built.Replayed || !built.Status.Usable || built.Record.Plan.Counts.TotalCount != 2 {
		t.Fatal("build", built)
	}
	if _, e = ValidateBuildResult(f.p, built); e != nil {
		t.Fatal("published DTO", e)
	}
	x.now++
	again, e := f.s.Build(ctx, f.seed, x.s, x.entry[1], x.session[1], "2026-11-21")
	if e != nil || !again.Replayed || !equal(built.Record, again.Record) || again.Status.ObservedAtMS != x.now {
		t.Fatal("exact replay", e)
	}
	info, _ := f.s.Info(ctx)
	if info.EventCount != 1 {
		t.Fatal(info)
	}
	info.Pin.SourcePin.SourcePin.Participants = nil
	if _, e = f.s.Info(ctx); e != nil {
		t.Fatal("owned pin", e)
	}
	f.s.Close()
	f.s, e = open(ctx, f.path, f.p, f.seed, func() time.Time { return time.UnixMilli(x.now) }, false)
	if e != nil {
		t.Fatal(e)
	}
	read, e := f.s.Read(ctx, f.seed, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil || !equal(read.Record, built.Record) {
		t.Fatal("reopen", e)
	}
	r, e := f.seed.Record(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	x.now++
	r.Conflicts = append(r.Conflicts, gymseed.Conflict{Kind: "chain", ObservedAtMS: x.now, ObservedTip: &gymseed.Tip{Height: 3, Hash: hash("reorg")}, ExpectedHeight: 3, ExpectedHash: r.Target.Hash, ObservedHash: hash("changed")})
	r.Observation.CurrentFinality = false
	f.replaceSeedRecord(t, r)
	replay, e := f.build(t)
	if e != nil || !replay.Replayed || replay.Status.Usable || !equal(replay.Record, built.Record) {
		t.Fatal("conflict changed card", e)
	}
	// A separate fresh card store refuses the same now-conflicted source.
	fresh, e := create(ctx, filepath.Join(filepath.Dir(f.path), "freshcards"), f.p, f.seed, func() time.Time { return time.UnixMilli(x.now) }, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Close()
	if _, e = fresh.Build(ctx, f.seed, x.s, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrSourceConflict) {
		t.Fatal("conflict new build", e)
	}
	x.revoke[0] = true
	if _, e = f.s.Read(ctx, f.seed, x.s, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("revoked", e)
	}
	x.revoke[0] = false
	x.now--
	if _, e = f.s.Status(ctx, f.seed, x.s, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrClock) {
		t.Fatal("clock rollback", e)
	}
}
func TestCardLostSyncAcknowledgementAndRevokedPreparation(t *testing.T) {
	f := newCardFixture(t)
	x := f.x
	ctx := context.Background()
	committed := false
	f.s.checkpoint = func(stage string) {
		if stage == "build-prepared" {
			x.revoke[0] = true
		}
		if stage == "card-synced" {
			committed = true
		}
	}
	if _, e := f.build(t); e == nil || committed {
		t.Fatal("revocation before final write", e)
	}
	x.revoke[0] = false
	original := f.s.writeBatch
	f.s.checkpoint = func(stage string) {
		if stage == "before-card-sync" {
			f.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				if e := original(b, o); e != nil {
					return e
				}
				return errors.New("lost ACK")
			}
		}
	}
	if _, e := f.build(t); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal(e)
	}
	if _, e := f.s.Info(ctx); !errors.Is(e, ErrPoisoned) {
		t.Fatal("uncertain store reusable", e)
	}
	f.s.Close()
	var e error
	f.s, e = open(ctx, f.path, f.p, f.seed, func() time.Time { return time.UnixMilli(x.now) }, false)
	if e != nil {
		t.Fatal(e)
	}
	out, e := f.build(t)
	if e != nil || !out.Replayed {
		t.Fatal("lost ACK replay", e)
	}
	// Revocation after durable observation cannot expose a stale result.
	x.now++
	f.s.checkpoint = func(stage string) {
		if stage == "read-observed" {
			x.revoke[0] = true
		}
	}
	if _, e = f.s.Read(ctx, f.seed, x.s, x.entry[0], x.session[0], "2026-11-21"); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("post-observation revocation", e)
	}
}
func TestCardMissingReadyAndSourceCreationIdentity(t *testing.T) {
	f := newCardFixture(t)
	ctx := context.Background()
	f.s.Close()
	if e := os.Rename(filepath.Join(f.path, "db"), filepath.Join(filepath.Dir(f.path), "saved-db")); e != nil {
		t.Fatal(e)
	}
	if _, e := open(ctx, f.path, f.p, f.seed, time.Now, false); e == nil {
		t.Fatal("missing ready recreated")
	}
	if _, e := os.Stat(filepath.Join(f.path, "db")); !os.IsNotExist(e) {
		t.Fatal("recreated db")
	}
}
