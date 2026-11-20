// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slugwallet/wallet"
)

// OwnedTransferReview is display data. Confirmation is bound to the retained
// private preparation and this process's one-use review, never to returned JSON.
type OwnedTransferReview struct {
	RequestID  string                `json:"request_id"`
	ReviewID   string                `json:"review_id"`
	ProspectID string                `json:"prospect_id"`
	Details    fightercarrier.Review `json:"details"`
}
type OwnedTransferActions struct {
	session        *OwnedWalletSession
	core           *fighterownership.Store
	chain          *fighterownership.OwnedChainSource
	node           *withdrawalNode
	release        func()
	stopParent     func() bool
	viewID         string
	life           context.Context
	cancel         context.CancelFunc
	mu             sync.Mutex
	closed         atomic.Bool
	closeOnce      sync.Once
	prepared       *wallet.PreparedFighterTransfer
	review         OwnedTransferReview
	deliveryReview *OwnedTransferDeliveryReview
	uncertain      bool
}

func NewOwnedTransferActions(ctx context.Context, s *OwnedWalletSession, runtime *PoolRuntime, viewID string) (*OwnedTransferActions, error) {
	if ctx == nil || s == nil || s.setup == nil || runtime == nil || !payoutNonzeroID(viewID) || s.Carrier() == nil || s.CarrierJournal() == nil {
		return nil, ErrPoolRuntime
	}
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed || runtime.life.Err() != nil || runtime.owned == nil || runtime.nodes == nil || runtime.owned.store == nil || runtime.owned.node == nil || runtime.source != s.setup.config.Source || runtime.owned.config.Ownership != s.setup.config.Ownership {
		return nil, ErrPoolRuntime
	}
	borrowID := s.login.Account.ID + ":fighter-transfer"
	for b := range runtime.borrows {
		if b.account == borrowID {
			return nil, ErrPoolRuntimeBusy
		}
	}
	node, e := newWithdrawalNode(runtime.nodes.rpcConfig())
	if e != nil {
		return nil, e
	}
	life, cancel := context.WithCancel(s.setup.ctx)
	o := &OwnedTransferActions{session: s, core: runtime.owned.store, chain: runtime.owned.node, node: node, viewID: viewID, life: life, cancel: cancel}
	o.stopParent = context.AfterFunc(runtime.life, cancel)
	borrow := &poolRuntimeBorrow{account: borrowID, done: make(chan struct{}), close: o.Close}
	runtime.borrows[borrow] = true
	o.release = func() {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		delete(runtime.borrows, borrow)
		close(borrow.done)
	}
	return o, nil
}
func ownedActionID() (string, error) {
	var id [32]byte
	if _, e := rand.Read(id[:]); e != nil {
		return "", e
	}
	return hex.EncodeToString(id[:]), nil
}
func (o *OwnedTransferActions) begin(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil || o == nil || o.closed.Load() {
		return nil, nil, ErrPoolRuntime
	}
	if !o.mu.TryLock() {
		return nil, nil, ErrPoolRuntimeBusy
	}
	if o.closed.Load() || o.life.Err() != nil {
		o.mu.Unlock()
		return nil, nil, ErrPoolRuntime
	}
	if o.uncertain {
		o.mu.Unlock()
		return nil, nil, fightercarrier.ErrOutcomeUnknown
	}
	if !o.session.opMu.TryLock() {
		o.mu.Unlock()
		return nil, nil, ErrPoolRuntimeBusy
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(o.life, cancel)
	done := func() { stop(); cancel(); o.session.opMu.Unlock(); o.mu.Unlock() }
	if e := o.session.check(work); e != nil {
		done()
		return nil, nil, e
	}
	return work, done, nil
}
func (o *OwnedTransferActions) Close() error {
	if o == nil {
		return nil
	}
	o.closeOnce.Do(func() {
		o.closed.Store(true)
		if o.cancel != nil {
			o.cancel()
		}
		o.mu.Lock()
		o.clearReview()
		o.deliveryReview = nil
		if o.node != nil {
			o.node.close()
		}
		if o.stopParent != nil {
			o.stopParent()
		}
		o.mu.Unlock()
		if o.release != nil {
			o.release()
		}
	})
	return nil
}
func (o *OwnedTransferActions) clearReview() {
	if o.prepared != nil {
		_ = o.prepared.Close()
	}
	o.prepared = nil
	o.review = OwnedTransferReview{}
}
func (o *OwnedTransferActions) fence(ctx context.Context, cap gymaccount.Capability) fightercarrier.Fence {
	return func(fn func(func() error) error) error {
		if fn == nil || o.closed.Load() || o.life.Err() != nil {
			return gymaccount.ErrUnauthorized
		}
		s := o.session
		return s.setup.accounts.WithSession(ctx, s.login.Token, cap, func(a gymaccount.Account, accountCheck func() error) error {
			if a.ID != s.login.Account.ID || a.Generation != s.login.Account.Generation || a.PrincipalID != s.intent.Binding.PrincipalID {
				return gymaccount.ErrUnauthorized
			}
			return s.epoch(ctx, func(parentCheck func() error) error {
				check := func() error {
					if o.closed.Load() || o.life.Err() != nil || s.closed.Load() || parentCheck == nil {
						return gymaccount.ErrUnauthorized
					}
					return errors.Join(ctx.Err(), accountCheck(), parentCheck())
				}
				if e := check(); e != nil {
					return e
				}
				if e := fn(check); e != nil {
					return e
				}
				return check()
			})
		})
	}
}
func (o *OwnedTransferActions) publish(ctx context.Context, p *fightercarrier.PreparedCommit, e error) (operation fightercarrier.Operation, resultErr error) {
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer func() { resultErr = errors.Join(resultErr, p.Close()) }()
	return p.Publish(o.fence(ctx, gymaccount.FighterTransfer))
}
func (o *OwnedTransferActions) selected(ctx context.Context, prospect string) (fighterownership.OwnedItem, *fighterownership.HeadLease, error) {
	c, e := o.core.CaptureOwned(ctx, o.session.intent.Carrier.Pin.GymPublicKey)
	if e != nil {
		return fighterownership.OwnedItem{}, nil, e
	}
	v, e := o.core.OwnedItem(ctx, c, prospect)
	if e != nil {
		return v, nil, e
	}
	h, e := o.core.AcquireHead(ctx, c.Head())
	return v, h, e
}

// Prepare requires the native controller's already retained request ID, so a
// lost response cannot erase the correlation needed for exact Resume. The ID
// is never authority; the current owned capture supplies the predecessor.
func (o *OwnedTransferActions) Prepare(ctx context.Context, id, prospect, recipient string) (out OwnedTransferReview, resultErr error) {
	if !payoutNonzeroID(id) {
		return out, fightercarrier.ErrInvalid
	}
	out.RequestID = id
	ctx, done, e := o.begin(ctx)
	if e != nil {
		return out, e
	}
	defer done()
	o.clearReview()
	o.deliveryReview = nil
	if e = o.fence(ctx, gymaccount.FighterTransfer)(func(check func() error) error { return check() }); e != nil {
		return out, e
	}
	v, h, e := o.selected(ctx, prospect)
	if e != nil {
		return out, e
	}
	defer h.Close()
	req := fightercarrier.Request{Version: 1, RequestID: id, Kind: "transfer", OwnershipPinID: o.session.intent.OwnershipPinID, PriorStateID: v.State.StateID, RecipientGym: recipient}
	if e = fightercarrier.ValidateRequest(req); e != nil {
		return out, e
	}
	journal, e := o.session.CarrierJournal().TryAcquire(ctx)
	if e != nil {
		return out, e
	}
	p, e := journal.StageIntent(req)
	_, e = o.publish(ctx, p, e)
	e = errors.Join(e, journal.Close())
	if e != nil {
		return out, e
	}
	saved, e := o.session.Carrier().InspectPrepared(ctx, id)
	var prepared *wallet.PreparedFighterTransfer
	if errors.Is(e, fightercarrier.ErrNotFound) {
		prepared, e = o.prepareRetained(ctx, req, prospect, h, nil)
	} else if e == nil {
		prepared, e = o.prepareRetained(ctx, req, prospect, h, &saved)
	}
	if e != nil {
		return out, e
	}
	review, e := o.retainReview(ctx, prospect, prepared)
	if e != nil {
		return OwnedTransferReview{RequestID: id}, e
	}
	return review, nil
}

func (o *OwnedTransferActions) prepareRetained(ctx context.Context, req fightercarrier.Request, prospect string, h *fighterownership.HeadLease, saved *fightercarrier.Preparation) (*wallet.PreparedFighterTransfer, error) {
	w := o.session.Carrier()
	var p *wallet.PreparedFighterTransfer
	if saved != nil {
		if saved.NewFunding == nil || saved.Request != req {
			return nil, fightercarrier.ErrConflict
		}
		tuple := make(fighterownership.FundingTuple, len(saved.NewFunding.Inputs))
		for i, in := range saved.NewFunding.Inputs {
			tuple[i] = fighterownership.FundingOutpoint{TxID: in.TxID, Index: in.OutputIndex}
		}
		public, e := o.core.PrepareTransfer(ctx, h, prospect, o.session.intent.Carrier.Pin.GymPublicKey, req.RecipientGym, tuple)
		if e != nil {
			return nil, e
		}
		p, e = w.ResumePrepared(ctx, req.RequestID, public)
		if e != nil {
			return nil, e
		}
	} else {
		plan, e := w.PlanFunding(ctx, req)
		if e != nil {
			return nil, e
		}
		public, e := o.core.PrepareTransfer(ctx, h, prospect, o.session.intent.Carrier.Pin.GymPublicKey, req.RecipientGym, plan.Funding())
		if e != nil {
			return nil, e
		}
		p, e = w.Prepare(ctx, plan, public)
		if e != nil {
			return nil, e
		}
	}
	journal, e := o.session.CarrierJournal().TryAcquire(ctx)
	if e != nil {
		p.Close()
		return nil, e
	}
	preparation, e := p.Preparation()
	if e != nil {
		journal.Close()
		p.Close()
		return nil, e
	}
	commit, e := journal.StagePrepared(req.RequestID, preparation)
	_, e = o.publish(ctx, commit, e)
	e = errors.Join(e, journal.Close())
	if e != nil {
		p.Close()
		return nil, e
	}
	return p, nil
}
func (o *OwnedTransferActions) retainReview(ctx context.Context, prospect string, p *wallet.PreparedFighterTransfer) (OwnedTransferReview, error) {
	if e := p.Check(ctx); e != nil {
		p.Close()
		return OwnedTransferReview{}, e
	}
	id, e := ownedActionID()
	if e != nil {
		p.Close()
		return OwnedTransferReview{}, e
	}
	prep, e := p.Preparation()
	if e != nil {
		p.Close()
		return OwnedTransferReview{}, e
	}
	o.prepared = p
	o.review = OwnedTransferReview{RequestID: prep.Request.RequestID, ReviewID: id, ProspectID: prospect, Details: prep.Review}
	if e = o.fence(ctx, gymaccount.FighterTransfer)(func(check func() error) error { return check() }); e != nil {
		o.clearReview()
		return OwnedTransferReview{}, e
	}
	return o.review, nil
}

// Resume reconstructs eligibility from current public ownership and the exact
// wallet preparation. Caller input cannot supply predecessor or funding bytes.
func (o *OwnedTransferActions) Resume(ctx context.Context, id, prospect string) (OwnedTransferReview, error) {
	ctx, done, e := o.begin(ctx)
	if e != nil {
		return OwnedTransferReview{}, e
	}
	defer done()
	o.clearReview()
	o.deliveryReview = nil
	journal, e := o.session.CarrierJournal().TryAcquire(ctx)
	if e != nil {
		return OwnedTransferReview{}, e
	}
	operation, e := journal.Lookup(id)
	e = errors.Join(e, journal.Close())
	if e != nil {
		return OwnedTransferReview{}, e
	}
	if operation.Request.Kind != "transfer" || (operation.Stage != "intent" && operation.Stage != "prepared") {
		return OwnedTransferReview{}, fightercarrier.ErrConflict
	}
	v, h, e := o.selected(ctx, prospect)
	if e != nil {
		return OwnedTransferReview{}, e
	}
	defer h.Close()
	if v.State.StateID != operation.Request.PriorStateID {
		return OwnedTransferReview{}, fightercarrier.ErrConflict
	}
	saved, e := o.session.Carrier().InspectPrepared(ctx, id)
	var prepared *wallet.PreparedFighterTransfer
	if errors.Is(e, fightercarrier.ErrNotFound) && operation.Stage == "intent" {
		prepared, e = o.prepareRetained(ctx, operation.Request, prospect, h, nil)
	} else if e == nil {
		prepared, e = o.prepareRetained(ctx, operation.Request, prospect, h, &saved)
	}
	if e != nil {
		return OwnedTransferReview{}, e
	}
	return o.retainReview(ctx, prospect, prepared)
}

// Confirm signs and durably retains both legs and wallet reservations. Delivery
// is a separate original-view action; merely reopening never transmits bytes.
func (o *OwnedTransferActions) Confirm(ctx context.Context, reviewID string) (fightercarrier.Operation, error) {
	ctx, done, e := o.begin(ctx)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer done()
	if o.prepared == nil || o.review.ReviewID != reviewID {
		return fightercarrier.Operation{}, fightercarrier.ErrUnauthorized
	}
	p, review := o.prepared, o.review
	o.prepared = nil
	o.review = OwnedTransferReview{}
	defer p.Close()
	if e = p.Check(ctx); e != nil {
		return fightercarrier.Operation{}, e
	}
	envelope, e := o.session.setup.vault.SignFighterTransfer(ctx, o.session.login.Token, p, o.session.epoch)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	// This explicit fixture route owns the wallet under the session operation
	// permit. Unlock only for the reviewed funding signature, then relock.
	w := o.session.runtime.wallet
	if w.Locked() {
		if e = w.Unlock([]byte(fixtureWalletPrivatePass), nil); e != nil {
			return fightercarrier.Operation{}, e
		}
		defer w.Lock()
	}
	pair, e := o.session.Carrier().Sign(ctx, p, envelope)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	action, e := ownedActionID()
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	journal, e := o.session.CarrierJournal().TryAcquire(ctx)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer journal.Close()
	commit, e := journal.StageSigned(review.RequestID, pair, fightercarrier.AuthorizationInput{ActionID: action, ReviewID: review.ReviewID, ViewID: o.viewID, Generation: o.session.login.Account.Generation})
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	// Staging can take time. Recheck concrete ownership and funding outside
	// account leases immediately before publishing the retained signed pair.
	if e = p.Check(ctx); e != nil {
		return fightercarrier.Operation{}, errors.Join(e, commit.Close())
	}
	operation, e := o.publish(ctx, commit, e)
	if e != nil {
		o.uncertain = true
		return fightercarrier.Operation{}, errors.Join(fightercarrier.ErrOutcomeUnknown, e)
	}
	postPublication := p.Check(ctx)
	proof, e := journal.Reconciliation(review.RequestID)
	if e != nil {
		o.uncertain = true
		return fightercarrier.Operation{}, errors.Join(fightercarrier.ErrOutcomeUnknown, postPublication, e)
	}
	e = o.session.Carrier().Reconcile(ctx, proof)
	if postPublication != nil || e != nil {
		o.uncertain = true
		return fightercarrier.Operation{}, errors.Join(fightercarrier.ErrOutcomeUnknown, postPublication, e)
	}
	return operation, nil
}
