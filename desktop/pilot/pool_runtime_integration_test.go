//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
)

// This qualification owns fresh Phase-A nodes and the two already published
// fixture wallets. It never generates a block and uses no operator accounts.
func TestPoolRuntimeActualPartialProvisionAndListenerConflict(t *testing.T) {
	binary, artifacts := os.Getenv("SLUG_POOL_NODE"), os.Getenv("SLUG_POOL_DESKTOP_ARTIFACTS")
	if binary == "" || artifacts == "" {
		t.Skip("explicit normal node and owned fixture artifacts required")
	}
	if !filepath.IsAbs(binary) || !filepath.IsAbs(artifacts) {
		t.Fatal("absolute artifact paths required")
	}
	root, e := os.MkdirTemp(artifacts, "runtime-partial-")
	if e != nil {
		t.Fatal(e)
	}
	t.Log("owned public-fixture runtime", root)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	store, e := gymaccount.Create(filepath.Join(root, "accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 128, MaxChallenges: 64, MaxEvents: 256})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := store.Close(); e != nil {
			t.Error(e)
		}
	})
	config := Config{DataDir: filepath.Join(root, "shared"), NodeBinary: binary}
	var r *PoolRuntime
	var blocked net.Listener
	t.Cleanup(func() {
		if blocked != nil {
			_ = blocked.Close()
		}
		if r != nil {
			if e := r.Close(); e != nil {
				t.Error(e)
			}
		}
	})
	open := func() {
		t.Helper()
		var e error
		r, e = NewPoolRuntime(ctx, config, store)
		if e != nil {
			t.Fatal("open runtime", e)
		}
	}
	provision := func(fixture, subject string) PoolRecipientBinding {
		t.Helper()
		login, e := store.Login(ctx, gymaccount.Principal{Issuer: "fixture:local", Subject: subject, AuthenticatedAtMS: time.Now().UnixMilli()})
		if e != nil {
			t.Fatal(e)
		}
		binding, workErr := r.ProvisionFixture(ctx, fixture, login)
		revokeErr := store.Logout(ctx, login.Token)
		if workErr != nil || revokeErr != nil {
			t.Fatal("provision exact fixture", workErr, revokeErr)
		}
		return binding
	}
	heightZero := func() {
		t.Helper()
		var height uint32
		if e := r.nodes.primary.rpc.call(ctx, "getblockcount", &height); e != nil || height != 0 {
			t.Fatal("setup or recovery mined", height, e)
		}
	}
	closeRuntime := func() {
		t.Helper()
		addresses := []string{r.nodes.primary.rpc.url, r.nodes.peer.rpc.url}
		if e := r.Close(); e != nil {
			t.Fatal("close runtime", e)
		}
		r = nil
		for _, address := range addresses {
			connection, e := net.DialTimeout("tcp", address[len("http://"):], time.Second)
			if e == nil {
				connection.Close()
				t.Fatal("owned node listener survived close")
			}
		}
	}
	open()
	source, ports := r.SourceConfig(), [2]uint16{r.marker.PoolPort, r.marker.AdmissionPort}
	first := provision("zero256", "alice")
	if r.Ready() {
		t.Fatal("one fixture made runtime ready")
	}
	if e = r.StartServices(ctx); e == nil {
		t.Fatal("one fixture started services")
	}
	if _, e = os.Lstat(filepath.Join(config.DataDir, "settlement")); !os.IsNotExist(e) {
		t.Fatal("partial setup created settlement table", e)
	}
	heightZero()
	closeRuntime()
	open()
	if r.SourceConfig() != source || [2]uint16{r.marker.PoolPort, r.marker.AdmissionPort} != ports || r.recipients["zero256"] != first || len(r.verified) != 0 {
		t.Fatal("partial reopen replaced identity or trusted marker as live wallet")
	}
	if e = r.StartServices(ctx); e == nil {
		t.Fatal("reopen started without both actual wallet verifications")
	}
	if got := provision("zero256", "alice"); got != first {
		t.Fatal("partial reopen changed original native84 recipient")
	}
	second := provision("incrementing256", "bob")
	if second.Recipient.Address == first.Recipient.Address {
		t.Fatal("two fixture wallets share destination")
	}
	var listenErr error
	blocked, listenErr = net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(int(ports[0])))
	if listenErr != nil {
		t.Fatal(listenErr)
	}
	if e = r.StartServices(ctx); e == nil {
		t.Fatal("pinned listener conflict silently changed port")
	}
	if r.Ready() || [2]uint16{r.marker.PoolPort, r.marker.AdmissionPort} != ports {
		t.Fatal("failed service start changed runtime readiness or routing")
	}
	if connection, e := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(ports[1])), time.Second); e == nil {
		connection.Close()
		t.Fatal("sibling admission listener survived failed startup")
	}
	heightZero()
	closeRuntime()
	if e = blocked.Close(); e != nil {
		t.Fatal(e)
	}
	blocked = nil
	open()
	if r.SourceConfig() != source || [2]uint16{r.marker.PoolPort, r.marker.AdmissionPort} != ports {
		t.Fatal("listener failure replaced source or ports")
	}
	if got := provision("zero256", "alice"); got != first {
		t.Fatal("recovery changed first recipient")
	}
	if got := provision("incrementing256", "bob"); got != second {
		t.Fatal("recovery changed second recipient")
	}
	if e = r.StartServices(ctx); e != nil {
		t.Fatal("exact listener recovery", e)
	}
	if !r.Ready() {
		t.Fatal("both verified wallets did not enable ready runtime")
	}
	heightZero()
	closeRuntime()
	// Borrowed accounts survive every runtime teardown.
	if _, e = store.Login(ctx, gymaccount.Principal{Issuer: "fixture:local", Subject: "alice", AuthenticatedAtMS: time.Now().UnixMilli()}); e != nil {
		t.Fatal("runtime closed borrowed account authority", e)
	}
}
