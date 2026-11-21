// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

type dailyReferenceRow struct {
	Key   string `json:"key_hex"`
	Value string `json:"value_hex"`
}
type dailyReferenceCase struct {
	Name         string              `json:"name"`
	Marker       dailyMarker         `json:"marker"`
	MarkerHex    string              `json:"marker_hex"`
	ConfigID     string              `json:"config_id"`
	SourceID     string              `json:"source_id"`
	ChangeClaims []Claim             `json:"change_claims"`
	ClaimsDigest string              `json:"change_claims_digest"`
	Body         *dailyBatchBody     `json:"body"`
	BodyHex      string              `json:"body_hex"`
	Rows         []dailyReferenceRow `json:"rows"`
}
type dailyReferenceCorpus struct {
	Positives []dailyReferenceCase `json:"positives"`
	Refusals  []struct {
		Name string              `json:"name"`
		Rows []dailyReferenceRow `json:"rows"`
	} `json:"refusals"`
	Budget struct {
		Logical uint64 `json:"logical_bytes_upper"`
		Limit   uint64 `json:"logical_limit"`
		Credit  uint64 `json:"largest_credit_upper"`
		Batch   uint64 `json:"largest_batch_upper"`
	} `json:"budget"`
}

func dailyLedgerReference(t *testing.T) dailyReferenceCorpus {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-profile independent source/config identities")
	}
	raw, err := os.ReadFile("testdata/daily-ledger/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "c490b5b918dd155008ff67013c87d708239a17394554f1a8e6eca05053722af5" {
		t.Fatal("frozen independent ledger corpus changed")
	}
	var corpus dailyReferenceCorpus
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Positives) != 19 || len(corpus.Refusals) != 12 || corpus.Budget.Logical != 1939922194 || corpus.Budget.Limit != dailyLogicalLimit || corpus.Budget.Credit > dailyCreditLimit || corpus.Budget.Batch > dailyBatchLimit {
		t.Fatal("independent budget/corpus")
	}
	return corpus
}
func referenceBytes(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

// This conditional store exercises the exact index/replay engine using frozen
// signed bytes. It has no source owner and cannot authorize a public operation.
func dailyConditional(t *testing.T, c dailyReferenceCase, rows []dailyReferenceRow) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "db")
	db, err := leveldb.OpenFile(path, &opt.Options{ErrorIfExist: true})
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(state{Config: c.Marker.Config, Source: c.Marker.Source}, nil, nil)
	s.db, s.writeBatch = db, db.Write
	s.daily = &dailyState{path: filepath.Dir(path), marker: c.Marker}
	marker, e := dailyEncode(c.Marker, dailyHeadLimit)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(s.daily.path, dailyMarkerName), marker, 0600); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	b := new(leveldb.Batch)
	for _, r := range rows {
		key, value := referenceBytes(t, r.Key), referenceBytes(t, r.Value)
		b.Put(key, value)
		if string(key) == "head" && dailyDecode(value, dailyHeadLimit, &s.daily.head) != nil {
			t.Fatal("head fixture")
		}
		if string(key) == "report" && dailyDecode(value, dailyReportLimit, &s.daily.report) != nil {
			t.Fatal("report fixture")
		}
	}
	if err = db.Write(b, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	return s
}
func dailyRows(t *testing.T, s *Store) map[string]string {
	t.Helper()
	out := map[string]string{}
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		out[string(it.Key())] = string(it.Value())
	}
	if it.Error() != nil {
		t.Fatal(it.Error())
	}
	return out
}
func conditionalAudit(t *testing.T, s *Store, ctx context.Context) (*dailyView, error) {
	t.Helper()
	snap, e := s.db.GetSnapshot()
	if e != nil {
		t.Fatal(e)
	}
	defer snap.Release()
	return s.auditDailyRows(ctx, snap)
}

