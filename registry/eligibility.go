// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package registry

import (
	"errors"
	"math/big"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/wire/v2"
)

const EligibilityRuleVersion2 uint32 = 2

// Eligibility preserves the trusted node guard's admission-time observation.
// It is neither a permanent canonicality claim nor an authenticated node proof.
// A later reorganization or retry never rewrites this original observation.
type Eligibility struct {
	RuleVersion uint32 `json:"rule_version"`
	Source      string `json:"source"`
	Hash        string `json:"hash"`
	Height      uint32 `json:"height"`
}

// CreateMining explicitly initializes a new schema-4 D68/D69 authority. The
// constructor never migrates, modifies, or opts in an existing registry.
func CreateMining(path string, network uint32, genesis string) (*Store, error) {
	if wire.TestPoW {
		return nil, errors.New("mining issuance requires production PoW; slugtestpow is unsupported")
	}
	return create(path, network, genesis, Version4)
}

// ValidateEligibility checks an observation's exact binding and block-target
// comparison without hashing or consulting a node. The caller must obtain
// verified from prospect.Verify; this helper does not authenticate a supplied
// Verified value or establish that the observation was ever canonical.
func ValidateEligibility(context prospect.Context, verified prospect.Verified, value Eligibility) error {
	canonical, err := normalizeHash(value.Hash)
	if err != nil || canonical != value.Hash || value.RuleVersion != EligibilityRuleVersion2 {
		return errors.New("invalid admission eligibility observation")
	}
	switch value.Source {
	case "current-parent":
		parent, err := normalizeHash(context.ParentHash)
		if err != nil || value.Hash != parent || value.Height != context.ParentHeight {
			return errors.New("current-parent eligibility does not match issued context")
		}
	case "canonical-winning-block":
		work, err := normalizeHash(verified.WorkHash)
		if err != nil || verified.WorkHash != work || value.Hash != work || context.ParentHeight > 2147483646 || value.Height != context.ParentHeight+1 {
			return errors.New("winning-block eligibility does not match verified work")
		}
		// WorkHash is already production-verified by the admission flow. The
		// display-order number is compared directly, without another Argon
		// calculation or a caller-supplied claim about the winning header.
		target := blockchain.CompactToBig(context.ExpectedBits)
		if target.Sign() <= 0 || target.BitLen() > 256 || blockchain.BigToCompact(target) != context.ExpectedBits {
			return errors.New("winning-block eligibility has invalid block target")
		}
		hash, ok := new(big.Int).SetString(work, 16)
		if !ok || hash.Cmp(target) > 0 {
			return errors.New("verified work does not meet the block target")
		}
	default:
		return errors.New("unsupported admission eligibility source")
	}
	return nil
}

func validateRecordEligibility(record Record) error {
	if record.Version != Version4 {
		if record.Eligibility != nil {
			return errors.New("legacy receipt contains eligibility metadata")
		}
		return nil
	}
	if record.Eligibility == nil {
		return errors.New("schema-4 receipt is missing admission eligibility")
	}
	return ValidateEligibility(record.Context, record.Verified, *record.Eligibility)
}
