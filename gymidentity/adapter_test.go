// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymidentity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
)

// unitSign uses only the RSA exponent published in RFC 7515 A.2. This signs
// changing behavioral-test inputs; independent cryptographic goldens are in
// reference_test.go and were generated without this implementation.
func unitSign(t *testing.T, claims map[string]any) string {
	t.Helper()
	data, err := os.ReadFile("testdata/rfc7515_rsa.json")
	if err != nil {
		t.Fatal(err)
	}
	var key struct{ N, D, Kid string }
	if err = json.Unmarshal(data, &key); err != nil {
		t.Fatal(err)
	}
	n, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		t.Fatal(err)
	}
	d, err := base64.RawURLEncoding.DecodeString(key.D)
	if err != nil {
		t.Fatal(err)
	}
	header, _ := json.Marshal(map[string]any{"alg": "RS256", "kid": key.Kid, "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	hash := sha256.Sum256([]byte(input))
	prefix, _ := hex.DecodeString("3031300d060960864801650304020105000420")
	em := make([]byte, len(n))
	em[1] = 1
	separator := len(em) - len(prefix) - len(hash) - 1
	for i := 2; i < separator; i++ {
		em[i] = 255
	}
	copy(em[separator+1:], prefix)
	copy(em[len(em)-len(hash):], hash[:])
	sig := new(big.Int).Exp(new(big.Int).SetBytes(em), new(big.Int).SetBytes(d), new(big.Int).SetBytes(n)).FillBytes(make([]byte, len(n)))
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func unitClaims(c Config, nonce string) map[string]any {
	now := time.Now().Unix()
	return map[string]any{"iss": c.Issuer, "sub": "public-fixture-a", "aud": c.Audience, "nonce": nonce, "iat": now, "exp": now + 250, "auth_time": now, c.EpochClaim: uint64(1)}
}

func TestLoginUsesDurableNonceAndPinnedVerifier(t *testing.T) {
	c := unitConfig(t)
	v, err := NewVerifier(c)
	if err != nil {
		t.Fatal(err)
	}
	s, err := gymaccount.CreateIdentity(filepath.Join(t.TempDir(), "authority"), gymaccount.IdentityConfig{Version: gymaccount.IdentityVersion, VerifierConfigHash: v.ConfigDigest(), MaxLoginAttempts: 16, Accounts: gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: c.Issuer, Subject: "public-fixture-a", Fixture: "zero256"}, {Issuer: c.Issuer, Subject: "public-fixture-b", Fixture: "incrementing256"}}, MaxSessions: 16, MaxChallenges: 16, MaxEvents: 64}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	attempt, err := s.IssueLogin(ctx, c.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	wrong := unitSign(t, unitClaims(c, strings.Repeat("0", 64)))
	if _, err = v.Login(ctx, s, attempt.ID, wrong); !errors.Is(err, ErrAssertion) {
		t.Fatalf("wrong nonce: %v", err)
	}
	other := c
	other.Audience += "-different"
	untrusted, err := NewVerifier(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = untrusted.Login(ctx, s, attempt.ID, unitSign(t, unitClaims(other, attempt.Nonce))); !errors.Is(err, gymaccount.ErrUnauthorized) {
		t.Fatalf("wrong trust digest: %v", err)
	}
	session, err := v.Login(ctx, s, attempt.ID, unitSign(t, unitClaims(c, attempt.Nonce)))
	if err != nil {
		t.Fatal(err)
	}
	if session.Token == "" || session.Account.Generation != 1 {
		t.Fatal("missing current session")
	}
	if _, err = v.Login(ctx, s, attempt.ID, unitSign(t, unitClaims(c, attempt.Nonce))); !errors.Is(err, gymaccount.ErrUnauthorized) {
		t.Fatalf("consumed nonce replay: %v", err)
	}
	if err = s.WithSession(ctx, session.Token, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestVerifierCancellationConcurrencyAndRedaction(t *testing.T) {
	c := unitConfig(t)
	v, err := NewVerifier(c)
	if err != nil {
		t.Fatal(err)
	}
	nonce := strings.Repeat("a", 64)
	token := unitSign(t, unitClaims(c, nonce))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = v.VerifyLogin(ctx, token, nonce); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	v.now = func() time.Time { cancel(); return time.Now() }
	if _, err = v.VerifyLogin(ctx, token, nonce); !errors.Is(err, context.Canceled) {
		t.Fatalf("post-validation cancellation: %v", err)
	}
	v.now = time.Now
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				p, err := v.VerifyLogin(context.Background(), token, nonce)
				if err != nil || p.VerifierConfigHash != v.ConfigDigest() {
					t.Errorf("concurrent verification: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	_, err = v.VerifyLogin(context.Background(), token+"private-assertion-sentinel", nonce)
	if !errors.Is(err, ErrAssertion) || strings.Contains(err.Error(), "sentinel") || strings.Contains(err.Error(), token) {
		t.Fatal("assertion error leaked input")
	}
	var nilVerifier *Verifier
	if _, err = nilVerifier.VerifyLogin(context.Background(), token, nonce); !errors.Is(err, ErrConfig) {
		t.Fatal("nil verifier accepted")
	}
	if _, err = (&Verifier{}).VerifyReset(context.Background(), token); !errors.Is(err, ErrConfig) {
		t.Fatal("uninitialized verifier accepted")
	}
}
