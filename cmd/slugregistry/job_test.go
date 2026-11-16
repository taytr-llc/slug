// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/prospectnodetest"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type jobFixture struct {
	Job           prospectjob.Job       `json:"job"`
	Context       json.RawMessage       `json:"context"`
	Proof         prospect.Proof        `json:"proof"`
	Authorization gymauth.Authorization `json:"authorization"`
}

func jobReference(t *testing.T) jobFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "prospectjob", "testdata", "jobs_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []jobFixture
	if err := json.Unmarshal(data, &fixtures); err != nil || len(fixtures) < 2 {
		t.Fatalf("read independent job fixtures: %v", err)
	}
	return fixtures[0]
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func initializeBirthRegistry(t *testing.T, f jobFixture) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "birth-registry")
	args := []string{"init-birth", "--db", path, "--network", fmt.Sprint(f.Job.Context.Network), "--genesis", f.Job.Context.Genesis}
	if wire.TestPoW {
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatal("cheap profile initialized a birth authority")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("refused cheap-profile initialization created a registry directory")
		}
		return path
	}
	invoke(t, args...)
	return path
}

func issueArguments(path string, node *prospectnodetest.Node, publicKey string) []string {
	return []string{"issue-job-node", "--db", path, "--policy", node.PolicyPath,
		"--rpc-config", node.ConfigPath, "--public-key", publicKey}
}

func issueTestJob(t *testing.T, path string, node *prospectnodetest.Node, publicKey string) prospectjob.Job {
	t.Helper()
	before := time.Now().UnixMilli()
	output := invoke(t, issueArguments(path, node, publicKey)...)
	after := time.Now().UnixMilli()
	var job prospectjob.Job
	if err := json.Unmarshal([]byte(output), &job); err != nil {
		t.Fatal(err)
	}
	if job.Version != 1 || job.RuleVersion != 1 || job.IssuedAtMS < before || job.IssuedAtMS > after ||
		job.ExpiresAtMS-job.IssuedAtMS != prospectjob.LifetimeMS || job.Context != node.Context {
		t.Fatalf("issuance did not use the fixed rule and current clock: %+v", job)
	}
	var stored prospectjob.Job
	if err := json.Unmarshal([]byte(invoke(t, "get-job", "--db", path, "--id", job.ID)), &stored); err != nil ||
		!reflect.DeepEqual(stored, job) {
		t.Fatalf("issued job did not survive reopen: %v", err)
	}
	return job
}

// This exercises the live workflow, not a cryptographic golden. A current
// issued nonce replaces the independent fixture's commitment. The all-ff
// prospect target needs no grinding; key 1 is a public test-only fixture.
func liveJobProof(t *testing.T, f jobFixture, job prospectjob.Job) (string, string) {
	t.Helper()
	proof := f.Proof
	if len(proof.MerkleBranch) != 0 {
		t.Fatal("live-job test requires the coinbase-only fixture")
	}
	coinbase, err := hex.DecodeString(proof.Coinbase)
	if err != nil {
		t.Fatal(err)
	}
	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(coinbase)); err != nil {
		t.Fatal(err)
	}
	script, err := prospectjob.CommitmentScript(job)
	if err != nil {
		t.Fatal(err)
	}
	replaced := 0
	for _, output := range tx.TxOut {
		if bytes.Contains(output.PkScript, []byte("SLUGPR02")) {
			output.PkScript = script
			replaced++
		}
	}
	if replaced != 1 {
		t.Fatal("fixture must contain one job commitment")
	}
	var encoded bytes.Buffer
	if err := tx.Serialize(&encoded); err != nil {
		t.Fatal(err)
	}
	proof.Coinbase = hex.EncodeToString(encoded.Bytes())
	headerBytes, err := hex.DecodeString(proof.Header)
	if err != nil {
		t.Fatal(err)
	}
	var header wire.BlockHeader
	if err := header.Deserialize(bytes.NewReader(headerBytes)); err != nil {
		t.Fatal(err)
	}
	header.MerkleRoot = tx.TxHash()
	encoded.Reset()
	if err := header.Serialize(&encoded); err != nil {
		t.Fatal(err)
	}
	proof.Header = hex.EncodeToString(encoded.Bytes())
	proof.JobID, proof.GymID = job.ID, job.GymID
	var scalar [32]byte
	scalar[31] = 1 // Public fixture key only; never an operator or wallet key.
	key, publicKey := btcec.PrivKeyFromBytes(scalar[:])
	if hex.EncodeToString(publicKey.SerializeCompressed()) != job.PublicKey {
		t.Fatal("fixture does not use public test key 1")
	}
	authorization, err := gymauth.SignAdmission(job.Context, proof, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	proofPath, authPath := filepath.Join(dir, "proof.json"), filepath.Join(dir, "authorization.json")
	writeTestJSON(t, proofPath, proof)
	writeTestJSON(t, authPath, authorization)
	return proofPath, authPath
}

