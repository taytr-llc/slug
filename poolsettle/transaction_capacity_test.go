// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

// Independent symbolic serialization fixtures, not signed/funded transactions.
func TestSettlementTransactionCapacityIndependentBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/payout-capacity/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "1353e4856d0fb601c7a7fa9ef99af8cf1ffe3d1ac83f9f23ead28c62d2e5b291" {
		t.Fatal("independent size corpus changed")
	}
	var corpus struct {
		Version uint32 `json:"version"`
		Cases   []struct {
			Inputs     int  `json:"inputs"`
			Payments   int  `json:"payments"`
			Change     bool `json:"change"`
			Allowed    bool `json:"allowed"`
			Dimensions *struct {
				Outputs  int `json:"outputs"`
				Stripped int `json:"stripped_bytes"`
				Witness  int `json:"witness_and_marker_bytes"`
				Raw      int `json:"raw_bytes"`
				VSize    int `json:"vsize"`
			} `json:"dimensions"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil || corpus.Version != 1 || len(corpus.Cases) != 19 {
		t.Fatal("invalid size corpus")
	}
	for _, tc := range corpus.Cases {
		t.Run(fmt.Sprintf("inputs-%d-payments-%d-change-%t", tc.Inputs, tc.Payments, tc.Change), func(t *testing.T) {
			if got := transactionCapacity(tc.Inputs, tc.Payments, tc.Change); got != tc.Allowed {
				t.Fatal("count/raw-cap admission differs from independent contract", got, tc.Allowed)
			}
			if tc.Dimensions == nil {
				return
			}
			// This serialized shape intentionally has no valid signatures and
			// repeated empty outpoints. It is never submitted to a signer/store.
			tx := wire.NewMsgTx(2)
			for i := 0; i < tc.Inputs; i++ {
				tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{}, nil, wire.TxWitness{make([]byte, 73), make([]byte, 33)}))
			}
			for i := 0; i < tc.Dimensions.Outputs; i++ {
				tx.AddTxOut(wire.NewTxOut(100000000, append([]byte{0, 20}, make([]byte, 20)...)))
			}
			if tx.SerializeSizeStripped() != tc.Dimensions.Stripped || tx.SerializeSize() != tc.Dimensions.Raw || tx.SerializeSize()-tx.SerializeSizeStripped() != tc.Dimensions.Witness {
				t.Fatal("native witness framing differs from independent full raw bytes")
			}
			if (3*tx.SerializeSizeStripped()+tx.SerializeSize()+3)/4 != tc.Dimensions.VSize {
				t.Fatal("independent vsize diagnostic differs")
			}
		})
	}
	maxInt := int(^uint(0) >> 1)
	for _, pair := range [][2]int{{maxInt, 1}, {1, maxInt}, {maxInt, maxInt}, {-maxInt, 1}, {1, -maxInt}} {
		for _, change := range []bool{false, true} {
			if transactionCapacity(pair[0], pair[1], change) {
				t.Fatal("integer overflow/invalid count admitted")
			}
		}
	}
}