func TestDailyLedgerIndependentCodecReplay(t *testing.T) {
	for _, c := range dailyLedgerReference(t).Positives {
		t.Run(c.Name, func(t *testing.T) {
			id, e := DailyConfigBindingID(c.Marker.Config)
			if e != nil || id != c.ConfigID {
				t.Fatal("config identity", e)
			}
			id, e = poolshare.DailyConfigID(c.Marker.Source)
			if e != nil || id != c.SourceID {
				t.Fatal("source identity", e)
			}
			marker, e := dailyEncode(c.Marker, dailyHeadLimit)
			if e != nil || !bytes.Equal(marker, referenceBytes(t, c.MarkerHex)) {
				t.Fatal("marker bytes", e)
			}
			if c.Body != nil {
				body, e := dailyEncode(c.Body, dailyBatchLimit)
				if e != nil || !bytes.Equal(body, referenceBytes(t, c.BodyHex)) {
					t.Fatal("body bytes", e)
				}
				digest, e := dailyClaimsDigest(context.Background(), c.ChangeClaims)
				if e != nil || digest != c.ClaimsDigest {
					t.Fatal("independent claim commitment", e)
				}
			}
			s := dailyConditional(t, c, c.Rows)
			before := dailyRows(t, s)
			v, e := conditionalAudit(t, s, context.Background())
			if e != nil {
				t.Fatal("conditional full indexed replay", e)
			}
			if v.head != s.daily.head || !reflect.DeepEqual(v.report, s.daily.report) || !reflect.DeepEqual(before, dailyRows(t, s)) {
				t.Fatal("audit changed durable rows")
			}
			if c.Body != nil && c.Body.Change != nil {
				front := v.replay.frontier()
				if len(front) != 1 || !reflect.DeepEqual(front[0].Claims, c.ChangeClaims) {
					t.Fatal("exact residual origin claims changed")
				}
				front[0].Claims[0].Amount.Numerator = "999"
				if reflect.DeepEqual(front, v.replay.coins) {
					t.Fatal("frontier aliases replay")
				}
			}
		})
	}
}
func TestDailyLedgerForgedInverseIndicesAndCancellation(t *testing.T) {
	corpus := dailyLedgerReference(t)
	for _, c := range corpus.Refusals {
		t.Run(c.Name, func(t *testing.T) {
			s := dailyConditional(t, corpus.Positives[0], c.Rows)
			before := dailyRows(t, s)
			if _, e := conditionalAudit(t, s, context.Background()); e == nil {
				t.Fatal("forged indexed evidence accepted")
			}
			if !reflect.DeepEqual(before, dailyRows(t, s)) {
				t.Fatal("refusal wrote rows")
			}
		})
	}
	c := corpus.Positives[4]
	s := dailyConditional(t, c, c.Rows)
	before := dailyRows(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := conditionalAudit(t, s, ctx); !errors.Is(e, context.Canceled) {
		t.Fatal("original cancellation lost", e)
	}
	if !reflect.DeepEqual(before, dailyRows(t, s)) {
		t.Fatal("canceled audit wrote rows")
	}
}
func TestDailyLedgerStagedImportAtomic(t *testing.T) {
	corpus := dailyLedgerReference(t)
	empty := corpus.Positives[len(corpus.Positives)-1]
	s := dailyConditional(t, empty, empty.Rows)
	v, e := conditionalAudit(t, s, context.Background())
	if e != nil {
		t.Fatal(e)
	}
	m := newDailyMutation(s, v)
	before := dailyRows(t, s)
	var credits []Credit
	for _, row := range corpus.Positives[2].Rows {
		key := referenceBytes(t, row.Key)
		if bytes.HasPrefix(key, []byte("c/")) {
			var c Credit
			if dailyDecode(referenceBytes(t, row.Value), dailyCreditLimit, &c) != nil {
				t.Fatal("credit")
			}
			credits = append(credits, c)
		}
	}
	if len(credits) != 2 {
		t.Fatal("two exact independent credits")
	}
	bad := append([]Credit(nil), credits...)
	bad[1].RoundDigest = "wrong"
	if e = m.stageCredits(context.Background(), bad); e == nil {
		t.Fatal("late invalid credit accepted")
	}
	if m.view.head.Credits != 0 || len(m.writes) != 0 || len(m.view.replay.coins) != 0 || !reflect.DeepEqual(before, dailyRows(t, s)) {
		t.Fatal("late refusal leaked partial import")
	}
	if e = m.stageCredits(context.Background(), credits); e != nil {
		t.Fatal(e)
	}
	if m.view.head.Credits != 2 || len(m.view.replay.coins) != 2 || !reflect.DeepEqual(before, dailyRows(t, s)) {
		t.Fatal("staging mutated durable journal or omitted credit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = m.finish(ctx, "2026-11-18", "deferred", "immature", nil, nil, false); !errors.Is(e, context.Canceled) {
		t.Fatal("canceled commit", e)
	}
	if !reflect.DeepEqual(before, dailyRows(t, s)) {
		t.Fatal("canceled commit wrote credits")
	}
}
func TestDailyLedgerCheapRefusalBeforeEffects(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-only profile refusal")
	}
	p := filepath.Join(t.TempDir(), "absent")
	for _, open := range []func() (*Store, error){func() (*Store, error) { return CreateDaily(context.Background(), p, Config{}, nil, nil) }, func() (*Store, error) { return OpenDaily(context.Background(), p, Config{}, nil, nil) }, func() (*Store, error) { return OpenDailyAudit(context.Background(), p, Config{}, nil) }} {
		s, e := open()
		if s != nil || e != ErrProfile {
			t.Fatal("cheap profile escaped", e)
		}
	}
	if _, e := os.Lstat(p); !os.IsNotExist(e) {
		t.Fatal("refusal touched path")
	}
}

// Reopen exactly the same conditional DB. It still has no concrete source and
// cannot publish authority; this isolates write-uncertainty/replay mechanics.
func reopenConditionalDaily(t *testing.T, old *Store) *Store {
	t.Helper()
	path, m := old.daily.path, old.daily.marker
	if e := old.Close(); e != nil {
		t.Fatal(e)
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true})
	if e != nil {
		t.Fatal(e)
	}
	s := newStore(state{Config: m.Config, Source: m.Source}, nil, nil)
	s.db, s.writeBatch = db, db.Write
	s.daily = &dailyState{path: path, marker: m}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	raw, e := db.Get([]byte("head"), nil)
	if e != nil || dailyDecode(raw, dailyHeadLimit, &s.daily.head) != nil {
		t.Fatal("retained head", e)
	}
	raw, e = db.Get([]byte("report"), nil)
	if e != nil || dailyDecode(raw, dailyReportLimit, &s.daily.report) != nil {
		t.Fatal("retained report", e)
	}
	return s
}

