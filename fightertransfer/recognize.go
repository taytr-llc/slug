// Copyright (c) 2026 The SLUG developers. ISC license.
package fightertransfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/taytr-llc/slug/btcec/v2/ecdsa"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"golang.org/x/crypto/ripemd160"
)

func hash160(raw []byte) [20]byte {
	h := sha256.Sum256(raw)
	r := ripemd160.New()
	r.Write(h[:])
	var out [20]byte
	copy(out[:], r.Sum(nil))
	return out
}
func nativeProgram(raw []byte) bool { return len(raw) == 22 && raw[0] == 0 && raw[1] == 20 }
func verifyDER(raw, digest, pub []byte) error {
	if len(raw) < 9 || len(raw) > 73 || raw[len(raw)-1] != byte(txscript.SigHashAll) {
		return invalid("funding SIGHASH_ALL")
	}
	der := raw[:len(raw)-1]
	if der[0] != 0x30 || int(der[1]) != len(der)-2 {
		return invalid("DER framing")
	}
	// Do not compare Serialize: it normalizes high-S. Outer consensus-valid high-S
	// is permitted, unlike the compact low-S gym authorization.
	sig, e := ecdsa.ParseDERSignature(der)
	if e != nil {
		return invalid("DER signature")
	}
	key, e := publicKey(pub)
	if e != nil {
		return e
	}
	if !sig.Verify(digest, key) {
		return invalid("funding signature")
	}
	return nil
}

// outerElements tokenizes the actual script rather than accepting a normalized
// replacement. Only the final two items are retained; all pushes/counts are checked.
func outerElements(ctx context.Context, raw []byte) (signature, redeem []byte, err error) {
	if len(raw) > MaxScriptSize {
		return nil, nil, invalid("executing script size")
	}
	t := txscript.MakeScriptTokenizer(0, raw)
	count := 0
	for t.Next() {
		if e := ctx.Err(); e != nil {
			return nil, nil, e
		}
		op := t.Opcode()
		var data []byte
		switch {
		case op <= txscript.OP_PUSHDATA4:
			data = t.Data()
		case op == txscript.OP_1NEGATE:
			data = []byte{0x81}
		case op >= txscript.OP_1 && op <= txscript.OP_16:
			data = []byte{op - txscript.OP_1 + 1}
		default:
			return nil, nil, invalid("non-push or reserved script opcode")
		}
		count++
		// The P2SH output script pushes its expected hash while the scriptSig
		// stack is still present, so one of the consensus 1000 slots is needed.
		if count > 999 || len(data) > 520 {
			return nil, nil, invalid("stack or element bound")
		}
		signature, redeem = redeem, data
	}
	if t.Err() != nil || count < 2 {
		return nil, nil, invalid("scriptSig framing")
	}
	return signature, redeem, nil
}

