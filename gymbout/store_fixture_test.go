//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type boutFixture struct {
	dir        string
	now        atomic.Int64
	revoke     atomic.Bool
	leases     atomic.Int64
	inLease    atomic.Int64
	p          Pin
	seedRecord gymseed.Record
	cardRecord gymcard.Record
	source     *registry.ManifestStore
	stable     *gymstable.Store
	entry      *gymentry.Store
	event      *gymevent.Store
	seed       *gymseed.Store
	card       *gymcard.Store
	s          *Store
	session    gymstable.Session
}

func realTemp(t testing.TB) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func (f *boutFixture) clock() time.Time { return time.UnixMilli(f.now.Load()) }

// This fixture intentionally assembles modeled histories and published fixture
// pins. Empty concrete registry/stable/entry stores authorize metadata only; it
// never claims genuine admission, mining, or a production ownership authority.
func newBoutFixture(t *testing.T) *boutFixture {
	t.Helper()
	requireNormal(t)
	ctx := context.Background()
	dir := os.Getenv("SLUG_BOUT_CRASH_ROOT")
	if dir == "" {
		dir = realTemp(t)
	}
	f := &boutFixture{dir: dir}
	var cp struct{ Value gymcard.Pin }
	raw, e := os.ReadFile("testdata/reference/inputs/card46/pin.json")
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(raw, &cp)
	f.p, e = CandidatePin(cp.Value)
	if e != nil {
		t.Fatal(e)
	}
	raw, e = os.ReadFile("testdata/reference/inputs/card46/modeled-five-class-seed.json")
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(raw, &f.seedRecord)
	f.now.Store(f.seedRecord.SourceReceipt.Input.Event.CutoffMS - 1000)
	ep := cp.Value.SourcePin.SourcePin
	part := ep.Participants[0]
	sp := part.StablePin
	sourceRows(t, filepath.Join(f.dir, "source"), sp, nil)
	f.source, e = registry.OpenManifestAuthority(filepath.Join(f.dir, "source"), sp.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	f.stable, e = gymstable.CreateForTestClock(filepath.Join(f.dir, "stable"), sp, f.source, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.entry, e = gymentry.CreateForTestClock(ctx, filepath.Join(f.dir, "entry"), part, f.stable, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.event, e = gymevent.CreateForTestClock(ctx, filepath.Join(f.dir, "event"), ep, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.seed, e = gymseed.CreateForTestClock(ctx, filepath.Join(f.dir, "seed"), cp.Value.SourcePin, f.event, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.session = gymstable.Session{Generation: 1, ViewID: strings.Repeat("a", 64), Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		f.leases.Add(1)
		if cap != gymaccount.View {
			return gymaccount.ErrForbidden
		}
		check := func() error {
			if f.revoke.Load() {
				return gymaccount.ErrUnauthorized
			}
			return ctx.Err()
		}
		if e := check(); e != nil {
			return e
		}
		f.inLease.Add(1)
		defer f.inLease.Add(-1)
		identity := sp.Identity
		e := fn(gymaccount.Account{ID: sp.AccountID, Generation: 1, Enrollment: &identity}, check)
		if e != nil {
			return e
		}
		return check()
	}}
	// The old model repeats cross-gym IDs; consistently rename only the second
	// modeled gym, then regenerate every affected canonical receipt/commitment.
	template := f.seedRecord.SourceReceipt
	second := &template.Input.Participants[1]
	rename := map[string]string{}
	for i := range second.Members {
		oldID := second.Members[i].Assignment.ProspectID
		newID := hash("bout modeled distinct prospect", rawID(second.Owner.GymID), rawID(oldID))
		rename[oldID] = newID
		second.Members[i].Assignment.ProspectID = newID
	}
	for i := range second.Entry.LatestReceipt.Members {
		a := &second.Entry.LatestReceipt.Members[i].Assignment
		a.ProspectID = rename[a.ProspectID]
	}
	for i, id := range second.Roster.Slots {
		if id != "" {
			second.Roster.Slots[i] = rename[id]
		}
	}
	sort.Slice(second.Members, func(i, j int) bool {
		return second.Members[i].Assignment.ProspectID < second.Members[j].Assignment.ProspectID
	})
	sort.Slice(second.Entry.LatestReceipt.Members, func(i, j int) bool {
		return second.Entry.LatestReceipt.Members[i].Assignment.ProspectID < second.Entry.LatestReceipt.Members[j].Assignment.ProspectID
	})
	f.seedRecord = capacitySeedRecord(t, f.p.SourcePin, template, f.date())
	f.now.Store(f.seedRecord.Finality.ObservedAtMS + 1000)
	f.replaceSeed(t, f.seedRecord)
	f.card, e = gymcard.CreateForTestClock(ctx, filepath.Join(f.dir, "card"), cp.Value, f.seed, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	built, e := f.card.Build(ctx, f.seed, f.event, f.entry, f.session, f.date())
	if e != nil {
		t.Fatal("modeled card build", e)
	}
	f.cardRecord = built.Record
	if len(built.Record.Plan.Bouts) != 5 {
		t.Fatalf("fixture requires five bouts, got%d", len(built.Record.Plan.Bouts))
	}
	f.s, e = create(ctx, filepath.Join(f.dir, "bout"), f.p, f.card, f.clock, boutCrashHook())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		for _, s := range []interface{ Close() error }{f.s, f.card, f.seed, f.event, f.entry, f.stable, f.source} {
			if s != nil {
				s.Close()
			}
		}
	})
	return f
}
func (f *boutFixture) date() string { return f.seedRecord.SourceReceipt.Input.Event.Saturday }
func (f *boutFixture) id() string   { return f.cardRecord.Plan.Bouts[0].BoutID }
func (f *boutFixture) replaceSeed(t testing.TB, r gymseed.Record) {
	t.Helper()
	f.seed.Close()
	p := filepath.Join(f.dir, "seed")
	db, e := leveldb.OpenFile(filepath.Join(p, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := gymseed.MarshalRecord(f.p.SourcePin.SourcePin, r)
	if e != nil {
		t.Fatal("modeled seed validation", e)
	}
	b := new(leveldb.Batch)
	b.Put(append([]byte{'r'}, rawID(r.EventKey)...), raw)
	meta, e := db.Get([]byte("meta"), nil)
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]json.RawMessage
	json.Unmarshal(meta, &m)
	for k, v := range map[string]any{"clock_ms": f.now.Load(), "event_count": 1} {
		n, _ := json.Marshal(v)
		meta = bytes.Replace(meta, append([]byte(`"`+k+`":`), m[k]...), append([]byte(`"`+k+`":`), n...), 1)
	}
	b.Put([]byte("meta"), meta)
	if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	db.Close()
	f.seed, e = gymseed.OpenForTestClock(context.Background(), p, f.p.SourcePin.SourcePin, f.event, f.clock)
	if e != nil {
		t.Fatal("model reopen", e)
	}
}
func (f *boutFixture) run(ctx context.Context) (RunResult, error) {
	return f.s.Run(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date(), f.id())
}
func (f *boutFixture) status(ctx context.Context) (Status, error) {
	return f.s.Status(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date(), f.id())
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

func capacitySeedRecord(t *testing.T, p gymcard.Pin, template gymevent.SnapshotReceipt, date string) gymseed.Record {
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
	return syntheticSeedFinality(t, sp, r, rec.CompletedAtMS+1)
}