func recordJobArguments(path string, node *prospectnodetest.Node, proofPath, authPath string) []string {
	return []string{"record-job-node", "--db", path, "--policy", node.PolicyPath,
		"--rpc-config", node.ConfigPath, "--proof", proofPath, "--authorization", authPath}
}

type jobRecordOutput struct {
	Source             string          `json:"context_source"`
	CurrentNodeChecked bool            `json:"current_node_checked"`
	Authentication     string          `json:"authentication"`
	BirthRule          string          `json:"birth_rule"`
	Inserted           bool            `json:"inserted"`
	Record             registry.Record `json:"record"`
}

func TestJobBirthAndHistoricalReplay(t *testing.T) {
	for _, outputFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("output_failure_%t", outputFailure), func(t *testing.T) {
			f := jobReference(t)
			path := initializeBirthRegistry(t, f)
			node := prospectnodetest.New(t, f.Context)
			if wire.TestPoW {
				var stdout, stderr bytes.Buffer
				err := run(issueArguments(path, node, f.Job.PublicKey), &stdout, &stderr)
				if err == nil || stdout.Len() != 0 || node.Requests() != 0 {
					t.Fatal("cheap profile issued a production mining job")
				}
				return
			}
			job := issueTestJob(t, path, node, f.Job.PublicKey)
			proofPath, authPath := liveJobProof(t, f, job)
			args := recordJobArguments(path, node, proofPath, authPath)
			var first jobRecordOutput
			if outputFailure {
				want := errors.New("output unavailable")
				if err := run(args, failedOutput{want}, io.Discard); !errors.Is(err, want) {
					t.Fatalf("output error = %v, want %v", err, want)
				}
			} else {
				if err := json.Unmarshal([]byte(invoke(t, args...)), &first); err != nil {
					t.Fatal(err)
				}
				if !first.Inserted || !first.CurrentNodeChecked || first.Source != "trusted-node-snapshot" {
					t.Fatal("new admission did not describe its current-node check")
				}
			}
			// The new operator target and unavailable node must not renew or
			// prevent retrieval of a receipt that was already committed.
			var policy prospectnodePolicy
			policy.Version, policy.Network, policy.Genesis = 1, job.Context.Network, job.Context.Genesis
			policy.DNAVersion, policy.Target = job.Context.DNAVersion, strings.Repeat("0", 63)+"1"
			writeTestJSON(t, node.PolicyPath, policy)
			writeTestJSON(t, node.ConfigPath, map[string]any{"version": 1, "url": "http://127.0.0.1:1",
				"username": "fixture", "password": "fixture-only", "ca_certificate": ""})
			before := node.Requests()
			var historical jobRecordOutput
			if err := json.Unmarshal([]byte(invoke(t, args...)), &historical); err != nil {
				t.Fatal(err)
			}
			if historical.Inserted || historical.CurrentNodeChecked || historical.Source != "historical-receipt" ||
				historical.Authentication != "gym-key-control" || historical.BirthRule != "job-issuance" ||
				historical.Record.Version != 3 || historical.Record.Sequence != 1 || historical.Record.Job == nil ||
				historical.Record.Birth == nil || historical.Record.Birth.RuleVersion != 1 ||
				historical.Record.Birth.BornAtMS != job.IssuedAtMS || historical.Record.Birth.AcceptedAtMS < job.IssuedAtMS ||
				historical.Record.Birth.AcceptedAtMS >= job.ExpiresAtMS || !reflect.DeepEqual(*historical.Record.Job, job) ||
				node.Requests() != before {
				t.Fatalf("historical retry altered or misrepresented birth: %+v", historical)
			}
			if !outputFailure && !reflect.DeepEqual(first.Record, historical.Record) {
				t.Fatal("historical retry changed first evidence or birth")
			}
		})
	}
}

