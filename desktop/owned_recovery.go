// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"io"
)

// The descriptor and original native context survive the entire dialog and
// handoff. Neither recovery bytes nor a selected path enter the webview.
func (g *Gym) ownedRecovery(view string, export bool) (message string, resultErr error) {
	if !g.exportMu.TryLock() {
		return "", bridge.ErrBusy
	}
	defer g.exportMu.Unlock()
	if g.owned.recoveryCleanup != nil {
		return "", g.owned.recoveryCleanup.err
	}
	b, o, e := g.ownedContext(view)
	if e != nil || g.app == nil || g.window == nil {
		return "", bridge.ErrUnavailable
	}
	s, e := g.OwnedSnapshot()
	if e != nil || s.View == nil || s.View.ViewID != view || !b.ContextMatches(o) {
		return "", bridge.ErrUnavailable
	}
	if s.Child == nil || (s.Child.State != "stopped" && s.Child.State != "failed") || s.Child.Backup.State == "running" {
		return "", errors.New("Stop mining before saving or verifying a wallet backup.")
	}
	if export && !s.Account.CanExport {
		return "", errors.New("Recovery export is unavailable during the security hold or after this session expires.")
	}
	var path string
	if export {
		path, e = g.app.Dialog.SaveFile().AttachToWindow(g.window).SetMessage("Save the recovery file for this public test wallet and gym.").SetFilename("slug-test-wallet-recovery.json").AddFilter("SLUG recovery file", "*.json").AllowsOtherFileTypes(false).CanCreateDirectories(true).PromptForSingleSelection()
	} else {
		path, e = g.app.Dialog.OpenFile().AttachToWindow(g.window).SetMessage("Choose this wallet's recovery file for a fresh restore check.").AddFilter("SLUG recovery file", "*.json").CanChooseFiles(true).CanChooseDirectories(false).ResolvesAliases(false).PromptForSingleSelection()
	}
	if e != nil {
		return "", errors.New("The file dialog is unavailable.")
	}
	if path == "" {
		return "", nil
	}
	if g.ownedHelper != b || !b.ContextMatches(o) {
		return "", &bridge.OwnedFailure{Code: "stale_view"}
	}
	mode, command := recoveryfile.ReadMode, "verify_backup"
	if export {
		mode, command = recoveryfile.CreateMode, "export_recovery"
	}
	selected, e := recoveryfile.Capture(path, g.protectedRoots, mode)
	if e != nil {
		if retained := g.retainOwnedRecoveryCaptureFailure(e); retained != nil {
			return "", retained
		}
		return "", errors.New("Choose a recovery file outside application data.")
	}
	defer func() {
		if e := g.closeOwnedRecoverySelection(selected); e != nil {
			message = ""
			if export {
				resultErr = bridge.ErrRecoveryOutcomeUnknown
			} else {
				resultErr = errors.New("The recovery selection could not be closed safely.")
			}
		}
	}()
	pin := selected.Pin()
	_, e = b.Request(context.Background(), o, pilot.OwnedGymRequest{Command: command, Selection: &pin})
	if e != nil {
		return "", e
	}
	if g.ownedHelper != b || !b.ContextMatches(o) {
		if export {
			return "", bridge.ErrRecoveryOutcomeUnknown
		}
		return "", &bridge.OwnedFailure{Code: "stale_view"}
	}
	if export {
		return "Test wallet and gym recovery file saved.", nil
	}
	return "Backup verification started.", nil
}
func (g *Gym) OwnedExportRecovery(view string) (string, error) { return g.ownedRecovery(view, true) }
func (g *Gym) OwnedVerifyBackup(view string) (string, error)   { return g.ownedRecovery(view, false) }
func (g *Gym) OwnedCancelBackup(view string) error {
	_, e := g.ownedRequest(view, pilot.OwnedGymRequest{Command: "cancel_backup"})
	return e
}

// exportMu protects the one retained failed-close owner and blocks a new
// dialog. A sticky error remains visible during native shutdown.
type nativeOwnedRecoveryCleanup struct {
	owner io.Closer
	err   error
}

// Capture may fail before returning a Selection. Its opaque cleanup error is
// then the descriptor owner; keep the full cause and block replacement dialogs.
// The caller holds exportMu, just as on the successful Selection path.
func (g *Gym) retainOwnedRecoveryCaptureFailure(e error) error {
	var owner *recoveryfile.CleanupError
	if !errors.As(e, &owner) {
		return nil
	}
	g.owned.recoveryCleanup = &nativeOwnedRecoveryCleanup{owner: owner, err: errors.Join(bridge.ErrRecoveryOutcomeUnknown, e)}
	return g.owned.recoveryCleanup.err
}

// The caller holds exportMu throughout capture, dispatch and cleanup.
func (g *Gym) closeOwnedRecoverySelection(owner io.Closer) error {
	if e := owner.Close(); e != nil {
		g.owned.recoveryCleanup = &nativeOwnedRecoveryCleanup{owner: owner, err: errors.Join(bridge.ErrRecoveryOutcomeUnknown, e)}
		return g.owned.recoveryCleanup.err
	}
	return nil
}

func (g *Gym) joinedOwnedRecovery() error {
	g.exportMu.Lock()
	defer g.exportMu.Unlock()
	if g.owned.recoveryCleanup != nil {
		return g.owned.recoveryCleanup.err
	}
	return nil
}
