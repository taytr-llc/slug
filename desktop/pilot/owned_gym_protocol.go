// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

// OwnedGymRequest is a separate closed protocol. Every field is present; the
// command selects the only active fields. No signed transaction is an input.
type OwnedGymRequest struct {
	Version      uint32                      `json:"version"`
	ID           uint64                      `json:"id"`
	Command      string                      `json:"command"`
	ViewID       string                      `json:"view_id"`
	Fixture      string                      `json:"fixture"`
	CaptureID    string                      `json:"capture_id"`
	ProspectID   string                      `json:"prospect_id"`
	RequestID    string                      `json:"request_id"`
	OperationID  string                      `json:"operation_id"`
	ReviewID     string                      `json:"review_id"`
	RecipientGym string                      `json:"recipient_gym"`
	Leg          string                      `json:"leg"`
	Saturday     string                      `json:"saturday"`
	After        uint32                      `json:"after"`
	Limit        uint32                      `json:"limit"`
	Slot         *gymstable.OwnedSlotRequest `json:"slot"`
	Entry        *gymentry.Request           `json:"entry"`
	Selection    *recoveryfile.Pin           `json:"selection"`
}

// The concrete controller supplies this interface from retained original
// resources. Wire objects and returned scopes do not confer capabilities.
type OwnedGymControllerAPI interface {
	Snapshot() OwnedGymSnapshot
	Scope(context.Context, string) (OwnedGymScope, error)
	LoginFixture(string) error
	Logout(context.Context, string) error
	ResetFixtureAccount(context.Context, string) error
	ResumePendingReset(context.Context, string) error
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Close() error
	CaptureStable(context.Context, string, string) (OwnedStableCapture, error)
	StablePage(context.Context, string, string, uint32, uint32) (gymstable.OwnedPage, error)
	PrepareSlot(context.Context, string, gymstable.OwnedSlotRequest) (gymstable.OwnedIntent, error)
	ApplySlot(context.Context, string, string) (gymstable.OwnedMutationResult, error)
	SlotOperation(context.Context, string, string) (gymstable.OwnedOperation, error)
	PendingSlot(context.Context, string) (*gymstable.OwnedOperation, error)
	AbandonSlot(context.Context, string, string) (gymstable.OwnedOperation, error)
	CaptureHistory(context.Context, string, string) (gymstable.OwnedCareerHistoryCapture, error)
	HistoryPage(context.Context, string, string, uint32, uint32) (gymstable.OwnedCareerHistoryPage, error)
	EntryUpcoming(context.Context, string) (gymentry.OwnedEventStatus, error)
	EntryStatus(context.Context, string, string) (gymentry.OwnedEventStatus, error)
	PrepareEntry(context.Context, string, gymentry.Request) (gymentry.OwnedIntent, error)
	ApplyEntry(context.Context, string, string) (gymentry.OwnedMutationResult, error)
	EntryOperation(context.Context, string, string) (gymentry.OwnedOperation, error)
	PendingEntry(context.Context, string) (*gymentry.OwnedOperation, error)
	AbandonEntry(context.Context, string, string) (gymentry.OwnedOperation, error)
	EventStatus(context.Context, string, string) (gymevent.OwnedEventStatus, error)
	EventRecord(context.Context, string, string) (gymevent.OwnedSnapshotReceipt, error)
	TransferPrepare(context.Context, string, string, string, string) (OwnedTransferReview, error)
	TransferResume(context.Context, string, string, string) (OwnedTransferReview, error)
	TransferConfirm(context.Context, string, string) (fightercarrier.Operation, error)
	TransferReviewDelivery(context.Context, string, string, string) (OwnedTransferDeliveryReview, error)
	TransferDeliver(context.Context, string, string) (fightercarrier.Operation, error)
	TransferObserve(context.Context, string, string, string) (fightercarrier.Operation, error)
	TransferOperation(context.Context, string, string) (fightercarrier.Operation, error)
	TransferFinality(context.Context, string, string) (OwnedTransferFinality, error)
	TransferAbandon(context.Context, string, string) (fightercarrier.Operation, error)
	ExportRecovery(context.Context, string, recoveryfile.Pin) error
	VerifyBackup(context.Context, string, recoveryfile.Pin) error
	CancelBackup(context.Context, string) error
}

