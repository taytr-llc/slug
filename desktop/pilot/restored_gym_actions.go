// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slugwallet/wallet"
)

// These projections contain no signed transaction, mnemonic or private key.
// Returning a review does not authorize signing: the exact private handle and
// one-use review ID must remain in this same live restored session.
type RestoredTransferReview struct {
	RequestID   string                `json:"request_id"`
	ReviewID    string                `json:"review_id"`
	ProspectID  string                `json:"prospect_id"`
	ExpiresAtMS int64                 `json:"expires_at_ms"`
	Details     fightercarrier.Review `json:"details"`
}
type RestoredActionStatus struct {
	RequestID         string                      `json:"request_id"`
	Kind              string                      `json:"kind"`
	Stage             string                      `json:"stage"`
	TransferID        string                      `json:"transfer_id"`
	CommitTxID        string                      `json:"commit_txid"`
	SpendTxID         string                      `json:"spend_txid"`
	CommitObservation *fightercarrier.Observation `json:"commit_observation,omitempty"`
	SpendObservation  *fightercarrier.Observation `json:"spend_observation,omitempty"`
}

func restoredActionStatus(v fightercarrier.Operation) RestoredActionStatus {
	s := RestoredActionStatus{RequestID: v.RequestID, Kind: v.Request.Kind, Stage: v.Stage, CommitTxID: v.Request.CommitTxID}
	if v.SignedPackage != nil {
		s.TransferID = v.SignedPackage.TransferID
		s.CommitTxID = v.SignedPackage.Commit.TxID
		s.SpendTxID = v.SignedPackage.Spend.TxID
	}
	if v.CommitObservation != nil {
		x := *v.CommitObservation
		s.CommitObservation = &x
	}
	if v.SpendObservation != nil {
		x := *v.SpendObservation
		s.SpendObservation = &x
	}
	return s
}

type RestoredRecoveryReviewProgress struct {
	RequestID string                  `json:"request_id"`
	Examined  uint32                  `json:"examined"`
	Total     uint32                  `json:"total"`
	Complete  bool                    `json:"complete"`
	Matched   bool                    `json:"matched"`
	Review    *RestoredTransferReview `json:"review,omitempty"`
}
type restoredRecoverySelection struct {
	request  fightercarrier.Request
	prospect string
	match    *wallet.CarrierRecoveryMatchScan
	progress wallet.CarrierRecoveryProgress
}

