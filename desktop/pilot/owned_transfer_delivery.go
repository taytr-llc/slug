// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/walletwithdraw"
)

// OwnedTransferDeliveryReview describes one already signed leg. Only the
// private retained instance authorizes delivery, once, in its original view.
type OwnedTransferDeliveryReview struct {
	RequestID   string                `json:"request_id"`
	ReviewID    string                `json:"review_id"`
	Leg         string                `json:"leg"`
	TxID        string                `json:"txid"`
	RawSHA256   string                `json:"raw_sha256"`
	TransferID  string                `json:"transfer_id"`
	ExpiresAtMS int64                 `json:"expires_at_ms"`
	Details     fightercarrier.Review `json:"details"`
}

func ownedCarrierLeg(v fightercarrier.Operation, leg string) (fightercarrier.RawTransaction, error) {
	if v.SignedPackage == nil || v.Preparation == nil {
		return fightercarrier.RawTransaction{}, fightercarrier.ErrConflict
	}
	switch leg {
	case "commit":
		if v.Request.Kind == "transfer" && (v.Stage == "signed" || v.Stage == "commit_dispatching") {
			return v.SignedPackage.Commit, nil
		}
	case "spend":
		if v.Stage == "commit_observed" || v.Stage == "spend_dispatching" || v.Request.Kind != "transfer" && v.Stage == "signed" {
			return v.SignedPackage.Spend, nil
		}
	}
	return fightercarrier.RawTransaction{}, fightercarrier.ErrConflict
}

func (o *OwnedTransferActions) publishFor(ctx context.Context, p *fightercarrier.PreparedCommit, e error, capability gymaccount.Capability) (v fightercarrier.Operation, resultErr error) {
	if e != nil {
		return v, e
	}
	defer func() { resultErr = errors.Join(resultErr, p.Close()) }()
	return p.Publish(o.fence(ctx, capability))
}

func (o *OwnedTransferActions) retained(ctx context.Context, owner *fightercarrier.Owner, id string) error {
	proof, e := owner.Reconciliation(id)
	if e == nil {
		e = o.session.Carrier().Reconcile(ctx, proof)
	}
	if e != nil {
		o.uncertain = true
		return errors.Join(fightercarrier.ErrOutcomeUnknown, e)
	}
	return nil
}

// Operation reads the exact request the native controller retained before
// dispatch. It never chooses an unrelated last operation or transmits anything.
func (o *OwnedTransferActions) Operation(ctx context.Context, id string) (fightercarrier.Operation, error) {
	ctx, done, e := o.begin(ctx)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer done()
	j, e := o.session.CarrierJournal().TryAcquire(ctx)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer j.Close()
	v, e := j.Lookup(id)
	if e == nil {
		e = o.fence(ctx, gymaccount.View)(func(check func() error) error { return check() })
	}
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	return v, nil
}

func (o *OwnedTransferActions) ReviewDelivery(ctx context.Context, id, leg string) (OwnedTransferDeliveryReview, error) {
	ctx, done, e := o.begin(ctx)
	if e != nil {
		return OwnedTransferDeliveryReview{}, e
	}
	defer done()
	o.clearReview()
	o.deliveryReview = nil
	if e = o.fence(ctx, gymaccount.FighterTransfer)(func(check func() error) error { return check() }); e != nil {
		return OwnedTransferDeliveryReview{}, e
	}
	j, e := o.session.CarrierJournal().TryAcquire(ctx)
	if e != nil {
		return OwnedTransferDeliveryReview{}, e
	}
	defer j.Close()
	v, e := j.Lookup(id)
	if e != nil {
		return OwnedTransferDeliveryReview{}, e
	}
	raw, e := ownedCarrierLeg(v, leg)
	if e != nil {
		return OwnedTransferDeliveryReview{}, e
	}
	if e = o.retained(ctx, j, id); e != nil {
		return OwnedTransferDeliveryReview{}, e
	}
	reviewID, e := ownedActionID()
	if e != nil {
		return OwnedTransferDeliveryReview{}, e
	}
	r := OwnedTransferDeliveryReview{RequestID: id, ReviewID: reviewID, Leg: leg, TxID: raw.TxID, RawSHA256: raw.RawSHA256, TransferID: v.SignedPackage.TransferID, ExpiresAtMS: time.Now().Add(time.Minute).UnixMilli(), Details: v.Preparation.Review}
	if e = o.fence(ctx, gymaccount.FighterTransfer)(func(check func() error) error { return check() }); e != nil {
		return OwnedTransferDeliveryReview{}, e
	}
	o.deliveryReview = &r
	return r, nil
}

