// Copyright (c) 2026 The SLUG developers. ISC license.
package walletwithdraw

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcutil/v2/txsort"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/mempool"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func hexValue(s string, n int, nonzero bool) bool {
	if n < 0 || len(s) != n*2 || s != strings.ToLower(s) {
		return false
	}
	b, e := hex.DecodeString(s)
	if e != nil {
		return false
	}
	if nonzero {
		for _, v := range b {
			if v != 0 {
				return true
			}
		}
		return false
	}
	return true
}
func recipient(s string) ([]byte, error) {
	if len(s) < 1 || len(s) > 90 {
		return nil, ErrInvalid
	}
	for i := range s {
		if s[i] < 33 || s[i] > 126 {
			return nil, ErrInvalid
		}
	}
	a, e := address.DecodeAddress(s, &chaincfg.MainNetParams)
	if e != nil || !a.IsForNet(&chaincfg.MainNetParams) || a.EncodeAddress() != s {
		return nil, ErrInvalid
	}
	switch a.(type) {
	case *address.AddressPubKeyHash, *address.AddressScriptHash, *address.AddressWitnessPubKeyHash, *address.AddressTaproot:
	default:
		return nil, ErrInvalid
	}
	p, e := txscript.PayToAddrScript(a)
	if e != nil || len(p) > 34 {
		return nil, ErrInvalid
	}
	return p, nil
}
func validOutput(atoms int64, script []byte) bool {
	return atoms > 0 && atoms <= maxMoney && !mempool.IsDust(wire.NewTxOut(atoms, script), 1000)
}
func requestScript(r Request) ([]byte, error) {
	if r.Version != 1 || !hexValue(r.OperationID, 32, true) {
		return nil, ErrInvalid
	}
	s, e := recipient(r.Recipient)
	if e != nil || !validOutput(r.AmountAtoms, s) {
		return nil, ErrInvalid
	}
	return s, nil
}
func digest(domain string, v any) string {
	raw, _ := json.Marshal(v)
	h := sha256.New()
	h.Write([]byte(domain))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

// ParseDraft validates bounded canonical syntax and static transaction grammar.
// It proves no wallet ownership, chain spendability or account authorization.
func ParseDraft(raw []byte) (Draft, error) {
	var d Draft
	if decode(raw, MaxDraftBytes, &d) != nil {
		return Draft{}, ErrInvalid
	}
	if _, e := validateDraft(d); e != nil {
		return Draft{}, e
	}
	return d, nil
}
func DraftDigest(d Draft) (string, error) {
	if _, e := validateDraft(d); e != nil {
		return "", e
	}
	return digest("SLUG candidate withdrawal draft\x00v1\x00", d), nil
}
func validateDraft(d Draft) (*wire.MsgTx, error) {
	bad := func() (*wire.MsgTx, error) { return nil, ErrInvalid }
	p, e := requestScript(Request{1, d.OperationID, d.Recipient, d.AmountAtoms})
	if e != nil || d.Version != 1 || !hexValue(d.BindingID, 32, true) || !hexValue(d.TipHash, 32, true) || d.TipHeight > math.MaxInt32 || d.RecipientScript != hex.EncodeToString(p) || d.FeeRateAtomsPerKB != 1000 || d.FeeCeilingAtoms != 100000 || d.MinConfirmations != 1 || d.TxVersion != 1 || d.InputSequence != math.MaxUint32 || d.LockTime != 0 || d.SighashType != 1 || len(d.Inputs) < 1 || len(d.Inputs) > 16 || len(d.UnsignedTxHex) > 2048 || len(d.UnsignedTxHex)%2 != 0 || !hexValue(d.UnsignedTxHex, len(d.UnsignedTxHex)/2, false) {
		return bad()
	}
	raw, _ := hex.DecodeString(d.UnsignedTxHex)
	if !transactionFrame(raw, false) {
		return bad()
	}
	tx := new(wire.MsgTx)
	r := bytes.NewReader(raw)
	if tx.DeserializeNoWitness(r) != nil || r.Len() != 0 || len(tx.TxIn) != len(d.Inputs) || !txsort.IsSorted(tx) {
		return bad()
	}
	var canon bytes.Buffer
	if tx.SerializeNoWitness(&canon) != nil || !bytes.Equal(raw, canon.Bytes()) {
		return bad()
	}
	seen := make(map[wire.OutPoint]bool, len(d.Inputs))
	var total int64
	for i, in := range d.Inputs {
		ti := tx.TxIn[i]
		if !hexValue(in.TxID, 32, false) || !hexValue(in.BlockHash, 32, true) || !hexValue(in.Script, 22, false) || !strings.HasPrefix(in.Script, "0014") || in.Branch > 1 || in.Index >= 4096 || in.BlockHeight > d.TipHeight || in.Atoms <= 0 || in.Atoms > maxMoney-total || ti.PreviousOutPoint.Hash.String() != in.TxID || ti.PreviousOutPoint.Index != in.OutputIndex || seen[ti.PreviousOutPoint] {
			return bad()
		}
		if in.Coinbase && int64(d.TipHeight)-int64(in.BlockHeight)+1 < int64(chaincfg.MainNetParams.CoinbaseMaturity) {
			return bad()
		}
		seen[ti.PreviousOutPoint] = true
		total += in.Atoms
	}
	if total != d.TotalInputAtoms || d.FeeAtoms < 1 || d.FeeAtoms > 100000 {
		return bad()
	}
	cp, e := recipient(d.Change.Address)
	if e != nil || !txscript.IsPayToWitnessPubKeyHash(cp) || d.Change.Script != hex.EncodeToString(cp) || d.Change.Index >= 4096 {
		return bad()
	}
	role := -1
	var outsum int64
	for i, out := range tx.TxOut {
		if !validOutput(out.Value, out.PkScript) || out.Value > maxMoney-outsum {
			return bad()
		}
		outsum += out.Value
		if role < 0 && out.Value == d.AmountAtoms && bytes.Equal(out.PkScript, p) {
			role = i
		}
	}
	if role < 0 || uint32(role) != d.RecipientOutputIndex || total-outsum != d.FeeAtoms {
		return bad()
	}
	// Fixed upstream native84 estimator: base + ceil((marker,count,109*n)/4).
	// One recipient plus one conservative change output are always estimated.
	n := int64(len(d.Inputs))
	fee := int64(10+wire.NewTxOut(d.AmountAtoms, p).SerializeSize()+31) + 41*n + (3+109*n+3)/4
	remainder := total - d.AmountAtoms - fee
	if remainder < 0 {
		return bad()
	}
	wantChange := remainder > 0 && validOutput(remainder, cp)
	if !wantChange {
		if len(tx.TxOut) != 1 || d.Change.OutputIndex != -1 || d.Change.Atoms != 0 || d.FeeAtoms != total-d.AmountAtoms {
			return bad()
		}
	} else {
		other := 1 - role
		if len(tx.TxOut) != 2 || d.Change.OutputIndex != int32(other) || d.Change.Atoms != remainder || tx.TxOut[other].Value != remainder || !bytes.Equal(tx.TxOut[other].PkScript, cp) || d.FeeAtoms != fee {
			return bad()
		}
	}
	encoded, e := json.Marshal(d)
	if e != nil || len(encoded) > MaxDraftBytes {
		return bad()
	}
	return tx, nil
}

// ValidateSigned verifies the exact saved unsigned bytes and every native84
// witness. It never signs or authorizes broadcast.
func ValidateSigned(d Draft, raw []byte) (Signed, error) {
	bad := func() (Signed, error) { return Signed{}, ErrInvalid }
	if _, e := validateDraft(d); e != nil {
		return bad()
	}
	if !transactionFrame(raw, true) {
		return bad()
	}
	tx := new(wire.MsgTx)
	r := bytes.NewReader(raw)
	if tx.Deserialize(r) != nil || r.Len() != 0 || len(tx.TxIn) != len(d.Inputs) {
		return bad()
	}
	var full, bare bytes.Buffer
	if tx.Serialize(&full) != nil || !bytes.Equal(full.Bytes(), raw) || tx.SerializeNoWitness(&bare) != nil || hex.EncodeToString(bare.Bytes()) != d.UnsignedTxHex {
		return bad()
	}
	f := txscript.NewMultiPrevOutFetcher(nil)
	for i, in := range d.Inputs {
		s, _ := hex.DecodeString(in.Script)
		f.AddPrevOut(tx.TxIn[i].PreviousOutPoint, wire.NewTxOut(in.Atoms, s))
	}
	hashes := txscript.NewTxSigHashes(tx, f)
	for i, in := range d.Inputs {
		s, _ := hex.DecodeString(in.Script)
		engine, e := txscript.NewEngine(s, tx, i, txscript.StandardVerifyFlags, nil, hashes, in.Atoms, f)
		if e != nil || engine.Execute() != nil {
			return bad()
		}
	}
	h := sha256.Sum256(raw)
	return Signed{hex.EncodeToString(raw), tx.TxHash().String(), hex.EncodeToString(h[:])}, nil
}
