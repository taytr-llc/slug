//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectnode

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/prospectwork"
)

type templateFixture struct {
	Template prospectwork.Template  `json:"template"`
	Expected prospectwork.Candidate `json:"expected"`
}

func independentTemplate(t *testing.T) templateFixture {
	t.Helper()
	data, err := os.ReadFile("../prospectwork/testdata/templates_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []templateFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures[0]
}

// Full template bytes come from the independent Python fixture. The HTTP mock
// asserts only node protocol behavior, not consensus acceptance of its inputs.
func templateNode(t *testing.T, f templateFixture, proposal any, after func(string, any) any) (*Client, *mockNode, *atomic.Int64) {
	t.Helper()
	proposed := new(atomic.Int64)
	node := &mockNode{t: t, fixture: fixture{Context: f.Template.Context}}
	node.mutate = func(method string, _ int, result any) any {
		if method == "getblocktemplate" {
			result = f.Template.Block
		}
		if proposed.Load() > 0 && after != nil {
			result = after(method, result)
		}
		return result
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var req struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
			Params []struct {
				Mode string `json:"mode"`
				Data string `json:"data"`
			} `json:"params"`
		}
		// getblockhash has an integer parameter; only decode proposal parameters.
		var envelope struct {
			Method string `json:"method"`
		}
		if json.Unmarshal(data, &envelope) != nil {
			t.Error("bad request")
			return
		}
		if envelope.Method == "getblocktemplate" && json.Unmarshal(data, &req) == nil && len(req.Params) == 1 && req.Params[0].Mode == "proposal" {
			if req.Params[0].Data != f.Expected.BlockHex {
				t.Error("proposal changed candidate bytes")
			}
			proposed.Add(1)
			json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "result": proposal, "error": nil})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(data))
		node.ServeHTTP(w, r)
	})
	return newTestClient(t, handler), node, proposed
}

func templatePolicy(f templateFixture) Policy {
	c := f.Template.Context
	return Policy{Version: 1, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}
}

func TestResolveTemplateUsesOneBracketedGBT(t *testing.T) {
	f := independentTemplate(t)
	client, node, proposals := templateNode(t, f, nil, nil)
	got, err := client.ResolveTemplate(context.Background(), templatePolicy(f))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, f.Template) {
		t.Fatal("resolved complete template differs from bracketed response")
	}
	node.mu.Lock()
	count := len(node.methods)
	node.mu.Unlock()
	if count != 7 || proposals.Load() != 0 {
		t.Fatalf("calls=%d proposals=%d", count, proposals.Load())
	}
}

func TestResolveTemplateRejectsIncompleteGBT(t *testing.T) {
	f := independentTemplate(t)
	node := &mockNode{t: t, fixture: fixture{Context: f.Template.Context}}
	node.mutate = func(method string, _ int, result any) any {
		if method != "getblocktemplate" {
			return result
		}
		data, _ := json.Marshal(f.Template.Block)
		var fields map[string]any
		json.Unmarshal(data, &fields)
		delete(fields, "coinbasevalue")
		return fields
	}
	client := newTestClient(t, node)
	if _, err := client.ResolveTemplate(context.Background(), templatePolicy(f)); err == nil {
		t.Fatal("accepted incomplete GBT")
	}
}

func TestProposalIsNullableNonBroadcastAndRechecked(t *testing.T) {
	f := independentTemplate(t)
	client, node, proposals := templateNode(t, f, nil, nil)
	if err := client.Proposal(context.Background(), f.Template.Context, f.Expected.BlockHex); err != nil {
		t.Fatal(err)
	}
	node.mu.Lock()
	count := len(node.methods)
	node.mu.Unlock()
	if count != 14 || proposals.Load() != 1 {
		t.Fatalf("read calls=%d proposal calls=%d", count, proposals.Load())
	}
	// Ordinary reads must retain their original non-null contract.
	nullClient := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID uint64 `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "result": nil, "error": nil})
	}))
	if _, err := nullClient.call(context.Background(), "getcurrentnet", []any{}); err == nil {
		t.Fatal("ordinary read accepted null")
	}
}

func TestProposalRejectsChangedNodeAndRedactsReason(t *testing.T) {
	f := independentTemplate(t)
	cases := []struct {
		name   string
		result any
		mutate func(string, any) any
	}{
		{"rejection text", "<html>secret-password rejected</html>", nil},
		{"false result", false, nil},
		{"timestamp no longer eligible", nil, func(method string, result any) any {
			if method == "getblocktemplate" {
				block := result.(btcjson.GetBlockTemplateResult)
				block.CurTime--
				block.MaxTime = block.CurTime
				return block
			}
			return result
		}},
		{"wrong identity", nil, func(method string, result any) any {
			if method == "getcurrentnet" {
				return uint32(1)
			}
			return result
		}},
		{"reorganized parent", nil, func(method string, result any) any {
			if method == "getbestblock" {
				result.(map[string]any)["hash"] = strings.Repeat("1", 64)
			}
			return result
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _, proposals := templateNode(t, f, tc.result, tc.mutate)
			err := client.Proposal(context.Background(), f.Template.Context, f.Expected.BlockHex)
			if err == nil || strings.Contains(err.Error(), "secret-password") || strings.Contains(err.Error(), "<html>") {
				t.Fatalf("unsafe result: %v", err)
			}
			if proposals.Load() != 1 {
				t.Fatal("proposal not checked")
			}
		})
	}
}

func TestProposalChecksContextBeforeProposal(t *testing.T) {
	f := independentTemplate(t)
	client, node, proposals := templateNode(t, f, nil, nil)
	original := node.mutate
	node.mutate = func(method string, index int, result any) any {
		result = original(method, index, result)
		if method == "getbestblock" {
			result.(map[string]any)["hash"] = strings.Repeat("1", 64)
		}
		return result
	}
	if err := client.Proposal(context.Background(), f.Template.Context, f.Expected.BlockHex); err == nil {
		t.Fatal("accepted stale candidate")
	}
	if proposals.Load() != 0 {
		t.Fatal("stale candidate reached proposal RPC")
	}
}

func TestProposalRejectsInvalidCandidateBeforeRPC(t *testing.T) {
	f := independentTemplate(t)
	cases := map[string]string{
		"trailing bytes":     f.Expected.BlockHex + "00",
		"bad hex":            "zz" + f.Expected.BlockHex[2:],
		"wrong parent":       f.Expected.BlockHex[:8] + strings.Repeat("00", 32) + f.Expected.BlockHex[72:],
		"wrong bits":         f.Expected.BlockHex[:144] + "00000000" + f.Expected.BlockHex[152:],
		"wrong time":         f.Expected.BlockHex[:136] + "00000000" + f.Expected.BlockHex[144:],
		"noncanonical count": f.Expected.BlockHex[:160] + "fd0100" + f.Expected.BlockHex[162:],
		"oversized":          strings.Repeat("00", 4_000_001),
	}
	for name, block := range cases {
		t.Run(name, func(t *testing.T) {
			client, node, proposals := templateNode(t, f, nil, nil)
			if err := client.Proposal(context.Background(), f.Template.Context, block); err == nil {
				t.Fatal("accepted invalid block")
			}
			node.mu.Lock()
			count := len(node.methods)
			node.mu.Unlock()
			if count != 0 || proposals.Load() != 0 {
				t.Fatal("invalid candidate reached RPC")
			}
		})
	}
}
