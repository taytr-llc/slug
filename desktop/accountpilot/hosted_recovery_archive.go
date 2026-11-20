// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
	"os"
	"path/filepath"
	"reflect"
)

// Only absence of the exact installed payload is recoverable. Classification
// never creates a directory or opens the unaffected wallet through a Loader.
func (c *PoolController) classifyHostedWallets() (bool, error) {
	h := c.hosted
	if h == nil || h.allocation == nil || !h.hadReady || len(h.ready.Fixtures) != 2 {
		return false, pilot.ErrManifestInitialization
	}
	missing := make(map[string]bool, 2)
	any := false
	for i, f := range hostedFixtures {
		b := h.ready.Fixtures[i].CurrentBinding
		if h.ready.Fixtures[i].Fixture != f {
			return false, pilot.ErrManifestInitialization
		}
		account, e := c.store.GetIdentityAccount(context.Background(), gymprovider.FixtureIssuer, f)
		if e != nil || account.ID != b.VaultBinding.AccountID || account.PrincipalID != b.VaultBinding.PrincipalID || account.Enrollment == nil {
			return false, pilot.ErrManifestInitialization
		}
		root := filepath.Join(c.config.DataDir, "shared", "gyms", account.ID)
		if found, e := hostedComponentExists(root); e != nil || !found {
			return false, pilot.ErrManifestInitialization
		}
		absent, e := hostedWalletAbsent(root)
		if e != nil {
			return false, e
		}
		if absent {
			missing[f] = true
			any = true
		}

	}
	h.allocation.missing = missing
	return any, nil
}
func (c *PoolController) openHostedRecoveryArchive(ctx context.Context) (resultErr error) {
	h := c.hosted
	if h == nil || h.allocation == nil || !h.hadReady || !h.allocation.setup.Ready() {
		return pilot.ErrManifestInitialization
	}
	bindings := make([]recoveryvault.CurrentBinding, 0, 2)
	for i, f := range hostedFixtures {
		scope, e := h.allocation.setup.ExpectedScope(f)
		if e != nil || scope.CurrentBinding != h.ready.Fixtures[i].CurrentBinding {
			return pilot.ErrManifestInitialization
		}
		bindings = append(bindings, scope.CurrentBinding)
	}
	if e := h.allocation.setup.OpenEstablished(ctx); e != nil {
		return e
	}
	if h.current == nil {
		var e error
		h.current, e = recoveryvault.OpenCurrentStore(ctx, filepath.Join(c.config.DataDir, "current-recovery"), h.vault, bindings)
		if e != nil {
			return e
		}
	}
	if h.allocation.archive == nil {
		var e error
		h.allocation.archive, e = pilot.OpenPoolRecoveryArchive(ctx, pilot.Config{DataDir: filepath.Join(c.config.DataDir, "shared"), NodeBinary: c.config.NodeBinary}, c.store, h.options, h.ready.SourceBindingID, bindings)
		if e != nil {
			return e
		}
	}
	c.sourceID = h.ready.SourceBindingID
	for _, stage := range hostedStages {
		hash := hostedReadyStageHash(h.init, h.ready, stage)
		if stage == "authority" || stage == "coordinator" || stage == "driver" || stage == "vault" {
			hash = hostedInitialStageHash(h.init, stage)
		}
		if e := c.hostedStage(stage, hash); e != nil {
			return errors.Join(pilot.ErrManifestInitialization, e)
		}
	}
	return nil
}

func hostedWalletAbsent(gymroot string) (bool, error) {
	path := filepath.Join(gymroot, "wallet")
	found, e := hostedComponentExists(path)
	if e != nil {
		return false, e
	}
	if !found {
		return true, nil
	}
	path = filepath.Join(path, wallet.WalletDBName)
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return true, nil
	}
	if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false, pilot.ErrManifestInitialization
	}
	file, e := os.Open(path)
	if e != nil {
		return false, pilot.ErrManifestInitialization
	}
	return false, file.Close()
}

// Allocation READY commits every prerequisite. Audit the exact existing stores
// before any normal constructor can take an initialization/resume branch.
func (c *PoolController) auditHostedAllocationReady(ctx context.Context) (resultErr error) {
	h := c.hosted
	bindings := make([]recoveryvault.CurrentBinding, 0, 2)
	for _, f := range hostedFixtures {
		scope, e := h.allocation.setup.ExpectedScope(f)
		if e != nil {
			return e
		}
		bindings = append(bindings, scope.CurrentBinding)
	}
	expected := HostedReady{Version: 1, InitSHA256: hostedCanonicalHash(h.init), AuthorityID: h.init.AuthorityID, CoordinatorID: h.init.CoordinatorID, HostedVaultID: h.init.HostedVaultID, SourceBindingID: bindings[0].SourceBindingID, Fixtures: []HostedFixtureReady{{"zero256", bindings[0]}, {"incrementing256", bindings[1]}}}
	if !hostedReadyMatches(h.init, expected) || len(h.prefix) != len(hostedStages) {
		return pilot.ErrManifestInitialization
	}
	for i, stage := range hostedStages {
		if h.prefix[i] != (SetupReceipt{1, stage, hostedCanonicalHash(h.init), hostedReadyStageHash(h.init, expected, stage)}) {
			return pilot.ErrManifestInitialization
		}
	}
	if h.hadReady && !reflect.DeepEqual(h.ready, expected) {
		return pilot.ErrManifestInitialization
	}
	if e := h.allocation.setup.OpenEstablished(ctx); e != nil {
		return e
	}
	if h.current == nil {
		var e error
		h.current, e = recoveryvault.OpenCurrentStore(ctx, filepath.Join(c.config.DataDir, "current-recovery"), h.vault, bindings)
		if e != nil {
			return e
		}
	}
	archive, e := pilot.OpenPoolRecoverySetupAudit(ctx, pilot.Config{DataDir: filepath.Join(c.config.DataDir, "shared"), NodeBinary: c.config.NodeBinary}, c.store, h.options, expected.SourceBindingID, bindings)
	if e != nil {
		if archive != nil {
			h.allocation.archive = archive
		}
		return e
	}
	if e = archive.Close(); e != nil {
		h.allocation.archive = archive
		return e
	}
	if !h.hadReady {
		if e = writeHostedReadyMode(c.config.DataDir, expected, true); e != nil {
			return e
		}
		h.ready, h.hadReady = expected, true
	}
	return nil
}
