// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"testing"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

func TestHostedPoolSignerVaultFailureHasNoLocalFallback(t *testing.T) {
	h := newPoolDesktopHarness(t)
	c := &PoolChild{account: StableAccountConfig{Manifest: h.account, Hosted: &HostedPoolSession{}}, authority: h.authority, hostedCurrent: &recoveryvault.CurrentSource{}, resources: poolChildResources{Source: h.config.Source, Hosted: &hostedPoolResources{vault: &recoveryvault.HostedVault{}}, WorkerConfig: prospectworker.PoolConfig{PublicKey: h.config.PublicKey}}}
	// The concrete local wallet is absent: any fallback would panic instead of
	// returning the exact unavailable-vault error.
	signer, e := c.hostedSigner(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	p := h.config.Source.Policy
	_, e = signer(context.Background(), prospect.Context{Network: p.Network, Genesis: p.Genesis, DNAVersion: p.DNAVersion, ProspectTarget: p.ProspectTarget}, prospect.Proof{Version: 2})
	if !errors.Is(e, recoveryvault.ErrClosed) {
		t.Fatal("vault failure replaced", e)
	}
}
func TestHostedPoolRecoveryPermitSurvivesChildGeneration(t *testing.T) {
	owner := &hostedPoolResources{recoveryPermit: make(chan struct{}, 1)}
	release, e := owner.acquireRecovery()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = owner.acquireRecovery(); !errors.Is(e, ErrPoolRuntimeBusy) {
		t.Fatal("second generation bypassed permit", e)
	}
	release()
	release()
	next, e := owner.acquireRecovery()
	if e != nil {
		t.Fatal(e)
	}
	next()
}
func TestHostedPoolFailedWalletCloseRetainsOwnership(t *testing.T) {
	c, _, _, released := poolChildLifecycleFixture(t)
	life, cancel := context.WithCancel(context.Background())
	runtime := &PoolRuntime{life: life, cancel: cancel, gate: make(chan struct{}, 1), borrows: make(map[*poolRuntimeBorrow]bool)}
	runtime.hosted = &hostedPoolResources{}
	c.resources.Hosted = runtime.hosted
	c.account.Hosted = &HostedPoolSession{runtime: runtime}
	failure := errors.New("joined loader cleanup failed")
	c.wallet = &walletRuntime{closed: true, closeErr: failure}
	if e := c.Close(); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	if released.Load() != 0 || runtime.hosted.failedWallet != c.wallet || !errors.Is(runtime.hosted.failure, failure) {
		t.Fatal("failed wallet owner was released")
	}
	if e := runtime.Close(); !errors.Is(e, failure) {
		t.Fatal("runtime hid child cleanup failure", e)
	}
}