func ValidateOwnedGymRequest(r OwnedGymRequest) error {
	bad := gymstable.ErrInvalid
	if r.Version != 1 || r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return bad
	}
	left := r
	left.Version, left.ID, left.Command = 0, 0, ""
	child := true
	switch r.Command {
	case "snapshot", "startup":
		child = false
	case "scope", "login_fixture":
		child = false
		if _, e := gymaccount.FixtureIdentity(r.Fixture); e != nil {
			return bad
		}
		left.Fixture = ""
	case "resume_reset":
		child = false
		if !payoutNonzeroID(r.OperationID) {
			return bad
		}
		left.OperationID = ""
	case "logout", "reset_fixture", "start", "stop", "pending_slot", "pending_entry", "entry_upcoming", "cancel_backup":
	case "capture_stable":
		if !payoutNonzeroID(r.CaptureID) {
			return bad
		}
		left.CaptureID = ""
	case "stable_page", "history_page":
		if !payoutNonzeroID(r.CaptureID) || r.Limit < 1 || r.Limit > 8 {
			return bad
		}
		left.CaptureID, left.After, left.Limit = "", 0, 0
	case "capture_history":
		if !payoutNonzeroID(r.ProspectID) {
			return bad
		}
		left.ProspectID = ""
	case "prepare_slot":
		if r.Slot == nil || r.Slot.ExpectedRevision > gymstable.MaxOperations || r.Slot.Slot < 1 || r.Slot.Slot > 10 || (r.Slot.ProspectID == "") != (r.Slot.ExpectedStateID == "") || r.Slot.ProspectID != "" && (!payoutNonzeroID(r.Slot.ProspectID) || !payoutNonzeroID(r.Slot.ExpectedStateID)) {
			return bad
		}
		left.Slot = nil // Complete head validation uses the independently sealed pin.
	case "apply_slot", "slot_operation", "abandon_slot", "apply_entry", "entry_operation", "abandon_entry":
		if !payoutNonzeroID(r.OperationID) {
			return bad
		}
		left.OperationID = ""
	case "prepare_entry":
		if r.Entry == nil {
			return bad
		}
		if _, e := gymentry.ValidateRequest(*r.Entry); e != nil {
			return bad
		}
		left.Entry = nil
	case "entry_status", "event_status", "event_record":
		t, e := time.Parse("2006-01-02", r.Saturday)
		if e != nil || t.Weekday() != time.Saturday || t.Format("2006-01-02") != r.Saturday {
			return bad
		}
		left.Saturday = ""
	case "transfer_prepare":
		if !payoutNonzeroID(r.RequestID) || !payoutNonzeroID(r.ProspectID) || !ownedcodec.Hex(r.RecipientGym, 33) {
			return bad
		}
		key, e := hexPublicKey(r.RecipientGym)
		if e != nil || key == nil {
			return bad
		}
		left.RequestID, left.ProspectID, left.RecipientGym = "", "", ""
	case "transfer_resume":
		if !payoutNonzeroID(r.RequestID) || !payoutNonzeroID(r.ProspectID) {
			return bad
		}
		left.RequestID, left.ProspectID = "", ""
	case "transfer_confirm", "transfer_deliver":
		if !payoutNonzeroID(r.ReviewID) {
			return bad
		}
		left.ReviewID = ""
	case "transfer_review_delivery", "transfer_observe":
		if !payoutNonzeroID(r.RequestID) || !stableOne(r.Leg, "commit", "spend") {
			return bad
		}
		left.RequestID, left.Leg = "", ""
	case "transfer_operation", "transfer_abandon", "transfer_finality":
		if !payoutNonzeroID(r.RequestID) {
			return bad
		}
		left.RequestID = ""
	case "export_recovery", "verify_backup":
		mode := recoveryfile.CreateMode
		if r.Command == "verify_backup" {
			mode = recoveryfile.ReadMode
		}
		if r.Selection == nil || recoveryfile.ValidatePin(*r.Selection, mode) != nil {
			return bad
		}
		left.Selection = nil
	default:
		return bad
	}
	if child {
		if !payoutNonzeroID(r.ViewID) {
			return bad
		}
		left.ViewID = ""
	}
	if !reflect.DeepEqual(left, OwnedGymRequest{}) || !ownedcodec.Fits(r, MaxOwnedGymRequestSize) {
		return bad
	}
	return nil
}

