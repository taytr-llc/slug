// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
)

// Native retains original contexts and consent. The webview may select public
// identifiers, but cannot supply a helper, source pin, signed bytes or consent.
type nativeOwnedState struct {
	recoveryCleanup *nativeOwnedRecoveryCleanup
	mu              sync.Mutex
	action          *nativeOwnedTransfer
	confirm         func(string, string, func(bool)) error
}
type nativeOwnedTransfer struct {
	helper         *bridge.OwnedBridge
	owner          *bridge.OwnedContext
	intent         bridge.OwnedNativeIntent
	review         *pilot.OwnedTransferReview
	delivery       *pilot.OwnedTransferDeliveryReview
	operation      *fightercarrier.Operation
	state, failure string
	answered       bool
}
type OwnedTransferStatus struct {
	Version      uint32                             `json:"version"`
	ViewID       string                             `json:"view_id"`
	RequestID    string                             `json:"request_id"`
	ProspectID   string                             `json:"prospect_id"`
	RecipientGym string                             `json:"recipient_gym"`
	State        string                             `json:"state"`
	Review       *pilot.OwnedTransferReview         `json:"review"`
	Delivery     *pilot.OwnedTransferDeliveryReview `json:"delivery"`
	Operation    *fightercarrier.Operation          `json:"operation"`
	Error        string                             `json:"error"`
}

func ownedNativeCopy[T any](in T) T {
	var out T
	raw, _ := json.Marshal(in)
	_ = json.Unmarshal(raw, &out)
	return out
}
func (a *nativeOwnedTransfer) snapshot() OwnedTransferStatus {
	return ownedNativeCopy(OwnedTransferStatus{Version: 1, ViewID: a.owner.View().ViewID, RequestID: a.intent.RequestID, ProspectID: a.intent.ProspectID, RecipientGym: a.intent.RecipientGym, State: a.state, Review: a.review, Delivery: a.delivery, Operation: a.operation, Error: a.failure})
}
func (g *Gym) OwnedSnapshot() (pilot.OwnedGymSnapshot, error) {
	b := g.ownedHelper
	if b == nil {
		return pilot.OwnedGymSnapshot{}, bridge.ErrUnavailable
	}
	reply, e := b.Request(context.Background(), nil, pilot.OwnedGymRequest{Command: "snapshot"})
	if e != nil {
		return pilot.OwnedGymSnapshot{}, e
	}
	s, ok := reply.Value.(pilot.OwnedGymSnapshot)
	if !ok || g.ownedHelper != b {
		return pilot.OwnedGymSnapshot{}, bridge.ErrUnavailable
	}
	g.owned.mu.Lock()
	if a := g.owned.action; a != nil && (a.helper != b || !b.ContextMatches(a.owner)) {
		a.answered = true
		a.state = "revoked"
		a.review = nil
		a.delivery = nil
	}
	g.owned.mu.Unlock()
	return s, nil
}
func (g *Gym) ownedContext(view string) (*bridge.OwnedBridge, *bridge.OwnedContext, error) {
	b := g.ownedHelper
	if b == nil || view == "" {
		return nil, nil, bridge.ErrUnavailable
	}
	owner, e := b.Context()
	if e != nil || owner.View().ViewID != view || g.ownedHelper != b {
		return nil, nil, &bridge.OwnedFailure{Code: "stale_view"}
	}
	return b, owner, nil
}
func (g *Gym) ownedRequest(view string, r pilot.OwnedGymRequest) (bridge.OwnedReply, error) {
	b, owner, e := g.ownedContext(view)
	if e != nil {
		return bridge.OwnedReply{}, e
	}
	reply, e := b.Request(context.Background(), owner, r)
	if e != nil {
		return reply, e
	}
	// Logout/reset intentionally revoke this context. Their validated snapshot
	// is the complete result; all other operations require the original owner.
	if g.ownedHelper != b || r.Command != "logout" && r.Command != "reset_fixture" && !b.ContextMatches(owner) {
		return bridge.OwnedReply{}, &bridge.OwnedFailure{Code: "stale_view"}
	}
	return reply, nil
}
func (g *Gym) OwnedLoginFixture(fixture string) error {
	b := g.ownedHelper
	if b == nil {
		return bridge.ErrUnavailable
	}
	_, e := b.Request(context.Background(), nil, pilot.OwnedGymRequest{Command: "login_fixture", Fixture: fixture})
	return e
}
func (g *Gym) OwnedLogout(view string) error {
	_, e := g.ownedRequest(view, pilot.OwnedGymRequest{Command: "logout"})
	return e
}
func (g *Gym) OwnedResetFixture(view string) error {
	_, e := g.ownedRequest(view, pilot.OwnedGymRequest{Command: "reset_fixture"})
	return e
}
func (g *Gym) OwnedResumeReset() error {
	s, e := g.OwnedSnapshot()
	if e != nil || s.PendingResetID == "" {
		return bridge.ErrUnavailable
	}
	_, e = g.ownedHelper.Request(context.Background(), nil, pilot.OwnedGymRequest{Command: "resume_reset", OperationID: s.PendingResetID})
	return e
}
func (g *Gym) OwnedStart(view string) error {
	_, e := g.ownedRequest(view, pilot.OwnedGymRequest{Command: "start"})
	return e
}
func (g *Gym) OwnedStop(view string) error {
	_, e := g.ownedRequest(view, pilot.OwnedGymRequest{Command: "stop"})
	return e
}

