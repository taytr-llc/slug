// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"reflect"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
)

func remotePoolUnknown(admission bool) error {
	if admission {
		return &prospectservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	return &poolservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
}

// Private fixed-operation adapters use this fence; it is not a public signer
// or arbitrary request callback. Account leases never span the network call.
func remotePoolWorkerCall[T any](ctx context.Context, a *RemotePoolAttachment, mut, admission bool, fn func(context.Context) (T, error)) (out T, err error) {
	if e := a.check(ctx); e != nil {
		return out, e
	}
	lease, _, e := a.local.capture(ctx)
	if e != nil {
		return out, e
	}
	out, err = fn(ctx)
	if ctx.Err() != nil || lease.Check() != nil || a.check(ctx) != nil {
		var zero T
		if mut {
			return zero, remotePoolUnknown(admission)
		}
		return zero, ErrRemotePoolUnavailable
	}
	return out, err
}

type remotePoolAuthority struct {
	owner *RemotePoolAttachment
	lease *remotePoolPoolLease
}

func (p *remotePoolAuthority) ProtocolVersion() uint32 {
	if p.owner.recovery != nil {
		return 3
	}
	return p.owner.config.Version
}
func (p *remotePoolAuthority) AuthorityOrigin() string        { return p.owner.config.Origin }
func (p *remotePoolAuthority) SourceConfig() poolshare.Config { return p.owner.config.Source }
func (p *remotePoolAuthority) PublicKey() string              { return p.owner.view.GymPublicKey }
func (p *remotePoolAuthority) TransportTrustID() string {
	if p.owner.recovery != nil {
		return p.owner.recoveryWorker.TransportTrustID
	}
	return p.owner.workerConfig.TransportTrustID
}
func (p *remotePoolAuthority) Issue(ctx context.Context) (poolshare.MiningWork, error) {
	return remotePoolWorkerCall(ctx, p.owner, true, false, p.lease.Issue)
}
func (p *remotePoolAuthority) Work(ctx context.Context, id string) (poolshare.MiningWork, error) {
	return remotePoolWorkerCall(ctx, p.owner, false, false, func(c context.Context) (poolshare.MiningWork, error) { return p.lease.Work(c, id) })
}
func (p *remotePoolAuthority) Submit(ctx context.Context, w poolshare.MiningWork, n uint32) (poolshare.Submission, error) {
	return remotePoolWorkerCall(ctx, p.owner, true, false, func(c context.Context) (poolshare.Submission, error) { return p.lease.Submit(c, w, n) })
}
func (p *remotePoolAuthority) Lookup(ctx context.Context, w poolshare.MiningWork, n uint32) (poolshare.Submission, error) {
	return remotePoolWorkerCall(ctx, p.owner, false, false, func(c context.Context) (poolshare.Submission, error) { return p.lease.Lookup(c, w, n) })
}
func (p *remotePoolAuthority) Balance(ctx context.Context) (poolservice.BalanceResponse, error) {
	return remotePoolWorkerCall(ctx, p.owner, false, false, p.lease.Balance)
}

type remotePoolAdmission struct {
	owner *RemotePoolAttachment
	lease *remotePoolAdmissionLease
}

func (p *remotePoolAdmission) ProtocolVersion() uint32 { return 3 }
func (p *remotePoolAdmission) AuthorityOrigin() string { return p.owner.config.Origin }
func (p *remotePoolAdmission) AuthorityPin() registry.ManifestAuthorityPin {
	return p.owner.config.Source.AuthorityPin
}
func (p *remotePoolAdmission) TransportTrustID() string { return p.owner.workerConfig.AdmissionTrustID }
func (p *remotePoolAdmission) IssueManifestJob(context.Context, string) (prospectjob.ManifestJob, error) {
	return prospectjob.ManifestJob{}, ErrRemotePoolUnavailable
}
func (p *remotePoolAdmission) SubmitManifest(ctx context.Context, j prospectjob.ManifestJob, proof prospect.Proof, auth gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error) {
	return remotePoolWorkerCall(ctx, p.owner, true, true, func(c context.Context) (prospectservice.ManifestAdmissionResult, error) {
		return p.lease.SubmitManifest(c, j, proof, auth)
	})
}
func (a *RemotePoolAttachment) signAdmission(ctx context.Context, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
	if a == nil || ctx == nil || len(proof.MerkleBranch) > 20 || len(proof.Coinbase) > 32768 || len(proof.Header) != 160 || proof.Version != prospect.Version2 {
		return gymauth.Authorization{}, gymaccount.ErrUnauthorized
	}
	p := a.config.Source.Policy
	if initial.Version != p.Version || initial.Network != p.Network || initial.Genesis != p.Genesis || initial.DNAVersion != p.DNAVersion || initial.ProspectTarget != p.ProspectTarget || proof.GymID != a.view.GymID {
		return gymauth.Authorization{}, gymaccount.ErrUnauthorized
	}
	proof = remotePoolOwnAdmissionProof(proof)
	if e := a.check(ctx); e != nil {
		return gymauth.Authorization{}, e
	}
	matches := 0
	var after uint64
	for {
		if ctx.Err() != nil {
			return gymauth.Authorization{}, ctx.Err()
		}
		rows, e := a.box.List(after, 100)
		if e != nil {
			return gymauth.Authorization{}, e
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if r.Sequence <= after || r.Sequence > a.workerConfig.MaxRecords {
				return gymauth.Authorization{}, ErrRemotePoolChanged
			}
			after = r.Sequence
			f := r.Fighter
			if remotePoolSavedAdmissionMatches(f, initial, proof, a.view.GymPublicKey, a.config.Source.AuthorityPin) {
				matches++
			}
		}
	}
	if matches != 1 {
		return gymauth.Authorization{}, gymaccount.ErrUnauthorized
	}
	lease, token, e := a.local.capture(ctx)
	if e != nil {
		return gymauth.Authorization{}, e
	}
	s := a.local.session
	auth, e := s.setup.vault.SignAdmission(ctx, token, initial, proof, s.epoch)
	if e != nil {
		return gymauth.Authorization{}, e
	}
	if auth.PublicKey != a.view.GymPublicKey || gymauth.VerifyAuthorization(initial, proof, auth) != nil || lease.Check() != nil || a.check(ctx) != nil {
		return gymauth.Authorization{}, gymaccount.ErrUnauthorized
	}
	return auth, nil
}

func (a *RemotePoolAttachment) Start(ctx context.Context, view string) error {
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return e
	}
	defer end()
	a.mu.Lock()
	running := a.run != nil
	a.mu.Unlock()
	if running {
		return a.check(work)
	}
	// Start's run publication uses the original View; each worker operation also
	// captures its separate actual Mining token before doing work.
	if _, _, e = a.local.capture(work); e != nil {
		return e
	}
	life, cancel := context.WithCancel(a.life)
	p, e := a.credentials.beginPool(life)
	if e != nil {
		cancel()
		return e
	}
	q, e := a.credentials.beginAdmission(life)
	if e != nil {
		cancel()
		return a.startCleanup(e, p, nil)
	}
	var w *prospectworker.PoolWorker
	if a.recovery != nil {
		w, e = prospectworker.NewRecoverableDailyPool(a.recoveryWorker, p.lease.pool, &remotePoolAuthority{a, p}, &remotePoolAdmission{a, q}, a.node, a.signAdmission, a.box)
	} else if a.config.Version == 2 {
		w, e = prospectworker.NewDailyPool(a.workerConfig, &remotePoolAuthority{a, p}, &remotePoolAdmission{a, q}, a.node, a.signAdmission, a.box)
	} else {
		w, e = prospectworker.NewPool(a.workerConfig, &remotePoolAuthority{a, p}, &remotePoolAdmission{a, q}, a.node, a.signAdmission, a.box)
	}
	if e != nil {
		cancel()
		return a.startCleanup(e, p, q)
	}
	run := &remotePoolRun{ctx: life, cancel: cancel, done: make(chan struct{}), worker: w}
	published := false
	e = a.local.withView(work, func(_ gymaccount.Account, check func() error) error {
		if a.config.Version == 2 {
			if e := check(); e != nil {
				return e
			}
			if e := work.Err(); e != nil {
				return e
			}
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.closing || !a.ready || a.run != nil || a.life.Err() != nil {
				return ErrRemotePoolUnavailable
			}
			a.run = run
			a.state, a.lastError = "starting", ""
			a.active.Add(1)
			published = true
			return nil
		}
		if check() == nil && work.Err() == nil {
			a.mu.Lock()
			defer a.mu.Unlock()
			if !a.closing && a.ready && a.run == nil && a.life.Err() == nil {
				a.run = run
				a.state = "starting"
				a.lastError = ""
				a.active.Add(1)
				published = true
				return nil
			}
		}
		return ErrRemotePoolUnavailable
	})
	if e != nil {
		cancel()
		e = a.startCleanup(e, p, q)
		if published {
			a.mu.Lock()
			if a.run == run {
				a.run = nil
				a.state = "stopped"
			}
			close(run.done)
			a.mu.Unlock()
			a.active.Done()
		}
		return e
	}
	go a.runWorker(run, p, q)
	return nil
}

