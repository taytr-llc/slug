// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/prospectworker"
	"math"
	"time"
)

func (c *ManifestController) Snapshot() ManifestSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.snapshot
	if s.View != nil {
		p := *s.View
		s.View = &p
	}
	s.Projection.Items = append([]ManifestProspectSummary{}, s.Projection.Items...)
	return s
}
func (c *ManifestController) refresh(worker *prospectworker.ManifestWorker, node *childNode, base uint64) error {
	c.resources.Lock()
	defer c.resources.Unlock()
	if c.store == nil {
		return nil
	}
	info, e := c.store.Info()
	if e != nil || info.Pin != c.manifest.AuthorityPin || info.Records > 1000 {
		return ErrManifestProjectionCorrupt
	}
	var status prospectworker.Status
	if worker != nil {
		status = worker.Status()
		if status.Attempts > manifestMaxSafe-base || status.Pending > 1000 || math.IsNaN(status.HashesPerSecond) || math.IsInf(status.HashesPerSecond, 0) {
			return errors.New("invalid worker status")
		}
	}
	var height uint64
	validHeight := false
	if node != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		validHeight = node.rpc.call(ctx, "getblockcount", &height) == nil && height <= manifestMaxSafe
		cancel()
	}
	var v walletView
	validWallet := false
	if c.wallet != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		v, e = c.wallet.Snapshot(ctx)
		cancel()
		validWallet = e == nil && v.Height >= 0 && v.Spendable >= 0 && v.Immature >= 0 && uint64(v.Spendable) <= manifestMaxSafe && uint64(v.Immature) <= manifestMaxSafe
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot.AcceptedCount = max(c.snapshot.AcceptedCount, info.Records)
	c.recomputeProjectionLocked()
	if worker != nil {
		c.snapshot.Attempts = base + status.Attempts
		c.snapshot.HashesPerSecond = status.HashesPerSecond
		c.snapshot.Pending = status.Pending
		c.snapshot.LastError = status.LastError
		if c.snapshot.LastError == "cycle_unavailable" {
			c.snapshot.LastError = "worker_failed"
		}
		c.snapshot.RecoveryError = status.RecoveryError
	}
	if validHeight {
		c.snapshot.Height = height
	}
	if validWallet {
		c.snapshot.WalletHeight = uint64(v.Height)
		c.snapshot.SpendableAtoms = v.Spendable
		c.snapshot.ImmatureAtoms = v.Immature
		c.snapshot.WalletState = "syncing"
		if v.Synced {
			c.snapshot.WalletState = "ready"
		}
	} else if c.wallet != nil {
		c.snapshot.WalletState = "syncing"
	}
	p := c.viewPin()
	c.snapshot.View = &p
	return nil
}