// Called synchronously by the native menu callback, before its worker starts.
// The returned operation uses exactly this context once; it never polls for a
// fresh account or retries Busy against a replacement view.
func (g *Gym) ownedMenuAction(command string) func() error {
	b := g.ownedHelper
	original, capturedErr := b.Context()
	return func() error {
		if capturedErr != nil {
			return capturedErr
		}
		if command != "start" && command != "stop" {
			return bridge.ErrUnavailable
		}
		if g.ownedHelper != b || !b.ContextMatches(original) {
			return &bridge.OwnedFailure{Code: "stale_view"}
		}
		_, e := b.Request(context.Background(), original, pilot.OwnedGymRequest{Command: command})
		if e != nil {
			return e
		}
		if g.ownedHelper != b || !b.ContextMatches(original) {
			return &bridge.OwnedFailure{Code: "stale_view"}
		}
		return nil
	}
}
func ownedValue[T any](reply bridge.OwnedReply, e error) (T, error) {
	var zero T
	if e != nil {
		return zero, e
	}
	v, ok := reply.Value.(T)
	if !ok {
		return zero, bridge.ErrUnavailable
	}
	return v, nil
}
func (g *Gym) OwnedCaptureStable(view string) (pilot.OwnedStableCapture, error) {
	id, e := withdrawalRandomID()
	if e != nil {
		return pilot.OwnedStableCapture{}, e
	}
	return ownedValue[pilot.OwnedStableCapture](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "capture_stable", CaptureID: id}))
}
func (g *Gym) OwnedStablePage(view, capture string, after, limit uint32) (gymstable.OwnedPage, error) {
	return ownedValue[gymstable.OwnedPage](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "stable_page", CaptureID: capture, After: after, Limit: limit}))
}
func (g *Gym) OwnedCaptureHistory(view, prospect string) (gymstable.OwnedCareerHistoryCapture, error) {
	return ownedValue[gymstable.OwnedCareerHistoryCapture](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "capture_history", ProspectID: prospect}))
}
func (g *Gym) OwnedHistoryPage(view, capture string, after, limit uint32) (gymstable.OwnedCareerHistoryPage, error) {
	return ownedValue[gymstable.OwnedCareerHistoryPage](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "history_page", CaptureID: capture, After: after, Limit: limit}))
}
func (g *Gym) OwnedPrepareSlot(view string, r gymstable.OwnedSlotRequest) (gymstable.OwnedIntent, error) {
	return ownedValue[gymstable.OwnedIntent](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "prepare_slot", Slot: &r}))
}
func (g *Gym) OwnedApplySlot(view, id string) (gymstable.OwnedMutationResult, error) {
	return ownedValue[gymstable.OwnedMutationResult](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "apply_slot", OperationID: id}))
}
func (g *Gym) OwnedPendingSlot(view string) (*gymstable.OwnedOperation, error) {
	reply, e := g.ownedRequest(view, pilot.OwnedGymRequest{Command: "pending_slot"})
	if e != nil {
		return nil, e
	}
	if v, ok := reply.Value.(*gymstable.OwnedOperation); ok {
		return v, nil
	}
	if v, ok := reply.Value.(gymstable.OwnedOperation); ok {
		return &v, nil
	}
	return nil, bridge.ErrUnavailable
}
func (g *Gym) OwnedSlotOperation(view, id string) (gymstable.OwnedOperation, error) {
	return ownedValue[gymstable.OwnedOperation](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "slot_operation", OperationID: id}))
}
func (g *Gym) OwnedAbandonSlot(view, id string) (gymstable.OwnedOperation, error) {
	return ownedValue[gymstable.OwnedOperation](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "abandon_slot", OperationID: id}))
}
func (g *Gym) OwnedEntryUpcoming(view string) (gymentry.OwnedEventStatus, error) {
	return ownedValue[gymentry.OwnedEventStatus](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "entry_upcoming"}))
}
func (g *Gym) OwnedEntryStatus(view, date string) (gymentry.OwnedEventStatus, error) {
	return ownedValue[gymentry.OwnedEventStatus](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "entry_status", Saturday: date}))
}
func (g *Gym) OwnedPrepareEntry(view string, r gymentry.Request) (gymentry.OwnedIntent, error) {
	return ownedValue[gymentry.OwnedIntent](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "prepare_entry", Entry: &r}))
}
func (g *Gym) OwnedApplyEntry(view, id string) (gymentry.OwnedMutationResult, error) {
	return ownedValue[gymentry.OwnedMutationResult](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "apply_entry", OperationID: id}))
}
func (g *Gym) OwnedPendingEntry(view string) (*gymentry.OwnedOperation, error) {
	reply, e := g.ownedRequest(view, pilot.OwnedGymRequest{Command: "pending_entry"})
	if e != nil {
		return nil, e
	}
	if v, ok := reply.Value.(*gymentry.OwnedOperation); ok {
		return v, nil
	}
	if v, ok := reply.Value.(gymentry.OwnedOperation); ok {
		return &v, nil
	}
	return nil, bridge.ErrUnavailable
}
func (g *Gym) OwnedEntryOperation(view, id string) (gymentry.OwnedOperation, error) {
	return ownedValue[gymentry.OwnedOperation](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "entry_operation", OperationID: id}))
}
func (g *Gym) OwnedAbandonEntry(view, id string) (gymentry.OwnedOperation, error) {
	return ownedValue[gymentry.OwnedOperation](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "abandon_entry", OperationID: id}))
}
func (g *Gym) OwnedEventStatus(view, date string) (gymevent.OwnedEventStatus, error) {
	return ownedValue[gymevent.OwnedEventStatus](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "event_status", Saturday: date}))
}
func (g *Gym) OwnedEventRecord(view, date string) (gymevent.OwnedSnapshotReceipt, error) {
	return ownedValue[gymevent.OwnedSnapshotReceipt](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "event_record", Saturday: date}))
}
func (g *Gym) OwnedTransferRequests(view string) ([]bridge.OwnedNativeIntent, error) {
	_, o, e := g.ownedContext(view)
	if e != nil || g.ownedIntents == nil {
		return nil, bridge.ErrUnavailable
	}
	return g.ownedIntents.List(o)
}
func (g *Gym) beginOwnedTransfer(view, request, prospect, recipient string) (*nativeOwnedTransfer, error) {
	b, o, e := g.ownedContext(view)
	if e != nil || g.ownedIntents == nil {
		return nil, bridge.ErrUnavailable
	}
	g.owned.mu.Lock()
	defer g.owned.mu.Unlock()
	if a := g.owned.action; a != nil && a.helper == b && a.owner == o && (a.state == "preparing" || a.state == "confirming" || a.state == "dialog" || a.state == "delivering") {
		return nil, bridge.ErrBusy
	}
	if a := g.owned.action; request == "" && a != nil && a.helper == b && a.owner == o && a.state == "unknown" {
		return nil, &bridge.OwnedFailure{Code: "outcome_unknown"}
	}
	var intent bridge.OwnedNativeIntent
	if request == "" {
		intent, e = g.ownedIntents.Prepare(context.Background(), o, prospect, recipient)
	} else {
		intent, e = g.ownedIntents.Lookup(o, request)
	}
	if e != nil {
		return nil, e
	}
	a := &nativeOwnedTransfer{helper: b, owner: o, intent: intent, state: "preparing"}
	g.owned.action = a
	return a, nil
}
func (g *Gym) OwnedPrepareTransfer(view, prospect, recipient string) (OwnedTransferStatus, error) {
	a, e := g.beginOwnedTransfer(view, "", prospect, recipient)
	if e != nil {
		return OwnedTransferStatus{}, e
	}
	return g.prepareOwnedTransfer(a, "transfer_prepare")
}
func (g *Gym) OwnedResumeTransfer(view, request string) (OwnedTransferStatus, error) {
	a, e := g.beginOwnedTransfer(view, request, "", "")
	if e != nil {
		return OwnedTransferStatus{}, e
	}
	return g.prepareOwnedTransfer(a, "transfer_resume")
}
func (g *Gym) prepareOwnedTransfer(a *nativeOwnedTransfer, command string) (OwnedTransferStatus, error) {
	req := pilot.OwnedGymRequest{Command: command, RequestID: a.intent.RequestID, ProspectID: a.intent.ProspectID}
	if command == "transfer_prepare" {
		req.RecipientGym = a.intent.RecipientGym
	}
	reply, e := a.helper.Request(context.Background(), a.owner, req)
	g.owned.mu.Lock()
	defer g.owned.mu.Unlock()
	if !g.ownedCurrent(a) {
		return OwnedTransferStatus{}, &bridge.OwnedFailure{Code: "stale_view"}
	}
	if e != nil {
		ownedTransferFailure(a, e)
		return a.snapshot(), e
	}
	review, ok := reply.Value.(pilot.OwnedTransferReview)
	if !ok || review.RequestID != a.intent.RequestID || review.ProspectID != a.intent.ProspectID || review.Details.RecipientGym != a.intent.RecipientGym {
		a.state = "unknown"
		a.failure = "outcome_unknown"
		return a.snapshot(), bridge.ErrUnavailable
	}
	a.review = &review
	a.state = "review"
	return a.snapshot(), nil
}