type prospectnodePolicy struct {
	Version    uint32 `json:"version"`
	Network    uint32 `json:"network"`
	Genesis    string `json:"genesis"`
	DNAVersion uint32 `json:"dna_version"`
	Target     string `json:"prospect_target"`
}

func TestJobAdmissionRejectsReorgAndUnknownJob(t *testing.T) {
	f := jobReference(t)
	path := initializeBirthRegistry(t, f)
	node := prospectnodetest.New(t, f.Context)
	if wire.TestPoW {
		return // The production-profile refusal is asserted in the workflow test.
	}
	job := issueTestJob(t, path, node, f.Job.PublicKey)
	proofPath, authPath := liveJobProof(t, f, job)
	policyBytes, err := os.ReadFile(node.PolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, node.PolicyPath, prospectnodePolicy{1, job.Context.Network, job.Context.Genesis,
		job.Context.DNAVersion, strings.Repeat("0", 63) + "1"})
	before := node.Requests()
	var stdout, stderr bytes.Buffer
	err = run(recordJobArguments(path, node, proofPath, authPath), &stdout, &stderr)
	if err == nil || stdout.Len() != 0 || node.Requests() != before || invoke(t, "list", "--db", path) != "[]\n" {
		t.Fatal("new admission ignored a changed prospect policy")
	}
	if err := os.WriteFile(node.PolicyPath, policyBytes, 0600); err != nil {
		t.Fatal(err)
	}
	node.ReorgAfter(node.Snapshots())
	err = run(recordJobArguments(path, node, proofPath, authPath), &stdout, &stderr)
	if err == nil || stdout.Len() != 0 || invoke(t, "list", "--db", path) != "[]\n" {
		t.Fatal("reorg guard admitted a new birth")
	}
	writeTestJSON(t, proofPath, f.Proof)
	writeTestJSON(t, authPath, f.Authorization)
	before = node.Requests()
	err = run(recordJobArguments(path, node, proofPath, authPath), &stdout, &stderr)
	if err == nil || stdout.Len() != 0 || node.Requests() != before || invoke(t, "list", "--db", path) != "[]\n" {
		t.Fatal("a job not issued by this authority admitted a birth")
	}
}

func TestJobCommandArgumentsAndHelp(t *testing.T) {
	for _, command := range []string{"init-birth", "issue-job-node", "get-job", "record-job-node"} {
		for _, args := range [][]string{{command}, {command, "--now", "0"}, {command, "--lifetime", "3600"},
			{command, "--issued-at-ms", "1"}} {
			var stdout, stderr bytes.Buffer
			if err := run(args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
				t.Fatalf("invalid job command accepted: %v", args)
			}
		}
		var stdout, stderr bytes.Buffer
		if err := run([]string{command, "-h"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) ||
			!strings.Contains(stderr.String(), "usage: slugregistry "+command) {
			t.Fatalf("job help failed: %v", err)
		}
	}
}
