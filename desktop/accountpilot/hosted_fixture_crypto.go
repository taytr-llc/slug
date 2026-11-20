// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"time"

	"github.com/taytr-llc/slug/gymidentity"
	"github.com/taytr-llc/slug/gymprovider"
)

// These are RFC7515 A.2's published educational fixture integers, never an
// operator credential. The only constructor is the explicit hosted fixture route.
const hostedFixtureModulus = "ofgWCuLjybRlzo0tZWJjNiuSfb4p4fAkd_wWJcyQoTbji9k0l8W26mPddxHmfHQp-Vaw-4qPCJrcS2mJPMEzP1Pt0Bm4d4QlL-yRT-SFd2lZS-pCgNMsD1W_YpRPEwOWvG6b32690r2jZ47soMZo9wGzjb_7OMg0LOL-bSf63kpaSHSXndS5z5rexMdbBYUsLA9e-KXBdQOS-UTo7WTBEMa2R2CapHg665xsmtdVMTBQY4uDZlxvb3qCo5ZwKh9kG4LT6_I5IhlJH7aGhyxXFvUK-DWNmoudF8NAco9_h9iaGNj8q2ethFkMLs91kzk2PAcDTW9gb54h4FRWyuXpoQ"
const hostedFixtureExponent = "Eq5xpGnNCivDflJsRQBXHx1hdR1k6Ulwe2JZD50LpXyWPEAeP88vLNO97IjlA7_GQ5sLKMgvfTeXZx9SE-7YwVol2NXOoAJe46sui395IW_GO-pWJ1O0BkTGoVEn2bKVRUCgu-GjBVaYLU6f3l9kJfFNS3E0QbVdxzubSu3Mkqzjkn439X0M_V51gfpRLI9JYanrC4D4qAdGcopV_0ZHHzQlBjudU2QvXt4ehNYTCBr6XCLQUShb1juUO1ZdiYoFaFQT5Tw8bGUl_x_jTj3ccPDVZFD9pIuhLhBOneufuBiB4cS98l2SR_RQyGWSeWjnczT0QU91p1DhOVRuOopznQ"

func hostedFixtureKey() (*rsa.PrivateKey, error) {
	n, e := base64.RawURLEncoding.DecodeString(hostedFixtureModulus)
	if e != nil {
		return nil, e
	}
	d, e := base64.RawURLEncoding.DecodeString(hostedFixtureExponent)
	if e != nil {
		return nil, e
	}
	// Pin the public key to the inherited verifier, not a caller selected key.
	cfg := gymprovider.FixtureVerifierConfig()
	var jwks struct{ Keys []struct{ N, E string } }
	if json.Unmarshal(cfg.PublicJWKS, &jwks) != nil || len(jwks.Keys) != 1 || jwks.Keys[0].N != hostedFixtureModulus || jwks.Keys[0].E != "AQAB" {
		return nil, gymprovider.ErrConfig
	}
	return &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: new(big.Int).SetBytes(n), E: 65537}, D: new(big.Int).SetBytes(d)}, nil
}
func hostedRandomID(r io.Reader) (string, error) {
	var b [32]byte
	if _, e := io.ReadFull(r, b[:]); e != nil {
		return "", e
	}
	if b == ([32]byte{}) {
		return "", gymprovider.ErrUnavailable
	}
	return hex.EncodeToString(b[:]), nil
}

type hostedCommonClaims struct {
	Issuer          string `json:"iss"`
	Subject         string `json:"sub"`
	Issued          int64  `json:"iat"`
	Expires         int64  `json:"exp"`
	ProviderVersion uint32 `json:"provider_version"`
	CoordinatorID   string `json:"coordinator_id"`
	AuthorityID     string `json:"authority_id"`
	TenantID        string `json:"tenant_id"`
	ClientID        string `json:"client_id"`
	ConnectionID    string `json:"connection_id"`
}
type hostedLoginClaims struct {
	hostedCommonClaims
	Audience       string `json:"aud"`
	AuthTime       int64  `json:"auth_time"`
	Nonce          string `json:"nonce"`
	Epoch          uint64 `json:"slug_auth_epoch"`
	FlowID         string `json:"login_flow_id"`
	AuthID         string `json:"original_auth_id"`
	OriginalFlowID string `json:"original_flow_id"`
	Mode           string `json:"auth_mode"`
}
type hostedResetClaims struct {
	hostedCommonClaims
	Audience    string `json:"aud"`
	EventID     string `json:"event_id"`
	Stage       string `json:"stage"`
	From        uint64 `json:"from_epoch"`
	To          uint64 `json:"to_epoch"`
	Verified    int64  `json:"verified_at"`
	Nonce       string `json:"reset_nonce"`
	OperationID string `json:"provider_operation_id"`
}

