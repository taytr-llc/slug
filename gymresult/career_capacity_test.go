//go:build slugtestclock && !slugtestpow && !race

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/wire/v2"
)

// The entire 256-row history is explicitly modeled storage stress data. It
// shifts births with event dates and uses published fixture signing keys; it
// proves neither new C1 admission, normal-PoW work nor new fight computation.
// The unchanged full Result validator still audits every source/signature/tx.
func careerCapacityRecord(t *testing.T, f *resultFixture, template Record, date string, i int) Record {
	t.Helper()
	var r Record
	b, _ := json.Marshal(template)
	json.Unmarshal(b, &r)
	src := &r.Statement.Statement.Source
	s := &src.Source
	p := f.pin.SourcePin.SourcePin
	ev, e := gymevent.EventForDate(p.SourcePin.SourcePin, date)
	if e != nil {
		t.Fatal(e)
	}
	delta := ev.CutoffMS - s.Event.CutoffMS
	s.Event = ev
	s.CardID, e = gymcard.CardID(p, ev)
	if e != nil {
		t.Fatal(e)
	}
	s.SnapshotDigest = hash("modeled career snapshot", []byte(date))
	s.SeedEventKey, e = gymseed.EventKey(p.SourcePin, ev)
	if e != nil {
		t.Fatal(e)
	}
	s.CardRecordDigest = hash("modeled career card", []byte(date))
	s.CardSourceDigest = hash("modeled career card source", []byte(date))
	s.CardCreatedAtMS = max(f.s.meta.CreatedAtMS, ev.CutoffMS+2000)
	s.FinalizedAtMS = ev.CutoffMS + 1999
	for j := range s.Corners {
		m := &s.Corners[j].Member
		m.Assignment.BornAtMS += delta
		m.Assignment.AcceptedAtMS += delta
		m.SlotAssignedAtMS += delta
		m.ConditionAtCutoff.BornAtMS += delta
		m.ConditionAtCutoff.AsOfMS += delta
	}
	input := hash("SLUG candidate card input\x00v1\x00", rawID(s.CardID), rawID(s.SnapshotDigest), rawID(s.SeedEventKey), le32(s.TargetHeight), rawID(s.TargetHash), rawID(p.RulesID))
	seed := hash("SLUG candidate card seed\x00v1\x00", rawID(input), rawID(s.TargetHash))
	var class uint32
	for j, c := range []string{"light", "welter", "middle", "light_heavy", "heavy"} {
		if c == s.Bout.Class {
			class = uint32(j)
		}
	}
	s.Bout.BoutID = hash("SLUG candidate card bout\x00v1\x00", rawID(input), le32(class), rawID(s.Bout.CornerA), rawID(s.Bout.CornerB))
	s.Bout.Seed = hash("SLUG candidate card bout seed\x00v1\x00", rawID(seed), rawID(s.Bout.BoutID))
	src.SourceDigest, e = gymcard.BoutSourceDigest(p, *s)
	if e != nil {
		t.Fatal("modeled compact source", e)
	}
	src.RecordDigest = hash("modeled career saved row", []byte(date))
	src.InputID = hash("modeled career input", []byte(date))
	src.ResultDigest = hash("modeled career result", []byte(date))
	src.CreatedAtMS = s.CardCreatedAtMS + 1
	r.Statement.Statement = expectedStatement(f.pin, *src)
	r.Statement.Signature = hex.EncodeToString(ed25519.Sign(fixtureKey(false), statementMessage(r.Statement.Statement)))
	r.ResultKey = r.Statement.Statement.ResultKey
	r.PreparedAtMS = src.CreatedAtMS + 1
	payload, e := Carrier(r.Statement.Statement)
	if e != nil {
		t.Fatal(e)
	}
	coin := template.Publication.Coin
	coin.Outpoint.TxID = hash("modeled career funding", []byte(date))
	coin.Outpoint.Index = uint32(i)
	coin.BlockHash = hash("modeled career funding block", []byte(date))
	builder := modelPublisher{dynamicCarrier: true}
	raw, e := builder.BuildPublication(context.Background(), payload, coin)
	if e != nil {
		t.Fatal(e)
	}
	var tx wire.MsgTx
	if e = tx.Deserialize(bytes.NewReader(raw)); e != nil {
		t.Fatal(e)
	}
	r.Publication = &Publication{Version: 1, RawTx: hex.EncodeToString(raw), TxID: tx.TxHash().String(), Coin: coin, BuiltAtMS: r.PreparedAtMS, DispatchCount: 1, FirstDispatchAtMS: r.PreparedAtMS + 1, LastDispatchAtMS: r.PreparedAtMS + 1, State: "submitted"}
	header := wire.BlockHeader{MerkleRoot: tx.TxHash()}
	var hb bytes.Buffer
	header.Serialize(&hb)
	height := uint32(1000 + i)
	anchor := Anchor{Height: height, Hash: hash("modeled career inclusion", []byte(date)), Header: hex.EncodeToString(hb.Bytes()), TxID: r.Publication.TxID, RawTxDigest: plainSHA(raw), RawTxSize: uint32(len(raw)), TxCount: 1, TxIndex: 0, OutputIndex: 0, Payload: hex.EncodeToString(payload[:]), MerkleBranch: []string{}}
	r.Inclusion = &Inclusion{Version: 1, Generation: 0, Anchor: anchor, ObservedAtMS: r.PreparedAtMS + 2, ObservedTip: Tip{Height: height, Hash: anchor.Hash}}
	genesis, _ := chainhash.NewHashFromStr(f.pin.Genesis)
	ah, _ := chainhash.NewHashFromStr(anchor.Hash)
	cp := chaincfg.SignedCheckpoint{Version: 1, Network: wire.BitcoinNet(f.pin.Network), GenesisHash: *genesis, Height: int32(height), Hash: *ah}
	message, e := cp.SigningBytes()
	if e != nil {
		t.Fatal(e)
	}
	cp.Signature = ed25519.Sign(fixtureKey(true), message)
	r.Checkpoint = &cp
	tip := Tip{Height: height + 60, Hash: hash("modeled career finality tip", []byte(date))}
	r.Finality = &Finality{Checkpoint: cp, Inclusion: *r.Inclusion, ObservedAtMS: r.PreparedAtMS + 3, ObservedTip: tip, Descendants: 60}
	r.Cursor = initialCursor(f.pin, r.ResultKey, 0)
	r.Cursor.Height = int32(height)
	r.Cursor.Hash = anchor.Hash
	r.Cursor.CompleteToHeight = int32(height)
	r.Cursor.CompleteToHash = anchor.Hash
	r.Observation = Observation{ObservedAtMS: r.Finality.ObservedAtMS, ChainObservedAtMS: r.Finality.ObservedAtMS, Tip: &tip, NodeState: "online", Descendants: 60, CurrentFinality: true}
	r.Conflicts = []Conflict{}
	r.PriorInclusions = []PriorInclusion{}
	r.ScanRestarts = []ScanRestart{}
	r.Equivocation = nil
	r.ConflictOverflow = false
	if e = validateRecord(f.pin, r); e != nil {
		t.Fatal("modeled finalized Result", i, e)
	}
	return r
}

