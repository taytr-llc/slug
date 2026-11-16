// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymidentity

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
)

type identityReferenceExpected struct {
	Issuer            string `json:"issuer"`
	Subject           string `json:"subject"`
	Nonce             string `json:"nonce"`
	AuthenticatedAtMS int64  `json:"authenticated_at_ms"`
	IssuedAtMS        int64  `json:"issued_at_ms"`
	ExpiresAtMS       int64  `json:"expires_at_ms"`
	CredentialEpoch   uint64 `json:"credential_epoch"`
	EventID           string `json:"event_id"`
	Stage             string `json:"stage"`
	FromEpoch         uint64 `json:"from_epoch"`
	ToEpoch           uint64 `json:"to_epoch"`
	VerifiedAtMS      int64  `json:"verified_at_ms"`
}

type identityReferenceCase struct {
	Name                 string                    `json:"name"`
	Kind                 string                    `json:"kind"`
	NowUnixSeconds       int64                     `json:"now_unix_seconds"`
	ExpectedNonce        string                    `json:"expected_nonce"`
	Token                string                    `json:"token"`
	MathematicallySigned bool                      `json:"mathematically_signed"`
	Accepted             bool                      `json:"accepted"`
	Expected             identityReferenceExpected `json:"expected"`
	RejectionReason      string                    `json:"rejection_reason"`
}

type identityReferenceFile struct {
	FixtureOnly bool   `json:"fixture_only"`
	Version     uint32 `json:"version"`
	Source      string `json:"source"`
	Config      struct {
		Version       uint32          `json:"version"`
		Issuer        string          `json:"issuer"`
		Audience      string          `json:"audience"`
		ResetAudience string          `json:"reset_audience"`
		EpochClaim    string          `json:"epoch_claim"`
		PublicJWKS    json.RawMessage `json:"public_jwks"`
	} `json:"config"`
	RFC7515Example string                  `json:"rfc7515_example"`
	Cases          []identityReferenceCase `json:"cases"`
}

func identityReferences(t *testing.T) identityReferenceFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/identity_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var file identityReferenceFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		t.Fatal(err)
	}
	if !file.FixtureOnly || file.Version != 1 || len(file.Cases) < 100 ||
		file.Source != "https://www.rfc-editor.org/rfc/rfc7515.html#appendix-A.2.1" {
		t.Fatal("expected independently generated public RFC-key corpus")
	}
	return file
}

func identityReferenceVerifier(t *testing.T, f identityReferenceFile) *Verifier {
	t.Helper()
	v, err := NewVerifier(Config{
		Version: f.Config.Version, Issuer: f.Config.Issuer,
		Audience: f.Config.Audience, ResetAudience: f.Config.ResetAudience,
		EpochClaim: f.Config.EpochClaim, PublicJWKS: f.Config.PublicJWKS,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := hex.DecodeString(v.ConfigDigest())
	if err != nil || len(digest) != 32 || strings.ToLower(v.ConfigDigest()) != v.ConfigDigest() {
		t.Fatal("verifier configuration digest is not canonical SHA256 hex")
	}
	return v
}

func identityReferencePublicKey(t *testing.T, f identityReferenceFile) *rsa.PublicKey {
	t.Helper()
	var jwks struct {
		Keys []struct{ N, E string }
	}
	if err := json.Unmarshal(f.Config.PublicJWKS, &jwks); err != nil || len(jwks.Keys) != 1 {
		t.Fatal("independent corpus must contain one public RFC RSA key")
	}
	n, err := base64.RawURLEncoding.DecodeString(jwks.Keys[0].N)
	if err != nil {
		t.Fatal(err)
	}
	e, err := base64.RawURLEncoding.DecodeString(jwks.Keys[0].E)
	if err != nil {
		t.Fatal(err)
	}
	key := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	if key.N.BitLen() != 2048 || key.E != 65537 {
		t.Fatal("published RFC key identity changed")
	}
	return key
}

func verifyReferenceRS256(t *testing.T, key *rsa.PublicKey, token string) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("expected compact signed fixture")
	}
	sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("independent Python RSA signature is invalid: %v", err)
	}
}

