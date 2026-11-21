// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlenode

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

// Controlled RPC statements only: no mining/signature-validity claim. Exact
// size/fee expectations are independently frozen in daily-shape CONTRACT.md.
type dailyNodeModel struct {
	base          *nodeModel
	calls         atomic.Int64
	active        atomic.Int64
	maximum       atomic.Int64
	mu            sync.Mutex
	orphan        uint32
	lateFlip      bool
	coinSeen      bool
	badIndex      int
	badTxID       string
	rawByID       map[string][]byte
	block         bool
	entered       chan struct{}
	exited        chan struct{}
	pauseSnapshot bool
	pauseEntered  chan struct{}
	pauseRelease  chan struct{}
}

func dailyOriginHash(height uint32) string { return fmt.Sprintf("%064x", uint64(height)+10000) }
func newDailyModel(n int) (*dailyNodeModel, []poolsettle.Coin) {
	base := newModel()
	base.height = 2000
	base.txHeight = 1000
	base.txBlock = dailyOriginHash(1000)
	tx := wire.NewMsgTx(2)
	prev := chainhash.Hash{7}
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&prev, 0), nil, nil))
	script, _ := hex.DecodeString(testScript)
	for i := 0; i < n; i++ {
		tx.AddTxOut(wire.NewTxOut(1000000000, script))
	}
	var raw bytes.Buffer
	_ = tx.Serialize(&raw)
	base.raw = raw.Bytes()
	base.coin.TxID = tx.TxHash().String()
	base.coin.Coinbase = false
	coins := make([]poolsettle.Coin, n)
	for i := range coins {
		coins[i] = base.coin
		coins[i].Index = uint32(i)
		coins[i].BlockHash = ""
		coins[i].BlockHeight = 0
		coins[i].Claims = []poolsettle.Claim{{RoundHash: dailyOriginHash(uint32(i + 1)), RoundHeight: uint32(i + 1), Amount: poolsettle.Fraction{Numerator: "1000000000", Denominator: "1"}}}
	}
	return &dailyNodeModel{base: base, badIndex: -1}, coins
}

// Separate small funding transactions avoid making the fixture transfer and
// parse one n-output transaction n times. They remain controlled RPC evidence.
func newDailyFrontierModel(n int) (*dailyNodeModel, []poolsettle.Coin) {
	m, one := newDailyModel(1)
	m.rawByID = make(map[string][]byte, n)
	coins := make([]poolsettle.Coin, n)
	var tx wire.MsgTx
	_ = tx.Deserialize(bytes.NewReader(m.base.raw))
	for i := range coins {
		tx.LockTime = uint32(i + 1)
		var raw bytes.Buffer
		_ = tx.Serialize(&raw)
		coins[i] = one[0]
		coins[i].TxID = tx.TxHash().String()
		coins[i].Claims = []poolsettle.Claim{{RoundHash: dailyOriginHash(uint32(i + 1)), RoundHeight: uint32(i + 1), Amount: poolsettle.Fraction{Numerator: "1000000000", Denominator: "1"}}}
		m.rawByID[coins[i].TxID] = raw.Bytes()
	}
	return m, coins
}
func (m *dailyNodeModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.calls.Add(1)
	active := m.active.Add(1)
	defer m.active.Add(-1)
	for old := m.maximum.Load(); active > old; old = m.maximum.Load() {
		if m.maximum.CompareAndSwap(old, active) {
			break
		}
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var q struct {
		ID     uint64
		Method string
		Params []json.RawMessage
	}
	if json.Unmarshal(raw, &q) != nil {
		http.Error(w, "bad", 400)
		return
	}
	m.mu.Lock()
	orphan, flip, seen, bad, block := m.orphan, m.lateFlip, m.coinSeen, m.badIndex, m.block
	badTxID := m.badTxID
	if q.Method == "getrawtransaction" {
		m.coinSeen = true
	}
	entered, exited := m.entered, m.exited
	pause := m.pauseSnapshot && q.Method == "getcurrentnet"
	pauseEntered, pauseRelease := m.pauseEntered, m.pauseRelease
	if pause {
		m.pauseSnapshot = false
	}
	m.mu.Unlock()
	if pause {
		close(pauseEntered)
		select {
		case <-pauseRelease:
		case <-r.Context().Done():
			return
		}
	}
	var result any
	switch q.Method {
	case "getrawtransaction":
		if m.rawByID != nil {
			var id string
			_ = json.Unmarshal(q.Params[0], &id)
			result = map[string]any{"hex": hex.EncodeToString(m.rawByID[id]), "txid": id, "blockhash": m.base.txBlock, "confirmations": m.base.height - m.base.txHeight + 1}
		}
	case "getblockhash":
		var h uint32
		_ = json.Unmarshal(q.Params[0], &h)
		if h == 0 || h == m.base.height {
			break
		}
		if block {
			entered <- struct{}{}
			<-r.Context().Done()
			exited <- struct{}{}
			return
		}
		result = dailyOriginHash(h)
		if h == orphan && (!flip || seen) {
			result = strings.Repeat("f", 64)
		}
	case "gettxspendingprevout":
		var refs []struct {
			TxID  string `json:"txid"`
			Index uint32 `json:"vout"`
		}
		_ = json.Unmarshal(q.Params[0], &refs)
		if len(refs) != 1 {
			http.Error(w, "bad", 400)
			return
		}
		row := map[string]any{"txid": refs[0].TxID, "vout": refs[0].Index}
		if int(refs[0].Index) == bad || refs[0].TxID == badTxID {
			delete(row, "vout")
		}
		result = []any{row}
	}
	if result != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "1.0", "id": q.ID, "result": result, "error": nil})
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	m.base.ServeHTTP(w, r)
}
func dailyClient(t *testing.T, m *dailyNodeModel, mode bool) *Client {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal node profile")
	}
	server := httptest.NewServer(m)
	t.Cleanup(server.Close)
	rpc := prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "test", Password: "public-fixture"}
	var b *Client
	var err error
	if mode {
		b, err = NewDaily(context.Background(), testConfig(), rpc)
	} else {
		b, err = New(context.Background(), testConfig(), rpc)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}