// Deliver publishes a fresh dispatch before taking the exact one-use bytes.
// A retry observes those same bytes first and never creates a replacement.
func (o *OwnedTransferActions) Deliver(ctx context.Context, reviewID string) (fightercarrier.Operation, error) {
	ctx, done, e := o.begin(ctx)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer done()
	r := o.deliveryReview
	o.deliveryReview = nil
	if r == nil || r.ReviewID != reviewID || time.Now().UnixMilli() >= r.ExpiresAtMS || time.Now().UnixMilli() < r.ExpiresAtMS-60000 {
		return fightercarrier.Operation{}, fightercarrier.ErrUnauthorized
	}
	j, e := o.session.CarrierJournal().TryAcquire(ctx)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer j.Close()
	v, e := j.Lookup(r.RequestID)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	raw, e := ownedCarrierLeg(v, r.Leg)
	if e != nil || raw.TxID != r.TxID || raw.RawSHA256 != r.RawSHA256 || v.SignedPackage.TransferID != r.TransferID {
		return fightercarrier.Operation{}, fightercarrier.ErrConflict
	}
	if e = o.retained(ctx, j, r.RequestID); e != nil {
		return fightercarrier.Operation{}, e
	}
	// Never send the reveal based only on a saved commit observation.
	if r.Leg == "spend" && v.Request.Kind == "transfer" {
		commit, x := o.observeCarrier(ctx, v.SignedPackage.Commit, v.SignedPackage.TransferID)
		if x != nil || commit.State != "mempool" && commit.State != "canonical" {
			return fightercarrier.Operation{}, errors.Join(fighterownership.ErrUnavailable, x)
		}
	}
	observed, e := o.observeCarrier(ctx, raw, v.SignedPackage.TransferID)
	if e != nil || observed.State == "unknown" || observed.State == "conflict" {
		return fightercarrier.Operation{}, errors.Join(fighterownership.ErrUnavailable, e)
	}
	action, e := ownedActionID()
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	p, e := j.StageDispatch(r.RequestID, r.Leg, fightercarrier.AuthorizationInput{ActionID: action, ReviewID: r.ReviewID, ViewID: o.viewID, Generation: o.session.login.Account.Generation})
	v, e = o.publish(ctx, p, e)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	dispatch := v.CommitDispatch
	if r.Leg == "spend" {
		dispatch = v.SpendDispatch
	}
	result, code := "unknown", ""
	if observed.State == "not_observed" {
		d, x := j.TakeDelivery(r.RequestID, r.Leg)
		if x != nil {
			return fightercarrier.Operation{}, x
		}
		// Account and parent lifetime are rechecked after durable dispatch;
		// network work never holds the account publication lease.
		if e = o.fence(ctx, gymaccount.FighterTransfer)(func(check func() error) error { return check() }); e != nil {
			return fightercarrier.Operation{}, e
		}
		b, txid, attempt, x := d.Take()
		if x != nil || txid != raw.TxID || attempt != dispatch.Attempt {
			return fightercarrier.Operation{}, fightercarrier.ErrConflict
		}
		outcome, reason := o.node.deliver(ctx, b, txid)
		switch outcome {
		case walletwithdraw.DeliveryAcknowledged:
			result = "acknowledged"
		case walletwithdraw.DeliveryRejected:
			result, code = "rejected", "rejected"
		default:
			code = "unavailable"
		}
		_ = reason // Node error strings never enter the closed carrier contract.
	}
	p, e = j.StageDeliveryResult(r.RequestID, r.Leg, dispatch.Attempt, result, code)
	_, e = o.publishFor(ctx, p, e, gymaccount.View)
	if e != nil {
		return fightercarrier.Operation{}, errors.Join(fightercarrier.ErrOutcomeUnknown, e)
	}
	return o.observeAndRetain(ctx, j, r.RequestID, r.Leg)
}

// Observe is read-authorized, including during a reset hold. It records an
// exact node observation for unresolved delivery but cannot create a dispatch
// ticket or send bytes. Completed delivery returns its immutable saved receipt;
// fresh ownership is obtained separately through Finality.
func (o *OwnedTransferActions) Observe(ctx context.Context, id, leg string) (fightercarrier.Operation, error) {
	ctx, done, e := o.begin(ctx)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer done()
	j, e := o.session.CarrierJournal().TryAcquire(ctx)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer j.Close()
	return o.observeAndRetain(ctx, j, id, leg)
}