func (s *RestoredGymSession) clearActions() error {
	var e error
	if s.prepared != nil {
		e = errors.Join(e, s.prepared.Close())
		s.prepared = nil
	}
	if s.selection != nil && s.selection.match != nil {
		e = errors.Join(e, s.selection.match.Close())
	}
	s.selection = nil
	s.review = nil
	s.deliveryReview = nil
	return e
}
func (s *RestoredGymSession) actionReady(ctx context.Context) error {
	if s.ready == nil || s.carrier == nil {
		return ErrRestoredGym
	}
	if e := s.connect(ctx); e != nil {
		s.rememberCleanup(e)
		return e
	}
	if s.actionViewID == "" {
		id, e := restoredID()
		if e != nil {
			return e
		}
		s.actionViewID = id
	}
	if !s.installed.ChainSynced() {
		return fighterownership.ErrUnavailable
	}
	observation, e := s.node.Observe(ctx)
	if e != nil {
		return e
	}
	stamp := s.installed.Manager.SyncedTo()
	if stamp.Height < 0 || uint32(stamp.Height) != observation.Height || stamp.Hash.String() != observation.Hash {
		return fighterownership.ErrUnavailable
	}
	return s.disclose(ctx)
}
func (s *RestoredGymSession) actionFence(ctx context.Context) fightercarrier.Fence {
	return func(fn func(func() error) error) error {
		check := func() error {
			if s.closing.Load() || s.poisoned.Load() {
				return fightercarrier.ErrClosed
			}
			return errors.Join(ctx.Err(), s.life.Err())
		}
		if e := check(); e != nil {
			return e
		}
		if fn == nil {
			return fightercarrier.ErrInvalid
		}
		if e := fn(check); e != nil {
			return e
		}
		return check()
	}
}
func (s *RestoredGymSession) publishAction(ctx context.Context, p *fightercarrier.PreparedCommit, e error) (v fightercarrier.Operation, resultErr error) {
	if e != nil {
		return v, e
	}
	defer func() { resultErr = errors.Join(resultErr, p.Close()) }()
	if e = s.disclose(ctx); e != nil {
		return v, e
	}
	v, e = p.Publish(s.actionFence(ctx))
	if e != nil {
		if errors.Is(e, fightercarrier.ErrOutcomeUnknown) || errors.Is(e, fightercarrier.ErrPoisoned) {
			s.poisoned.Store(true)
		}
		return v, e
	}
	if e = s.disclose(ctx); e != nil {
		s.poisoned.Store(true)
		return v, errors.Join(fightercarrier.ErrOutcomeUnknown, e)
	}
	return v, nil
}
func (s *RestoredGymSession) reconcileAction(ctx context.Context, j *fightercarrier.Owner, id string) error {
	proof, e := j.Reconciliation(id)
	if e == nil {
		e = s.carrier.Reconcile(ctx, proof)
	}
	if e != nil {
		s.poisoned.Store(true)
		return errors.Join(fightercarrier.ErrOutcomeUnknown, e)
	}
	return nil
}
func (s *RestoredGymSession) retainActionReview(ctx context.Context, prospect string, p *wallet.PreparedFighterTransfer) (RestoredTransferReview, error) {
	if e := p.Check(ctx); e != nil {
		return RestoredTransferReview{}, errors.Join(e, p.Close())
	}
	prep, e := p.Preparation()
	if e != nil {
		return RestoredTransferReview{}, errors.Join(e, p.Close())
	}
	id, e := restoredID()
	if e != nil {
		return RestoredTransferReview{}, errors.Join(e, p.Close())
	}
	r := RestoredTransferReview{RequestID: prep.Request.RequestID, ReviewID: id, ProspectID: prospect, ExpiresAtMS: time.Now().Add(time.Minute).UnixMilli(), Details: prep.Review}
	if e = s.disclose(ctx); e != nil {
		return RestoredTransferReview{}, errors.Join(e, p.Close())
	}
	s.prepared = p
	s.review = &r
	return r, nil
}
func (s *RestoredGymSession) publishPreparation(ctx context.Context, id string, p *wallet.PreparedFighterTransfer) error {
	prep, e := p.Preparation()
	if e != nil {
		return e
	}
	j, e := s.journal.TryAcquire(ctx)
	if e != nil {
		return e
	}
	defer j.Close()
	staged, e := j.StagePrepared(id, prep)
	_, e = s.publishAction(ctx, staged, e)
	return e
}
func (s *RestoredGymSession) prepareNewTransfer(ctx context.Context, req fightercarrier.Request, prospect string, h *fighterownership.HeadLease) (*wallet.PreparedFighterTransfer, error) {
	saved, e := s.carrier.InspectPrepared(ctx, req.RequestID)
	var prepared *wallet.PreparedFighterTransfer
	var public *fighterownership.PreparedTransfer
	if e == nil {
		if saved.Request != req || saved.NewFunding == nil {
			return nil, fightercarrier.ErrConflict
		}
		tuple := make(fighterownership.FundingTuple, len(saved.NewFunding.Inputs))
		for i, in := range saved.NewFunding.Inputs {
			tuple[i] = fighterownership.FundingOutpoint{TxID: in.TxID, Index: in.OutputIndex}
		}
		public, e = s.store.PrepareTransfer(ctx, h, prospect, s.initial.OriginalManifest.GymPublicKey, req.RecipientGym, tuple)
		if e != nil {
			return nil, e
		}
		prepared, e = s.carrier.ResumePrepared(ctx, req.RequestID, public)
	} else if errors.Is(e, fightercarrier.ErrNotFound) {
		var plan *wallet.CarrierFundingPlan
		plan, e = s.carrier.PlanFunding(ctx, req)
		if e != nil {
			return nil, e
		}
		public, e = s.store.PrepareTransfer(ctx, h, prospect, s.initial.OriginalManifest.GymPublicKey, req.RecipientGym, plan.Funding())
		if e != nil {
			return nil, e
		}
		prepared, e = s.carrier.Prepare(ctx, plan, public)
	}
	if e != nil {
		return nil, e
	}
	if e = s.publishPreparation(ctx, req.RequestID, prepared); e != nil {
		return nil, errors.Join(e, prepared.Close())
	}
	return prepared, nil
}

