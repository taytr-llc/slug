// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlenode

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

const testPublic = "02f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f9"
const testScript = "00147dd65592d0ab2fe0d0257d571abf032cd9db93dc"

func testConfig() Config {
	return Config{1, uint32(chaincfg.MainNetParams.Net), chaincfg.MainNetParams.GenesisHash.String(), testPublic, testScript, FeePolicy{Mode: "node", ConfirmationTarget: 2, MaxAtomsPerKVB: 100000, MaxFeeAtoms: 1000000}}
}

// Controlled authenticated node responses. These tests do not assert mined
// membership or transaction signature validity; actual-node acceptance follows.
type nodeModel struct {
	mu           sync.Mutex
	mode         string
	height       uint32
	calls        map[string]int
	coin         poolsettle.Coin
	raw          []byte
	blockEntered chan struct{}
	txHeight     uint32
	txBlock      string
}

func newModel() *nodeModel {
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{}, wire.MaxPrevOutIndex), []byte{1, 1}, nil))
	script, _ := hex.DecodeString(testScript)
	tx.AddTxOut(wire.NewTxOut(1000000000, script))
	var b bytes.Buffer
	_ = tx.Serialize(&b)
	coin := poolsettle.Coin{TxID: tx.TxHash().String(), Atoms: 1000000000, Script: testScript, Coinbase: true, BlockHash: strings.Repeat("1", 64), BlockHeight: 1, Claims: []poolsettle.Claim{{RoundHash: strings.Repeat("1", 64), RoundHeight: 1, Amount: poolsettle.Fraction{Numerator: "1000000000", Denominator: "1"}}}}
	return &nodeModel{height: 100, txHeight: 1, txBlock: strings.Repeat("1", 64), calls: map[string]int{}, coin: coin, raw: b.Bytes()}
}
func (m *nodeModel) set(mode string) { m.mu.Lock(); m.mode = mode; m.mu.Unlock() }
func (m *nodeModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, p, ok := r.BasicAuth()
	if !ok || u != "test" || p != "public-fixture" {
		http.Error(w, "unauthorized", 401)
		return
	}
	var q struct {
		ID     uint64
		Method string
		Params []json.RawMessage
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if json.Unmarshal(raw, &q) != nil {
		http.Error(w, "bad", 400)
		return
	}
	m.mu.Lock()
	m.calls[q.Method]++
	mode, height := m.mode, m.height
	entered := m.blockEntered
	m.mu.Unlock()
	if mode == "block" {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-r.Context().Done()
		return
	}
	if mode == "redirect" {
		http.Redirect(w, r, "http://127.0.0.1:1", 302)
		return
	}
	if mode == "encoding" {
		w.Header().Set("Content-Encoding", "gzip")
	}
	if mode == "oversize" {
		_, _ = w.Write(bytes.Repeat([]byte(" "), (2<<20)+1))
		return
	}
	if mode == "duplicate" {
		_, _ = io.WriteString(w, `{"jsonrpc":"1.0","id":1,"id":1,"error":null,"result":1}`)
		return
	}
	var result any
	var refusal *rpcError
	tip := strings.Repeat("a", 64)
	switch q.Method {
	case "getcurrentnet":
		result = uint32(chaincfg.MainNetParams.Net)
		if mode == "wrong-network" {
			result = uint32(7)
		}
	case "getbestblock":
		result = map[string]any{"hash": tip, "height": height}
	case "getblockhash":
		var h uint32
		_ = json.Unmarshal(q.Params[0], &h)
		switch h {
		case 0:
			result = chaincfg.MainNetParams.GenesisHash.String()
			if mode == "wrong-genesis" {
				result = strings.Repeat("f", 64)
			}
		case 1:
			result = strings.Repeat("1", 64)
			if mode == "orphan" {
				result = strings.Repeat("2", 64)
			}
		case 90:
			result = m.txBlock
		default:
			result = tip
			if mode == "changed-tip" {
				result = strings.Repeat("b", 64)
			}
		}
	case "getblockchaininfo":
		d := map[string]any{}
		for _, n := range []string{"csv", "segwit", "taproot"} {
			d[n] = map[string]any{"status": "active", "always_active_height": 62}
		}
		result = map[string]any{"chain": "mainnet", "blocks": height, "bestblockhash": tip, "bip9_softforks": d}
	case "getrawtransaction":
		result = map[string]any{"hex": hex.EncodeToString(m.raw), "txid": m.coin.TxID, "blockhash": m.txBlock, "confirmations": height - m.txHeight + 1}
		if mode == "unknown" {
			result = nil
			refusal = &rpcError{-5, "No information available about transaction " + m.coin.TxID}
		}
		if mode == "txindex" {
			result = nil
			refusal = &rpcError{-5, "transaction index disabled"}
		}
	case "getblockheader":
		result = map[string]any{"hash": m.txBlock, "height": m.txHeight}
	case "gettxout":
		result = map[string]any{"bestblock": tip, "confirmations": height - m.txHeight + 1, "value": 10, "coinbase": m.coin.Coinbase, "scriptPubKey": map[string]any{"hex": testScript}}
		if mode == "spent" {
			result = nil
		}
	case "gettxspendingprevout":
		v := map[string]any{"txid": m.coin.TxID, "vout": 0}
		if mode == "mempool-spend" {
			v["spendingtxid"] = strings.Repeat("c", 64)
		}
		result = []any{v}
	case "estimatefee":
		result = json.RawMessage("0.00001")
		if mode == "fee-unavailable" {
			result = nil
			refusal = &rpcError{-32603, "not enough observations"}
		}
		if mode == "fee-cap" {
			result = json.RawMessage("100")
		}
	case "sendrawtransaction":
		if mode == "lost-ack" {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		var h string
		_ = json.Unmarshal(q.Params[0], &h)
		v, _ := hex.DecodeString(h)
		var tx wire.MsgTx
		_ = tx.Deserialize(bytes.NewReader(v))
		result = tx.TxHash().String()
	default:
		http.Error(w, "unexpected method", 400)
		return
	}

	if mode == "contradictory-error" && q.Method == "getrawtransaction" {
		refusal = &rpcError{-5, "No information available about transaction " + m.coin.TxID}
	}
	if mode == "missing-spend-vout" && q.Method == "gettxspendingprevout" {
		result = []any{map[string]any{"txid": m.coin.TxID}}
	}
	if mode == "null-spend" && q.Method == "gettxspendingprevout" {
		result = []any{map[string]any{"txid": m.coin.TxID, "vout": 0, "spendingtxid": nil}}
	}
	if mode == "missing-coinbase" && q.Method == "gettxout" {
		delete(result.(map[string]any), "coinbase")
	}
	if mode == "missing-confirmations" && q.Method == "gettxout" {
		delete(result.(map[string]any), "confirmations")
	}
	if mode == "missing-height" && q.Method == "getbestblock" {
		delete(result.(map[string]any), "height")
	}
	if mode == "missing-tx-confirmations" && q.Method == "getrawtransaction" {
		delete(result.(map[string]any), "confirmations")
	}

	id := q.ID
	if mode == "wrong-id" {
		id++
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "1.0", "id": id, "result": result, "error": refusal})
}
func modelClient(t *testing.T, m *nodeModel, tlsMode bool) (*Client, *httptest.Server) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal node evidence only")
	}
	var s *httptest.Server
	if tlsMode {
		s = httptest.NewTLSServer(m)
	} else {
		s = httptest.NewServer(m)
	}
	t.Cleanup(s.Close)
	rpc := prospectnode.RPCConfig{Version: 1, URL: s.URL, Username: "test", Password: "public-fixture"}
	if tlsMode {
		rpc.CACertificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}))
	}
	b, err := New(context.Background(), testConfig(), rpc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b, s
}
func TestNodeFundingMaturityAndOriginalProvenance(t *testing.T) {
	m := newModel()
	b, _ := modelClient(t, m, false)
	for _, x := range []struct{ mode, state string }{{"", "ready"}, {"orphan", "funding-conflict"}, {"spent", "funding-conflict"}, {"mempool-spend", "funding-conflict"}} {
		m.set(x.mode)
		got, err := b.Inspect(context.Background(), []poolsettle.Coin{m.coin})
		if err != nil || len(got) != 1 || got[0].State != x.state {
			t.Fatal(x, got, err)
		}
	}
	m.set("")
	m.mu.Lock()
	m.height = 99
	m.mu.Unlock()
	got, err := b.Inspect(context.Background(), []poolsettle.Coin{m.coin})
	if err != nil || len(got) != 1 || got[0].State != "immature" {
		t.Fatal("next block before maturity", got, err)
	}
	for _, mode := range []string{"txindex", "changed-tip", "wrong-network", "wrong-genesis", "missing-spend-vout", "null-spend", "missing-coinbase", "missing-confirmations", "missing-height", "missing-tx-confirmations"} {
		m.set(mode)
		if got, err := b.Inspect(context.Background(), []poolsettle.Coin{m.coin}); err == nil || got != nil {
			t.Fatal("unavailable became observation", mode, got, err)
		}
	}
}
func TestNodeTLSStrictResponsesAndDefiniteAbsence(t *testing.T) {
	m := newModel()
	b, _ := modelClient(t, m, true)
	for _, mode := range []string{"duplicate", "wrong-id", "oversize", "encoding", "redirect", "txindex", "contradictory-error", "missing-tx-confirmations"} {
		m.set(mode)
		if _, err := b.ObserveTransaction(context.Background(), m.coin.TxID); err == nil {
			t.Fatal("response accepted", mode)
		}
	}
	m.set("unknown")
	got, err := b.ObserveTransaction(context.Background(), m.coin.TxID)
	if err != nil || len(got.Raw) != 0 || got.Confirmations != 0 {
		t.Fatal("definite absence", got, err)
	}
	m.set("")
	got, err = b.ObserveTransaction(context.Background(), m.coin.TxID)
	if err != nil || !bytes.Equal(got.Raw, m.raw) || got.Confirmations != 100 {
		t.Fatal("exact observed transaction", err)
	}
	for _, raw := range []string{`{"x":1,"x":2}`, `{"x":{"a":1,"a":2}}`, `[1] [2]`, strings.Repeat("[", 18) + "1" + strings.Repeat("]", 18)} {
		if uniqueJSON([]byte(raw)) == nil {
			t.Fatal("ambiguous JSON accepted")
		}
	}
}
func TestNodeQuoteCapsAndNoFallback(t *testing.T) {
	m := newModel()
	b, _ := modelClient(t, m, false)
	p := []poolsettle.Payment{{GymID: strings.Repeat("3", 64), Script: "0014" + strings.Repeat("4", 40), Atoms: 100000000}}
	got, err := b.EstimateFee(context.Background(), []poolsettle.Coin{m.coin}, p, testScript)
	if err != nil || got != 141 {
		t.Fatal("native quote", got, err)
	}
	for _, mode := range []string{"fee-unavailable", "fee-cap", "changed-tip"} {
		m.set(mode)
		if _, err = b.EstimateFee(context.Background(), []poolsettle.Coin{m.coin}, p, testScript); err == nil {
			t.Fatal("quote silently fell back", mode)
		}
	}
}
func TestNodeLostBroadcastAndJoinedClose(t *testing.T) {
	m := newModel()
	b, _ := modelClient(t, m, false)
	tx := wire.NewMsgTx(2)
	h, _ := chainhash.NewHashFromStr(m.coin.TxID)
	in := wire.NewTxIn(wire.NewOutPoint(h, 0), nil, nil)
	pub, _ := hex.DecodeString(testPublic)
	in.Witness = wire.TxWitness{[]byte{1, 1}, pub}
	tx.AddTxIn(in)
	tx.AddTxOut(wire.NewTxOut(1, []byte{0x6a}))
	var buf bytes.Buffer
	_ = tx.Serialize(&buf)
	m.set("lost-ack")
	if _, err := b.Broadcast(context.Background(), buf.Bytes()); err == nil {
		t.Fatal("lost ack became success")
	}
	m.mu.Lock()
	n := m.calls["sendrawtransaction"]
	m.mu.Unlock()
	if n != 1 {
		t.Fatal("broadcast retried", n)
	}
	entered := make(chan struct{}, 1)
	m.mu.Lock()
	m.blockEntered = entered
	m.mode = "block"
	m.mu.Unlock()
	done := make(chan error, 1)
	go func() { _, err := b.Inspect(context.Background(), []poolsettle.Coin{m.coin}); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	closed := make(chan error, 1)
	go func() { closed <- b.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not join")
	}
	if err := <-done; err == nil {
		t.Fatal("cancelled evidence escaped")
	}
	if _, err := b.Inspect(context.Background(), nil); err == nil {
		t.Fatal("closed client reused")
	}
}
func TestNodeConfigAndCheapProfile(t *testing.T) {
	c := testConfig()
	if ValidateConfig(c) != nil {
		t.Fatal("independently published scalar3 public identity")
	}
	for _, mut := range []func(*Config){func(c *Config) { c.Genesis = strings.Repeat("0", 64) }, func(c *Config) { c.PoolScript = "0014" + strings.Repeat("0", 40) }, func(c *Config) { c.Fee.Mode = "fallback" }, func(c *Config) { c.Fee.AtomsPerKVB = 1000 }, func(c *Config) { c.Fee.MaxFeeAtoms = 0 }} {
		bad := c
		mut(&bad)
		if ValidateConfig(bad) == nil {
			t.Fatal("invalid config accepted")
		}
	}
	if wire.TestPoW {
		if b, err := New(context.Background(), c, prospectnode.RPCConfig{}); err != poolsettle.ErrProfile || b != nil {
			t.Fatal("cheap profile touched transport", err)
		}
	}
}

func TestNodeCanonicalChangeRetainsOrphanedRound(t *testing.T) {
	m := newModel()
	m.coin.Coinbase = false
	m.coin.BlockHash = ""
	m.coin.BlockHeight = 0
	m.txHeight = 90
	m.txBlock = strings.Repeat("b", 64)
	tx := wire.NewMsgTx(2)
	h := chainhash.Hash{9}
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&h, 0), nil, nil))
	script, _ := hex.DecodeString(testScript)
	tx.AddTxOut(wire.NewTxOut(m.coin.Atoms, script))
	var raw bytes.Buffer
	_ = tx.Serialize(&raw)
	m.raw = raw.Bytes()
	m.coin.TxID = tx.TxHash().String()
	b, _ := modelClient(t, m, false)
	got, err := b.Inspect(context.Background(), []poolsettle.Coin{m.coin})
	if err != nil || len(got) != 1 || got[0].State != "ready" {
		t.Fatal("canonical change", got, err)
	}

	m.set("missing-coinbase")
	if _, err = b.Inspect(context.Background(), []poolsettle.Coin{m.coin}); err == nil {
		t.Fatal("omitted false coinbase field became change evidence")
	}
	m.set("orphan")
	got, err = b.Inspect(context.Background(), []poolsettle.Coin{m.coin})
	if err != nil || len(got) != 1 || got[0].State != "funding-conflict" {
		t.Fatal("original funding orphan hidden by change", got, err)
	}
}
func TestNodeAuthenticationTrustAndExplicitFixedRate(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal transport only")
	}
	m := newModel()
	server := httptest.NewTLSServer(m)
	defer server.Close()
	rpc := prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "test", Password: "public-fixture"}
	if b, err := New(context.Background(), testConfig(), rpc); err == nil || b != nil {
		t.Fatal("untrusted TLS succeeded")
	}
	rpc.CACertificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	rpc.Password = "wrong"
	if b, err := New(context.Background(), testConfig(), rpc); err == nil || b != nil {
		t.Fatal("wrong RPC authentication succeeded")
	}
	rpc.Password = "public-fixture"
	c := testConfig()
	c.Fee.Mode = "fixed"
	c.Fee.ConfirmationTarget = 0
	c.Fee.AtomsPerKVB = 1000
	b, err := New(context.Background(), c, rpc)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	m.set("fee-unavailable")
	p := []poolsettle.Payment{{GymID: strings.Repeat("3", 64), Script: "0014" + strings.Repeat("4", 40), Atoms: 100000000}}
	got, err := b.EstimateFee(context.Background(), []poolsettle.Coin{m.coin}, p, testScript)
	if err != nil || got != 141 {
		t.Fatal("explicit fixed quote", got, err)
	}
	m.mu.Lock()
	n := m.calls["estimatefee"]
	m.mu.Unlock()
	if n != 0 {
		t.Fatal("fixed mode called estimator")
	}
	if uniqueJSON([]byte{'"', 0xff, '"'}) == nil {
		t.Fatal("invalid UTF8 accepted")
	}
}
