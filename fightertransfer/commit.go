// Copyright (c) 2026 The SLUG developers. ISC license.
package fightertransfer

import (
	"bytes"
	"context"
	"fmt"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

type commitFunding struct {
	ops    []wire.OutPoint
	digest [32]byte
}

func checkCommitFunding(ctx context.Context, c *wire.MsgTx, fundingParents [][]byte) (commitFunding, error) {
	var err error
	ops := make([]wire.OutPoint, len(c.TxIn))
	wanted := make(map[uint32]bool, len(c.TxIn))
	parentsWanted := make(map[chainhash.Hash]bool, len(c.TxIn))
	for n, in := range c.TxIn {
		if in.Sequence != wire.MaxTxInSequenceNum || len(in.SignatureScript) != 0 || len(in.Witness) != 2 {
			return commitFunding{}, invalid("native commit input")
		}
		if _, err = publicKey(in.Witness[1]); err != nil {
			return commitFunding{}, err
		}
		ops[n] = in.PreviousOutPoint
		wanted[in.PreviousOutPoint.Index] = true
		parentsWanted[in.PreviousOutPoint.Hash] = true
	}
	funding, err := FundingDigest(ops)
	if err != nil {
		return commitFunding{}, err
	}
	parents := make(map[chainhash.Hash]map[uint32]*wire.TxOut, len(fundingParents))
	for _, raw := range fundingParents {
		if len(raw) == 0 {
			return commitFunding{}, fmt.Errorf("%w: empty parent", ErrUnavailable)
		}
		p, err := scanTransaction(ctx, raw, parentMode, wanted)
		if err != nil {
			return commitFunding{}, err
		}
		if _, exists := parents[p.hash]; exists {
			return commitFunding{}, invalid("duplicate funding parent")
		}
		parents[p.hash] = p.outputs
	}
	fetch := txscript.NewMultiPrevOutFetcher(nil)
	var total int64
	for _, in := range c.TxIn {
		p, exists := parents[in.PreviousOutPoint.Hash]
		if !exists {
			return commitFunding{}, fmt.Errorf("%w: funding parent", ErrUnavailable)
		}
		out, exists := p[in.PreviousOutPoint.Index]
		if !exists {
			return commitFunding{}, invalid("funding output index")
		}
		if !nativeProgram(out.PkScript) || out.Value <= 0 || out.Value > maxMoney-total {
			return commitFunding{}, invalid("funding output")
		}
		keyHash := hash160(in.Witness[1])
		if !bytes.Equal(out.PkScript[2:], keyHash[:]) {
			return commitFunding{}, invalid("funding key program")
		}
		total += out.Value
		fetch.AddPrevOut(in.PreviousOutPoint, out)
	}
	// Extra well-formed parents have no authority role and are refused only after
	// missing required parents have been classified as unavailable.
	if len(parents) != len(parentsWanted) {
		return commitFunding{}, invalid("unreferenced funding parent")
	}
	if c.TxOut[0].Value != CommitAtoms || c.TxOut[1].Value != 0 || c.TxOut[2].Value < MinChangeAtoms || !nativeProgram(c.TxOut[2].PkScript) || total-c.TxOut[0].Value-c.TxOut[2].Value != LegFeeAtoms {
		return commitFunding{}, invalid("commit values or return")
	}
	hashes := txscript.NewTxSigHashes(c, fetch)
	for n, in := range c.TxIn {
		if err = ctx.Err(); err != nil {
			return commitFunding{}, err
		}
		out := fetch.FetchPrevOutput(in.PreviousOutPoint)
		script := append([]byte{0x76, 0xa9, 0x14}, out.PkScript[2:]...)
		script = append(script, 0x88, 0xac)
		digest, e := txscript.CalcWitnessSigHash(script, hashes, txscript.SigHashAll, c, n, out.Value)
		if e != nil {
			return commitFunding{}, invalid("native signature hash")
		}
		if err = verifyDER(in.Witness[0], digest, in.Witness[1]); err != nil {
			return commitFunding{}, err
		}
	}
	return commitFunding{ops: ops, digest: funding}, nil
}

// CommitInspection checks supplied commit/funding bytes only. It deliberately
// does not assert a gym signature, the P2SH redeem preimage, chain inclusion,
// predecessor authority, eligibility or ownership.
type CommitInspection struct {
	Intent       Intent
	CommitTxID   chainhash.Hash
	FundingKey   [33]byte
	Funding      []wire.OutPoint
	ReturnScript [22]byte
	CommitScript [23]byte
}

func InspectCommit(ctx context.Context, pin, prior [32]byte, recipient [33]byte, raw []byte, parents [][]byte) (CommitInspection, error) {
	var result CommitInspection
	if ctx == nil {
		return result, invalid("nil context")
	}
	if e := ctx.Err(); e != nil {
		return result, e
	}
	if len(parents) > MaxInputs {
		return result, fmt.Errorf("%w: parent count", ErrCapacity)
	}
	total := 0
	for _, v := range append([][]byte{raw}, parents...) {
		if len(v) > MaxRawSize || len(v) > MaxEvidenceSize-total {
			return result, fmt.Errorf("%w: supplied bytes", ErrCapacity)
		}
		total += len(v)
	}
	if len(raw) == 0 {
		return result, fmt.Errorf("%w: commit", ErrUnavailable)
	}
	parsed, e := scanTransaction(ctx, raw, commitMode, nil)
	if e != nil {
		return result, e
	}
	c := parsed.tx
	if c.Version != 1 || c.LockTime != 0 {
		return result, invalid("version or locktime")
	}
	funding, e := checkCommitFunding(ctx, c, parents)
	if e != nil {
		return result, e
	}
	intent := Intent{PinID: pin, PriorStateID: prior, RecipientGym: recipient, FundingDigest: funding.digest}
	locator, e := CommitLocator(intent)
	if e != nil {
		return result, e
	}
	script := c.TxOut[0].PkScript
	if !bytes.Equal(c.TxOut[1].PkScript, locator) || len(script) != 23 || script[0] != 0xa9 || script[1] != 0x14 || script[22] != 0x87 {
		return result, invalid("commit locator or script")
	}
	if e = ctx.Err(); e != nil {
		return result, e
	}
	result.Intent = intent
	result.CommitTxID = parsed.hash
	result.Funding = funding.ops
	copy(result.FundingKey[:], c.TxIn[0].Witness[1])
	copy(result.ReturnScript[:], c.TxOut[2].PkScript)
	copy(result.CommitScript[:], script)
	return result, nil
}
