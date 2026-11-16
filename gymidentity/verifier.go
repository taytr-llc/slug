// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymidentity

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"math"
	"strings"

	"github.com/taytr-llc/slug/gymaccount"
)

func (v *Verifier) assertion(ctx context.Context, token string, reset bool) (map[string]any, error) {
	if ctx == nil || v == nil || v.now == nil || len(v.keys) == 0 || v.digest == "" {
		return nil, ErrConfig
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(token) == 0 || len(token) > MaxAssertionSize || strings.Count(token, ".") != 2 {
		return nil, ErrAssertion
	}
	parts := strings.Split(token, ".")
	headerBytes, err := rawURL(parts[0], MaxHeaderSize)
	if err != nil {
		return nil, ErrAssertion
	}
	header, err := parseObject(headerBytes, MaxHeaderSize)
	if err != nil || !onlyFields(header, "alg", "kid", "typ") || header["alg"] != "RS256" {
		return nil, ErrAssertion
	}
	kid, err := stringField(header, "kid", 128)
	if err != nil || v.keys[kid] == nil {
		return nil, ErrAssertion
	}
	typ, present := header["typ"]
	if (reset && typ != "slug-reset+jwt") || (!reset && present && typ != "JWT") {
		return nil, ErrAssertion
	}
	payload, err := rawURL(parts[1], MaxPayloadSize)
	if err != nil {
		return nil, ErrAssertion
	}
	signature, err := rawURL(parts[2], 512)
	key := v.keys[kid]
	if err != nil || len(signature) != key.Size() {
		return nil, ErrAssertion
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		return nil, ErrAssertion
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	claims, err := parseObject(payload, MaxPayloadSize)
	if err != nil || !v.canonicalClaimNames(claims) {
		return nil, ErrAssertion
	}
	return claims, nil
}

func (v *Verifier) canonicalClaimNames(claims map[string]any) bool {
	for k := range claims {
		lower := strings.ToLower(k)
		if lower == v.epochClaim && k != v.epochClaim {
			return false
		}
		for _, name := range reservedClaims {
			if lower == name && k != name {
				return false
			}
		}
	}
	return true
}

func secondsField(claims map[string]any, name string) (int64, error) {
	n, err := uintField(claims, name)
	if err != nil || n == 0 || n > math.MaxInt64/1000 {
		return 0, ErrAssertion
	}
	return int64(n) * 1000, nil
}

func (v *Verifier) common(claims map[string]any) (subject string, issued, expires, now int64, err error) {
	issuer, err := stringField(claims, "iss", 1024)
	if err != nil || issuer != v.issuer {
		return "", 0, 0, 0, ErrAssertion
	}
	subject, err = stringField(claims, "sub", 1024)
	if err != nil {
		return "", 0, 0, 0, ErrAssertion
	}
	issued, err = secondsField(claims, "iat")
	if err != nil {
		return "", 0, 0, 0, ErrAssertion
	}
	expires, err = secondsField(claims, "exp")
	now = v.now().UnixMilli()
	if err != nil || now <= 0 || issued > now || expires <= now || expires <= issued || expires-issued > MaxTokenLifetimeSeconds*1000 || now-issued > FreshnessSeconds*1000 {
		return "", 0, 0, 0, ErrAssertion
	}
	if _, present := claims["nbf"]; present {
		nbf, err := secondsField(claims, "nbf")
		if err != nil || nbf > now || nbf >= expires {
			return "", 0, 0, 0, ErrAssertion
		}
	}
	return subject, issued, expires, now, nil
}

func audienceMatches(claims map[string]any, expected string, reset bool) bool {
	count := 0
	switch aud := claims["aud"].(type) {
	case string:
		if aud != expected {
			return false
		}
		count = 1
	case []any:
		if reset || len(aud) == 0 || len(aud) > 8 {
			return false
		}
		seen := make(map[string]bool)
		for _, value := range aud {
			s, ok := value.(string)
			if !ok || !boundedText(s, 256) || seen[s] {
				return false
			}
			seen[s] = true
		}
		if !seen[expected] {
			return false
		}
		count = len(aud)
	default:
		return false
	}
	azp, present := claims["azp"]
	return (!present && count == 1) || (present && azp == expected)
}

// VerifyLogin authenticates an assertion, not an account session. ExpectedNonce
// must come from the authority's persisted attempt. Prefer Login for consuming
// an assertion: its store transaction also rejects used or pre-reset attempts,
// stale epochs and pending resets. No claim can grant reset authority.
func (v *Verifier) VerifyLogin(ctx context.Context, token, expectedNonce string) (gymaccount.VerifiedLogin, error) {
	if !canonicalID(expectedNonce) {
		return gymaccount.VerifiedLogin{}, ErrAssertion
	}
	claims, err := v.assertion(ctx, token, false)
	if err != nil {
		return gymaccount.VerifiedLogin{}, err
	}
	subject, issued, expires, now, err := v.common(claims)
	if err != nil || !audienceMatches(claims, v.audience, false) {
		return gymaccount.VerifiedLogin{}, ErrAssertion
	}
	nonce, err := stringField(claims, "nonce", 64)
	if err != nil || nonce != expectedNonce {
		return gymaccount.VerifiedLogin{}, ErrAssertion
	}
	authenticated, err := secondsField(claims, "auth_time")
	if err != nil || authenticated > issued || now-authenticated > FreshnessSeconds*1000 {
		return gymaccount.VerifiedLogin{}, ErrAssertion
	}
	epoch, err := uintField(claims, v.epochClaim)
	if err != nil || epoch == 0 {
		return gymaccount.VerifiedLogin{}, ErrAssertion
	}
	if err := ctx.Err(); err != nil {
		return gymaccount.VerifiedLogin{}, err
	}
	return gymaccount.VerifiedLogin{VerifierConfigHash: v.digest, Issuer: v.issuer, Subject: subject, Nonce: nonce, AuthenticatedAtMS: authenticated, IssuedAtMS: issued, ExpiresAtMS: expires, CredentialEpoch: epoch}, nil
}

// VerifyReset authenticates a provider barrier assertion on its separate
// audience and JWS type. Fresh transport may attest an old immutable event;
// BeginReset applies freshness to new events and matches stored event fields on
// reconciliation. An ordinary ID token cannot pass this method.
func (v *Verifier) VerifyReset(ctx context.Context, token string) (gymaccount.VerifiedReset, error) {
	claims, err := v.assertion(ctx, token, true)
	if err != nil {
		return gymaccount.VerifiedReset{}, err
	}
	subject, issued, expires, _, err := v.common(claims)
	if err != nil || !audienceMatches(claims, v.resetAudience, true) {
		return gymaccount.VerifiedReset{}, ErrAssertion
	}
	id, err := stringField(claims, "event_id", 64)
	if err != nil || !canonicalID(id) {
		return gymaccount.VerifiedReset{}, ErrAssertion
	}
	stage, err := stringField(claims, "stage", 8)
	if err != nil || (stage != "begin" && stage != "complete") {
		return gymaccount.VerifiedReset{}, ErrAssertion
	}
	from, err := uintField(claims, "from_epoch")
	if err != nil || from == 0 || from == math.MaxUint64 {
		return gymaccount.VerifiedReset{}, ErrAssertion
	}
	to, err := uintField(claims, "to_epoch")
	if err != nil || to != from+1 {
		return gymaccount.VerifiedReset{}, ErrAssertion
	}
	verified, err := secondsField(claims, "verified_at")
	if err != nil || verified > issued {
		return gymaccount.VerifiedReset{}, ErrAssertion
	}
	if err := ctx.Err(); err != nil {
		return gymaccount.VerifiedReset{}, err
	}
	return gymaccount.VerifiedReset{VerifierConfigHash: v.digest, Issuer: v.issuer, Subject: subject, EventID: id, Stage: stage, FromEpoch: from, ToEpoch: to, VerifiedAtMS: verified, IssuedAtMS: issued, ExpiresAtMS: expires}, nil
}

// Login obtains the expected nonce from durable authority state, validates the
// signed assertion, then consumes the attempt and creates the session atomically.
// The store rechecks ordering and epoch after verification; no network or crypto
// operation runs under its mutation lease. Tokens are never retained or logged.
func (v *Verifier) Login(ctx context.Context, store *gymaccount.Store, attemptID, token string) (gymaccount.Session, error) {
	if store == nil || v == nil {
		return gymaccount.Session{}, ErrConfig
	}
	attempt, err := store.GetLoginAttempt(ctx, attemptID)
	if err != nil {
		return gymaccount.Session{}, err
	}
	if attempt.Issuer != v.issuer {
		return gymaccount.Session{}, ErrAssertion
	}
	verified, err := v.VerifyLogin(ctx, token, attempt.Nonce)
	if err != nil {
		return gymaccount.Session{}, err
	}
	return store.LoginWithAssertion(ctx, attempt.ID, verified)
}