func TestDailyLedgerWriteUncertaintyRetainsExactSignedHistory(t *testing.T) {
	c := dailyLedgerReference(t).Positives[0]
	for _, afterSync := range []bool{false, true} {
		name := "before-sync"
		if afterSync {
			name = "after-sync"
		}
		t.Run(name, func(t *testing.T) {
			s := dailyConditional(t, c, c.Rows)
			before := dailyRows(t, s)
			v, e := conditionalAudit(t, s, context.Background())
			if e != nil || v.pending == nil {
				t.Fatal("original pending", e)
			}
			original := *v.pending
			m := newDailyMutation(s, v)
			if e = m.delivery("confirmed"); e != nil {
				t.Fatal(e)
			}
			write := s.writeBatch
			s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				if afterSync {
					if e := write(b, o); e != nil {
						return e
					}
				}
				return errors.New("published fixture sync outcome unknown")
			}
			if _, e = m.finish(context.Background(), original.Day, "complete", "", nil, nil, true); e != ErrPoisoned || !s.poisoned {
				t.Fatal("uncertain sync published success", e)
			}
			current := dailyRows(t, s)
			if current[string(dailyNumber("b/", 1))] != before[string(dailyNumber("b/", 1))] {
				t.Fatal("signed body changed on delivery update")
			}
			if !afterSync && !reflect.DeepEqual(before, current) {
				t.Fatal("failed pre-sync changed durable rows")
			}
			if afterSync && reflect.DeepEqual(before, current) {
				t.Fatal("post-sync fixture did not persist the original transition")
			}
			s = reopenConditionalDaily(t, s)
			reopened, e := conditionalAudit(t, s, context.Background())
			if e != nil {
				t.Fatal("same-path retained replay", e)
			}
			if afterSync {
				if reopened.pending != nil || reopened.head.PendingBatch != 0 || reopened.replay.accounting.paid[original.Payments[0].GymID] != original.Payments[0].Atoms {
					t.Fatal("synced delivery lost/duplicated")
				}
			} else if reopened.pending == nil || !reflect.DeepEqual(*reopened.pending, original) {
				t.Fatal("pre-sync original pending changed")
			}
			if reopened.head.Credits != 1 || reopened.head.Batches != 1 || !reflect.DeepEqual(current, dailyRows(t, s)) {
				t.Fatal("reopen repaired, duplicated or discarded original evidence")
			}
		})
	}
}

type dailyCapacityBackend struct {
	Backend
	inspections, quotes, signs, broadcasts, seen int
}

