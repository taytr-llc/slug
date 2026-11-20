// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
)

type poolRecoveryRequest func(context.Context, pilot.PoolRequest) (pilot.PoolResponse, error)

// The selected account is captured before showing a native dialog. The retained
// selection pins its directory and, for verification, its exact file until the
// helper has acquired its own matching handles. Private bytes never cross IPC.
func poolRecoveryAction(ctx context.Context, request poolRecoveryRequest, choose func() (string, error), protected []string, export bool) (result string, resultErr error) {
	response, err := request(ctx, pilot.PoolRequest{Command: "snapshot"})
	if err != nil || response.Snapshot == nil || pilot.ValidatePoolSnapshot(*response.Snapshot) != nil {
		return "", bridge.ErrUnavailable
	}
	s := *response.Snapshot
	if s.Account.State != "signed_in" || s.View == nil || (s.State != "stopped" && s.State != "failed") || s.BackupState == "running" {
		return "", errors.New("Stop mining before saving or verifying a wallet backup.")
	}
	if export && !s.Account.CanExport {
		if s.Account.Held {
			return "", errors.New("Recovery export is unavailable during the 24-hour security hold.")
		}
		return "", errors.New("Sign out and sign in again before exporting recovery.")
	}
	viewID := s.View.ViewID
	path, err := choose()
	if err != nil {
		return "", errors.New("The file dialog is unavailable.")
	}
	if path == "" {
		return "", nil
	}
	mode, command := recoveryfile.ReadMode, "verify_backup"
	if export {
		mode, command = recoveryfile.CreateMode, "export_recovery"
	}
	selection, err := recoveryfile.Capture(path, protected, mode)
	if err != nil {
		return "", errors.New("Choose a recovery file outside application data.")
	}
	defer func() {
		if err := selection.Close(); err != nil {
			result = ""
			if export {
				resultErr = bridge.ErrRecoveryOutcomeUnknown
			} else {
				resultErr = errors.New("The backup selection could not be closed safely.")
			}
		}
	}()
	pin := selection.Pin()
	if _, err = request(ctx, pilot.PoolRequest{Command: command, ViewID: viewID, Selection: &pin}); err != nil {
		if export {
			return "", bridge.ErrRecoveryOutcomeUnknown
		}
		return "", errors.New("Backup verification could not start. Choose this signed-in wallet's recovery file again.")
	}
	if export {
		return "Test wallet recovery file saved.", nil
	}
	return "Backup verification started.", nil
}

func (g *Gym) poolRecovery(export bool) (string, error) {
	if g.poolHelper == nil || g.poolHelper.StableBridge == nil {
		return "", bridge.ErrUnavailable
	}
	if !g.exportMu.TryLock() {
		return "", bridge.ErrBusy
	}
	defer g.exportMu.Unlock()
	choose := func() (string, error) {
		if export {
			return g.app.Dialog.SaveFile().AttachToWindow(g.window).
				SetMessage("Save the recovery file for this public test wallet.").
				SetFilename("slug-test-wallet-recovery.json").
				AddFilter("SLUG recovery file", "*.json").AllowsOtherFileTypes(false).
				CanCreateDirectories(true).PromptForSingleSelection()
		}
		return g.app.Dialog.OpenFile().AttachToWindow(g.window).
			SetMessage("Choose this test wallet's recovery file to verify a fresh restore.").
			AddFilter("SLUG recovery file", "*.json").
			CanChooseFiles(true).CanChooseDirectories(false).ResolvesAliases(false).
			PromptForSingleSelection()
	}
	if g.poolHelper.Mode() == pilot.HostedRecoveryPoolMode {
		original := g.poolHelper
		return hostedRecoveryAction(context.Background(), original, func() bool { return g.poolHelper == original }, choose, g.protectedRoots, export)
	}
	return poolRecoveryAction(context.Background(), g.poolHelper.PoolRequest, choose, g.protectedRoots, export)
}

func (g *Gym) CancelPoolBackup() error {
	b := g.poolHelper
	if b == nil {
		return bridge.ErrUnavailable
	}
	if b.Mode() == pilot.HostedRecoveryPoolMode {
		s, e := g.HostedRecoverySnapshot()
		if e != nil || g.poolHelper != b {
			return bridge.ErrUnavailable
		}
		owner, e := bridge.HostedContextForSnapshot(s)
		if e != nil || !b.HostedContextMatches(owner) {
			return bridge.ErrUnavailable
		}
		_, e = b.HostedRecoveryRequest(context.Background(), pilot.HostedRecoveryRequest{Command: "hosted_recovery_cancel_pool_backup", OwnerKind: owner.Kind, ViewID: owner.ViewID})
		if g.poolHelper != b || !b.HostedContextMatches(owner) {
			return bridge.ErrUnavailable
		}
		return e
	}
	s, err := g.PoolSnapshot()
	if err != nil || s.View == nil || s.Account.State != "signed_in" {
		return bridge.ErrUnavailable
	}
	_, err = g.poolHelper.PoolRequest(context.Background(), pilot.PoolRequest{Command: "cancel_pool_backup", ViewID: s.View.ViewID})
	return err
}
