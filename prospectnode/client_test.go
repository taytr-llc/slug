//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectnode

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospect"
)

type fixture struct {
	Context  prospect.Context  `json:"context"`
	Proof    prospect.Proof    `json:"proof"`
	Expected prospect.Verified `json:"expected"`
}

func independentFixture(t *testing.T) fixture {
	t.Helper()
	data, err := os.ReadFile("../prospect/testdata/proofs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []fixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures[0]
}

// These mocked node assertions deliberately reuse the independent synthetic
// fixture's target-one header. They do not claim its bits match the live chain.
type mockNode struct {
	t       *testing.T
	fixture fixture
	mu      sync.Mutex
	methods []string
	mutate  func(string, int, any) any
}

func (node *mockNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
		node.t.Error("invalid RPC HTTP request")
	}
	if user, password, ok := r.BasicAuth(); !ok || user != "test-user" || password != "secret-password" {
		node.t.Error("missing RPC authentication")
	}
	var request struct {
		JSONRPC string            `json:"jsonrpc"`
		ID      uint64            `json:"id"`
		Method  string            `json:"method"`
		Params  []json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		node.t.Error(err)
		return
	}
	node.mu.Lock()
	node.methods = append(node.methods, request.Method)
	index := len(node.methods)
	node.mu.Unlock()
	ctx := node.fixture.Context
	var result any
	switch request.Method {
	case "getcurrentnet":
		result = ctx.Network
	case "getblockhash":
		if len(request.Params) != 1 || string(request.Params[0]) != "0" {
			node.t.Error("genesis lookup did not request height zero")
		}
		result = ctx.Genesis
	case "getblockchaininfo":
		result = map[string]any{"chain": "mainnet", "blocks": ctx.ParentHeight, "bestblockhash": ctx.ParentHash, "mediantime": ctx.MinTimestamp - 1, "headers": ctx.ParentHeight}
	case "getbestblock":
		result = map[string]any{"hash": ctx.ParentHash, "height": ctx.ParentHeight}
	case "getblocktemplate":
		if len(request.Params) != 1 || string(request.Params[0]) != `{"mode":"template","capabilities":["coinbasevalue"],"rules":["segwit"]}` {
			node.t.Errorf("unexpected template request: %s", request.Params)
		}
		result = map[string]any{"previousblockhash": ctx.ParentHash, "height": ctx.ParentHeight + 1, "bits": fmt.Sprintf("%x", ctx.ExpectedBits), "target": strings.Repeat("0", 63) + "1", "mintime": ctx.MinTimestamp, "curtime": uint32(1794787200), "maxtime": ctx.MaxTimestamp, "transactions": []any{}}
	default:
		node.t.Errorf("unexpected RPC %q", request.Method)
		return
	}
	if node.mutate != nil {
		result = node.mutate(request.Method, index, result)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"id": request.ID, "error": nil, "result": result}); err != nil {
		node.t.Error(err)
	}
}

