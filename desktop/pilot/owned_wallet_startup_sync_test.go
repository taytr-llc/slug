// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slugwallet/chain"
	"github.com/taytr-llc/slugwallet/walletdb"
)

// A real RPCClient passes the inherited network/genesis handshake and stays
// connected. This is a transport fixture, not a node, rescan or consensus proof.
func ownedStartupBackend(t *testing.T) *chain.RPCClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		for {
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if ws.ReadJSON(&req) != nil {
				return
			}
			var result any
			switch req.Method {
			case "getcurrentnet":
				result = uint32(chaincfg.MainNetParams.Net)
			case "getblockhash":
				result = chaincfg.MainNetParams.GenesisHash.String()
			case "getbestblock":
				result = map[string]any{"hash": chaincfg.MainNetParams.GenesisHash.String(), "height": 0}
			default:
				t.Errorf("unexpected readiness backend call %q", req.Method)
				return
			}
			if ws.WriteJSON(map[string]any{"id": req.ID, "result": result, "error": nil}) != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	c, e := chain.NewRPCClient(&chaincfg.MainNetParams, strings.TrimPrefix(server.URL, "http://"), "public-fixture", "public-fixture", nil, true, 0)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Stop(); c.WaitForShutdown() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e = c.Start(ctx); e != nil || c.Disconnected() {
		t.Fatal("connected fixture backend", e)
	}
	return c
}

func ownedStartupWallet(t *testing.T) (*ownedWalletTestFixture, *OwnedWalletSession) {
	t.Helper()
	h := newOwnedWalletTestFixture(t)
	setup := h.open(t)
	s, e := setup.OpenFixture(context.Background(), "zero256", h.sessions["zero256"], h.epoch, h.current["zero256"])
	if e != nil {
		t.Fatal(e)
	}
	return h, s
}

func TestOwnedWalletStartupSyncDoesNotSampleCatchupBalances(t *testing.T) {
	_, s := ownedStartupWallet(t)
	r := s.runtime
	r.backend = ownedStartupBackend(t)
	r.wallet.SetChainSynced(false)

	// Match the real manager cache/DB ordering: SetSyncedTo is visible while
	// the catch-up write transaction is still open, and ChainSynced becomes
	// true only after the whole transaction commits. Synthetic stamps below
	// exercise synchronization only; they are not asserted canonical blocks.
	paused, proceed, committed, joined := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var release sync.Once
	txResult := make(chan error, 1)
	go func() {
		e := walletdb.Update(r.wallet.Database(), func(tx walletdb.ReadWriteTx) error {
			ns := tx.ReadWriteBucket([]byte("waddrmgr"))
			stamp := r.wallet.Manager.SyncedTo()
			stamp.Height++
			stamp.Hash[0] ^= 1
			if e := r.wallet.Manager.SetSyncedTo(ns, &stamp); e != nil {
				return e
			}
			close(paused)
			<-proceed
			stamp.Height++
			stamp.Hash[0] ^= 3
			return r.wallet.Manager.SetSyncedTo(ns, &stamp)
		})
		if e == nil {
			r.wallet.SetChainSynced(true)
		}
		txResult <- e
		close(committed)
		close(joined)
	}()
	defer func() { release.Do(func() { close(proceed) }); <-joined }()
	select {
	case <-paused:
	case <-joined:
		t.Fatal("catch-up transaction", <-txResult)
	case <-time.After(5 * time.Second):
		t.Fatal("catch-up transaction did not enter")
	}

	balanceRequested := make(chan struct{})
	var first atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var v struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if json.NewDecoder(req.Body).Decode(&v) != nil {
			http.Error(w, "request", 400)
			return
		}
		if v.Method != "getbalance" && v.Method != "getunconfirmedbalance" {
			http.Error(w, "unexpected", 400)
			return
		}
		if v.Method == "getbalance" && first.CompareAndSwap(false, true) {
			close(balanceRequested)
			select {
			case <-committed:
			case <-req.Context().Done():
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"id": v.ID, "result": 0, "error": nil})
	}))
	defer server.Close()
	r.rpc = newWalletHTTPClient(server.URL, "public-fixture", "public-fixture")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	ready, e := r.ownedStartupSynced(ctx)
	cancel()
	if e != nil || ready || len(r.rpc.callCounts()) != 0 {
		t.Fatal("readiness requested balances or treated catch-up as failure", ready, e)
	}

	// Deterministically reproduce the old readiness path's legitimate refusal:
	// it samples intermediate height, then obtains balances after catch-up.
	oldSnapshot := make(chan error, 1)
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	snapshotDone := make(chan struct{})
	go func() {
		defer close(snapshotDone)
		_, e := r.Snapshot(ctx)
		oldSnapshot <- e
	}()
	defer func() {
		// Fatal/error paths must unblock the retained DB writer before joining
		// the reader; cancellation also interrupts a stalled HTTP response.
		release.Do(func() { close(proceed) })
		cancel()
		<-snapshotDone
	}()
	select {
	case <-balanceRequested:
	case <-ctx.Done():
		t.Fatal("old snapshot did not reach balance read")
	}
	release.Do(func() { close(proceed) })
	if e = <-txResult; e != nil {
		t.Fatal("catch-up commit", e)
	}
	if e = <-oldSnapshot; e == nil || e.Error() != "wallet balance snapshot changed" {
		t.Fatal("old full snapshot did not expose the catch-up race", e)
	}
	calls := r.rpc.callCounts()
	ready, e = r.ownedStartupSynced(context.Background())
	if e != nil || !ready || r.rpc.callCounts()["getbalance"] != calls["getbalance"] || r.rpc.callCounts()["getunconfirmedbalance"] != calls["getunconfirmedbalance"] {
		t.Fatal("committed readiness changed balance-call count", ready, e)
	}
	final, e := r.Snapshot(context.Background())
	if e != nil || !final.Synced || final.Height != 2 || final.Spendable != 0 || final.Immature != 0 || final.Unconfirmed != 0 {
		t.Fatal("final coherent snapshot no longer required/usable", final, e)
	}
}

