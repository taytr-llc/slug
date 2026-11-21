// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func issueReference(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("../testdata/issue-recovery/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if len(raw) != 6134980 || hex.EncodeToString(h[:]) != "4bbe4eb9556ed5a894cee6ff565b0dcca805f27fea2d77cfe3a56242dc3dac91" {
		t.Fatal("independent issue corpus changed")
	}
	var out map[string]json.RawMessage
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("reference JSON")
	}
	return out
}
func TestRecoverableIssueIndependentProfiles(t *testing.T) {
	x := issueReference(t)
	var f struct {
		Profiles []struct {
			Name      string
			Profile   IssueProfile
			ProfileID string `json:"profile_id"`
			Canonical string `json:"canonical_hex"`
		}
		Cases []struct {
			Name    string
			Valid   bool
			Value   json.RawMessage
			Refusal string
		} `json:"profile_cases"`
	}
	if json.Unmarshal(x["fixtures"], &f) != nil || len(f.Profiles) == 0 || len(f.Cases) == 0 {
		t.Fatal("reference families")
	}
	for _, v := range f.Profiles {
		raw, _ := json.Marshal(v.Profile)
		id, err := IssueProfileID(v.Profile)
		if err != nil || id != v.ProfileID || hex.EncodeToString(raw) != v.Canonical {
			t.Fatal(v.Name, id, err)
		}
	}
	for _, v := range f.Cases {
		// Full source/setup selection belongs to the explicit outer constructors.
		if v.Refusal == "source-count" || v.Refusal == "source-config" || v.Refusal == "setup-selection" {
			continue
		}
		var p IssueProfile
		err := json.Unmarshal(v.Value, &p)
		if err == nil {
			err = ValidateIssueProfile(p)
		}
		if (err == nil) != v.Valid {
			t.Fatal(v.Name, err)
		}
	}
}
func TestRecoverableIssueIndependentIntentsAndBounds(t *testing.T) {
	x := issueReference(t)
	var states struct {
		Pairs []struct {
			Name  string
			Value struct {
				Requests []struct {
					Intent   IssueIntent
					IntentID string `json:"intent_id"`
				} `json:"source_requests"`
			}
		} `json:"state_pairs"`
	}
	if json.Unmarshal(x["states"], &states) != nil {
		t.Fatal("states")
	}
	seen := 0
	for _, v := range states.Pairs {
		for _, r := range v.Value.Requests {
			id, err := IssueIntentID(r.Intent)
			if err != nil || id != r.IntentID {
				t.Fatal(v.Name, id, err)
			}
			seen++
		}
	}
	if seen == 0 {
		t.Fatal("missing independent intent cases")
	}
	if err := ValidateRecoverableIssueCapacity(PublicOwnershipDefaultLimits(), 8192); err != nil {
		t.Fatal("maximum conservative preflight", err)
	}
	for _, n := range []uint64{0, 8193, ^uint64(0)} {
		if ValidateRecoverableIssueCapacity(PublicOwnershipDefaultLimits(), n) == nil {
			t.Fatal("invalid count", n)
		}
	}
	l := PublicOwnershipDefaultLimits()
	l.LogicalBytes--
	if ValidateRecoverableIssueCapacity(l, 8) == nil {
		t.Fatal("changed signed limits")
	}
}
