// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

func authorizationValid(a Authorization, mode string, from, to int64) bool {
	if a.Mode != mode || !hexValue(a.ActionID, 32, true) || !hexValue(a.ReviewID, 32, true) || !hexValue(a.ViewID, 32, true) || !journalTime(a.AuthorizedAtMS) || a.AuthorizedAtMS < from || a.AuthorizedAtMS > to {
		return false
	}
	return mode == "hosted" && a.Generation > 0 && a.Generation <= 9007199254740991 || mode == "restored" && a.Generation == 0
}
func dispatchValid(d Dispatch, mode string, from, to int64) bool {
	if d.Attempt == 0 || !authorizationValid(d.Authorization, mode, from, to) || d.IntentAtMS < d.Authorization.AuthorizedAtMS || d.IntentAtMS > to {
		return false
	}
	if d.Result == "pending" {
		return d.ResultAtMS == 0 && d.Error == ""
	}
	if d.Result != "acknowledged" && d.Result != "unknown" && d.Result != "rejected" || d.ResultAtMS < d.IntentAtMS || d.ResultAtMS > to || !journalTime(d.ResultAtMS) {
		return false
	}
	switch d.Error {
	case "", "unavailable", "not_observed", "rejected", "cancelled", "outcome_unknown":
		return true
	}
	return false
}
func observationValid(o Observation, txid string, from, to int64) bool {
	if !journalTime(o.ObservedAtMS) || o.ObservedAtMS < from || o.ObservedAtMS > to || o.ExpectedTxID != txid || o.NodeHeight < 0 || o.BlockHeight < 0 {
		return false
	}
	switch o.Error {
	case "", "unavailable", "not_observed", "conflict":
	default:
		return false
	}
	if o.State == "unknown" {
		return o.ObservedTxID == "" && o.NodeTip == "" && o.NodeHeight == 0 && o.BlockHash == "" && o.BlockHeight == 0 && o.TransactionIndex == 0 && o.Confirmations == 0 && o.TransferID == "" && o.RawSHA256 == ""
	}
	if !hexValue(o.NodeTip, 32, true) {
		return false
	}
	if o.State == "not_observed" {
		if o.ObservedTxID != "" || o.TransferID != "" || o.RawSHA256 != "" {
			return false
		}
	} else if !hexValue(o.ObservedTxID, 32, false) || !hexValue(o.TransferID, 32, false) || !hexValue(o.RawSHA256, 32, false) {
		return false
	}
	switch o.State {
	case "not_observed", "mempool":
		return o.BlockHash == "" && o.BlockHeight == 0 && o.TransactionIndex == 0 && o.Confirmations == 0
	case "canonical":
		return hexValue(o.BlockHash, 32, true) && o.NodeHeight >= o.BlockHeight && uint64(o.Confirmations) == uint64(o.NodeHeight)-uint64(o.BlockHeight)+1
	case "conflict":
		return hexValue(o.BlockHash, 32, true) && o.Confirmations == 0
	}
	return false
}
func observed(o *Observation) bool {
	return o != nil && (o.State == "mempool" || o.State == "canonical")
}

