// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slug/wire/v2"
)

func hostedPendingRoot(t *testing.T) (string, gymaccount.Session, gymprovider.ResetRequest) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal fixture identity")
	}
	ctx := context.Background()
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(parent, "hosted")
	options, e := pilot.ResolvePoolOptions(pilot.PoolOptions{})
	if e != nil {
		t.Fatal(e)
	}
	lock, e := openHostedRoot(root, options)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	verifier, e := gymidentity.NewVerifier(gymprovider.FixtureVerifierConfig())
	if e != nil {
		t.Fatal(e)
	}
	pc, e := hostedProviderConfig(strings.Repeat("11", 32))
	if e != nil {
		t.Fatal(e)
	}
	ic := hostedIdentityConfig(verifier.ConfigDigest())
	init := HostedInit{Version: 1, Mode: pilot.HostedPoolMode, DescriptorID: options.CandidateDescriptorID, AuthorityID: pc.AuthorityID, CoordinatorID: strings.Repeat("22", 32), HostedVaultID: strings.Repeat("33", 32), ProviderDriverID: hostedDriverID, WrapperID: hostedWrapperID, VerifierConfigHash: verifier.ConfigDigest(), IdentityConfigSHA256: hostedCanonicalHash(ic), ProviderConfigSHA256: hostedCanonicalHash(pc), VaultConfigSHA256: hostedCanonicalHash(hostedVaultConfig()), Wallets: []WalletOwnerPin{{"zero256", strings.Repeat("41", 32)}, {"incrementing256", strings.Repeat("42", 32)}}}
	if e = writeHostedInit(root, init); e != nil {
		t.Fatal(e)
	}
	authority, e := gymaccount.CreateIdentityPinned(filepath.Join(root, "authority"), ic, init.AuthorityID)
	if e != nil {
		t.Fatal(e)
	}
	defer authority.Close()
	stage := func(name string) {
		t.Helper()
		if e := writeHostedSetupReceipt(root, SetupReceipt{1, name, hostedCanonicalHash(init), hostedInitialStageHash(init, name)}); e != nil {
			t.Fatal(e)
		}
	}
	stage("authority")
	coordinator, e := gymprovider.CreatePinned(ctx, filepath.Join(root, "coordinator"), pc, authority, verifier, init.CoordinatorID)
	if e != nil {
		t.Fatal(e)
	}
	defer coordinator.Close()
	stage("coordinator")
	driver, e := openHostedDriver(ctx, filepath.Join(root, "fixture-provider"), pc, authority, coordinator, true)
	if e != nil {
		t.Fatal(e)
	}
	defer driver.Close()
	stage("driver")
	flow, e := coordinator.IssueLogin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	evidence, e := driver.FreshLogin(ctx, flow, "zero256")
	if e != nil {
		t.Fatal(e)
	}
	session, e := coordinator.FinishLogin(ctx, flow.ID, evidence)
	if e != nil {
		t.Fatal(e)
	}
	request, e := coordinator.IssueReset(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = driver.BeginResetEvidence(ctx, request, "zero256"); e != nil {
		t.Fatal(e)
	}
	return root, session, request
}
func TestHostedPendingStartupDoesNotMutateCredentials(t *testing.T) {
	root, _, request := hostedPendingRoot(t)
	c, e := NewHostedFixturePool(context.Background(), pilot.Config{DataDir: root, NodeBinary: "/nonexistent/slugd"}, pilot.PoolOptions{})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	snapshot := c.Snapshot()
	if snapshot.Mode != pilot.HostedPoolMode || snapshot.Account.State != "signed_out" || snapshot.Account.Fixture != "zero256" || snapshot.Account.LastError != "hosted_reset_pending" || snapshot.View != nil || c.runtime != nil {
		t.Fatal("pending startup did not retain reachable control", snapshot.Account)
	}
	pending, e := c.hosted.driver.PendingReset(context.Background(), "zero256")
	if e != nil || pending.Request != request || pending.Phase != "verified" || pending.FromEpoch != 1 {
		t.Fatal("startup changed reset", e)
	}
	if _, e = c.store.GetResetBarrier(context.Background(), gymprovider.FixtureIssuer, "zero256", request.EventID); !errors.Is(e, gymaccount.ErrNotFound) {
		t.Fatal("startup created barrier", e)
	}
	if e = pilot.ValidatePoolSnapshot(snapshot); e != nil {
		t.Fatal(e)
	}
}
func TestHostedResetBarrierPrecedesJoinAndCredentialCAS(t *testing.T) {
	root, session, request := hostedPendingRoot(t)
	ctx := context.Background()
	c, e := NewHostedFixturePool(ctx, pilot.Config{DataDir: root, NodeBinary: "/nonexistent/slugd"}, pilot.PoolOptions{})
	if e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
		_ = c.Close()
	}()
	child := &poolParentChild{a: pilot.ManifestAccountConfig{Account: pilot.AccountConfig{AccountID: session.Account.ID}}, stopGate: func() {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
	}}
	c.mu.Lock()
	c.child = child
	c.session = session
	c.viewID = strings.Repeat("73", 32)
	c.state = "signed_in"
	c.mu.Unlock()
	if e = c.ResetFixtureAccount(); e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("reset did not reach joined child")
	}
	barrier, e := c.store.GetResetBarrier(ctx, gymprovider.FixtureIssuer, "zero256", request.EventID)
	if e != nil || barrier.Status != "pending" {
		t.Fatal("barrier was not first", e)
	}
	if e = c.store.WithSession(ctx, session.Token, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); !errors.Is(e, gymaccount.ErrForbidden) {
		t.Fatal("old token survived barrier", e)
	}
	c.hosted.driver.mu.Lock()
	epoch := c.hosted.driver.state.Epochs[0].CredentialEpoch
	c.hosted.driver.mu.Unlock()
	if epoch != 1 {
		t.Fatal("credential changed before join")
	}
	close(release)
	released = true
	poolParentWait(t, c)
	barrier, e = c.store.GetResetBarrier(ctx, gymprovider.FixtureIssuer, "zero256", request.EventID)
	if e != nil || barrier.Status != "completed" {
		t.Fatal("original reset not completed", e)
	}
	if _, e = c.hosted.driver.PendingReset(ctx, "zero256"); !errors.Is(e, gymprovider.ErrNotFound) {
		t.Fatal(e)
	}
	if c.hosted.driver.state.Epochs[0].CredentialEpoch != 2 {
		t.Fatal("expected one credential mutation")
	}
	// The deliberately absent node makes later setup fail, without changing the
	// already completed event or silently restarting the old child/miner.
	if child.starts.Load() != 0 || c.Snapshot().View != nil {
		t.Fatal("implicit child/mining after setup failure")
	}
}
func TestHostedEpochFenceFailsFastAndOriginalView(t *testing.T) {
	c := &PoolController{hosted: &hostedPoolOwner{}, viewID: strings.Repeat("11", 32), session: gymaccount.Session{Token: "original", Account: gymaccount.Account{ID: strings.Repeat("22", 32), Generation: 1}}}
	fence := c.hostedViewFence(c.session, c.viewID, func() poolChild { return nil })
	ctx := context.Background()
	called := false
	c.mu.Lock()
	e := fence(ctx, func(func() error) error { called = true; return nil })
	c.mu.Unlock()
	if e == nil || called {
		t.Fatal("epoch waited or entered under locked parent")
	}
	if e = fence(ctx, func(check func() error) error { return check() }); e != nil {
		t.Fatal(e)
	}
	c.session.Token = "replacement"
	if e = fence(ctx, func(func() error) error { called = true; return nil }); e == nil || called {
		t.Fatal("substituted action token")
	}
}
func TestHostedFixtureWrapperPurposeIsolation(t *testing.T) {
	w := hostedFixtureWrapper{}
	var key [32]byte
	for i := range key {
		key[i] = byte(i)
	}
	ctx := context.Background()
	a, e := w.Wrap(ctx, "fixture-gym-v1", key, []byte("exact aad"))
	if e != nil {
		t.Fatal(e)
	}
	b, e := w.Wrap(ctx, "fixture-gym-v1", key, []byte("exact aad"))
	if e != nil || string(a) == string(b) {
		t.Fatal("fixture wrapper reused nonce", e)
	}
	got, e := w.Unwrap(ctx, "fixture-gym-v1", a, []byte("exact aad"))
	if e != nil || got != key {
		t.Fatal(e)
	}
	for _, test := range []struct {
		ref string
		aad []byte
	}{{"fixture-recovery-v1", []byte("exact aad")}, {"fixture-gym-v1", []byte("changed aad")}, {"operator-key", []byte("exact aad")}} {
		if _, e = w.Unwrap(ctx, test.ref, a, test.aad); e == nil {
			t.Fatal("purpose/authority substitution")
		}
	}
}
