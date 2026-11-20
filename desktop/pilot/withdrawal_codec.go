// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/hex"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/txscript/v2"
)

const withdrawalMaxMoney int64 = 21000000 * 100000000

func WithdrawalErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "unauthorized", "stale_view", "held", "reauthentication_required", "busy", "capacity", "withdrawal_unavailable", "withdrawal_not_found", "operation_conflict", "draft_stale", "insufficient_funds", "fee_limit", "outcome_unknown")
}
func withdrawalOptionalID(s string) bool { return s == "" || payoutNonzeroID(s) }
func withdrawalBindingValid(b WithdrawalBinding) bool {
	return stableViewValid(b.View) && payoutNonzeroID(b.SourceBindingID) && payoutNonzeroID(b.JournalID) && canonicalManifestID(b.WithdrawalBindingID) && historySize(b, 4096)
}
func WithdrawalDestination(destination string) (string, error) {
	if len(destination) == 0 || len(destination) > 90 {
		return "", gymstable.ErrInvalid
	}
	for _, c := range []byte(destination) {
		if c < 33 || c > 126 {
			return "", gymstable.ErrInvalid
		}
	}
	a, e := address.DecodeAddress(destination, &chaincfg.MainNetParams)
	if e != nil || !a.IsForNet(&chaincfg.MainNetParams) || a.EncodeAddress() != destination {
		return "", gymstable.ErrInvalid
	}
	switch a.(type) {
	case *address.AddressPubKeyHash, *address.AddressScriptHash, *address.AddressWitnessPubKeyHash, *address.AddressTaproot:
	default:
		return "", gymstable.ErrInvalid
	}
	s, e := txscript.PayToAddrScript(a)
	if e != nil {
		return "", gymstable.ErrInvalid
	}
	return hex.EncodeToString(s), nil
}
func withdrawalIntentValid(i WithdrawalIntent) bool {
	script, e := WithdrawalDestination(i.Destination)
	return e == nil && script == i.RecipientScript && i.AmountAtoms > 0 && i.AmountAtoms <= withdrawalMaxMoney && i.FeeRateAtomsPerKB == 1000 && i.FeeLimitAtoms == 100000 && historySize(i, 512)
}
func withdrawalDraftValid(d WithdrawalDraftView) bool {
	if d.Version != 1 || !canonicalManifestID(d.DraftDigest) || !canonicalManifestID(d.UnsignedTxID) || !withdrawalIntentValid(WithdrawalIntent{d.Destination, d.RecipientScript, d.AmountAtoms, 1000, 100000}) || d.FeeAtoms < 1 || d.FeeAtoms > 100000 || d.AmountAtoms > withdrawalMaxMoney-d.FeeAtoms || d.TotalDebitAtoms != d.AmountAtoms+d.FeeAtoms || d.ChangeAtoms < 0 || d.ChangeAtoms > withdrawalMaxMoney-d.TotalDebitAtoms || d.InputAtoms != d.TotalDebitAtoms+d.ChangeAtoms || d.InputCount < 1 || d.InputCount > 16 || d.RecipientOutputIndex > 1 || !payoutTime(d.PreparedAtMS, false) || !canonicalManifestID(d.ObservedTip) || d.ObservedHeight > 2147483647 || !historySize(d, 2048) {
		return false
	}
	return d.ChangeAtoms == 0 && d.ChangeOutputIndex == -1 && d.RecipientOutputIndex == 0 || d.ChangeAtoms > 0 && d.ChangeOutputIndex >= 0 && d.ChangeOutputIndex <= 1 && uint32(d.ChangeOutputIndex) != d.RecipientOutputIndex
}
func ValidateWithdrawalReview(r WithdrawalReview) error {
	if r.Version != 1 || !withdrawalBindingValid(r.Binding) || !payoutNonzeroID(r.OperationID) || !payoutNonzeroID(r.ActionID) || !payoutNonzeroID(r.ReviewID) || !withdrawalDraftValid(r.Draft) || !payoutTime(r.ReviewedAtMS, false) || !payoutTime(r.ExpiresAtMS, false) || r.ExpiresAtMS <= r.ReviewedAtMS || r.ExpiresAtMS-r.ReviewedAtMS > 300000 || !historySize(r, 8192) {
		return gymstable.ErrInvalid
	}
	if r.Kind == "send" && r.TxID == "" || r.Kind == "retry" && r.TxID == r.Draft.UnsignedTxID {
		return nil
	}
	return gymstable.ErrInvalid
}
func withdrawalDispatchValid(d WithdrawalDispatch, created, updated int64) bool {
	a := d.Authorization
	if d.Attempt == 0 || !payoutNonzeroID(a.ActionID) || !payoutNonzeroID(a.ReviewID) || !payoutNonzeroID(a.ViewID) || a.Generation == 0 || a.Generation > gymstable.MaxSafeInteger || a.AuthorizedAtMS < created || a.AuthorizedAtMS > updated || d.IntentAtMS < a.AuthorizedAtMS || d.IntentAtMS > updated {
		return false
	}
	if d.Result == "pending" {
		return d.ResultAtMS == 0 && d.Error == ""
	}
	if d.ResultAtMS < d.IntentAtMS || d.ResultAtMS > updated {
		return false
	}
	return d.Result == "acknowledged" && d.Error == "" || d.Result == "unknown" && d.Error == "node_unavailable" || d.Result == "rejected" && d.Error == "node_rejected"
}
func withdrawalObservationValid(o WithdrawalObservation) bool {
	if o.Version != 1 || !canonicalManifestID(o.TxID) || !payoutTime(o.ObservedAtMS, false) || o.NodeHeight > 2147483647 || !historySize(o, 2048) {
		return false
	}
	if o.State == "unknown" {
		return o.NodeTip == "" && o.NodeHeight == 0 && o.Confirmations == 0 && o.BlockHash == "" && o.Error == "node_unavailable"
	}
	if !payoutNonzeroID(o.NodeTip) || o.Error != "" {
		return false
	}
	return stableOne(o.State, "not_observed", "mempool") && o.Confirmations == 0 && o.BlockHash == "" || o.State == "confirmed" && o.Confirmations > 0 && o.Confirmations <= o.NodeHeight+1 && payoutNonzeroID(o.BlockHash)
}
func ValidateWithdrawalOperation(o WithdrawalOperation) error {
	bad := gymstable.ErrInvalid
	if o.Version != 1 || !payoutNonzeroID(o.OperationID) || !payoutNonzeroID(o.JournalID) || !canonicalManifestID(o.WithdrawalBindingID) || o.Sequence < 1 || o.Sequence > 128 || o.Revision < 1 || !payoutTime(o.CreatedAtMS, false) || !payoutTime(o.UpdatedAtMS, false) || o.UpdatedAtMS < o.CreatedAtMS || !withdrawalIntentValid(o.Intent) || !canonicalManifestID(o.IntentDigest) || !historySize(o, 8192) {
		return bad
	}
	if o.Draft != nil {
		d := *o.Draft
		if !withdrawalDraftValid(d) || d.PreparedAtMS != o.PreparedAtMS || o.PreparedAtMS < o.CreatedAtMS || o.PreparedAtMS > o.UpdatedAtMS || d.Destination != o.Intent.Destination || d.RecipientScript != o.Intent.RecipientScript || d.AmountAtoms != o.Intent.AmountAtoms {
			return bad
		}
	} else if o.PreparedAtMS != 0 {
		return bad
	}
	signed := stableOne(o.Stage, "signed", "dispatching", "submitted")
	if signed {
		if o.Draft == nil || o.TxID != o.Draft.UnsignedTxID || !canonicalManifestID(o.SignedSHA256) {
			return bad
		}
	} else if o.TxID != "" || o.SignedSHA256 != "" || o.Dispatch != nil || o.Observation != nil {
		return bad
	}
	if o.Dispatch != nil && !withdrawalDispatchValid(*o.Dispatch, o.CreatedAtMS, o.UpdatedAtMS) {
		return bad
	}
	if o.Observation != nil && (!withdrawalObservationValid(*o.Observation) || o.Observation.TxID != o.TxID || o.Observation.ObservedAtMS < o.CreatedAtMS || o.Observation.ObservedAtMS > o.UpdatedAtMS) {
		return bad
	}
	switch o.Stage {
	case "intent":
		if o.Draft != nil {
			return bad
		}
	case "prepared":
		if o.Draft == nil {
			return bad
		}
	case "signed":
		if o.Dispatch != nil {
			return bad
		}
	case "dispatching":
		if o.Dispatch == nil {
			return bad
		}
	case "submitted":
		if o.Observation == nil {
			return bad
		}
	case "abandoned":
	default:
		return bad
	}
	return nil
}
func ValidateWithdrawalAction(a WithdrawalAction) error {
	if a.Version != 1 || !withdrawalBindingValid(a.Binding) || !payoutNonzeroID(a.OperationID) || !payoutNonzeroID(a.ActionID) || !stableOne(a.Kind, "prepare", "send", "retry", "abandon", "observe") || !withdrawalOptionalID(a.ReviewID) || !historySize(a, 14336) {
		return gymstable.ErrInvalid
	}
	if stableOne(a.Kind, "send", "retry") != (a.ReviewID != "") {
		return gymstable.ErrInvalid
	}
	if a.Operation != nil && (ValidateWithdrawalOperation(*a.Operation) != nil || a.Operation.OperationID != a.OperationID || a.Operation.JournalID != a.Binding.JournalID || a.Operation.WithdrawalBindingID != a.Binding.WithdrawalBindingID) {
		return gymstable.ErrInvalid
	}
	switch a.State {
	case "review":
		if !stableOne(a.Kind, "send", "retry") || a.Operation == nil || a.Error != "" || a.Kind == "send" && a.Operation.Stage != "prepared" || a.Kind == "retry" && !stableOne(a.Operation.Stage, "signed", "dispatching") {
			return gymstable.ErrInvalid
		}
	case "running", "done", "stopped":
		if a.Error != "" {
			return gymstable.ErrInvalid
		}
	case "failed":
		if !WithdrawalErrorCode(a.Error) || a.Error == "outcome_unknown" {
			return gymstable.ErrInvalid
		}
	case "outcome_unknown":
		if a.Error != "outcome_unknown" {
			return gymstable.ErrInvalid
		}
	default:
		return gymstable.ErrInvalid
	}
	return nil
}
func ValidateWithdrawalSnapshot(s WithdrawalSnapshot) error {
	if s.Version != 1 || !withdrawalOptionalID(s.ActiveActionID) || !withdrawalOptionalID(s.LastActionID) || !withdrawalOptionalID(s.UnresolvedOperationID) || !withdrawalOptionalID(s.LatestOperationID) || !historySize(s, 5120) {
		return gymstable.ErrInvalid
	}
	if s.State == "unavailable" {
		if s.Error == "withdrawal_unavailable" && s.Binding == nil && s.ActiveActionID == "" && s.LastActionID == "" && s.UnresolvedOperationID == "" && s.LatestOperationID == "" && !s.CanPrepare && !s.CanReview {
			return nil
		}
		return gymstable.ErrInvalid
	}
	if s.Binding == nil || !withdrawalBindingValid(*s.Binding) || s.Error != "" || !stableOne(s.State, "idle", "busy") || s.State == "busy" && s.ActiveActionID == "" || s.State == "idle" && s.ActiveActionID != "" || s.CanPrepare && (s.UnresolvedOperationID != "" || s.State != "idle") {
		return gymstable.ErrInvalid
	}
	return nil
}
func withdrawalRequestValid(r WithdrawalRequest) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger || r.Version != 1 {
		return false
	}
	if r.Command == "withdrawal_snapshot" {
		return r.ViewID == "" && r.OperationID == "" && r.ActionID == "" && r.ReviewID == "" && r.ReviewKind == "" && r.DraftDigest == "" && r.TxID == "" && r.Intent == nil
	}
	if !canonicalManifestID(r.ViewID) || !payoutNonzeroID(r.OperationID) || !withdrawalOptionalID(r.ActionID) || !withdrawalOptionalID(r.ReviewID) || !stableOptionalID(r.DraftDigest) || !stableOptionalID(r.TxID) {
		return false
	}
	switch r.Command {
	case "withdrawal_operation":
		return r.ActionID == "" && r.ReviewID == "" && r.ReviewKind == "" && r.DraftDigest == "" && r.TxID == "" && r.Intent == nil
	case "withdrawal_action", "withdrawal_stop":
		return r.ActionID != "" && r.ReviewID == "" && r.ReviewKind == "" && r.DraftDigest == "" && r.TxID == "" && r.Intent == nil
	case "withdrawal_prepare":
		return r.ActionID != "" && r.ReviewID == "" && r.ReviewKind == "" && r.DraftDigest == "" && r.TxID == "" && r.Intent != nil && withdrawalIntentValid(*r.Intent)
	case "withdrawal_review":
		return r.ActionID != "" && r.ReviewID != "" && r.DraftDigest != "" && r.Intent == nil && (r.ReviewKind == "send" && r.TxID == "" || r.ReviewKind == "retry" && r.TxID != "")
	case "withdrawal_approve":
		return r.ActionID != "" && r.ReviewID != "" && r.ReviewKind == "" && r.DraftDigest != "" && r.Intent == nil
	case "withdrawal_abandon":
		return r.ActionID != "" && r.ReviewID == "" && r.ReviewKind == "" && r.TxID == "" && r.Intent == nil
	case "withdrawal_observe":
		return r.ActionID != "" && r.ReviewID == "" && r.ReviewKind == "" && r.DraftDigest == "" && r.TxID != "" && r.Intent == nil
	}
	return false
}
func withdrawalResponseValid(r WithdrawalResponse) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger || r.Version != 1 {
		return false
	}
	n := 0
	valid := true
	if r.Snapshot != nil {
		n++
		valid = valid && ValidateWithdrawalSnapshot(*r.Snapshot) == nil
	}
	if r.Operation != nil {
		n++
		valid = valid && ValidateWithdrawalOperation(*r.Operation) == nil
	}
	if r.Review != nil {
		n++
		valid = valid && ValidateWithdrawalReview(*r.Review) == nil
	}
	if r.Action != nil {
		n++
		valid = valid && ValidateWithdrawalAction(*r.Action) == nil
	}
	if !r.OK {
		return n == 0 && WithdrawalErrorCode(r.Error)
	}
	return n == 1 && valid && r.Error == ""
}
func ParseWithdrawalRequest(raw []byte) (WithdrawalRequest, error) {
	var r WithdrawalRequest
	if _, e := withdrawalDecode(raw, MaxWithdrawalRequestSize-1, 4, &r); e != nil || !withdrawalRequestValid(r) {
		return WithdrawalRequest{}, gymstable.ErrInvalid
	}
	return r, nil
}
func MarshalWithdrawalRequest(r WithdrawalRequest) ([]byte, error) {
	if !withdrawalRequestValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := payoutJSON(r)
	if e != nil || len(b) >= MaxWithdrawalRequestSize {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}
func ParseWithdrawalResponse(raw []byte) (WithdrawalResponse, error) {
	var r WithdrawalResponse
	if _, e := withdrawalDecode(raw, MaxWithdrawalResponseSize-1, 10, &r); e != nil || !withdrawalResponseValid(r) {
		return WithdrawalResponse{}, gymstable.ErrInvalid
	}
	return r, nil
}
func MarshalWithdrawalResponse(r WithdrawalResponse) ([]byte, error) {
	if !withdrawalResponseValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := payoutJSON(r)
	if e != nil || len(b) >= MaxWithdrawalResponseSize {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}