func hostedClaims(c gymprovider.Config, coordinator, subject string, now int64) hostedCommonClaims {
	return hostedCommonClaims{gymprovider.FixtureIssuer, subject, now / 1000, now/1000 + 120, 1, coordinator, c.AuthorityID, c.TenantID, c.ClientID, c.ConnectionID}
}
func hostedSign(key *rsa.PrivateKey, kind string, claims any) (string, error) {
	header := struct {
		Alg  string `json:"alg"`
		Kid  string `json:"kid"`
		Type string `json:"typ"`
	}{"RS256", "rfc7515-a2", kind}
	h, e := json.Marshal(header)
	if e != nil {
		return "", e
	}
	b, e := json.Marshal(claims)
	if e != nil {
		return "", e
	}
	in := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(b)
	digest := sha256.Sum256([]byte(in))
	// The RFC publishes n/e/d without CRT primes. This explicit public-fixture
	// path uses the inherited RS256 encoding directly; it is not a general key
	// import/signing facility and never handles an operator's private key.
	prefix, _ := hex.DecodeString("3031300d060960864801650304020105000420")
	tail := append(prefix, digest[:]...)
	encoded := make([]byte, key.Size())
	encoded[1] = 1
	for i := 2; i < len(encoded)-len(tail)-1; i++ {
		encoded[i] = 0xff
	}
	copy(encoded[len(encoded)-len(tail):], tail)
	number := new(big.Int).SetBytes(encoded)
	signed := new(big.Int).Exp(number, key.D, key.N)
	check := new(big.Int).Exp(signed, big.NewInt(int64(key.E)), key.N)
	if check.Cmp(number) != 0 {
		return "", gymprovider.ErrUnavailable
	}
	sig := signed.FillBytes(make([]byte, key.Size()))
	token := in + "." + base64.RawURLEncoding.EncodeToString(sig)
	if len(token) > 16384 {
		return "", gymprovider.ErrCapacity
	}
	return token, nil
}
func hostedProviderConfig(authority string) (gymprovider.Config, error) {
	v, e := gymidentity.NewVerifier(gymprovider.FixtureVerifierConfig())
	if e != nil {
		return gymprovider.Config{}, e
	}
	return gymprovider.Config{Version: 1, Provider: gymprovider.ProviderFixture, Issuer: gymprovider.FixtureIssuer, TenantID: "fixture-tenant", ClientID: "fixture-client", ConnectionID: "fixture-database", AuthorityID: authority, VerifierConfigHash: v.ConfigDigest(), MaxLoginFlows: 1024, MaxResetFlows: 256}, nil
}

// hostedFixtureWrapper is not configurable. Only the two published fixture
// purpose keys are available, and normal envelope randomness remains fresh.
type hostedFixtureWrapper struct{}

func (hostedFixtureWrapper) ID() string { return hostedWrapperID }
func hostedWrapperAEAD(ref string) (cipher.AEAD, error) {
	var key [32]byte
	var start byte
	switch ref {
	case "fixture-recovery-v1":
	case "fixture-gym-v1":
		start = 64
	default:
		return nil, errors.New("fixture key reference unavailable")
	}
	for i := range key {
		key[i] = start + byte(i)
	}
	defer clear(key[:])
	b, e := aes.NewCipher(key[:])
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(b)
}
func (hostedFixtureWrapper) Wrap(ctx context.Context, ref string, dek [32]byte, aad []byte) ([]byte, error) {
	defer clear(dek[:])
	if ctx == nil {
		return nil, gymprovider.ErrConfig
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if e := c.Err(); e != nil {
		return nil, e
	}
	a, e := hostedWrapperAEAD(ref)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, 12)
	if _, e = io.ReadFull(rand.Reader, nonce); e != nil {
		return nil, e
	}
	out := a.Seal(nonce, nonce, dek[:], aad)
	if e = c.Err(); e != nil {
		clear(out)
		return nil, e
	}
	return out, nil
}
func (hostedFixtureWrapper) Unwrap(ctx context.Context, ref string, blob, aad []byte) ([32]byte, error) {
	var out [32]byte
	if ctx == nil || len(blob) != 60 {
		return out, gymprovider.ErrConfig
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if e := c.Err(); e != nil {
		return out, e
	}
	a, e := hostedWrapperAEAD(ref)
	if e != nil {
		return out, e
	}
	raw, e := a.Open(nil, blob[:12], blob[12:], aad)
	if e != nil {
		return out, gymprovider.ErrUnavailable
	}
	defer clear(raw)
	if len(raw) != 32 {
		return out, gymprovider.ErrUnavailable
	}
	if e = c.Err(); e != nil {
		return out, e
	}
	copy(out[:], raw)
	return out, nil
}
