// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectnode

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

// ResolveTemplate returns the complete template from the same bracketed node
// observation used to derive Context. It does not build or hash a candidate.
func (client *Client) ResolveTemplate(ctx context.Context, policy Policy) (prospectwork.Template, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	resolved, data, err := client.resolveTemplateSnapshot(ctx, policy)
	if err != nil {
		return prospectwork.Template{}, err
	}
	template, err := prospectwork.ParseTemplate(resolved, data)
	if err != nil {
		return prospectwork.Template{}, errors.New("invalid complete node block template")
	}
	if err := ctx.Err(); err != nil {
		return prospectwork.Template{}, err
	}
	return template, nil
}

// Proposal asks the node to check a candidate without checking proof of work or
// submitting it. Success is a bounded observation of this node's current parent,
// not admission, a block reward, or a promise of later acceptance.
func (client *Client) Proposal(ctx context.Context, expected prospect.Context, blockHex string) error {
	if err := productionOnly(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	raw, block, err := canonicalBlock(expected, blockHex)
	if err != nil {
		return err
	}
	timestamp := block.Header.Timestamp.Unix()
	check := func() error {
		fresh, err := client.resolveSameParent(ctx, expected)
		if err != nil {
			return err
		}
		if timestamp < int64(fresh.MinTimestamp) || timestamp > int64(fresh.MaxTimestamp) {
			return errors.New("proposal timestamp is no longer current")
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	result, err := client.callResult(ctx, "getblocktemplate", []any{struct {
		Mode string `json:"mode"`
		Data string `json:"data"`
	}{"proposal", hex.EncodeToString(raw)}}, true)
	if err != nil {
		return err
	}
	if !isNull(result) {
		// Rejection strings are remote-controlled and can contain secrets or
		// terminal escapes. Never include their contents in diagnostics.
		return errors.New("node rejected block proposal")
	}
	return check()
}

// canonicalBlock performs bounded binary and header-binding checks only. It
// intentionally does not hash the header or assert full block validity.
func canonicalBlock(expected prospect.Context, blockHex string) ([]byte, *wire.MsgBlock, error) {
	if err := validateInitial(expected); err != nil {
		return nil, nil, err
	}
	if len(blockHex) < 2*(wire.MaxBlockHeaderPayload+1) || len(blockHex) > 2*wire.MaxBlockPayload || len(blockHex)%2 != 0 {
		return nil, nil, errors.New("invalid candidate block size")
	}
	raw, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, nil, errors.New("invalid candidate block encoding")
	}
	reader := bytes.NewReader(raw)
	var block wire.MsgBlock
	if block.Deserialize(reader) != nil || reader.Len() != 0 {
		return nil, nil, errors.New("invalid candidate block encoding")
	}
	var canonical bytes.Buffer
	if block.Serialize(&canonical) != nil || !bytes.Equal(raw, canonical.Bytes()) {
		return nil, nil, errors.New("noncanonical candidate block encoding")
	}
	parent, _ := normalizeHash(expected.ParentHash, true)
	timestamp := block.Header.Timestamp.Unix()
	if block.Header.PrevBlock.String() != parent || block.Header.Bits != expected.ExpectedBits ||
		timestamp < int64(expected.MinTimestamp) || timestamp > int64(expected.MaxTimestamp) {
		return nil, nil, errors.New("candidate header does not match expected context")
	}
	return raw, &block, nil
}
