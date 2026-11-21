// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolshare"
)

// Logical rows are the read-only assertion. This does not claim filesystem
// metadata or a concurrently open LevelDB's own log files are immutable.
func walkTestRows(t *testing.T, s *Store) map[string]string {
	t.Helper()
	rows := make(map[string]string)
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		rows[string(it.Key())] = string(it.Value())
	}
	if err := it.Error(); err != nil {
		t.Fatal(err)
	}
	return rows
}

func walkTestRead(t *testing.T, s *Store, want poolshare.Config) []Record {
	t.Helper()
	var got []Record
	cfg, err := s.WalkSettlement(context.Background(), func(page []Record) error {
		if len(page) == 0 || len(page) > 8 {
			t.Fatalf("invalid owned page length %d", len(page))
		}
		for _, row := range page {
			if row.Round.Sequence != uint64(len(got)+1) {
				t.Fatal("nonascending global round sequence")
			}
			got = append(got, row)
		}
		return nil
	})
	if err != nil || cfg != want {
		t.Fatal("walk did not authenticate original source", cfg, err)
	}
	return got
}

func TestRoundWalkZeroConsumedPrefixWithNewerSource(t *testing.T) {
	h := newRoundHarness(t, false)
	ctx := context.Background()
	current, err := h.source.AccountingPage(ctx, 0, 0, 8)
	if err != nil || current.Through != 3 || h.store.meta.Through != 0 {
		t.Fatal("fixture is not an empty consumed prefix with newer source", err)
	}
	before := walkTestRows(t, h.store)
	if got := walkTestRead(t, h.store, h.sourceConfig); len(got) != 0 {
		t.Fatal("zero prefix consumed current source tail")
	}
	if cfg, err := h.store.WalkSettlement(ctx, nil); err != nil || cfg != h.sourceConfig {
		t.Fatal("nil visitor did not authenticate empty prefix", err)
	}
	if !reflect.DeepEqual(before, walkTestRows(t, h.store)) || h.observer.calls != 0 {
		t.Fatal("read-only walk changed journal or observed node")
	}
	if got, err := h.store.Sync(ctx); err != nil || got != (Summary{Through: 3, Rounds: 1}) {
		t.Fatal("walk interfered with later explicit Sync", got, err)
	}
	if got := walkTestRead(t, h.store, h.sourceConfig); len(got) != 1 || got[0].Round.FirstAccepted != 1 || got[0].Round.LastAccepted != 3 {
		t.Fatal("explicit Sync did not retain original membership")
	}
}

func TestRoundWalkLateWinnerReopenPreservesAcceptedOrder(t *testing.T) {
	h := newRoundHarness(t, true)
	ctx := context.Background()
	if got, err := h.store.Sync(ctx); err != nil || got != (Summary{Through: 2, PendingShares: 2}) {
		t.Fatal("saved winning intent consumed open window", got, err)
	}
	if got := walkTestRead(t, h.store, h.sourceConfig); len(got) != 0 {
		t.Fatal("unfinished shares became a settlement record")
	}
	h.reopen(t)
	h.poolNode.canonical = true
	late, err := h.source.Submit(ctx, roundToken, h.input.Job.ID, h.winner.Nonce)
	if err != nil || late.Sequence != 1 || late.AcceptedSequence != 3 || late.Acceptance != "canonical-late-winner" {
		t.Fatal("exact saved winner did not append late acceptance", late, err)
	}
	if got, err := h.store.Sync(ctx); err != nil || got != (Summary{Through: 3, Rounds: 1}) {
		t.Fatal("late winner did not close original interval", got, err)
	}
	first := walkTestRead(t, h.store, h.sourceConfig)
	if len(first) != 1 || first[0].Round.FirstAccepted != 1 || first[0].Round.LastAccepted != 3 || first[0].Round.Winner.Share.Sequence != 1 || first[0].Round.Winner.Share.AcceptedSequence != 3 || first[0].Round.ShareKeys[2] != h.winner.Key || len(first[0].Round.Allocations) != 1 || first[0].Round.Allocations[0] != (Allocation{GymID: h.input.Job.GymID, Shares: 3, Numerator: "1000000000", Denominator: "1"}) {
		t.Fatal("late acceptance changed window or exact existing gross allocation", first)
	}
	h.reopen(t)
	if duplicate, err := h.source.Submit(ctx, roundToken, h.input.Job.ID, h.winner.Nonce); err != nil || duplicate != late {
		t.Fatal("source duplicate changed original receipt", err)
	}
	if _, err := h.store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if got := walkTestRead(t, h.store, h.sourceConfig); !reflect.DeepEqual(got, first) {
		t.Fatal("reopen/duplicate changed one original round")
	}
}

