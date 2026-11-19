// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

var amountPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// amountSatoshis preserves exact decimal RPC values, including the node's 1e-8.
func amountSatoshis(raw []byte) (int64, error) {
	s := string(raw)
	if len(s) > 128 || !amountPattern.MatchString(s) {
		return 0, ErrUnavailable
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		exp, e := strconv.ParseInt(s[i+1:], 10, 32)
		if e != nil || exp < -32 || exp > 32 {
			return 0, ErrUnavailable
		}
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, ErrUnavailable
	}
	r.Mul(r, new(big.Rat).SetInt64(100000000))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, ErrUnavailable
	}
	n := r.Num().Int64()
	if n < 0 || n > btcutil.MaxSatoshi {
		return 0, ErrUnavailable
	}
	return n, nil
}

func fundingKey() (*btcec.PrivateKey, []byte, []byte) {
	var scalar [32]byte
	scalar[31] = 1
	key, pub := btcec.PrivKeyFromBytes(scalar[:])
	hash := address.Hash160(pub.SerializeCompressed())
	script := append([]byte{txscript.OP_0, 20}, hash...)
	code := append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, hash...)
	code = append(code, txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
	return key, script, code
}

func validCarrier(c [73]byte) bool {
	return string(c[:8]) == "SLUGSEED" && c[8] == 1 && !bytes.Equal(c[9:41], make([]byte, 32)) && !bytes.Equal(c[41:], make([]byte, 32))
}
func carrierScript(c [73]byte) []byte { return append([]byte{txscript.OP_RETURN, 73}, c[:]...) }
func validCoin(c FundingCoin) bool {
	_, script, _ := fundingKey()
	return hexValue(c.Outpoint.TxID, 32, true) && c.ValueSatoshis >= FeeSatoshis+MinimumChangeSatoshis && c.ValueSatoshis <= btcutil.MaxSatoshi && c.Script == hex.EncodeToString(script) && c.BlockHeight <= math.MaxInt32 && hexValue(c.BlockHash, 32, true)
}

func (p *Publisher) unspent(ctx context.Context, outpoint Outpoint) (*UnspentOutput, error) {
	if !hexValue(outpoint.TxID, 32, true) {
		return nil, ErrInvalid
	}
	tip, e := p.observe(ctx)
	if e != nil {
		return nil, e
	}
	raw, e := p.nodes[0].call(ctx, "gettxout", []any{outpoint.TxID, outpoint.Index, true})
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
		header, e := p.nodes[0].call(ctx, "getblockheader", []any{output.BestBlock, true})
		h, e2 := fields(header, "hash", "height")
		var hash string
		if e != nil || e2 != nil || json.Unmarshal(h["hash"], &hash) != nil || hash != output.BestBlock || json.Unmarshal(h["height"], &observedHeight) != nil || observedHeight < tip.Height || observedHeight > math.MaxInt32 {
			return nil, ErrUnavailable
		}
		if hash, e := p.canonical(ctx, observedHeight); e != nil || hash != output.BestBlock {
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
	_, fundingScript, _ := fundingKey()
	if e != nil || json.Unmarshal(script["hex"], &output.Script) != nil || output.Script != hex.EncodeToString(fundingScript) {
		return nil, ErrUnavailable
	}
	if hash, e := p.canonical(ctx, tip.Height); e != nil || hash != tip.Hash {
		return nil, ErrUnavailable
	}
	return &output, nil
}
func (p *Publisher) Unspent(ctx context.Context, outpoint Outpoint) (*UnspentOutput, error) {
	c, end, e := p.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer end()
	out, e := p.unspent(c, outpoint)
	if e == nil {
		e = p.finishShared(c)
	}
	if e != nil {
		return nil, e
	}
	return out, nil
}

func (p *Publisher) checkCoin(ctx context.Context, coin FundingCoin) error {
	if !validCoin(coin) {
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
	tip, e := p.observe(ctx)
	if e != nil {
		return e
	}
	if coin.BlockHeight > tip.Height || uint64(coin.BlockHeight)+uint64(out.Confirmations)-1 > uint64(tip.Height) {
		return ErrUnavailable
	}
	observedHeight := coin.BlockHeight + out.Confirmations - 1
	if hash, e := p.canonical(ctx, observedHeight); e != nil || hash != out.BestBlock {
		return ErrUnavailable
	}
	if tip.Height+1 < 62 || (coin.Coinbase && tip.Height+1-coin.BlockHeight < 100) {
		return ErrNotMature
	}
	hash, e := p.canonical(ctx, coin.BlockHeight)
	if e != nil || hash != coin.BlockHash {
		return ErrUnavailable
	}
	return nil
}

// publication creates only the fixed key1 native witness transaction. This
// unexported function never accepts a key, destination, fee or signing mode.
func publication(carrier [73]byte, coin FundingCoin) (*wire.MsgTx, []byte, error) {
	if !validCarrier(carrier) || !validCoin(coin) {
		return nil, nil, ErrInvalid
	}
	key, script, code := fundingKey()
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

func (p *Publisher) BuildPublication(ctx context.Context, carrier [73]byte, coin FundingCoin) ([]byte, error) {
	c, end, e := p.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer end()
	if !validCarrier(carrier) || !validCoin(coin) {
		return nil, ErrInvalid
	}
	if e = p.checkCoin(c, coin); e != nil {
		return nil, e
	}
	_, raw, e := publication(carrier, coin)
	if e != nil {
		return nil, e
	}
	if e = p.checkCoin(c, coin); e != nil {
		return nil, e
	}
	if e = p.finishShared(c); e != nil {
		return nil, e
	}
	return raw, nil
}

func (p *Publisher) SubmitPublication(ctx context.Context, carrier [73]byte, coin FundingCoin, raw []byte) (string, error) {
	c, end, e := p.begin(ctx)
	if e != nil {
		return "", e
	}
	defer end()
	if len(raw) > MaxTransactionSize {
		return "", ErrInvalid
	}
	tx, expected, e := publication(carrier, coin)
	if e != nil || !bytes.Equal(raw, expected) {
		return "", ErrInvalid
	}
	if e = p.checkCoin(c, coin); e != nil {
		return "", e
	}
	if c.Err() != nil {
		return "", ErrOffline
	}
	result, e := p.nodes[0].call(c, "sendrawtransaction", []any{hex.EncodeToString(raw), false})
	// Every error after the transport boundary is uncertain. No retry/rebuild.
	var txid string
	if e != nil || json.Unmarshal(result, &txid) != nil || txid != tx.TxHash().String() {
		return "", ErrOutcomeUnknown
	}
	if _, e = p.observe(c); e != nil {
		return "", ErrOutcomeUnknown
	}
	return txid, nil
}
