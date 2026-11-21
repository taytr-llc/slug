// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// Private readiness-selection tests only. Modeled coins do not become credits,
// source records, payment authority or a beyond-V1-cap live ledger.
type payoutInspectionBackend struct {
	pages                               [][]Coin
	onPage                              func(context.Context, []Coin, int) ([]Funding, error)
	quotes, signs, observes, broadcasts int
}

func (b *payoutInspectionBackend) Inspect(ctx context.Context, coins []Coin) ([]Funding, error) {
	b.pages = append(b.pages, clone(coins))
	if len(coins) == 0 || len(coins) > 256 {
		return nil, ErrBackend
	}
	if b.onPage != nil {
		return b.onPage(ctx, coins, len(b.pages))
	}
	return payoutInspectionReady(coins), nil
}
func (b *payoutInspectionBackend) EstimateFee(context.Context, []Coin, []Payment, string) (int64, error) {
	b.quotes++
	return 0, ErrBackend
}
func (b *payoutInspectionBackend) Sign(context.Context, []Coin, []byte) ([]byte, error) {
	b.signs++
	return nil, ErrBackend
}
func (b *payoutInspectionBackend) ObserveTransaction(context.Context, string) (TransactionObservation, error) {
	b.observes++
	return TransactionObservation{}, ErrBackend
}
func (b *payoutInspectionBackend) Broadcast(context.Context, []byte) (string, error) {
	b.broadcasts++
	return "", ErrBackend
}
func payoutInspectionReady(coins []Coin) []Funding {
	rows := make([]Funding, len(coins))
	for i, c := range coins {
		rows[i] = Funding{TxID: c.TxID, Index: c.Index, State: "ready"}
	}
	return rows
}
func payoutInspectionCoins(n int) []Coin {
	coins := make([]Coin, n)
	for i := range coins {
		hash := fmt.Sprintf("%064x", i+1)
		coins[i] = Coin{TxID: hash, Index: uint32(i % 2), Atoms: 100000000, Script: "0014" + strings.Repeat("1", 40), Coinbase: true, BlockHash: hash, BlockHeight: uint32(i + 1), Claims: []Claim{{RoundHash: hash, RoundHeight: uint32(i + 1), GymID: strings.Repeat("a", 64), Amount: Fraction{"100000000", "1"}}}}
	}
	return coins
}
func payoutInspectionStore(t *testing.T, backend *payoutInspectionBackend) (*Store, func()) {
	t.Helper()
	s := &Store{backend: backend, state: state{Config: Config{Version: Version1}, LastClockMS: 123}}
	before := clone(s.state)
	writes := 0
	s.writeBatch = func(*leveldb.Batch, *opt.WriteOptions) error { writes++; return ErrPoisoned }
	return s, func() {
		if !reflect.DeepEqual(s.state, before) || writes != 0 || backend.quotes+backend.signs+backend.observes+backend.broadcasts != 0 {
			t.Fatal("inspection changed state or attempted payment effect")
		}
	}
}

func TestSettlementInspectPagesReturnsWholeReadySet(t *testing.T) {
	coins := payoutInspectionCoins(513)
	before := clone(coins)
	b := &payoutInspectionBackend{}
	s, check := payoutInspectionStore(t, b)
	defer check()
	ready, reasons, err := s.inspect(context.Background(), coins)
	if err != nil || len(reasons) != 0 || !reflect.DeepEqual(ready, before) {
		t.Fatal("selection truncated or reordered the ready set", len(ready), err)
	}
	if len(b.pages) != 3 || !reflect.DeepEqual(b.pages[0], before[:256]) || !reflect.DeepEqual(b.pages[1], before[256:512]) || !reflect.DeepEqual(b.pages[2], before[512:]) {
		t.Fatal("funding pages did not cover exact original candidates once")
	}
	if transactionCapacity(len(ready), 1, true) {
		t.Fatal("whole oversized set admitted instead of capacity deferral")
	}
	if !reflect.DeepEqual(coins, before) {
		t.Fatal("inspection mutated input claims")
	}
}

