//go:build slugtestclock && !slugtestpow

package pilot

import (
	"bytes"
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
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// These 256 histories and finality headers are explicitly modeled disk fixtures.
// They are not admitted ten-fighter records or a mined chain. The concrete stores
// audit them normally; the helper borrows its original real fixture entry/session.
// Construction mirrors the core capacity control and creates no production API.
func cardCapacityRawID(s string) []byte { b, _ := hex.DecodeString(s); return b }
func cardCapacityLP(b []byte) []byte {
	return append(binary.LittleEndian.AppendUint32(nil, uint32(len(b))), b...)
}
func cardCapacityHash(domain string, parts ...[]byte) string {
	b := []byte(domain)
	for _, p := range parts {
		b = append(b, p...)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func cardCapacitySeedRecord(t *testing.T, p gymcard.Pin, template gymevent.SnapshotReceipt, date string) gymseed.Record {
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
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(keySeed[:]), append([]byte("SLUG candidate seed declaration\x00v1\x00"), cardCapacityLP(b)...))
	r := gymseed.Record{Version: 1, StoreID: p.SourceStoreID, EventKey: key, SourceReceipt: rec, Declaration: gymseed.SignedDeclaration{Version: 1, Declaration: decl, Signature: hex.EncodeToString(sig)}, PreparedAtMS: rec.CompletedAtMS, Cursor: gymseed.Cursor{Height: -1, CompleteToHeight: -1, ScanDigest: cardCapacityHash("modeled scan")}, ScanRestarts: []gymseed.ScanRestart{}, Conflicts: []gymseed.Conflict{}, Observation: gymseed.Observation{NodeState: "unobserved"}}
	return cardCapacityFinality(t, sp, r, rec.CompletedAtMS+1)
}
func cardCapacityFinality(t testing.TB, p gymseed.Pin, r gymseed.Record, at int64) gymseed.Record {
	t.Helper()
	// Only journal model evidence: source members were fully normal-proof verified,
	// but these synthetic headers are not passed to a Publisher or claimed mined.
	did, e := gymseed.DeclarationID(r.Declaration.Declaration)
	if e != nil {
		t.Fatal(e)
	}
	payload := append([]byte("SLUGSEED\x01"), cardCapacityRawID(r.EventKey)...)
	payload = append(payload, cardCapacityRawID(did)...)
	tx := wire.NewMsgTx(2)
	tx.AddTxOut(wire.NewTxOut(0, append([]byte{0x6a, 73}, payload...)))
	var raw bytes.Buffer
	tx.Serialize(&raw)
	digest := sha256.Sum256(raw.Bytes())
	h := wire.BlockHeader{MerkleRoot: tx.TxHash()}
	var hb bytes.Buffer
	h.Serialize(&hb)
	anchor := gymseed.Anchor{Height: 2, Hash: cardCapacityHash("synthetic anchor"), Header: hex.EncodeToString(hb.Bytes()), TxID: tx.TxHash().String(), TxIndex: 0, OutputIndex: 0, Payload: hex.EncodeToString(payload), RawTxDigest: hex.EncodeToString(digest[:]), RawTxSize: uint32(raw.Len()), TxCount: 1, MerkleBranch: []string{}}
	r.Anchor = &anchor
	r.Cursor.Height = 2
	r.Cursor.Hash = anchor.Hash
	r.Cursor.CompleteToHeight = 2
	r.Cursor.CompleteToHash = anchor.Hash
	parent, _ := chainhash.NewHashFromStr(anchor.Hash)
	h = wire.BlockHeader{PrevBlock: *parent}
	hb.Reset()
	h.Serialize(&hb)
	tip := gymseed.Tip{Height: 63, Hash: cardCapacityHash("synthetic tip")}
	target := gymseed.Target{Height: 3, Hash: cardCapacityHash("synthetic target"), Header: hex.EncodeToString(hb.Bytes()), ObservedAtMS: at, ObservedTip: tip}
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

func cardCapacityFixture(t *testing.T) (*StableController, []string) {
	t.Helper()
	ctx := t.Context()
	s, _ := entryLaneFixture(t)
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UnixMilli()
	clock := func() time.Time { return time.UnixMilli(now) }
	ep := eventFixturePin(t)
	events, e := gymevent.CreateForTestClock(ctx, filepath.Join(root, "events"), ep, clock)
	if e != nil {
		t.Fatal(e)
	}
	attachEventFixture(t, s, events)
	sp, e := gymseed.CandidatePin(ep)
	if e != nil {
		t.Fatal(e)
	}
	seedPath, cardPath := filepath.Join(root, "seeds"), filepath.Join(root, "cards")
	seeds, e := gymseed.CreateForTestClock(ctx, seedPath, sp, events, clock)
	if e != nil {
		t.Fatal(e)
	}
	si, e := seeds.Info(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p, e := gymcard.CandidatePin(sp)
	if e != nil {
		t.Fatal(e)
	}
	cards, e := gymcard.CreateForTestClock(ctx, cardPath, p, seeds, clock)
	if e != nil {
		t.Fatal(e)
	}
	ci, e := cards.Info(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = cards.Close(); e != nil {
		t.Fatal(e)
	}
	if e = seeds.Close(); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../../gymevent/testdata/reference/event_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a" {
		t.Fatal("frozen old source fixture changed")
	}
	var old struct {
		Records []struct{ Value gymevent.SnapshotReceipt }
	}
	if json.Unmarshal(raw, &old) != nil {
		t.Fatal("template")
	}
	template := old.Records[10].Value
	second := &template.Input.Participants[1]
	rename := map[string]string{}
	for i := range second.Members {
		oldID := second.Members[i].Assignment.ProspectID
		id := cardCapacityHash("capacity-only distinct prospect", cardCapacityRawID(second.Owner.GymID), cardCapacityRawID(oldID))
		rename[oldID] = id
		second.Members[i].Assignment.ProspectID = id
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
	seedBatch, cardBatch := new(leveldb.Batch), new(leveldb.Batch)
	ids := []string{}
	date, _ := time.Parse("2006-01-02", "2026-11-21")
	for i := 0; i < 256; i++ {
		r := cardCapacitySeedRecord(t, p, template, date.AddDate(0, 0, 7*i).Format("2006-01-02"))
		source := gymseed.CardSource{Version: 1, StoreID: r.StoreID, EventKey: r.EventKey, SourceReceipt: r.SourceReceipt, Declaration: r.Declaration, Anchor: *r.Anchor, Target: *r.Target, Finality: *r.Finality}
		plan, e := gymcard.DerivePlan(p, source)
		if e != nil {
			t.Fatal(e)
		}
		sd, e := gymseed.CardSourceDigest(sp, source)
		if e != nil {
			t.Fatal(e)
		}
		record := gymcard.Record{Version: 1, StoreID: ci.StoreID, CardID: plan.CardID, Source: source, SourceDigest: sd, Plan: plan, CreatedAtMS: max(ci.CreatedAtMS, r.Finality.ObservedAtMS)}
		b, e := gymseed.MarshalRecord(sp, r)
		if e != nil {
			t.Fatal(e)
		}
		seedBatch.Put(append([]byte{'r'}, cardCapacityRawID(r.EventKey)...), b)
		b, e = gymcard.MarshalRecord(p, record)
		if e != nil {
			t.Fatal(e)
		}
		cardBatch.Put(append([]byte{'r'}, cardCapacityRawID(record.CardID)...), b)
		ids = append(ids, record.CardID)
		now = record.CreatedAtMS
	}
	for _, entry := range []struct {
		path  string
		batch *leveldb.Batch
	}{{seedPath, seedBatch}, {cardPath, cardBatch}} {
		db, e := leveldb.OpenFile(filepath.Join(entry.path, "db"), nil)
		if e != nil {
			t.Fatal(e)
		}
		meta, e := db.Get([]byte("meta"), nil)
		if e != nil {
			t.Fatal(e)
		}
		var old map[string]json.RawMessage
		if json.Unmarshal(meta, &old) != nil {
			t.Fatal("meta")
		}
		for k, v := range map[string]any{"clock_ms": now, "event_count": 256} {
			n, _ := json.Marshal(v)
			needle := append([]byte(`"`+k+`":`), old[k]...)
			if bytes.Count(meta, needle) != 1 {
				t.Fatal("bounded meta patch")
			}
			meta = bytes.Replace(meta, needle, append([]byte(`"`+k+`":`), n...), 1)
		}
		entry.batch.Put([]byte("meta"), meta)
		if e = db.Write(entry.batch, &opt.WriteOptions{Sync: true}); e != nil {
			t.Fatal(e)
		}
		if e = db.Close(); e != nil {
			t.Fatal(e)
		}
	}
	seeds, e = gymseed.OpenForTestClock(ctx, seedPath, sp, events, clock)
	if e != nil {
		t.Fatal("seed full audit", e)
	}
	cards, e = gymcard.OpenForTestClock(ctx, cardPath, p, seeds, clock)
	if e != nil {
		t.Fatal("card full audit", e)
	}
	if e = s.entryStore.Close(); e != nil {
		t.Fatal(e)
	}
	s.entryStore, e = gymentry.OpenForTestClock(ctx, filepath.Join(s.config.DataDir, "entries"), s.entryPin, s.stableStore, clock)
	if e != nil {
		t.Fatal(e)
	}
	s.seedEnabled, s.seedAttached, s.cardEnabled, s.cardAttached = true, true, true, true
	s.seedStore, s.cardStore = seeds, cards
	s.seedPin, s.cardPin = sp, p
	s.seedID, s.cardID = si.StoreID, ci.StoreID
	s.seedCreated, s.cardCreated, s.cardSourceCreated = si.CreatedAtMS, ci.CreatedAtMS, ci.SourceCreatedAtMS
	s.seedBytes, s.cardBytes = MaxSeedMetadataSize, MaxCardMetadataSize
	t.Cleanup(func() { _ = cards.Close(); _ = seeds.Close() })
	return s, ids
}
func TestCardHelperHistoryAtCapacity(t *testing.T) {
	s, ids := cardCapacityFixture(t)
	v := s.view.ViewID
	for _, start := range []int{0, 248, 256} {
		after := ""
		if start > 0 {
			after = ids[start-1]
		}
		begin := time.Now()
		page, e := s.CardHistory(t.Context(), v, after, 8)
		elapsed := time.Since(begin)
		if e != nil {
			t.Fatalf("helper capacity page%d %s: %v", start, elapsed, e)
		}
		want := min(8, 256-start)
		if len(page.Cards) != want || page.HasMore != (start+want < 256) {
			t.Fatal("page", start, len(page.Cards))
		}
		t.Logf("actual helper metadata lane: modeled256 page%d count%d %s", start, want, elapsed)
	}
}

func TestCardPostSuccessPublicationUnknown(t *testing.T) {
	s, _ := cardCapacityFixture(t)
	v := s.view.ViewID
	page, e := s.CardHistory(t.Context(), v, "", 1)
	if e != nil {
		t.Fatal(e)
	}
	date := page.Cards[0].Event.Saturday
	before, e := s.CardRecord(t.Context(), v, date)
	if e != nil {
		t.Fatal(e)
	}
	ack, e := s.BeginCardBuild(t.Context(), v, date)
	if e != nil {
		t.Fatal(e)
	}
	// Only corrupt the queued public wrapper. The original lease, requested date
	// and durable core source remain unchanged; replay succeeds before publication.
	s.queued.card.CardID = strings.Repeat("f", 64)
	if e = s.cycle(t.Context()); e == nil {
		t.Fatal("invalid post-success wrapper was not fatal")
	}
	j, e := s.CardJob(t.Context(), v, ack.JobID)
	if e != nil || j.State != "failed" || j.Error != "outcome_unknown" || j.Result != nil {
		t.Fatal("post-success definite failure", j, e)
	}
	after, e := s.CardRecord(t.Context(), v, date)
	if e != nil {
		t.Fatal("durable reconciliation", e)
	}
	a, _ := gymcard.MarshalRecord(s.cardPin, before.Record)
	b, _ := gymcard.MarshalRecord(s.cardPin, after.Record)
	if !bytes.Equal(a, b) {
		t.Fatal("publication failure changed immutable record")
	}
}
