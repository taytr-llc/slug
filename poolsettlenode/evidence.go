// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlenode

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var decimalAmount = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func hash(s string) bool {
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

type tip struct {
	Hash   string `json:"hash"`
	Height uint32 `json:"height"`
}

func (b *Client) snapshot(ctx context.Context) (tip, error) {
	var magic uint32
	var genesis, canonical string
	var t tip
	if b.call(ctx, "getcurrentnet", nil, &magic) != nil || magic != uint32(chaincfg.MainNetParams.Net) || b.call(ctx, "getblockhash", []any{0}, &genesis) != nil || genesis != chaincfg.MainNetParams.GenesisHash.String() || b.callFields(ctx, "getbestblock", nil, &t, "hash", "height") != nil || !hash(t.Hash) || t.Height > math.MaxInt32 || b.call(ctx, "getblockhash", []any{t.Height}, &canonical) != nil || canonical != t.Hash {
		return tip{}, ErrUnavailable
	}
	var info struct {
		Chain       string `json:"chain"`
		Blocks      uint32 `json:"blocks"`
		Best        string `json:"bestblockhash"`
		Deployments map[string]struct {
			Status string `json:"status"`
			Height uint32 `json:"always_active_height"`
		} `json:"bip9_softforks"`
	}
	if b.callFields(ctx, "getblockchaininfo", nil, &info, "chain", "blocks", "bestblockhash", "bip9_softforks") != nil || info.Chain != "mainnet" || info.Blocks != t.Height || info.Best != t.Hash {
		return tip{}, ErrUnavailable
	}
	for _, name := range []string{"csv", "segwit", "taproot"} {
		d := info.Deployments[name]
		if d.Height != chaincfg.PhaseAActivationHeight || (d.Status == "active") != (t.Height+1 >= chaincfg.PhaseAActivationHeight) {
			return tip{}, ErrUnavailable
		}
		switch d.Status {
		case "defined", "started", "locked_in", "active", "failed":
		default:
			return tip{}, ErrUnavailable
		}
	}
	return t, nil
}
func (b *Client) unchanged(ctx context.Context, before tip) error {
	after, err := b.snapshot(ctx)
	if err != nil || before != after {
		return ErrUnavailable
	}
	return nil
}

func amount(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || len(raw) > 64 || !decimalAmount.Match(raw) {
		return 0, ErrUnavailable
	}
	if i := strings.IndexAny(string(raw), "eE"); i >= 0 {
		exponent, err := strconv.ParseInt(string(raw[i+1:]), 10, 32)
		if err != nil || exponent < -32 || exponent > 32 {
			return 0, ErrUnavailable
		}
	}
	r, ok := new(big.Rat).SetString(string(raw))
	if !ok || r.Sign() < 0 {
		return 0, ErrUnavailable
	}
	r.Mul(r, big.NewRat(100000000, 1))
	if !r.IsInt() || !r.Num().IsInt64() || r.Num().Int64() > poolsettle.MaxMoney {
		return 0, ErrUnavailable
	}
	return r.Num().Int64(), nil
}

type transaction struct {
	Hex           string `json:"hex"`
	TxID          string `json:"txid"`
	BlockHash     string `json:"blockhash"`
	Confirmations int64  `json:"confirmations"`
}

func (b *Client) transaction(ctx context.Context, txid string, at tip) (*wire.MsgTx, []byte, transaction, uint32, error) {
	var v transaction
	var response json.RawMessage
	err := b.call(ctx, "getrawtransaction", []any{txid, 1}, &response)
	if err != nil {
		var refusal *rpcError
		// Distinguish a definite absent indexed transaction from disabled index,
		// transport failure or any other RPC refusal.
		if errors.As(err, &refusal) && refusal.Code == -5 && refusal.Message == "No information available about transaction "+txid {
			return nil, nil, v, 0, nil
		}
		return nil, nil, v, 0, ErrUnavailable
	}
	if decodeFields(response, &v, "hex", "txid") != nil {
		return nil, nil, v, 0, ErrUnavailable
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(response, &fields)
	for _, key := range []string{"blockhash", "confirmations"} {
		if value, ok := fields[key]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, nil, v, 0, ErrUnavailable
		}
	}
	if v.BlockHash != "" && fields["confirmations"] == nil {
		return nil, nil, v, 0, ErrUnavailable
	}
	raw, err := hex.DecodeString(v.Hex)
	if err != nil || len(raw) == 0 || len(raw) > poolsettle.MaxTransactionBytes || v.Hex != hex.EncodeToString(raw) || v.TxID != txid || v.Confirmations < 0 {
		return nil, nil, v, 0, ErrUnavailable
	}
	var tx wire.MsgTx
	r := bytes.NewReader(raw)
	if tx.Deserialize(r) != nil || r.Len() != 0 || tx.TxHash().String() != txid {
		return nil, nil, v, 0, ErrUnavailable
	}
	var canonicalRaw bytes.Buffer
	if tx.Serialize(&canonicalRaw) != nil || !bytes.Equal(canonicalRaw.Bytes(), raw) {
		return nil, nil, v, 0, ErrUnavailable
	}
	if v.BlockHash == "" {
		if v.Confirmations != 0 {
			return nil, nil, v, 0, ErrUnavailable
		}
		return &tx, raw, v, 0, nil
	}
	var header struct {
		Hash   string `json:"hash"`
		Height uint32 `json:"height"`
	}
	var canonical string
	if !hash(v.BlockHash) || b.callFields(ctx, "getblockheader", []any{v.BlockHash, true}, &header, "hash", "height") != nil || header.Hash != v.BlockHash || header.Height > at.Height || b.call(ctx, "getblockhash", []any{header.Height}, &canonical) != nil {
		return nil, nil, v, 0, ErrUnavailable
	}
	if canonical != v.BlockHash || v.Confirmations != int64(at.Height-header.Height)+1 {
		return nil, nil, v, 0, ErrUnavailable
	}
	return &tx, raw, v, header.Height, nil
}

func (b *Client) validCoin(c poolsettle.Coin) bool {
	return hash(c.TxID) && c.Atoms > 0 && c.Atoms <= poolsettle.MaxMoney && c.Script == b.config.PoolScript && (!c.Coinbase || hash(c.BlockHash) && c.BlockHeight > 0)
}

func (b *Client) inspect(ctx context.Context, coins []poolsettle.Coin) ([]poolsettle.Funding, error) {
	if len(coins) > poolsettle.MaxInputs {
		return nil, ErrUnavailable
	}
	before, err := b.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]poolsettle.Funding, 0, len(coins))
	type origin struct {
		height    uint32
		canonical string
	}
	origins := make(map[string]origin)
	for _, c := range coins {
		if !b.validCoin(c) || len(c.Claims) == 0 || len(c.Claims) > 256*257 {
			return nil, ErrUnavailable
		}
		f := poolsettle.Funding{TxID: c.TxID, Index: c.Index, State: "funding-conflict"}
		// Change retains each original round's funding provenance. Even a
		// canonical change transaction cannot make an orphaned original round
		// spendable: its same coinbase may have returned under another header.
		conflict := false
		for _, claim := range c.Claims {
			if !hash(claim.RoundHash) || claim.RoundHeight == 0 || claim.RoundHeight > math.MaxInt32 {
				return nil, ErrUnavailable
			}
			o, ok := origins[claim.RoundHash]
			if ok && o.height != claim.RoundHeight {
				return nil, ErrUnavailable
			}
			if !ok {
				if len(origins) >= 256 {
					return nil, ErrUnavailable
				}
				o.height = claim.RoundHeight
				if o.height <= before.Height && (b.call(ctx, "getblockhash", []any{o.height}, &o.canonical) != nil || !hash(o.canonical)) {
					return nil, ErrUnavailable
				}
				origins[claim.RoundHash] = o
			}
			if o.canonical != claim.RoundHash {
				conflict = true
			}
		}
		if conflict {
			out = append(out, f)
			continue
		}
		if c.Coinbase {
			if c.BlockHeight > before.Height {
				out = append(out, f)
				continue
			}
			var canonical string
			if b.call(ctx, "getblockhash", []any{c.BlockHeight}, &canonical) != nil {
				return nil, ErrUnavailable
			}
			if canonical != c.BlockHash {
				out = append(out, f)
				continue
			}
		}
		tx, _, observed, height, err := b.transaction(ctx, c.TxID, before)
		if err != nil {
			return nil, err
		}
		if tx == nil {
			out = append(out, f)
			continue
		}
		isCoinbase := len(tx.TxIn) == 1 && tx.TxIn[0].PreviousOutPoint.Index == wire.MaxPrevOutIndex && tx.TxIn[0].PreviousOutPoint.Hash == (chainhash.Hash{})
		if int(c.Index) >= len(tx.TxOut) || tx.TxOut[c.Index].Value != c.Atoms || hex.EncodeToString(tx.TxOut[c.Index].PkScript) != c.Script || isCoinbase != c.Coinbase || c.Coinbase && (height != c.BlockHeight || observed.BlockHash != c.BlockHash) {
			return nil, ErrUnavailable
		}
		var utxo *struct {
			BestBlock     string          `json:"bestblock"`
			Confirmations uint32          `json:"confirmations"`
			Value         json.RawMessage `json:"value"`
			Coinbase      bool            `json:"coinbase"`
			Script        struct {
				Hex string `json:"hex"`
			} `json:"scriptPubKey"`
		}
		if b.callNullableFields(ctx, "gettxout", []any{c.TxID, c.Index, true}, &utxo, "bestblock", "confirmations", "value", "coinbase", "scriptPubKey.hex") != nil {
			return nil, ErrUnavailable
		}
		if utxo == nil {
			out = append(out, f)
			continue
		}
		// This node's gettxout does not hide canonical UTXOs reserved by
		// mempool spends. Inspect that separate state explicitly.
		var spends []struct {
			TxID     string `json:"txid"`
			Index    uint32 `json:"vout"`
			Spending string `json:"spendingtxid"`
		}
		var spendRows []json.RawMessage
		if b.call(ctx, "gettxspendingprevout", []any{[]map[string]any{{"txid": c.TxID, "vout": c.Index}}}, &spendRows) != nil || len(spendRows) != 1 {
			return nil, ErrUnavailable
		}
		spends = append(spends, struct {
			TxID     string `json:"txid"`
			Index    uint32 `json:"vout"`
			Spending string `json:"spendingtxid"`
		}{})
		if decodeFields(spendRows[0], &spends[0], "txid", "vout") != nil || spends[0].TxID != c.TxID || spends[0].Index != c.Index {
			return nil, ErrUnavailable
		}
		var spendFields map[string]json.RawMessage
		_ = json.Unmarshal(spendRows[0], &spendFields)
		if value, ok := spendFields["spendingtxid"]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, ErrUnavailable
		}
		if spends[0].Spending != "" {
			if !hash(spends[0].Spending) {
				return nil, ErrUnavailable
			}
			out = append(out, f)
			continue
		}
		atoms, err := amount(utxo.Value)
		if err != nil || utxo.BestBlock != before.Hash || atoms != c.Atoms || utxo.Script.Hex != c.Script || utxo.Coinbase != c.Coinbase || uint64(utxo.Confirmations) != uint64(observed.Confirmations) {
			return nil, ErrUnavailable
		}
		switch {
		case observed.Confirmations == 0:
			f.State = "awaiting-confirmation"
		case c.Coinbase && before.Height+1-height < uint32(chaincfg.MainNetParams.CoinbaseMaturity):
			f.State = "immature"
		default:
			f.State = "ready"
		}
		out = append(out, f)
	}
	for _, o := range origins {
		if o.height > before.Height {
			continue
		}
		var canonical string
		if b.call(ctx, "getblockhash", []any{o.height}, &canonical) != nil || canonical != o.canonical {
			return nil, ErrUnavailable
		}
	}
	if err := b.unchanged(ctx, before); err != nil {
		return nil, err
	}
	return out, nil
}

