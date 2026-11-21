// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
)

// Metadata is an immutable selected public binding, not a liveness assertion.
func (l *remotePoolPoolLease) ProtocolVersion() uint32 {
	if l.lease.owner.recovery != nil {
		return 3
	}
	return l.lease.owner.config.Version
}
func (l *remotePoolPoolLease) AuthorityOrigin() string        { return l.lease.owner.config.Origin }
func (l *remotePoolPoolLease) SourceConfig() poolshare.Config { return l.lease.owner.config.Source }
func (l *remotePoolPoolLease) PublicKey() string {
	return l.lease.owner.config.ExpectedLocal.Recipient.Config.Identity.GymPublicKey
}
func (l *remotePoolPoolLease) TransportTrustID() string {
	if l.lease.owner.recovery != nil {
		c, _ := remotePoolRecoverableWorkerConfig(*l.lease.owner.recovery)
		return c.TransportTrustID
	}
	return l.lease.owner.worker.TransportTrustID
}
func (l *remotePoolPoolLease) Issue(ctx context.Context) (poolshare.MiningWork, error) {
	return remotePoolLeaseCall(ctx, l.lease, true, func(c context.Context) (poolshare.MiningWork, error) { return l.lease.pool.Issue(c) })
}
func (l *remotePoolPoolLease) Work(ctx context.Context, id string) (poolshare.MiningWork, error) {
	return remotePoolLeaseCall(ctx, l.lease, false, func(c context.Context) (poolshare.MiningWork, error) { return l.lease.pool.Work(c, id) })
}
func (l *remotePoolPoolLease) Submit(ctx context.Context, w poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	return remotePoolLeaseCall(ctx, l.lease, true, func(c context.Context) (poolshare.Submission, error) { return l.lease.pool.Submit(c, w, nonce) })
}
func (l *remotePoolPoolLease) Lookup(ctx context.Context, w poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	return remotePoolLeaseCall(ctx, l.lease, false, func(c context.Context) (poolshare.Submission, error) { return l.lease.pool.Lookup(c, w, nonce) })
}
func (l *remotePoolPoolLease) Balance(ctx context.Context) (poolservice.BalanceResponse, error) {
	return remotePoolLeaseCall(ctx, l.lease, false, func(c context.Context) (poolservice.BalanceResponse, error) { return l.lease.pool.Balance(c) })
}
func (l *remotePoolPoolLease) Payouts(ctx context.Context) (poolservice.PayoutHistoryResponse, error) {
	return remotePoolLeaseCall(ctx, l.lease, false, func(c context.Context) (poolservice.PayoutHistoryResponse, error) { return l.lease.pool.Payouts(c) })
}

func (l *remotePoolAdmissionLease) ProtocolVersion() uint32 { return 3 }
func (l *remotePoolAdmissionLease) AuthorityOrigin() string { return l.lease.owner.config.Origin }
func (l *remotePoolAdmissionLease) AuthorityPin() registry.ManifestAuthorityPin {
	return l.lease.owner.config.Source.AuthorityPin
}
func (l *remotePoolAdmissionLease) TransportTrustID() string {
	return l.lease.owner.worker.AdmissionTrustID
}
func (l *remotePoolAdmissionLease) IssueManifestJob(context.Context, string) (prospectjob.ManifestJob, error) {
	// Only the pool supplies jobs; the remote admission capability cannot issue.
	return prospectjob.ManifestJob{}, errRemotePoolCredential
}
func (l *remotePoolAdmissionLease) SubmitManifest(ctx context.Context, job prospectjob.ManifestJob, proof prospect.Proof, auth gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error) {
	return remotePoolLeaseCall(ctx, l.lease, true, func(c context.Context) (prospectservice.ManifestAdmissionResult, error) {
		return l.lease.admission.SubmitManifest(c, job, proof, auth)
	})
}

var _ poolservice.Authority = (*remotePoolPoolLease)(nil)