// Requires native state lock. This checks the retained context, never a view
// reconstructed from webview input or the current account after a dialog.
func (g *Gym) ownedCurrent(a *nativeOwnedTransfer) bool {
	return a != nil && g.owned.action == a && g.ownedHelper == a.helper && a.helper.ContextMatches(a.owner)
}
func ownedTransferFailure(a *nativeOwnedTransfer, e error) {
	a.state = "unknown"
	a.failure = "outcome_unknown"
	var failure *bridge.OwnedFailure
	if errors.As(e, &failure) {
		a.failure = failure.Code
		if failure.Code != "outcome_unknown" {
			a.state = "failed"
		}
	}
	if errors.Is(e, bridge.ErrBusy) {
		a.failure = "busy"
		a.state = "failed"
	}
}
func (g *Gym) OwnedTransferStatus(view, request string) (OwnedTransferStatus, error) {
	_, o, e := g.ownedContext(view)
	if e != nil {
		return OwnedTransferStatus{}, e
	}
	g.owned.mu.Lock()
	defer g.owned.mu.Unlock()
	a := g.owned.action
	if a == nil || a.owner != o || a.intent.RequestID != request || !g.ownedCurrent(a) {
		return OwnedTransferStatus{}, &bridge.OwnedFailure{Code: "not_found"}
	}
	return a.snapshot(), nil
}
func (g *Gym) OwnedTransferOperation(view, request string) (fightercarrier.Operation, error) {
	return g.ownedTransferOperation(view, request, "transfer_operation", "")
}
func (g *Gym) OwnedObserveTransfer(view, request, leg string) (fightercarrier.Operation, error) {
	return g.ownedTransferOperation(view, request, "transfer_observe", leg)
}
func (g *Gym) OwnedAbandonTransfer(view, request string) (fightercarrier.Operation, error) {
	return g.ownedTransferOperation(view, request, "transfer_abandon", "")
}
func (g *Gym) ownedTransferOperation(view, request, command, leg string) (fightercarrier.Operation, error) {
	b, o, e := g.ownedContext(view)
	if e != nil || g.ownedIntents == nil {
		return fightercarrier.Operation{}, bridge.ErrUnavailable
	}
	intent, e := g.ownedIntents.Lookup(o, request)
	if e != nil {
		return fightercarrier.Operation{}, e
	}
	g.owned.mu.Lock()
	a := g.owned.action
	if a != nil && g.ownedCurrent(a) && (a.state == "dialog" || a.state == "preparing" || a.state == "confirming" || a.state == "delivering") {
		g.owned.mu.Unlock()
		return fightercarrier.Operation{}, bridge.ErrBusy
	}
	a = &nativeOwnedTransfer{helper: b, owner: o, intent: intent, state: "preparing"}
	g.owned.action = a
	g.owned.mu.Unlock()
	reply, e := b.Request(context.Background(), o, pilot.OwnedGymRequest{Command: command, RequestID: request, Leg: leg})
	g.owned.mu.Lock()
	defer g.owned.mu.Unlock()
	if !g.ownedCurrent(a) {
		return fightercarrier.Operation{}, &bridge.OwnedFailure{Code: "stale_view"}
	}
	if e != nil {
		ownedTransferFailure(a, e)
		return fightercarrier.Operation{}, e
	}
	operation, ok := reply.Value.(fightercarrier.Operation)
	if !ok || operation.RequestID != intent.RequestID || operation.Request.RecipientGym != intent.RecipientGym {
		a.state = "unknown"
		return fightercarrier.Operation{}, bridge.ErrUnavailable
	}
	a.operation = &operation
	a.state = operation.Stage
	return ownedNativeCopy(operation), nil
}
func (g *Gym) OwnedConfirmTransfer(view, request string) (OwnedTransferStatus, error) {
	_, o, e := g.ownedContext(view)
	if e != nil {
		return OwnedTransferStatus{}, e
	}
	g.owned.mu.Lock()
	a := g.owned.action
	if !g.ownedCurrent(a) || a.owner != o || a.intent.RequestID != request || a.state != "review" || a.review == nil || a.answered {
		g.owned.mu.Unlock()
		return OwnedTransferStatus{}, &bridge.OwnedFailure{Code: "stale_view"}
	}
	a.state = "dialog"
	review := *a.review
	confirm := g.owned.confirm
	out := a.snapshot()
	g.owned.mu.Unlock()
	if confirm == nil {
		confirm = g.showOwnedConfirmation
	}
	if e = confirm("Sign fighter transfer", ownedTransferMessage(a.intent.ProspectID, review.Details), func(yes bool) { go g.answerOwnedTransfer(a, yes, false) }); e != nil {
		g.answerOwnedTransfer(a, false, false)
		return OwnedTransferStatus{}, e
	}
	return out, nil
}
func (g *Gym) OwnedDeliverTransfer(view, request, leg string) (OwnedTransferStatus, error) {
	a, e := g.beginOwnedTransfer(view, request, "", "")
	if e != nil {
		return OwnedTransferStatus{}, e
	}
	reply, e := a.helper.Request(context.Background(), a.owner, pilot.OwnedGymRequest{Command: "transfer_review_delivery", RequestID: request, Leg: leg})
	g.owned.mu.Lock()
	if !g.ownedCurrent(a) {
		g.owned.mu.Unlock()
		return OwnedTransferStatus{}, &bridge.OwnedFailure{Code: "stale_view"}
	}
	if e != nil {
		ownedTransferFailure(a, e)
		out := a.snapshot()
		g.owned.mu.Unlock()
		return out, e
	}
	review, ok := reply.Value.(pilot.OwnedTransferDeliveryReview)
	if !ok || review.RequestID != request || review.Details.RecipientGym != a.intent.RecipientGym {
		a.state = "unknown"
		g.owned.mu.Unlock()
		return OwnedTransferStatus{}, bridge.ErrUnavailable
	}
	a.delivery = &review
	a.state = "dialog"
	confirm := g.owned.confirm
	out := a.snapshot()
	g.owned.mu.Unlock()
	if confirm == nil {
		confirm = g.showOwnedConfirmation
	}
	message := ownedTransferMessage(a.intent.ProspectID, review.Details) + "\n\nSend the retained " + review.Leg + " transaction:\n" + review.TxID + "\n\nA retry sends these same signed bytes. Ownership changes only after its signed checkpoint and 60 descendant blocks."
	if e = confirm("Broadcast fighter transfer", message, func(yes bool) { go g.answerOwnedTransfer(a, yes, true) }); e != nil {
		g.answerOwnedTransfer(a, false, true)
		return OwnedTransferStatus{}, e
	}
	return out, nil
}
func ownedTransferMessage(prospect string, r fightercarrier.Review) string {
	return "Candidate network fighter transfer\n\nFighter ID:\n" + prospect + "\n\nFrom gym:\n" + r.SenderGym + "\n\nTo gym:\n" + r.RecipientGym + "\n\nTotal network fee: " + withdrawalAmount(r.TotalFeeAtoms) + "\nCommit fee: " + withdrawalAmount(r.CommitFeeAtoms) + "\nReveal fee: " + withdrawalAmount(r.SpendFeeAtoms) + "\n\nReturn address:\n" + r.ReturnAddress + fmt.Sprintf("\nReturn path: native SegWit branch %d, index %d", r.ReturnBranch, r.ReturnIndex) + "\nCommit change: " + withdrawalAmount(r.CommitChangeAtoms) + "\nEscrow returned: " + withdrawalAmount(r.ReturnedAtoms)
}
func (g *Gym) showOwnedConfirmation(title, message string, answer func(bool)) error {
	if g.app == nil || g.window == nil {
		return bridge.ErrUnavailable
	}
	d := g.app.Dialog.Question().SetTitle(title).SetMessage(message).AttachToWindow(g.window)
	no := d.AddButton("Cancel").OnClick(func() { answer(false) })
	d.SetDefaultButton(no).SetCancelButton(no)
	d.AddButton(title).OnClick(func() { answer(true) })
	d.Show()
	return nil
}
func (a *nativeOwnedTransfer) takeAnswer(current, yes, delivery bool) (pilot.OwnedGymRequest, bool) {
	if !current {
		a.answered, a.state, a.review, a.delivery = true, "revoked", nil, nil
		return pilot.OwnedGymRequest{}, false
	}
	if a.answered || a.state != "dialog" {
		return pilot.OwnedGymRequest{}, false
	}
	a.answered = true
	if !yes {
		a.state = "cancelled"
		a.review = nil
		a.delivery = nil
		return pilot.OwnedGymRequest{}, false
	}
	r := pilot.OwnedGymRequest{Command: "transfer_confirm"}
	if delivery {
		if a.delivery == nil {
			a.state, a.failure = "failed", "stale_view"
			return r, false
		}
		r.Command = "transfer_deliver"
		r.ReviewID = a.delivery.ReviewID
		a.state = "delivering"
	} else {
		if a.review == nil {
			a.state, a.failure = "failed", "stale_view"
			return r, false
		}
		r.ReviewID = a.review.ReviewID
		a.state = "confirming"
	}
	return r, true
}
func (g *Gym) answerOwnedTransfer(a *nativeOwnedTransfer, yes, delivery bool) {
	g.owned.mu.Lock()
	r, send := a.takeAnswer(g.ownedCurrent(a), yes, delivery)
	g.owned.mu.Unlock()
	if !send {
		return
	}
	reply, e := a.helper.Request(context.Background(), a.owner, r)
	g.owned.mu.Lock()
	defer g.owned.mu.Unlock()
	if !g.ownedCurrent(a) {
		return
	}
	a.review = nil
	a.delivery = nil
	if e != nil {
		ownedTransferFailure(a, e)
		return
	}
	op, ok := reply.Value.(fightercarrier.Operation)
	if !ok || op.RequestID != a.intent.RequestID || op.Request.RecipientGym != a.intent.RecipientGym {
		a.state = "unknown"
		a.failure = "outcome_unknown"
		return
	}
	a.operation = &op
	a.state = op.Stage
	a.failure = ""
}

// This separate read displays pending/void/finalized at a completed public
// head. A signed operation or 61 transaction confirmations alone never does.
func (g *Gym) OwnedTransferFinality(view, request string) (pilot.OwnedTransferFinality, error) {
	_, o, e := g.ownedContext(view)
	if e != nil || g.ownedIntents == nil {
		return pilot.OwnedTransferFinality{}, bridge.ErrUnavailable
	}
	row, e := g.ownedIntents.Lookup(o, request)
	if e != nil {
		return pilot.OwnedTransferFinality{}, e
	}
	v, e := ownedValue[pilot.OwnedTransferFinality](g.ownedRequest(view, pilot.OwnedGymRequest{Command: "transfer_finality", RequestID: request}))
	if e == nil && v.State == "finalized" && v.OwnerGym != row.RecipientGym {
		return pilot.OwnedTransferFinality{}, bridge.ErrUnavailable
	}
	return v, e
}
