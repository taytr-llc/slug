//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectnode

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// The exact winner/header come from the pre-existing independent normal-PoW
// fixture. Tip heights and alternate branch hashes are controlled RPC models,
// not a claim that this test mines a 60-block chain.
type poolRoundNode struct {
	t          *testing.T
	f          winningFixture
	height     uint32
	parent     string
	winner     string
	header     string
	tipHash    string
	failMethod string
	mutate     func(string, []json.RawMessage, int, any) any
	mu         sync.Mutex
	counts     map[string]int
}

func newPoolRoundNode(t *testing.T, descendants uint32) *poolRoundNode {
	t.Helper()
	f := independentWinner(t)
	n := &poolRoundNode{t: t, f: f, height: f.Context.ParentHeight + 1 + descendants,
		parent: f.Context.ParentHash, winner: f.Verified.WorkHash, header: f.Proof.Header,
		tipHash: strings.Repeat("3", 64), counts: make(map[string]int)}
	if descendants == 0 {
		n.tipHash = n.winner
	}
	return n
}

func (n *poolRoundNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     uint64            `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		n.t.Error("invalid node request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.counts[req.Method]++
	var result any
	switch req.Method {
	case "getcurrentnet":
		result = n.f.Context.Network
	case "getblockchaininfo":
		result = map[string]any{"chain": "mainnet", "blocks": n.height,
			"bestblockhash": n.tipHash, "mediantime": n.f.Context.MinTimestamp - 1}
	case "getblockhash":
		var height uint32
		if len(req.Params) != 1 || json.Unmarshal(req.Params[0], &height) != nil {
			n.t.Error("invalid height lookup")
			return
		}
		switch height {
		case 0:
			result = n.f.Context.Genesis
		case n.f.Context.ParentHeight:
			result = n.parent
		case n.f.Context.ParentHeight + 1:
			result = n.winner
		case n.height:
			result = n.tipHash
		default:
			n.t.Errorf("unexpected height %d", height)
			return
		}
	case "getblockheader":
		var hash string
		if len(req.Params) != 2 || json.Unmarshal(req.Params[0], &hash) != nil || hash != n.f.Verified.WorkHash || string(req.Params[1]) != "false" {
			n.t.Error("header query lost exact work binding")
			return
		}
		result = n.header
	default:
		n.t.Errorf("unexpected node method %s", req.Method)
		return
	}
	if n.mutate != nil {
		result = n.mutate(req.Method, req.Params, n.counts[req.Method], result)
	}
	var rpcError any
	if req.Method == n.failMethod {
		rpcError = map[string]any{"code": -1, "message": "secret-fixture-password node error"}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "result": result, "error": rpcError})
}

func (n *poolRoundNode) count(method string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.counts[method]
}

func TestPoolRoundObservationDescendants(t *testing.T) {
	for _, depth := range []uint32{0, 59, 60, 100} {
		n := newPoolRoundNode(t, depth)
		client := newTestClient(t, n)
		got, err := client.ObservePoolBlock(context.Background(), n.f.Context, n.f.Proof, n.f.Verified.WorkHash)
		want := PoolBlockObservation{BlockHash: n.f.Verified.WorkHash, BlockHeight: n.f.Context.ParentHeight + 1,
			TipHash: n.tipHash, TipHeight: n.height, Canonical: true, Descendants: depth}
		if err != nil || got != want {
			t.Fatalf("depth %d: got %+v, err=%v; want %+v", depth, got, err, want)
		}
		if n.count("getblockheader") != 1 || n.count("getblockchaininfo") != 2 || n.count("getcurrentnet") != 2 {
			t.Fatal("observation omitted exact header or stable identity/tip brackets")
		}
	}
}

func TestPoolRoundOrphanAndExactReturn(t *testing.T) {
	n := newPoolRoundNode(t, 60)
	n.winner = strings.Repeat("4", 64)
	client := newTestClient(t, n)
	orphan, err := client.ObservePoolBlock(context.Background(), n.f.Context, n.f.Proof, n.f.Verified.WorkHash)
	if err != nil || orphan.Canonical || orphan.Descendants != 0 || orphan.BlockHash != n.f.Verified.WorkHash || orphan.BlockHeight != n.f.Context.ParentHeight+1 {
		t.Fatal("noncanonical branch did not produce a definite zero-depth observation", orphan, err)
	}
	if n.count("getblockheader") != 0 {
		t.Fatal("orphan observation unnecessarily required an indexed orphan body/header")
	}
	n.mu.Lock()
	n.winner = n.f.Verified.WorkHash
	n.tipHash = strings.Repeat("5", 64)
	n.mu.Unlock()
	restored, err := client.ObservePoolBlock(context.Background(), n.f.Context, n.f.Proof, n.f.Verified.WorkHash)
	if err != nil || !restored.Canonical || restored.Descendants != 60 || restored.BlockHash != orphan.BlockHash || restored.BlockHeight != orphan.BlockHeight {
		t.Fatal("exact returned block changed identity or failed confirmation-depth observation", restored, err)
	}

	short := newPoolRoundNode(t, 0)
	short.height = short.f.Context.ParentHeight
	short.tipHash = short.parent
	got, err := newTestClient(t, short).ObservePoolBlock(context.Background(), short.f.Context, short.f.Proof, short.f.Verified.WorkHash)
	if err != nil || got.Canonical || got.Descendants != 0 || got.TipHeight != short.height {
		t.Fatal("shorter branch was not a definite noncanonical observation", got, err)
	}
}

func TestPoolRoundUnavailableIsNotOrphan(t *testing.T) {
	for _, mode := range []string{"header", "tip-race", "final-membership", "tip-anchor", "identity", "lookup-error", "ancestry"} {
		t.Run(mode, func(t *testing.T) {
			n := newPoolRoundNode(t, 60)
			switch mode {
			case "header":
				n.header = n.f.Proof.Header[:152] + "ffffffff"
			case "tip-race":
				n.mutate = func(method string, _ []json.RawMessage, count int, value any) any {
					if method == "getblockchaininfo" && count == 2 {
						return map[string]any{"chain": "mainnet", "blocks": n.height + 1,
							"bestblockhash": strings.Repeat("6", 64), "mediantime": n.f.Context.MinTimestamp - 1}
					}
					return value
				}
			case "final-membership":
				winnerReads := 0
				n.mutate = func(method string, params []json.RawMessage, _ int, value any) any {
					if method == "getblockhash" {
						var height uint32
						_ = json.Unmarshal(params[0], &height)
						if height == n.f.Context.ParentHeight+1 {
							winnerReads++
							if winnerReads == 2 {
								return strings.Repeat("4", 64)
							}
						}
					}
					return value
				}
			case "tip-anchor":
				n.mutate = func(method string, params []json.RawMessage, _ int, value any) any {
					if method == "getblockhash" {
						var height uint32
						_ = json.Unmarshal(params[0], &height)
						if height == n.height {
							return strings.Repeat("4", 64)
						}
					}
					return value
				}
			case "identity":
				n.mutate = func(method string, _ []json.RawMessage, count int, value any) any {
					if method == "getcurrentnet" && count == 2 {
						return uint32(1)
					}
					return value
				}
			case "lookup-error":
				n.failMethod = "getblockhash"
			case "ancestry":
				n.parent = strings.Repeat("4", 64)
			}
			got, err := newTestClient(t, n).ObservePoolBlock(context.Background(), n.f.Context, n.f.Proof, n.f.Verified.WorkHash)
			if err == nil || got != (PoolBlockObservation{}) || strings.Contains(err.Error(), "secret-fixture-password") {
				t.Fatal("moving, unavailable or inconsistent evidence became an observation", got, err)
			}
		})
	}
}

func TestPoolRoundInvalidInputRefusesBeforeNetwork(t *testing.T) {
	n := newPoolRoundNode(t, 60)
	client := newTestClient(t, n)
	for _, mode := range []string{"nil-context", "cancelled", "bad-hash", "above-target", "wrong-header"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			proof, hash := n.f.Proof, n.f.Verified.WorkHash
			switch mode {
			case "nil-context":
				ctx = nil
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			case "bad-hash":
				hash = "invalid"
			case "above-target":
				hash = strings.Repeat("f", 64)
			case "wrong-header":
				proof.Header = "00"
			}
			got, err := client.ObservePoolBlock(ctx, n.f.Context, proof, hash)
			if err == nil || got != (PoolBlockObservation{}) {
				t.Fatal("invalid input produced observation", got, err)
			}
		})
	}
	if n.count("getcurrentnet") != 0 {
		t.Fatal("invalid request reached the node")
	}
}