// ValidateOperation validates bounded supplied private state. It does not
// replace original account authorization, wallet reconciliation, chain replay,
// native review, or a concrete retained ownership capability.
func ValidateOperation(o Operation, pin Pin, binding string) error {
	if ValidatePin(pin) != nil || !hexValue(binding, 32, false) || o.Version != 1 || o.Sequence < 1 || o.Sequence > MaxOperations || o.Revision == 0 || !journalTime(o.CreatedAtMS) || !journalTime(o.UpdatedAtMS) || o.CreatedAtMS > o.UpdatedAtMS || o.RequestID != o.Request.RequestID || o.Request.OwnershipPinID != pin.OwnershipPinID || o.Request.RecipientGym == pin.GymPublicKey {
		return ErrInvalid
	}
	d, e := RequestDigest(o.Request)
	if e != nil || d != o.RequestDigest {
		return ErrInvalid
	}
	switch o.Stage {
	case "intent", "prepared", "signed", "commit_dispatching", "commit_observed", "spend_dispatching", "complete", "abandoned":
	default:
		return ErrInvalid
	}
	if o.Preparation == nil {
		if o.PreparationDigest != "" || o.Stage != "intent" && o.Stage != "abandoned" {
			return ErrInvalid
		}
	} else {
		if o.Stage == "intent" || ValidatePreparation(*o.Preparation, pin, binding) != nil || !sameValue(o.Preparation.Request, o.Request) || o.Preparation.PreparedAtMS < o.CreatedAtMS || o.Preparation.PreparedAtMS > o.UpdatedAtMS {
			return ErrInvalid
		}
		d, e := PreparationDigest(*o.Preparation)
		if e != nil || d != o.PreparationDigest {
			return ErrInvalid
		}
	}
	unsigned := o.Stage == "intent" || o.Stage == "prepared" || o.Stage == "abandoned"
	if unsigned {
		if o.SignedPackage != nil || o.SignedPackageDigest != "" || o.SignAuthorization != nil || o.CommitDispatch != nil || o.SpendDispatch != nil || o.CommitObservation != nil || o.SpendObservation != nil {
			return ErrInvalid
		}
	} else {
		if o.Preparation == nil || o.SignedPackage == nil || o.SignAuthorization == nil || ValidateSignedPackage(*o.SignedPackage, *o.Preparation, pin) != nil || !authorizationValid(*o.SignAuthorization, pin.OwnerMode, o.Preparation.PreparedAtMS, o.UpdatedAtMS) {
			return ErrInvalid
		}
		d, e := SignedPackageDigest(*o.SignedPackage)
		if e != nil || d != o.SignedPackageDigest {
			return ErrInvalid
		}
		for _, d := range []*Dispatch{o.CommitDispatch, o.SpendDispatch} {
			if d != nil && !dispatchValid(*d, pin.OwnerMode, o.SignAuthorization.AuthorizedAtMS, o.UpdatedAtMS) {
				return ErrInvalid
			}
		}
		for _, v := range []struct {
			o  *Observation
			tx RawTransaction
		}{{o.CommitObservation, o.SignedPackage.Commit}, {o.SpendObservation, o.SignedPackage.Spend}} {
			if v.o != nil && !observationValid(*v.o, v.tx.TxID, o.CreatedAtMS, o.UpdatedAtMS) {
				return ErrInvalid
			}
			if observed(v.o) && (v.o.TransferID != o.SignedPackage.TransferID || v.o.ObservedTxID == v.tx.TxID && v.o.RawSHA256 != v.tx.RawSHA256) {
				return ErrInvalid
			}
		}
	}
	if o.Request.Kind != "transfer" && o.CommitDispatch != nil {
		return ErrInvalid
	}
	switch o.Stage {
	case "signed":
		if o.CommitDispatch != nil || o.SpendDispatch != nil {
			return ErrInvalid
		}
	case "commit_dispatching", "commit_observed":
		if o.Request.Kind != "transfer" || o.CommitDispatch == nil || o.SpendDispatch != nil {
			return ErrInvalid
		}
	case "spend_dispatching", "complete":
		if o.SpendDispatch == nil || o.Request.Kind == "transfer" && o.CommitDispatch == nil {
			return ErrInvalid
		}
	}
	if (o.Stage == "commit_observed" || o.Stage == "spend_dispatching" || o.Stage == "complete") && o.Request.Kind == "transfer" && !observed(o.CommitObservation) {
		return ErrInvalid
	}
	if o.Stage == "complete" && !observed(o.SpendObservation) {
		return ErrInvalid
	}
	return bounded(o, MaxOperationBytes)
}

func ValidateWalletRecord(v WalletRecord, pin Pin, binding string) error {
	if v.Version != 1 || v.Sequence < 1 || v.Sequence > MaxOperations || v.BindingID != binding || v.RequestID != v.Preparation.Request.RequestID || ValidatePreparation(v.Preparation, pin, binding) != nil {
		return ErrInvalid
	}
	d, e := PreparationDigest(v.Preparation)
	if e != nil || d != v.PreparationDigest {
		return ErrInvalid
	}
	switch v.State {
	case "prepared", "abandoned":
		if v.SignedPackageDigest != "" || v.CommitTxID != "" || v.SpendTxID != "" {
			return ErrInvalid
		}
	case "retained", "complete":
		if !hexValue(v.SignedPackageDigest, 32, false) || !hexValue(v.CommitTxID, 32, false) || !hexValue(v.SpendTxID, 32, false) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return bounded(v, MaxWalletRecordBytes)
}

func AuthorizationFor(p Preparation) (AuthorizationRequest, error) {
	d, e := ReviewDigest(p.Review)
	if e != nil || d != p.ReviewDigest || p.Request.Kind != "transfer" || p.Review.RequestID != p.Request.RequestID {
		return AuthorizationRequest{}, ErrInvalid
	}
	r := p.Review
	return AuthorizationRequest{1, p.BindingID, p.Request.RequestID, p.ReviewDigest, r.OwnershipPinID, r.PriorStateID, r.SenderGym, r.RecipientGym, r.FundingDigest}, nil
}

func ParseRequest(raw []byte) (Request, error) {
	var v Request
	if e := decodeJournal(raw, MaxRequestBytes, &v); e != nil {
		return v, ErrInvalid
	}
	return v, ValidateRequest(v)
}
func ParseReview(raw []byte) (Review, error) {
	var v Review
	if e := decodeJournal(raw, MaxReviewBytes, &v); e != nil {
		return v, ErrInvalid
	}
	return v, ValidateReview(v)
}
func ParsePreparation(raw []byte, pin Pin, binding string) (Preparation, error) {
	var v Preparation
	if e := decodeJournal(raw, MaxPreparationBytes, &v); e != nil {
		return v, ErrInvalid
	}
	return v, ValidatePreparation(v, pin, binding)
}
func ParseSignedPackage(raw []byte, p Preparation, pin Pin) (SignedPackage, error) {
	var v SignedPackage
	if e := decodeJournal(raw, MaxSignedPackageBytes, &v); e != nil {
		return v, ErrInvalid
	}
	return v, ValidateSignedPackage(v, p, pin)
}
func ParseOperation(raw []byte, pin Pin, binding string) (Operation, error) {
	var v Operation
	if e := decodeJournal(raw, MaxOperationBytes, &v); e != nil {
		return v, ErrInvalid
	}
	return v, ValidateOperation(v, pin, binding)
}