// PrepareTransfer requires the caller's exact preselected request ID. Failure
// never implies that a committed wallet preparation or intent was discarded.
func (s *RestoredGymSession) PrepareTransfer(ctx context.Context, id, prospect, recipient string) (RestoredTransferReview, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredTransferReview{}, e
	}
	defer end()
	if !payoutNonzeroID(id) {
		return RestoredTransferReview{}, fightercarrier.ErrInvalid
	}
	if e = s.clearActions(); e != nil {
		return RestoredTransferReview{}, e
	}
	if e = s.actionReady(work); e != nil {
		return RestoredTransferReview{}, e
	}
	c, e := s.store.CaptureOwned(work, s.initial.OriginalManifest.GymPublicKey)
	if e != nil {
		return RestoredTransferReview{}, e
	}
	v, e := s.store.OwnedItem(work, c, prospect)
	if e != nil {
		return RestoredTransferReview{}, e
	}
	h, e := s.store.AcquireHead(work, c.Head())
	if e != nil {
		return RestoredTransferReview{}, e
	}
	defer h.Close()
	req := fightercarrier.Request{Version: 1, RequestID: id, Kind: "transfer", OwnershipPinID: s.initial.OwnershipPinID, PriorStateID: v.State.StateID, RecipientGym: recipient}
	if e = fightercarrier.ValidateRequest(req); e != nil {
		return RestoredTransferReview{}, e
	}
	j, e := s.journal.TryAcquire(work)
	if e != nil {
		return RestoredTransferReview{}, e
	}
	p, e := j.StageIntent(req)
	_, e = s.publishAction(work, p, e)
	e = errors.Join(e, j.Close())
	if e != nil {
		return RestoredTransferReview{}, e
	}
	prepared, e := s.prepareNewTransfer(work, req, prospect, h)
	if e != nil {
		return RestoredTransferReview{}, e
	}
	return s.retainActionReview(work, prospect, prepared)
}

// BeginRecoveryReview selects a candidate/kind but performs no durable journal
// write yet. The original file must match through bounded read-only steps first.
func (s *RestoredGymSession) BeginRecoveryReview(ctx context.Context, id, commitID, kind string) (RestoredRecoveryReviewProgress, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredRecoveryReviewProgress{}, e
	}
	defer end()
	if !payoutNonzeroID(id) || (kind != "recover_reveal" && kind != "recover_refund") {
		return RestoredRecoveryReviewProgress{}, fightercarrier.ErrInvalid
	}
	if e = s.clearActions(); e != nil {
		return RestoredRecoveryReviewProgress{}, e
	}
	if e = s.actionReady(work); e != nil {
		return RestoredRecoveryReviewProgress{}, e
	}
	head, e := s.store.Head(work)
	if e != nil {
		return RestoredRecoveryReviewProgress{}, e
	}
	h, e := s.store.AcquireHead(work, head)
	if e != nil {
		return RestoredRecoveryReviewProgress{}, e
	}
	defer h.Close()
	public, e := s.store.PrepareRecovery(work, h, commitID)
	if e != nil {
		return RestoredRecoveryReviewProgress{}, e
	}
	i := public.Intent()
	req := fightercarrier.Request{Version: 1, RequestID: id, Kind: kind, OwnershipPinID: s.initial.OwnershipPinID, PriorStateID: i.PriorStateID, RecipientGym: i.Recipient, CommitTxID: commitID}
	if e = fightercarrier.ValidateRequest(req); e != nil {
		return RestoredRecoveryReviewProgress{}, e
	}
	match, e := s.carrier.BeginRecoveryMatch(work, public)
	if e != nil {
		return RestoredRecoveryReviewProgress{}, e
	}
	s.selection = &restoredRecoverySelection{request: req, prospect: i.ProspectID, match: match}
	if e = s.disclose(work); e != nil {
		return RestoredRecoveryReviewProgress{}, e
	}
	return RestoredRecoveryReviewProgress{RequestID: id}, nil
}
func (s *RestoredGymSession) RecoveryReviewStep(ctx context.Context) (RestoredRecoveryReviewProgress, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredRecoveryReviewProgress{}, e
	}
	defer end()
	selected := s.selection
	if selected == nil {
		return RestoredRecoveryReviewProgress{}, fightercarrier.ErrNotFound
	}
	out := RestoredRecoveryReviewProgress{RequestID: selected.request.RequestID}
	if s.review != nil && s.prepared != nil {
		v := *s.review
		out.Review = &v
		out.Complete = true
		out.Matched = true
		return out, s.disclose(work)
	}
	p, e := selected.match.Step(work)
	if e != nil {
		return out, e
	}
	selected.progress = p
	out.Examined = p.Examined
	out.Total = p.Total
	out.Complete = p.Complete
	out.Matched = p.Matched
	if !p.Complete || !p.Matched {
		return out, s.disclose(work)
	}
	if e = s.actionReady(work); e != nil {
		return out, e
	}
	j, e := s.journal.TryAcquire(work)
	if e != nil {
		return out, e
	}
	staged, e := j.StageIntent(selected.request)
	_, e = s.publishAction(work, staged, e)
	e = errors.Join(e, j.Close())
	if e != nil {
		return out, e
	}
	// Only the committed gym authorization is reconstructed at preparation;
	// spending signatures remain behind the later one-use economic approval.
	if e = s.installed.Unlock([]byte(fixtureWalletPrivatePass), nil); e != nil {
		return out, e
	}
	prepared, e := s.carrier.PrepareMatchedRecovery(work, selected.request, selected.match)
	s.installed.Lock()
	if e != nil {
		return out, e
	}
	if e = s.publishPreparation(work, selected.request.RequestID, prepared); e != nil {
		return out, errors.Join(e, prepared.Close())
	}
	review, e := s.retainActionReview(work, selected.prospect, prepared)
	if e != nil {
		return out, e
	}
	out.Review = &review
	return out, nil
}

