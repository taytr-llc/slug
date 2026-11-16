// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gymauth

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/btcec/v2/ecdsa"
	"github.com/taytr-llc/slug/prospect"
)

const (
	Version1             uint32 = 1
	Version2             uint32 = 2
	MaxAuthorizationSize        = 4096

	gymDomain          = "SLUG gym identity\x00v1\x00"
	admissionDomain    = "SLUG prospect admission\x00v1\x00"
	jobAdmissionDomain = "SLUG prospect admission\x00v2\x00"
	policyDomain       = "SLUG prospect policy\x00v1\x00"
)

// Authorization is public, relayable evidence of permission to admit one proof.
// The version fixes the algorithm to secp256k1 ECDSA, compressed public keys,
// SHA256 admission digests, and strict canonical DER low-S signatures.
type Authorization struct {
	Version   uint32 `json:"version"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

func decodeHash(value string, nonzero bool) ([32]byte, error) {
	var decoded [32]byte
	if len(value) != 64 {
		return decoded, errors.New("expected exactly 64 hexadecimal characters")
	}
	if _, err := hex.Decode(decoded[:], []byte(value)); err != nil {
		return [32]byte{}, errors.New("invalid hexadecimal value")
	}
	if nonzero && decoded == ([32]byte{}) {
		return decoded, errors.New("zero identifier is invalid")
	}
	return decoded, nil
}

func publicKey(value string) (*btcec.PublicKey, []byte, error) {
	if len(value) != 66 {
		return nil, nil, errors.New("public key must be a 33-byte compressed secp256k1 key")
	}
	encoded, err := hex.DecodeString(value)
	if err != nil || !btcec.IsCompressedPubKey(encoded) {
		return nil, nil, errors.New("invalid compressed public key")
	}
	key, err := btcec.ParsePubKey(encoded)
	if err != nil || !bytes.Equal(key.SerializeCompressed(), encoded) {
		return nil, nil, errors.New("invalid compressed public key")
	}
	return key, encoded, nil
}

func canonicalSignature(value string) (*ecdsa.Signature, error) {
	if len(value) < ecdsa.MinSigLen*2 || len(value) > ecdsa.MaxSigLen*2 || len(value)%2 != 0 {
		return nil, errors.New("invalid authorization signature length")
	}
	encoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid signature hexadecimal encoding")
	}
	signature, err := ecdsa.ParseDERSignature(encoded)
	if err != nil || ecdsa.VerifyLowS(encoded) != nil {
		return nil, errors.New("signature must be strict DER with low S")
	}
	// ParseDERSignature accepts a suffix beyond its internal DER length.
	// Exact serialization equality rejects that suffix and all aliases.
	if !bytes.Equal(signature.Serialize(), encoded) {
		return nil, errors.New("signature is not canonical DER")
	}
	return signature, nil
}

func putUint32(dst hash.Hash, value uint32) {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], value)
	_, _ = dst.Write(raw[:])
}

// GymID derives a chain-bound identifier from a public key. Only the context
// version, network and genesis are relevant; creating an identity does not
// require a current parent or mining policy. The private key remains external.
//
// Bytes: "SLUG gym identity\x00v1\x00" || network_LE32 ||
// genesis_display32 || compressed_public_key33, hashed once with SHA256.
func GymID(context prospect.Context, key string) (string, error) {
	if context.Version != prospect.Version1 || context.Network == 0 {
		return "", errors.New("unsupported gym chain identity")
	}
	genesis, err := decodeHash(context.Genesis, true)
	if err != nil {
		return "", errors.New("invalid gym genesis")
	}
	_, encoded, err := publicKey(key)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(gymDomain))
	putUint32(digest, context.Network)
	_, _ = digest.Write(genesis[:])
	_, _ = digest.Write(encoded)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// AdmissionDigest validates bounded encodings and forms the action-specific
// digest. The complete proof is committed as decoded bytes, so hexadecimal case
// does not change the authorization. It does not validate the coinbase/Merkle
// semantics or perform Argon2id work verification; use prospect.Verify as well.
//
// The set-14 policy ID is SHA256("SLUG prospect policy\x00v1\x00" ||
// network_LE32 || genesis_display32 || DNA_version_LE32 || target_BE32).
// Admission bytes are "SLUG prospect admission\x00v1\x00" || policy_ID32 ||
// gym_ID32 || pubkey33 || proof_version_LE32 || header80 || coinbase_length_LE32
// || coinbase || branch_count_LE32 || each sibling_display32 in supplied order.
// Version 2 uses the "SLUG prospect admission\x00v2\x00" domain and inserts
// job_ID32 immediately after proof_version_LE32. Gym identity is unchanged.
// Node-derived transient timestamp bounds are excluded from the signed bytes.
func AdmissionDigest(context prospect.Context, proof prospect.Proof, key string) ([32]byte, error) {
	var empty [32]byte
	gym, err := GymID(context, key)
	if err != nil {
		return empty, err
	}
	gymBytes, _ := decodeHash(gym, true)
	claimedGym, err := decodeHash(proof.GymID, true)
	if err != nil || claimedGym != gymBytes {
		return empty, errors.New("proof gym does not match public key identity")
	}
	// Reuse semantic context/job validators without hashing work or asserting
	// that a job was issued. Issuance and freshness are separate admission rules.
	var job [32]byte
	switch proof.Version {
	case prospect.Version1:
		if proof.JobID != "" {
			return empty, errors.New("version-1 proof must not carry a job ID")
		}
		_, err = prospect.CommitmentScript(context, gym)
	case prospect.Version2:
		_, err = prospect.CommitmentScriptForJob(context, gym, proof.JobID)
		if err == nil {
			job, err = decodeHash(proof.JobID, false)
		}
	default:
		return empty, errors.New("unsupported prospect proof version")
	}
	if err != nil {
		return empty, err
	}
	if len(proof.Header) != 160 {
		return empty, errors.New("proof header must contain exactly 80 bytes")
	}
	header, err := hex.DecodeString(proof.Header)
	if err != nil {
		return empty, errors.New("invalid proof header encoding")
	}
	parent, _ := decodeHash(context.ParentHash, true)
	for i := range parent {
		if header[35-i] != parent[i] {
			return empty, errors.New("proof header does not match context parent")
		}
	}
	timestamp := binary.LittleEndian.Uint32(header[68:72])
	if binary.LittleEndian.Uint32(header[72:76]) != context.ExpectedBits || timestamp < context.MinTimestamp || timestamp > context.MaxTimestamp {
		return empty, errors.New("proof header does not match context bits or time bounds")
	}
	if len(proof.Coinbase) == 0 || len(proof.Coinbase) > 16384*2 || len(proof.Coinbase)%2 != 0 {
		return empty, errors.New("coinbase must encode 1 to 16384 bytes")
	}
	coinbase, err := hex.DecodeString(proof.Coinbase)
	if err != nil {
		return empty, errors.New("invalid coinbase encoding")
	}
	if len(proof.MerkleBranch) > 20 {
		return empty, errors.New("proof exceeds 20 Merkle siblings")
	}
	var siblings [20][32]byte
	for i, text := range proof.MerkleBranch {
		siblings[i], err = decodeHash(text, false)
		if err != nil {
			return empty, errors.New("invalid Merkle sibling encoding")
		}
	}
	genesis, _ := decodeHash(context.Genesis, true)
	target, _ := decodeHash(context.ProspectTarget, true)
	policy := sha256.New()
	_, _ = policy.Write([]byte(policyDomain))
	putUint32(policy, context.Network)
	_, _ = policy.Write(genesis[:])
	putUint32(policy, context.DNAVersion)
	_, _ = policy.Write(target[:])
	_, pubBytes, _ := publicKey(key)
	digest := sha256.New()
	domain := admissionDomain
	if proof.Version == prospect.Version2 {
		domain = jobAdmissionDomain
	}
	_, _ = digest.Write([]byte(domain))
	_, _ = digest.Write(policy.Sum(nil))
	_, _ = digest.Write(gymBytes[:])
	_, _ = digest.Write(pubBytes)
	putUint32(digest, proof.Version)
	if proof.Version == prospect.Version2 {
		_, _ = digest.Write(job[:])
	}
	_, _ = digest.Write(header)
	putUint32(digest, uint32(len(coinbase)))
	_, _ = digest.Write(coinbase)
	putUint32(digest, uint32(len(proof.MerkleBranch)))
	for i := range proof.MerkleBranch {
		_, _ = digest.Write(siblings[i][:])
	}
	var result [32]byte
	copy(result[:], digest.Sum(nil))
	return result, nil
}

// VerifyAuthorization proves key control and permission for this exact
// admission action. It does not replace production proof verification or a
// trusted node/freshness check and cannot authorize any other gym action.
func VerifyAuthorization(context prospect.Context, proof prospect.Proof, authorization Authorization) error {
	if (authorization.Version != Version1 && authorization.Version != Version2) || authorization.Version != proof.Version {
		return errors.New("unsupported authorization version")
	}
	key, _, err := publicKey(authorization.PublicKey)
	if err != nil {
		return err
	}
	signature, err := canonicalSignature(authorization.Signature)
	if err != nil {
		return err
	}
	digest, err := AdmissionDigest(context, proof, authorization.PublicKey)
	if err != nil {
		return err
	}
	if !signature.Verify(digest[:], key) {
		return errors.New("invalid prospect admission authorization")
	}
	return nil
}

// SignAdmission is an automatic client helper for the single admission action.
// The caller supplies a nonzero key already held in memory. No key generation,
// import, persistence, wallet access or private-key serialization is performed.
func SignAdmission(context prospect.Context, proof prospect.Proof, key *btcec.PrivateKey) (Authorization, error) {
	if key == nil || key.Key.IsZero() {
		return Authorization{}, errors.New("nonzero gym signing key is required")
	}
	public := hex.EncodeToString(key.PubKey().SerializeCompressed())
	digest, err := AdmissionDigest(context, proof, public)
	if err != nil {
		return Authorization{}, err
	}
	authorization := Authorization{Version: proof.Version, PublicKey: public,
		Signature: hex.EncodeToString(ecdsa.Sign(key, digest[:]).Serialize())}
	if err := VerifyAuthorization(context, proof, authorization); err != nil {
		return Authorization{}, err
	}
	return authorization, nil
}
