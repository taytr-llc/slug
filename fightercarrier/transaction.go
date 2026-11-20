// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/fightertransfer"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

// txFrame rejects hostile CompactSize counts before wire allocates. This is
// the bounded private writer grammar, not the broader public replay grammar.
type frameCursor struct {
	b []byte
	i int
}

func (c *frameCursor) take(n uint64) ([]byte, bool) {
	if n > uint64(len(c.b)-c.i) {
		return nil, false
	}
	b := c.b[c.i : c.i+int(n)]
	c.i += int(n)
	return b, true
}
func (c *frameCursor) count() (uint64, bool) {
	b, ok := c.take(1)
	if !ok {
		return 0, false
	}
	switch b[0] {
	case 253:
		b, ok = c.take(2)
		if !ok {
			return 0, false
		}
		n := uint64(binary.LittleEndian.Uint16(b))
		return n, n >= 253
	case 254:
		b, ok = c.take(4)
		if !ok {
			return 0, false
		}
		n := uint64(binary.LittleEndian.Uint32(b))
		return n, n >= 65536
	case 255:
		b, ok = c.take(8)
		if !ok {
			return 0, false
		}
		n := binary.LittleEndian.Uint64(b)
		return n, n >= 1<<32
	default:
		return uint64(b[0]), true
	}
}
func (c *frameCursor) blob(limit uint64) ([]byte, bool) {
	n, ok := c.count()
	if !ok || n > limit {
		return nil, false
	}
	return c.take(n)
}
func txFrame(raw []byte, commit bool) bool {
	c := frameCursor{b: raw}
	v, ok := c.take(4)
	if !ok || binary.LittleEndian.Uint32(v) != 1 {
		return false
	}
	if commit {
		v, ok = c.take(2)
		if !ok || !bytes.Equal(v, []byte{0, 1}) {
			return false
		}
	}
	n, ok := c.count()
	if !ok || n < 1 || n > 16 || !commit && n != 1 {
		return false
	}
	for i := uint64(0); i < n; i++ {
		if _, ok = c.take(36); !ok {
			return false
		}
		s, k := c.blob(316)
		if !k || commit && len(s) != 0 {
			return false
		}
		v, ok = c.take(4)
		if !ok || binary.LittleEndian.Uint32(v) != wire.MaxTxInSequenceNum {
			return false
		}
	}
	outs, ok := c.count()
	if !ok || commit && outs != 3 || !commit && (outs < 1 || outs > 2) {
		return false
	}
	for i := uint64(0); i < outs; i++ {
		v, ok = c.take(8)
		if !ok || binary.LittleEndian.Uint64(v) > uint64(MaxMoney) {
			return false
		}
		if _, ok = c.blob(76); !ok {
			return false
		}
	}
	if commit {
		for i := uint64(0); i < n; i++ {
			v, ok := c.count()
			if !ok || v != 2 {
				return false
			}
			sig, k := c.blob(73)
			if !k || len(sig) < 9 || sig[len(sig)-1] != 1 {
				return false
			}
			pub, k := c.blob(33)
			if !k || len(pub) != 33 || !point(hex.EncodeToString(pub)) {
				return false
			}
		}
	}
	v, ok = c.take(4)
	return ok && binary.LittleEndian.Uint32(v) == 0 && c.i == len(raw)
}
func parseWriterRaw(s string, cap int, commit bool) (*wire.MsgTx, error) {
	if len(s) == 0 || len(s) > 2*cap || len(s)%2 != 0 || !hexValue(s, len(s)/2, false) {
		return nil, ErrInvalid
	}
	b, _ := hex.DecodeString(s)
	if !txFrame(b, commit) {
		return nil, ErrInvalid
	}
	tx := new(wire.MsgTx)
	r := bytes.NewReader(b)
	if tx.Deserialize(r) != nil || r.Len() != 0 {
		return nil, ErrInvalid
	}
	var out bytes.Buffer
	if tx.Serialize(&out) != nil || !bytes.Equal(out.Bytes(), b) {
		return nil, ErrInvalid
	}
	return tx, nil
}
func checkCommit(p Preparation, tx *wire.MsgTx, signature string) error {
	intent, e := reviewIntent(p.Review)
	if e != nil {
		return e
	}
	var envelope fightertransfer.Envelope
	envelope.Intent = intent
	if !hexValue(signature, 64, false) {
		return ErrInvalid
	}
	sig, _ := hex.DecodeString(signature)
	copy(envelope.Signature[:], sig)
	expected := fightertransfer.Expected{PinID: intent.PinID, PriorStateID: intent.PriorStateID}
	b, _ := hex.DecodeString(p.Review.SenderGym)
	copy(expected.SenderGym[:], b)
	if fightertransfer.VerifyAuthorization(expected, envelope) != nil {
		return ErrInvalid
	}
	var key [33]byte
	copy(key[:], tx.TxIn[0].Witness[1])
	redeem, e := fightertransfer.RedeemScript(envelope, key)
	if e != nil {
		return ErrInvalid
	}
	script := append([]byte{0xa9, 0x14}, address.Hash160(redeem)...)
	script = append(script, 0x87)
	locator, e := fightertransfer.CommitLocator(intent)
	if e != nil || !bytes.Equal(script, tx.TxOut[0].PkScript) || !bytes.Equal(locator, tx.TxOut[1].PkScript) || tx.TxOut[0].Value != 500000 || tx.TxOut[1].Value != 0 || tx.TxOut[2].Value < 1000 || tx.TxOut[2].Value > MaxMoney-500000 || hex.EncodeToString(tx.TxOut[2].PkScript) != p.Review.ReturnScript {
		return ErrInvalid
	}
	ops := make([]wire.OutPoint, len(tx.TxIn))
	for i, in := range tx.TxIn {
		ops[i] = in.PreviousOutPoint
	}
	fund, e := fightertransfer.FundingDigest(ops)
	if e != nil || fund != intent.FundingDigest {
		return ErrInvalid
	}
	if p.NewFunding != nil {
		n := p.NewFunding
		if len(tx.TxIn) != len(n.Inputs) || tx.TxOut[2].Value != n.CommitChangeAtoms {
			return ErrInvalid
		}
		fetch := txscript.NewMultiPrevOutFetcher(nil)
		for i, v := range n.Inputs {
			in := tx.TxIn[i]
			if in.PreviousOutPoint.Hash.String() != v.TxID || in.PreviousOutPoint.Index != v.OutputIndex || hex.EncodeToString(address.Hash160(in.Witness[1])) != v.Script[4:] {
				return ErrInvalid
			}
			script, _ := hex.DecodeString(v.Script)
			fetch.AddPrevOut(in.PreviousOutPoint, wire.NewTxOut(v.Atoms, script))
		}
		hashes := txscript.NewTxSigHashes(tx, fetch)
		for i, v := range n.Inputs {
			script, _ := hex.DecodeString(v.Script)
			engine, e := txscript.NewEngine(script, tx, i, txscript.StandardVerifyFlags, nil, hashes, v.Atoms, fetch)
			if e != nil || engine.Execute() != nil {
				return ErrInvalid
			}
		}
	}
	return nil
}

