// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

type manifestFixtureCase struct {
	Name          string                  `json:"name"`
	Scalar        uint32                  `json:"public_test_scalar"`
	Job           prospectjob.ManifestJob `json:"job"`
	Proof         prospect.Proof          `json:"proof"`
	Authorization gymauth.Authorization   `json:"authorization"`
	Verified      prospect.Verified       `json:"expected_verified"`
	Record        ManifestRecord          `json:"expected_record"`
	Canonical     string                  `json:"record_canonical"`
	Binding       StaticBinding           `json:"static_binding"`
	Eligibility   Eligibility             `json:"eligibility_input"`
	Accepted      int64                   `json:"accepted_at_ms"`
	MeetsBlock    bool                    `json:"meets_block_target"`
}

type manifestFixtureCorpus struct {
	Pin      ManifestAuthorityPin  `json:"pin"`
	Marker   string                `json:"marker_canonical"`
	Metadata string                `json:"metadata_canonical"`
	Info     ManifestAuthorityInfo `json:"expected_info"`
	Random   string                `json:"public_fixture_random_stream_hex"`
	Issued   int64                 `json:"issued_at_ms"`
	Jobs     []struct {
		Job       prospectjob.ManifestJob `json:"job"`
		Canonical string                  `json:"canonical"`
	} `json:"issued_jobs"`
	Cases  []manifestFixtureCase `json:"cases"`
	Inputs map[string][]struct {
		Name     string `json:"name"`
		Input    string `json:"input_hex"`
		Accepted bool   `json:"accepted_encoding"`
	} `json:"storage_inputs"`
	Mutations []struct {
		Name     string         `json:"name"`
		Record   ManifestRecord `json:"record"`
		Accepted bool           `json:"accepted"`
	} `json:"record_mutations"`
	Alternates []struct {
		Name          string                `json:"name"`
		Proof         prospect.Proof        `json:"proof"`
		Authorization gymauth.Authorization `json:"authorization"`
		Sequence      uint64                `json:"expected_original_record_sequence"`
	} `json:"alternate_authorizations"`
	Pins []struct {
		Name    string               `json:"name"`
		Pin     ManifestAuthorityPin `json:"pin"`
		Shape   bool                 `json:"canonical_shape"`
		Matches bool                 `json:"matches_expected_authority"`
	} `json:"pin_cases"`
	TrustLimit struct {
		Record        ManifestRecord `json:"record"`
		Consistent    bool           `json:"passes_local_consistency_checks"`
		Authenticated bool           `json:"authenticates_actual_acceptance"`
	} `json:"trust_limit_case"`
}

func readManifestCorpus(t *testing.T) manifestFixtureCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/manifest/reference/manifest_admission_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "d32a8653c76c643addb3df091ca8d7dfdba66c2486f2ca305e718113f71d5794" {
		t.Fatal("independent manifest admission corpus changed")
	}
	var f manifestFixtureCorpus
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) != 5 || len(f.Jobs) != 2 || len(f.Mutations) != 26 || len(f.Alternates) != 2 {
		t.Fatal("incomplete independent corpus")
	}
	return f
}