func (b *dailyCapacityBackend) Inspect(_ context.Context, coins []Coin) ([]Funding, error) {
	b.inspections++
	b.seen = len(coins)
	out := make([]Funding, len(coins))
	for i, c := range coins {
		out[i] = Funding{TxID: c.TxID, Index: c.Index, State: "ready"}
	}
	return out, nil
}
func (b *dailyCapacityBackend) EstimateFee(context.Context, []Coin, []Payment, string) (int64, error) {
	b.quotes++
	return 0, errors.New("unexpected fee call")
}
func (b *dailyCapacityBackend) Sign(context.Context, []Coin, []byte) ([]byte, error) {
	b.signs++
	return nil, errors.New("unexpected sign")
}
func (b *dailyCapacityBackend) Broadcast(context.Context, []byte) (string, error) {
	b.broadcasts++
	return "", errors.New("unexpected broadcast")
}

// Conditional coins isolate the selected actual inspect/plan/quote/finish
// leaves. They are not imported through a public Store and prove no source or
// funding authority. The independent shape corpus separately pins the weight.
func TestDailyLedgerConditionalWholeFrontierWeightDeferral(t *testing.T) {
	corpus := dailyLedgerReference(t)
	c := corpus.Positives[len(corpus.Positives)-1]
	c.Marker.Config.Destinations = make([]Destination, 256)
	for i := range c.Marker.Config.Destinations {
		c.Marker.Config.Destinations[i] = Destination{GymID: fmt.Sprintf("%064x", i+1), Script: fmt.Sprintf("0014%040x", i+1)}
	}
	var e error
	c.Marker.ConfigID, e = DailyConfigBindingID(c.Marker.Config)
	if e != nil {
		t.Fatal(e)
	}
	// This is fixture setup, not an expected-ID vector or product-output oracle.
	c.Rows = append([]dailyReferenceRow(nil), c.Rows...)
	for i, row := range c.Rows {
		if string(referenceBytes(t, row.Key)) == "head" {
			var h dailyHeader
			if json.Unmarshal(referenceBytes(t, row.Value), &h) != nil {
				t.Fatal("head")
			}
			h.ConfigID = c.Marker.ConfigID
			raw, _ := json.Marshal(h)
			c.Rows[i].Value = hex.EncodeToString(raw)
		}
	}
	s := dailyConditional(t, c, c.Rows)
	backend := &dailyCapacityBackend{}
	s.backend = backend
	v, e := conditionalAudit(t, s, context.Background())
	if e != nil {
		t.Fatal(e)
	}
	m := newDailyMutation(s, v)
	credits := make([]Credit, 1349)
	for i := range credits {
		id := fmt.Sprintf("%064x", i+1000)
		gym := c.Marker.Config.Destinations[i%256].GymID
		credits[i] = Credit{RoundHash: id, RoundDigest: fmt.Sprintf("%064x", 1), Coin: Coin{TxID: id, Atoms: 1000000000, Script: c.Marker.Config.ChangeScript, Coinbase: true, BlockHash: id, BlockHeight: uint32(i + 1), Claims: []Claim{{RoundHash: id, RoundHeight: uint32(i + 1), Amount: Fraction{"10000000", "1"}}, {RoundHash: id, RoundHeight: uint32(i + 1), GymID: gym, Amount: Fraction{"990000000", "1"}}}}}
	}
	if e = m.stageCredits(context.Background(), credits); e != nil {
		t.Fatal(e)
	}
	ready, reasons, e := s.inspectDaily(context.Background(), m.view.replay.coins)
	if e != nil || len(ready) != 1349 {
		t.Fatal("whole readiness", e)
	}
	before := dailyOwnedCoins(ready)
	_, _, _, reason, e := s.dailyPlanQuote(context.Background(), ready)
	if e != nil || reason != "transaction-capacity" || !reflect.DeepEqual(before, ready) {
		t.Fatal("whole overweight candidate classification", reason, e)
	}
	r, e := m.finish(context.Background(), "2026-11-18", "deferred", reason, ready, reasons, false)
	if e != nil {
		t.Fatal(e)
	}
	if backend.inspections != 1 || backend.seen != 1349 || backend.quotes != 0 || backend.signs != 0 || backend.broadcasts != 0 {
		t.Fatal("subset/paging or effect before capacity", backend)
	}
	if r.State != "deferred" || r.Reason != "transaction-capacity" || s.daily.head.Credits != 1349 || s.daily.head.Batches != 0 || s.daily.head.PendingBatch != 0 || s.daily.head.LastDay != "" {
		t.Fatal("deferral consumed a day/batch or dropped earnings")
	}
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		for _, prefix := range []string{"b/", "bs/", "bd/", "bt/", "spent/"} {
			if bytes.HasPrefix(it.Key(), []byte(prefix)) {
				t.Fatal("financial reservation on deferral")
			}
		}
	}
	if it.Error() != nil {
		t.Fatal(it.Error())
	}
}
