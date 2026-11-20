// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/walletwithdraw"
)

type RestoredDeliveryReview = OwnedTransferDeliveryReview

// This separate constructor accepts exactly the already-pinned restored HTTP
// literal-loopback profile (including IPv6); the old Pool constructor is closed.
func (s *RestoredGymSession) connectDelivery() error {
	if s.deliveryNode != nil {
		return nil
	}
	if e := restoredValidateTransport(s.initial.PublicRead, s.initial.NodeBinding); e != nil {
		return e
	}
	if s.nodeConfig.URL != s.initial.NodeBinding.URL || s.nodeConfig.CACertificate != "" {
		return ErrRestoredGym
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 8192, DisableCompression: true, DisableKeepAlives: true}
	s.deliveryNode = &withdrawalNode{config: s.nodeConfig, http: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	return nil
}
func (s *RestoredGymSession) observeAction(ctx context.Context, j *fightercarrier.Owner, id, leg string) (fightercarrier.Operation, error) {
	v, e := j.Lookup(id)
	if e != nil || v.SignedPackage == nil {
		return fightercarrier.Operation{}, errors.Join(fightercarrier.ErrConflict, e)
	}
	var raw fightercarrier.RawTransaction
	switch leg {
	case "commit":
		if v.CommitDispatch == nil {
			return v, fightercarrier.ErrConflict
		}
		raw = v.SignedPackage.Commit
	case "spend":
		if v.SpendDispatch == nil {
			return v, fightercarrier.ErrConflict
		}
		raw = v.SignedPackage.Spend
	default:
		return v, fightercarrier.ErrInvalid
	}
	if v.Stage == "complete" {
		// The exact completed receipt is historical delivery evidence, not a
		// newly sampled node observation. Do not rewrite or reconcile it.
		if e = s.disclose(ctx); e != nil {
			return fightercarrier.Operation{}, e
		}
		return v, nil
	}
	observed, observationErr := observeOwnedCarrier(ctx, s.node, s.deliveryNode, raw, v.SignedPackage.TransferID)
	if observationErr != nil {
		observed = fightercarrier.Observation{State: "unknown", ExpectedTxID: raw.TxID, Error: "unavailable"}
	}
	p, e := j.StageObservation(id, leg, observed)
	v, e = s.publishAction(ctx, p, e)
	if e != nil {
		return v, e
	}
	if e = s.reconcileAction(ctx, j, id); e != nil {
		return v, e
	}
	return v, errors.Join(observationErr, s.disclose(ctx))
}
func (s *RestoredGymSession) ReviewDelivery(ctx context.Context, id, leg string) (RestoredDeliveryReview, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredDeliveryReview{}, e
	}
	defer end()
	if e = s.clearActions(); e != nil {
		return RestoredDeliveryReview{}, e
	}
	if e = s.actionReady(work); e != nil {
		return RestoredDeliveryReview{}, e
	}
	j, e := s.journal.TryAcquire(work)
	if e != nil {
		return RestoredDeliveryReview{}, e
	}
	defer j.Close()
	v, e := j.Lookup(id)
	if e != nil {
		return RestoredDeliveryReview{}, e
	}
	raw, e := ownedCarrierLeg(v, leg)
	if e != nil {
		return RestoredDeliveryReview{}, e
	}
	if e = s.reconcileAction(work, j, id); e != nil {
		return RestoredDeliveryReview{}, e
	}
	reviewID, e := restoredID()
	if e != nil {
		return RestoredDeliveryReview{}, e
	}
	r := RestoredDeliveryReview{RequestID: id, ReviewID: reviewID, Leg: leg, TxID: raw.TxID, RawSHA256: raw.RawSHA256, TransferID: v.SignedPackage.TransferID, ExpiresAtMS: time.Now().Add(time.Minute).UnixMilli(), Details: v.Preparation.Review}
	if e = s.disclose(work); e != nil {
		return RestoredDeliveryReview{}, e
	}
	s.deliveryReview = &r
	return r, nil
}
func (s *RestoredGymSession) Deliver(ctx context.Context, reviewID string) (RestoredActionStatus, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	defer end()
	r := s.deliveryReview
	s.deliveryReview = nil
	now := time.Now().UnixMilli()
	if r == nil || r.ReviewID != reviewID || now >= r.ExpiresAtMS || now < r.ExpiresAtMS-60000 {
		return RestoredActionStatus{}, fightercarrier.ErrUnauthorized
	}
	if e = s.actionReady(work); e != nil {
		return RestoredActionStatus{}, e
	}
	if e = s.connectDelivery(); e != nil {
		return RestoredActionStatus{}, e
	}
	j, e := s.journal.TryAcquire(work)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	defer j.Close()
	v, e := j.Lookup(r.RequestID)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	raw, e := ownedCarrierLeg(v, r.Leg)
	if e != nil || raw.TxID != r.TxID || raw.RawSHA256 != r.RawSHA256 || v.SignedPackage.TransferID != r.TransferID {
		return RestoredActionStatus{}, fightercarrier.ErrConflict
	}
	if e = s.reconcileAction(work, j, r.RequestID); e != nil {
		return RestoredActionStatus{}, e
	}
	if r.Leg == "spend" && v.Request.Kind == "transfer" {
		commit, e := observeOwnedCarrier(work, s.node, s.deliveryNode, v.SignedPackage.Commit, v.SignedPackage.TransferID)
		if e != nil || commit.State != "mempool" && commit.State != "canonical" {
			return RestoredActionStatus{}, errors.Join(fighterownership.ErrUnavailable, e)
		}
	}
	observed, e := observeOwnedCarrier(work, s.node, s.deliveryNode, raw, v.SignedPackage.TransferID)
	if e != nil || observed.State == "unknown" || observed.State == "conflict" {
		return RestoredActionStatus{}, errors.Join(fighterownership.ErrUnavailable, e)
	}
	action, e := restoredID()
	if e != nil {
		return RestoredActionStatus{}, e
	}
	staged, e := j.StageDispatch(r.RequestID, r.Leg, fightercarrier.AuthorizationInput{ActionID: action, ReviewID: r.ReviewID, ViewID: s.actionViewID, Generation: 0})
	v, e = s.publishAction(work, staged, e)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	dispatch := v.CommitDispatch
	if r.Leg == "spend" {
		dispatch = v.SpendDispatch
	}
	result, code := "unknown", ""
	if observed.State == "not_observed" {
		ticket, e := j.TakeDelivery(r.RequestID, r.Leg)
		if e != nil {
			return RestoredActionStatus{}, e
		}
		if e = s.disclose(work); e != nil {
			return RestoredActionStatus{}, e
		}
		bytes, txid, attempt, e := ticket.Take()
		if e != nil || txid != raw.TxID || attempt != dispatch.Attempt {
			return RestoredActionStatus{}, errors.Join(fightercarrier.ErrConflict, e)
		}
		outcome, _ := s.deliveryNode.deliver(work, bytes, txid)
		switch outcome {
		case walletwithdraw.DeliveryAcknowledged:
			result = "acknowledged"
		case walletwithdraw.DeliveryRejected:
			result, code = "rejected", "rejected"
		default:
			code = "unavailable"
		}
	}
	staged, e = j.StageDeliveryResult(r.RequestID, r.Leg, dispatch.Attempt, result, code)
	if _, e = s.publishAction(work, staged, e); e != nil {
		s.poisoned.Store(true)
		return RestoredActionStatus{}, errors.Join(fightercarrier.ErrOutcomeUnknown, e)
	}
	v, e = s.observeAction(work, j, r.RequestID, r.Leg)
	return restoredActionStatus(v), e
}

