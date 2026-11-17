// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymprovider

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
)

const providerTestTenant = "fixture-tenant"
const providerTestClient = "fixture-client"
const providerTestConnection = "fixture-password"

type providerFixture struct {
	Authority      *gymaccount.Store
	Verifier       *gymidentity.Verifier
	Coordinator    *Coordinator
	Config         Config
	IdentityConfig gymaccount.IdentityConfig
	Root           string
}

func newProviderFixture(t *testing.T) *providerFixture {
	t.Helper()
	f := &providerFixture{Root: t.TempDir()}
	v, err := gymidentity.NewVerifier(FixtureVerifierConfig())
	if err != nil {
		t.Fatal(err)
	}
	f.Verifier = v
	f.IdentityConfig = gymaccount.IdentityConfig{Version: 2, VerifierConfigHash: v.ConfigDigest(), MaxLoginAttempts: 128,
		Accounts: gymaccount.Config{Version: 1, MaxSessions: 128, MaxChallenges: 128, MaxEvents: 512,
			Principals: []gymaccount.FixturePrincipal{{Issuer: FixtureIssuer, Subject: "alice", Fixture: "zero256"}, {Issuer: FixtureIssuer, Subject: "bob", Fixture: "incrementing256"}}}}
	f.Authority, err = gymaccount.CreateIdentity(filepath.Join(f.Root, "authority"), f.IdentityConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if f.Authority != nil {
			_ = f.Authority.Close()
		}
	})
	info, err := f.Authority.AuthorityInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.Config = Config{Version: 1, Provider: ProviderFixture, Issuer: FixtureIssuer, TenantID: providerTestTenant, ClientID: providerTestClient, ConnectionID: providerTestConnection,
		AuthorityID: info.AuthorityID, VerifierConfigHash: v.ConfigDigest(), MaxLoginFlows: 64, MaxResetFlows: 32}
	f.Coordinator, err = Create(context.Background(), filepath.Join(f.Root, "coordinator"), f.Config, f.Authority, v)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if f.Coordinator != nil {
			_ = f.Coordinator.Close()
		}
	})
	return f
}

// signProviderClaims uses only the RSA exponent published in RFC7515 A.2.
// These dynamic state-test signatures are not expected cryptographic vectors;
// the independent Python corpus tests the exact signature/claim contract.
func signProviderClaims(t *testing.T, reset bool, claims map[string]any) string {
	t.Helper()
	raw, err := os.ReadFile("../gymidentity/testdata/rfc7515_rsa.json")
	if err != nil {
		t.Fatal(err)
	}
	var key struct{ N, E, D, Kid string }
	if err = json.Unmarshal(raw, &key); err != nil {
		t.Fatal(err)
	}
	decode := func(s string) []byte {
		b, e := base64.RawURLEncoding.Strict().DecodeString(s)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	n, d := decode(key.N), decode(key.D)
	typ := "JWT"
	if reset {
		typ = "slug-reset+jwt"
	}
	head, err := json.Marshal(map[string]any{"alg": "RS256", "kid": key.Kid, "typ": typ})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(body)
	hash := sha256.Sum256([]byte(input))
	digestInfo, _ := hex.DecodeString("3031300d060960864801650304020105000420")
	em := make([]byte, len(n))
	em[1] = 1
	separator := len(em) - len(digestInfo) - len(hash) - 1
	for i := 2; i < separator; i++ {
		em[i] = 0xff
	}
	copy(em[separator+1:], digestInfo)
	copy(em[len(em)-len(hash):], hash[:])
	modulus := new(big.Int).SetBytes(n)
	signature := new(big.Int).Exp(new(big.Int).SetBytes(em), new(big.Int).SetBytes(d), modulus).FillBytes(make([]byte, len(n)))
	pub := rsa.PublicKey{N: modulus, E: 65537}
	if rsa.VerifyPKCS1v15(&pub, crypto.SHA256, hash[:], signature) != nil {
		t.Fatal("public RFC fixture signature failed independent verification")
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func providerLoginClaims(flow LoginFlow, subject, originID, originalFlowID, mode string, epoch uint64, authMS int64) map[string]any {
	now := time.Now().Unix()
	return map[string]any{"iss": FixtureIssuer, "aud": "slug-provider-fixture-login", "sub": subject,
		"iat": now, "exp": now + 250, "auth_time": authMS / 1000, "nonce": flow.Nonce, "slug_auth_epoch": epoch,
		"provider_version": 1, "coordinator_id": flow.CoordinatorID, "authority_id": flow.AuthorityID,
		"tenant_id": providerTestTenant, "client_id": providerTestClient, "connection_id": providerTestConnection,
		"login_flow_id": flow.ID, "original_auth_id": originID, "original_flow_id": originalFlowID, "auth_mode": mode}
}
func signLogin(t *testing.T, flow LoginFlow, subject, originID, originalFlowID, mode string, epoch uint64, authMS int64) string {
	t.Helper()
	return signProviderClaims(t, false, providerLoginClaims(flow, subject, originID, originalFlowID, mode, epoch, authMS))
}
func providerResetClaims(request ResetRequest, subject, operationID, stage string, from, to uint64, verifiedMS int64) map[string]any {
	now := time.Now().Unix()
	return map[string]any{"iss": FixtureIssuer, "aud": "slug-provider-fixture-reset", "sub": subject,
		"iat": now, "exp": now + 250, "event_id": request.EventID, "stage": stage, "from_epoch": from, "to_epoch": to, "verified_at": verifiedMS / 1000,
		"provider_version": 1, "coordinator_id": request.CoordinatorID, "authority_id": request.AuthorityID,
		"tenant_id": providerTestTenant, "client_id": providerTestClient, "connection_id": providerTestConnection,
		"reset_nonce": request.Nonce, "provider_operation_id": operationID}
}
func signReset(t *testing.T, request ResetRequest, subject, operationID, stage string, from, to uint64, verifiedMS int64) string {
	t.Helper()
	return signProviderClaims(t, true, providerResetClaims(request, subject, operationID, stage, from, to, verifiedMS))
}