// This normal-only latency gate keeps the 25-second startup and 500ms metadata
// budgets. Race correctness is covered separately; instrumentation is not a
// production latency benchmark. Run only in the coordinated quiet test slot.
func TestCareerCapacityFull256AuditAndSnapshot(t *testing.T) {
	f := newResultFixture(t)
	activateFixture(t, f)
	activation := *f.s.meta.Career
	template := referenceRecord(t, "record-finalized60")
	startDate, e := time.Parse("2006-01-02", template.Statement.Statement.Source.Source.Event.Saturday)
	if e != nil {
		t.Fatal(e)
	}
	batch := new(leveldb.Batch)
	last := f.now.Load()
	var lastRow Record
	for i := 0; i < 256; i++ {
		r := careerCapacityRecord(t, f, template, startDate.AddDate(0, 0, 7*i).Format("2006-01-02"), i)
		if e = put(batch, key('r', r.ResultKey), r, MaxRecordSize); e != nil {
			t.Fatal(e)
		}
		if e = put(batch, coinKey('u', r.Publication.Coin.Outpoint), reservation{1, r.ResultKey, r.Publication.TxID}, 256); e != nil {
			t.Fatal(e)
		}
		last = max(last, recordMaxTime(r))
		lastRow = r
	}
	next := f.s.meta
	next.ResultCount = 256
	next.ClockMS = last + 1
	if e = put(batch, []byte("meta"), next, MaxMetadataSize); e != nil {
		t.Fatal(e)
	}
	if e = f.s.db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	f.s.Close()
	f.now.Store(last + 1)
	path := filepath.Join(f.dir, "result")
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	start := time.Now()
	f.s, e = OpenCareerForTestClock(ctx, path, f.pin, f.bout, f.clock)
	elapsed := time.Since(start)
	cancel()
	if e != nil || elapsed >= 25*time.Second {
		t.Fatal("full256 startup", elapsed, e)
	}
	if len(f.s.index) != 256 || len(f.s.careerIndex) != 256 || *f.s.meta.Career != activation {
		t.Fatal("partial index or activation changed")
	}
	encodedIndex, _ := json.Marshal(f.s.careerIndex)
	if len(encodedIndex) > careerstate.MaxIndexSize {
		t.Fatal("index capacity")
	}
	t.Logf("modeled256 complete Open audit=%s career_index_bytes=%d cap=%d", elapsed, len(encodedIndex), careerstate.MaxIndexSize)
	reader, _ := f.s.CareerReader()
	ids := careerIDs(f)
	for _, mode := range []careerstate.Mode{careerstate.Current, careerstate.Cutoff} {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		start = time.Now()
		snap, e := reader.Snapshot(ctx, ids, last+1, mode)
		elapsed = time.Since(start)
		cancel()
		if e != nil || elapsed >= 500*time.Millisecond {
			t.Fatal("full256 sealed snapshot", mode, elapsed, e)
		}
		for _, id := range ids {
			v, ok := snap.Totals(id)
			if !ok || v.XP != 15 || v.Wins+v.Losses+v.Draws != 256 {
				t.Fatal("missing/capped record", v)
			}
		}
		t.Logf("modeled256 %s snapshot=%s", mode, elapsed)
	}
	// Refuse a changed first-finality event before issuing another database write.
	c, done := fixtureCall(t, f)
	_ = c
	writes := 0
	write := f.s.writeBatch
	f.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error { writes++; return write(b, o) }
	lastRow.Finality.ObservedAtMS++
	if _, e = f.s.proposedCareerIndex(lastRow); !errors.Is(e, ErrCorrupt) || writes != 0 {
		t.Fatal("immutable event mutation", e, writes)
	}
	f.s.writeBatch = write
	done()
	f.s.Close()
	cancelCtx, stop := context.WithCancel(context.Background())
	rows := 0
	failed, e := openCareer(cancelCtx, path, f.pin, f.bout, f.clock, "open", func(stage string) {
		if stage == "career-audit-row" {
			rows++
			if rows == 3 {
				stop()
			}
		}
	})
	stop()
	if failed != nil || !errors.Is(e, ErrCancelled) || rows != 3 {
		if failed != nil {
			failed.Close()
		}
		t.Fatal("partial cancelled audit", e, rows)
	}
	f.s, e = OpenCareerForTestClock(context.Background(), path, f.pin, f.bout, f.clock)
	if e != nil || len(f.s.careerIndex) != 256 {
		t.Fatal("cancelled audit retained owner", e)
	}
	t.Log(fmt.Sprintf("all256 events retained; cancelled audit joined after %d rows", rows))
}