// ObserveAction returns the saved receipt for completed delivery. Unresolved
// delivery may record a fresh node observation; public replay owns finality.
func (s *RestoredGymSession) ObserveAction(ctx context.Context, id, leg string) (RestoredActionStatus, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	defer end()
	// Read terminal state before connecting any network or wallet backend.
	// Release this short read owner before the existing nonterminal connect
	// path, preserving its lock order and rechecking the exact row afterward.
	if s.journal == nil {
		return RestoredActionStatus{}, ErrRestoredGym
	}
	retained, e := s.journal.TryAcquire(work)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	old, readErr := retained.Lookup(id)
	if readErr == nil && old.Stage == "complete" {
		v, readErr := s.observeAction(work, retained, id, leg)
		return restoredActionStatus(v), errors.Join(readErr, retained.Close())
	}
	if e = errors.Join(readErr, retained.Close()); e != nil {
		return RestoredActionStatus{}, e
	}
	if e = s.connect(work); e != nil {
		s.rememberCleanup(e)
		return RestoredActionStatus{}, e
	}
	if e = s.connectDelivery(); e != nil {
		return RestoredActionStatus{}, e
	}
	j, e := s.journal.TryAcquire(work)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	defer j.Close()
	v, e := s.observeAction(work, j, id, leg)
	return restoredActionStatus(v), e
}
func (s *RestoredGymSession) AbandonAction(ctx context.Context, id string) (RestoredActionStatus, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	defer end()
	if s.carrier == nil {
		return RestoredActionStatus{}, ErrRestoredGym
	}
	if e = s.clearActions(); e != nil {
		return RestoredActionStatus{}, e
	}
	j, e := s.journal.TryAcquire(work)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	defer func() {
		if j != nil {
			_ = j.Close()
		}
	}()
	v, e := j.Lookup(id)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	if v.Stage == "intent" {
		saved, x := s.carrier.InspectPrepared(work, id)
		if x == nil {
			p, x := j.StagePrepared(id, saved)
			v, x = s.publishAction(work, p, x)
			if x != nil {
				return RestoredActionStatus{}, x
			}
		} else if errors.Is(x, fightercarrier.ErrNotFound) {
			if e = j.Close(); e != nil {
				return RestoredActionStatus{}, e
			}
			j = nil
			if e = s.carrier.AbandonUnprepared(work, id); e != nil {
				return RestoredActionStatus{}, e
			}
			j, e = s.journal.TryAcquire(work)
			if e != nil {
				return RestoredActionStatus{}, e
			}
			current, e := j.Lookup(id)
			if e != nil || current.Request != v.Request || current.Stage != "intent" {
				return RestoredActionStatus{}, errors.Join(fightercarrier.ErrConflict, e)
			}
		} else {
			return RestoredActionStatus{}, x
		}
	}
	p, e := j.StageAbandon(id)
	v, e = s.publishAction(work, p, e)
	if e != nil {
		return RestoredActionStatus{}, e
	}
	if v.Preparation != nil {
		e = s.reconcileAction(work, j, id)
	}
	return restoredActionStatus(v), e
}
