// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"math"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func resultFundingKey() (*btcec.PrivateKey, []byte, []byte) {
	var scalar [32]byte
	scalar[31] = 2
	key, pub := btcec.PrivKeyFromBytes(scalar[:])
	hash := address.Hash160(pub.SerializeCompressed())
	script := append([]byte{txscript.OP_0, 20}, hash...)
	code := append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, hash...)
	code = append(code, txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
	return key, script, code
}

func validResultCarrier(c [73]byte) bool {
	return string(c[:8]) == "SLUGRSLT" && c[8] == 1 && !bytes.Equal(c[9:41], make([]byte, 32)) && !bytes.Equal(c[41:], make([]byte, 32))
}
func validResultCoin(c FundingCoin) bool {
	_, script, _ := resultFundingKey()
	return hexValue(c.Outpoint.TxID, 32, true) && c.ValueSatoshis >= FeeSatoshis+MinimumChangeSatoshis && c.ValueSatoshis <= btcutil.MaxSatoshi && c.Script == hex.EncodeToString(script) && c.BlockHeight <= math.MaxInt32 && hexValue(c.BlockHash, 32, true)
}

func (p *ResultPublisher) unspent(ctx context.Context, outpoint Outpoint) (*UnspentOutput, error) {
	if !hexValue(outpoint.TxID, 32, true) {
		return nil, ErrInvalid
	}
	tip, e := p.owner.observe(ctx)
	if e != nil {
		return nil, e
	}
	raw, e := p.owner.nodes[0].call(ctx, "gettxout", []any{outpoint.TxID, outpoint.Index, true})
	if e != nil {
		return nil, e
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	m, e := fields(raw, "bestblock", "confirmations", "value", "scriptPubKey", "coinbase")
	if e != nil {
		return nil, e
	}
	var output UnspentOutput
	if json.Unmarshal(m["bestblock"], &output.BestBlock) != nil || !hexValue(output.BestBlock, 32, true) || json.Unmarshal(m["confirmations"], &output.Confirmations) != nil || json.Unmarshal(m["coinbase"], &output.Coinbase) != nil {
		return nil, ErrUnavailable
	}
	// gettxout may observe a normal extension after the captured tip. Bind its
	// own bestblock to an exact canonical height instead of requiring tip equality.
	observedHeight := tip.Height
	if output.BestBlock != tip.Hash {
		header, e := p.owner.nodes[0].call(ctx, "getblockheader", []any{output.BestBlock, true})
		h, e2 := fields(header, "hash", "height")
		var hash string
		if e != nil || e2 != nil || json.Unmarshal(h["hash"], &hash) != nil || hash != output.BestBlock || json.Unmarshal(h["height"], &observedHeight) != nil || observedHeight < tip.Height || observedHeight > math.MaxInt32 {
			return nil, ErrUnavailable
		}
		if hash, e := p.owner.canonical(ctx, observedHeight); e != nil || hash != output.BestBlock {
			return nil, ErrUnavailable
		}
	}
	if output.Confirmations > observedHeight+1 {
		return nil, ErrUnavailable
	}
	output.ValueSatoshis, e = amountSatoshis(m["value"])
	if e != nil {
		return nil, e
	}
	script, e := fields(m["scriptPubKey"], "hex")
	_, fundingScript, _ := resultFundingKey()
	if e != nil || json.Unmarshal(script["hex"], &output.Script) != nil || output.Script != hex.EncodeToString(fundingScript) {
		return nil, ErrUnavailable
	}
	if hash, e := p.owner.canonical(ctx, tip.Height); e != nil || hash != tip.Hash {
		return nil, ErrUnavailable
	}
	return &output, nil
}
func (p *ResultPublisher) Unspent(ctx context.Context, outpoint Outpoint) (*UnspentOutput, error) {
	c, end, e := p.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer end()
	out, e := p.unspent(c, outpoint)
	if e != nil {
		return nil, e
	}
	if e = p.finish(c); e != nil {
		return nil, e
	}
	return out, nil
}

func (p *ResultPublisher) checkCoin(ctx context.Context, coin FundingCoin) error {
	if !validResultCoin(coin) {
		return ErrInvalid
	}
	out, e := p.unspent(ctx, coin.Outpoint)
	if e != nil {
		return e
	}
	if out == nil {
		return ErrUnavailable
	}
	if out.ValueSatoshis != coin.ValueSatoshis || out.Script != coin.Script || out.Coinbase != coin.Coinbase || out.Confirmations == 0 {
		return ErrUnavailable
	}
	tip, e := p.owner.observe(ctx)
	if e != nil {
		return e
	}
	if coin.BlockHeight > tip.Height || uint64(coin.BlockHeight)+uint64(out.Confirmations)-1 > uint64(tip.Height) {
		return ErrUnavailable
	}
	observedHeight := coin.BlockHeight + out.Confirmations - 1
	if hash, e := p.owner.canonical(ctx, observedHeight); e != nil || hash != out.BestBlock {
		return ErrUnavailable
	}
	if tip.Height+1 < 62 || (coin.Coinbase && tip.Height+1-coin.BlockHeight < 100) {
		return ErrNotMature
	}
	hash, e := p.owner.canonical(ctx, coin.BlockHeight)
	if e != nil || hash != coin.BlockHash {
		return ErrUnavailable
	}
	return nil
}

// publication creates only the fixed key2 native witness transaction. This
// unexported function never accepts a key, destination, fee or signing mode.
func resultPublication(carrier [73]byte, coin FundingCoin) (*wire.MsgTx, []byte, error) {
	if !validResultCarrier(carrier) || !validResultCoin(coin) {
		return nil, nil, ErrInvalid
	}
	key, script, code := resultFundingKey()
	hash, _ := chainhash.NewHashFromStr(coin.Outpoint.TxID)
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, coin.Outpoint.Index), nil, nil))
	tx.AddTxOut(wire.NewTxOut(0, carrierScript(carrier)))
	tx.AddTxOut(wire.NewTxOut(coin.ValueSatoshis-FeeSatoshis, script))
	fetcher := txscript.NewCannedPrevOutputFetcher(script, coin.ValueSatoshis)
	sigHashes := txscript.NewTxSigHashes(tx, fetcher)
	witness, e := txscript.WitnessSignature(tx, sigHashes, 0, coin.ValueSatoshis, code, txscript.SigHashAll, key, true)
	if e != nil {
		return nil, nil, ErrInvalid
	}
	tx.TxIn[0].Witness = witness
	engine, e := txscript.NewEngine(script, tx, 0, txscript.StandardVerifyFlags, nil, sigHashes, coin.ValueSatoshis, fetcher)
	if e != nil || engine.Execute() != nil {
		return nil, nil, ErrInvalid
	}
	var b bytes.Buffer
	if tx.Serialize(&b) != nil || b.Len() > MaxTransactionSize {
		return nil, nil, ErrInvalid
	}
	return tx, b.Bytes(), nil
}

