// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
)

type poolBackupTestChild struct {
	*poolParentChild
	config                                pilot.StableAccountConfig
	export                                func(context.Context) error
	exports, verifications, cancellations atomic.Int32
}

func (f *poolBackupTestChild) ExportPoolRecovery(ctx context.Context, _ recoveryfile.Pin) error {
	f.exports.Add(1)
	if f.export != nil {
		return f.export(ctx)
	}
	return f.config.Lease(ctx, gymaccount.RecoveryExport, func(_ gymaccount.Account, check func() error) error { return check() })
}
func (f *poolBackupTestChild) VerifyPoolBackup(ctx context.Context, _ recoveryfile.Pin) error {
	f.verifications.Add(1)
	return f.config.Lease(ctx, gymaccount.View, func(_ gymaccount.Account, check func() error) error { return check() })
}
func (f *poolBackupTestChild) CancelPoolBackup() { f.cancellations.Add(1) }
func poolBackupParent(t *testing.T) (*PoolController, *[]*poolBackupTestChild) {
	t.Helper()
	c, _ := poolParent(t)
	children := []*poolBackupTestChild{}
	c.newChild = func(_ context.Context, _ *pilot.PoolRuntime, a pilot.StableAccountConfig) (poolChild, error) {
		f := &poolBackupTestChild{poolParentChild: &poolParentChild{a: a.Manifest, source: c.sourceID}, config: a}
		children = append(children, f)
		return f, nil
	}
	return c, &children
}
func poolBackupPin(t *testing.T, mode string) recoveryfile.Pin {
	t.Helper()
	path := filepath.Join(t.TempDir(), "public-fixture.json")
	if mode == recoveryfile.ReadMode {
		if err := os.WriteFile(path, []byte("public fixture selection only\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := recoveryfile.Capture(path, nil, mode)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s.Pin()
}
func TestPoolBackupParentOriginalViewAndHold(t *testing.T) {
	c, children := poolBackupParent(t)
	a := poolParentLogin(t, c, "zero256")
	first := (*children)[0]
	create, read := poolBackupPin(t, recoveryfile.CreateMode), poolBackupPin(t, recoveryfile.ReadMode)
	if !a.Account.CanExport {
		t.Fatal("fresh enrolled login cannot export")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.ExportPoolRecovery(ctx, a.View.ViewID, create); err != nil {
		t.Fatal("nested export lease", err)
	}
	if err := c.VerifyPoolBackup(ctx, a.View.ViewID, read); err != nil {
		t.Fatal("nested view lease", err)
	}
	if err := c.ExportPoolRecovery(ctx, a.View.ViewID, read); err == nil || first.exports.Load() != 1 {
		t.Fatal("read selection admitted as export")
	}
	if err := c.VerifyPoolBackup(ctx, a.View.ViewID, create); err == nil || first.verifications.Load() != 1 {
		t.Fatal("create selection admitted as read")
	}
	if err := c.ResetFixtureAccount(); err != nil {
		t.Fatal(err)
	}
	poolParentWait(t, c)
	b := c.Snapshot()
	second := (*children)[1]
	if b.View == nil || !b.Account.Held || b.Account.CanExport {
		t.Fatal("reset did not retain export hold")
	}
	if err := c.ExportPoolRecovery(ctx, b.View.ViewID, create); !errors.Is(err, gymaccount.ErrHeld) || second.exports.Load() != 0 {
		t.Fatal("held export reached wallet", err)
	}
	if err := c.VerifyPoolBackup(ctx, b.View.ViewID, read); err != nil || second.verifications.Load() != 1 {
		t.Fatal("view-only verification blocked by hold", err)
	}
	if err := c.VerifyPoolBackup(ctx, a.View.ViewID, read); err == nil || second.verifications.Load() != 1 {
		t.Fatal("old dialog adopted replacement view")
	}
	if err := c.CancelPoolBackup(ctx, a.View.ViewID); err == nil || second.cancellations.Load() != 0 {
		t.Fatal("old dialog cancelled replacement backup")
	}
	if err := c.CancelPoolBackup(ctx, b.View.ViewID); err != nil || second.cancellations.Load() != 1 {
		t.Fatal("current cancel failed", err)
	}
}
func TestPoolBackupParentResetDuringBlockedExport(t *testing.T) {
	c, children := poolBackupParent(t)
	a := poolParentLogin(t, c, "zero256")
	first := (*children)[0]
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	first.export = func(ctx context.Context) error {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		return first.config.Lease(ctx, gymaccount.RecoveryExport, func(_ gymaccount.Account, check func() error) error { return check() })
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	pin := poolBackupPin(t, recoveryfile.CreateMode)
	go func() { done <- c.ExportPoolRecovery(ctx, a.View.ViewID, pin) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("export never reached wallet work")
	}
	if err := c.ResetFixtureAccount(); err != nil {
		t.Fatal(err)
	}
	poolParentWait(t, c)
	b := c.Snapshot()
	if b.View == nil || b.View.ViewID == a.View.ViewID || !b.Account.Held {
		t.Fatal("blocked export prevented reset")
	}
	// The late original callback must fail even though a replacement login is
	// already active. The parent must never substitute that credential.
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stale export succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("stale export failed to join")
	}
	if (*children)[1].exports.Load() != 0 {
		t.Fatal("replacement child inherited old export")
	}
}
func TestPoolBackupParentRejectsManagedPathBeforeChild(t *testing.T) {
	c, children := poolBackupParent(t)
	a := poolParentLogin(t, c, "zero256")
	// Native Capture already rejects managed roots. Also reject a forged path
	// sent directly to the parent before dispatching any wallet work.
	pin := poolBackupPin(t, recoveryfile.CreateMode)
	pin.Path = filepath.Join(c.config.DataDir, "recovery.json")
	if err := c.ExportPoolRecovery(context.Background(), a.View.ViewID, pin); err == nil || (*children)[0].exports.Load() != 0 {
		t.Fatal("managed destination reached wallet")
	}
}
