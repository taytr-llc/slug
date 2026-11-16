// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectnode

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/wire/v2"
)

// admissionHeader checks public header binding without computing its hash.
func admissionHeader(initial prospect.Context, proof prospect.Proof) error {
	if err := validateInitial(initial); err != nil {
		return err
	}
	switch proof.Version {
	case prospect.Version1:
		if proof.JobID != "" {
			return errors.New("version-1 proof must not carry a job ID")
		}
	case prospect.Version2:
		if _, err := normalizeHash(proof.JobID, false); err != nil {
			return errors.New("invalid proof job ID")
		}
	default:
		return errors.New("unsupported proof version")
	}
	if len(proof.Header) != 2*wire.MaxBlockHeaderPayload {
		return errors.New("invalid admission header size")
	}
	raw, err := hex.DecodeString(proof.Header)
	if err != nil {
		return errors.New("invalid admission header")
	}
	var header wire.BlockHeader
	if header.Deserialize(bytes.NewReader(raw)) != nil || !strings.EqualFold(header.PrevBlock.String(), initial.ParentHash) ||
		header.Bits != initial.ExpectedBits || header.Timestamp.Unix() < int64(initial.MinTimestamp) ||
		header.Timestamp.Unix() > int64(initial.MaxTimestamp) {
		return errors.New("admission header does not match context")
	}
	return nil
}

// trustedAdmission checks inexpensive binding fields only. The caller must
// supply the result of prospect.Verify(initial, proof), never claimant-provided
// JSON. Recomputing Argon here would duplicate work already serialized by the
// registry's admission path. This is not a standalone proof verifier.
func trustedAdmission(initial prospect.Context, proof prospect.Proof, verified prospect.Verified) (string, error) {
	if err := admissionHeader(initial, proof); err != nil {
		return "", err
	}
	gym, err := normalizeHash(proof.GymID, true)
	if err != nil || verified.Version != proof.Version || verified.Verification != "supplied-context" ||
		verified.Network != initial.Network || !strings.EqualFold(verified.Genesis, initial.Genesis) ||
		!strings.EqualFold(verified.ParentHash, initial.ParentHash) || verified.ParentHeight != initial.ParentHeight ||
		!strings.EqualFold(verified.GymID, gym) {
		return "", errors.New("verified admission binding does not match proof")
	}
	workHash, err := normalizeHash(verified.WorkHash, false)
	if err != nil {
		return "", errors.New("invalid verified work hash")
	}
	return workHash, nil
}

// RecheckAdmission implements eligibility rule 2. verified MUST be the trusted
// result of prospect.Verify(initial, proof), such as the value supplied inside
// registry.RecordJobChecked's guard; accepting claimant JSON here is unsafe.
// One node-tip observation chooses the current-parent or canonical-winning-block
// path. RPC failures never trigger a fallback. The registry separately enforces
// the original job's expiry after this guard and before a new receipt commit.
func (client *Client) RecheckAdmission(ctx context.Context, initial prospect.Context, proof prospect.Proof, verified prospect.Verified) (string, error) {
	if err := productionOnly(); err != nil {
		return "", err
	}
	workHash, err := trustedAdmission(initial, proof, verified)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := client.identity(ctx); err != nil {
		return "", err
	}
	current, err := client.chainTip(ctx)
	if err != nil {
		return "", err
	}
	if current.height == int64(initial.ParentHeight) && strings.EqualFold(current.hash, initial.ParentHash) {
		if err := client.Recheck(ctx, initial, proof); err != nil {
			return "", err
		}
		return "current-parent", nil
	}
	if current.height <= int64(initial.ParentHeight) {
		return "", errors.New("node has not advanced beyond the original parent")
	}
	if err := client.canonicalWinner(ctx, initial, proof, workHash); err != nil {
		return "", err
	}
	return "canonical-winning-block", nil
}

// CanonicalWinner checks only the canonical-winning-block path for recovery of
// a block submission with an uncertain outcome. verified must come from a local
// prospect.Verify call for this exact context and proof. No new Argon work is
// performed. Success is an observation, not finality or a birth admission.
func (client *Client) CanonicalWinner(ctx context.Context, initial prospect.Context, proof prospect.Proof, verified prospect.Verified) error {
	if err := productionOnly(); err != nil {
		return err
	}
	workHash, err := trustedAdmission(initial, proof, verified)
	if err != nil {
		return err
	}
	return client.CanonicalBlock(ctx, initial, proof, workHash)
}

// CanonicalBlock checks a block-only winner, including one that did not meet the
// prospect target. workHash MUST be the worker-computed production Argon hash of
// this exact proof.Header; the method does not compute that hash or verify a
// prospect. It must not replace RecheckAdmission in the registry authority path,
// where only the internally verified result is trusted. Success observes exact
// canonical membership and never grants admission, birth, or finality.
func (client *Client) CanonicalBlock(ctx context.Context, initial prospect.Context, proof prospect.Proof, workHash string) error {
	if err := productionOnly(); err != nil {
		return err
	}
	if err := admissionHeader(initial, proof); err != nil {
		return err
	}
	workHash, err := normalizeHash(workHash, false)
	if err != nil {
		return errors.New("invalid worker block hash")
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := client.identity(ctx); err != nil {
		return err
	}
	return client.canonicalWinner(ctx, initial, proof, workHash)
}

func (client *Client) hashAt(ctx context.Context, height uint32) (string, error) {
	data, err := client.call(ctx, "getblockhash", []any{height})
	if err != nil {
		return "", err
	}
	var hash string
	if json.Unmarshal(data, &hash) != nil {
		return "", errors.New("invalid node canonical hash response")
	}
	hash, err = normalizeHash(hash, false)
	if err != nil {
		return "", errors.New("invalid node canonical hash response")
	}
	return hash, nil
}

// Entry requires an identity check in this same operation. Normal tip progress
// is allowed; only membership at the original parent and winning height matters.
func (client *Client) canonicalWinner(ctx context.Context, initial prospect.Context, proof prospect.Proof, workHash string) error {
	_, target, err := compactTarget(strconv.FormatUint(uint64(initial.ExpectedBits), 16))
	if err != nil || workHash > target {
		return errors.New("verified work does not meet block difficulty")
	}
	checkMembership := func() error {
		parent, err := client.hashAt(ctx, initial.ParentHeight)
		if err != nil {
			return err
		}
		if !strings.EqualFold(parent, initial.ParentHash) {
			return errors.New("original parent is not canonical")
		}
		winner, err := client.hashAt(ctx, initial.ParentHeight+1)
		if err != nil {
			return err
		}
		if winner != workHash {
			return errors.New("winning block is not canonical")
		}
		return nil
	}
	if err := checkMembership(); err != nil {
		return err
	}
	data, err := client.call(ctx, "getblockheader", []any{workHash, false})
	if err != nil {
		return err
	}
	var header string
	if json.Unmarshal(data, &header) != nil || len(header) != 2*wire.MaxBlockHeaderPayload || !strings.EqualFold(header, proof.Header) {
		return errors.New("canonical block header does not match proof")
	}
	if err := client.identity(ctx); err != nil {
		return err
	}
	// The final RPC observes membership again; an intervening tip advance is
	// harmless, while a reorganization excluding either block fails closed.
	if err := checkMembership(); err != nil {
		return err
	}
	return ctx.Err()
}