// ResumeAction reads only this exact journal request. It never selects a latest
// operation or changes a saved funding tuple/return path after a lost reply.
func (s *RestoredGymSession) ResumeAction(ctx context.Context, id, prospect string) (RestoredTransferReview, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredTransferReview{}, e
	}
	defer end()
	if e = s.clearActions(); e != nil {
		return RestoredTransferReview{}, e
	}
	if e = s.actionReady(work); e != nil {
		return RestoredTransferReview{}, e
	}
	j, e := s.journal.TryAcquire(work)
	if e != nil {
		return RestoredTransferReview{}, e
	}
	v, e := j.Lookup(id)
	e = errors.Join(e, j.Close())
	if e != nil {
		return RestoredTransferReview{}, e
	}
	if v.Stage != "intent" && v.Stage != "prepared" {
		return RestoredTransferReview{}, fightercarrier.ErrConflict
	}
	head, e := s.store.Head(work)
	if e != nil {
		return RestoredTransferReview{}, e
	}
	h, e := s.store.AcquireHead(work, head)
	if e != nil {
		return RestoredTransferReview{}, e
	}
	defer h.Close()
	var p *wallet.PreparedFighterTransfer
	if v.Request.Kind == "transfer" {
		c, e := s.store.CaptureOwned(work, s.initial.OriginalManifest.GymPublicKey)
		if e != nil {
			return RestoredTransferReview{}, e
		}
		item, e := s.store.OwnedItem(work, c, prospect)
		if e != nil || item.State.StateID != v.Request.PriorStateID {
			return RestoredTransferReview{}, errors.Join(fightercarrier.ErrConflict, e)
		}
		p, e = s.prepareNewTransfer(work, v.Request, prospect, h)
	} else {
		public, x := s.store.PrepareRecovery(work, h, v.Request.CommitTxID)
		if x != nil {
			return RestoredTransferReview{}, x
		}
		prospect = public.Intent().ProspectID
		p, e = s.carrier.ResumeRecoveryPrepared(work, id, public)
		if e == nil {
			e = s.publishPreparation(work, id, p)
		}
	}
	if e != nil {
		if p != nil {
			e = errors.Join(e, p.Close())
		}
		return RestoredTransferReview{}, e
	}
	return s.retainActionReview(work, prospect, p)
}

