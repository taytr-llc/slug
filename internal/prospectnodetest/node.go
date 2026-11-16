// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package prospectnodetest serves synthetic node responses for CLI tests.
// Its context and proofs come from the independent prospect reference fixtures.
package prospectnodetest

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/taytr-llc/slug/prospect"
)

const Password = "synthetic-test-rpc-password"

// Node is one independent, local scripted node. No real node credentials or
// state are read. A configured reorg begins after the selected tip response.
type Node struct {
	PolicyPath string
	ConfigPath string
	Context    prospect.Context
	requests   atomic.Int64
	snapshots  atomic.Int64
	reorgAfter atomic.Int64
}

// New starts a synthetic node and writes explicit policy/configuration files.
func New(t testing.TB, contextJSON []byte) *Node {
	t.Helper()
	context, err := prospect.ParseContext(contextJSON)
	if err != nil {
		t.Fatal(err)
	}
	node := &Node{Context: context}
	node.reorgAfter.Store(-1)
	server := httptest.NewServer(http.HandlerFunc(node.serve))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	node.PolicyPath = filepath.Join(dir, "policy.json")
	node.ConfigPath = filepath.Join(dir, "rpc.json")
	writeJSON(t, node.PolicyPath, map[string]any{
		"version": context.Version, "network": context.Network,
		"genesis": context.Genesis, "dna_version": context.DNAVersion,
		"prospect_target": context.ProspectTarget,
	})
	writeJSON(t, node.ConfigPath, map[string]any{
		"version": 1, "url": server.URL, "username": "synthetic-test-user",
		"password": Password, "ca_certificate": "",
	})
	return node
}

func writeJSON(t testing.TB, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

// ReorgAfter changes the reported parent after this many completed snapshots.
func (node *Node) ReorgAfter(snapshots int64) { node.reorgAfter.Store(snapshots) }

func (node *Node) Snapshots() int64 { return node.snapshots.Load() }
func (node *Node) Requests() int64  { return node.requests.Load() }

func (node *Node) serve(writer http.ResponseWriter, request *http.Request) {
	node.requests.Add(1)
	user, password, ok := request.BasicAuth()
	if !ok || user != "synthetic-test-user" || password != Password {
		http.Error(writer, "authentication failed", http.StatusUnauthorized)
		return
	}
	var call struct {
		Method string          `json:"method"`
		ID     json.RawMessage `json:"id"`
	}
	if request.Method != http.MethodPost || json.NewDecoder(io.LimitReader(request.Body, 16384)).Decode(&call) != nil {
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	parent := node.Context.ParentHash
	if after := node.reorgAfter.Load(); after >= 0 && node.snapshots.Load() >= after {
		parent = strings.Repeat("a", 64)
	}
	var result any
	switch call.Method {
	case "getcurrentnet":
		result = node.Context.Network
	case "getblockhash":
		result = node.Context.Genesis
	case "getblockchaininfo":
		result = map[string]any{"chain": "mainnet", "blocks": node.Context.ParentHeight,
			"bestblockhash": parent, "mediantime": node.Context.MinTimestamp - 1}
	case "getblocktemplate":
		// Decode the compact target independently of production helpers. This
		// models the node's block target, not the policy's prospect target.
		bits := node.Context.ExpectedBits
		target := new(big.Int).SetUint64(uint64(bits & 0x007fffff))
		exponent := uint(bits >> 24)
		if exponent <= 3 {
			target.Rsh(target, 8*(3-exponent))
		} else {
			target.Lsh(target, 8*(exponent-3))
		}
		result = map[string]any{
			"bits": fmt.Sprintf("%x", bits), "target": fmt.Sprintf("%064x", target),
			"previousblockhash": parent, "height": node.Context.ParentHeight + 1,
			"mintime": node.Context.MinTimestamp, "maxtime": node.Context.MaxTimestamp,
			"curtime": uint64(node.Context.MinTimestamp) + uint64(node.Context.MaxTimestamp-node.Context.MinTimestamp)/2,
		}
	case "getbestblock":
		result = map[string]any{"hash": parent, "height": node.Context.ParentHeight}
		node.snapshots.Add(1)
	default:
		http.Error(writer, "unexpected method", http.StatusBadRequest)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(struct {
		Result any             `json:"result"`
		Error  any             `json:"error"`
		ID     json.RawMessage `json:"id"`
	}{result, nil, call.ID})
}
