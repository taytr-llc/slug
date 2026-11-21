// Copyright (c) 2026 The SLUG developers. ISC license.
package pooltxshape

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

// Root fills this only from the approved independently reproduced artifact.
// Empty deliberately fails; no generated product output or skip is accepted.
const nativeReferenceSHA256 = "482c9e42ecb154cc0e38fad962cc5df4e08705f933e13cc7f571f0d5bf75ff33"

func TestNativeP2WPKHIndependentReference(t *testing.T) {
	if len(nativeReferenceSHA256) != 64 {
		t.Fatal("independent producer artifact has not been frozen")
	}
	raw, err := os.ReadFile("testdata/native84-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != nativeReferenceSHA256 {
		t.Fatal("independent shape bytes changed")
	}
	var ref struct {
		Version     int    `json:"version"`
		InputSHA    string `json:"input_sha256"`
		ContractSHA string `json:"contract_sha256"`
		Cases       []struct {
			Name       string `json:"name"`
			Inputs     int    `json:"inputs"`
			Payees     int    `json:"payees"`
			Change     bool   `json:"change"`
			Result     string `json:"result"`
			Dimensions *struct {
				Stripped int64 `json:"stripped_bytes"`
				Witness  int64 `json:"witness_bytes"`
				Raw      int64 `json:"raw_bytes"`
				Weight   int64 `json:"weight"`
				VSize    int64 `json:"vsize"`
			} `json:"dimensions"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &ref) != nil || ref.Version != 1 || len(ref.Cases) != 26 || ref.InputSHA != "eb270e0b4cc22ecc5b3b6c768e58611da9588b301cdf0075e8bc1b80cb404715" || ref.ContractSHA != "671997100f7dfe07e726249e0121bafad3fbd095e81c289ed196b200617df16b" {
		t.Fatal("wrong independent input contract")
	}
	for _, tc := range ref.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := NativeP2WPKH(tc.Inputs, tc.Payees, tc.Change)
			switch tc.Result {
			case "invalid":
				if !errors.Is(err, ErrInvalid) || got != (Shape{}) {
					t.Fatal("invalid count accepted or partial shape returned", got, err)
				}
			case "capacity":
				if !errors.Is(err, ErrCapacity) || got != (Shape{}) {
					t.Fatal("oversized shape accepted or partial shape returned", got, err)
				}
			case "ok":
				if tc.Dimensions == nil {
					t.Fatal("missing independent dimensions")
				}
				d := tc.Dimensions
				want := Shape{StrippedBytes: d.Stripped, WitnessBytes: d.Witness, RawBytes: d.Raw, Weight: d.Weight, VSize: d.VSize}
				if err != nil || got != want {
					t.Fatal("shape differs from independent serialization", got, want, err)
				}
			default:
				t.Fatal("unknown reference result")
			}
			if tc.Dimensions == nil {
				return
			}
			// Symbolic serialization only: repeated outpoints and placeholder
			// witnesses are not signatures, spend authority or ledger fixtures.
			tx := wire.NewMsgTx(2)
			for i := 0; i < tc.Inputs; i++ {
				tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{}, nil, wire.TxWitness{make([]byte, 73), make([]byte, 33)}))
			}
			outputs := tc.Payees
			if tc.Change {
				outputs++
			}
			for i := 0; i < outputs; i++ {
				tx.AddTxOut(wire.NewTxOut(100000000, append([]byte{0, 20}, make([]byte, 20)...)))
			}
			if int64(tx.SerializeSizeStripped()) != tc.Dimensions.Stripped || int64(tx.SerializeSize()) != tc.Dimensions.Raw {
				t.Fatal("wire serializer contradicts independently sized shape")
			}
		})
	}
}

func TestNativeP2WPKHRejectsMachineIntExtremes(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	for _, pair := range [][2]int{{maxInt, 1}, {1, maxInt}, {maxInt, maxInt}, {minInt, 1}, {1, minInt}, {minInt, minInt}} {
		for _, change := range []bool{false, true} {
			if shape, err := NativeP2WPKH(pair[0], pair[1], change); !errors.Is(err, ErrInvalid) || shape != (Shape{}) {
				t.Fatal("unbounded integer escaped validation", shape, err)
			}
		}
	}
}