func ParseOwnedGymRequest(raw []byte) (OwnedGymRequest, error) {
	var r OwnedGymRequest
	if ownedcodec.Decode(raw, MaxOwnedGymRequestSize, &r) != nil {
		return r, gymstable.ErrInvalid
	}
	return r, ValidateOwnedGymRequest(r)
}
func MarshalOwnedGymRequest(r OwnedGymRequest) ([]byte, error) {
	if e := ValidateOwnedGymRequest(r); e != nil {
		return nil, e
	}
	return json.Marshal(r)
}

// A response's result is parsed into the exact command-selected public DTO.
// The raw field is never handed to the webview or used as source configuration.
type OwnedGymResponse struct {
	Version uint32          `json:"version"`
	ID      uint64          `json:"id"`
	Command string          `json:"command"`
	ViewID  string          `json:"view_id"`
	OK      bool            `json:"ok"`
	Result  json.RawMessage `json:"result"`
	Error   string          `json:"error"`
}

func OwnedGymError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, recoveryvault.ErrExportUncertain), errors.Is(err, recoveryfile.ErrOutcomeUnknown):
		return "backup_export_uncertain"
	case errors.Is(err, fightercarrier.ErrOutcomeUnknown), errors.Is(err, gymstable.ErrOutcomeUnknown), errors.Is(err, gymentry.ErrOutcomeUnknown), errors.Is(err, gymevent.ErrOutcomeUnknown), errors.Is(err, wallet.ErrAllocationOutcomeUnknown), errors.Is(err, recoveryvault.ErrOutcomeUnknown):
		return "outcome_unknown"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "cancelled"
	case errors.Is(err, gymaccount.ErrHeld), errors.Is(err, gymaccount.ErrForbidden):
		return "forbidden"
	case errors.Is(err, gymaccount.ErrUnauthorized), errors.Is(err, fightercarrier.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, ErrHostedStaleView):
		return "stale_view"
	case errors.Is(err, ErrPoolRuntimeBusy), errors.Is(err, fightercarrier.ErrBusy):
		return "busy"
	case errors.Is(err, fightercarrier.ErrNotFound):
		return "not_found"
	case errors.Is(err, fightercarrier.ErrConflict):
		return "conflict"
	case errors.Is(err, fightercarrier.ErrCapacity):
		return "capacity"
	case errors.Is(err, fightercarrier.ErrInvalid), errors.Is(err, gymstable.ErrInvalid):
		return "invalid_request"
	default:
		code := HostedRecoveryError(err)
		if stableOne(code, "backup_export_uncertain", "backup_storage_failed", "outcome_unknown", "allocation_capacity", "allocation_conflict", "allocation_resume_required") {
			return code
		}
		return "owned_unavailable"
	}
}

func ownedGymErrorCode(code string) bool {
	return stableOne(code, "outcome_unknown", "cancelled", "forbidden", "unauthorized", "stale_view", "busy", "not_found", "conflict", "capacity", "invalid_request", "owned_unavailable", "backup_export_uncertain", "backup_storage_failed", "allocation_capacity", "allocation_conflict", "allocation_resume_required")
}

// hexPublicKey accepts only a canonical compressed key; a 33-byte string is
// insufficient because most such strings are not valid curve points.
func hexPublicKey(s string) (*btcec.PublicKey, error) {
	b, e := hex.DecodeString(s)
	if e != nil || len(b) != 33 || b[0] != 2 && b[0] != 3 {
		return nil, gymstable.ErrInvalid
	}
	return btcec.ParsePubKey(b)
}

