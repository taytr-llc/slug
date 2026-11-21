// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"

	"github.com/taytr-llc/slug/poolsettle"
)

// Borrow the exact original Run lease if one was already selected. A closing
// lease refuses; this call never retries by substituting a replacement client.
// A stopped read owns a temporary lease and must join it before returning.
func (o *remotePoolCredentials) dailyReadLease(ctx context.Context) (*remotePoolLease, bool, error) {
	if o == nil || o.config.Version != 2 || ctx == nil || ctx.Err() != nil {
		return nil, false, errRemotePoolCredential
	}
	o.mu.Lock()
	if o.closing || o.failed || o.ctx.Err() != nil {
		o.mu.Unlock()
		return nil, false, errRemotePoolCredential
	}
	l := o.slots[0].lease
	o.mu.Unlock()
	if l != nil {
		// Registration precedes construction. Wait for that exact construction,
		// not for another owner. Close also waits for ready before release.
		select {
		case <-l.ready:
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-o.ctx.Done():
			return nil, false, errRemotePoolCredential
		}
		return l, false, nil
	}
	l, e := o.begin(ctx, 0)
	return l, e == nil, dailyReadCredentialError(e)
}

func (l *remotePoolLease) dailyPayoutPage(ctx context.Context, q poolsettle.DailyPayoutRequest) (poolsettle.DailyPayoutPage, error) {
	return remotePoolLeaseCall(ctx, l, false, func(work context.Context) (poolsettle.DailyPayoutPage, error) {
		if l.audience != 0 || l.owner.config.Version != 2 || l.pool == nil {
			return poolsettle.DailyPayoutPage{}, errRemotePoolCredential
		}
		return l.pool.PayoutsPage(work, q)
	})
}

// Cached disclosure checks the same original protected files and selected lease
// without issuing an HTTP request or obtaining any Mining/signing authority.
func (l *remotePoolLease) dailyReadCheck(ctx context.Context) error {
	_, e := remotePoolLeaseCall(ctx, l, false, func(context.Context) (struct{}, error) {
		if l.audience != 0 || l.owner.config.Version != 2 || l.pool == nil {
			return struct{}{}, errRemotePoolCredential
		}
		return struct{}{}, nil
	})
	return dailyReadCredentialError(e)
}

// The selected native read API exposes its public busy sentinel. Keep exact
// provenance: a wrapped or joined credential error is not merely gate busy.
// Other credential users and the original worker lease retain their errors.
func dailyReadCredentialError(e error) error {
	if e == errRemotePoolCredentialBusy {
		return ErrRemotePoolBusy
	}
	return e
}
