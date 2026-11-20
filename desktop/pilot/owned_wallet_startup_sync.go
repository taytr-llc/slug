// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import "context"

// ownedStartupSynced is only the readiness predicate between the original
// schema14 Attach and InitializeRecipient. catchUpHashes updates manager height
// inside its DB transaction, then sets ChainSynced after that transaction commits.
// A coherent balance Snapshot during catch-up may therefore correctly refuse a
// changed height. Startup must wait for readiness without requesting balances.
// InitializeRecipient still requires the unchanged coherent Snapshot, followed
// by the original session/epoch checks before recipient publication.
func (r *walletRuntime) ownedStartupSynced(ctx context.Context) (bool, error) {
	if r == nil || ctx == nil {
		return false, ErrPoolRuntime
	}
	if e := ctx.Err(); e != nil {
		return false, e
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return false, e
	}
	if r.closed || r.wallet == nil || r.wallet.Manager == nil || r.wallet.ShuttingDown() ||
		r.allocationGuard2 == nil || r.allocationGuard2.State != "bound" || !r.wallet.Manager.HostedAllocation2() ||
		r.rpc == nil || r.backend == nil || r.backend.Client == nil || r.backend.Disconnected() {
		return false, ErrPoolRuntime
	}
	ready := r.wallet.ChainSynced()
	if r.wallet.ShuttingDown() || r.backend.Disconnected() {
		return false, ErrPoolRuntime
	}
	if e := ctx.Err(); e != nil {
		return false, e
	}
	return ready, nil
}
