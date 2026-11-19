// Copyright (c) 2026 The SLUG developers. ISC license.
// Package poolsettlefixture supplies a deliberately public-key fixture backend
// for isolated Phase-A settlement tests. It is not a production wallet or signer.
package poolsettlefixture

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

var ErrFixture = errors.New("isolated pool settlement fixture unavailable")

const fixtureFee int64 = 1000

var decimalAmount = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// Backend uses the published scalar 3 only. Anyone can spend this key's coins;
// never send value to it or configure this backend as a production service.
type Backend struct {
	endpoint, user, password string
	http                     *http.Client
	id                       atomic.Uint64
}

func fixtureKey() (*btcec.PrivateKey, []byte) {
	var scalar [32]byte
	scalar[31] = 3
	key, pub := btcec.PrivKeyFromBytes(scalar[:])
	return key, address.Hash160(pub.SerializeCompressed())
}

func PoolPublicKey() string {
	key, _ := fixtureKey()
	return hex.EncodeToString(key.PubKey().SerializeCompressed())
}
func PoolScript() string {
	_, hash := fixtureKey()
	return hex.EncodeToString(append([]byte{txscript.OP_0, 20}, hash...))
}
func PoolAddress() string {
	_, hash := fixtureKey()
	a, _ := address.NewAddressWitnessPubKeyHash(hash, &chaincfg.MainNetParams)
	return a.EncodeAddress()
}

// New borrows an explicitly configured literal-loopback fixture node. The
// caller owns its process and temporary directory. The node must have txindex
// enabled; missing transaction-index evidence fails closed.
func New(ctx context.Context, c prospectnode.RPCConfig) (*Backend, error) {
	if wire.TestPoW {
		return nil, poolsettle.ErrProfile
	}
	u, err := url.Parse(c.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || c.Version != 1 || c.Username == "" || c.Password == "" || c.CACertificate != "" || len(c.Username)+len(c.Password)+len(c.URL) > 4096 || strings.Contains(c.Username, ":") || ctx == nil {
		return nil, ErrFixture
	}
	port, err := strconv.ParseUint(u.Port(), 10, 16)
	if err != nil || port == 0 || u.Port() != strconv.FormatUint(port, 10) {
		return nil, ErrFixture
	}
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: 65536, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, ResponseHeaderTimeout: 10 * time.Second}
	b := &Backend{endpoint: c.URL, user: c.Username, password: c.Password, http: &http.Client{Transport: tr, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if _, err := b.snapshot(ctx); err != nil {
		b.Close()
		return nil, err
	}
	return b, nil
}
func (b *Backend) Close() {
	if b != nil && b.http != nil {
		b.http.CloseIdleConnections()
	}
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return "fixture RPC refused" }

func (b *Backend) call(ctx context.Context, method string, params []any, out any) error {
	if b == nil || b.http == nil || ctx == nil || wire.TestPoW {
		return ErrFixture
	}
	id := b.id.Add(1)
	raw, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
	}{"1.0", id, method, params})
	if err != nil {
		return ErrFixture
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(raw))
	if err != nil {
		return ErrFixture
	}
	req.SetBasicAuth(b.user, b.password)
	req.Header.Set("Content-Type", "application/json")
	res, err := b.http.Do(req)
	if err != nil {
		return ErrFixture
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Encoding") != "" {
		return ErrFixture
	}
	raw, err = io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return ErrFixture
	}
	var envelope struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.ID != id || len(envelope.Result) == 0 {
		return ErrFixture
	}
	if envelope.Error != nil {
		return envelope.Error
	}
	if out == nil {
		return nil
	}
	if json.Unmarshal(envelope.Result, out) != nil {
		return ErrFixture
	}
	return nil
}

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

