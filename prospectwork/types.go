// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectwork

import (
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/prospect"
)

const (
	Version1 uint32 = 1
	// MaxTemplateSize bounds a complete GBT JSON result, including hex data.
	MaxTemplateSize = 16 * 1024 * 1024
)

// Template is an unmined node template and its separately trusted snapshot.
// Exported values remain untrusted inputs: Build validates them again and never
// retains caller-owned slices or pointers.
type Template struct {
	Context prospect.Context               `json:"context"`
	Block   btcjson.GetBlockTemplateResult `json:"block"`
}

// Candidate contains a payable, job-bound block candidate and its matching
// stripped-coinbase proof. It makes no claim of qualifying work, currentness,
// job issuance, birth, or node acceptance. Build never hashes the header.
type Candidate struct {
	Version          uint32         `json:"version"`
	JobID            string         `json:"job_id"`
	BlockHex         string         `json:"block_hex"`
	Proof            prospect.Proof `json:"proof"`
	PayoutAddress    string         `json:"payout_address"`
	CoinbaseValue    int64          `json:"coinbase_value"`
	TransactionCount uint32         `json:"transaction_count"`
	Weight           int64          `json:"weight"`
	SigOpCost        int64          `json:"sigop_cost"`
}
