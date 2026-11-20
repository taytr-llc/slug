// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

// Package gymaccount contains the account enrollment and hosted recovery
// contracts. The current service policy permits published fixture identities
// only; these contracts do not allocate a production wallet profile.
package gymaccount

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"strings"
	"unicode/utf8"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/btcec/v2/ecdsa"
	"github.com/taytr-llc/slug/btcutil/v2/hdkeychain"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

const (
	Version1               uint32 = 1
	MaxChallengeLifetimeMS int64  = 300000
	MaxChallengeSize              = 4096
	MaxAuthorizationSize          = 4096
	FixtureProfile                = "slug-phase-a-fixture-bip39-v1"
	FixtureNetwork         uint32 = 0x534c5547
	FixtureGenesis                = "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd"
	enrollmentDomain              = "SLUG account enrollment\x00v1\x00"
	principalDomain               = "SLUG account principal\x00v1\x00"
)

// EnrollmentChallenge commits one bounded enroll/recover action. Its signature
// does not authenticate the issuer, prove a live session, consume a challenge,
// or override a reset hold. Those are durable service-policy checks.
type EnrollmentChallenge struct {
	Version       uint32 `json:"version"`
	AuthorityID   string `json:"authority_id"`
	AccountID     string `json:"account_id"`
	PrincipalID   string `json:"principal_id"`
	ChallengeID   string `json:"challenge_id"`
	Generation    uint64 `json:"generation"`
	Action        string `json:"action"`
	Network       uint32 `json:"network"`
	Genesis       string `json:"genesis"`
	Profile       string `json:"profile"`
	RootPublicKey string `json:"root_public_key"`
	GymPublicKey  string `json:"gym_public_key"`
	GymID         string `json:"gym_id"`
	IssuedAtMS    int64  `json:"issued_at_ms"`
	ExpiresAtMS   int64  `json:"expires_at_ms"`
}

