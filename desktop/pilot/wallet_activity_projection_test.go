// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slugwallet/wallet"
)

type activityReferenceCase struct {
	Name     string          `json:"name"`
	Height   uint32          `json:"capture_height"`
	Record   json.RawMessage `json:"record"`
	Layer    string          `json:"layer"`
	Expected struct {
		Item    WalletActivityItem     `json:"item"`
		Outputs []WalletActivityOutput `json:"outputs"`
	} `json:"expected"`
}
type activityReference struct {
	Transactions struct {
		Positives []activityReferenceCase `json:"positives"`
		Negatives []struct {
			Name   string          `json:"name"`
			Height uint32          `json:"capture_height"`
			Record json.RawMessage `json:"record"`
			Layer  string          `json:"layer"`
		} `json:"negatives"`
		Raw []struct {
			Name string `json:"name"`
			Hex  string `json:"raw_hex"`
		} `json:"raw_cases"`
	} `json:"transactions"`
}

func activityReferenceCorpus(t *testing.T) activityReference {
	t.Helper()
	raw, e := os.ReadFile("../../../slugwallet/wallet/testdata/own-activity/core-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "4b3880f7facb9286502aa6a3bdcf38db7821a96e1d1b622f4d27a898a057598e" {
		t.Fatal("independent corpus changed")
	}
	var corpus activityReference
	if e = json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	return corpus
}
func activityReferenceRecord(t *testing.T, raw json.RawMessage) wallet.OwnActivityRecord {
	t.Helper()
	var source struct {
		TxID  string `json:"txid"`
		Raw   string `json:"raw_hex"`
		At    int64  `json:"recorded_at_ms"`
		Block *struct {
			Height uint32 `json:"height"`
			Hash   string `json:"hash"`
			Time   int64  `json:"time_ms"`
		} `json:"block"`
		Credits []wallet.OwnActivityCredit `json:"credits"`
		Debits  []wallet.OwnActivityDebit  `json:"debits"`
	}
	if e := json.Unmarshal(raw, &source); e != nil {
		t.Fatal(e)
	}
	h, e := chainhash.NewHashFromStr(source.TxID)
	if e != nil {
		t.Fatal(e)
	}
	b, e := hex.DecodeString(source.Raw)
	if e != nil {
		t.Fatal(e)
	}
	out := wallet.OwnActivityRecord{TxID: *h, Raw: b, RecordedAtMS: source.At, Credits: source.Credits, Debits: source.Debits}
	if source.Block != nil {
		hash, e := chainhash.NewHashFromStr(source.Block.Hash)
		if e != nil {
			t.Fatal(e)
		}
		out.Block = &wallet.OwnActivityBlock{Height: source.Block.Height, Hash: *hash, TimeMS: source.Block.Time}
	}
	return out
}
func activityReferenceMeta(height uint32) WalletActivityCapture {
	return WalletActivityCapture{Version: 1, CapturedAtMS: 1795121300000, WalletHeight: height, NodeHeight: height, WalletTip: genesis, NodeTip: genesis, Count: 1}
}
func TestWalletActivityIndependentProjection(t *testing.T) {
	corpus := activityReferenceCorpus(t)
	if len(corpus.Transactions.Positives) != 44 || len(corpus.Transactions.Negatives) != 18 || len(corpus.Transactions.Raw) != 18 {
		t.Fatal("reference inventory changed")
	}
	for _, c := range corpus.Transactions.Positives {
		t.Run(c.Name, func(t *testing.T) {
			item, outputs, e := projectWalletActivity(t.Context(), activityReferenceRecord(t, c.Record), activityReferenceMeta(c.Height), true)
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(item, c.Expected.Item) || !reflect.DeepEqual(outputs, c.Expected.Outputs) {
				t.Fatal("projection differs from independent transaction/accounting oracle")
			}
		})
	}
	for _, c := range corpus.Transactions.Negatives {
		t.Run(c.Name, func(t *testing.T) {
			if c.Layer == "fixture-parser-only" {
				t.Skip("fixture JSON spelling is erased before typed wallet DTO; not a wallet semantic claim")
			}
			_, _, e := projectWalletActivity(t.Context(), activityReferenceRecord(t, c.Record), activityReferenceMeta(c.Height), true)
			if e == nil {
				t.Fatal("invalid supplied record accepted")
			}
		})
	}
	for _, c := range corpus.Transactions.Raw {
		t.Run("raw/"+c.Name, func(t *testing.T) {
			raw, e := hex.DecodeString(c.Hex)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = walletActivityTx(t.Context(), raw); e == nil {
				t.Fatal("invalid/over-limit raw transaction accepted")
			}
		})
	}
}
func TestWalletActivityProjectionCancellationAndBounds(t *testing.T) {
	first := activityReferenceCorpus(t).Transactions.Positives[0]
	record := activityReferenceRecord(t, first.Record)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, e := projectWalletActivity(ctx, record, activityReferenceMeta(first.Height), true); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, _, e := projectWalletActivity(nil, record, activityReferenceMeta(first.Height), true); !errors.Is(e, gymstable.ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := walletActivityTx(t.Context(), make([]byte, 262145)); !errors.Is(e, gymstable.ErrCapacity) {
		t.Fatal("raw capacity precedes decoding", e)
	}
	if _, e := buildWalletActivityCache(nil, WalletActivityJob{}, walletActivityRead{}); !errors.Is(e, gymstable.ErrInvalid) {
		t.Fatal(e)
	}
}
func TestWalletActivityRuntimeExactObservation(t *testing.T) {
	for _, mode := range []string{"stable", "same-height-reorg", "wallet-after", "persisted", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			before := localTip{Hash: genesis, Height: 110}
			changed := localTip{Hash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Height: 110}
			tip := func(context.Context) (localTip, error) {
				calls++
				if calls == 2 && mode == "same-height-reorg" {
					return changed, nil
				}
				return before, nil
			}
			local := func(_ context.Context, read bool) (walletActivityRead, localTip, error) {
				if read {
					if mode == "cancel" {
						cancel()
					}
					if mode == "persisted" {
						return walletActivityRead{}, localTip{}, ErrActivityUnavailable
					}
					return walletActivityRead{}, before, nil
				}
				if mode == "wallet-after" {
					return walletActivityRead{}, changed, nil
				}
				return walletActivityRead{}, before, nil
			}
			got, e := captureWalletActivityObserved(ctx, tip, local, func() time.Time { return time.UnixMilli(1795121300000) })
			if mode == "stable" {
				if e != nil || got.at != 1795121300000 {
					t.Fatal(e)
				}
			} else if e == nil {
				t.Fatal("changed or cancelled observation escaped")
			}
		})
	}
}
