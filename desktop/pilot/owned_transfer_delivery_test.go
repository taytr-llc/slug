// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

func ownedDeliveryInputs(t *testing.T) (fighterownership.Pin, map[string]fightercarrier.Operation) {
	t.Helper()
	read := func(path, hash string, out any) {
		b, e := os.ReadFile(path)
		if e != nil || fmt.Sprintf("%x", sha256.Sum256(b)) != hash {
			t.Fatal("independent input changed", path, e)
		}
		if e = json.Unmarshal(b, out); e != nil {
			t.Fatal(e)
		}
	}
	var core struct {
		Positives []struct {
			Family string
			Value  json.RawMessage
		}
	}
	read("../../fighterownership/testdata/reference/public-core-v1.json", "1145dede4fcfa453e8bb1cdffff01079f6cca1df3bbe810dc32868207292b507", &core)
	var pin fighterownership.Pin
	for _, c := range core.Positives {
		if c.Family == "pin" {
			if e := json.Unmarshal(c.Value, &pin); e != nil {
				t.Fatal(e)
			}
			break
		}
	}
	var wallet struct {
		Positives []struct {
			Name, Kind string
			Value      json.RawMessage
		}
	}
	read("../../fightercarrier/testdata/reference/wallet-v1.json", "8648ac23d5e077df1a4dd1e3b8f07157922e650e92e9eea99c233a6993283495", &wallet)
	rows := map[string]fightercarrier.Operation{}
	for _, c := range wallet.Positives {
		if c.Kind != "operation" || !strings.HasPrefix(c.Name, "zero256/hosted/transfer/one/stage-") {
			continue
		}
		var row fightercarrier.Operation
		if e := json.Unmarshal(c.Value, &row); e != nil {
			t.Fatal(e)
		}
		rows[row.Stage] = row
	}
	if pin.Version == 0 || len(rows) != 8 || rows["signed"].SignedPackage == nil {
		t.Fatal("missing independent inputs")
	}
	return pin, rows
}

func TestOwnedTransferDeliveryLegsAndClosedOwner(t *testing.T) {
	_, rows := ownedDeliveryInputs(t)
	allowed := map[string]map[string]bool{"signed": {"commit": true}, "commit_dispatching": {"commit": true}, "commit_observed": {"spend": true}, "spend_dispatching": {"spend": true}}
	for stage, v := range rows {
		for _, leg := range []string{"commit", "spend", "other"} {
			_, e := ownedCarrierLeg(v, leg)
			if (e == nil) != allowed[stage][leg] {
				t.Errorf("%s/%s: %v", stage, leg, e)
			}
		}
	}
	var empty OwnedTransferActions
	if e := empty.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := empty.Deliver(context.Background(), strings.Repeat("1", 64)); !errors.Is(e, ErrPoolRuntime) {
		t.Fatal(e)
	}
	if _, e := NewOwnedTransferActions(context.Background(), nil, nil, strings.Repeat("1", 64)); !errors.Is(e, ErrPoolRuntime) {
		t.Fatal(e)
	}
}

type ownedDeliveryModel struct {
	mu                 sync.Mutex
	mode               string
	pin                fighterownership.Pin
	raw                fightercarrier.RawTransaction
	header, block      string
	entry              map[string]any
	obs                fighterownership.ChainObservation
	obsCalls, sends    int
	entered, cancelled chan struct{}
}