func (o *OwnedTransferActions) observeAndRetain(ctx context.Context, j *fightercarrier.Owner, id, leg string) (fightercarrier.Operation, error) {
	v, e := j.Lookup(id)
	if e != nil || v.SignedPackage == nil {
		return fightercarrier.Operation{}, errors.Join(fightercarrier.ErrConflict, e)
	}
	var raw fightercarrier.RawTransaction
	switch leg {
	case "commit":
		if v.CommitDispatch == nil {
			return fightercarrier.Operation{}, fightercarrier.ErrConflict
		}
		raw = v.SignedPackage.Commit
	case "spend":
		if v.SpendDispatch == nil {
			return fightercarrier.Operation{}, fightercarrier.ErrConflict
		}
		raw = v.SignedPackage.Spend
	default:
		return fightercarrier.Operation{}, fightercarrier.ErrInvalid
	}
	if v.Stage == "complete" {
		// Completion already retained the exact delivered spend. Re-observation
		// must not rewrite its dated receipt, reopen delivery, or reconcile a
		// wallet mutation. Lookup owns this returned copy; only View discloses it.
		if e = o.fence(ctx, gymaccount.View)(func(check func() error) error { return check() }); e != nil {
			return fightercarrier.Operation{}, e
		}
		return v, nil
	}
	observation, observationErr := o.observeCarrier(ctx, raw, v.SignedPackage.TransferID)
	if observationErr != nil {
		observation = fightercarrier.Observation{State: "unknown", ExpectedTxID: raw.TxID, Error: "unavailable"}
	}
	p, e := j.StageObservation(id, leg, observation)
	v, e = o.publishFor(ctx, p, e, gymaccount.View)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	if e = o.retained(ctx, j, id); e != nil {
		return fightercarrier.Operation{}, e
	}
	if e = o.fence(ctx, gymaccount.View)(func(check func() error) error { return check() }); e != nil {
		return fightercarrier.Operation{}, e
	}
	return v, observationErr
}

func (o *OwnedTransferActions) AbandonUnsigned(ctx context.Context, id string) (fightercarrier.Operation, error) {
	ctx, done, e := o.begin(ctx)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer done()
	o.clearReview()
	o.deliveryReview = nil
	j, e := o.session.CarrierJournal().TryAcquire(ctx)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	defer func() {
		if j != nil {
			_ = j.Close()
		}
	}()
	v, e := j.Lookup(id)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	if v.Stage == "intent" {
		// Preparation may have committed in the wallet before its app reply.
		// Bind that exact original row before releasing its reservations.
		saved, x := o.session.Carrier().InspectPrepared(ctx, id)
		if x == nil {
			p, x := j.StagePrepared(id, saved)
			v, x = o.publish(ctx, p, x)
			if x != nil {
				return fightercarrier.Operation{}, x
			}
		} else if errors.Is(x, fightercarrier.ErrNotFound) {
			// Reserve may have committed before wallet consumption. The original
			// concrete carrier audits that exact intent and burns only an unlinked
			// range; uncertain/local-consumed work must never be released here.
			if x = j.Close(); x != nil {
				return fightercarrier.Operation{}, x
			}
			j = nil
			if x = o.session.Carrier().AbandonUnprepared(ctx, id); x != nil {
				if errors.Is(x, fightercarrier.ErrOutcomeUnknown) {
					o.uncertain = true
				}
				return fightercarrier.Operation{}, x
			}
			j, x = o.session.CarrierJournal().TryAcquire(ctx)
			if x != nil {
				return fightercarrier.Operation{}, x
			}
			current, x := j.Lookup(id)
			if x != nil || current.Request != v.Request || current.Stage != "intent" {
				return fightercarrier.Operation{}, errors.Join(fightercarrier.ErrConflict, x)
			}
		} else {
			return fightercarrier.Operation{}, x
		}
	}
	p, e := j.StageAbandon(id)
	v, e = o.publish(ctx, p, e)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	if v.Preparation != nil {
		e = o.retained(ctx, j, id)
	}
	return v, e
}

// observeCarrier uses only the original runtime's concrete node endpoints.
// The full instance/revision observation brackets raw bytes and canonical
// inclusion, so a restart or same-tip ABA cannot supply a current observation.
func (o *OwnedTransferActions) observeCarrier(ctx context.Context, expected fightercarrier.RawTransaction, transferID string) (fightercarrier.Observation, error) {
	return observeOwnedCarrier(ctx, o.chain, o.node, expected, transferID)
}

