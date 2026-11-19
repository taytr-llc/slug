// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"

	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectwork"
)

// MiningWork contains the original durable preparation needed to reconstruct
// the exact nonce-zero candidate under the separately pinned pool config.
// Extra nonce is always zero. The owned template and job do not authenticate
// a caller, renew expiry, or confer share credit or fighter admission.
type MiningWork struct {
	Job      prospectjob.ManifestJob `json:"job"`
	Template prospectwork.Template   `json:"template"`
}

// IssueMiningWork uses the same single issuance and synchronous persistence as
// Issue. It exports an owned original template rather than a second candidate.
func (p *Pool) IssueMiningWork(ctx context.Context, token string) (MiningWork, error) {
	w, _, err := p.issue(ctx, token)
	if err != nil {
		return MiningWork{}, err
	}
	return MiningWork{Job: w.Job, Template: w.Template}, nil
}

// RecoverMiningWork returns the original gym's retained work, including after
// expiry. It neither issues new work nor asks the node to refresh that work.
func (p *Pool) RecoverMiningWork(ctx context.Context, token, jobID string) (MiningWork, error) {
	var result MiningWork
	err := p.withSession(ctx, token, func(ctx context.Context, key string, recheck func() error) error {
		w, _, err := p.loadWork(jobID)
		if err != nil {
			return err
		}
		if w.Job.PublicKey != key {
			return ErrUnauthorized
		}
		result = MiningWork{Job: w.Job, Template: w.Template}
		return recheck()
	})
	if err != nil {
		return MiningWork{}, err
	}
	return result, nil
}