func evidenceBudget(ctx context.Context, e Evidence) error {
	if ctx == nil {
		return invalid("nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(e.FundingTransactions) > MaxInputs {
		return fmt.Errorf("%w: parent count", ErrCapacity)
	}
	total := 0
	for _, raw := range append([][]byte{e.CommitRaw, e.SpendRaw}, e.FundingTransactions...) {
		if len(raw) > MaxRawSize || len(raw) > MaxEvidenceSize-total {
			return fmt.Errorf("%w: supplied bytes", ErrCapacity)
		}
		total += len(raw)
	}
	if len(e.CommitRaw) == 0 || len(e.SpendRaw) == 0 {
		return fmt.Errorf("%w: commit or spend", ErrUnavailable)
	}
	return nil
}

// ValidateSupplied validates only the supplied cryptographic/transaction
// relationships. The core must separately authenticate Expected, complete actual
// parent/block evidence, canonical positions, predecessor chronology and finality.
// Candidate resource failures are ErrCapacity, never proof of an invalid send.
func ValidateSupplied(ctx context.Context, expected Expected, evidence Evidence) (Report, error) {
	var result Report
	if err := evidenceBudget(ctx, evidence); err != nil {
		return result, err
	}
	commit, err := scanTransaction(ctx, evidence.CommitRaw, commitMode, nil)
	if err != nil {
		return result, err
	}
	spend, err := scanTransaction(ctx, evidence.SpendRaw, spendMode, nil)
	if err != nil {
		return result, err
	}
	c, s := commit.tx, spend.tx
	if c.Version != 1 || c.LockTime != 0 || s.Version != 1 || s.LockTime != 0 {
		return result, invalid("version or locktime")
	}
	ops := make([]wire.OutPoint, len(c.TxIn))
	wanted := make(map[uint32]bool, len(c.TxIn))
	parentsWanted := make(map[chainhash.Hash]bool, len(c.TxIn))
	for n, in := range c.TxIn {
		if in.Sequence != wire.MaxTxInSequenceNum || len(in.SignatureScript) != 0 || len(in.Witness) != 2 {
			return result, invalid("native commit input")
		}
		if _, err = publicKey(in.Witness[1]); err != nil {
			return result, err
		}
		ops[n] = in.PreviousOutPoint
		wanted[in.PreviousOutPoint.Index] = true
		parentsWanted[in.PreviousOutPoint.Hash] = true
	}
	funding, err := FundingDigest(ops)
	if err != nil {
		return result, err
	}
	parents := make(map[chainhash.Hash]map[uint32]*wire.TxOut, len(evidence.FundingTransactions))
	for _, raw := range evidence.FundingTransactions {
		if len(raw) == 0 {
			return result, fmt.Errorf("%w: empty parent", ErrUnavailable)
		}
		p, err := scanTransaction(ctx, raw, parentMode, wanted)
		if err != nil {
			return result, err
		}
		if _, exists := parents[p.hash]; exists {
			return result, invalid("duplicate funding parent")
		}
		parents[p.hash] = p.outputs
	}
	fetch := txscript.NewMultiPrevOutFetcher(nil)
	var total int64
	for _, in := range c.TxIn {
		p, exists := parents[in.PreviousOutPoint.Hash]
		if !exists {
			return result, fmt.Errorf("%w: funding parent", ErrUnavailable)
		}
		out, exists := p[in.PreviousOutPoint.Index]
		if !exists {
			return result, invalid("funding output index")
		}
		if !nativeProgram(out.PkScript) || out.Value <= 0 || out.Value > maxMoney-total {
			return result, invalid("funding output")
		}
		keyHash := hash160(in.Witness[1])
		if !bytes.Equal(out.PkScript[2:], keyHash[:]) {
			return result, invalid("funding key program")
		}
		total += out.Value
		fetch.AddPrevOut(in.PreviousOutPoint, out)
	}
	// Extra well-formed parents have no authority role and are refused only after
	// missing required parents have been classified as unavailable.
	if len(parents) != len(parentsWanted) {
		return result, invalid("unreferenced funding parent")
	}
	if c.TxOut[0].Value != CommitAtoms || c.TxOut[1].Value != 0 || c.TxOut[2].Value < MinChangeAtoms || !nativeProgram(c.TxOut[2].PkScript) || total-c.TxOut[0].Value-c.TxOut[2].Value != LegFeeAtoms {
		return result, invalid("commit values or return")
	}
	hashes := txscript.NewTxSigHashes(c, fetch)
	for n, in := range c.TxIn {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		out := fetch.FetchPrevOutput(in.PreviousOutPoint)
		script := append([]byte{0x76, 0xa9, 0x14}, out.PkScript[2:]...)
		script = append(script, 0x88, 0xac)
		digest, e := txscript.CalcWitnessSigHash(script, hashes, txscript.SigHashAll, c, n, out.Value)
		if e != nil {
			return result, invalid("native signature hash")
		}
		if err = verifyDER(in.Witness[0], digest, in.Witness[1]); err != nil {
			return result, err
		}
	}
	in := s.TxIn[0]
	if in.PreviousOutPoint.Hash != commit.hash || in.PreviousOutPoint.Index != 0 || in.Sequence != wire.MaxTxInSequenceNum || len(in.Witness) != 0 {
		return result, invalid("spend input")
	}
	signature, redeem, err := outerElements(ctx, in.SignatureScript)
	if err != nil {
		return result, err
	}
	envelope, key, err := ParseRedeem(redeem)
	if err != nil {
		return result, err
	}
	if !bytes.Equal(key[:], c.TxIn[0].Witness[1]) {
		return result, invalid("first funding key")
	}
	program := hash160(redeem)
	expectedScript := append([]byte{0xa9, 0x14}, program[:]...)
	expectedScript = append(expectedScript, 0x87)
	if !bytes.Equal(c.TxOut[0].PkScript, expectedScript) {
		return result, invalid("P2SH commitment")
	}
	locator, err := CommitLocator(envelope.Intent)
	if err != nil {
		return result, err
	}
	if !bytes.Equal(locator, c.TxOut[1].PkScript) || envelope.Intent.FundingDigest != funding {
		return result, invalid("locator or funding tuple")
	}
	if err = VerifyAuthorization(expected, envelope); err != nil {
		return result, err
	}
	digest, err := txscript.CalcSignatureHash(redeem, txscript.SigHashAll, s, 0)
	if err != nil {
		return result, invalid("legacy signature hash")
	}
	if err = verifyDER(signature, digest, key[:]); err != nil {
		return result, err
	}
	if s.TxOut[0].Value != ReturnAtoms || !bytes.Equal(s.TxOut[0].PkScript, c.TxOut[2].PkScript) {
		return result, invalid("spend return")
	}
	kind := Refund
	if len(s.TxOut) == 2 {
		reveal, err := RevealLocator(envelope.Intent)
		if err != nil {
			return result, err
		}
		if s.TxOut[1].Value != 0 || !bytes.Equal(s.TxOut[1].PkScript, reveal) {
			return result, invalid("reveal locator")
		}
		kind = Reveal
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	id, err := TransferID(envelope.Intent)
	if err != nil {
		return result, err
	}
	result = Report{Kind: kind, Envelope: envelope, TransferID: id, CommitTxID: commit.hash, SpendTxID: spend.hash, FundingKey: key, Funding: ops}
	copy(result.ReturnScript[:], c.TxOut[2].PkScript)
	return result, nil
}