func TestDailyNode721AndLegacyBounds(t *testing.T) {
	m, coins := newDailyModel(721)
	daily := dailyClient(t, m, true)
	got, err := daily.Inspect(context.Background(), coins)
	if err != nil || len(got) != 721 {
		t.Fatal("daily evidence", len(got), err)
	}
	for i, f := range got {
		if f.TxID != coins[i].TxID || f.Index != uint32(i) || f.State != "ready" {
			t.Fatal("ordered complete evidence", i, f)
		}
	}
	legacy := dailyClient(t, m, false)
	before := m.calls.Load()
	if out, err := legacy.Inspect(context.Background(), coins); err == nil || out != nil || m.calls.Load() != before {
		t.Fatal("legacy input admission changed")
	}
	if daily.Config() != legacy.Config() {
		t.Fatal("profile changed node pin")
	}
}

func TestDailyNodeWholeFrontierBeyondTransactionLimit(t *testing.T) {
	// These are controlled RPC statements, not mined UTXOs. Inspect must
	// classify the complete ready set even when no single payout can spend it.
	m, coins := newDailyFrontierModel(1465)
	b := dailyClient(t, m, true)
	got, err := b.Inspect(context.Background(), coins)
	if err != nil || len(got) != len(coins) {
		t.Fatal("complete oversized frontier", len(got), err)
	}
	for i, f := range got {
		if f.TxID != coins[i].TxID || f.Index != coins[i].Index || f.State != "ready" {
			t.Fatal("ordered frontier evidence", i, f)
		}
	}
	payments := []poolsettle.Payment{{GymID: fmt.Sprintf("%064x", 1), Script: "0014" + strings.Repeat("01", 20), Atoms: poolsettle.MinimumAtoms}}
	before := m.calls.Load()
	if _, err = b.EstimateFee(context.Background(), coins, payments, testScript); err == nil || m.calls.Load() != before {
		t.Fatal("inspection widened transaction admission", err)
	}
	// A corrupt tail must refuse the entire result, not return a payable prefix.
	m.mu.Lock()
	m.badTxID = coins[1464].TxID
	m.mu.Unlock()
	if out, err := b.Inspect(context.Background(), coins); err == nil || out != nil {
		t.Fatal("oversized frontier disclosed partial evidence", len(out), err)
	}
}

