// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
)

// poolChildAdmission has its own drained transport gate. The pool issues every
// job; this adapter can only submit/recover the separately signed fighter.
type poolChildAdmission struct {
	fence  *PoolSessionAuthority
	config prospectworker.PoolConfig
}

func (a *poolChildAdmission) ProtocolVersion() uint32 { return 3 }
func (a *poolChildAdmission) AuthorityOrigin() string { return a.config.AdmissionOrigin }
func (a *poolChildAdmission) AuthorityPin() registry.ManifestAuthorityPin {
	return a.config.Source.AuthorityPin
}
func (a *poolChildAdmission) TransportTrustID() string { return a.config.AdmissionTrustID }
func (a *poolChildAdmission) IssueManifestJob(context.Context, string) (prospectjob.ManifestJob, error) {
	return prospectjob.ManifestJob{}, errors.New("pool jobs require the pool authority")
}
func (a *poolChildAdmission) SubmitManifest(ctx context.Context, job prospectjob.ManifestJob, proof prospect.Proof, auth gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error) {
	var zero prospectservice.ManifestAdmissionResult
	c, finish, err := a.fence.begin(ctx)
	if err != nil {
		return zero, err
	}
	defer finish()
	if err = a.fence.checkView(c); err != nil {
		return zero, err
	}
	token, err := a.fence.account.Account.MiningToken(c)
	if err != nil || a.fence.checkToken(c, token) != nil {
		return zero, poolAccessError(false)
	}
	client, err := prospectservice.NewManifestClient(prospectservice.ManifestClientConfig{Version: 3, URL: a.config.AdmissionOrigin, Token: token, AuthorityPin: a.config.Source.AuthorityPin}, a.config.Source.Policy)
	if err != nil {
		return zero, err
	}
	defer client.Close()
	if client.ProtocolVersion() != 3 || client.AuthorityOrigin() != a.config.AdmissionOrigin || client.AuthorityPin() != a.config.Source.AuthorityPin || client.TransportTrustID() != a.config.AdmissionTrustID {
		return zero, poolAccessError(false)
	}
	result, err := client.SubmitManifest(c, job, proof, auth)
	if a.fence.checkToken(c, token) != nil || a.fence.checkView(c) != nil {
		return zero, poolAccessError(true)
	}
	return result, err
}
func (a *poolChildAdmission) Close() error { return a.fence.Close() }

// Neither the wallet lock nor signature computation runs under an account
// lease. A reset during signing discards the result under the original token.
func poolChildSigner(fence *PoolSessionAuthority, signer prospectworker.Signer) prospectworker.Signer {
	return func(ctx context.Context, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
		if fence.checkView(ctx) != nil {
			return gymauth.Authorization{}, poolAccessError(false)
		}
		token, err := fence.account.Account.MiningToken(ctx)
		if err != nil {
			return gymauth.Authorization{}, poolAccessError(false)
		}
		lease, err := mininglease.Capture(ctx, fence.account.Account.Authorizer, token)
		if err != nil || lease.PublicKey() != fence.config.PublicKey {
			return gymauth.Authorization{}, poolAccessError(false)
		}
		auth, err := signer(ctx, initial, proof)
		if lease.Check() != nil || fence.checkView(ctx) != nil {
			return gymauth.Authorization{}, poolAccessError(false)
		}
		return auth, err
	}
}