func observeOwnedCarrier(ctx context.Context, chain *fighterownership.OwnedChainSource, node *withdrawalNode, expected fightercarrier.RawTransaction, transferID string) (fightercarrier.Observation, error) {
	unknown := fightercarrier.Observation{State: "unknown", ExpectedTxID: expected.TxID, Error: "unavailable"}
	if chain == nil || node == nil {
		return unknown, fighterownership.ErrUnavailable
	}
	before, e := chain.Observe(ctx)
	if e != nil {
		return unknown, e
	}
	var raw string
	e = node.read(ctx, "getrawtransaction", []any{expected.TxID, 0}, &raw)
	if e != nil {
		var refusal *withdrawalRPCError
		if errors.As(e, &refusal) && refusal.Code == -5 && refusal.Message == "No information available about transaction "+expected.TxID && chain.Check(ctx, before) == nil {
			return fightercarrier.Observation{State: "not_observed", ExpectedTxID: expected.TxID, NodeTip: before.Hash, NodeHeight: int32(before.Height), Error: "not_observed"}, nil
		}
		return unknown, e
	}
	decoded, e := hex.DecodeString(raw)
	if e != nil || raw != expected.Raw {
		return unknown, fightercarrier.ErrConflict
	}
	digest := sha256.Sum256(decoded)
	if hex.EncodeToString(digest[:]) != expected.RawSHA256 {
		return unknown, fightercarrier.ErrConflict
	}
	var verbose struct {
		Hex           string `json:"hex"`
		TxID          string `json:"txid"`
		BlockHash     string `json:"blockhash"`
		Confirmations uint32 `json:"confirmations"`
	}
	data, e := node.call(ctx, "getrawtransaction", []any{expected.TxID, 1})
	if e != nil {
		return unknown, e
	}
	fields, e := withdrawalFields(data, "hex", "txid")
	if e != nil || json.Unmarshal(data, &verbose) != nil || verbose.Hex != raw || verbose.TxID != expected.TxID {
		return unknown, fighterownership.ErrUnavailable
	}
	if _, ok := fields["blockhash"]; ok && withdrawalField(fields, "blockhash", &verbose.BlockHash) != nil {
		return unknown, fighterownership.ErrUnavailable
	}
	if _, ok := fields["confirmations"]; ok && withdrawalField(fields, "confirmations", &verbose.Confirmations) != nil {
		return unknown, fighterownership.ErrUnavailable
	}
	result := fightercarrier.Observation{State: "mempool", ExpectedTxID: expected.TxID, ObservedTxID: expected.TxID, NodeTip: before.Hash, NodeHeight: int32(before.Height), TransferID: transferID, RawSHA256: expected.RawSHA256}
	if verbose.BlockHash == "" {
		if verbose.Confirmations != 0 {
			return unknown, fighterownership.ErrUnavailable
		}
		entry, x := node.call(ctx, "getmempoolentry", []any{expected.TxID})
		if x != nil {
			return unknown, x
		}
		if x = validateOwnedMempoolEntry(ctx, entry, decoded, expected.TxID); x != nil {
			return unknown, x
		}
	} else {
		if !withdrawalHash(verbose.BlockHash) {
			return unknown, fighterownership.ErrUnavailable
		}
		var header struct {
			Height uint32 `json:"height"`
			Hash   string `json:"hash"`
		}
		if e = node.read(ctx, "getblockheader", []any{verbose.BlockHash, true}, &header, "height", "hash"); e != nil || header.Height > math.MaxInt32 || header.Hash != verbose.BlockHash {
			return unknown, fighterownership.ErrUnavailable
		}
		result.BlockHash, result.BlockHeight = verbose.BlockHash, int32(header.Height)
		if header.Height > before.Height {
			return unknown, fighterownership.ErrUnavailable
		}
		canonical, x := chain.HeaderAt(ctx, header.Height)
		if x != nil {
			return unknown, x
		}
		if canonical.Hash != verbose.BlockHash {
			return unknown, fightercarrier.ErrConflict
		}
		b, x := chain.Block(ctx, verbose.BlockHash)
		if x != nil {
			return unknown, x
		}
		position, x := fighterownership.VerifyBlockTransaction(ctx, b, canonical, expected.TxID, decoded)
		if x != nil {
			return unknown, x
		}
		result.TransactionIndex = position
		result.State, result.Confirmations = "canonical", before.Height-header.Height+1
		if result.Confirmations != verbose.Confirmations {
			return unknown, fighterownership.ErrUnavailable
		}
	}
	if e = chain.Check(ctx, before); e != nil {
		return unknown, e
	}
	return result, nil
}
