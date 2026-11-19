// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectnode

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/wire/v2"
)

// PoolBlockObservation reports one bounded observation of the configured node.
// Canonical false is a definite branch observation, not an RPC failure. The
// block itself is excluded from Descendants. This grants no maturity,
// spendability, finality, fighter admission or payment.
type PoolBlockObservation struct {
	BlockHash   string `json:"block_hash"`
	BlockHeight uint32 `json:"block_height"`
	TipHash     string `json:"tip_hash"`
	TipHeight   uint32 `json:"tip_height"`
	Canonical   bool   `json:"canonical"`
	Descendants uint32 `json:"descendants"`
}

type poolMembership struct {
	parent string
	winner string
}

// ObservePoolBlock obtains fresh canonical membership and descendant depth.
// workHash MUST be the locally verified production Argon hash of proof.Header;
// this method does not rehash or authenticate claimant-supplied summaries. The
// caller must separately bind that work to the retained pool payout, just as
// for CanonicalBlock. A stable node tip and exact membership are bracketed in
// this operation. A moving tip, RPC failure or mismatched header returns an
// error and zero observation, never an orphan report or confirmation.
func (client *Client) ObservePoolBlock(ctx context.Context, initial prospect.Context, proof prospect.Proof, workHash string) (PoolBlockObservation, error) {
	if err := productionOnly(); err != nil {
		return PoolBlockObservation{}, err
	}
	if ctx == nil || client == nil || client.http == nil {
		return PoolBlockObservation{}, errors.New("invalid pool block observation request")
	}
	if err := admissionHeader(initial, proof); err != nil {
		return PoolBlockObservation{}, err
	}
	workHash, err := normalizeHash(workHash, false)
	if err != nil {
		return PoolBlockObservation{}, errors.New("invalid worker block hash")
	}
	_, target, err := compactTarget(strconv.FormatUint(uint64(initial.ExpectedBits), 16))
	if err != nil || workHash > target {
		return PoolBlockObservation{}, errors.New("verified work does not meet block difficulty")
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := client.identity(ctx); err != nil {
		return PoolBlockObservation{}, err
	}
	before, err := client.chainTip(ctx)
	if err != nil {
		return PoolBlockObservation{}, err
	}
	anchor, err := client.hashAt(ctx, uint32(before.height))
	if err != nil {
		return PoolBlockObservation{}, err
	}
	if anchor != before.hash {
		return PoolBlockObservation{}, errors.New("pool observation tip is not canonical")
	}
	blockHeight := initial.ParentHeight + 1
	readMembership := func() (poolMembership, error) {
		// A shorter canonical branch definitely does not contain this height.
		if uint32(before.height) < blockHeight {
			return poolMembership{}, nil
		}
		parent, err := client.hashAt(ctx, initial.ParentHeight)
		if err != nil {
			return poolMembership{}, err
		}
		winner, err := client.hashAt(ctx, blockHeight)
		if err != nil {
			return poolMembership{}, err
		}
		return poolMembership{parent: parent, winner: winner}, nil
	}
	membership, err := readMembership()
	if err != nil {
		return PoolBlockObservation{}, err
	}
	if membership.winner == workHash && !strings.EqualFold(membership.parent, initial.ParentHash) {
		return PoolBlockObservation{}, errors.New("pool block ancestry is inconsistent")
	}
	if uint32(before.height) == blockHeight && membership.winner != before.hash {
		return PoolBlockObservation{}, errors.New("pool block membership differs from observed tip")
	}
	canonical := uint32(before.height) >= blockHeight &&
		strings.EqualFold(membership.parent, initial.ParentHash) && membership.winner == workHash
	if canonical {
		raw, err := client.call(ctx, "getblockheader", []any{workHash, false})
		if err != nil {
			return PoolBlockObservation{}, err
		}
		var header string
		if json.Unmarshal(raw, &header) != nil || len(header) != 2*wire.MaxBlockHeaderPayload || !strings.EqualFold(header, proof.Header) {
			return PoolBlockObservation{}, errors.New("canonical pool block header does not match proof")
		}
	}
	if err := client.identity(ctx); err != nil {
		return PoolBlockObservation{}, err
	}
	after, err := client.chainTip(ctx)
	if err != nil {
		return PoolBlockObservation{}, err
	}
	if after != before {
		return PoolBlockObservation{}, errors.New("node tip changed during pool observation")
	}
	finalMembership, err := readMembership()
	if err != nil {
		return PoolBlockObservation{}, err
	}
	if finalMembership != membership {
		return PoolBlockObservation{}, errors.New("canonical membership changed during pool observation")
	}
	anchor, err = client.hashAt(ctx, uint32(after.height))
	if err != nil {
		return PoolBlockObservation{}, err
	}
	if anchor != after.hash {
		return PoolBlockObservation{}, errors.New("pool observation tip is no longer canonical")
	}
	if err := ctx.Err(); err != nil {
		return PoolBlockObservation{}, err
	}
	observation := PoolBlockObservation{BlockHash: workHash, BlockHeight: blockHeight,
		TipHash: before.hash, TipHeight: uint32(before.height), Canonical: canonical}
	if canonical {
		observation.Descendants = observation.TipHeight - blockHeight
	}
	return observation, nil
}