func TestRoundWalkCorruptionBeforeAnyVisitor(t *testing.T) {
	for _, mode := range []string{"missing-row", "missing-index", "wrong-index", "extra-row", "extra-index", "unknown-key", "malformed-row", "changed-meta", "changed-prefix"} {
		t.Run(mode, func(t *testing.T) {
			h := newRoundHarness(t, false)
			ctx := context.Background()
			if _, err := h.store.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			batch := new(leveldb.Batch)
			switch mode {
			case "missing-row":
				batch.Delete(roundKey(h.winner.Hash))
			case "missing-index":
				batch.Delete(indexKey(1))
			case "wrong-index":
				batch.Put(indexKey(1), []byte(h.input.Job.ID))
			case "extra-row":
				raw, err := h.store.db.Get(roundKey(h.winner.Hash), nil)
				if err != nil {
					t.Fatal(err)
				}
				batch.Put(roundKey(strings.Repeat("e", 64)), raw)
			case "extra-index":
				batch.Put(indexKey(2), []byte(h.winner.Hash))
			case "unknown-key":
				batch.Put([]byte("unexpected"), []byte("retained"))
			case "malformed-row":
				batch.Put(roundKey(h.winner.Hash), []byte("{}"))
			case "changed-meta":
				batch.Put([]byte("meta"), []byte("{}"))
			case "changed-prefix":
				h.store.meta.Prefix = strings.Repeat("0", 64)
			}
			if err := h.store.db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
				t.Fatal(err)
			}
			before := walkTestRows(t, h.store)
			called := 0
			cfg, err := h.store.WalkSettlement(ctx, func([]Record) error { called++; return nil })
			if err == nil || cfg != (poolshare.Config{}) || called != 0 {
				t.Fatal("corrupt journal disclosed a prefix or source", cfg, called, err)
			}
			if cfg, err := h.store.WalkSettlement(ctx, nil); err == nil || cfg != (poolshare.Config{}) {
				t.Fatal("nil visitor skipped full audit", cfg, err)
			}
			if !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
				t.Fatal("failed read repaired journal")
			}
		})
	}
}

func TestRoundWalkVisitorFailureAndCancellationNoWrites(t *testing.T) {
	h := newRoundHarness(t, false)
	if _, err := h.store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := walkTestRows(t, h.store)
	sentinel := errors.New("visitor stopped")
	for _, mode := range []string{"visitor", "cancel-in-visitor", "pre-canceled", "nil-context"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			called := 0
			want := sentinel
			if mode == "pre-canceled" {
				cancel()
				want = context.Canceled
			}
			if mode == "cancel-in-visitor" {
				want = context.Canceled
			}
			if mode == "nil-context" {
				ctx = nil
				want = ErrInvalid
			}
			cfg, err := h.store.WalkSettlement(ctx, func([]Record) error {
				called++
				if mode == "cancel-in-visitor" {
					cancel()
					return nil
				}
				return sentinel
			})
			wantCalls := 1
			if mode == "pre-canceled" || mode == "nil-context" {
				wantCalls = 0
			}
			if !errors.Is(err, want) || cfg != (poolshare.Config{}) || called != wantCalls {
				t.Fatal("failed walk returned success/config or wrong callback count", cfg, called, err)
			}
			if !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
				t.Fatal("failed walk wrote journal")
			}
			if len(walkTestRead(t, h.store, h.sourceConfig)) != 1 {
				t.Fatal("failed walk lost original gate")
			}
		})
	}
}