// The daily path retains any observed failed cleanup owner. Legacy callers keep
// their original error behavior; both modes still close the same exact leases.
type remotePoolStartCleanup struct {
	pool      *remotePoolPoolLease
	admission *remotePoolAdmissionLease
	cause     error
}

func (*remotePoolStartCleanup) Error() string   { return "remote pool start cleanup incomplete" }
func (e *remotePoolStartCleanup) Unwrap() error { return e.cause }
func (a *RemotePoolAttachment) retainStartCleanup(p *remotePoolPoolLease, q *remotePoolAdmissionLease, ce error) error {
	if ce == nil || a.config.Version != 2 {
		return nil
	}
	held := &remotePoolStartCleanup{pool: p, admission: q, cause: errors.Join(ErrRemotePoolCleanup, ce)}
	a.mu.Lock()
	a.closeErr = errors.Join(a.closeErr, held)
	a.mu.Unlock()
	return held
}
func (a *RemotePoolAttachment) startCleanup(cause error, p *remotePoolPoolLease, q *remotePoolAdmissionLease) error {
	ce := errors.Join(q.Close(), p.Close())
	held := a.retainStartCleanup(p, q, ce)
	if held == nil {
		return cause
	}
	return errors.Join(cause, held)
}

func (a *RemotePoolAttachment) runWorker(run *remotePoolRun, p *remotePoolPoolLease, q *remotePoolAdmissionLease) {
	defer a.active.Done()
	e := run.worker.Run(run.ctx)
	run.cancel()
	ce := errors.Join(q.Close(), p.Close())
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.run == run {
		a.last = run.worker.Status()
		a.run = nil
		a.state = "stopped"
		a.lastError = ""
		if e != nil && a.life.Err() == nil && !errors.Is(e, context.Canceled) {
			a.state = "failed"
			a.lastError = "worker_unavailable"
		}
		if ce != nil {
			a.closeErr = errors.Join(a.closeErr, ce)
			a.state = "failed"
			a.lastError = "cleanup_incomplete"
		}
	}
	close(run.done)
}
func (a *RemotePoolAttachment) Stop(ctx context.Context, view string) error {
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return e
	}
	defer end()
	if a.config.Version == 2 {
		a.cancelDailyPayoutRead()
	}
	a.mu.Lock()
	run := a.run
	if run != nil {
		run.cancel()
	}
	a.mu.Unlock()
	// The admitted stop joins even if its caller expires; return then reports
	// original-view failure rather than claiming the run is still active.
	if run != nil {
		<-run.done
	}
	return a.check(work)
}

// Preserve the canonical nonnil empty branch of a coinbase-only block.
func remotePoolOwnAdmissionProof(proof prospect.Proof) prospect.Proof {
	proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	return proof
}

// Only an already-audited outbox row reaches this private selection predicate.
// Matching a shaped caller value alone never grants signing authority.
func remotePoolSavedAdmissionMatches(f *prospectworker.ManifestEntry, initial prospect.Context, proof prospect.Proof, key string, pin registry.ManifestAuthorityPin) bool {
	return f != nil && f.ProspectWin && f.Authorization == nil && f.AdmissionState == "needs-signature" && f.Job.Context == initial && f.Job.PublicKey == key && f.Job.AuthorityID == pin.AuthorityID && f.Job.DescriptorID == pin.DescriptorID && reflect.DeepEqual(f.Candidate.Proof, proof)
}
