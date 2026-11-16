// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectjob_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

type fixture struct {
	Name               string                `json:"name"`
	TestOnlyPrivateKey string                `json:"test_only_private_key"`
	Job                prospectjob.Job       `json:"job"`
	Context            prospect.Context      `json:"context"`
	Proof              prospect.Proof        `json:"proof"`
	Authorization      gymauth.Authorization `json:"authorization"`
	ExpectedDigest     string                `json:"expected_digest"`
	JobIDPreimage      string                `json:"job_id_preimage"`
	CommitmentScript   string                `json:"commitment_script"`
	ExpectedVerified   prospect.Verified     `json:"expected_verified"`
}

func fixtures(t *testing.T) []fixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/jobs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var records []fixture
	if err := json.Unmarshal(raw, &records); err != nil || len(records) != 2 {
		t.Fatalf("independent job fixtures: %v", err)
	}
	return records
}

func decodeHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestIndependentJobProofAndAuthorization(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.Name, func(t *testing.T) {
			job, err := prospectjob.New(f.Context, f.Job.PublicKey, f.Job.Challenge, f.Job.IssuedAtMS)
			if err != nil || job != f.Job {
				t.Fatalf("job differs from independent byte encoding: got %+v, %v; want %+v", job, err, f.Job)
			}
			if err := prospectjob.Validate(f.Job); err != nil {
				t.Fatalf("independently generated job rejected: %v", err)
			}
			preimageHash := sha256.Sum256(decodeHex(t, f.JobIDPreimage))
			if hex.EncodeToString(preimageHash[:]) != f.Job.ID {
				t.Fatal("independent preimage does not reproduce the expected job ID")
			}
			script, err := prospectjob.CommitmentScript(f.Job)
			if err != nil || hex.EncodeToString(script) != f.CommitmentScript {
				t.Fatalf("job commitment = %x, %v; want independent %s", script, err, f.CommitmentScript)
			}
			digest, err := gymauth.AdmissionDigest(f.Context, f.Proof, f.Authorization.PublicKey)
			if err != nil || hex.EncodeToString(digest[:]) != f.ExpectedDigest {
				t.Fatalf("job admission digest = %x, %v; want independent %s", digest, err, f.ExpectedDigest)
			}
			if err := gymauth.VerifyAuthorization(f.Context, f.Proof, f.Authorization); err != nil {
				t.Fatalf("independent v2 signature rejected: %v", err)
			}
			// Scalars 1 and 2 are public deterministic fixture inputs only.
			private, _ := btcec.PrivKeyFromBytes(decodeHex(t, f.TestOnlyPrivateKey))
			signed, err := gymauth.SignAdmission(f.Context, f.Proof, private)
			if err != nil || signed != f.Authorization {
				t.Fatalf("v2 signature differs from independent RFC6979: %+v, %v", signed, err)
			}
			verified, err := prospect.Verify(f.Context, f.Proof)
			if wire.TestPoW {
				if err == nil || verified != (prospect.Verified{}) {
					t.Fatal("cheap profile emitted production job work")
				}
			} else if err != nil || verified != f.ExpectedVerified {
				t.Fatalf("v2 work or DNA differs from independent oracle: %+v, %v; want %+v", verified, err, f.ExpectedVerified)
			}
		})
	}
}

func TestNewCanonicalizesAndChecksTimeBounds(t *testing.T) {
	f := fixtures(t)[0]
	context := f.Context
	context.Genesis = strings.ToUpper(context.Genesis)
	context.ParentHash = strings.ToUpper(context.ParentHash)
	context.ProspectTarget = strings.ToUpper(context.ProspectTarget)
	job, err := prospectjob.New(context, strings.ToUpper(f.Job.PublicKey), strings.ToUpper(f.Job.Challenge), f.Job.IssuedAtMS)
	if err != nil || job != f.Job {
		t.Fatalf("hex normalization changed immutable job: %+v, %v", job, err)
	}
	for _, issued := range []int64{-1, 0, math.MaxInt64 - prospectjob.LifetimeMS + 1, math.MaxInt64} {
		got, err := prospectjob.New(f.Context, f.Job.PublicKey, f.Job.Challenge, issued)
		if err == nil || got != (prospectjob.Job{}) {
			t.Fatalf("invalid issuance %d produced %+v, %v", issued, got, err)
		}
	}
	for _, issued := range []int64{1, math.MaxInt64 - prospectjob.LifetimeMS} {
		got, err := prospectjob.New(f.Context, f.Job.PublicKey, f.Job.Challenge, issued)
		if err != nil || got.ExpiresAtMS-issued != 300000 || prospectjob.Validate(got) != nil {
			t.Fatalf("valid issuance boundary %d rejected or overflowed: %+v, %v", issued, got, err)
		}
	}
	zeroChallenge, err := prospectjob.New(f.Context, f.Job.PublicKey, strings.Repeat("0", 64), f.Job.IssuedAtMS)
	if err != nil || prospectjob.Validate(zeroChallenge) != nil {
		t.Fatalf("all-zero 32-byte challenge was rejected as a byte value: %v", err)
	}
	for _, challenge := range []string{"", "00", strings.Repeat("z", 64), "0x" + f.Job.Challenge, f.Job.Challenge + "00"} {
		if got, err := prospectjob.New(f.Context, f.Job.PublicKey, challenge, f.Job.IssuedAtMS); err == nil || got != (prospectjob.Job{}) {
			t.Fatal("invalid challenge constructed a job")
		}
	}
}