// ParseOwnedGymResponse authenticates no source. expected is the immutable
// scope sealed from the original helper's dedicated setup disclosure, and view
// is the original view retained before dispatch. Native additionally compares
// paged results to its retained capture and mutations to the reviewed intent.
func ParseOwnedGymResponse(raw []byte, req OwnedGymRequest, expected *OwnedGymScope, view *OwnedGymView) (OwnedGymResponse, any, error) {
	if req.Command == "startup" {
		return ParseOwnedGymBootstrapResponse(raw, req, nil)
	}
	var r OwnedGymResponse
	bad := gymstable.ErrInvalid
	if ValidateOwnedGymRequest(req) != nil || ownedcodec.Decode(raw, MaxOwnedGymResponseSize, &r) != nil || r.Version != 1 || r.ID != req.ID || r.Command != req.Command || r.ViewID != req.ViewID {
		return r, nil, bad
	}
	if !r.OK {
		if string(r.Result) != "null" || !ownedGymErrorCode(r.Error) {
			return r, nil, bad
		}
		return r, nil, nil
	}
	if r.Error != "" {
		return r, nil, bad
	}
	if req.Command == "scope" {
		var s OwnedGymScope
		if ownedcodec.Decode(r.Result, MaxOwnedGymResponseSize, &s) != nil || ValidateOwnedGymScope(s) != nil || s.Fixture != req.Fixture {
			return r, nil, bad
		}
		if expected != nil && !ownedcodec.Equal(s, *expected) {
			return r, nil, bad
		}
		return r, s, nil
	}
	if expected == nil || ValidateOwnedGymScope(*expected) != nil {
		return r, nil, bad
	}
	if req.ViewID != "" && (view == nil || view.ViewID != req.ViewID || !expected.ViewMatches(*view)) {
		return r, nil, bad
	}
	decode := func(dst any, limit int) error { return ownedcodec.Decode(r.Result, limit, dst) }
	switch req.Command {
	case "snapshot", "login_fixture", "logout", "reset_fixture", "resume_reset", "start", "stop", "export_recovery", "verify_backup", "cancel_backup":
		var s OwnedGymSnapshot
		if decode(&s, MaxOwnedGymResponseSize) != nil || ValidateOwnedGymSnapshotWithScope(s, *expected) != nil {
			return r, nil, bad
		}
		// Controls may legitimately end the original view. Controls that only
		// inspect or operate its child cannot acknowledge a replacement login.
		if req.ViewID != "" && req.Command != "logout" && req.Command != "reset_fixture" && (s.View == nil || *s.View != *view) {
			return r, nil, bad
		}
		return r, s, nil
	case "capture_stable":
		var c OwnedStableCapture
		if decode(&c, MaxOwnedGymResponseSize) != nil || view == nil || c.CaptureID != req.CaptureID || ValidateOwnedStableCaptureWithScope(c, *expected, *view) != nil {
			return r, nil, bad
		}
		return r, c, nil
	case "stable_page":
		p, e := gymstable.ParseOwnedPage(r.Result)
		if e != nil || p.Status.Pin != expected.StablePin || p.After != req.After || len(p.Items) > int(req.Limit) {
			return r, nil, bad
		}
		return r, p, nil
	case "prepare_slot":
		var i gymstable.OwnedIntent
		if decode(&i, gymstable.OwnedMaxOperationSize) != nil || view == nil || i.ViewID != req.ViewID || i.Generation != view.Generation || req.Slot == nil || i.Request != *req.Slot {
			return r, nil, bad
		}
		if _, e := gymstable.ValidateOwnedOperation(expected.StablePin, gymstable.OwnedOperation{Version: 1, Intent: i, State: "prepared"}); e != nil {
			return r, nil, bad
		}
		return r, i, nil
	case "slot_operation", "abandon_slot", "pending_slot":
		if req.Command == "pending_slot" && string(r.Result) == "null" {
			return r, (*gymstable.OwnedOperation)(nil), nil
		}
		o, e := gymstable.ParseOwnedOperation(expected.StablePin, r.Result)
		if e != nil || req.Command != "pending_slot" && o.Intent.OperationID != req.OperationID || req.Command == "abandon_slot" && o.State != "abandoned" {
			return r, nil, bad
		}
		return r, o, nil
	case "apply_slot":
		var m gymstable.OwnedMutationResult
		if decode(&m, gymstable.OwnedMaxOperationSize) != nil || m.Version != 1 || m.Receipt.OperationID != req.OperationID {
			return r, nil, bad
		}
		v := m.Receipt
		// The wire has no preparation timestamp/sequence. Validate the full
		// receipt using an internally assembled minimum enclosing operation;
		// native also checks the actual retained intent before acknowledging.
		i := gymstable.OwnedIntent{Version: 1, Sequence: 1, OperationID: v.OperationID, Generation: v.Generation, ViewID: v.ViewID, Request: v.Request, PreparedAtMS: v.CommittedAtMS}
		if _, e := gymstable.ValidateOwnedOperation(expected.StablePin, gymstable.OwnedOperation{Version: 1, Intent: i, State: "committed", Receipt: &v}); e != nil {
			return r, nil, bad
		}
		return r, m, nil
	case "capture_history":
		c, e := gymstable.ParseOwnedCareerHistoryCapture(r.Result)
		if e != nil || c.Status.Pin != expected.StablePin || c.Birth.ProspectID != req.ProspectID {
			return r, nil, bad
		}
		return r, c, nil
	case "history_page":
		p, e := gymstable.ParseOwnedCareerHistoryPage(r.Result)
		if e != nil || p.Capture.Status.Pin != expected.StablePin || p.Capture.HistoryID != req.CaptureID || p.After != req.After || len(p.Items) > int(req.Limit) {
			return r, nil, bad
		}
		return r, p, nil
	case "entry_upcoming", "entry_status":
		var s gymentry.OwnedEventStatus
		if decode(&s, gymentry.OwnedMaxMetadataSize) != nil {
			return r, nil, bad
		}
		if _, e := gymentry.ValidateOwnedEventStatus(expected.EntryPin, s); e != nil || req.Command == "entry_status" && s.Event.Saturday != req.Saturday {
			return r, nil, bad
		}
		return r, s, nil
	case "prepare_entry":
		i, e := gymentry.ParseOwnedIntent(r.Result)
		if e != nil || view == nil || i.ViewID != req.ViewID || i.Generation != view.Generation || req.Entry == nil || !ownedcodec.Equal(i.Request, *req.Entry) {
			return r, nil, bad
		}
		if _, e = gymentry.ValidateOwnedOperation(expected.EntryPin, gymentry.OwnedOperation{Version: 1, Intent: i, State: "prepared"}); e != nil {
			return r, nil, bad
		}
		return r, i, nil
	case "entry_operation", "abandon_entry", "pending_entry":
		if req.Command == "pending_entry" && string(r.Result) == "null" {
			return r, (*gymentry.OwnedOperation)(nil), nil
		}
		o, e := gymentry.ParseOwnedOperation(expected.EntryPin, r.Result)
		if e != nil || req.Command != "pending_entry" && o.Intent.OperationID != req.OperationID || req.Command == "abandon_entry" && o.State != "abandoned" {
			return r, nil, bad
		}
		return r, o, nil
	case "apply_entry":
		var m gymentry.OwnedMutationResult
		if decode(&m, gymentry.OwnedMaxOperationSize) != nil || m.Version != 1 || m.Receipt.OperationID != req.OperationID {
			return r, nil, bad
		}
		if _, e := gymentry.ValidateOwnedReceipt(expected.EntryPin, m.Receipt); e != nil {
			return r, nil, bad
		}
		return r, m, nil
	case "event_status":
		var s gymevent.OwnedEventStatus
		if decode(&s, gymevent.OwnedMaxMetadataSize) != nil {
			return r, nil, bad
		}
		if _, e := gymevent.ValidateOwnedEventStatus(expected.EventPin, s); e != nil || s.Event.Saturday != req.Saturday {
			return r, nil, bad
		}
		return r, s, nil
	case "event_record":
		v, e := gymevent.ParseOwnedSnapshotReceipt(expected.EventPin, r.Result)
		if e != nil || v.Input.Event.Saturday != req.Saturday {
			return r, nil, bad
		}
		return r, v, nil
	case "transfer_prepare", "transfer_resume":
		var v OwnedTransferReview
		if decode(&v, 8192) != nil || v.RequestID != req.RequestID || v.Details.RequestID != v.RequestID || v.ProspectID != req.ProspectID || !payoutNonzeroID(v.ReviewID) || !ownedReviewMatches(v.Details, *expected) || req.Command == "transfer_prepare" && v.Details.RecipientGym != req.RecipientGym {
			return r, nil, bad
		}
		return r, v, nil
	case "transfer_review_delivery":
		var v OwnedTransferDeliveryReview
		if decode(&v, 8192) != nil || v.RequestID != req.RequestID || v.Details.RequestID != v.RequestID || v.Leg != req.Leg || !payoutNonzeroID(v.ReviewID) || !ownedcodec.Hex(v.TxID, 32) || !ownedcodec.Hex(v.RawSHA256, 32) || v.TransferID != v.Details.TransferID || v.ExpiresAtMS <= 0 || v.ExpiresAtMS > fightercarrier.MaxTimeMS || !ownedReviewMatches(v.Details, *expected) {
			return r, nil, bad
		}
		return r, v, nil
	case "transfer_finality":
		var v OwnedTransferFinality
		if decode(&v, 8192) != nil || v.RequestID != req.RequestID || ValidateOwnedTransferFinality(v, expected.Source.Ownership) != nil {
			return r, nil, bad
		}
		return r, v, nil
	case "transfer_confirm", "transfer_deliver", "transfer_observe", "transfer_operation", "transfer_abandon":
		o, e := fightercarrier.ParseOperation(r.Result, expected.Carrier.Pin, expected.CarrierBindingID)
		if e != nil || req.RequestID != "" && o.RequestID != req.RequestID || req.Command == "transfer_abandon" && o.Stage != "abandoned" {
			return r, nil, bad
		}
		return r, o, nil
	}
	return r, nil, bad
}