func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(RPCConfig{Version: 1, URL: server.URL, Username: "test-user", Password: "secret-password"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestResolveConsistentContext(t *testing.T) {
	f := independentFixture(t)
	node := &mockNode{t: t, fixture: f}
	client := newTestClient(t, node)
	got, err := client.Resolve(context.Background(), testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if got != f.Context {
		t.Fatalf("context differs from independent fixture: %+v", got)
	}
	wantMethods := []string{"getcurrentnet", "getblockhash", "getblockchaininfo", "getblocktemplate", "getbestblock", "getcurrentnet", "getblockhash"}
	node.mu.Lock()
	methods := append([]string(nil), node.methods...)
	node.mu.Unlock()
	if !reflect.DeepEqual(methods, wantMethods) {
		t.Fatalf("RPC sequence: %v", methods)
	}
	if got.ProspectTarget == strings.Repeat("0", 63)+"1" {
		t.Fatal("node target replaced independent policy")
	}
}

func TestResolveRejectsMalformedAndInconsistentNodeAssertions(t *testing.T) {
	f := independentFixture(t)
	type change struct {
		name, method, key string
		value             any
		remove            bool
		onlyAt            int
	}
	changes := []change{
		{"wrong network", "getcurrentnet", "", uint32(1), false, 0},
		{"switched network", "getcurrentnet", "", uint32(1), false, 6},
		{"wrong genesis", "getblockhash", "", strings.Repeat("1", 64), false, 0},
		{"switched genesis", "getblockhash", "", strings.Repeat("1", 64), false, 7},
		{"wrong chain", "getblockchaininfo", "chain", "regtest", false, 0},
		{"negative height", "getblockchaininfo", "blocks", -1, false, 0},
		{"height overflow", "getblockchaininfo", "blocks", 2147483647, false, 0},
		{"fractional height", "getblockchaininfo", "blocks", 1.5, false, 0},
		{"median overflow", "getblockchaininfo", "mediantime", uint64(4294967295), false, 0},
		{"parent mismatch", "getblocktemplate", "previousblockhash", strings.Repeat("1", 64), false, 0},
		{"height mismatch", "getblocktemplate", "height", 63, false, 0},
		{"MTP mismatch", "getblocktemplate", "mintime", f.Context.MinTimestamp + 1, false, 0},
		{"future template", "getblocktemplate", "curtime", f.Context.MaxTimestamp + 1, false, 0},
		{"old template", "getblocktemplate", "curtime", f.Context.MinTimestamp - 1, false, 0},
		{"timestamp overflow", "getblocktemplate", "maxtime", uint64(4294967296), false, 0},
		{"negative timestamp", "getblocktemplate", "mintime", -1, false, 0},
		{"target mismatch", "getblocktemplate", "target", strings.Repeat("f", 64), false, 0},
		{"short target", "getblocktemplate", "target", "1", false, 0},
		{"noncanonical compact", "getblocktemplate", "bits", "02000100", false, 0},
		{"negative compact", "getblocktemplate", "bits", "01810000", false, 0},
		{"tip reorganization", "getbestblock", "hash", strings.Repeat("1", 64), false, 0},
		{"tip extension", "getbestblock", "height", 62, false, 0},
		{"duplicate critical key", "getbestblock", "", json.RawMessage(`{"hash":"` + f.Context.ParentHash + `","height":61,"\u0068eight":61}`), false, 0},
	}
	for method, keys := range map[string][]string{
		"getblockchaininfo": {"chain", "blocks", "bestblockhash", "mediantime"},
		"getblocktemplate":  {"previousblockhash", "height", "bits", "target", "mintime", "curtime", "maxtime"},
		"getbestblock":      {"hash", "height"},
	} {
		for _, key := range keys {
			changes = append(changes, change{"missing " + method + "/" + key, method, key, nil, true, 0}, change{"null " + method + "/" + key, method, key, nil, false, 0})
		}
	}
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) {
			node := &mockNode{t: t, fixture: f, mutate: func(method string, index int, value any) any {
				if method != tc.method || (tc.onlyAt != 0 && index != tc.onlyAt) {
					return value
				}
				if tc.key == "" {
					return tc.value
				}
				result := value.(map[string]any)
				if tc.remove {
					delete(result, tc.key)
				} else {
					result[tc.key] = tc.value
				}
				return result
			}}
			client := newTestClient(t, node)
			got, err := client.Resolve(context.Background(), testPolicy())
			if err == nil || got != (prospect.Context{}) {
				t.Fatalf("accepted inconsistent context: %+v %v", got, err)
			}
		})
	}
}

func TestCanonicalCompactTargets(t *testing.T) {
	// Standard mathematical compact encodings, independent of implementation.
	for _, tc := range []struct{ bits, target string }{
		{"01010000", strings.Repeat("0", 63) + "1"},
		{"02008000", strings.Repeat("0", 62) + "80"},
		{"207fffff", "7fffff" + strings.Repeat("0", 58)},
		{"1d00ffff", strings.Repeat("0", 8) + "ffff" + strings.Repeat("0", 52)},
	} {
		_, target, err := compactTarget(tc.bits)
		if err != nil || target != tc.target {
			t.Errorf("bits %s: target %s error %v", tc.bits, target, err)
		}
	}
	for _, bits := range []string{"", "0", "00000000", "01000000", "00010000", "01810000", "02000100", "21008000", "ff7fffff", "+1010000", "0x1010000", "101000000", "zzzz"} {
		if _, _, err := compactTarget(bits); err == nil {
			t.Errorf("accepted bits %q", bits)
		}
	}
}

