// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// A pool operation owns the pool gate for its lifetime, but holds the account
// lease only for a local commit or final disclosure. No node, proof, or blocking
// issuer read may run in do. The original token is never replaced after reset.
type poolSession struct {
	lease   *mininglease.Lease
	ctx     context.Context // original bounded operation; never a replacement
	gym     string          // derived from the original captured key and policy
	effects bool            // issued work, retained share, or attempted block transmission
}

func poolLeaseError(err error) error {
	switch {
	case errors.Is(err, mininglease.ErrOutcomeUnknown):
		return ErrOutcomeUnknown
	case errors.Is(err, mininglease.ErrUnauthorized):
		return ErrUnauthorized
	default:
		return err
	}
}

func (s *poolSession) check() error { return poolLeaseError(s.lease.Check()) }
func (s *poolSession) do(fn func(func() error) error) error {
	return poolLeaseError(s.lease.Do(fn))
}
func (p *Pool) sessionClock(s *poolSession) (int64, error) {
	var at int64
	err := s.do(func(check func() error) error {
		var e error
		at, e = p.clock(check)
		return e
	})
	return at, err
}
func (p *Pool) sessionSubmission(s *poolSession, job prospectjob.ManifestJob, r Submission, previous *Submission) (Submission, error) {
	// The issuer may be serving a long historical proof read. Wait only while
	// the account gate is free, and preserve the previous exact-job health check.
	actual, err := p.originalJob(s.ctx, job.ID)
	if err != nil {
		return Submission{}, p.sourceReadError(s.ctx, err)
	}
	if actual != job {
		return Submission{}, ErrCorrupt
	}
	var saved Submission
	err = s.do(func(check func() error) error {
		var e error
		saved, e = p.saveSubmission(job, r, previous, check)
		if e == nil {
			s.effects = true
		}
		return e
	})
	if err != nil {
		return Submission{}, err
	}
	return saved, nil
}

