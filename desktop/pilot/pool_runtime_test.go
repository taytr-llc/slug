// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/wallet"
)

func poolRuntimeTest(t *testing.T) *PoolRuntime {
	t.Helper()
	if wire.TestPoW {
		t.Skip("runtime uses the normal Phase-A profile")
	}
	life, cancel := context.WithCancel(context.Background())
	r := &PoolRuntime{config: Config{DataDir: t.TempDir()}, gate: make(chan struct{}, 1), life: life, cancel: cancel, recipients: make(map[string]PoolRecipientBinding), verified: make(map[string]bool), borrows: make(map[*poolRuntimeBorrow]bool)}
	r.marker = poolRuntimeMarker{"slug-shared-pool-runtime", 1, strings.Repeat("c", 64), policy().Network, genesis, chaincfg.PhaseAActivationHeight, recovery.ProfileID, manifestDescriptorID, true, 19061, 19062}
	r.source = r.marker.source(registry.ManifestAuthorityPin{Version: 5, Network: policy().Network, Genesis: genesis, AuthorityID: strings.Repeat("a", 64), DescriptorID: manifestDescriptorID})
	t.Cleanup(func() {
		if e := r.Close(); e != nil {
			t.Error(e)
		}
	})
	return r
}

func TestPoolRuntimeImmutableRecords(t *testing.T) {
	r := poolRuntimeTest(t)
	if !r.marker.valid(r.marker.AccountAuthorityID) {
		t.Fatal("valid marker refused")
	}
	path := filepath.Join(r.config.DataDir, poolRuntimeMarkerName)
	if e := writePoolRuntimeRecord(path, r.marker); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = writePoolRuntimeRecord(path, r.marker); e != nil {
		t.Fatal("matching publication must re-sync", e)
	}
	changed := r.marker
	changed.PoolPort++
	if e = writePoolRuntimeRecord(path, changed); e == nil {
		t.Fatal("port pin overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("immutable bytes changed")
	}
	for _, mutate := range []func(*poolRuntimeMarker){func(m *poolRuntimeMarker) { m.PrimaryTXIndex = false }, func(m *poolRuntimeMarker) { m.AdmissionPort = m.PoolPort }, func(m *poolRuntimeMarker) { m.ActivationHeight++ }, func(m *poolRuntimeMarker) { m.RecoveryProfile = "wrong" }, func(m *poolRuntimeMarker) { m.Version++ }, func(m *poolRuntimeMarker) { m.AccountAuthorityID = strings.Repeat("b", 64) }} {
		bad := r.marker
		mutate(&bad)
		if bad.valid(r.marker.AccountAuthorityID) {
			t.Fatal("changed runtime identity accepted")
		}
	}
	if e = os.WriteFile(path, append(before, ' '), 0600); e != nil {
		t.Fatal(e)
	}
	if e = readPoolRuntimeRecord(path, r.marker); e == nil {
		t.Fatal("noncanonical marker accepted")
	}
	link := filepath.Join(r.config.DataDir, "linked")
	if e = os.Symlink(path, link); e != nil {
		t.Fatal(e)
	}
	if e = writePoolRuntimeRecord(link, r.marker); e == nil {
		t.Fatal("symlink publication accepted")
	}
}

func TestPoolRuntimeOrphanIssuerRefused(t *testing.T) {
	r := poolRuntimeTest(t)
	path := filepath.Join(r.config.DataDir, "issuer")
	issuer, e := registry.CreateManifestAuthority(path, r.marker.Network, r.marker.Genesis, r.marker.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	if e = issuer.Close(); e != nil {
		t.Fatal(e)
	}
	if e = r.openPoolIssuer(false); !errors.Is(e, ErrPoolRuntime) {
		t.Fatal("orphan random issuer adopted", e)
	}
	if r.issuer != nil {
		t.Fatal("orphan issuer opened before trust check")
	}
	if _, e = os.Lstat(filepath.Join(r.config.DataDir, poolRuntimeSourceName)); !os.IsNotExist(e) {
		t.Fatal("inspection published source pin", e)
	}
}

func TestPoolRuntimePinnedIssuerReopen(t *testing.T) {
	r := poolRuntimeTest(t)
	r.source = poolshare.Config{}
	if e := r.openPoolIssuer(false); e != nil {
		t.Fatal(e)
	}
	pinned := r.source
	if pinned.MaxWorks != 256 || pinned.MaxSubmissions != 4096 || pinned.MaxBytes != 256<<20 {
		t.Fatal("candidate finite limits changed")
	}
	if e := r.issuer.Close(); e != nil {
		t.Fatal(e)
	}
	r.issuer = nil
	if e := r.openPoolIssuer(true); e != nil {
		t.Fatal("pinned reopen", e)
	}
	if r.source != pinned {
		t.Fatal("source changed on reopen")
	}
	if e := r.issuer.Close(); e != nil {
		t.Fatal(e)
	}
	r.issuer = nil
	if e := os.Remove(filepath.Join(r.config.DataDir, poolRuntimeSourceName)); e != nil {
		t.Fatal(e)
	}
	if e := r.openPoolIssuer(true); !errors.Is(e, ErrPoolRuntime) {
		t.Fatal("established source loss silently adopted", e)
	}
}

func TestPoolRuntimePinnedIssuerCannotBeReplaced(t *testing.T) {
	r := poolRuntimeTest(t)
	if e := writePoolRuntimeRecord(filepath.Join(r.config.DataDir, poolRuntimeSourceName), r.source); e != nil {
		t.Fatal(e)
	}
	if e := r.openPoolIssuer(false); !errors.Is(e, ErrPoolRuntime) {
		t.Fatal("missing issuer recreated", e)
	}
	if _, e := os.Lstat(filepath.Join(r.config.DataDir, "issuer")); !os.IsNotExist(e) {
		t.Fatal("missing issuer touched", e)
	}
}

func TestPoolRuntimeRootNamespace(t *testing.T) {
	r := poolRuntimeTest(t)
	if e := writePoolRuntimeRecord(filepath.Join(r.config.DataDir, poolRuntimeMarkerName), r.marker); e != nil {
		t.Fatal(e)
	}
	if e := poolRuntimeRootFiles(r.config.DataDir); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(r.config.DataDir, "SLUG-PILOT.json")
	if e := os.WriteFile(path, []byte("{}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := poolRuntimeRootFiles(r.config.DataDir); e == nil {
		t.Fatal("legacy namespace mixed with pool root")
	}
	if e := os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(t.TempDir(), filepath.Join(r.config.DataDir, "shares")); e != nil {
		t.Fatal(e)
	}
	if e := poolRuntimeRootFiles(r.config.DataDir); e == nil {
		t.Fatal("source symlink accepted")
	}
}

func poolRuntimeTestBinding(t *testing.T, r *PoolRuntime, fixture string) PoolRecipientBinding {
	t.Helper()
	id, e := gymaccount.FixtureIdentity(fixture)
	if e != nil {
		t.Fatal(e)
	}
	account := strings.Repeat("b", 64)
	if fixture == "incrementing256" {
		account = strings.Repeat("d", 64)
	}
	b, _, e := poolRecipientExpected(PoolRecipientConfig{Version: 1, AccountAuthorityID: r.marker.AccountAuthorityID, AccountID: account, Fixture: fixture, Identity: id, Source: r.source})
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func poolRuntimeTestPublishGym(t *testing.T, r *PoolRuntime, b PoolRecipientBinding, ready bool) string {
	t.Helper()
	root := filepath.Join(r.config.DataDir, "gyms", b.Config.AccountID)
	if e := os.MkdirAll(root, 0700); e != nil {
		t.Fatal(e)
	}
	marker := poolRuntimeGym{1, b.Config.AccountAuthorityID, b.Config.AccountID, b.Config.Fixture}
	if e := writePoolRuntimeRecord(filepath.Join(root, poolRuntimeGymName), marker); e != nil {
		t.Fatal(e)
	}
	if ready {
		if e := writePoolRuntimeRecord(filepath.Join(root, poolRuntimeGymReadyName), b); e != nil {
			t.Fatal(e)
		}
		if e := writePoolRuntimeRecord(filepath.Join(root, poolRecipientIntentName), recipientMarker(b, "initializing")); e != nil {
			t.Fatal(e)
		}
		if e := writePoolRuntimeRecord(filepath.Join(root, poolRecipientReadyName), recipientMarker(b, "ready")); e != nil {
			t.Fatal(e)
		}
		if e := os.Mkdir(filepath.Join(root, "wallet"), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(root, "wallet", wallet.WalletDBName), []byte("public test placeholder; load does not open this file"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	return root
}

func TestPoolRuntimePartialRecipientsRequireWalletVerification(t *testing.T) {
	r := poolRuntimeTest(t)
	a, b := poolRuntimeTestBinding(t, r, "zero256"), poolRuntimeTestBinding(t, r, "incrementing256")
	poolRuntimeTestPublishGym(t, r, a, true)
	poolRuntimeTestPublishGym(t, r, b, false)
	if e := r.loadPoolRecipients(); e != nil {
		t.Fatal(e)
	}
	if len(r.recipients) != 1 || r.recipients["zero256"] != a {
		t.Fatal("partial setup did not retain completed binding")
	}
	if len(r.verified) != 0 {
		t.Fatal("public marker became wallet verification")
	}
	if e := r.StartServices(context.Background()); !errors.Is(e, ErrPoolRuntime) {
		t.Fatal("partial setup started services", e)
	}
	if r.ready {
		t.Fatal("partial setup marked ready")
	}
	if _, e := os.Lstat(filepath.Join(r.config.DataDir, "settlement")); !os.IsNotExist(e) {
		t.Fatal("partial setup touched payout table", e)
	}
}

func TestPoolRuntimeRecipientWalletAndSourceLossRefused(t *testing.T) {
	for _, kind := range []string{"wallet", "source"} {
		t.Run(kind, func(t *testing.T) {
			r := poolRuntimeTest(t)
			b := poolRuntimeTestBinding(t, r, "zero256")
			root := poolRuntimeTestPublishGym(t, r, b, true)
			if kind == "wallet" {
				if e := os.Remove(filepath.Join(root, "wallet", wallet.WalletDBName)); e != nil {
					t.Fatal(e)
				}
			} else {
				r.source.MaxWorks--
			}
			if e := r.loadPoolRecipients(); !errors.Is(e, ErrPoolRuntime) {
				t.Fatal("missing or rebound recipient accepted", e)
			}
		})
	}
}

func TestPoolRuntimeReadyRequiresBothExactGyms(t *testing.T) {
	r := poolRuntimeTest(t)
	a, b := poolRuntimeTestBinding(t, r, "zero256"), poolRuntimeTestBinding(t, r, "incrementing256")
	r.recipients[a.Config.Fixture] = a
	if _, e := r.readyRecord(); e == nil {
		t.Fatal("one gym finalized payout table")
	}
	r.recipients[b.Config.Fixture] = b
	ready, e := r.readyRecord()
	if e != nil {
		t.Fatal(e)
	}
	if len(ready.Recipients) != 2 || ready.Recipients[0].Config.Identity.GymID >= ready.Recipients[1].Config.Identity.GymID {
		t.Fatal("destinations not canonical")
	}
	copy := r.SourceConfig()
	copy.MaxWorks--
	if r.SourceConfig() == copy {
		t.Fatal("source getter leaked mutable state")
	}
}

func TestPoolRuntimeCloseCancelsAndJoinsBorrower(t *testing.T) {
	r := poolRuntimeTest(t)
	borrowed := &poolRuntimeBorrow{account: "fixture", done: make(chan struct{})}
	var closes atomic.Int32
	borrowed.close = func() error {
		if r.life.Err() == nil {
			t.Error("borrower close preceded cancellation")
		}
		closes.Add(1)
		r.mu.Lock()
		delete(r.borrows, borrowed)
		close(borrowed.done)
		r.mu.Unlock()
		return nil
	}
	r.borrows[borrowed] = true
	work, end, e := r.begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = r.begin(context.Background()); !errors.Is(e, ErrPoolRuntimeBusy) {
		t.Fatal("parallel provision accepted", e)
	}
	closed := make(chan error, 1)
	go func() { closed <- r.Close() }()
	select {
	case <-work.Done():
	case <-time.After(time.Second):
		t.Fatal("close did not cancel bounded work")
	}
	select {
	case <-closed:
		t.Fatal("close did not join active provision")
	default:
	}
	end()
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("close stuck after all owners released")
	}
	if closes.Load() != 1 {
		t.Fatal("actual child owner not closed once")
	}
	if e = r.Close(); e != nil || closes.Load() != 1 {
		t.Fatal("close not idempotent", e)
	}
	if _, _, e = r.begin(context.Background()); !errors.Is(e, ErrPoolRuntime) {
		t.Fatal("closed runtime accepted work", e)
	}
}

func TestPoolRuntimeRejectsManagedAncestorAndSymlink(t *testing.T) {
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(parent, "shared")
	if e = poolRuntimePath(root); e != nil {
		t.Fatal(e)
	}
	for _, marker := range []string{"SLUG-MANIFEST-PILOT.json", "SLUG-MANIFEST-ACCOUNTS.json", poolRuntimeMarkerName} {
		path := filepath.Join(parent, marker)
		if e = os.WriteFile(path, []byte("{}\n"), 0600); e != nil {
			t.Fatal(e)
		}
		if e = poolRuntimePath(root); e == nil {
			t.Fatal("managed ancestor accepted", marker)
		}
		if e = os.Remove(path); e != nil {
			t.Fatal(e)
		}
	}
	if e = os.WriteFile(filepath.Join(parent, "SLUG-POOL-ACCOUNTS.json"), []byte("{}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = poolRuntimePath(root); e != nil {
		t.Fatal("explicit parent/shared route refused", e)
	}
	if e = poolRuntimePath(filepath.Join(parent, "unrelated")); e == nil {
		t.Fatal("unrelated child of account root accepted")
	}
	link := filepath.Join(parent, "linked")
	if e = os.Symlink(t.TempDir(), link); e != nil {
		t.Fatal(e)
	}
	if e = poolRuntimePath(filepath.Join(link, "shared")); e == nil {
		t.Fatal("symlink parent accepted")
	}
}
