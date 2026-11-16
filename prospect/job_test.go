// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospect

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestVersionedProofJSON(t *testing.T) {
	legacy := proofJSON(t)
	parsed, err := ParseProof(legacy)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := json.Marshal(parsed)
	if err != nil || !bytes.Equal(roundTrip, legacy) {
		t.Fatal("version-1 serialized proof changed")
	}
	for _, value := range []json.RawMessage{json.RawMessage(`""`), json.RawMessage(`"` + strings.Repeat("0", 64) + `"`), json.RawMessage(`null`), json.RawMessage(`1`)} {
		if _, err := ParseProof(replaceJSONField(t, legacy, "job_id", value)); err == nil {
			t.Fatal("version1 accepted an explicitly present job_id")
		}
	}
	v2 := replaceJSONField(t, legacy, "version", json.RawMessage(`2`))
	for _, value := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`""`), json.RawMessage(`"0"`), json.RawMessage(`"0x` + strings.Repeat("0", 64) + `"`), json.RawMessage(`"` + strings.Repeat("g", 64) + `"`), json.RawMessage(`[]`)} {
		if _, err := ParseProof(replaceJSONField(t, v2, "job_id", value)); err == nil {
			t.Fatal("version2 accepted missing or malformed job_id")
		}
	}
	for _, job := range []string{strings.Repeat("0", 64), strings.Repeat("A", 64)} {
		valid := replaceJSONField(t, v2, "job_id", json.RawMessage(`"`+job+`"`))
		got, err := ParseProof(valid)
		if err != nil || got.Version != Version2 || got.JobID != job {
			t.Fatalf("valid v2 job rejected: %v", err)
		}
		duplicate := append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"\u006aob_id":"`+job+`"}`)...)
		if _, err := ParseProof(duplicate); err == nil {
			t.Fatal("accepted escaped duplicate job_id")
		}
	}
	if _, err := ParseProof(replaceJSONField(t, v2, "version", json.RawMessage(`3`))); err == nil {
		t.Fatal("accepted unknown proof version")
	}
}

func TestProofVersionJobRulesForDirectCallers(t *testing.T) {
	for _, proof := range []Proof{{Version: Version1, JobID: strings.Repeat("0", 64)}, {Version: Version2}, {Version: Version2, JobID: "0"}, {Version: 3}} {
		if _, err := proofJob(proof); err == nil {
			t.Fatal("accepted invalid direct proof/version combination")
		}
	}
	if job, err := proofJob(Proof{Version: Version2, JobID: strings.Repeat("0", 64)}); err != nil || job != ([32]byte{}) {
		t.Fatal("zero SHA256 job ID rejected")
	}
}

func TestCommitmentFamilySeparatesProofVersions(t *testing.T) {
	raw, err := os.ReadFile("testdata/proofs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Context Context `json:"context"`
		Proof   Proof   `json:"proof"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	f := fixtures[0]
	legacy, err := CommitmentScript(f.Context, f.Proof.GymID)
	if err != nil {
		t.Fatal(err)
	}
	jobScript, err := CommitmentScriptForJob(f.Context, f.Proof.GymID, strings.Repeat("0", 64))
	if err != nil {
		t.Fatal(err)
	}
	if len(jobScript) != 42 || !bytes.Equal(jobScript[:10], []byte{0x6a, 40, 'S', 'L', 'U', 'G', 'P', 'R', '0', '2'}) || bytes.Equal(jobScript, legacy) {
		t.Fatal("v2 reserved marker or separation is incorrect")
	}
	for _, job := range []string{"", "00", "0x" + strings.Repeat("0", 64), strings.Repeat("z", 64)} {
		if _, err := CommitmentScriptForJob(f.Context, f.Proof.GymID, job); err == nil {
			t.Fatal("accepted malformed job identifier")
		}
	}
	coinbase, err := hex.DecodeString(f.Proof.Coinbase)
	if err != nil {
		t.Fatal(err)
	}
	var tx wire.MsgTx
	if err := tx.DeserializeNoWitness(bytes.NewReader(coinbase)); err != nil {
		t.Fatal(err)
	}
	if _, err := validateCoinbase(coinbase, jobScript, f.Context.ParentHeight+1); err == nil {
		t.Fatal("v1 commitment accepted as v2")
	}
	for _, output := range tx.TxOut {
		if bytes.Equal(output.PkScript, legacy) {
			output.PkScript = jobScript
		}
	}
	serialize := func() []byte {
		t.Helper()
		var output bytes.Buffer
		if err := tx.SerializeNoWitness(&output); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	if _, err := validateCoinbase(serialize(), jobScript, f.Context.ParentHeight+1); err != nil {
		t.Fatal(err)
	}
	if _, err := validateCoinbase(serialize(), legacy, f.Context.ParentHeight+1); err == nil {
		t.Fatal("v2 commitment accepted as v1")
	}
	tx.TxOut = append(tx.TxOut, wire.NewTxOut(0, legacy))
	if _, err := validateCoinbase(serialize(), jobScript, f.Context.ParentHeight+1); err == nil {
		t.Fatal("mixed v1/v2 duplicate commitment accepted")
	}
	// A nonminimal push remains in the reserved family and cannot hide an
	// otherwise valid old-version commitment beside the new one.
	tx.TxOut[len(tx.TxOut)-1].PkScript = append([]byte{0x6a, 0x4c, 40}, legacy[2:]...)
	if _, err := validateCoinbase(serialize(), jobScript, f.Context.ParentHeight+1); err == nil {
		t.Fatal("nonminimal mixed-family commitment accepted")
	}
}
