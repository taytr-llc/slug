// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectjob

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

func manifestReference(t *testing.T, out any) {
	t.Helper()
	raw, err := os.ReadFile("../fighterdescriptor/testdata/reference/manifest_job_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "34b9484db90fd6d9ca9d704ee1ac76843a717bddaeb38194df1932f77fe754ad" {
		t.Fatal("independent corpus changed")
	}
	if err = json.Unmarshal(raw, out); err != nil {
		t.Fatal(err)
	}
}

func TestIndependentManifestJobCorpus(t *testing.T) {
	var corpus struct {
		Jobs []struct {
			Name       string
			Job        ManifestJob
			Canonical  string
			Preimage   string `json:"job_id_preimage"`
			Commitment string `json:"commitment_script"`
			Supported  bool   `json:"supported_candidate"`
		} `json:"jobs"`
		Inputs []struct {
			Name     string
			Hex      string `json:"input_hex"`
			Accepted bool
			Expected ManifestJob
		} `json:"job_inputs"`
		Mutations []struct {
			Name     string
			Job      ManifestJob
			Accepted bool
		} `json:"mutation_cases"`
		Constructors []struct {
			Name     string
			Input    json.RawMessage
			Accepted bool
			Expected ManifestJob
		} `json:"constructor_cases"`
		Normalize []struct {
			Name, Kind string
			Input      json.RawMessage
			Accepted   bool
			Expected   ManifestJob
		} `json:"normalization_cases"`
		Auth []struct {
			Name          string
			Scalar        uint32 `json:"public_test_scalar"`
			Job           ManifestJob
			Proof         prospect.Proof
			Authorization gymauth.Authorization
			Digest        string `json:"expected_digest"`
			WorkVerified  bool   `json:"work_verified"`
			Issued        bool
		} `json:"authorizations"`
	}
	manifestReference(t, &corpus)
	if len(corpus.Jobs) != 7 || len(corpus.Inputs) != 127 || len(corpus.Mutations) != 20 || len(corpus.Constructors) != 39 || len(corpus.Normalize) != 3 || len(corpus.Auth) != 3 {
		t.Fatal("incomplete independent job corpus")
	}
	for _, tc := range corpus.Jobs {
		t.Run(tc.Name, func(t *testing.T) {
			preimage, err := hex.DecodeString(tc.Preimage)
			if err != nil || len(preimage) != 328 {
				t.Fatal("independent preimage shape", err)
			}
			h := sha256.Sum256(preimage)
			if hex.EncodeToString(h[:]) != tc.Job.ID {
				t.Fatal("independent preimage ID inconsistency")
			}
			if !bytes.Equal(manifestPreimage(tc.Job), preimage) {
				t.Fatal("all 328 preimage bytes differ")
			}
			job, err := NewManifest(tc.Job.Context, tc.Job.AuthorityID, tc.Job.DescriptorID, tc.Job.PublicKey, tc.Job.Challenge, tc.Job.IssuedAtMS)
			if !tc.Supported {
				if err == nil || job != (ManifestJob{}) || ValidateManifest(tc.Job) == nil {
					t.Fatal("counterfactual descriptor operational")
				}
				if raw, err := MarshalManifestJob(tc.Job); err == nil || raw != nil {
					t.Fatal("unsupported job serialized")
				}
				if script, err := ManifestCommitmentScript(tc.Job); err == nil || script != nil {
					t.Fatal("unsupported job commitment approved")
				}
				if parsed, err := ParseManifestJob([]byte(tc.Canonical)); err == nil || parsed != (ManifestJob{}) {
					t.Fatal("unsupported self-consistent transport accepted")
				}
				// The low-level opaque-ID codec is intentionally not issuance or
				// support validation; its mathematical linkage remains unchanged.
				script, err := prospect.CommitmentScriptForJob(tc.Job.Context, tc.Job.GymID, tc.Job.ID)
				if err != nil || hex.EncodeToString(script) != tc.Commitment {
					t.Fatal("opaque commitment byte parity", err)
				}
				return
			}
			if err != nil || job != tc.Job {
				t.Fatal("independent new job mismatch", err)
			}
			raw, err := MarshalManifestJob(job)
			if err != nil || string(raw) != tc.Canonical {
				t.Fatal("canonical job bytes mismatch", err)
			}
			script, err := ManifestCommitmentScript(job)
			if err != nil || hex.EncodeToString(script) != tc.Commitment {
				t.Fatal("independent script mismatch", err)
			}
		})
	}
	for _, tc := range corpus.Inputs {
		t.Run(tc.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.Hex)
			if err != nil {
				t.Fatal(err)
			}
			job, err := ParseManifestJob(raw)
			if tc.Accepted {
				if err != nil || job != tc.Expected {
					t.Fatal("independent transport expected success", err)
				}
			} else if err == nil || job != (ManifestJob{}) {
				t.Fatal("independent transport refusal failed")
			}
		})
	}
	for _, tc := range corpus.Mutations {
		if tc.Accepted || ValidateManifest(tc.Job) == nil {
			t.Fatal("independent immutable mutation accepted", tc.Name)
		}
	}
	construct := func(name string, input json.RawMessage, accepted bool, expected ManifestJob) {
		t.Helper()
		var v struct {
			Context    prospect.Context
			Authority  string `json:"authority_id"`
			Descriptor string `json:"descriptor_id"`
			PublicKey  string `json:"public_key"`
			Challenge  string
			Issued     int64 `json:"issued_at_ms"`
		}
		if err := json.Unmarshal(input, &v); err != nil {
			if accepted {
				t.Fatal(name, err)
			}
			return
		} // Typed numeric overflow never reaches constructor.
		got, err := NewManifest(v.Context, v.Authority, v.Descriptor, v.PublicKey, v.Challenge, v.Issued)
		if accepted {
			if err != nil || got != expected {
				t.Fatal("independent constructor mismatch", name, err)
			}
		} else if err == nil || got != (ManifestJob{}) {
			t.Fatal("independent constructor refusal failed", name)
		}
	}
	for _, tc := range corpus.Constructors {
		construct(tc.Name, tc.Input, tc.Accepted, tc.Expected)
	}
	for _, tc := range corpus.Normalize {
		if tc.Kind == "job_constructor" {
			construct(tc.Name, tc.Input, tc.Accepted, tc.Expected)
		}
	}
	for _, tc := range corpus.Auth {
		t.Run("auth_"+tc.Name, func(t *testing.T) {
			if tc.WorkVerified || tc.Issued {
				t.Fatal("fixture claims unsupported work/issuance")
			}
			if ValidateManifest(tc.Job) != nil || tc.Proof.Version != 2 || tc.Proof.JobID != tc.Job.ID {
				t.Fatal("job/proof link")
			}
			digest, err := gymauth.AdmissionDigest(tc.Job.Context, tc.Proof, tc.Authorization.PublicKey)
			if err != nil || hex.EncodeToString(digest[:]) != tc.Digest {
				t.Fatal("independent signature preimage mismatch", err)
			}
			if err = gymauth.VerifyAuthorization(tc.Job.Context, tc.Proof, tc.Authorization); err != nil {
				t.Fatal(err)
			}
			if tc.Scalar != 1 && tc.Scalar != 2 {
				t.Fatal("expected published test scalars only")
			}
			var keyBytes [32]byte
			binary.BigEndian.PutUint32(keyBytes[28:], tc.Scalar)
			private, _ := btcec.PrivKeyFromBytes(keyBytes[:])
			auth, err := gymauth.SignAdmission(tc.Job.Context, tc.Proof, private)
			private.Zero()
			if err != nil || auth != tc.Authorization {
				t.Fatal("independent public fixture signature mismatch", err)
			}
		})
	}
}