func TestRecheckFreshBoundsAndParent(t *testing.T) {
	f := independentFixture(t)
	for _, mode := range []string{"advancing max", "expired timestamp", "changed parent", "changed bits", "changed MTP"} {
		t.Run(mode, func(t *testing.T) {
			node := &mockNode{t: t, fixture: f, mutate: func(method string, _ int, value any) any {
				m, ok := value.(map[string]any)
				if !ok {
					return value
				}
				switch mode {
				case "advancing max":
					if method == "getblocktemplate" {
						m["maxtime"] = f.Context.MaxTimestamp + 10
					}
				case "expired timestamp":
					if method == "getblocktemplate" {
						m["maxtime"] = 1794787199
						m["curtime"] = f.Context.MinTimestamp
					}
				case "changed parent":
					switch method {
					case "getblockchaininfo":
						m["bestblockhash"] = strings.Repeat("1", 64)
					case "getblocktemplate":
						m["previousblockhash"] = strings.Repeat("1", 64)
					case "getbestblock":
						m["hash"] = strings.Repeat("1", 64)
					}
				case "changed bits":
					if method == "getblocktemplate" {
						m["bits"] = "01020000"
						m["target"] = strings.Repeat("0", 63) + "2"
					}
				case "changed MTP":
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
			err := client.Recheck(context.Background(), f.Context, f.Proof)
			if (err == nil) != (mode == "advancing max") {
				t.Fatalf("unexpected recheck result: %v", err)
			}
		})
	}
}

func TestVerifyIndependentProofAndFinalRecheck(t *testing.T) {
	f := independentFixture(t)
	for _, changeAfterVerification := range []bool{false, true} {
		t.Run(fmt.Sprint(changeAfterVerification), func(t *testing.T) {
			node := &mockNode{t: t, fixture: f, mutate: func(method string, index int, value any) any {
				if changeAfterVerification && index > 7 && method == "getcurrentnet" {
					return uint32(1)
				}
				return value
			}}
			client := newTestClient(t, node)
			ctx, verified, err := client.Verify(context.Background(), testPolicy(), f.Proof)
			if changeAfterVerification {
				if err == nil || ctx != (prospect.Context{}) || verified != (prospect.Verified{}) {
					t.Fatal("returned result after failed final recheck")
				}
			} else if err != nil || ctx != f.Context || verified != f.Expected {
				t.Fatalf("independent proof result mismatch: %v", err)
			}
		})
	}
}

func TestRPCEnvelopeErrorsAreRedacted(t *testing.T) {
	for _, body := range []string{
		`{"result":1397511495,"error":null,"id":2}`,
		`{"result":1397511495,"error":null,"id":"1"}`,
		`{"result":1397511495,"error":null,"id":null}`,
		`{"result":1397511495,"error":null,"id":1,"\u0069d":1}`,
		`{"result":1397511495,"id":1}`,
		`{"result":null,"error":null,"id":1}`,
		`{"error":null,"id":1}`,
		`{"result":1397511495,"error":{"message":"secret-password test-user"},"id":1}`,
		`{"result":1397511495,"error":null,"id":1,"jsonrpc":null}`,
		`{"result":1397511495,"error":null,"id":1} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
			_, err := client.Resolve(context.Background(), testPolicy())
			if err == nil {
				t.Fatal("accepted invalid RPC envelope")
			}
			for _, secret := range []string{"test-user", "secret-password", client.endpoint, body} {
				if strings.Contains(err.Error(), secret) {
					t.Fatal("RPC error exposed request or response contents")
				}
			}
		})
	}
}

func TestBoundedRPCTransport(t *testing.T) {
	t.Run("oversized response", func(t *testing.T) {
		client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"result":1397511495,"error":null,"id":1}`)
			_, _ = io.CopyN(w, strings.NewReader(strings.Repeat(" ", maxRPCResponseSize)), maxRPCResponseSize)
		}))
		if _, err := client.Resolve(context.Background(), testPolicy()); err == nil {
			t.Fatal("accepted oversized response")
		}
	})
	t.Run("cancelled operation", func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := client.Resolve(ctx, testPolicy())
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
			t.Fatalf("request not cancelled: %v", err)
		}
	})
	t.Run("redirect denied", func(t *testing.T) {
		var hits atomic.Int32
		destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
		defer destination.Close()
		client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
		}))
		if _, err := client.Resolve(context.Background(), testPolicy()); err == nil || hits.Load() != 0 {
			t.Fatal("followed redirect")
		}
	})
	t.Run("no proxy compression or replay", func(t *testing.T) {
		client := newTestClient(t, &mockNode{t: t, fixture: independentFixture(t)})
		transport := client.http.Transport.(*http.Transport)
		if transport.Proxy != nil || !transport.DisableCompression || !transport.DisableKeepAlives || transport.TLSClientConfig.InsecureSkipVerify || transport.TLSClientConfig.MinVersion < 0x0303 {
			t.Fatal("unsafe transport configuration")
		}
		for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
			var calls atomic.Int32
			client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "secret-password", status) }))
			_, err := client.Resolve(context.Background(), testPolicy())
			if err == nil || strings.Contains(err.Error(), "secret-password") || calls.Load() != 1 {
				t.Fatal("error response was exposed, accepted, or retried")
			}
		}
	})
}

func TestTLSCertificateVerification(t *testing.T) {
	node := &mockNode{t: t, fixture: independentFixture(t)}
	server := httptest.NewUnstartedServer(node)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	config := RPCConfig{Version: 1, URL: server.URL, Username: "test-user", Password: "secret-password"}
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Resolve(context.Background(), testPolicy()); err == nil {
		t.Fatal("trusted unknown self-signed certificate")
	}
	config.CACertificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	trusted, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer trusted.Close()
	if _, err := trusted.Resolve(context.Background(), testPolicy()); err != nil {
		t.Fatal(err)
	}
	config.URL = strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	wrongName, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	defer wrongName.Close()
	if _, err := wrongName.Resolve(context.Background(), testPolicy()); err == nil {
		t.Fatal("accepted certificate hostname mismatch")
	}
	config.CACertificate = "invalid PEM"
	if _, err := NewClient(config); err == nil {
		t.Fatal("accepted invalid custom roots")
	}
}