func (b *Backend) snapshot(ctx context.Context) (tip, error) {
	var magic uint32
	var genesis, canonical string
	var t tip
	if b.call(ctx, "getcurrentnet", nil, &magic) != nil || magic != uint32(chaincfg.MainNetParams.Net) || b.call(ctx, "getblockhash", []any{0}, &genesis) != nil || genesis != chaincfg.MainNetParams.GenesisHash.String() || b.call(ctx, "getbestblock", nil, &t) != nil || !hash(t.Hash) || t.Height > math.MaxInt32 || b.call(ctx, "getblockhash", []any{t.Height}, &canonical) != nil || canonical != t.Hash {
		return tip{}, ErrFixture
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
	if b.call(ctx, "getblockchaininfo", nil, &info) != nil || info.Chain != "mainnet" || info.Blocks != t.Height || info.Best != t.Hash {
		return tip{}, ErrFixture
	}
	for _, name := range []string{"csv", "segwit", "taproot"} {
		d := info.Deployments[name]
		if d.Height != chaincfg.PhaseAActivationHeight || (d.Status == "active") != (t.Height+1 >= chaincfg.PhaseAActivationHeight) {
			return tip{}, ErrFixture
		}
		switch d.Status {
		case "defined", "started", "locked_in", "active", "failed":
		default:
			return tip{}, ErrFixture
		}
	}
	return t, nil
}
func (b *Backend) unchanged(ctx context.Context, before tip) error {
	after, err := b.snapshot(ctx)
	if err != nil || before != after {
		return ErrFixture
	}
	return nil
}

func amount(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || len(raw) > 64 || !decimalAmount.Match(raw) {
		return 0, ErrFixture
	}
	if i := strings.IndexAny(string(raw), "eE"); i >= 0 {
		exponent, err := strconv.ParseInt(string(raw[i+1:]), 10, 32)
		if err != nil || exponent < -32 || exponent > 32 {
			return 0, ErrFixture
		}
	}
	r, ok := new(big.Rat).SetString(string(raw))
	if !ok || r.Sign() < 0 {
		return 0, ErrFixture
	}
	r.Mul(r, big.NewRat(100000000, 1))
	if !r.IsInt() || !r.Num().IsInt64() || r.Num().Int64() > poolsettle.MaxMoney {
		return 0, ErrFixture
	}
	return r.Num().Int64(), nil
}

type transaction struct {
	Hex           string `json:"hex"`
	TxID          string `json:"txid"`
	BlockHash     string `json:"blockhash"`
	Confirmations int64  `json:"confirmations"`
}

func (b *Backend) transaction(ctx context.Context, txid string, at tip) (*wire.MsgTx, []byte, transaction, uint32, error) {
	var v transaction
	err := b.call(ctx, "getrawtransaction", []any{txid, 1}, &v)
	if err != nil {
		var refusal *rpcError
		// Distinguish a definite absent indexed transaction from disabled index,
		// transport failure or any other RPC refusal.
		if errors.As(err, &refusal) && refusal.Code == -5 && refusal.Message == "No information available about transaction "+txid {
			return nil, nil, v, 0, nil
		}
		return nil, nil, v, 0, ErrFixture
	}
	raw, err := hex.DecodeString(v.Hex)
	if err != nil || len(raw) == 0 || len(raw) > poolsettle.MaxTransactionBytes || v.Hex != hex.EncodeToString(raw) || v.TxID != txid || v.Confirmations < 0 {
		return nil, nil, v, 0, ErrFixture
	}
	var tx wire.MsgTx
	r := bytes.NewReader(raw)
	if tx.Deserialize(r) != nil || r.Len() != 0 || tx.TxHash().String() != txid {
		return nil, nil, v, 0, ErrFixture
	}
	if v.BlockHash == "" {
		if v.Confirmations != 0 {
			return nil, nil, v, 0, ErrFixture
		}
		return &tx, raw, v, 0, nil
	}
	var header struct {
		Hash   string `json:"hash"`
		Height uint32 `json:"height"`
	}
	var canonical string
	if !hash(v.BlockHash) || b.call(ctx, "getblockheader", []any{v.BlockHash, true}, &header) != nil || header.Hash != v.BlockHash || header.Height > at.Height || b.call(ctx, "getblockhash", []any{header.Height}, &canonical) != nil {
		return nil, nil, v, 0, ErrFixture
	}
	if canonical != v.BlockHash || v.Confirmations != int64(at.Height-header.Height)+1 {
		return nil, nil, v, 0, ErrFixture
	}
	return &tx, raw, v, header.Height, nil
}

func validCoin(c poolsettle.Coin) bool {
	return hash(c.TxID) && c.Atoms > 0 && c.Atoms <= poolsettle.MaxMoney && c.Script == PoolScript() && (!c.Coinbase || hash(c.BlockHash) && c.BlockHeight > 0)
}

func (b *Backend) Inspect(ctx context.Context, coins []poolsettle.Coin) ([]poolsettle.Funding, error) {
	if len(coins) > poolsettle.MaxInputs {
		return nil, ErrFixture
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
		if !validCoin(c) || len(c.Claims) == 0 || len(c.Claims) > 256*257 {
			return nil, ErrFixture
		}
		f := poolsettle.Funding{TxID: c.TxID, Index: c.Index, State: "funding-conflict"}
		// Change retains each original round's funding provenance. Even a
		// canonical change transaction cannot make an orphaned original round
		// spendable: its same coinbase may have returned under another header.
		conflict := false
		for _, claim := range c.Claims {
			if !hash(claim.RoundHash) || claim.RoundHeight == 0 || claim.RoundHeight > math.MaxInt32 {
				return nil, ErrFixture
			}
			o, ok := origins[claim.RoundHash]
			if ok && o.height != claim.RoundHeight {
				return nil, ErrFixture
			}
			if !ok {
				if len(origins) >= 256 {
					return nil, ErrFixture
				}
				o.height = claim.RoundHeight
				if o.height <= before.Height && (b.call(ctx, "getblockhash", []any{o.height}, &o.canonical) != nil || !hash(o.canonical)) {
					return nil, ErrFixture
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
				return nil, ErrFixture
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
			return nil, ErrFixture
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
		if b.call(ctx, "gettxout", []any{c.TxID, c.Index, true}, &utxo) != nil {
			return nil, ErrFixture
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
		if b.call(ctx, "gettxspendingprevout", []any{[]map[string]any{{"txid": c.TxID, "vout": c.Index}}}, &spends) != nil || len(spends) != 1 || spends[0].TxID != c.TxID || spends[0].Index != c.Index {
			return nil, ErrFixture
		}
		if spends[0].Spending != "" {
			if !hash(spends[0].Spending) {
				return nil, ErrFixture
			}
			out = append(out, f)
			continue
		}
		atoms, err := amount(utxo.Value)
		if err != nil || utxo.BestBlock != before.Hash || atoms != c.Atoms || utxo.Script.Hex != c.Script || utxo.Coinbase != c.Coinbase || uint64(utxo.Confirmations) != uint64(observed.Confirmations) {
			return nil, ErrFixture
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
			return nil, ErrFixture
		}
	}
	if err := b.unchanged(ctx, before); err != nil {
		return nil, err
	}
	return out, nil
}

// EstimateFee is a fixed fixture tariff, not a network fee estimator.
func (b *Backend) EstimateFee(ctx context.Context, coins []poolsettle.Coin, payments []poolsettle.Payment, change string) (int64, error) {
	if ctx == nil || ctx.Err() != nil || b == nil || len(coins) == 0 || len(coins) > 8 || len(payments) == 0 || len(payments) > 8 || change != PoolScript() {
		return 0, ErrFixture
	}
	return fixtureFee, nil
}

// Sign uses only the published fixture scalar. It never broadcasts, selects
// coins or changes the caller's fully specified transaction outputs.
func (b *Backend) Sign(ctx context.Context, coins []poolsettle.Coin, raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > poolsettle.MaxTransactionBytes {
		return nil, ErrFixture
	}
	funding, err := b.Inspect(ctx, coins)
	if err != nil {
		return nil, err
	}
	for _, f := range funding {
		if f.State != "ready" {
			return nil, ErrFixture
		}
	}
	var tx wire.MsgTx
	r := bytes.NewReader(raw)
	if tx.Deserialize(r) != nil || r.Len() != 0 || tx.Version != 2 || tx.LockTime != 0 || len(tx.TxIn) != len(coins) || len(tx.TxOut) == 0 {
		return nil, ErrFixture
	}
	key, keyHash := fixtureKey()
	pkScript, _ := hex.DecodeString(PoolScript())
	code := append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, keyHash...)
	code = append(code, txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for i, c := range coins {
		in := tx.TxIn[i]
		if in.PreviousOutPoint.Hash.String() != c.TxID || in.PreviousOutPoint.Index != c.Index || in.Sequence != wire.MaxTxInSequenceNum || len(in.SignatureScript) != 0 || len(in.Witness) != 0 {
			return nil, ErrFixture
		}
		fetcher.AddPrevOut(in.PreviousOutPoint, wire.NewTxOut(c.Atoms, pkScript))
	}
	hashes := txscript.NewTxSigHashes(&tx, fetcher)
	for i, c := range coins {
		witness, err := txscript.WitnessSignature(&tx, hashes, i, c.Atoms, code, txscript.SigHashAll, key, true)
		if err != nil {
			return nil, ErrFixture
		}
		tx.TxIn[i].Witness = witness
		vm, err := txscript.NewEngine(pkScript, &tx, i, txscript.StandardVerifyFlags, nil, hashes, c.Atoms, fetcher)
		if err != nil || vm.Execute() != nil {
			return nil, ErrFixture
		}
	}
	var encoded bytes.Buffer
	if tx.Serialize(&encoded) != nil || encoded.Len() > poolsettle.MaxTransactionBytes {
		return nil, ErrFixture
	}
	return encoded.Bytes(), nil
}

func (b *Backend) ObserveTransaction(ctx context.Context, txid string) (poolsettle.TransactionObservation, error) {
	if !hash(txid) {
		return poolsettle.TransactionObservation{}, ErrFixture
	}
	before, err := b.snapshot(ctx)
	if err != nil {
		return poolsettle.TransactionObservation{}, err
	}
	_, raw, v, _, err := b.transaction(ctx, txid, before)
	if err != nil || v.Confirmations > math.MaxUint32 {
		return poolsettle.TransactionObservation{}, ErrFixture
	}
	if err := b.unchanged(ctx, before); err != nil {
		return poolsettle.TransactionObservation{}, err
	}
	return poolsettle.TransactionObservation{Raw: raw, Confirmations: uint32(v.Confirmations)}, nil
}

func (b *Backend) Broadcast(ctx context.Context, raw []byte) (string, error) {
	if len(raw) == 0 || len(raw) > poolsettle.MaxTransactionBytes {
		return "", ErrFixture
	}
	var tx wire.MsgTx
	r := bytes.NewReader(raw)
	if tx.Deserialize(r) != nil || r.Len() != 0 {
		return "", ErrFixture
	}
	if len(tx.TxIn) == 0 || len(tx.TxIn) > poolsettle.MaxInputs {
		return "", ErrFixture
	}
	public, _ := hex.DecodeString(PoolPublicKey())
	for _, in := range tx.TxIn {
		if len(in.SignatureScript) != 0 || len(in.Witness) != 2 || len(in.Witness[0]) < 2 || in.Witness[0][len(in.Witness[0])-1] != byte(txscript.SigHashAll) || !bytes.Equal(in.Witness[1], public) {
			return "", ErrFixture
		}
	}
	if _, err := b.snapshot(ctx); err != nil {
		return "", err
	}
	var txid string
	if b.call(ctx, "sendrawtransaction", []any{hex.EncodeToString(raw), false}, &txid) != nil || txid != tx.TxHash().String() {
		return "", ErrFixture
	}
	return txid, nil
}

var _ poolsettle.Backend = (*Backend)(nil)