func TestRoundWalkOwnedPagesKeepOriginalRefreshGate(t *testing.T) {
	h := newRoundHarness(t, false)
	ctx := context.Background()
	if _, err := h.store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	h.observer.depth = 60
	want, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil || want.Confirmation == nil || want.Observation == nil {
		t.Fatal("missing original lock", err)
	}
	before := walkTestRows(t, h.store)
	observations := h.observer.calls
	cfg, err := h.store.WalkSettlement(ctx, func(page []Record) error {
		if len(page) != 1 || !reflect.DeepEqual(page[0], want) {
			t.Fatal("wrong original record")
		}
		if _, err := h.store.Refresh(ctx, h.winner.Hash); !errors.Is(err, ErrBusy) {
			t.Fatal("refresh interleaved with walk", err)
		}
		if _, err := h.store.SettlementSnapshot(ctx); !errors.Is(err, ErrBusy) {
			t.Fatal("snapshot reentered original gate", err)
		}
		page[0].Round.ShareKeys[0] = "changed"
		page[0].Round.Allocations[0].Numerator = "0"
		page[0].Round.Winner.Proof.Header = "changed"
		page[0].Round.Winner.Proof.MerkleBranch = append(page[0].Round.Winner.Proof.MerkleBranch, "changed")
		page[0].Observation.TipHash = "changed"
		page[0].Confirmation.TipHash = "changed"
		return nil
	})
	if err != nil || cfg != h.sourceConfig {
		t.Fatal(err)
	}
	if h.observer.calls != observations || !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
		t.Fatal("walk observed or changed original source")
	}
	if got, err := h.store.Get(ctx, h.winner.Hash); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("visitor owned data aliases stored record", err)
	}
}

func TestRoundWalkClosedSourceNoDisclosure(t *testing.T) {
	h := newRoundHarness(t, false)
	ctx := context.Background()
	if _, err := h.store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	before := walkTestRows(t, h.store)
	if err := h.source.Close(); err != nil {
		t.Fatal(err)
	}
	called := false
	cfg, err := h.store.WalkSettlement(ctx, func([]Record) error { called = true; return nil })
	if !errors.Is(err, ErrSourceUnavailable) || cfg != (poolshare.Config{}) || called {
		t.Fatal("closed concrete source disclosed evidence", cfg, err)
	}
	if !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
		t.Fatal("closed source walk altered journal")
	}
}

func TestRoundWalkCloseJoinsOriginalVisitor(t *testing.T) {
	h := newRoundHarness(t, false)
	if _, err := h.store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	allowReturn := func() { releaseOnce.Do(func() { close(release) }) }
	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		cfg poolshare.Config
		err error
	}
	resultCh := make(chan result, 1)
	walkDone := make(chan struct{})
	go func() {
		defer close(walkDone)
		cfg, err := h.store.WalkSettlement(ctx, func([]Record) error {
			close(entered)
			<-release
			return nil
		})
		resultCh <- result{cfg, err}
	}()
	defer func() { allowReturn(); cancel(); <-walkDone }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("walk did not enter visitor")
	}
	closeResult := make(chan error, 1)
	closeDone := make(chan struct{})
	go func() { defer close(closeDone); closeResult <- h.store.Close() }()
	defer func() { allowReturn(); <-closeDone }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		h.store.lifecycle.Lock()
		closing := h.store.closing
		h.store.lifecycle.Unlock()
		if closing {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Close did not fence new calls")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-closeDone:
		t.Fatal("Close returned while original visitor was active")
	default:
	}
	if cfg, err := h.store.WalkSettlement(context.Background(), nil); !errors.Is(err, ErrClosed) || cfg != (poolshare.Config{}) {
		t.Fatal("closing store admitted new walk", err)
	}
	allowReturn()
	select {
	case got := <-resultCh:
		if got.err != nil || got.cfg != h.sourceConfig {
			t.Fatal("original admitted walk did not finish", got.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("original walk did not join")
	}
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("original Close did not join")
	}
}
