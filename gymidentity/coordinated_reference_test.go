// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymidentity

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
)

type coordinatedReferenceBinding struct {
	Version       uint32 `json:"version"`
	CoordinatorID string `json:"coordinator_id"`
	AuthorityID   string `json:"authority_id"`
	TenantID      string `json:"tenant_id"`
	ClientID      string `json:"client_id"`
	ConnectionID  string `json:"connection_id"`
}

func (b coordinatedReferenceBinding) binding() CoordinatorBinding {
	return CoordinatorBinding{Version: b.Version, CoordinatorID: b.CoordinatorID,
		AuthorityID: b.AuthorityID, TenantID: b.TenantID, ClientID: b.ClientID, ConnectionID: b.ConnectionID}
}

type coordinatedReferenceExpected struct {
	Login               identityReferenceExpected `json:"login"`
	Reset               identityReferenceExpected `json:"reset"`
	FlowID              string                    `json:"flow_id"`
	OriginalAuthID      string                    `json:"original_auth_id"`
	OriginalFlowID      string                    `json:"original_flow_id"`
	Mode                string                    `json:"mode"`
	ProviderOperationID string                    `json:"provider_operation_id"`
	ResetNonce          string                    `json:"reset_nonce"`
}

type coordinatedReferenceCase struct {
	Name                 string                       `json:"name"`
	Kind                 string                       `json:"kind"`
	NowUnixSeconds       int64                        `json:"now_unix_seconds"`
	Binding              coordinatedReferenceBinding  `json:"binding"`
	ExpectedFlowID       string                       `json:"expected_flow_id"`
	ExpectedEventID      string                       `json:"expected_event_id"`
	ExpectedNonce        string                       `json:"expected_nonce"`
	Token                string                       `json:"token"`
	MathematicallySigned bool                         `json:"mathematically_signed"`
	Accepted             bool                         `json:"verifier_accepted"`
	Expected             coordinatedReferenceExpected `json:"expected"`
	RejectionReason      string                       `json:"rejection_reason"`
}

type coordinatedReferenceFile struct {
	FixtureOnly bool                       `json:"fixture_only"`
	Version     uint32                     `json:"version"`
	Config      json.RawMessage            `json:"config"`
	Cases       []coordinatedReferenceCase `json:"cases"`
}

func coordinatedReferences(t *testing.T) (coordinatedReferenceFile, identityReferenceFile) {
	t.Helper()
	raw, err := os.ReadFile("testdata/coordinated/coordinated_identity_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var file coordinatedReferenceFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if !file.FixtureOnly || file.Version != 1 || len(file.Cases) != 306 {
		t.Fatal("incomplete independent coordinated corpus")
	}
	var config identityReferenceFile
	if err := json.Unmarshal(file.Config, &config.Config); err != nil {
		t.Fatal(err)
	}
	return file, config
}

// This corpus predates the Go coordinated verifier. Python reproduced the
// published RFC signature and all152 prior identity cases before producing
// these306 signed contract cases. Stateful journal scenarios in the same JSON
// are deliberately not counted as executed tests by this verifier bridge.
func TestCoordinatedReferenceCorpus(t *testing.T) {
	file, config := coordinatedReferences(t)
	key := identityReferencePublicKey(t, config)
	v := identityReferenceVerifier(t, config)
	seen := make(map[string]bool)
	accepted := 0
	for _, fixture := range file.Cases {
		if seen[fixture.Name] {
			t.Fatalf("duplicate coordinated fixture name %q", fixture.Name)
		}
		seen[fixture.Name] = true
		if fixture.Accepted {
			accepted++
		}
		t.Run(fixture.Name, func(t *testing.T) {
			if fixture.MathematicallySigned {
				verifyReferenceRS256(t, key, fixture.Token)
			}
			if !fixture.Accepted && fixture.RejectionReason == "" {
				t.Fatal("missing rejection reason")
			}
			v.now = func() time.Time { return time.Unix(fixture.NowUnixSeconds, 0) }
			switch fixture.Kind {
			case "login":
				got, err := v.VerifyCoordinatedLogin(context.Background(), fixture.Token, fixture.Binding.binding(), fixture.ExpectedFlowID, fixture.ExpectedNonce)
				if !fixture.Accepted {
					if err == nil || got != (CoordinatedLogin{}) {
						t.Fatalf("accepted/exposed rejected coordinated login: %s", fixture.RejectionReason)
					}
					return
				}
				want := fixture.Expected.Login
				expected := CoordinatedLogin{Login: gymaccount.VerifiedLogin{
					VerifierConfigHash: v.ConfigDigest(), Issuer: want.Issuer, Subject: want.Subject, Nonce: want.Nonce,
					AuthenticatedAtMS: want.AuthenticatedAtMS, IssuedAtMS: want.IssuedAtMS,
					ExpiresAtMS: want.ExpiresAtMS, CredentialEpoch: want.CredentialEpoch},
					FlowID: fixture.Expected.FlowID, OriginalAuthID: fixture.Expected.OriginalAuthID,
					OriginalFlowID: fixture.Expected.OriginalFlowID, Mode: fixture.Expected.Mode}
				if err != nil || !reflect.DeepEqual(got, expected) {
					t.Fatalf("independent coordinated login differs: error %v; got %+v; want %+v", err, got, expected)
				}
			case "reset":
				got, err := v.VerifyCoordinatedReset(context.Background(), fixture.Token, fixture.Binding.binding(), fixture.ExpectedEventID, fixture.ExpectedNonce)
				if !fixture.Accepted {
					if err == nil || got != (CoordinatedReset{}) {
						t.Fatalf("accepted/exposed rejected coordinated reset: %s", fixture.RejectionReason)
					}
					return
				}
				want := fixture.Expected.Reset
				expected := CoordinatedReset{Reset: gymaccount.VerifiedReset{
					VerifierConfigHash: v.ConfigDigest(), Issuer: want.Issuer, Subject: want.Subject,
					EventID: want.EventID, Stage: want.Stage, FromEpoch: want.FromEpoch, ToEpoch: want.ToEpoch,
					VerifiedAtMS: want.VerifiedAtMS, IssuedAtMS: want.IssuedAtMS, ExpiresAtMS: want.ExpiresAtMS},
					ProviderOperationID: fixture.Expected.ProviderOperationID, ResetNonce: fixture.Expected.ResetNonce}
				if err != nil || !reflect.DeepEqual(got, expected) {
					t.Fatalf("independent coordinated reset differs: error %v; got %+v; want %+v", err, got, expected)
				}
			default:
				t.Fatal("unknown coordinated fixture kind")
			}
		})
	}
	if accepted != 28 {
		t.Fatal("independent accepted-case inventory changed")
	}
}
