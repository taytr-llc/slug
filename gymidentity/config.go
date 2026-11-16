// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymidentity

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	Version1                uint32 = 1
	MaxJWKSSize                    = 65536
	MaxAssertionSize               = 16384
	MaxHeaderSize                  = 2048
	MaxPayloadSize                 = 8192
	MaxKeys                        = 8
	FreshnessSeconds        int64  = 300
	MaxTokenLifetimeSeconds int64  = 300
)

var (
	ErrConfig    = errors.New("invalid identity verifier configuration")
	ErrAssertion = errors.New("identity assertion rejected")
)

// Config contains public trust only. PublicJWKS must be supplied explicitly;
// this package never discovers providers, fetches keys, or accepts private JWKs.
// EpochClaim must attest the credential epoch of the original authentication,
// including after token refresh. A provider must support that extension and the
// reset barrier protocol before it can be used here.
type Config struct {
	Version       uint32
	Issuer        string
	Audience      string
	ResetAudience string
	EpochClaim    string
	PublicJWKS    []byte
}

// Verifier owns an immutable parsed public keyset and trust configuration.
// Verification is safe for concurrent use; no assertion or subject is retained.
type Verifier struct {
	issuer, audience, resetAudience, epochClaim string
	keys                                        map[string]*rsa.PublicKey
	digest                                      string
	now                                         func() time.Time
}

type publicKeyRecord struct {
	KID string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// NewVerifier validates all public trust before accepting any assertion.
func NewVerifier(c Config) (*Verifier, error) {
	if c.Version != Version1 || !validIssuer(c.Issuer) || !boundedText(c.Audience, 256) || !boundedText(c.ResetAudience, 256) || c.Audience == c.ResetAudience || !validEpochClaim(c.EpochClaim) {
		return nil, ErrConfig
	}
	object, err := parseObject(c.PublicJWKS, MaxJWKSSize)
	if err != nil || !onlyFields(object, "keys") {
		return nil, ErrConfig
	}
	entries, ok := object["keys"].([]any)
	if !ok || len(entries) == 0 || len(entries) > MaxKeys {
		return nil, ErrConfig
	}
	keys := make(map[string]*rsa.PublicKey, len(entries))
	records := make([]publicKeyRecord, 0, len(entries))
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok || !onlyFields(m, "kid", "kty", "n", "e", "alg", "use") {
			return nil, ErrConfig
		}
		kid, err := stringField(m, "kid", 128)
		if err != nil || keys[kid] != nil || m["kty"] != "RSA" {
			return nil, ErrConfig
		}
		if alg, exists := m["alg"]; exists && alg != "RS256" {
			return nil, ErrConfig
		}
		if use, exists := m["use"]; exists && use != "sig" {
			return nil, ErrConfig
		}
		n, err := stringField(m, "n", 684)
		if err != nil {
			return nil, ErrConfig
		}
		e, err := stringField(m, "e", 8)
		if err != nil || e != "AQAB" {
			return nil, ErrConfig
		}
		nBytes, err := rawURL(n, 512)
		if err != nil || nBytes[0] == 0 {
			return nil, ErrConfig
		}
		modulus := new(big.Int).SetBytes(nBytes)
		if modulus.BitLen() < 2048 || modulus.BitLen() > 4096 || modulus.Bit(0) != 1 {
			return nil, ErrConfig
		}
		keys[kid] = &rsa.PublicKey{N: modulus, E: 65537}
		records = append(records, publicKeyRecord{KID: kid, N: n, E: e})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].KID < records[j].KID })
	// Format and key order do not change trust. The algorithm/profile version,
	// issuer, audiences, epoch claim and complete normalized keyset do.
	canonical, err := json.Marshal(struct {
		Version       uint32            `json:"version"`
		Algorithm     string            `json:"algorithm"`
		Issuer        string            `json:"issuer"`
		Audience      string            `json:"audience"`
		ResetAudience string            `json:"reset_audience"`
		EpochClaim    string            `json:"epoch_claim"`
		Keys          []publicKeyRecord `json:"keys"`
	}{c.Version, "RS256", c.Issuer, c.Audience, c.ResetAudience, c.EpochClaim, records})
	if err != nil {
		return nil, ErrConfig
	}
	hash := sha256.Sum256(append([]byte("SLUG identity verifier configuration\x00v1\x00"), canonical...))
	return &Verifier{issuer: c.Issuer, audience: c.Audience, resetAudience: c.ResetAudience, epochClaim: c.EpochClaim, keys: keys, digest: hex.EncodeToString(hash[:]), now: time.Now}, nil
}

// ConfigDigest is the public trust identity pinned in an explicit schema-2
// account authority. It is computed locally, never read from token claims.
func (v *Verifier) ConfigDigest() string {
	if v == nil {
		return ""
	}
	return v.digest
}

func boundedText(s string, max int) bool {
	_, err := stringField(map[string]any{"s": s}, "s", max)
	return err == nil
}

func validIssuer(s string) bool {
	if !boundedText(s, 1024) {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && !strings.Contains(s, "#") && u.Opaque == ""
}

var reservedClaims = []string{"iss", "aud", "azp", "sub", "nonce", "iat", "exp", "auth_time", "nbf", "event_id", "stage", "from_epoch", "to_epoch", "verified_at"}

func validEpochClaim(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for i, c := range s {
		if (c < 'a' || c > 'z') && c != '_' && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	for _, name := range reservedClaims {
		if s == name {
			return false
		}
	}
	return true
}