func (b *Client) observeTransaction(ctx context.Context, txid string) (poolsettle.TransactionObservation, error) {
	if !hash(txid) {
		return poolsettle.TransactionObservation{}, ErrUnavailable
	}
	before, err := b.snapshot(ctx)
	if err != nil {
		return poolsettle.TransactionObservation{}, err
	}
	_, raw, v, _, err := b.transaction(ctx, txid, before)
	if err != nil || v.Confirmations > math.MaxUint32 {
		return poolsettle.TransactionObservation{}, ErrUnavailable
	}
	if err := b.unchanged(ctx, before); err != nil {
		return poolsettle.TransactionObservation{}, err
	}
	return poolsettle.TransactionObservation{Raw: raw, Confirmations: uint32(v.Confirmations)}, nil
}

func (b *Client) broadcast(ctx context.Context, raw []byte) (string, error) {
	if len(raw) == 0 || len(raw) > poolsettle.MaxTransactionBytes {
		return "", ErrUnavailable
	}
	var tx wire.MsgTx
	r := bytes.NewReader(raw)
	if tx.Deserialize(r) != nil || r.Len() != 0 {
		return "", ErrUnavailable
	}
	var canonicalRaw bytes.Buffer
	if tx.Serialize(&canonicalRaw) != nil || !bytes.Equal(canonicalRaw.Bytes(), raw) || tx.Version != 2 || tx.LockTime != 0 || len(tx.TxOut) == 0 || len(tx.TxOut) > 257 {
		return "", ErrInvalid
	}
	if len(tx.TxIn) == 0 || len(tx.TxIn) > poolsettle.MaxInputs {
		return "", ErrUnavailable
	}
	public, _ := hex.DecodeString(b.config.PoolPublicKey)
	for _, in := range tx.TxIn {
		if in.Sequence != wire.MaxTxInSequenceNum || len(in.SignatureScript) != 0 || len(in.Witness) != 2 || len(in.Witness[0]) < 2 || in.Witness[0][len(in.Witness[0])-1] != byte(txscript.SigHashAll) || !bytes.Equal(in.Witness[1], public) {
			return "", ErrUnavailable
		}
	}
	if _, err := b.snapshot(ctx); err != nil {
		return "", err
	}
	var txid string
	if b.call(ctx, "sendrawtransaction", []any{hex.EncodeToString(raw), false}, &txid) != nil || txid != tx.TxHash().String() {
		return "", ErrUnavailable
	}
	return txid, nil
}
