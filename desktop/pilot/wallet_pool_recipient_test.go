// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/chain"
	"github.com/taytr-llc/slugwallet/rpc/legacyrpc"
)

func poolRecipientOracle(t *testing.T, name string) walletPoolRecipient {
	t.Helper()
	raw, err := os.ReadFile("../../../slugwallet/recovery/testdata/recovery_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "d5141f4a8ee4378d77d483b09f6ecd809f8b328f82e79a69ff83164e73bc799a" {
		t.Fatal("independent recovery corpus changed")
	}
	var corpus struct {
		Cases []struct {
			Name      string `json:"name"`
			Addresses []struct {
				Path    string `json:"path"`
				Address string `json:"address"`
				Script  string `json:"script_pub_key"`
			} `json:"addresses"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("decode independent recovery corpus")
	}
	for _, c := range corpus.Cases {
		for _, a := range c.Addresses {
			if c.Name == name && a.Path == "m/84'/0'/0'/0/0" {
				return walletPoolRecipient{Payout: walletPayout{Address: a.Address, Purpose: 84}, Script: a.Script}
			}
		}
	}
	t.Fatal("missing independent pool recipient")
	return walletPoolRecipient{}
}

// This sink allows actual wallet address commits through actual legacy HTTP.
// It makes no chain synchronization or receipt-of-funds claim.
type poolRecipientNotifications struct{ chain.Interface }

func (*poolRecipientNotifications) NotifyReceived([]address.Address) error { return nil }
func (*poolRecipientNotifications) Notifications() <-chan interface{}      { return nil }
func (*poolRecipientNotifications) Stop()                                  {}
func (*poolRecipientNotifications) WaitForShutdown()                       {}

func attachPoolRecipientRPC(t *testing.T, r *walletRuntime) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r.wallet.SynchronizeRPC(&poolRecipientNotifications{})
	r.server = legacyrpc.NewServer(&legacyrpc.Options{Username: "pool-recipient-fixture", Password: fixtureWalletPublicPass, MaxPOSTClients: 2, MaxWebsocketClients: 0, AllowedMethods: walletRPCMethods()}, r.loader, []net.Listener{listener})
	r.server.RegisterWallet(r.wallet)
	r.rpc = newWalletHTTPClient("http://"+listener.Addr().String(), "pool-recipient-fixture", fixtureWalletPublicPass)
}

func TestPoolRecipientFrozenIdentities(t *testing.T) {
	for _, name := range []string{"zero256", "incrementing256"} {
		f, err := gymaccount.FixtureIdentity(name)
		if err != nil {
			t.Fatal(err)
		}
		id := walletIdentity{RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID}
		got, err := expectedPoolRecipient(id)
		if err != nil || got != poolRecipientOracle(t, name) {
			t.Fatal("compiled value differs from independent native84 fixture", err)
		}
		id.RootPublicKey += "wrong"
		if _, err := expectedPoolRecipient(id); !errors.Is(err, ErrPoolRecipient) {
			t.Fatal("wrong root accepted")
		}
	}
}

func TestPoolRecipientActualWalletRPCPreservesLegacyAndRestores(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal fixture wallet only")
	}
	for _, name := range []string{"zero256", "incrementing256"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "wallet")
			r, err := openAccountFixtureWallet(path, name, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if r != nil {
					_ = r.Close()
				}
			})
			attachPoolRecipientRPC(t, r)
			want := poolRecipientOracle(t, name)
			if _, err := r.EnsurePoolRecipient(ctx, &want); !errors.Is(err, ErrPoolRecipient) || len(r.rpc.callCounts()) != 0 {
				t.Fatal("ready recipient check allocated a missing address", err)
			}
			legacy, err := r.EnsurePayout(ctx, nil)
			if err != nil || legacy.Purpose != 44 || legacy.Index != 0 {
				t.Fatal("legacy payout", err)
			}
			got, err := r.EnsurePoolRecipient(ctx, nil)
			if err != nil || got != want {
				t.Fatalf("actual native84 RPC recipient: %+v %v", got, err)
			}
			calls := r.rpc.callCounts()["getnewaddress"]
			if calls != 2 {
				t.Fatal("unexpected initial allocation count", calls)
			}
			if again, err := r.EnsurePoolRecipient(ctx, &got); err != nil || again != got || r.rpc.callCounts()["getnewaddress"] != calls {
				t.Fatal("repeat allocated another address", err)
			}
			if old, err := r.EnsurePayout(ctx, &legacy); err != nil || old != legacy {
				t.Fatal("native recipient altered legacy payout", err)
			}
			for _, mutate := range []func(*walletPoolRecipient){
				func(v *walletPoolRecipient) { v.Script = "0014" },
				func(v *walletPoolRecipient) { v.Payout.Index = 1 },
				func(v *walletPoolRecipient) { v.Payout.Purpose = 44 },
				func(v *walletPoolRecipient) { v.Payout.Address = legacy.Address },
			} {
				bad := got
				mutate(&bad)
				before := r.rpc.callCounts()
				if _, err := r.EnsurePoolRecipient(ctx, &bad); err == nil {
					t.Fatal("changed persisted recipient accepted")
				}
				if after := r.rpc.callCounts(); len(after) != len(before) || after["getnewaddress"] != before["getnewaddress"] || after["validateaddress"] != before["validateaddress"] {
					t.Fatal("invalid recipient reached RPC")
				}
			}
			bundle, err := r.ExportRecovery(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range bundle.Manifest.Accounts {
				if (a.Purpose == 44 || a.Purpose == 84) && (a.NextExternal != 1 || a.NextInternal != 0) {
					t.Fatal("unexpected public recovery cursor", a.Purpose, a.NextExternal, a.NextInternal)
				}
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			r, err = openAccountFixtureWallet(path, name, false)
			if err != nil {
				t.Fatal(err)
			}
			attachPoolRecipientRPC(t, r)
			if got, err := r.EnsurePoolRecipient(ctx, &want); err != nil || got != want || r.rpc.callCounts()["getnewaddress"] != 0 {
				t.Fatal("reopen changed/allocated recipient", err)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			r, err = restoreAccountFixtureWallet(filepath.Join(t.TempDir(), "restored"), name, bundle)
			if err != nil {
				t.Fatal(err)
			}
			attachPoolRecipientRPC(t, r)
			if got, err := r.EnsurePoolRecipient(ctx, &want); err != nil || got != want || r.rpc.callCounts()["getnewaddress"] != 0 {
				t.Fatal("restored wallet lost recipient", err)
			}
		})
	}
}

func TestPoolRecipientLostAllocationAcknowledgmentRequiresReopen(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal fixture wallet only")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wallet")
	r, err := openAccountFixtureWallet(path, "zero256", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if r != nil {
			_ = r.Close()
		}
	})
	attachPoolRecipientRPC(t, r)
	origin, _ := url.Parse(r.rpc.url)
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	var dropped atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(req.Body, 65537))
		if err != nil || len(raw) > 65536 {
			http.Error(w, "fixture request unavailable", 400)
			return
		}
		var method struct {
			Method string `json:"method"`
		}
		if json.Unmarshal(raw, &method) != nil {
			http.Error(w, "fixture request unavailable", 400)
			return
		}
		forward := req.Clone(req.Context())
		forward.URL.Scheme, forward.URL.Host = origin.Scheme, origin.Host
		forward.Host = origin.Host
		forward.RequestURI = ""
		forward.Body = io.NopCloser(bytes.NewReader(raw))
		response, err := transport.RoundTrip(forward)
		if err != nil {
			http.Error(w, "fixture RPC unavailable", 503)
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
		if err != nil {
			http.Error(w, "fixture RPC response unavailable", 503)
			return
		}
		if method.Method == "getnewaddress" && response.StatusCode == 200 && dropped.CompareAndSwap(false, true) {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		for key, values := range response.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(body)
	}))
	t.Cleanup(func() { proxy.Close(); transport.CloseIdleConnections() })
	r.rpc = newWalletHTTPClient(proxy.URL, "pool-recipient-fixture", fixtureWalletPublicPass)
	if got, err := r.EnsurePoolRecipient(ctx, nil); !errors.Is(err, ErrPoolRecipientUncertain) || got != (walletPoolRecipient{}) || !dropped.Load() {
		t.Fatal("lost allocation acknowledgement did not remain uncertain", err)
	}
	if next, err := r.poolRecipientCursor(); err != nil || next != 1 {
		t.Fatal("real RPC did not commit before lost response", next, err)
	}
	if _, err := r.EnsurePoolRecipient(ctx, nil); !errors.Is(err, ErrPoolRecipientUncertain) || r.rpc.callCounts()["getnewaddress"] != 1 {
		t.Fatal("same runtime retried uncertain allocation", err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = openAccountFixtureWallet(path, "zero256", false)
	if err != nil {
		t.Fatal(err)
	}
	attachPoolRecipientRPC(t, r)
	want := poolRecipientOracle(t, "zero256")
	if got, err := r.EnsurePoolRecipient(ctx, nil); err != nil || got != want || r.rpc.callCounts()["getnewaddress"] != 0 {
		t.Fatal("reopen failed to recover committed original address", err)
	}
	if next, err := r.poolRecipientCursor(); err != nil || next != 1 {
		t.Fatal("recovery advanced native84 cursor", next, err)
	}
}
