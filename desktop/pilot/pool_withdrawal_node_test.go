// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slug/wire/v2"
)

// Inputs and modeled confirmation were independently frozen before either the
// journal or node adapter existed. No new adapter output supplies expectations.
func withdrawalNodeInputs(t *testing.T) (walletwithdraw.Draft, walletwithdraw.Signed, withdrawalTip) {
	t.Helper()
	raw, err := os.ReadFile("../../walletwithdraw/testdata/reference/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "ee4ba4ecc89289b76e2ce805e4d8f88ced79c6b69542d71ee74a7a37b1c1bd29" {
		t.Fatal("reference changed")
	}
	var reference struct {
		Cases []struct {
			Phases []struct {
				Name      string
				Operation walletwithdraw.Operation
			}
		}
	}
	if err = json.Unmarshal(raw, &reference); err != nil {
		t.Fatal(err)
	}
	for _, phase := range reference.Cases[0].Phases {
		if phase.Name == "submitted_modeled_confirmation" {
			op := phase.Operation
			if op.Draft == nil || op.Signed == nil || op.Observation == nil {
				t.Fatal("missing frozen phase")
			}
			return *op.Draft, *op.Signed, withdrawalTip{op.Observation.NodeTip, op.Observation.NodeHeight}
		}
	}
	t.Fatal("missing frozen confirmation")
	return walletwithdraw.Draft{}, walletwithdraw.Signed{}, withdrawalTip{}
}

type withdrawalNodeModel struct {
	t                  *testing.T
	draft              walletwithdraw.Draft
	signed             walletwithdraw.Signed
	tip                withdrawalTip
	mode               string
	mu                 sync.Mutex
	calls              map[string]int
	entered, cancelled chan struct{}
}