func (p *ResultPublisher) BuildPublication(ctx context.Context, carrier [73]byte, coin FundingCoin) ([]byte, error) {
	c, end, e := p.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer end()
	if !validResultCarrier(carrier) || !validResultCoin(coin) {
		return nil, ErrInvalid
	}
	if e = p.checkCoin(c, coin); e != nil {
		return nil, e
	}
	_, raw, e := resultPublication(carrier, coin)
	if e != nil {
		return nil, e
	}
	if e = p.checkCoin(c, coin); e != nil {
		return nil, e
	}
	if e = p.finish(c); e != nil {
		return nil, e
	}
	return raw, nil
}

// validateResultTransaction verifies retained bytes rather than reconstructing
// a deterministic signature. An alternative valid low-S signature is allowed;
// the caller's journal still owns the exact originally retained witness bytes.
func validateResultTransaction(carrier [73]byte, coin FundingCoin, raw []byte) (*wire.MsgTx, error) {
	if !validResultCarrier(carrier) || !validResultCoin(coin) || len(raw) == 0 || len(raw) > MaxTransactionSize {
		return nil, ErrInvalid
	}
	r := bytes.NewReader(raw)
	tx := new(wire.MsgTx)
	if tx.Deserialize(r) != nil || r.Len() != 0 || tx.Version != 2 || tx.LockTime != 0 || len(tx.TxIn) != 1 || len(tx.TxOut) != 2 {
		return nil, ErrInvalid
	}
	key, script, _ := resultFundingKey()
	in := tx.TxIn[0]
	if in.PreviousOutPoint.Hash.String() != coin.Outpoint.TxID || in.PreviousOutPoint.Index != coin.Outpoint.Index || in.Sequence != wire.MaxTxInSequenceNum || len(in.SignatureScript) != 0 || len(in.Witness) != 2 || len(in.Witness[0]) < 2 || in.Witness[0][len(in.Witness[0])-1] != byte(txscript.SigHashAll) || !bytes.Equal(in.Witness[1], key.PubKey().SerializeCompressed()) || tx.TxOut[0].Value != 0 || !bytes.Equal(tx.TxOut[0].PkScript, carrierScript(carrier)) || tx.TxOut[1].Value != coin.ValueSatoshis-FeeSatoshis || !bytes.Equal(tx.TxOut[1].PkScript, script) {
		return nil, ErrInvalid
	}
	var canonical bytes.Buffer
	if tx.Serialize(&canonical) != nil || !bytes.Equal(raw, canonical.Bytes()) {
		return nil, ErrInvalid
	}
	fetcher := txscript.NewCannedPrevOutputFetcher(script, coin.ValueSatoshis)
	hashes := txscript.NewTxSigHashes(tx, fetcher)
	engine, err := txscript.NewEngine(script, tx, 0, txscript.StandardVerifyFlags, nil, hashes, coin.ValueSatoshis, fetcher)
	if err != nil || engine.Execute() != nil {
		return nil, ErrInvalid
	}
	return tx, nil
}

func (p *ResultPublisher) SubmitPublication(ctx context.Context, carrier [73]byte, coin FundingCoin, raw []byte) (string, error) {
	c, end, err := p.begin(ctx)
	if err != nil {
		return "", err
	}
	defer end()
	tx, err := validateResultTransaction(carrier, coin, raw)
	if err != nil {
		return "", err
	}
	if err = p.checkCoin(c, coin); err != nil {
		return "", err
	}
	if err = p.finish(c); err != nil {
		return "", err
	}
	result, err := p.owner.nodes[0].call(c, "sendrawtransaction", []any{hex.EncodeToString(raw), false})
	// Never retry, rebuild or replace a possible dispatch.
	var txid string
	if err != nil || json.Unmarshal(result, &txid) != nil || txid != tx.TxHash().String() {
		return "", ErrOutcomeUnknown
	}
	if _, err = p.owner.observe(c); err != nil {
		return "", ErrOutcomeUnknown
	}
	return txid, nil
}
