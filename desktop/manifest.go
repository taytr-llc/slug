// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) Mode() string {
	if g.manifestHelper != nil {
		return pilot.ManifestMode
	}
	return pilot.AccountMode
}
func (g *Gym) control(command string) error {
	if g.manifestHelper != nil {
		_, err := g.manifestHelper.Request(context.Background(), command, 0)
		return err
	}
	if g.helper == nil {
		return bridge.ErrUnavailable
	}
	_, err := g.helper.Request(context.Background(), command, 0)
	return err
}
func (g *Gym) requestPath(command, path string) error {
	if g.manifestHelper != nil {
		_, err := g.manifestHelper.RequestPath(context.Background(), command, path)
		return err
	}
	if g.helper == nil {
		return bridge.ErrUnavailable
	}
	_, err := g.helper.RequestPath(context.Background(), command, path)
	return err
}
func (g *Gym) closeHelper() error {
	if g.manifestHelper != nil {
		return g.manifestHelper.Close()
	}
	if g.helper != nil {
		return g.helper.Close()
	}
	return nil
}
func (g *Gym) recoveryAccount() (*pilot.AccountSnapshot, error) {
	if g.manifestHelper != nil {
		s, err := g.ManifestSnapshot()
		if err != nil {
			return nil, err
		}
		return &s.Account, nil
	}
	s, err := g.Snapshot()
	return s.Account, err
}
func (g *Gym) ManifestSnapshot() (pilot.ManifestSnapshot, error) {
	if g.manifestHelper == nil {
		return pilot.ManifestSnapshot{}, bridge.ErrUnavailable
	}
	r, err := g.manifestHelper.Request(context.Background(), "snapshot", 0)
	if err != nil {
		return pilot.ManifestSnapshot{}, err
	}
	return *r.Snapshot, nil
}
func (g *Gym) RefreshManifestProspects() error {
	if g.manifestHelper == nil {
		return bridge.ErrUnavailable
	}
	_, err := g.manifestHelper.Request(context.Background(), "refresh_manifest_prospects", 0)
	return err
}
func (g *Gym) ManifestPortrait(sequence uint64) (pilot.ManifestPortrait, error) {
	if g.manifestHelper == nil {
		return pilot.ManifestPortrait{}, bridge.ErrUnavailable
	}
	r, err := g.manifestHelper.Request(context.Background(), "manifest_portrait", sequence)
	if err != nil {
		return pilot.ManifestPortrait{}, err
	}
	return *r.Portrait, nil
}
func (g *Gym) ExportManifestReceipt(sequence uint64) (string, error) {
	if g.manifestHelper == nil {
		return "", bridge.ErrUnavailable
	}
	if !g.exportMu.TryLock() {
		return "", bridge.ErrBusy
	}
	defer g.exportMu.Unlock()
	r, err := g.manifestHelper.Request(context.Background(), "manifest_receipt", sequence)
	if err != nil {
		return "", err
	}
	receipt := r.ManifestReceipt
	// Release the bounded selected receipt even when the native dialog is
	// cancelled. Account invalidation independently clears this private pin.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = g.manifestHelper.ReleaseManifest(ctx, receipt.View.ViewID, receipt.Record.Verified.ProspectID)
	}()
	path, err := g.app.Dialog.SaveFile().AttachToWindow(g.window).
		SetMessage("Save this local candidate's public receipt.").
		SetFilename("slug-candidate-receipt-"+strconv.FormatUint(sequence, 10)+".json").
		AddFilter("JSON receipt", "*.json").AllowsOtherFileTypes(false).CanCreateDirectories(true).PromptForSingleSelection()
	if err != nil {
		return "", errors.New("The save dialog is unavailable.")
	}
	if path == "" {
		return "", nil
	}
	path, err = selectedExportPath(path, g.protectedRoots)
	if err != nil {
		return "", errors.New("Choose a receipt destination outside application data.")
	}
	// The helper owns the exclusive file publication and rechecks the captured
	// view after the dialog. JavaScript never supplies a path or another account.
	_, err = g.manifestHelper.ExportManifest(context.Background(), sequence, receipt.View.ViewID, receipt.Record.Verified.ProspectID, path)
	if err != nil {
		return "", errors.New("The candidate receipt could not be saved.")
	}
	return "Receipt saved.", nil
}
