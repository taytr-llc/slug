// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectnode

import (
	"context"
	"encoding/hex"
	"encoding/json"

	"github.com/taytr-llc/slug/prospect"
)

// SubmissionError distinguishes a definite preflight failure or explicit node
// rejection from an uncertain submission outcome. On uncertainty preserve and
// retry the exact block; do not infer that it was rejected or is canonical.
type SubmissionError struct {
	Code           string
	OutcomeUnknown bool
}

func (err *SubmissionError) Error() string {
	switch err.Code {
	case "unsupported_profile":
		return "block submission is unavailable in slugtestpow builds"
	case "invalid_request":
		return "invalid block submission request"
	case "unavailable":
		return "block submission node identity could not be verified"
	case "rejected":
		return "node rejected block submission"
	default:
		return "block submission outcome is unknown"
	}
}

// SubmitBlock sends one complete canonical block to the explicitly configured
// production node. It does not rehash, retry, or wait for canonical membership.
// A nil result means only that the node reported processing it; use a separate
// CanonicalWinner observation before invoking the winning-block exception.
func (client *Client) SubmitBlock(ctx context.Context, expected prospect.Context, blockHex string) error {
	if productionOnly() != nil {
		return &SubmissionError{Code: "unsupported_profile"}
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	raw, _, err := canonicalBlock(expected, blockHex)
	if err != nil {
		return &SubmissionError{Code: "invalid_request"}
	}
	if err := client.identity(ctx); err != nil {
		return &SubmissionError{Code: "unavailable"}
	}
	result, err := client.callResult(ctx, "submitblock", []any{hex.EncodeToString(raw)}, true)
	if err != nil {
		return &SubmissionError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	if !isNull(result) {
		var reason string
		if json.Unmarshal(result, &reason) == nil && reason != "" {
			return &SubmissionError{Code: "rejected"}
		}
		return &SubmissionError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	if err := client.identity(ctx); err != nil {
		return &SubmissionError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	return nil
}