func (s *RestoredGymSession) ConfirmAction(ctx context.Context, reviewID string) (RestoredActionStatus, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	defer end()
	p, r := s.prepared, s.review
	s.prepared = nil
	s.review = nil
	if p == nil || r == nil {
		return RestoredActionStatus{}, fightercarrier.ErrUnauthorized
	}
	defer p.Close()
	now := time.Now().UnixMilli()
	if r.ReviewID != reviewID || now >= r.ExpiresAtMS || now < r.ExpiresAtMS-60000 {
		return RestoredActionStatus{}, fightercarrier.ErrUnauthorized
	}
	if e = s.actionReady(work); e != nil {
		return RestoredActionStatus{}, e
	}
	if e = p.Check(work); e != nil {
		return RestoredActionStatus{}, e
	}
	if e = s.installed.Unlock([]byte(fixtureWalletPrivatePass), nil); e != nil {
		return RestoredActionStatus{}, e
	}
	var pair fightercarrier.SignedPackage
	if r.Details.Kind == "transfer" {
		env, x := s.carrier.SignRestoredAuthorization(work, p)
		e = x
		if e == nil {
			pair, e = s.carrier.Sign(work, p, env)
		}
	} else {
		pair, e = s.carrier.SignRecovery(work, p)
	}
	s.installed.Lock()
	if e != nil {
		return RestoredActionStatus{}, e
	}
	action, e := restoredID()
	if e != nil {
		return RestoredActionStatus{}, e
	}
	j, e := s.journal.TryAcquire(work)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	defer j.Close()
	staged, e := j.StageSigned(r.RequestID, pair, fightercarrier.AuthorizationInput{ActionID: action, ReviewID: r.ReviewID, ViewID: s.actionViewID, Generation: 0})
	if e != nil {
		return RestoredActionStatus{}, e
	}
	if e = p.Check(work); e != nil {
		return RestoredActionStatus{}, errors.Join(e, staged.Close())
	}
	v, e := s.publishAction(work, staged, e)
	if e != nil {
		s.poisoned.Store(true)
		return RestoredActionStatus{}, errors.Join(fightercarrier.ErrOutcomeUnknown, e)
	}
	post := p.Check(work)
	e = s.reconcileAction(work, j, r.RequestID)
	if post != nil || e != nil {
		s.poisoned.Store(true)
		return RestoredActionStatus{}, errors.Join(fightercarrier.ErrOutcomeUnknown, post, e)
	}
	return restoredActionStatus(v), s.disclose(work)
}
func (s *RestoredGymSession) Action(ctx context.Context, id string) (RestoredActionStatus, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	defer end()
	if s.journal == nil {
		return RestoredActionStatus{}, ErrRestoredGym
	}
	j, e := s.journal.TryAcquire(work)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	defer j.Close()
	v, e := j.Lookup(id)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	return restoredActionStatus(v), s.disclose(work)
}

// WaitSynchronized does no mining or wallet mutation. It waits for the concrete
// installed backend to reach one unchanged node observation before an action.
func (s *RestoredGymSession) WaitSynchronized(ctx context.Context) error {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return e
	}
	defer end()
	if e = s.connect(work); e != nil {
		s.rememberCleanup(e)
		return e
	}
	observation, e := s.node.Observe(work)
	if e != nil {
		return e
	}
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		stamp := s.installed.Manager.SyncedTo()
		if s.installed.ChainSynced() && stamp.Height >= 0 && uint32(stamp.Height) == observation.Height && stamp.Hash.String() == observation.Hash {
			if e = s.node.Check(work, observation); e != nil {
				return e
			}
			return s.disclose(work)
		}
		select {
		case <-work.Done():
			return work.Err()
		case <-tick.C:
		}
	}
}

type RestoredActionPage struct {
	Next     uint32                 `json:"next"`
	Complete bool                   `json:"complete"`
	Actions  []RestoredActionStatus `json:"actions"`
}

func (s *RestoredGymSession) Actions(ctx context.Context, after uint32) (RestoredActionPage, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredActionPage{}, e
	}
	defer end()
	if s.journal == nil {
		return RestoredActionPage{}, ErrRestoredGym
	}
	j, e := s.journal.TryAcquire(work)
	if e != nil {
		return RestoredActionPage{}, e
	}
	defer j.Close()
	snap, e := j.Snapshot()
	if e != nil {
		return RestoredActionPage{}, e
	}
	rows, e := j.List(after, 8)
	if e != nil {
		return RestoredActionPage{}, e
	}
	v := RestoredActionPage{Next: after + uint32(len(rows)), Actions: make([]RestoredActionStatus, 0, len(rows))}
	if v.Next > snap.Count {
		return RestoredActionPage{}, fightercarrier.ErrCorrupt
	}
	v.Complete = v.Next == snap.Count
	for _, r := range rows {
		v.Actions = append(v.Actions, restoredActionStatus(r))
	}
	return v, s.disclose(work)
}