func TestOwnedWalletStartupSyncLifecycleRefusals(t *testing.T) {
	_, s := ownedStartupWallet(t)
	r := s.runtime
	r.backend = ownedStartupBackend(t)
	// No transport call is made by readiness. A real RPC owner exists, but this
	// endpoint deliberately cannot serve a balance request.
	r.rpc = newWalletHTTPClient("http://127.0.0.1:1", "public-fixture", "public-fixture")
	r.wallet.SetChainSynced(true)
	if ready, e := r.ownedStartupSynced(context.Background()); e != nil || !ready {
		t.Fatal("initial readiness", ready, e)
	}
	for _, name := range []string{"canceled", "closed", "missing-backend", "missing-rpc", "missing-guard", "disconnected"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch name {
			case "canceled":
				cancel()
			case "closed":
				r.closed = true
				defer func() { r.closed = false }()
			case "missing-backend":
				prior := r.backend
				r.backend = nil
				defer func() { r.backend = prior }()
			case "missing-rpc":
				prior := r.rpc
				r.rpc = nil
				defer func() { r.rpc = prior }()
			case "missing-guard":
				prior := r.allocationGuard2
				r.allocationGuard2 = nil
				defer func() { r.allocationGuard2 = prior }()
			case "disconnected":
				r.backend.Stop()
				r.backend.WaitForShutdown()
				if !r.backend.Disconnected() {
					t.Fatal("connected backend did not disconnect")
				}
			}
			if ready, e := r.ownedStartupSynced(ctx); e == nil || ready || name == "canceled" && !errors.Is(e, context.Canceled) {
				t.Fatal("lifecycle refusal", ready, e)
			}
		})
	}
	if len(r.rpc.callCounts()) != 0 {
		t.Fatal("lifecycle readiness requested balances")
	}
}

func TestOwnedWalletStartupSyncOriginalSessionFenceBeforeAttach(t *testing.T) {
	h, s := ownedStartupWallet(t)
	life, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &PoolRuntime{accounts: h.accounts, life: life, cancel: cancel, gate: make(chan struct{}, 1),
		source: h.config.Source, owned: &ownedRuntimeResources{config: OwnedSourceConfig{Ownership: h.config.Ownership}},
		ownedGym: &ownedGymRuntimeResources{}}
	for i, f := range h.config.Fixtures {
		r.ownedGym.pins[i] = f.Pin
	}
	// No node owner is provided. Any weakened original View fence would reach
	// Attach and fail this test instead of returning the retained denial.
	h.revoked.Store(true)
	if _, e := r.PrepareOwnedWallet(context.Background(), s); !errors.Is(e, gymaccount.ErrUnauthorized) {
		t.Fatal("original epoch refusal lost", e)
	}
	h.revoked.Store(false)
	if e := h.accounts.Logout(context.Background(), s.login.Token); e != nil {
		t.Fatal(e)
	}
	if _, e := r.PrepareOwnedWallet(context.Background(), s); !errors.Is(e, gymaccount.ErrUnauthorized) {
		t.Fatal("logged-out original session reached attachment", e)
	}
	if s.runtime.backend != nil || s.runtime.rpc != nil || s.ready.Load() {
		t.Fatal("refused startup created attachment/readiness")
	}
}