func newWithdrawalNodeModel(t *testing.T, mode string) (*withdrawalNode, *withdrawalNodeModel) {
	t.Helper()
	d, s, tip := withdrawalNodeInputs(t)
	m := &withdrawalNodeModel{t: t, draft: d, signed: s, tip: tip, mode: mode, calls: make(map[string]int), entered: make(chan struct{}), cancelled: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(server.Close)
	n, err := newWithdrawalNode(prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "public-fixture-user", Password: "public-fixture-password"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.close)
	return n, m
}
func (m *withdrawalNodeModel) count(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[method]
}
func (m *withdrawalNodeModel) serve(w http.ResponseWriter, r *http.Request) {
	user, password, ok := r.BasicAuth()
	if r.Method != http.MethodPost || !ok || user != "public-fixture-user" || password != "public-fixture-password" || r.Header.Get("Accept-Encoding") != "" {
		m.t.Error("unexpected transport request")
		http.Error(w, "request", 400)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 16385))
	var req btcjson.Request
	if err != nil || len(raw) > 16384 || json.Unmarshal(raw, &req) != nil {
		m.t.Error("invalid request framing")
		http.Error(w, "request", 400)
		return
	}
	cmd, err := btcjson.UnmarshalCmd(&req)
	if err != nil {
		m.t.Errorf("request violates inherited RPC schema: %s: %v", req.Method, err)
		http.Error(w, "request", 400)
		return
	}
	m.mu.Lock()
	m.calls[req.Method]++
	m.mu.Unlock()
	tip := m.tip
	if (m.mode == "changed_tip" || m.mode == "missing_changed_tip") && m.count("getbestblock") >= 2 {
		tip.Hash = strings.Repeat("a", 64)
	}
	var result any
	var failure any
	switch c := cmd.(type) {
	case *btcjson.GetCurrentNetCmd:
		result = uint32(chaincfg.MainNetParams.Net)
		if m.mode == "wrong_network" {
			result = uint32(1)
		}
	case *btcjson.GetBlockHashCmd:
		switch uint32(c.Index) {
		case 0:
			result = chaincfg.MainNetParams.GenesisHash.String()
			if m.mode == "wrong_genesis" {
				result = strings.Repeat("b", 64)
			}
		case tip.Height:
			result = tip.Hash
		default:
			for _, in := range m.draft.Inputs {
				if uint32(c.Index) == in.BlockHeight {
					result = in.BlockHash
				}
			}
			if m.mode == "orphan_input" {
				result = strings.Repeat("c", 64)
			}
		}
		if result == nil {
			m.t.Errorf("unexpected block height %d", c.Index)
		}
	case *btcjson.GetBestBlockCmd:
		result = tip
	case *btcjson.GetBlockChainInfoCmd:
		deployments := map[string]any{}
		for _, name := range []string{"csv", "segwit", "taproot"} {
			deployments[name] = map[string]any{"status": "active", "always_active_height": chaincfg.PhaseAActivationHeight}
		}
		if m.mode == "wrong_activation" {
			deployments["segwit"] = map[string]any{"status": "active", "always_active_height": 63}
		}
		result = map[string]any{"chain": "mainnet", "blocks": tip.Height, "bestblockhash": tip.Hash, "bip9_softforks": deployments}
	case *btcjson.GetTxOutCmd:
		var in *walletwithdraw.Input
		for i := range m.draft.Inputs {
			candidate := &m.draft.Inputs[i]
			if c.Txid == candidate.TxID && c.Vout == candidate.OutputIndex {
				in = candidate
			}
		}
		if in == nil || c.IncludeMempool == nil || !*c.IncludeMempool {
			m.t.Error("wrong UTXO selector")
			http.Error(w, "selector", 400)
			return
		}
		amount := in.Atoms
		script := in.Script
		conf := tip.Height - in.BlockHeight + 1
		if m.mode == "wrong_amount" {
			amount++
		}
		if m.mode == "wrong_script" {
			script = "0014" + strings.Repeat("0", 40)
		}
		if m.mode == "wrong_confirmations" {
			conf++
		}
		row := map[string]any{"bestblock": tip.Hash, "confirmations": conf, "value": json.RawMessage(fmt.Sprintf("%d.%08d", amount/100000000, amount%100000000)), "coinbase": in.Coinbase, "scriptPubKey": map[string]any{"hex": script}}
		if m.mode == "null_coinbase" {
			row["coinbase"] = nil
		}
		result = row
		if m.mode == "missing_utxo" {
			result = nil
		}
	case *btcjson.GetTxSpendingPrevOutCmd:
		if len(c.Outputs) != len(m.draft.Inputs) {
			m.t.Error("wrong outpoint count")
			http.Error(w, "selector", 400)
			return
		}
		rows := make([]map[string]any, len(c.Outputs))
		for i, point := range c.Outputs {
			in := m.draft.Inputs[i]
			if point == nil || point.Txid != in.TxID || point.Vout != in.OutputIndex {
				m.t.Error("wrong outpoint order")
				http.Error(w, "selector", 400)
				return
			}
			rows[i] = map[string]any{"txid": in.TxID, "vout": in.OutputIndex}
		}
		switch m.mode {
		case "mempool_spent":
			rows[0]["spendingtxid"] = strings.Repeat("d", 64)
		case "null_spending":
			rows[0]["spendingtxid"] = nil
		case "unknown_spending_field":
			rows[0]["unexpected"] = true
		case "foreign_outpoint":
			rows[0]["vout"] = m.draft.Inputs[0].OutputIndex + 1
		}
		result = rows
	case *btcjson.GetRawTransactionCmd:
		if c.Txid != m.signed.TxID || c.Verbose == nil || (*c.Verbose != 0 && *c.Verbose != 1) {
			m.t.Error("wrong transaction selector")
			http.Error(w, "selector", 400)
			return
		}
		if m.mode == "missing" || m.mode == "missing_changed_tip" {
			failure = map[string]any{"code": -5, "message": "No information available about transaction " + m.signed.TxID}
			break
		}
		if m.mode == "index_disabled" {
			failure = map[string]any{"code": -5, "message": "The transaction index must be enabled"}
			break
		}
		if *c.Verbose == 0 {
			result = m.signed.Raw
			if m.mode == "foreign_witness" {
				result = withdrawalAlteredWitness(m.t, m.signed)
			}
			break
		}
		row := map[string]any{"hex": m.signed.Raw, "txid": m.signed.TxID}
		switch m.mode {
		case "confirmed", "wrong_block", "wrong_location_count":
			row["blockhash"] = m.tip.Hash
			row["confirmations"] = 1
		case "null_blockhash":
			row["blockhash"] = nil
		case "null_location_count":
			row["confirmations"] = nil
		case "mempool_with_count":
			row["confirmations"] = 1
		}
		if m.mode == "wrong_location_count" {
			row["confirmations"] = 2
		}
		result = row
	case *btcjson.GetBlockHeaderCmd:
		if c.Hash != m.tip.Hash || c.Verbose == nil || !*c.Verbose {
			m.t.Error("wrong header selector")
		}
		result = m.tip
		if m.mode == "wrong_block" {
			result = withdrawalTip{strings.Repeat("e", 64), m.tip.Height}
		}
	case *btcjson.SendRawTransactionCmd:
		if c.HexTx != m.signed.Raw {
			m.t.Error("changed signed bytes")
		}
		if m.mode == "lost_ack" {
			conn, _, e := w.(http.Hijacker).Hijack()
			if e != nil {
				m.t.Error(e)
			} else {
				conn.Close()
			}
			return
		}
		if m.mode == "blocked_send" {
			close(m.entered)
			<-r.Context().Done()
			close(m.cancelled)
			return
		}
		switch m.mode {
		case "reject22":
			failure = map[string]any{"code": -22, "message": "fixture rejection"}
		case "reject25":
			failure = map[string]any{"code": -25, "message": "fixture rejection"}
		case "reject26":
			failure = map[string]any{"code": -26, "message": "fixture rejection"}
		case "unclassified_error":
			failure = map[string]any{"code": -1, "message": "fixture unavailable"}
		case "wrong_ack":
			result = strings.Repeat("f", 64)
		default:
			result = m.signed.TxID
		}
	default:
		m.t.Errorf("unexpected RPC %s", req.Method)
		http.Error(w, "method", 400)
		return
	}
	id := req.ID
	if m.mode == "wrong_id" {
		id = 999999
	}
	envelope := map[string]any{"jsonrpc": "1.0", "id": id, "result": result, "error": failure}
	body, err := json.Marshal(envelope)
	if err != nil {
		m.t.Error(err)
		return
	}
	switch m.mode {
	case "duplicate_envelope":
		body = append([]byte(`{"id":0,`), body[1:]...)
	case "trailing_value":
		body = append(body, []byte(" {}")...)
	case "oversized_body":
		body = []byte(strings.Repeat(" ", withdrawalRPCBytes+1))
	case "compressed":
		w.Header().Set("Content-Encoding", "gzip")
	case "redirect":
		w.Header().Set("Location", "http://127.0.0.1:1/")
		w.WriteHeader(http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func withdrawalAlteredWitness(t *testing.T, s walletwithdraw.Signed) string {
	t.Helper()
	raw, err := hex.DecodeString(s.Raw)
	if err != nil {
		t.Fatal(err)
	}
	var tx wire.MsgTx
	if err = tx.Deserialize(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if len(tx.TxIn) == 0 || len(tx.TxIn[0].Witness) == 0 {
		t.Fatal("frozen witness absent")
	}
	tx.TxIn[0].Witness[0][10] ^= 1
	if tx.TxHash().String() != s.TxID {
		t.Fatal("witness mutation changed nonwitness txid")
	}
	var changed bytes.Buffer
	if err = tx.Serialize(&changed); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(changed.Bytes())
}

func TestWithdrawalNodeInspect(t *testing.T) {
	for _, mode := range []string{"valid", "wrong_network", "wrong_genesis", "wrong_activation", "orphan_input", "wrong_amount", "wrong_script", "wrong_confirmations", "missing_utxo", "null_coinbase", "mempool_spent", "null_spending", "unknown_spending_field", "foreign_outpoint", "changed_tip"} {
		t.Run(mode, func(t *testing.T) {
			n, m := newWithdrawalNodeModel(t, mode)
			err := n.inspect(context.Background(), m.draft, m.tip)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("inspect: %v", err)
			}
			if m.count("sendrawtransaction") != 0 {
				t.Fatal("read sent transaction")
			}
		})
	}
	t.Run("wallet_tip", func(t *testing.T) {
		n, m := newWithdrawalNodeModel(t, "valid")
		tip := m.tip
		tip.Height--
		if n.inspect(context.Background(), m.draft, tip) == nil {
			t.Fatal("accepted different wallet tip")
		}
	})
}
func TestWithdrawalNodeObserve(t *testing.T) {
	for _, mode := range []string{"mempool", "confirmed", "missing", "missing_changed_tip", "index_disabled", "foreign_witness", "null_blockhash", "null_location_count", "mempool_with_count", "wrong_block", "wrong_location_count", "changed_tip"} {
		t.Run(mode, func(t *testing.T) {
			n, m := newWithdrawalNodeModel(t, mode)
			got := n.observe(context.Background(), m.signed)
			want := walletwithdraw.ObservationInput{State: walletwithdraw.ObservationUnknown, Error: "node_unavailable"}
			switch mode {
			case "mempool":
				want = walletwithdraw.ObservationInput{State: walletwithdraw.ObservationMempool, NodeTip: m.tip.Hash, NodeHeight: m.tip.Height}
			case "confirmed":
				want = walletwithdraw.ObservationInput{State: walletwithdraw.ObservationConfirmed, NodeTip: m.tip.Hash, NodeHeight: m.tip.Height, BlockHash: m.tip.Hash, Confirmations: 1}
			case "missing":
				want = walletwithdraw.ObservationInput{State: walletwithdraw.ObservationNotObserved, NodeTip: m.tip.Hash, NodeHeight: m.tip.Height}
			}
			if got != want {
				t.Fatalf("observation got %+v want %+v", got, want)
			}
			if m.count("sendrawtransaction") != 0 {
				t.Fatal("observation sent transaction")
			}
		})
	}
}
func TestWithdrawalNodeDelivery(t *testing.T) {
	for _, mode := range []string{"valid", "reject22", "reject25", "reject26", "wrong_ack", "unclassified_error", "lost_ack"} {
		t.Run(mode, func(t *testing.T) {
			n, m := newWithdrawalNodeModel(t, mode)
			raw, _ := hex.DecodeString(m.signed.Raw)
			got, code := n.deliver(context.Background(), raw, m.signed.TxID)
			want, wcode := walletwithdraw.DeliveryUnknown, "node_unavailable"
			if mode == "valid" {
				want, wcode = walletwithdraw.DeliveryAcknowledged, ""
			}
			if strings.HasPrefix(mode, "reject") {
				want, wcode = walletwithdraw.DeliveryRejected, "node_rejected"
			}
			if got != want || code != wcode {
				t.Fatalf("delivery %s/%s want %s/%s", got, code, want, wcode)
			}
			if m.count("sendrawtransaction") != 1 {
				t.Fatal("delivery retried or omitted")
			}
		})
	}
}
func TestWithdrawalNodeCancellationJoinsTransport(t *testing.T) {
	n, m := newWithdrawalNodeModel(t, "blocked_send")
	raw, _ := hex.DecodeString(m.signed.Raw)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan walletwithdraw.DeliveryResult, 1)
	go func() { v, _ := n.deliver(ctx, raw, m.signed.TxID); done <- v }()
	select {
	case <-m.entered:
	case <-time.After(time.Second):
		t.Fatal("send not entered")
	}
	cancel()
	select {
	case got := <-done:
		if got != walletwithdraw.DeliveryUnknown {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("call did not cancel")
	}
	select {
	case <-m.cancelled:
	case <-time.After(time.Second):
		t.Fatal("transport not cancelled")
	}
	if m.count("sendrawtransaction") != 1 {
		t.Fatal("automatic retry")
	}
}
func TestWithdrawalNodeTransportRefusal(t *testing.T) {
	for _, mode := range []string{"wrong_id", "duplicate_envelope", "trailing_value", "oversized_body", "compressed", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			n, m := newWithdrawalNodeModel(t, mode)
			if _, err := n.snapshot(context.Background()); err == nil {
				t.Fatal("accepted malformed transport")
			}
			if m.count("getcurrentnet") != 1 || m.count("sendrawtransaction") != 0 {
				t.Fatal("unexpected retry/dispatch")
			}
		})
	}
}
func TestWithdrawalNodeGrammarAndEndpoint(t *testing.T) {
	for _, raw := range []string{`{"x":1,"\u0078":2}`, `[` + strings.Repeat(`0,`, 32) + `0]`, `"` + strings.Repeat("a", 8193) + `"`, strings.Repeat("[", 10) + "0" + strings.Repeat("]", 10), `{} {}`} {
		if withdrawalRPCJSON([]byte(raw)) == nil {
			t.Fatal("accepted unbounded/ambiguous JSON")
		}
	}
	for _, endpoint := range []string{"http://localhost:1", "http://127.0.0.2:1", "https://127.0.0.1:1", "http://127.0.0.1:1/path", "http://127.0.0.1:1?x=1"} {
		if n, err := newWithdrawalNode(prospectnode.RPCConfig{Version: 1, URL: endpoint, Username: "fixture", Password: "fixture"}); err == nil {
			n.close()
			t.Fatalf("accepted endpoint %s", endpoint)
		}
	}
	var req btcjson.Request
	if json.Unmarshal([]byte(`{"jsonrpc":"1.0","id":1,"method":"getrawtransaction","params":["`+strings.Repeat("1", 64)+`",false]}`), &req) != nil {
		t.Fatal("bad test input")
	}
	if _, err := btcjson.UnmarshalCmd(&req); err == nil {
		t.Fatal("inherited RPC unexpectedly accepts boolean verbosity")
	}
}