func ValidateSignedPackage(v SignedPackage, p Preparation, pin Pin) error {
	if ValidatePreparation(p, pin, p.BindingID) != nil || v.Version != 1 || v.ReviewDigest != p.ReviewDigest || v.TransferID != p.Review.TransferID || !hexValue(v.GymAuthorization, 64, false) {
		return ErrInvalid
	}
	commit, e := parseWriterRaw(v.Commit.Raw, 4096, true)
	if e != nil {
		return e
	}
	spend, e := parseWriterRaw(v.Spend.Raw, 512, false)
	if e != nil {
		return e
	}
	for _, r := range []struct {
		value RawTransaction
		tx    *wire.MsgTx
	}{{v.Commit, commit}, {v.Spend, spend}} {
		b, _ := hex.DecodeString(r.value.Raw)
		if r.value.TxID != r.tx.TxHash().String() || r.value.RawSHA256 != hashBytes(b) {
			return ErrInvalid
		}
	}
	if p.RecoveryFunding != nil && (v.Commit.Raw != p.RecoveryFunding.CommitRaw || v.Commit.TxID != p.RecoveryFunding.CommitTxID || v.GymAuthorization != p.RecoveryFunding.GymAuthorization) {
		return ErrInvalid
	}
	if e = checkCommit(p, commit, v.GymAuthorization); e != nil {
		return e
	}
	in := spend.TxIn[0]
	if in.PreviousOutPoint.Hash != commit.TxHash() || in.PreviousOutPoint.Index != 0 || len(in.Witness) != 0 {
		return ErrInvalid
	}
	pushes, e := txscript.PushedData(in.SignatureScript)
	if e != nil || len(pushes) != 2 || len(pushes[0]) < 9 || len(pushes[0]) > 73 || pushes[0][len(pushes[0])-1] != 1 {
		return ErrInvalid
	}
	canonical, e := txscript.NewScriptBuilder().AddData(pushes[0]).AddData(pushes[1]).Script()
	if e != nil || !bytes.Equal(canonical, in.SignatureScript) {
		return ErrInvalid
	}
	envelope, key, e := fightertransfer.ParseRedeem(pushes[1])
	if e != nil || !bytes.Equal(key[:], commit.TxIn[0].Witness[1]) || hex.EncodeToString(envelope.Signature[:]) != v.GymAuthorization {
		return ErrInvalid
	}
	intent, e := reviewIntent(p.Review)
	if e != nil || envelope.Intent != intent {
		return ErrInvalid
	}
	want := "reveal"
	if p.Request.Kind == "recover_refund" {
		want = "refund"
	}
	if v.SpendKind != want || spend.TxOut[0].Value != 490000 || !bytes.Equal(spend.TxOut[0].PkScript, commit.TxOut[2].PkScript) {
		return ErrInvalid
	}
	if want == "refund" {
		if len(spend.TxOut) != 1 {
			return ErrInvalid
		}
	} else {
		loc, e := fightertransfer.RevealLocator(intent)
		if e != nil || len(spend.TxOut) != 2 || spend.TxOut[1].Value != 0 || !bytes.Equal(spend.TxOut[1].PkScript, loc) {
			return ErrInvalid
		}
	}
	fetch := txscript.NewCannedPrevOutputFetcher(commit.TxOut[0].PkScript, 500000)
	engine, e := txscript.NewEngine(commit.TxOut[0].PkScript, spend, 0, txscript.StandardVerifyFlags, nil, nil, 500000, fetch)
	if e != nil || engine.Execute() != nil {
		return ErrInvalid
	}
	return bounded(v, MaxSignedPackageBytes)
}
