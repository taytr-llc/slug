//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectnode

import (
	"context"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospect"
)

func TestRecheckContextIssuanceGuard(t *testing.T) {
	f := independentFixture(t)
	for _, mode := range []string{"same", "forward time", "backward time", "parent", "height", "bits", "minimum time"} {
		t.Run(mode, func(t *testing.T) {
			node := &mockNode{t: t, fixture: f, mutate: func(method string, _ int, value any) any {
				m, ok := value.(map[string]any)
				if !ok {
					return value
				}
				switch mode {
				case "forward time":
					if method == "getblocktemplate" {
						m["maxtime"] = f.Context.MaxTimestamp + 1
					}
				case "backward time":
					if method == "getblocktemplate" {
						m["maxtime"] = f.Context.MaxTimestamp - 1
					}
				case "parent":
					switch method {
					case "getblockchaininfo":
						m["bestblockhash"] = strings.Repeat("1", 64)
					case "getblocktemplate":
						m["previousblockhash"] = strings.Repeat("1", 64)
					case "getbestblock":
						m["hash"] = strings.Repeat("1", 64)
					}
				case "height":
					switch method {
					case "getblockchaininfo":
						m["blocks"] = f.Context.ParentHeight + 1
					case "getblocktemplate":
						m["height"] = f.Context.ParentHeight + 2
					case "getbestblock":
						m["height"] = f.Context.ParentHeight + 1
					}
				case "bits":
					if method == "getblocktemplate" {
						m["bits"] = "1020000"
						m["target"] = strings.Repeat("0", 63) + "2"
					}
				case "minimum time":
					if method == "getblockchaininfo" {
						m["mediantime"] = f.Context.MinTimestamp
					}
					if method == "getblocktemplate" {
						m["mintime"] = f.Context.MinTimestamp + 1
					}
				}
				return m
			}}
			client := newTestClient(t, node)
			err := client.RecheckContext(context.Background(), f.Context)
			if (err == nil) != (mode == "same" || mode == "forward time") {
				t.Fatalf("unexpected issuance guard result: %v", err)
			}
		})
	}
}

func TestRecheckContextRejectsMalformedInitialBeforeNetwork(t *testing.T) {
	f := independentFixture(t)
	for _, mutate := range []func(*prospect.Context){
		func(c *prospect.Context) { c.Version = 2 }, func(c *prospect.Context) { c.Network++ },
		func(c *prospect.Context) { c.ParentHash = "00" }, func(c *prospect.Context) { c.ParentHeight = 2147483647 },
		func(c *prospect.Context) { c.ExpectedBits = 0x02000100 }, func(c *prospect.Context) { c.ExpectedBits = 0 },
		func(c *prospect.Context) { c.MinTimestamp = 0 }, func(c *prospect.Context) { c.MaxTimestamp = c.MinTimestamp - 1 },
		func(c *prospect.Context) { c.ProspectTarget = "" }, func(c *prospect.Context) { c.Genesis = "" },
	} {
		node := &mockNode{t: t, fixture: f}
		client := newTestClient(t, node)
		initial := f.Context
		mutate(&initial)
		if err := client.RecheckContext(context.Background(), initial); err == nil {
			t.Fatal("accepted malformed initial context")
		}
		node.mu.Lock()
		calls := len(node.methods)
		node.mu.Unlock()
		if calls != 0 {
			t.Fatal("malformed initial context caused node requests")
		}
	}
}

func TestRecheckVersionedJobShapes(t *testing.T) {
	f := independentFixture(t)
	for _, tc := range []struct {
		version uint32
		job     string
		valid   bool
	}{
		{1, "", true}, {1, strings.Repeat("0", 64), false},
		{2, strings.Repeat("0", 64), true}, {2, strings.Repeat("A", 64), true},
		{2, "", false}, {2, "0", false}, {2, strings.Repeat("z", 64), false}, {3, "", false},
	} {
		node := &mockNode{t: t, fixture: f}
		client := newTestClient(t, node)
		proof := f.Proof
		proof.Version = tc.version
		proof.JobID = tc.job
		// Recheck inspects only an already-verified proof's header and job
		// encoding. Actual v2 work is checked by independent prospectjob tests.
		err := client.Recheck(context.Background(), f.Context, proof)
		if (err == nil) != tc.valid {
			t.Fatalf("version %d job %q: %v", tc.version, tc.job, err)
		}
		node.mu.Lock()
		calls := len(node.methods)
		node.mu.Unlock()
		if !tc.valid && calls != 0 {
			t.Fatal("malformed job/version caused node requests")
		}
	}
}