func TestImmutableFieldsAndCanonicalValue(t *testing.T) {
	f := fixtures(t)[0]
	mutations := []struct {
		name string
		edit func(*prospectjob.Job)
	}{
		{"format version", func(j *prospectjob.Job) { j.Version++ }},
		{"rule version", func(j *prospectjob.Job) { j.RuleVersion++ }},
		{"ID", func(j *prospectjob.Job) { j.ID = strings.Repeat("0", 64) }},
		{"context version", func(j *prospectjob.Job) { j.Context.Version++ }},
		{"network", func(j *prospectjob.Job) { j.Context.Network++ }},
		{"genesis", func(j *prospectjob.Job) { j.Context.Genesis = strings.Repeat("a", 64) }},
		{"DNA version", func(j *prospectjob.Job) { j.Context.DNAVersion++ }},
		{"target", func(j *prospectjob.Job) { j.Context.ProspectTarget = strings.Repeat("e", 64) }},
		{"parent hash", func(j *prospectjob.Job) { j.Context.ParentHash = strings.Repeat("b", 64) }},
		{"parent height", func(j *prospectjob.Job) { j.Context.ParentHeight++ }},
		{"expected bits", func(j *prospectjob.Job) { j.Context.ExpectedBits++ }},
		{"minimum timestamp", func(j *prospectjob.Job) { j.Context.MinTimestamp++ }},
		{"maximum timestamp", func(j *prospectjob.Job) { j.Context.MaxTimestamp++ }},
		{"public key", func(j *prospectjob.Job) { j.PublicKey = fixtures(t)[1].Job.PublicKey }},
		{"gym ID", func(j *prospectjob.Job) { j.GymID = strings.Repeat("c", 64) }},
		{"challenge", func(j *prospectjob.Job) { j.Challenge = strings.Repeat("d", 64) }},
		{"issuance", func(j *prospectjob.Job) { j.IssuedAtMS++ }},
		{"expiry", func(j *prospectjob.Job) { j.ExpiresAtMS++ }},
		{"negative issuance", func(j *prospectjob.Job) { j.IssuedAtMS = -1 }},
		{"uppercase ID", func(j *prospectjob.Job) { j.ID = strings.ToUpper(j.ID) }},
		{"uppercase public key", func(j *prospectjob.Job) { j.PublicKey = strings.ToUpper(j.PublicKey) }},
		{"uppercase challenge", func(j *prospectjob.Job) { j.Challenge = strings.ToUpper(j.Challenge) }},
		{"uppercase context", func(j *prospectjob.Job) { j.Context.Genesis = strings.ToUpper(j.Context.Genesis) }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			job := f.Job
			mutation.edit(&job)
			if err := prospectjob.Validate(job); err == nil {
				t.Fatal("changed immutable or noncanonical job accepted")
			}
			if script, err := prospectjob.CommitmentScript(job); err == nil || script != nil {
				t.Fatal("invalid job produced a commitment")
			}
		})
	}
}

func TestNewIDsBindFullValidContextAndTime(t *testing.T) {
	f := fixtures(t)[0]
	for _, edit := range []func(*prospect.Context){
		func(c *prospect.Context) { c.Network++ },
		func(c *prospect.Context) { c.Genesis = strings.Repeat("a", 64) },
		func(c *prospect.Context) { c.ProspectTarget = strings.Repeat("e", 64) },
		func(c *prospect.Context) { c.ParentHash = strings.Repeat("b", 64) },
		func(c *prospect.Context) { c.ParentHeight++ },
		func(c *prospect.Context) { c.ExpectedBits++ },
		func(c *prospect.Context) { c.MinTimestamp++ },
		func(c *prospect.Context) { c.MaxTimestamp++ },
	} {
		context := f.Context
		edit(&context)
		job, err := prospectjob.New(context, f.Job.PublicKey, f.Job.Challenge, f.Job.IssuedAtMS)
		if err != nil || job.ID == f.Job.ID {
			t.Fatalf("changed valid context did not derive a new ID: %+v, %v", context, err)
		}
	}
	job, err := prospectjob.New(f.Context, f.Job.PublicKey, f.Job.Challenge, f.Job.IssuedAtMS+1)
	if err != nil || job.ID == f.Job.ID || job.ExpiresAtMS != f.Job.ExpiresAtMS+1 {
		t.Fatalf("new issuance did not bind both interval endpoints: %+v, %v", job, err)
	}
}

func TestOldWorkCannotBeReboundToNewJob(t *testing.T) {
	f := fixtures(t)[0]
	job, err := prospectjob.New(f.Context, f.Job.PublicKey, f.Job.Challenge, f.Job.IssuedAtMS+1)
	if err != nil {
		t.Fatal(err)
	}
	proof := f.Proof
	proof.JobID = job.ID
	if err := gymauth.VerifyAuthorization(f.Context, proof, f.Authorization); err == nil {
		t.Fatal("old signature authorized a substituted job ID")
	}
	private, _ := btcec.PrivKeyFromBytes(decodeHex(t, f.TestOnlyPrivateKey))
	authorization, err := gymauth.SignAdmission(f.Context, proof, private)
	if err != nil || gymauth.VerifyAuthorization(f.Context, proof, authorization) != nil {
		t.Fatalf("test owner could not authorize changed job assertion: %v", err)
	}
	// Even the legitimate key holder cannot reuse the old coinbase/header.
	// A new valid signature cannot rewrite the job already committed by work.
	verified, err := prospect.Verify(f.Context, proof)
	if err == nil || verified != (prospect.Verified{}) {
		t.Fatal("new authorization rejuvenated old work")
	}
	if !wire.TestPoW && !strings.Contains(err.Error(), "commitment") {
		t.Fatalf("rebound work did not fail its coinbase commitment: %v", err)
	}
}