func ownedReviewMatches(v fightercarrier.Review, s OwnedGymScope) bool {
	return fightercarrier.ValidateReview(v) == nil && v.Kind == "transfer" && v.SenderGym == s.Carrier.Pin.GymPublicKey && v.OwnershipPinID == s.Carrier.Pin.OwnershipPinID && v.BindingID == s.CarrierBindingID
}

func HandleOwnedGymRequest(ctx context.Context, r OwnedGymRequest, c OwnedGymControllerAPI) OwnedGymResponse {
	response := OwnedGymResponse{Version: 1, ID: r.ID, Command: r.Command, ViewID: r.ViewID, Result: json.RawMessage("null"), Error: "invalid_request"}
	if ctx == nil || c == nil || ValidateOwnedGymRequest(r) != nil {
		return response
	}
	var result any
	var err error
	snapshot := false
	switch r.Command {
	case "scope":
		result, err = c.Scope(ctx, r.Fixture)
	case "snapshot", "startup":
		snapshot = true
	case "login_fixture":
		err = c.LoginFixture(r.Fixture)
		snapshot = true
	case "logout":
		err = c.Logout(ctx, r.ViewID)
		snapshot = true
	case "reset_fixture":
		err = c.ResetFixtureAccount(ctx, r.ViewID)
		snapshot = true
	case "resume_reset":
		err = c.ResumePendingReset(ctx, r.OperationID)
		snapshot = true
	case "start":
		err = c.Start(ctx, r.ViewID)
		snapshot = true
	case "stop":
		err = c.Stop(ctx, r.ViewID)
		snapshot = true
	case "capture_stable":
		result, err = c.CaptureStable(ctx, r.ViewID, r.CaptureID)
	case "stable_page":
		result, err = c.StablePage(ctx, r.ViewID, r.CaptureID, r.After, r.Limit)
	case "prepare_slot":
		result, err = c.PrepareSlot(ctx, r.ViewID, *r.Slot)
	case "apply_slot":
		result, err = c.ApplySlot(ctx, r.ViewID, r.OperationID)
	case "slot_operation":
		result, err = c.SlotOperation(ctx, r.ViewID, r.OperationID)
	case "pending_slot":
		result, err = c.PendingSlot(ctx, r.ViewID)
	case "abandon_slot":
		result, err = c.AbandonSlot(ctx, r.ViewID, r.OperationID)
	case "capture_history":
		result, err = c.CaptureHistory(ctx, r.ViewID, r.ProspectID)
	case "history_page":
		result, err = c.HistoryPage(ctx, r.ViewID, r.CaptureID, r.After, r.Limit)
	case "entry_upcoming":
		result, err = c.EntryUpcoming(ctx, r.ViewID)
	case "entry_status":
		result, err = c.EntryStatus(ctx, r.ViewID, r.Saturday)
	case "prepare_entry":
		result, err = c.PrepareEntry(ctx, r.ViewID, *r.Entry)
	case "apply_entry":
		result, err = c.ApplyEntry(ctx, r.ViewID, r.OperationID)
	case "entry_operation":
		result, err = c.EntryOperation(ctx, r.ViewID, r.OperationID)
	case "pending_entry":
		result, err = c.PendingEntry(ctx, r.ViewID)
	case "abandon_entry":
		result, err = c.AbandonEntry(ctx, r.ViewID, r.OperationID)
	case "event_status":
		result, err = c.EventStatus(ctx, r.ViewID, r.Saturday)
	case "event_record":
		result, err = c.EventRecord(ctx, r.ViewID, r.Saturday)
	case "transfer_prepare":
		result, err = c.TransferPrepare(ctx, r.ViewID, r.RequestID, r.ProspectID, r.RecipientGym)
	case "transfer_resume":
		result, err = c.TransferResume(ctx, r.ViewID, r.RequestID, r.ProspectID)
	case "transfer_confirm":
		result, err = c.TransferConfirm(ctx, r.ViewID, r.ReviewID)
	case "transfer_review_delivery":
		result, err = c.TransferReviewDelivery(ctx, r.ViewID, r.RequestID, r.Leg)
	case "transfer_deliver":
		result, err = c.TransferDeliver(ctx, r.ViewID, r.ReviewID)
	case "transfer_observe":
		result, err = c.TransferObserve(ctx, r.ViewID, r.RequestID, r.Leg)
	case "transfer_operation":
		result, err = c.TransferOperation(ctx, r.ViewID, r.RequestID)
	case "transfer_finality":
		result, err = c.TransferFinality(ctx, r.ViewID, r.RequestID)
	case "transfer_abandon":
		result, err = c.TransferAbandon(ctx, r.ViewID, r.RequestID)
	case "export_recovery":
		err = c.ExportRecovery(ctx, r.ViewID, *r.Selection)
		snapshot = true
	case "verify_backup":
		err = c.VerifyBackup(ctx, r.ViewID, *r.Selection)
		snapshot = true
	case "cancel_backup":
		err = c.CancelBackup(ctx, r.ViewID)
		snapshot = true
	default:
		return response
	}
	if err != nil {
		response.Error = OwnedGymError(err)
		return response
	}
	if snapshot {
		result = c.Snapshot()
	}
	raw, e := json.Marshal(result)
	if e != nil || len(raw) > MaxOwnedGymResponseSize-512 {
		response.Error = "outcome_unknown"
		return response
	}
	response.OK, response.Error, response.Result = true, "", raw
	return response
}

