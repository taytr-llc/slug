package pilot

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"
)

type payoutReferenceSelection struct {
	TxID            string `json:"txid"`
	OutputIndex     uint32 `json:"output_index"`
	Atoms           int64  `json:"atoms"`
	RecipientScript string `json:"recipient_script"`
}

func (s payoutReferenceSelection) selected() walletPayoutSelection {
	return walletPayoutSelection{s.TxID, s.OutputIndex, s.Atoms, s.RecipientScript}
}

type payoutReferenceObservation struct {
	State         string `json:"state"`
	ObservedAtMS  int64  `json:"observed_at_ms"`
	WalletHeight  uint32 `json:"wallet_height"`
	WalletTip     string `json:"wallet_tip"`
	Confirmations uint32 `json:"confirmations"`
	BlockHash     string `json:"block_hash"`
}
type payoutReferenceSample struct {
	Synced bool   `json:"synced"`
	Height int32  `json:"height"`
	Hash   string `json:"hash"`
}

func (s payoutReferenceSample) tip() walletPayoutTip {
	return walletPayoutTip{s.Synced, s.Height, s.Hash}
}

type payoutReferenceCorpus struct {
	Transactions []struct {
		Name      string                   `json:"name"`
		RawHex    string                   `json:"raw_hex"`
		Selection payoutReferenceSelection `json:"selection"`
		Valid     bool                     `json:"valid"`
		ParseOnly bool                     `json:"parse_only"`
		Expected  *struct {
			TxID    string `json:"txid"`
			Inputs  uint64 `json:"input_count"`
			Outputs uint64 `json:"output_count"`
			Witness bool   `json:"has_witness"`
		} `json:"expected"`
	} `json:"transactions"`
	WalletRPC []struct {
		Name         string                      `json:"name"`
		RawHex       string                      `json:"raw_hex"`
		Selection    payoutReferenceSelection    `json:"selection"`
		Before       payoutReferenceSample       `json:"before"`
		After        payoutReferenceSample       `json:"after"`
		ObservedAtMS int64                       `json:"observed_at_ms"`
		RequestID    uint64                      `json:"request_id"`
		Valid        bool                        `json:"valid"`
		Expected     *payoutReferenceObservation `json:"expected"`
	} `json:"wallet_rpc"`
}

func readPayoutReference(t *testing.T) payoutReferenceCorpus {
	t.Helper()
	raw, err := os.ReadFile("../../poolsettle/testdata/payout-history/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus payoutReferenceCorpus
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Transactions) != 36 || len(corpus.WalletRPC) != 37 {
		t.Fatal("independent case inventory changed")
	}
	return corpus
}

func TestWalletPayoutIndependentTransactions(t *testing.T) {
	for _, tc := range readPayoutReference(t).Transactions {
		t.Run(tc.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.RawHex)
			if err != nil {
				t.Fatal(err)
			}
			selected := tc.Selection.selected()
			var accepted bool
			if tc.ParseOnly {
				_, accepted = parseWalletPayoutTransaction(raw, 0)
			} else {
				accepted = validateWalletPayoutTransaction(raw, selected)
			}
			if accepted != tc.Valid {
				t.Fatalf("accept=%v want%v", accepted, tc.Valid)
			}
			if tc.Valid {
				index := selected.OutputIndex
				if tc.ParseOnly {
					index = 0
				}
				parsed, ok := parseWalletPayoutTransaction(raw, index)
				if !ok || tc.Expected == nil || parsed.txid != tc.Expected.TxID || parsed.inputs != tc.Expected.Inputs || parsed.outputs != tc.Expected.Outputs || parsed.witness != tc.Expected.Witness {
					t.Fatal("independent parsed identity mismatch")
				}
			}
		})
	}
}

func TestWalletPayoutIndependentRPCObservations(t *testing.T) {
	for _, tc := range readPayoutReference(t).WalletRPC {
		t.Run(tc.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.RawHex)
			if err != nil {
				t.Fatal(err)
			}
			samples, calls := 0, 0
			sample := func() walletPayoutTip {
				samples++
				if samples == 1 {
					return tc.Before.tip()
				}
				return tc.After.tip()
			}
			query := func(context.Context, string) (walletPayoutTransaction, bool, error) {
				calls++
				tx, missing, ok := decodeWalletPayoutRPC(raw, tc.RequestID)
				if !ok {
					return walletPayoutTransaction{}, false, errWalletPayoutUnavailable
				}
				return tx, missing, nil
			}
			got, err := observeWalletPayout(context.Background(), tc.Selection.selected(), sample, query, func() time.Time { return time.UnixMilli(tc.ObservedAtMS) })
			if (err == nil) != tc.Valid {
				t.Fatalf("accepted=%v want%v err%v", err == nil, tc.Valid, err)
			}
			if calls > 1 || samples > 2 {
				t.Fatal("unbounded wallet resample")
			}
			if !tc.Valid {
				if got != (walletPayoutRead{}) {
					t.Fatal("partial disclosure")
				}
				return
			}
			if tc.Expected == nil {
				t.Fatal("missing independently derived result")
			}
			want := tc.Expected
			if got.State != want.State || got.ObservedAtMS != want.ObservedAtMS || got.WalletHeight != want.WalletHeight || got.WalletTip != want.WalletTip || got.Confirmations != want.Confirmations || got.BlockHash != want.BlockHash {
				t.Fatalf("observed%+v want%+v", got, want)
			}
		})
	}
}