func (p *Pool) withSession(ctx context.Context, token string, fn func(context.Context, string, *poolSession) error) error {
	if wire.TestPoW {
		return ErrProfile
	}
	if p == nil || p.auditOnly || ctx == nil || len(token) == 0 || len(token) > 4096 {
		return ErrInvalid
	}
	if p.gate == nil || p.db == nil {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.lifecycle.Lock()
	if p.closing {
		p.lifecycle.Unlock()
		return ErrClosed
	}
	select {
	case p.gate <- struct{}{}:
	default:
		p.lifecycle.Unlock()
		return ErrBusy
	}
	p.lifecycle.Unlock()
	defer func() { <-p.gate }()
	if p.poisoned {
		return ErrPoisoned
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	lease, err := mininglease.Capture(ctx, p.sessions, token)
	if err != nil {
		return poolLeaseError(err)
	}
	key := lease.PublicKey()
	identity := prospect.Context{Version: p.config.Policy.Version, Network: p.config.Policy.Network, Genesis: p.config.Policy.Genesis, DNAVersion: p.config.Policy.DNAVersion}
	gym, err := gymauth.GymID(identity, key)
	if err != nil {
		return ErrUnauthorized
	}
	session := &poolSession{lease: lease, ctx: ctx, gym: gym}
	err = fn(ctx, key, session)
	if (err == ErrCapacity || err == registry.ErrPublicCapacity) && p.operatorAdmission != nil {
		p.operatorAdmission.draining = true
	}
	if err == nil {
		err = session.check()
	}
	if p.poisoned {
		return ErrPoisoned
	}
	if err != nil && session.effects {
		return ErrOutcomeUnknown
	}
	return err
}

// Issue uses the live trusted issuer and node. Its immutable exact candidate is
// durable before acknowledgment. A failure after issuer commit may leave an
// unused issuer job; it is never assigned a new birth or silently recycled.
func (p *Pool) Issue(ctx context.Context, token string) (Work, error) {
	_, result, err := p.issue(ctx, token)
	return result, err
}

// issue is the single durable issuance path shared by candidate and transport
// exports. Neither export performs a second issuance or changes the template.
func (p *Pool) issue(ctx context.Context, token string) (savedWork, Work, error) {
	var retained savedWork
	var result Work
	err := p.withSession(ctx, token, func(ctx context.Context, key string, session *poolSession) error {
		if err := p.dailyRequestMember(session.gym); err != nil {
			return err
		}
		if err := p.operatorIntake(); err != nil {
			return err
		}
		info, err := p.originalInfo(ctx)
		if err != nil {
			return p.sourceReadError(ctx, err)
		}
		if info.Pin != p.config.AuthorityPin || info.ReadOnly {
			return ErrCorrupt
		}
		if info.Jobs >= p.config.MaxWorks || p.meta.Works >= p.config.MaxWorks || p.meta.Submissions >= p.config.MaxSubmissions || maxSubmissionSize >= p.config.MaxBytes-p.meta.Bytes {
			return ErrCapacity
		}
		if err := p.dailyWorkHeadroom(maxWorkSize); err != nil {
			return err
		}
		template, err := p.node.ResolveTemplate(ctx, p.config.Policy)
		if err != nil || !p.samePolicy(template.Context) || !templateBounds(template) {
			return ErrNode
		}
		// Own every template pointer/slice before an adapter can reuse its result.
		raw, err := json.Marshal(template)
		if err != nil || len(raw) > prospectwork.MaxTemplateSize {
			return ErrNode
		}
		var owned prospectwork.Template
		if decode(raw, prospectwork.MaxTemplateSize, &owned) != nil {
			return ErrNode
		}
		if prospectwork.ValidateTemplate(owned) != nil {
			return ErrNode
		}
		if _, err := p.sessionClock(session); err != nil {
			return err
		}
		issuance, err := p.authority.PrepareManifestIssue(owned.Context, key)
		if err != nil {
			return ErrNode
		}
		if p.node.RecheckContext(ctx, owned.Context) != nil {
			return ErrNode
		}
		var job prospectjob.ManifestJob
		err = session.do(func(check func() error) error {
			var e error
			job, e = p.authority.CommitManifestIssue(issuance, check)
			if e == nil {
				session.effects = true
			} else if errors.Is(e, registry.ErrBusy) {
				return ErrBusy
			} else if errors.Is(e, registry.ErrPoisoned) {
				return ErrOutcomeUnknown
			}
			return e
		})
		if err != nil {
			return err
		}
		w := savedWork{Job: job, Template: owned}
		prepared, err := p.prepareContext(ctx, w)
		if err != nil {
			return err
		}
		candidate := prepared.Candidate()
		if p.node.Proposal(ctx, job.Context, candidate.BlockHex) != nil {
			return ErrNode
		}
		if p.node.RecheckContext(ctx, job.Context) != nil {
			return ErrNode
		}
		if _, err := p.prepareContext(ctx, w); err != nil {
			return err
		}
		if err := session.do(func(check func() error) error { return p.saveWork(w, check) }); err != nil {
			return err
		}
		result = Work{Job: job, PoolTarget: p.config.PoolTarget, Candidate: candidate}
		retained = w
		return nil
	})
	if err != nil {
		return savedWork{}, Work{}, err
	}
	return retained, result, nil
}

// Work recovers saved public preparation for its original gym. It does not
// refresh expiry, consult the node or claim that the work is still current.
func (p *Pool) Work(ctx context.Context, token, jobID string) (Work, error) {
	var result Work
	err := p.withSession(ctx, token, func(ctx context.Context, key string, session *poolSession) error {
		w, prepared, err := p.loadWorkContext(ctx, jobID)
		if err != nil {
			return err
		}
		if w.Job.PublicKey != key {
			return ErrUnauthorized
		}
		result = Work{Job: w.Job, PoolTarget: p.config.PoolTarget, Candidate: prepared.Candidate()}
		return session.check()
	})
	if err != nil {
		return Work{}, err
	}
	return result, nil
}

// Lookup recovers an existing receipt by the original submitted identity. It
// verifies its work but never contacts the node or creates/renews acceptance.
func (p *Pool) Lookup(ctx context.Context, token, jobID string, nonce uint32) (Submission, error) {
	var result Submission
	err := p.withSession(ctx, token, func(ctx context.Context, key string, session *poolSession) error {
		w, prepared, err := p.loadWorkContext(ctx, jobID)
		if err != nil {
			return err
		}
		if w.Job.PublicKey != key {
			return ErrUnauthorized
		}
		r, err := p.loadSubmission(jobID, nonce)
		if err != nil {
			return err
		}
		if _, err := p.validateSubmission(ctx, r, w, prepared); err != nil {
			return err
		}
		result = r
		return session.check()
	})
	if err != nil {
		return Submission{}, err
	}
	return result, nil
}

// Submit accepts only a saved job ID and nonce from the original gym. A return
// error may follow a durable write or block transmission: callers recover the
// same identity, never infer nonacceptance or create replacement work. A retained
// pending/unknown/processed block is not canonical merely because this returns.
func (p *Pool) Submit(ctx context.Context, token, jobID string, nonce uint32) (Submission, error) {
	var result Submission
	err := p.withSession(ctx, token, func(ctx context.Context, key string, session *poolSession) error {
		w, prepared, err := p.loadWorkContext(ctx, jobID)
		if err != nil {
			return err
		}
		if w.Job.PublicKey != key {
			return ErrUnauthorized
		}
		original, err := p.loadSubmission(jobID, nonce)
		if err == nil {
			check, err := p.validateSubmission(ctx, original, w, prepared)
			if err != nil {
				return err
			}
			result, err = p.deliver(ctx, w, original, check, session)
			return err
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := p.dailyRequestMember(session.gym); err != nil {
			return err
		}
		intakeErr := p.operatorIntake()
		canonicalRecovery := intakeErr == ErrCapacity && p.operatorAdmission != nil && (p.operatorAdmission.recovery != nil || p.operatorAdmission.daily != nil)
		if intakeErr != nil && !canonicalRecovery {
			return intakeErr
		}
		if !(canonicalRecovery && p.operatorAdmission.daily != nil) && (p.meta.Submissions >= p.config.MaxSubmissions || maxSubmissionSize > p.config.MaxBytes-p.meta.Bytes) {
			return ErrCapacity
		}
		received, err := p.sessionClock(session)
		if err != nil {
			return err
		}
		if received < w.Job.IssuedAtMS {
			return ErrClock
		}
		check, err := prepared.CheckNonce(ctx, nonce)
		if err != nil {
			return err
		}
		if canonicalRecovery {
			result, err = p.recoverCanonical(ctx, w, check, received, session)
			return err
		}
		observed, err := p.sessionClock(session)
		if err != nil {
			return err
		}
		if !check.Pool && !check.Block {
			return ErrLowWork
		}
		current := false
		if check.Pool && observed < w.Job.ExpiresAtMS {
			current = p.node.RecheckContext(ctx, w.Job.Context) == nil
		}
		observed, err = p.sessionClock(session)
		if err != nil {
			return err
		}
		if !check.Block {
			if observed >= w.Job.ExpiresAtMS {
				return ErrExpired
			}
			if !current {
				return ErrNode
			}
		}
		r := Submission{Version: Version1, Sequence: p.meta.Submissions + 1, JobID: jobID, GymID: w.Job.GymID, Nonce: nonce, PoolTarget: p.config.PoolTarget, Header: check.Header, WorkHash: check.WorkHash, DuplicateKey: check.DuplicateKey, Pool: check.Pool, Prospect: check.Prospect, Block: check.Block, ReceivedAtMS: received, BlockState: "none"}
		if check.Block {
			r.BlockHeight = w.Job.Context.ParentHeight + 1
			r.BlockState = "pending"
		}
		if check.Pool && current && observed < w.Job.ExpiresAtMS {
			r.AcceptedSequence = p.meta.Accepted + 1
			r.AcceptedAtMS = observed
			r.Acceptance = "current-parent"
		}
		r, err = p.sessionSubmission(session, w.Job, r, nil)
		if err != nil {
			return err
		}
		if err := p.rememberRetainedNonce(ctx, w, check); err != nil {
			return err
		}
		result, err = p.deliver(ctx, w, r, check, session)
		return err
	})
	if err != nil {
		return Submission{}, err
	}
	return result, nil
}

func (p *Pool) deliver(ctx context.Context, w savedWork, r Submission, check prospectwork.ShareCheck, session *poolSession) (Submission, error) {
	if !r.Block || r.BlockState == "canonical-observed" {
		return r, session.check()
	}
	if check.Candidate == nil || !check.Block {
		return Submission{}, ErrCorrupt
	}
	if err := session.check(); err != nil {
		return Submission{}, err
	}
	// The exact block intent is already synchronized. Submission happens before
	// any fighter operation (this package has none), including for expired work.
	if r.BlockState != "rejected" {
		session.effects = true
		err := p.node.SubmitBlock(ctx, w.Job.Context, check.Candidate.BlockHex)
		next := r
		next.BlockState = "processed"
		if err != nil {
			next.BlockState = "unknown"
			var submission *prospectnode.SubmissionError
			if errors.As(err, &submission) && submission.Code == "rejected" && !submission.OutcomeUnknown {
				next.BlockState = "rejected"
			}
		}
		var saveErr error
		next, saveErr = p.sessionSubmission(session, w.Job, next, &r)
		if saveErr != nil {
			return Submission{}, saveErr
		}
		r = next
	}
	if err := session.check(); err != nil {
		return Submission{}, err
	}
	proof := check.Candidate.Proof
	if proof.MerkleBranch != nil {
		branch := make([]string, len(proof.MerkleBranch))
		copy(branch, proof.MerkleBranch)
		proof.MerkleBranch = branch
	}
	// This independent check cannot turn an ordinary RPC error into eligibility.
	// Its exact header commits the retained configured-payout coinbase.
	if p.node.CanonicalBlock(ctx, w.Job.Context, proof, check.WorkHash) != nil {
		return r, session.check()
	}
	next := r
	next.BlockState = "canonical-observed"
	if next.AcceptedSequence == 0 {
		next.AcceptedSequence = p.meta.Accepted + 1
		next.Acceptance = "canonical-winning-block"
	}
	return p.sessionSubmission(session, w.Job, next, &r)
}
