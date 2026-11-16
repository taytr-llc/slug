//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectnode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/prospect"
)

type winningFixture struct {
	Context  prospect.Context  `json:"context"`
	Proof    prospect.Proof    `json:"proof"`
	Verified prospect.Verified `json:"expected_verified"`
	BlockHex string            `json:"block_hex"`
}

func independentWinner(t *testing.T) winningFixture {
	t.Helper()
	data, err := os.ReadFile("../registry/testdata/mining_winner_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f winningFixture
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

type admissionRequest struct {
	Method string
	Params []json.RawMessage
}
type admissionNode struct {
	t             *testing.T
	f             winningFixture
	currentParent bool
	failMethod    string
	mutate        func(string, []json.RawMessage, int, any) any
	mu            sync.Mutex
	calls         []admissionRequest
	counts        map[string]int
}

func (node *admissionNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     uint64            `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		node.t.Error("invalid node request")
		return
	}
	node.mu.Lock()
	defer node.mu.Unlock()
	node.calls = append(node.calls, admissionRequest{req.Method, req.Params})
	if node.counts == nil {
		node.counts = make(map[string]int)
	}
	node.counts[req.Method]++
	var result any
	c := node.f.Context
	switch req.Method {
	case "getcurrentnet":
		result = c.Network
	case "getblockhash":
		var h uint32
		if len(req.Params) != 1 || json.Unmarshal(req.Params[0], &h) != nil {
			node.t.Error("invalid height request")
			return
		}
		switch h {
		case 0:
			result = c.Genesis
		case c.ParentHeight:
			result = c.ParentHash
		case c.ParentHeight + 1:
			result = node.f.Verified.WorkHash
		default:
			node.t.Errorf("unexpected height %d", h)
			return
		}
	case "getblockheader":
		want, _ := json.Marshal(node.f.Verified.WorkHash)
		if len(req.Params) != 2 || string(req.Params[0]) != string(want) || string(req.Params[1]) != "false" {
			node.t.Error("header request did not bind exact winner/raw bytes")
			return
		}
		result = node.f.Proof.Header
	case "getblockchaininfo":
		height, hash := c.ParentHeight, c.ParentHash
		if !node.currentParent {
			height += 5
			hash = strings.Repeat("3", 64)
		}
		result = map[string]any{"chain": "mainnet", "blocks": height, "bestblockhash": hash, "mediantime": c.MinTimestamp - 1}
	case "getbestblock":
		result = map[string]any{"hash": c.ParentHash, "height": c.ParentHeight}
	case "getblocktemplate":
		_, target, _ := compactTarget(strconv.FormatUint(uint64(c.ExpectedBits), 16))
		result = map[string]any{"previousblockhash": c.ParentHash, "height": c.ParentHeight + 1, "bits": fmt.Sprintf("%x", c.ExpectedBits), "target": target, "mintime": c.MinTimestamp, "curtime": c.MinTimestamp, "maxtime": c.MaxTimestamp}
	default:
		node.t.Errorf("unexpected RPC %s", req.Method)
		return
	}
	if node.mutate != nil {
		result = node.mutate(req.Method, req.Params, node.counts[req.Method], result)
	}
	var rpcError any
	if req.Method == node.failMethod {
		rpcError = map[string]any{"code": -1, "message": "secret-password <html>failure</html>"}
	}
	json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "result": result, "error": rpcError})
}
func (node *admissionNode) count(method string) int {
	node.mu.Lock()
	defer node.mu.Unlock()
	return node.counts[method]
}

func TestRecheckAdmissionCurrentParentAndCanonicalWinner(t *testing.T) {
	f := independentWinner(t)
	for _, current := range []bool{true, false} {
		t.Run(fmt.Sprint(current), func(t *testing.T) {
			node := &admissionNode{t: t, f: f, currentParent: current}
			client := newTestClient(t, node)
			got, err := client.RecheckAdmission(context.Background(), f.Context, f.Proof, f.Verified)
			if err != nil {
				t.Fatal(err)
			}
			want := "canonical-winning-block"
			if current {
				want = "current-parent"
			}
			if got != want {
				t.Fatalf("source=%q", got)
			}
			if current && node.count("getblockheader") != 0 {
				t.Fatal("current-parent path queried historical header")
			}
			if !current {
				if node.count("getblockheader") != 1 || node.count("getblocktemplate") != 0 || node.count("getblockchaininfo") != 1 {
					t.Fatal("canonical path repeated decision or used current GBT")
				}
				node.mu.Lock()
				last := node.calls[len(node.calls)-1]
				node.mu.Unlock()
				if last.Method != "getblockhash" || string(last.Params[0]) != fmt.Sprint(f.Context.ParentHeight+1) {
					t.Fatal("last observation was not winner membership")
				}
			}
		})
	}
}

func TestCanonicalWinnerAllowsTipProgressButRejectsReorgs(t *testing.T) {
	f := independentWinner(t)
	cases := []struct {
		name   string
		mutate func(string, []json.RawMessage, int, any) any
	}{
		{"side chain winner", func(m string, p []json.RawMessage, n int, v any) any {
			if m == "getblockhash" && string(p[0]) == fmt.Sprint(f.Context.ParentHeight+1) {
				return strings.Repeat("1", 64)
			}
			return v
		}},
		{"original parent replaced", func(m string, p []json.RawMessage, n int, v any) any {
			if m == "getblockhash" && string(p[0]) == fmt.Sprint(f.Context.ParentHeight) {
				return strings.Repeat("1", 64)
			}
			return v
		}},
		{"last membership changed", func(m string, p []json.RawMessage, n int, v any) any {
			if m == "getblockhash" && n == 6 {
				return strings.Repeat("1", 64)
			}
			return v
		}},
		{"header bytes changed", func(m string, p []json.RawMessage, n int, v any) any {
			if m == "getblockheader" {
				return f.Proof.Header[:152] + "ffffffff"
			}
			return v
		}},
		{"verbose header object", func(m string, p []json.RawMessage, n int, v any) any {
			if m == "getblockheader" {
				return map[string]any{"hex": f.Proof.Header}
			}
			return v
		}},
		{"identity switched", func(m string, p []json.RawMessage, n int, v any) any {
			if m == "getcurrentnet" && n == 2 {
				return uint32(1)
			}
			return v
		}},
	}
	good := &admissionNode{t: t, f: f}
	if err := newTestClient(t, good).CanonicalWinner(context.Background(), f.Context, f.Proof, f.Verified); err != nil {
		t.Fatal(err)
	}
	if good.count("getblockchaininfo") != 0 {
		t.Fatal("canonical recovery unnecessarily depends on current tip equality")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := &admissionNode{t: t, f: f, mutate: tc.mutate}
			if err := newTestClient(t, node).CanonicalWinner(context.Background(), f.Context, f.Proof, f.Verified); err == nil {
				t.Fatal("accepted noncanonical/mismatched winner")
			}
		})
	}
}

func TestAdmissionNeverFallsBackAfterCurrentPathRPCFailure(t *testing.T) {
	f := independentWinner(t)
	for _, method := range []string{"getblockchaininfo", "getblocktemplate", "getbestblock"} {
		t.Run(method, func(t *testing.T) {
			node := &admissionNode{t: t, f: f, currentParent: true, failMethod: method}
			source, err := newTestClient(t, node).RecheckAdmission(context.Background(), f.Context, f.Proof, f.Verified)
			if err == nil || source != "" || strings.Contains(err.Error(), "secret-password") {
				t.Fatalf("unsafe failure %q %v", source, err)
			}
			if node.count("getblockheader") != 0 {
				t.Fatal("RPC error triggered canonical fallback")
			}
			node.mu.Lock()
			defer node.mu.Unlock()
			for _, call := range node.calls {
				if call.Method == "getblockhash" && string(call.Params[0]) != "0" {
					t.Fatal("RPC error triggered historical lookup")
				}
			}
		})
	}
}

func TestCanonicalBlockDoesNotRequireProspectEligibility(t *testing.T) {
	f := independentWinner(t)
	initial := f.Context
	// The complete block's canonicality is independent of the league's target
	// or coinbase policy commitment. No prospect admission is asserted here.
	initial.ProspectTarget = strings.Repeat("0", 63) + "1"
	node := &admissionNode{t: t, f: f}
	client := newTestClient(t, node)
	if err := client.CanonicalBlock(context.Background(), initial, f.Proof, f.Verified.WorkHash); err != nil {
		t.Fatal(err)
	}
	if err := client.CanonicalBlock(context.Background(), initial, f.Proof, strings.Repeat("f", 64)); err == nil {
		t.Fatal("accepted work above the block target")
	}
}

func TestAdmissionRejectsWrongBindingAndNonWinningWork(t *testing.T) {
	f := independentWinner(t)
	for _, field := range []string{"network", "gym", "source", "parent", "version", "header", "work target"} {
		t.Run(field, func(t *testing.T) {
			proof, verified := f.Proof, f.Verified
			switch field {
			case "network":
				verified.Network++
			case "gym":
				verified.GymID = strings.Repeat("1", 64)
			case "source":
				verified.Verification = "claimant"
			case "parent":
				verified.ParentHeight++
			case "version":
				verified.Version++
			case "header":
				proof.Header = proof.Header[:158]
			case "work target":
				verified.WorkHash = strings.Repeat("f", 64)
			}
			node := &admissionNode{t: t, f: f}
			if err := newTestClient(t, node).CanonicalWinner(context.Background(), f.Context, proof, verified); err == nil {
				t.Fatal("accepted invalid trusted-result binding or block target")
			}
			if node.count("getblockheader") != 0 {
				t.Fatal("invalid binding or target reached header query")
			}
		})
	}
}