func TestSettlementInspectPagesSelectsLaterReadyCoin(t *testing.T) {
	coins := payoutInspectionCoins(257)
	before := clone(coins)
	b := &payoutInspectionBackend{onPage: func(_ context.Context, page []Coin, number int) ([]Funding, error) {
		rows := payoutInspectionReady(page)
		if number == 1 {
			for i := range rows {
				rows[i].State = "immature"
			}
		}
		// Backend ownership must not include caller slices or nested claims.
		page[0].TxID = "changed"
		page[0].Claims[0].Amount = Fraction{"0", "1"}
		return rows, nil
	}}
	s, check := payoutInspectionStore(t, b)
	defer check()
	ready, reasons, err := s.inspect(context.Background(), coins)
	if err != nil || !reflect.DeepEqual(ready, before[256:]) || reasons[strings.Repeat("a", 64)] != "immature" || len(b.pages) != 2 {
		t.Fatal("inspection selected first page instead of all mature candidates", ready, reasons, err)
	}
	if !transactionCapacity(len(ready), 1, true) || !reflect.DeepEqual(coins, before) {
		t.Fatal("candidate count was confused with ready count or borrowed inputs mutated")
	}
}

func TestSettlementInspectPagesReasonPrecedence(t *testing.T) {
	coins := payoutInspectionCoins(513)
	b := &payoutInspectionBackend{onPage: func(_ context.Context, page []Coin, number int) ([]Funding, error) {
		rows := payoutInspectionReady(page)
		rows[0].State = []string{"immature", "awaiting-confirmation", "funding-conflict"}[number-1]
		return rows, nil
	}}
	s, check := payoutInspectionStore(t, b)
	defer check()
	ready, reasons, err := s.inspect(context.Background(), coins)
	want := append(append([]Coin(nil), coins[1:256]...), coins[257:512]...)
	if err != nil || !reflect.DeepEqual(ready, want) || len(reasons) != 1 || reasons[strings.Repeat("a", 64)] != "funding-conflict" {
		t.Fatal("page boundary lost conflict precedence or changed selection", reasons, err)
	}
}

func TestSettlementInspectLaterPageFailureDiscardsPartial(t *testing.T) {
	sentinel := errors.New("later original funding observer failed")
	for _, mode := range []string{"backend-error", "missing", "extra", "wrong-txid", "wrong-index", "reordered", "unknown-state"} {
		t.Run(mode, func(t *testing.T) {
			coins := payoutInspectionCoins(513)
			before := clone(coins)
			b := &payoutInspectionBackend{onPage: func(_ context.Context, page []Coin, number int) ([]Funding, error) {
				rows := payoutInspectionReady(page)
				if number != 2 {
					return rows, nil
				}
				switch mode {
				case "backend-error":
					return nil, sentinel
				case "missing":
					return rows[:len(rows)-1], nil
				case "extra":
					return append(rows, rows[0]), nil
				case "wrong-txid":
					rows[0].TxID = strings.Repeat("f", 64)
				case "wrong-index":
					rows[0].Index++
				case "reordered":
					rows[0], rows[1] = rows[1], rows[0]
				case "unknown-state":
					rows[0].State = "probably-ready"
				}
				return rows, nil
			}}
			s, check := payoutInspectionStore(t, b)
			defer check()
			ready, reasons, err := s.inspect(context.Background(), coins)
			want := ErrBackend
			if mode == "backend-error" {
				want = sentinel
			}
			// Complete bounded observation collection before identity validation
			// is allowed; no additional payment effect or partial result is.
			if !errors.Is(err, want) || ready != nil || reasons != nil || len(b.pages) < 2 || len(b.pages) > 3 || !reflect.DeepEqual(coins, before) {
				t.Fatal("later refusal exposed partial readiness or hid exact error", len(ready), reasons, err)
			}
		})
	}
}

func TestSettlementInspectPagedCancellationDiscardsPartial(t *testing.T) {
	for _, mode := range []string{"before", "between-pages", "after-final-page"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			coins := payoutInspectionCoins(257)
			before := clone(coins)
			if mode == "before" {
				cancel()
			}
			b := &payoutInspectionBackend{onPage: func(_ context.Context, page []Coin, number int) ([]Funding, error) {
				rows := payoutInspectionReady(page)
				if mode == "between-pages" && number == 1 || mode == "after-final-page" && number == 2 {
					cancel()
				}
				return rows, nil
			}}
			s, check := payoutInspectionStore(t, b)
			defer check()
			ready, reasons, err := s.inspect(ctx, coins)
			wantCalls := map[string]int{"before": 0, "between-pages": 1, "after-final-page": 2}[mode]
			if !errors.Is(err, context.Canceled) || ready != nil || reasons != nil || len(b.pages) != wantCalls || !reflect.DeepEqual(coins, before) {
				t.Fatal("canceled page selection disclosed partial readiness or continued work", len(b.pages), err)
			}
		})
	}
}
