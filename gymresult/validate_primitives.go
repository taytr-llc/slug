// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
)

func hexSize(s string, n int) bool {
	if len(s) != n*2 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func hashID(s string) bool     { return hexSize(s, 32) }
func optionalID(s string) bool { return s == "" || hashID(s) }
func timestamp(n int64) bool   { return n > 0 && n <= MaxTimestampMS }
func fits(v any, n int) bool   { b, e := json.Marshal(v); return e == nil && len(b) <= n }
func validatePin(p Pin) error {
	want, e := CandidatePin(p.SourcePin)
	if e != nil || !equal(p, want) {
		return ErrInvalid
	}
	return nil
}
func validateOutpoint(_ Pin, v Outpoint) error {
	if !hashID(v.TxID) {
		return ErrInvalid
	}
	return nil
}
func validateFundingCoin(p Pin, v FundingCoin) error {
	if validateOutpoint(p, v.Outpoint) != nil || v.ValueSatoshis <= 0 || v.ValueSatoshis > btcutil.MaxSatoshi || v.Script != p.FundingScript || v.BlockHeight > math.MaxInt32 || !hashID(v.BlockHash) {
		return ErrInvalid
	}
	return nil
}
func validatePublication(p Pin, v Publication) error {
	if v.Version != 1 || !hexSize(v.RawTx, len(v.RawTx)/2) || len(v.RawTx) == 0 || len(v.RawTx) > 2*MaxRawTxSize || !hashID(v.TxID) || validateFundingCoin(p, v.Coin) != nil || !timestamp(v.BuiltAtMS) {
		return ErrInvalid
	}
	if v.DispatchCount == 0 {
		if v.State != "built" || v.FirstDispatchAtMS != 0 || v.LastDispatchAtMS != 0 {
			return ErrInvalid
		}
	} else {
		if (v.State != "dispatch_unknown" && v.State != "submitted") || !timestamp(v.FirstDispatchAtMS) || !timestamp(v.LastDispatchAtMS) || v.FirstDispatchAtMS < v.BuiltAtMS || v.LastDispatchAtMS < v.FirstDispatchAtMS {
			return ErrInvalid
		}
	}
	raw := rawID(v.RawTx)
	reader := bytes.NewReader(raw)
	var tx wire.MsgTx
	if tx.Deserialize(reader) != nil || reader.Len() != 0 || tx.Version != 2 || tx.LockTime != 0 || len(tx.TxIn) != 1 || len(tx.TxOut) != 2 || tx.TxHash().String() != v.TxID {
		return ErrInvalid
	}
	var encoded bytes.Buffer
	if tx.Serialize(&encoded) != nil || !bytes.Equal(encoded.Bytes(), raw) {
		return ErrInvalid
	}
	in := tx.TxIn[0]
	if in.PreviousOutPoint.Hash.String() != v.Coin.Outpoint.TxID || in.PreviousOutPoint.Index != v.Coin.Outpoint.Index || in.Sequence != math.MaxUint32 || len(in.SignatureScript) != 0 || len(in.Witness) != 2 || !bytes.Equal(in.Witness[1], rawID(p.FundingPublicKey)) || len(in.Witness[0]) < 2 || in.Witness[0][len(in.Witness[0])-1] != byte(txscript.SigHashAll) {
		return ErrInvalid
	}
	if v.Coin.ValueSatoshis < 1294 || tx.TxOut[0].Value != 0 || len(tx.TxOut[0].PkScript) != 75 || tx.TxOut[0].PkScript[0] != txscript.OP_RETURN || tx.TxOut[0].PkScript[1] != 73 || !bytes.Equal(tx.TxOut[0].PkScript[2:10], []byte("SLUGRSLT")) || tx.TxOut[0].PkScript[10] != 1 || bytes.Equal(tx.TxOut[0].PkScript[11:43], make([]byte, 32)) || bytes.Equal(tx.TxOut[0].PkScript[43:], make([]byte, 32)) || tx.TxOut[1].Value != v.Coin.ValueSatoshis-1000 || !bytes.Equal(tx.TxOut[1].PkScript, rawID(p.FundingScript)) {
		return ErrInvalid
	}
	fetch := txscript.NewCannedPrevOutputFetcher(rawID(p.FundingScript), v.Coin.ValueSatoshis)
	hashes := txscript.NewTxSigHashes(&tx, fetch)
	vm, e := txscript.NewEngine(rawID(p.FundingScript), &tx, 0, txscript.StandardVerifyFlags, nil, hashes, v.Coin.ValueSatoshis, fetch)
	if e != nil || vm.Execute() != nil {
		return ErrInvalid
	}
	return nil
}
func validateCursor(_ Pin, v Cursor) error {
	if v.Generation > MaxScanRestarts || v.Height < -1 || v.CompleteToHeight < -1 || v.CompleteToHeight > v.Height || !optionalID(v.Hash) || !optionalID(v.CompleteToHash) || !hashID(v.ScanDigest) || (v.Height < 0) != (v.Hash == "") || (v.CompleteToHeight < 0) != (v.CompleteToHash == "") {
		return ErrInvalid
	}
	if v.Height == v.CompleteToHeight && v.Hash != v.CompleteToHash {
		return ErrInvalid
	}
	return nil
}
func validateTip(_ Pin, v Tip) error {
	if v.Height > math.MaxInt32 || !hashID(v.Hash) {
		return ErrInvalid
	}
	return nil
}
func validateScanRestart(p Pin, v ScanRestart) error {
	if v.Generation < 1 || v.Generation > MaxScanRestarts || validateCursor(p, v.Previous) != nil || v.Previous.Generation+1 != v.Generation || v.Previous.Height < 0 || !timestamp(v.ObservedAtMS) || validateTip(p, v.ObservedTip) != nil || !optionalID(v.ReplacementHash) {
		return ErrInvalid
	}
	if v.ObservedTip.Height < uint32(v.Previous.Height) {
		if v.ReplacementHash != "" {
			return ErrInvalid
		}
	} else if !hashID(v.ReplacementHash) || v.ReplacementHash == v.Previous.Hash {
		return ErrInvalid
	}
	return nil
}
func header(raw string) (wire.BlockHeader, error) {
	var h wire.BlockHeader
	if !hexSize(raw, 80) {
		return h, ErrInvalid
	}
	if h.Deserialize(bytes.NewReader(rawID(raw))) != nil {
		return h, ErrInvalid
	}
	return h, nil
}
func validateAnchor(p Pin, v Anchor) error {
	if v.Height == 0 || v.Height > math.MaxInt32 || !hashID(v.Hash) || !hashID(v.TxID) || !hashID(v.RawTxDigest) || v.RawTxSize == 0 || v.RawTxSize > MaxRawTxSize || v.TxCount == 0 || v.TxCount > 400000 || v.TxIndex >= v.TxCount || v.OutputIndex != 0 || !hexSize(v.Payload, 73) || v.MerkleBranch == nil || len(v.MerkleBranch) > 20 {
		return ErrInvalid
	}
	payload := rawID(v.Payload)
	if !bytes.Equal(payload[:8], []byte("SLUGRSLT")) || payload[8] != 1 || bytes.Equal(payload[9:41], make([]byte, 32)) || bytes.Equal(payload[41:], make([]byte, 32)) {
		return ErrInvalid
	}
	h, e := header(v.Header)
	if e != nil {
		return e
	}
	leaf, e := chainhash.NewHashFromStr(v.TxID)
	if e != nil {
		return ErrInvalid
	}
	index, count, used := v.TxIndex, v.TxCount, 0
	for count > 1 {
		if used >= len(v.MerkleBranch) || !hashID(v.MerkleBranch[used]) {
			return ErrInvalid
		}
		sib, e := chainhash.NewHashFromStr(v.MerkleBranch[used])
		if e != nil {
			return ErrInvalid
		}
		if index%2 == 0 && index+1 == count && *sib != *leaf {
			return ErrInvalid
		}
		var pair []byte
		if index%2 == 0 {
			pair = append(append([]byte{}, leaf[:]...), sib[:]...)
		} else {
			pair = append(append([]byte{}, sib[:]...), leaf[:]...)
		}
		sum := chainhash.DoubleHashH(pair)
		leaf = &sum
		index /= 2
		count = (count + 1) / 2
		used++
	}
	if used != len(v.MerkleBranch) || *leaf != h.MerkleRoot {
		return ErrInvalid
	}
	return nil
}
func validCheckpoint(p Pin, c chaincfg.SignedCheckpoint) bool {
	if c.Network != wire.BitcoinNet(p.Network) || c.GenesisHash.String() != p.Genesis || c.Height <= 0 || len(c.Signature) != 64 {
		return false
	}
	params := chaincfg.MainNetParams
	return c.Verify(&params, rawID(p.CheckpointPublicKey)) == nil
}