type Authorization struct {
	Version   uint32 `json:"version"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

func put32(h hash.Hash, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	_, _ = h.Write(b[:])
}
func put64(h hash.Hash, v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	_, _ = h.Write(b[:])
}
func putString(h hash.Hash, v string) { put32(h, uint32(len(v))); _, _ = h.Write([]byte(v)) }
func canonicalHex(value string, size int) ([]byte, error) {
	if len(value) != size*2 || value != strings.ToLower(value) {
		return nil, errors.New("invalid canonical hexadecimal encoding")
	}
	result, err := hex.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid canonical hexadecimal encoding")
	}
	return result, nil
}
func canonicalPublic(value string) (*btcec.PublicKey, []byte, error) {
	raw, err := canonicalHex(value, 33)
	if err != nil || !btcec.IsCompressedPubKey(raw) {
		return nil, nil, errors.New("invalid compressed public key")
	}
	key, err := btcec.ParsePubKey(raw)
	if err != nil || !bytes.Equal(key.SerializeCompressed(), raw) {
		return nil, nil, errors.New("invalid compressed public key")
	}
	return key, raw, nil
}
func canonicalRoot(value string) error {
	// Bound before Base58 decoding or curve operations.
	if len(value) == 0 || len(value) > 128 {
		return errors.New("invalid public wallet root")
	}
	key, err := hdkeychain.NewKeyFromString(value)
	if err != nil {
		return errors.New("invalid public wallet root")
	}
	defer key.Zero()
	if key.IsPrivate() || !bytes.Equal(key.Version(), chaincfg.MainNetParams.HDPublicKeyID[:]) || key.Depth() != 0 || key.ParentFingerprint() != 0 || key.ChildIndex() != 0 || key.String() != value {
		return errors.New("invalid public wallet root")
	}
	return nil
}

// PrincipalID identifies the exact trusted (issuer, subject) pair. It does not
// verify a provider assertion; input ownership and verification belong to the
// provider adapter. No email matching, trimming or case folding is performed.
func PrincipalID(issuer, subject string) (string, error) {
	if len(issuer) == 0 || len(issuer) > 1024 || len(subject) == 0 || len(subject) > 1024 || !utf8.ValidString(issuer) || !utf8.ValidString(subject) {
		return "", errors.New("invalid provider principal")
	}
	h := sha256.New()
	_, _ = h.Write([]byte(principalDomain))
	putString(h, issuer)
	putString(h, subject)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ChallengeDigest is SHA256 of the frozen version-1 binary enrollment contract.
// Hash/ID fields use displayed hex byte order, not chainhash internal reversal.
// It has no wall-clock or session authority: the store must enforce actual
// expiry, generation, single-use consumption, reset holds and fixture policy.
func ChallengeDigest(c EnrollmentChallenge) ([32]byte, error) {
	var empty [32]byte
	if c.Version != Version1 || c.Generation == 0 || (c.Action != "enroll" && c.Action != "recover") || c.Network != FixtureNetwork || c.Genesis != FixtureGenesis || c.Profile != FixtureProfile {
		return empty, errors.New("unsupported account challenge")
	}
	if c.IssuedAtMS <= 0 || c.ExpiresAtMS <= c.IssuedAtMS || c.ExpiresAtMS-c.IssuedAtMS > MaxChallengeLifetimeMS {
		return empty, errors.New("invalid challenge lifetime")
	}
	ids := make([][]byte, 4)
	for i, id := range []string{c.AuthorityID, c.AccountID, c.PrincipalID, c.ChallengeID} {
		var err error
		ids[i], err = canonicalHex(id, 32)
		if err != nil {
			return empty, errors.New("invalid challenge identifier")
		}
	}
	genesis, _ := canonicalHex(c.Genesis, 32)
	if err := canonicalRoot(c.RootPublicKey); err != nil {
		return empty, err
	}
	_, public, err := canonicalPublic(c.GymPublicKey)
	if err != nil {
		return empty, err
	}
	gym, err := canonicalHex(c.GymID, 32)
	if err != nil {
		return empty, errors.New("invalid gym identifier")
	}
	expected, err := gymauth.GymID(prospect.Context{Version: 1, Network: c.Network, Genesis: c.Genesis}, c.GymPublicKey)
	if err != nil || expected != c.GymID {
		return empty, errors.New("gym identifier does not match public key")
	}
	h := sha256.New()
	_, _ = h.Write([]byte(enrollmentDomain))
	put32(h, c.Version)
	for _, id := range ids {
		_, _ = h.Write(id)
	}
	put64(h, c.Generation)
	putString(h, c.Action)
	put32(h, c.Network)
	_, _ = h.Write(genesis)
	putString(h, c.Profile)
	putString(h, c.RootPublicKey)
	_, _ = h.Write(public)
	_, _ = h.Write(gym)
	put64(h, uint64(c.IssuedAtMS))
	put64(h, uint64(c.ExpiresAtMS))
	copy(empty[:], h.Sum(nil))
	return empty, nil
}

func canonicalSignature(value string) (*ecdsa.Signature, error) {
	if len(value) < ecdsa.MinSigLen*2 || len(value) > ecdsa.MaxSigLen*2 || len(value)%2 != 0 || value != strings.ToLower(value) {
		return nil, errors.New("invalid authorization signature")
	}
	raw, err := hex.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid authorization signature")
	}
	signature, err := ecdsa.ParseDERSignature(raw)
	if err != nil || ecdsa.VerifyLowS(raw) != nil || !bytes.Equal(signature.Serialize(), raw) {
		return nil, errors.New("signature must be canonical DER with low S")
	}
	return signature, nil
}

// ValidateAuthorization checks encoding only, not proof or session authority.
func ValidateAuthorization(a Authorization) error {
	if a.Version != Version1 {
		return errors.New("unsupported authorization version")
	}
	if _, _, err := canonicalPublic(a.PublicKey); err != nil {
		return err
	}
	_, err := canonicalSignature(a.Signature)
	return err
}
func VerifyAuthorization(c EnrollmentChallenge, a Authorization) error {
	if err := ValidateAuthorization(a); err != nil {
		return err
	}
	if a.PublicKey != c.GymPublicKey {
		return errors.New("authorization public key mismatch")
	}
	digest, err := ChallengeDigest(c)
	if err != nil {
		return err
	}
	public, _, _ := canonicalPublic(a.PublicKey)
	signature, _ := canonicalSignature(a.Signature)
	if !signature.Verify(digest[:], public) {
		return errors.New("invalid account authorization")
	}
	return nil
}

// SignChallenge signs only the bounded action-specific challenge. It performs
// no file access and does not grant session or recovery authority.
func SignChallenge(c EnrollmentChallenge, key *btcec.PrivateKey) (Authorization, error) {
	if key == nil || key.Key.IsZero() {
		return Authorization{}, errors.New("missing enrollment signing key")
	}
	digest, err := ChallengeDigest(c)
	if err != nil {
		return Authorization{}, err
	}
	public := hex.EncodeToString(key.PubKey().SerializeCompressed())
	if public != c.GymPublicKey {
		return Authorization{}, errors.New("signing key does not match gym")
	}
	return Authorization{Version: Version1, PublicKey: public, Signature: hex.EncodeToString(ecdsa.Sign(key, digest[:]).Serialize())}, nil
}

// ValidateFixtureIdentity enforces the independently published root/gym pairs.
// A signature by a hardened gym child alone cannot prove its derivation from a
// claimed public root. The current service must call this in both configuration
// and completion; production provisioning/root-control policy is unimplemented.
func ValidateFixtureIdentity(rootPublicKey, gymPublicKey, gymID string) error {
	for _, name := range []string{"zero256", "incrementing256"} {
		identity, err := FixtureIdentity(name)
		if err == nil && rootPublicKey == identity.RootPublicKey && gymPublicKey == identity.GymPublicKey && gymID == identity.GymID {
			return nil
		}
	}
	return errors.New("identity is not an approved public fixture pair")
}