// Expected acceptance and output came from the frozen English claim contract.
// The generator signed exact JSON bytes with standalone Python modular RSA,
// and reproduced the RFC example before generating any SLUG token. Negative
// signed vectors have valid signatures, so a parser/claim failure cannot be
// masked by a broken signature fixture. No real provider or private file is used.
func TestReferenceIdentityCorpus(t *testing.T) {
	f := identityReferences(t)
	key := identityReferencePublicKey(t, f)
	verifyReferenceRS256(t, key, f.RFC7515Example)
	v := identityReferenceVerifier(t, f)
	seen := make(map[string]bool)
	for _, fixture := range f.Cases {
		if seen[fixture.Name] {
			t.Fatalf("duplicate reference case %q", fixture.Name)
		}
		seen[fixture.Name] = true
		t.Run(fixture.Name, func(t *testing.T) {
			if fixture.MathematicallySigned {
				verifyReferenceRS256(t, key, fixture.Token)
			}
			if !fixture.Accepted && fixture.RejectionReason == "" {
				t.Fatal("negative case must explain the rejected contract")
			}
			v.now = func() time.Time { return time.Unix(fixture.NowUnixSeconds, 0) }
			want := fixture.Expected
			switch fixture.Kind {
			case "login":
				got, err := v.VerifyLogin(context.Background(), fixture.Token, fixture.ExpectedNonce)
				if !fixture.Accepted {
					if err == nil || got != (gymaccount.VerifiedLogin{}) {
						t.Fatalf("accepted or exposed a rejected login: %s", fixture.RejectionReason)
					}
					return
				}
				expected := gymaccount.VerifiedLogin{
					VerifierConfigHash: v.ConfigDigest(), Issuer: want.Issuer,
					Subject: want.Subject, Nonce: want.Nonce,
					AuthenticatedAtMS: want.AuthenticatedAtMS, IssuedAtMS: want.IssuedAtMS,
					ExpiresAtMS: want.ExpiresAtMS, CredentialEpoch: want.CredentialEpoch,
				}
				if err != nil || !reflect.DeepEqual(got, expected) {
					t.Fatalf("independent login result differs: got %+v, error %v; want %+v", got, err, expected)
				}
			case "reset":
				got, err := v.VerifyReset(context.Background(), fixture.Token)
				if !fixture.Accepted {
					if err == nil || got != (gymaccount.VerifiedReset{}) {
						t.Fatalf("accepted or exposed a rejected reset: %s", fixture.RejectionReason)
					}
					return
				}
				expected := gymaccount.VerifiedReset{
					VerifierConfigHash: v.ConfigDigest(), Issuer: want.Issuer,
					Subject: want.Subject, EventID: want.EventID, Stage: want.Stage,
					FromEpoch: want.FromEpoch, ToEpoch: want.ToEpoch,
					VerifiedAtMS: want.VerifiedAtMS, IssuedAtMS: want.IssuedAtMS,
					ExpiresAtMS: want.ExpiresAtMS,
				}
				if err != nil || !reflect.DeepEqual(got, expected) {
					t.Fatalf("independent reset result differs: got %+v, error %v; want %+v", got, err, expected)
				}
			default:
				t.Fatal("unknown reference kind")
			}
		})
	}
}

func TestReferenceIdentityExpiryAndCancellation(t *testing.T) {
	f := identityReferences(t)
	v := identityReferenceVerifier(t, f)
	for _, fixture := range f.Cases {
		if fixture.Name != "valid-login-auth-at-issuance" && fixture.Name != "valid-reset-begin" {
			continue
		}
		t.Run(fixture.Name, func(t *testing.T) {
			verify := func(ctx context.Context) error {
				if fixture.Kind == "login" {
					_, err := v.VerifyLogin(ctx, fixture.Token, fixture.ExpectedNonce)
					return err
				}
				_, err := v.VerifyReset(ctx, fixture.Token)
				return err
			}
			v.now = func() time.Time { return time.UnixMilli(fixture.Expected.ExpiresAtMS - 1) }
			if err := verify(context.Background()); err != nil {
				t.Fatalf("fresh assertion rejected before exclusive expiry: %v", err)
			}
			v.now = func() time.Time { return time.UnixMilli(fixture.Expected.ExpiresAtMS) }
			if err := verify(context.Background()); err == nil {
				t.Fatal("accepted token at its exclusive expiry")
			}
			v.now = func() time.Time { return time.Unix(fixture.NowUnixSeconds, 0) }
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := verify(ctx); err == nil {
				t.Fatal("accepted already-cancelled verification")
			}
		})
	}
}
