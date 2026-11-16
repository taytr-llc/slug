// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/prospectnodetest"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

type jobFixture struct {
	Job           prospectjob.Job       `json:"job"`
	Context       json.RawMessage       `json:"context"`
	Proof         prospect.Proof        `json:"proof"`
	Authorization gymauth.Authorization `json:"authorization"`
	Verified      prospect.Verified     `json:"expected_verified"`
	Digest        string                `json:"expected_digest"`
	Script        string                `json:"commitment_script"`
}

func jobFiles(t *testing.T) (jobFixture, string, string, string, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "prospectjob", "testdata", "jobs_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []jobFixture
	if err := json.Unmarshal(data, &fixtures); err != nil || len(fixtures) < 2 {
		t.Fatalf("read independent job fixtures: %v", err)
	}
	f := fixtures[0]
	dir := t.TempDir()
	jobPath, contextPath := filepath.Join(dir, "job.json"), filepath.Join(dir, "context.json")
	proofPath, authPath := filepath.Join(dir, "proof.json"), filepath.Join(dir, "authorization.json")
	for path, value := range map[string]any{jobPath: f.Job, contextPath: f.Context, proofPath: f.Proof, authPath: f.Authorization} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return f, jobPath, contextPath, proofPath, authPath
}

func TestJobCommitmentAndSigningPayload(t *testing.T) {
	f, jobPath, contextPath, proofPath, _ := jobFiles(t)
	var stdout, stderr bytes.Buffer
	if err := run([]string{"commitment-job", "--job", jobPath}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var result struct {
		ProofVersion uint32           `json:"proof_version"`
		JobID        string           `json:"job_id"`
		Context      prospect.Context `json:"context"`
		GymID        string           `json:"gym_id"`
		Script       string           `json:"script_hex"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ProofVersion != 2 || result.JobID != f.Job.ID || result.Context != f.Job.Context ||
		result.GymID != f.Job.GymID || result.Script != f.Script {
		t.Fatalf("job commitment differs from independent fixture: %+v", result)
	}
	stdout.Reset()
	if err := run([]string{"auth-payload", "--context", contextPath, "--proof", proofPath,
		"--public-key", f.Job.PublicKey}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Version uint32 `json:"version"`
		Digest  string `json:"digest"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil || payload.Version != 2 || payload.Digest != f.Digest {
		t.Fatalf("version-2 signing payload differs: %+v (%v)", payload, err)
	}
}

func TestJobWorkVerificationDoesNotClaimBirth(t *testing.T) {
	f, _, contextPath, proofPath, authPath := jobFiles(t)
	for _, online := range []bool{false, true} {
		args := []string{"verify-auth", "--context", contextPath, "--proof", proofPath, "--authorization", authPath}
		if online {
			node := prospectnodetest.New(t, f.Context)
			args = []string{"verify-auth-node", "--policy", node.PolicyPath, "--rpc-config", node.ConfigPath,
				"--proof", proofPath, "--authorization", authPath}
		}
		var stdout, stderr bytes.Buffer
		err := run(args, &stdout, &stderr)
		if wire.TestPoW {
			if err == nil || stdout.Len() != 0 {
				t.Fatal("cheap profile accepted production job work")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Authentication string            `json:"authentication"`
			BirthVerified  *bool             `json:"birth_verified"`
			Verified       prospect.Verified `json:"verified"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Authentication != "gym-key-control" || result.BirthVerified == nil || *result.BirthVerified ||
			!reflect.DeepEqual(result.Verified, f.Verified) {
			t.Fatal("conditional job verification claimed birth or changed evidence")
		}
	}
}

func TestPlainJobVerificationDoesNotClaimBirth(t *testing.T) {
	f, _, contextPath, proofPath, _ := jobFiles(t)
	var stdout, stderr bytes.Buffer
	err := run([]string{"verify", "--context", contextPath, "--proof", proofPath}, &stdout, &stderr)
	if wire.TestPoW {
		if err == nil || stdout.Len() != 0 {
			t.Fatal("cheap profile accepted production job work")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		prospect.Verified
		BirthVerified *bool `json:"birth_verified"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.BirthVerified == nil || *result.BirthVerified || !reflect.DeepEqual(result.Verified, f.Verified) {
		t.Fatal("plain version-2 verification claimed birth or changed its flat verified fields")
	}

	// The established version-1 output remains the exact bare Verified object.
	legacy, contextPath, proofPath := fixtureFiles(t)
	stdout.Reset()
	if err := run([]string{"verify", "--context", contextPath, "--proof", proofPath}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var expected bytes.Buffer
	encoder := json.NewEncoder(&expected)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(legacy.Expected); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stdout.Bytes(), expected.Bytes()) {
		t.Fatal("version-1 verification output changed")
	}
}

func TestJobCommandInputBoundary(t *testing.T) {
	_, jobPath, _, _, _ := jobFiles(t)
	for _, data := range []string{"null", "{}", strings.Repeat(" ", prospectjob.MaxJobSize+1)} {
		if err := os.WriteFile(jobPath, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if err := run([]string{"commitment-job", "--job", jobPath}, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatal("malformed job produced a commitment")
		}
	}
	for _, args := range [][]string{{"commitment-job"}, {"commitment-job", "--job", jobPath, "extra"},
		{"commitment-job", "--job", jobPath, "--now", "0"}, {"commitment-job", "--job", jobPath, "--lifetime", "3600"}} {
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatal("invalid job arguments accepted")
		}
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"commitment-job", "-h"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) ||
		!strings.Contains(stderr.String(), "usage: slugprospect commitment-job") {
		t.Fatalf("job help failed: %v", err)
	}
}
