// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
)

// A dated read of the completed public replay. Delivery completion never
// supplies this status. A later reported conflict does not undo finality.
type OwnedTransferFinality struct {
	Version               uint32                   `json:"version"`
	RequestID             string                   `json:"request_id"`
	TransferID            string                   `json:"transfer_id"`
	State                 string                   `json:"state"`
	Head                  fighterownership.HeadRef `json:"head"`
	EffectiveStateID      string                   `json:"effective_state_id"`
	OwnerGym              string                   `json:"owner_gym"`
	PublicEvidencePending bool                     `json:"public_evidence_pending"`
	ConflictID            string                   `json:"conflict_id"`
}

func ValidateOwnedTransferFinality(v OwnedTransferFinality, pin fighterownership.Pin) error {
	if v.Version != 1 || !payoutNonzeroID(v.RequestID) || fighterownership.ValidateHead(pin, v.Head) != nil || v.ConflictID != v.Head.ConflictID {
		return fightercarrier.ErrInvalid
	}
	if v.State == "unsigned" {
		if v.TransferID != "" {
			return fightercarrier.ErrInvalid
		}
	} else if !payoutNonzeroID(v.TransferID) {
		return fightercarrier.ErrInvalid
	}
	switch v.State {
	case "unsigned", "not_observed", "pending", "void":
		if v.EffectiveStateID != "" || v.OwnerGym != "" || v.PublicEvidencePending {
			return fightercarrier.ErrInvalid
		}
	case "finalized":
		if !payoutNonzeroID(v.EffectiveStateID) {
			return fightercarrier.ErrInvalid
		}
		if _, e := hexPublicKey(v.OwnerGym); e != nil {
			return fightercarrier.ErrInvalid
		}
	default:
		return fightercarrier.ErrInvalid
	}
	return nil
}
func (o *OwnedTransferActions) Finality(ctx context.Context, id string) (out OwnedTransferFinality, resultErr error) {
	ctx, done, e := o.begin(ctx)
	if e != nil {
		return out, e
	}
	defer done()
	journal, e := o.session.CarrierJournal().TryAcquire(ctx)
	if e != nil {
		return out, e
	}
	defer func() { resultErr = errors.Join(resultErr, journal.Close()) }()
	op, e := journal.Lookup(id)
	if e != nil {
		return out, e
	}
	head, e := o.core.Head(ctx)
	if e != nil {
		return out, e
	}
	lease, e := o.core.AcquireHead(ctx, head)
	if e != nil {
		return out, e
	}
	defer lease.Close()
	out = OwnedTransferFinality{Version: 1, RequestID: id, State: "unsigned", Head: head, ConflictID: head.ConflictID}
	if op.SignedPackage != nil {
		out.TransferID = op.SignedPackage.TransferID
		status, e := o.core.TransferStatus(ctx, out.TransferID)
		if errors.Is(e, fighterownership.ErrNotFound) {
			out.State = "not_observed"
		} else if e != nil {
			return OwnedTransferFinality{}, e
		} else {
			if status.TransferID != out.TransferID || status.ConflictID != head.ConflictID {
				return OwnedTransferFinality{}, fighterownership.ErrStale
			}
			out.State = status.State
			if status.EffectiveState != nil {
				if status.State != "finalized" || status.EffectiveState.OwnerPublicKey != op.Request.RecipientGym {
					return OwnedTransferFinality{}, fighterownership.ErrCorrupt
				}
				out.EffectiveStateID = status.EffectiveState.StateID
				out.OwnerGym = status.EffectiveState.OwnerPublicKey
				out.PublicEvidencePending = status.PublicEvidencePending
			}
		}
	}
	if e = lease.Check(); e != nil {
		return OwnedTransferFinality{}, e
	}
	if e = o.fence(ctx, gymaccount.View)(func(check func() error) error {
		if e := lease.Check(); e != nil {
			return e
		}
		return check()
	}); e != nil {
		return OwnedTransferFinality{}, e
	}
	if e = ValidateOwnedTransferFinality(out, o.session.setup.config.Ownership); e != nil {
		return OwnedTransferFinality{}, e
	}
	return out, nil
}