func TestDailyNodeFrontierAdmissionBound(t *testing.T) {
	m, one := newDailyModel(1)
	b := dailyClient(t, m, true)
	// Symbolic references exercise bounded admission only; no 16k-UTXO or
	// transaction-size claim is made by this case.
	coins := make([]poolsettle.Coin, 16384)
	for i := range coins {
		coins[i] = one[0]
		coins[i].Index = uint32(i)
	}
	before := m.calls.Load()
	plan, err := b.dailyPlan(context.Background(), coins)
	if err != nil || len(plan.coins) != 16384 || len(plan.origins) != 1 || m.calls.Load() != before {
		t.Fatal("inclusive inspection frontier", len(plan.coins), len(plan.origins), err)
	}
	coins = append(coins, one[0])
	if out, err := b.Inspect(context.Background(), coins); err == nil || out != nil || m.calls.Load() != before {
		t.Fatal("inspection bound touched transport", len(out), err)
	}
}
func TestDailyNodeCarriedOriginsAndLateFailure(t *testing.T) {
	m, coins := newDailyModel(1)
	coins[0].Claims = nil
	for h := uint32(1); h <= 300; h++ {
		coins[0].Claims = append(coins[0].Claims, poolsettle.Claim{RoundHash: dailyOriginHash(h), RoundHeight: h})
	}
	b := dailyClient(t, m, true)
	if out, err := b.Inspect(context.Background(), coins); err != nil || len(out) != 1 || out[0].State != "ready" {
		t.Fatal("carried origins", out, err)
	}
	m.mu.Lock()
	m.orphan = 299
	m.mu.Unlock()
	if out, err := b.Inspect(context.Background(), coins); err != nil || len(out) != 1 || out[0].State != "funding-conflict" {
		t.Fatal("old origin lost behind canonical change", out, err)
	}
	m.mu.Lock()
	m.lateFlip = true
	m.coinSeen = false
	m.mu.Unlock()
	if out, err := b.Inspect(context.Background(), coins); err == nil || out != nil {
		t.Fatal("late origin change disclosed partial", out, err)
	}
	m2, many := newDailyModel(9)
	b2 := dailyClient(t, m2, true)
	m2.mu.Lock()
	m2.badIndex = 8
	m2.mu.Unlock()
	if out, err := b2.Inspect(context.Background(), many); err == nil || out != nil {
		t.Fatal("later coin corruption disclosed prefix", out, err)
	}
	m3, one := newDailyModel(1)
	b3 := dailyClient(t, m3, true)
	m3.base.set("changed-tip")
	if out, err := b3.Inspect(context.Background(), one); err == nil || out != nil {
		t.Fatal("changed tip disclosed evidence", out, err)
	}
}
func TestDailyNodeFullRequestAdmissionAndOwnedCopies(t *testing.T) {
	m, coins := newDailyModel(2)
	b := dailyClient(t, m, true)
	plan, err := b.dailyPlan(context.Background(), coins)
	if err != nil {
		t.Fatal(err)
	}
	old := plan.coins[0].Claims[0]
	coins[0].Claims[0].RoundHeight = 77
	if plan.coins[0].Claims[0] != old {
		t.Fatal("caller claim alias")
	}
	coins[0].Claims[0] = old
	before := m.calls.Load()
	for _, mut := range []func([]poolsettle.Coin){
		func(c []poolsettle.Coin) { c[1].Claims[0].RoundHash = "bad" },
		func(c []poolsettle.Coin) { c[1].TxID = c[0].TxID; c[1].Index = c[0].Index },
		func(c []poolsettle.Coin) {
			c[1].Claims[0].RoundHash = c[0].Claims[0].RoundHash
			c[1].Claims[0].RoundHeight = 99
		},
		func(c []poolsettle.Coin) { c[1].Claims = make([]poolsettle.Claim, dailyMaxClaims) },
	} {
		copy := append([]poolsettle.Coin(nil), coins...)
		for i := range copy {
			copy[i].Claims = append([]poolsettle.Claim(nil), copy[i].Claims...)
		}
		mut(copy)
		if out, err := b.Inspect(context.Background(), copy); err == nil || out != nil || m.calls.Load() != before {
			t.Fatal("invalid tail touched RPC/disclosed", out, err)
		}
	}
	one := coins[:1]
	one[0].Claims = make([]poolsettle.Claim, dailyMaxClaims)
	for i := range one[0].Claims {
		one[0].Claims[i] = poolsettle.Claim{RoundHash: dailyOriginHash(uint32(i + 1)), RoundHeight: uint32(i + 1)}
	}
	p, err := b.dailyPlan(context.Background(), one)
	if err != nil || len(p.origins) != 65536 {
		t.Fatal("inclusive origin bound", len(p.origins), err)
	}
	one[0].Claims = append(one[0].Claims, one[0].Claims[0])
	if _, err = b.dailyPlan(context.Background(), one); err == nil {
		t.Fatal("aggregate claim bound")
	}
}
func TestDailyNodeFourWorkersCancelAndClose(t *testing.T) {
	for _, closeOwner := range []bool{false, true} {
		t.Run(fmt.Sprint(closeOwner), func(t *testing.T) {
			m, coins := newDailyModel(8)
			b := dailyClient(t, m, true)
			m.mu.Lock()
			m.block = true
			m.entered = make(chan struct{}, 4)
			m.exited = make(chan struct{}, 4)
			m.mu.Unlock()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type result struct {
				out []poolsettle.Funding
				err error
			}
			done := make(chan result, 1)
			go func() { out, err := b.Inspect(ctx, coins); done <- result{out, err} }()
			// Cleanup cancellation is installed before any fatal assertion.
			t.Cleanup(func() { cancel(); _ = b.Close() })
			for i := 0; i < 4; i++ {
				select {
				case <-m.entered:
				case <-time.After(5 * time.Second):
					t.Fatal("four RPC workers not admitted")
				}
			}
			if _, err := b.Inspect(context.Background(), nil); err != ErrBusy {
				t.Fatal("original Client gate released", err)
			}
			closed := make(chan error, 1)
			if closeOwner {
				go func() { closed <- b.Close() }()
			} else {
				cancel()
			}
			select {
			case got := <-done:
				if got.err == nil || got.out != nil {
					t.Fatal("canceled result disclosed")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Inspect did not join")
			}
			if closeOwner {
				select {
				case err := <-closed:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("Close did not join")
				}
			}
			for i := 0; i < 4; i++ {
				select {
				case <-m.exited:
				case <-time.After(5 * time.Second):
					t.Fatal("RPC request survived cancellation")
				}
			}
			if m.maximum.Load() != 4 {
				t.Fatal("worker ceiling", m.maximum.Load())
			}
		})
	}
}
func dailyShapeTransaction(inputs, payees int, change bool) []byte {
	tx := wire.NewMsgTx(2)
	public, _ := hex.DecodeString(testPublic)
	for i := 0; i < inputs; i++ {
		h := chainhash.Hash{byte(i), byte(i >> 8)}
		in := wire.NewTxIn(wire.NewOutPoint(&h, uint32(i)), nil, nil)
		sig := make([]byte, 73)
		sig[72] = 1
		in.Witness = wire.TxWitness{sig, public}
		tx.AddTxIn(in)
	}
	for i := 0; i < payees; i++ {
		script := append([]byte{0, 20}, bytes.Repeat([]byte{byte(i + 1)}, 20)...)
		tx.AddTxOut(wire.NewTxOut(100000000, script))
	}
	if change {
		script, _ := hex.DecodeString(testScript)
		tx.AddTxOut(wire.NewTxOut(100000, script))
	}
	var raw bytes.Buffer
	_ = tx.Serialize(&raw)
	return raw.Bytes()
}
func TestDailyNodeFeeRawAndWeightProfile(t *testing.T) {
	m, coins := newDailyModel(721)
	b := dailyClient(t, m, true)
	old := dailyClient(t, m, false)
	payments := make([]poolsettle.Payment, 64)
	for i := range payments {
		payments[i] = poolsettle.Payment{GymID: fmt.Sprintf("%064x", i+1), Script: "0014" + strings.Repeat(fmt.Sprintf("%02x", i+1), 20), Atoms: 100000000}
	}
	fee, err := b.EstimateFee(context.Background(), coins, payments, testScript)
	if err != nil || fee != 51236 {
		t.Fatal("independent 721/64 conservative quote", fee, err)
	}
	if _, err = old.EstimateFee(context.Background(), coins, payments, testScript); err == nil {
		t.Fatal("legacy fee widened")
	}
	raw := dailyShapeTransaction(721, 64, true)
	if len(raw) != 110179 {
		t.Fatal("independent raw size", len(raw))
	}
	if _, err = b.Broadcast(context.Background(), raw); err != nil {
		t.Fatal("daily shape", err)
	}
	if _, err = old.Broadcast(context.Background(), raw); err == nil {
		t.Fatal("legacy raw widened")
	}
	before := m.calls.Load()
	for _, raw := range [][]byte{dailyShapeTransaction(1349, 256, true), dailyShapeTransaction(1465, 1, false), append([]byte(nil), bytes.Repeat([]byte{0}, 400001)...)} {
		if _, err = b.Broadcast(context.Background(), raw); err == nil || m.calls.Load() != before {
			t.Fatal("invalid daily shape touched node")
		}
	}
	if got, err := b.ObserveTransaction(context.Background(), m.base.coin.TxID); err != nil || !bytes.Equal(got.Raw, m.base.raw) {
		t.Fatal("daily exact observed bytes", err)
	}
	m.base.set("lost-ack")
	m.base.mu.Lock()
	prior := m.base.calls["sendrawtransaction"]
	m.base.mu.Unlock()
	if _, err = b.Broadcast(context.Background(), raw); err == nil {
		t.Fatal("unknown send succeeded")
	}
	m.base.mu.Lock()
	after := m.base.calls["sendrawtransaction"]
	m.base.mu.Unlock()
	if after != prior+1 {
		t.Fatal("daily broadcast retried")
	}
}
func TestDailyNodeCheapProfile(t *testing.T) {
	if !wire.TestPoW {
		return
	}
	if b, err := NewDaily(context.Background(), testConfig(), prospectnode.RPCConfig{}); b != nil || err != poolsettle.ErrProfile {
		t.Fatal("daily cheap profile touched transport", err)
	}
}

func TestDailyNodeObservedWeightAndOwnedBroadcast(t *testing.T) {
	m, _ := newDailyModel(1)
	b := dailyClient(t, m, true)
	raw := dailyShapeTransaction(721, 64, true)
	var tx wire.MsgTx
	if tx.Deserialize(bytes.NewReader(raw)) != nil {
		t.Fatal("fixture serialization")
	}
	m.base.mu.Lock()
	m.base.raw = append([]byte(nil), raw...)
	m.base.coin.TxID = tx.TxHash().String()
	m.base.mu.Unlock()
	got, err := b.ObserveTransaction(context.Background(), tx.TxHash().String())
	if err != nil || !bytes.Equal(got.Raw, raw) {
		t.Fatal("raw above100k but standard weight", err)
	}
	overweight := wire.NewMsgTx(2)
	prev := chainhash.Hash{1}
	overweight.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&prev, 0), nil, nil))
	script, _ := hex.DecodeString(testScript)
	for i := 0; i < 4000; i++ {
		overweight.AddTxOut(wire.NewTxOut(1, script))
	}
	var buf bytes.Buffer
	_ = overweight.Serialize(&buf)
	if buf.Len() >= 400000 || 4*buf.Len() <= 400000 {
		t.Fatal("independent stripped-weight fixture")
	}
	m.base.mu.Lock()
	m.base.raw = buf.Bytes()
	m.base.coin.TxID = overweight.TxHash().String()
	m.base.mu.Unlock()
	if got, err := b.ObserveTransaction(context.Background(), overweight.TxHash().String()); err == nil || got.Raw != nil {
		t.Fatal("raw envelope mistaken for weight", err)
	}

	// Once the first RPC begins, mutation of caller bytes must not retarget the
	// already-admitted send. The server only echoes its received transaction ID.
	m.mu.Lock()
	m.pauseSnapshot = true
	m.pauseEntered = make(chan struct{})
	m.pauseRelease = make(chan struct{})
	m.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var release sync.Once
	t.Cleanup(func() { cancel(); release.Do(func() { close(m.pauseRelease) }); _ = b.Close() })
	type result struct {
		id  string
		err error
	}
	done := make(chan result, 1)
	go func() { id, err := b.Broadcast(ctx, raw); done <- result{id, err} }()
	select {
	case <-m.pauseEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("broadcast snapshot did not start")
	}
	clear(raw)
	release.Do(func() { close(m.pauseRelease) })
	select {
	case got := <-done:
		if got.err != nil || got.id != tx.TxHash().String() {
			t.Fatal("original raw lost", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("broadcast did not join")
	}
	work, end, err := b.begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	deadline, ok := work.Deadline()
	remaining := time.Until(deadline)
	end()
	if !ok || remaining <= 0 || remaining > 30*time.Second {
		t.Fatal("operation deadline changed", remaining)
	}
}