func (m *ownedDeliveryModel) serve(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID     uint64            `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&q) != nil {
		http.Error(w, "invalid", 400)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var value, failure any
	switch q.Method {
	case "getcurrentnet":
		value = m.pin.Source.Network
	case "getblockhash":
		var h uint32
		_ = json.Unmarshal(q.Params[0], &h)
		value = m.pin.Source.Genesis
		if h > 0 {
			value = strings.Repeat("7", 64)
		}
	case "getchainobservation":
		m.obsCalls++
		v := m.obs
		if m.obsCalls > 1 {
			if m.mode == "aba" || m.mode == "absent_aba" {
				v.Revision = "2"
			}
			if m.mode == "restart" {
				v.InstanceID = strings.Repeat("2", 64)
			}
		}
		value = v
	case "getrawtransaction":
		var verbosity int
		_ = json.Unmarshal(q.Params[1], &verbosity)
		if m.mode == "blocked" {
			m.mu.Unlock()
			close(m.entered)
			<-r.Context().Done()
			close(m.cancelled)
			m.mu.Lock()
			return
		}
		if strings.HasPrefix(m.mode, "absent") {
			failure = map[string]any{"code": -5, "message": "No information available about transaction " + m.raw.TxID}
			break
		}
		if verbosity == 0 {
			value = m.raw.Raw
			if m.mode == "wrong_raw" {
				value = "00"
			}
			break
		}
		v := map[string]any{"hex": m.raw.Raw, "txid": m.raw.TxID}
		if strings.HasPrefix(m.mode, "canonical") || m.mode == "missing_in_block" {
			v["blockhash"] = strings.Repeat("7", 64)
			v["confirmations"] = uint32(1)
		}
		if m.mode == "malformed" {
			v["blockhash"] = nil
		}
		value = v
	case "getmempoolentry":
		entry := make(map[string]any, len(m.entry))
		for k, v := range m.entry {
			entry[k] = v
		}
		value = entry
		switch m.mode {
		case "mempool_null":
			value = nil
		case "mempool_scalar":
			value = true
		case "mempool_empty":
			value = map[string]any{}
		case "mempool_wrong_size":
			entry["size"] = 1
		case "mempool_wrong_vsize":
			entry["vsize"] = 1
		case "mempool_wrong_weight":
			entry["weight"] = 1
		case "mempool_wrong_wtxid":
			entry["wtxid"] = strings.Repeat("4", 64)
		case "mempool_null_height":
			entry["height"] = nil
		case "mempool_negative_height":
			entry["height"] = -1
		case "mempool_large_height":
			entry["height"] = int64(math.MaxInt32) + 1
		case "mempool_prior_high_height":
			entry["height"] = 101
		}
		if m.mode == "mempool_gone" {
			value = nil
			failure = map[string]any{"code": -5, "message": "not in mempool"}
		}
	case "getblockheader":
		var verbose bool
		_ = json.Unmarshal(q.Params[1], &verbose)
		if verbose {
			value = map[string]any{"height": 100, "hash": strings.Repeat("7", 64)}
		} else {
			value = m.header
		}
	case "getblock":
		// This frame has an independently composed TxID Merkle root. It is
		// still a transport fixture, not a normal-work/canonicality proof.
		value = m.block
		if m.mode == "missing_in_block" {
			value = m.header + "00"
		}
		if m.mode == "canonical_mutated_body" {
			value = m.header + "02" + m.raw.Raw + m.raw.Raw
		}
		if m.mode == "canonical_claimed_count" {
			value = m.header + "feffffffff"
		}
	case "sendrawtransaction":
		m.sends++
		value = m.raw.TxID
	default:
		http.Error(w, "unexpected method", 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": q.ID, "result": value, "error": failure})
}

func ownedDeliveryNode(t *testing.T, mode string) (*OwnedTransferActions, *ownedDeliveryModel, string) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("owned chain refuses cheap profile")
	}
	pin, rows := ownedDeliveryInputs(t)
	pair := rows["signed"].SignedPackage
	m := &ownedDeliveryModel{mode: mode, pin: pin, raw: pair.Spend, obs: fighterownership.ChainObservation{Version: 1, InstanceID: strings.Repeat("1", 64), Revision: "1", Hash: strings.Repeat("7", 64), Height: 100}, entered: make(chan struct{}), cancelled: make(chan struct{})}
	decoded, e := hex.DecodeString(m.raw.Raw)
	if e != nil {
		t.Fatal(e)
	}
	var tx wire.MsgTx
	if e = tx.Deserialize(bytes.NewReader(decoded)); e != nil {
		t.Fatal(e)
	}
	var stripped bytes.Buffer
	if e = tx.SerializeNoWitness(&stripped); e != nil {
		t.Fatal(e)
	}
	id := sha256.Sum256(stripped.Bytes())
	id = sha256.Sum256(id[:])
	header := make([]byte, 80)
	copy(header[36:68], id[:])
	m.header = hex.EncodeToString(header)
	m.block = m.header + "01" + m.raw.Raw
	wtxid := sha256.Sum256(decoded)
	wtxid = sha256.Sum256(wtxid[:])
	for i, j := 0, 31; i < j; i, j = i+1, j-1 {
		wtxid[i], wtxid[j] = wtxid[j], wtxid[i]
	}
	weight := stripped.Len()*3 + len(decoded)
	m.entry = map[string]any{"size": len(decoded), "vsize": (weight + 3) / 4, "weight": weight, "height": 100, "wtxid": hex.EncodeToString(wtxid[:]), "time": 1000, "fee": 0.0001, "depends": []string{}}
	server := httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(server.Close)
	cfg := prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "public-fixture", Password: "public-fixture"}
	chain, e := fighterownership.NewOwnedChainSource(context.Background(), pin, cfg, context.Background())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = chain.Close() })
	node, e := newWithdrawalNode(cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(node.close)
	return &OwnedTransferActions{chain: chain, node: node}, m, pair.TransferID
}

func TestOwnedTransferDeliveryExactNodeObservations(t *testing.T) {
	for _, tc := range []struct {
		mode, state string
		fail        bool
	}{{"mempool", "mempool", false}, {"canonical", "canonical", false}, {"absent", "not_observed", false}, {"aba", "unknown", true}, {"absent_aba", "unknown", true}, {"restart", "unknown", true}, {"wrong_raw", "unknown", true}, {"malformed", "unknown", true}, {"missing_in_block", "unknown", true}, {"mempool_gone", "unknown", true}, {"mempool_null", "unknown", true}, {"mempool_scalar", "unknown", true}, {"mempool_empty", "unknown", true}, {"mempool_wrong_size", "unknown", true}, {"mempool_wrong_vsize", "unknown", true}, {"mempool_wrong_weight", "unknown", true}, {"mempool_wrong_wtxid", "unknown", true}, {"mempool_null_height", "unknown", true}, {"mempool_negative_height", "unknown", true}, {"mempool_large_height", "unknown", true}, {"mempool_prior_high_height", "mempool", false}, {"canonical_mutated_body", "unknown", true}, {"canonical_claimed_count", "unknown", true}} {
		t.Run(tc.mode, func(t *testing.T) {
			o, m, transfer := ownedDeliveryNode(t, tc.mode)
			v, e := o.observeCarrier(context.Background(), m.raw, transfer)
			if (e != nil) != tc.fail || v.State != tc.state {
				t.Fatalf("state=%s error=%v", v.State, e)
			}
			if !tc.fail && v.ExpectedTxID != m.raw.TxID {
				t.Fatal("wrong requested tx")
			}
			if v.State == "mempool" || v.State == "canonical" {
				if v.ObservedTxID != m.raw.TxID || v.RawSHA256 != m.raw.RawSHA256 || v.TransferID != transfer {
					t.Fatal("not exact retained transaction")
				}
			}
			if v.State == "canonical" && (v.BlockHeight != 100 || v.TransactionIndex != 0 || v.Confirmations != 1) {
				t.Fatal("inclusion mismatch")
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.sends != 0 {
				t.Fatal("observation transmitted")
			}
		})
	}
}

func TestOwnedTransferDeliveryCancellationJoinsRequest(t *testing.T) {
	o, m, transfer := ownedDeliveryNode(t, "blocked")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := o.observeCarrier(ctx, m.raw, transfer); done <- e }()
	select {
	case <-m.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not enter")
	}
	cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancelled observation succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("observation did not join")
	}
	select {
	case <-m.cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("socket request not cancelled")
	}
}

// Assert the independently supplied raw digest, separate from adapter output.
func TestOwnedTransferDeliveryReferenceRawHash(t *testing.T) {
	_, rows := ownedDeliveryInputs(t)
	for _, r := range []fightercarrier.RawTransaction{rows["signed"].SignedPackage.Commit, rows["signed"].SignedPackage.Spend} {
		b, e := hex.DecodeString(r.Raw)
		if e != nil {
			t.Fatal(e)
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != r.RawSHA256 {
			t.Fatal("independent carrier digest")
		}
	}
}
