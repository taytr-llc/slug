// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectwork

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

// These bytes come from reference_templates.py's independent Python encoders,
// ordinary SHA256d Merkle trees, witness commitments and Base58Check. They are
// unmined structural fixtures with synthetic outpoints, never accepted blocks.
type templateReference struct {
	Name                   string          `json:"name"`
	Job                    prospectjob.Job `json:"job"`
	Template               Template        `json:"template"`
	PayoutAddress          string          `json:"payout_address"`
	ExtraNonce             uint64          `json:"extra_nonce"`
	Expected               Candidate       `json:"expected"`
	ExpectedMerkleRoot     string          `json:"expected_merkle_root"`
	ExpectedCoinbaseTxID   string          `json:"expected_coinbase_txid"`
	ExpectedSerializedSize int             `json:"expected_serialized_size"`
	ExpectedBaseSize       int             `json:"expected_base_size"`
	ExpectedPayoutScript   string          `json:"expected_payout_script"`
}

func templateReferences(t *testing.T) []templateReference {
	t.Helper()
	data, err := os.ReadFile("testdata/templates_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []templateReference
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 3 {
		t.Fatalf("want three independently derived template fixtures, got %d", len(fixtures))
	}
	return fixtures
}

func TestIndependentTemplateCandidates(t *testing.T) {
	for _, fixture := range templateReferences(t) {
		t.Run(fixture.Name, func(t *testing.T) {
			encoded, err := json.Marshal(fixture.Template.Block)
			if err != nil {
				t.Fatal(err)
			}
			template, err := ParseTemplate(fixture.Template.Context, encoded)
			if wire.TestPoW {
				if err == nil {
					t.Fatal("cheap PoW profile parsed a production work template")
				}
				if _, err := Build(fixture.Job, fixture.Template, fixture.PayoutAddress, fixture.ExtraNonce); err == nil {
					t.Fatal("cheap PoW profile built a production work candidate")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := prospectjob.Validate(fixture.Job); err != nil {
				t.Fatalf("independently derived public job: %v", err)
			}
			candidate, err := Build(fixture.Job, template, fixture.PayoutAddress, fixture.ExtraNonce)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(candidate, fixture.Expected) {
				got, _ := json.MarshalIndent(candidate, "", "  ")
				want, _ := json.MarshalIndent(fixture.Expected, "", "  ")
				t.Fatalf("independent full candidate mismatch\ngot: %s\nwant: %s", got, want)
			}
			assertIndependentBlock(t, candidate, fixture)
		})
	}
}

func assertIndependentBlock(t *testing.T, candidate Candidate, fixture templateReference) {
	t.Helper()
	encoded, err := hex.DecodeString(candidate.BlockHex)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(encoded)
	var block wire.MsgBlock
	if err := block.Deserialize(reader); err != nil || reader.Len() != 0 {
		t.Fatalf("full block decoding: %v; remaining bytes %d", err, reader.Len())
	}
	if block.SerializeSize() != fixture.ExpectedSerializedSize || block.SerializeSizeStripped() != fixture.ExpectedBaseSize {
		t.Fatalf("serialization sizes: got full/base %d/%d, want %d/%d", block.SerializeSize(), block.SerializeSizeStripped(), fixture.ExpectedSerializedSize, fixture.ExpectedBaseSize)
	}
	if block.Header.MerkleRoot.String() != fixture.ExpectedMerkleRoot {
		t.Fatal("header Merkle root differs from independent tree")
	}
	if block.Transactions[0].TxHash().String() != fixture.ExpectedCoinbaseTxID {
		t.Fatal("coinbase txid differs from independent stripped serialization")
	}
	if hex.EncodeToString(block.Transactions[0].TxOut[0].PkScript) != fixture.ExpectedPayoutScript {
		t.Fatal("reward script differs from independent SLUG address payload")
	}
	if block.Transactions[0].TxOut[0].Value != candidate.CoinbaseValue {
		t.Fatal("full subsidy and fees were not paid to the explicit payout script")
	}
	for _, output := range block.Transactions[0].TxOut[1:] {
		if output.Value != 0 {
			t.Fatal("commitment output diverted coinbase reward")
		}
	}
	var stripped bytes.Buffer
	if err := block.Transactions[0].SerializeNoWitness(&stripped); err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(stripped.Bytes()) != candidate.Proof.Coinbase {
		t.Fatal("proof does not contain exactly the stripped full-block coinbase")
	}
	wrapped := btcutil.NewBlock(&block)
	if err := blockchain.ValidateWitnessCommitment(wrapped); err != nil {
		t.Fatalf("full-block witness commitment: %v", err)
	}
	if blockchain.GetBlockWeight(wrapped) != candidate.Weight {
		t.Fatal("full-block weight differs from independent stripped/full accounting")
	}
	if blockchain.CalcMerkleRoot(wrapped.Transactions(), false) != block.Header.MerkleRoot {
		t.Fatal("full transaction tree does not match header root")
	}
	if len(block.Transactions) != int(candidate.TransactionCount) {
		t.Fatal("full block transaction count differs from candidate")
	}
	for index, tx := range block.Transactions[1:] {
		var full bytes.Buffer
		if err := tx.Serialize(&full); err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(full.Bytes()) != fixture.Template.Block.Transactions[index].Data {
			t.Fatalf("template transaction %d was changed or stripped", index)
		}
	}
}

func TestIndependentCandidateChangesExtraNonceWithoutChangingPayment(t *testing.T) {
	if wire.TestPoW {
		return // Production-profile refusal is asserted by the fixture test.
	}
	fixture := templateReferences(t)[2]
	first, err := Build(fixture.Job, fixture.Template, fixture.PayoutAddress, fixture.ExtraNonce)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(fixture.Job, fixture.Template, fixture.PayoutAddress, fixture.ExtraNonce-1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Proof.Header == second.Proof.Header || first.Proof.Coinbase == second.Proof.Coinbase {
		t.Fatal("extra nonce did not change coinbase txid and header Merkle root")
	}
	if first.CoinbaseValue != second.CoinbaseValue || first.PayoutAddress != second.PayoutAddress || first.JobID != second.JobID {
		t.Fatal("extra nonce changed reward or job binding")
	}
	if !reflect.DeepEqual(first.Proof.MerkleBranch, second.Proof.MerkleBranch) {
		t.Fatal("coinbase extra nonce changed the right-sibling transaction branch")
	}
}
