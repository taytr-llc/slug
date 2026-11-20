// Copyright (c) 2026 The SLUG developers. ISC license.
package fightertransfer

import (
	"bytes"
	"context"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

// Discovery is an unverified discriminator observation. A malformed locator or
// envelope may be discovered and then rejected by ValidateSupplied. FirstInput
// is only a lookup hint; it is not proof of an actual committed spend or ownership.
type Discovery struct {
	TxID             chainhash.Hash
	InputCount       uint32
	FirstInput       wire.OutPoint
	CommitLocator    bool
	RevealLocator    bool
	EmbeddedEnvelope bool
}

// Discover scans a complete bounded transaction, without requiring a locally
// saved TxID. Refund candidates are found by the last pushed redeem envelope;
// exposing that envelope alone does not classify the spend as a transfer.
func Discover(ctx context.Context, raw []byte) (Discovery, error) {
	var d Discovery
	scan := func(input bool, script []byte) {
		if !input {
			if len(script) == 0 || script[0] != txscript.OP_RETURN {
				return
			}
			t := txscript.MakeScriptTokenizer(0, script[1:])
			if !t.Next() {
				return
			}
			data := t.Data()
			d.CommitLocator = d.CommitLocator || bytes.HasPrefix(data, []byte("SLUGFCM1"))
			d.RevealLocator = d.RevealLocator || bytes.HasPrefix(data, []byte("SLUGFRV1"))
			return
		}
		if len(script) > MaxScriptSize {
			return
		}
		t := txscript.MakeScriptTokenizer(0, script)
		var last []byte
		for t.Next() {
			if ctx.Err() != nil {
				return
			}
			if t.Opcode() > txscript.OP_PUSHDATA4 {
				last = nil
			} else {
				last = t.Data()
			}
		}
		if t.Err() != nil || len(last) > 520 {
			return
		}
		redeem := txscript.MakeScriptTokenizer(0, last)
		if redeem.Next() && bytes.HasPrefix(redeem.Data(), []byte("SLUGFTR1")) {
			d.EmbeddedEnvelope = true
		}
	}
	p, err := scanTransaction(ctx, raw, parentMode, nil, scan)
	if err != nil {
		return Discovery{}, err
	}
	if err = ctx.Err(); err != nil {
		return Discovery{}, err
	}
	if !d.CommitLocator && !d.RevealLocator && !d.EmbeddedEnvelope {
		return Discovery{}, ErrNotCarrier
	}
	d.TxID = p.hash
	d.InputCount = uint32(p.inputCount)
	d.FirstInput = p.firstInput
	return d, nil
}
