// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectwork

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestIndependentManifestCandidates(t *testing.T) {
	raw, err := os.ReadFile("../fighterdescriptor/testdata/reference/manifest_job_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "34b9484db90fd6d9ca9d704ee1ac76843a717bddaeb38194df1932f77fe754ad" {
		t.Fatal("independent corpus changed")
	}
	var corpus struct {
		Candidates []struct {
			Name     string
			Job      prospectjob.ManifestJob
			Template Template
			Payout   string `json:"payout_address"`
			Extra    uint64 `json:"extra_nonce"`
			Expected Candidate
			Merkle   string `json:"expected_merkle_root"`
			TxID     string `json:"expected_coinbase_txid"`
			Size     int    `json:"expected_serialized_size"`
			Base     int    `json:"expected_base_size"`
			Script   string `json:"expected_payout_script"`
		} `json:"candidates"`
		Jobs []struct {
			Job       prospectjob.ManifestJob
			Supported bool `json:"supported_candidate"`
		} `json:"jobs"`
	}
	if err = json.Unmarshal(raw, &corpus); err != nil || len(corpus.Candidates) != 3 {
		t.Fatal("independent candidates", err)
	}
	for _, tc := range corpus.Candidates {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := BuildManifest(tc.Job, tc.Template, tc.Payout, tc.Extra)
			if wire.TestPoW {
				if err == nil || !reflect.DeepEqual(got, Candidate{}) {
					t.Fatal("cheap profile emitted production candidate")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.Expected) {
				t.Fatalf("independent complete candidate mismatch (%v)", err)
			}
			assertIndependentBlock(t, got, templateReference{Template: tc.Template, ExpectedMerkleRoot: tc.Merkle, ExpectedCoinbaseTxID: tc.TxID, ExpectedSerializedSize: tc.Size, ExpectedBaseSize: tc.Base, ExpectedPayoutScript: tc.Script})
		})
	}
	for _, tc := range corpus.Jobs {
		if !tc.Supported {
			c := corpus.Candidates[0]
			// Match the counterfactual's target/context. Prove the same inputs
			// would build with the supported descriptor, so this refusal cannot
			// be caused solely by an unrelated context or template mismatch.
			template := c.Template
			template.Context = tc.Job.Context
			template.Block.Bits = fmt.Sprintf("%08x", tc.Job.Context.ExpectedBits)
			template.Block.Target = fmt.Sprintf("%064x", blockchain.CompactToBig(tc.Job.Context.ExpectedBits))
			id, err := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
			if err != nil {
				t.Fatal(err)
			}
			supported, err := prospectjob.NewManifest(tc.Job.Context, tc.Job.AuthorityID, id, tc.Job.PublicKey, tc.Job.Challenge, tc.Job.IssuedAtMS)
			if err != nil {
				t.Fatal(err)
			}
			if !wire.TestPoW {
				if _, err := BuildManifest(supported, template, c.Payout, c.Extra); err != nil {
					t.Fatal("unsupported-descriptor negative control was not otherwise buildable", err)
				}
			}
			got, err := BuildManifest(tc.Job, template, c.Payout, c.Extra)
			if err == nil || !reflect.DeepEqual(got, Candidate{}) {
				t.Fatal("unsupported self-consistent descriptor built")
			}
		}
	}
}