// ServeOwnedGym is a distinct private pipe. Requiring owned closeable streams
// makes cancellation join the blocked reader/writer rather than abandon it.
// IDs strictly increase; a caller recovers a mutation using its own durable ID
// and a new lookup request, never by replaying the last transport request.
func ServeOwnedGym(ctx context.Context, input io.ReadCloser, output io.WriteCloser, c OwnedGymControllerAPI) (resultErr error) {
	if ctx == nil || input == nil || output == nil || c == nil {
		return gymstable.ErrInvalid
	}
	ctx, cancel := context.WithCancel(ctx)
	joined := make(chan struct{})
	go func() { defer close(joined); <-ctx.Done(); _ = input.Close(); _ = output.Close(); _ = c.Close() }()
	defer func() { cancel(); <-joined; resultErr = errors.Join(resultErr, c.Close()) }()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024), MaxOwnedGymRequestSize+1)
	var last uint64
	// These are bounded public configuration copies, never account capabilities.
	// Normal account transitions temporarily have no child and Scope may refuse
	// while the transition owns the controller gate. Retain the exact original
	// scope before dispatch instead of mistaking those states for startup.
	sealedScopes := make(map[string]OwnedGymScope, 2)
	var source *OwnedSourceConfig
	for scanner.Scan() {
		req, e := ParseOwnedGymRequest(scanner.Bytes())
		if e != nil || req.ID <= last || req.Command == "startup" && last != 0 {
			return gymstable.ErrInvalid
		}
		last = req.ID
		if e = ctx.Err(); e != nil {
			return e
		}
		before := c.Snapshot()
		if ValidateOwnedGymSource(before.Source) != nil || source != nil && before.Source != *source {
			return gymstable.ErrInvalid
		}
		if source == nil {
			value := before.Source
			source = &value
		}
		fixture := before.Account.Fixture
		if req.Command == "scope" || req.Command == "login_fixture" {
			fixture = req.Fixture
		}
		if fixture == "" {
			fixture = "zero256"
		}
		bootstrap := req.Command == "startup" || len(sealedScopes) == 0 && before.View == nil && before.Child == nil && stableOne(req.Command, "snapshot", "resume_reset")
		var scope OwnedGymScope
		if !bootstrap {
			var sealed bool
			scope, sealed = sealedScopes[fixture]
			if !sealed {
				if !stableOne(fixture, "zero256", "incrementing256") || len(sealedScopes) >= 2 {
					return gymstable.ErrInvalid
				}
				value, scopeErr := c.Scope(ctx, fixture)
				if scopeErr != nil {
					return scopeErr
				}
				if ValidateOwnedGymScope(value) != nil || value.Fixture != fixture || value.Source != *source {
					return gymstable.ErrInvalid
				}
				// Strict re-decoding owns nested slices independently of the
				// controller's returned DTO. No caller can mutate the sealed pin.
				raw, scopeErr := json.Marshal(value)
				if scopeErr != nil || ownedcodec.Decode(raw, MaxOwnedGymResponseSize, &scope) != nil {
					return gymstable.ErrInvalid
				}
				sealedScopes[fixture] = scope
			}
		}
		var response OwnedGymResponse
		if req.ViewID != "" && (before.View == nil || before.View.ViewID != req.ViewID || !scope.ViewMatches(*before.View)) {
			response = OwnedGymResponse{Version: 1, ID: req.ID, Command: req.Command, ViewID: req.ViewID, Result: json.RawMessage("null"), Error: "stale_view"}
		} else {
			response = HandleOwnedGymRequest(ctx, req, c)
		}
		raw, e := json.Marshal(response)
		if e != nil {
			return e
		}
		if bootstrap {
			_, _, e = ParseOwnedGymBootstrapResponse(raw, req, source)
		} else {
			_, _, e = ParseOwnedGymResponse(raw, req, &scope, before.View)
		}
		if e != nil {
			// Preserve possible mutation success when post-action disclosure is
			// no longer attributable to the original view.
			response = OwnedGymResponse{Version: 1, ID: req.ID, Command: req.Command, ViewID: req.ViewID, Result: json.RawMessage("null"), Error: "outcome_unknown"}
			raw, _ = json.Marshal(response)
		}
		raw = append(raw, '\n')
		if n, e := output.Write(raw); e != nil || n != len(raw) {
			return errors.Join(e, io.ErrShortWrite)
		}
	}
	return scanner.Err()
}

