// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymaccount"
)

// The requested view was captured before the native file dialog opened. The
// child owns that original credential and re-enters it only for publication;
// wallet and filesystem preparation never run inside this parent's lease.
func (c *PoolController) poolBackupChild(ctx context.Context, viewID string, pin recoveryfile.Pin, mode string, capability gymaccount.Capability) (poolChild, error) {
	if ctx == nil || recoveryfile.ValidatePin(pin, mode) != nil {
		return nil, gymaccount.ErrForbidden
	}
	if err := c.backupPath(pin.Path); err != nil {
		return nil, err
	}
	child, session, view, err := c.active()
	if err != nil || viewID != view {
		return nil, gymaccount.ErrUnauthorized
	}
	if err = c.withCapability(ctx, session, viewID, child, capability, func(_ gymaccount.Account, check func() error) error { return check() }); err != nil {
		return nil, err
	}
	return child, nil
}

func (c *PoolController) ExportPoolRecovery(ctx context.Context, viewID string, pin recoveryfile.Pin) error {
	child, err := c.poolBackupChild(ctx, viewID, pin, recoveryfile.CreateMode, gymaccount.RecoveryExport)
	if err != nil {
		return err
	}
	return child.ExportPoolRecovery(ctx, pin)
}

func (c *PoolController) VerifyPoolBackup(ctx context.Context, viewID string, pin recoveryfile.Pin) error {
	child, err := c.poolBackupChild(ctx, viewID, pin, recoveryfile.ReadMode, gymaccount.View)
	if err != nil {
		return err
	}
	return child.VerifyPoolBackup(ctx, pin)
}

func (c *PoolController) CancelPoolBackup(ctx context.Context, viewID string) error {
	if ctx == nil {
		return gymaccount.ErrForbidden
	}
	child, session, view, err := c.active()
	if err != nil || viewID != view {
		return gymaccount.ErrUnauthorized
	}
	if err = c.withView(ctx, session, viewID, child, func(_ gymaccount.Account, check func() error) error { return check() }); err != nil {
		return err
	}
	child.CancelPoolBackup()
	return nil
}