// Bootstrap is restricted to a childless owner. It learns only the original
// audited outer setup source and pending reset ID, never a wallet capability.
// Native alone enters this parser before its two complete scope handshakes.
func ParseOwnedGymBootstrapResponse(raw []byte, req OwnedGymRequest, source *OwnedSourceConfig) (OwnedGymResponse, any, error) {
	var r OwnedGymResponse
	bad := gymstable.ErrInvalid
	if !stableOne(req.Command, "startup", "snapshot", "resume_reset") || req.ViewID != "" || ValidateOwnedGymRequest(req) != nil || ownedcodec.Decode(raw, MaxOwnedGymResponseSize, &r) != nil || r.Version != 1 || r.ID != req.ID || r.Command != req.Command || r.ViewID != "" {
		return r, nil, bad
	}
	if !r.OK {
		if string(r.Result) != "null" || !ownedGymErrorCode(r.Error) {
			return r, nil, bad
		}
		return r, nil, nil
	}
	var s OwnedGymSnapshot
	if r.Error != "" || ownedcodec.Decode(r.Result, MaxOwnedGymResponseSize, &s) != nil || s.View != nil || s.Child != nil || !stableOne(s.State, "signed_out", "failed", "resetting") || s.State == "failed" && s.PendingResetID == "" {
		return r, nil, bad
	}
	expected := s.Source
	if source != nil {
		expected = *source
	}
	if ValidateOwnedGymSource(expected) != nil || ValidateOwnedGymSnapshot(s, expected, gymevent.OwnedPin{}) != nil {
		return r, nil, bad
	}
	return r, s, nil
}
