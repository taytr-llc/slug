// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
	"strconv"
	"strings"
	"sync"
	"time"
)

type nativeWithdrawalState struct {
	mu      sync.Mutex
	action  *nativeWithdrawalAction
	confirm func(pilot.WithdrawalReview, func(bool)) error
}
type nativeWithdrawalAction struct {
	helper   *bridge.PoolBridge
	request  pilot.WithdrawalRequest
	binding  pilot.WithdrawalBinding
	review   *pilot.WithdrawalReview
	state    string
	answered bool
	terminal bool
}

func withdrawalRandomID() (string, error) {
	var b [32]byte
	_, e := rand.Read(b[:])
	return hex.EncodeToString(b[:]), e
}
func withdrawalAtoms(s string) (int64, error) {
	if s == "" || len(s) > 25 || strings.TrimSpace(s) != s {
		return 0, bridge.ErrUnavailable
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, bridge.ErrUnavailable
	}
	for _, p := range parts {
		if p == "" {
			return 0, bridge.ErrUnavailable
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return 0, bridge.ErrUnavailable
			}
		}
	}
	if len(parts[0]) > 1 && parts[0][0] == '0' {
		return 0, bridge.ErrUnavailable
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 8 {
		return 0, bridge.ErrUnavailable
	}
	whole, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || whole > 21000000 {
		return 0, bridge.ErrUnavailable
	}
	f, e := strconv.ParseInt(fraction+strings.Repeat("0", 8-len(fraction)), 10, 64)
	if e != nil {
		return 0, e
	}
	atoms := whole*100000000 + f
	if atoms < 1 || atoms > 2100000000000000 {
		return 0, bridge.ErrUnavailable
	}
	return atoms, nil
}
func withdrawalAmount(atoms int64) string {
	return fmt.Sprintf("%d.%08d SLUG", atoms/100000000, atoms%100000000)
}
func (g *Gym) observeWithdrawalAccount(helper *bridge.PoolBridge, state string, view *pilot.ManifestViewPin) {
	g.withdrawal.mu.Lock()
	defer g.withdrawal.mu.Unlock()
	a := g.withdrawal.action
	if a != nil && (a.helper != helper || state != "signed_in" || view == nil || a.binding.View != *view) {
		a.answered = true
		a.state = "revoked"
	}
}
func (g *Gym) withdrawalScope(view string) (pilot.WithdrawalBinding, pilot.WithdrawalResponse) {
	if g.poolHelper == nil {
		return pilot.WithdrawalBinding{}, pilot.WithdrawalFailure(1, "withdrawal_unavailable")
	}
	snap, e := g.PoolSnapshot()
	if e != nil || snap.View == nil || snap.View.ViewID != view {
		return pilot.WithdrawalBinding{}, pilot.WithdrawalFailure(1, "stale_view")
	}
	out := g.poolHelper.WithdrawalRequest(context.Background(), pilot.WithdrawalRequest{Command: "withdrawal_snapshot"})
	if !out.OK || out.Snapshot == nil || out.Snapshot.Binding == nil {
		return pilot.WithdrawalBinding{}, out
	}
	return *out.Snapshot.Binding, out
}
func (g *Gym) WithdrawalSnapshot() (pilot.WithdrawalResponse, error) {
	if g.poolHelper == nil {
		return pilot.WithdrawalFailure(1, "withdrawal_unavailable"), nil
	}
	// Recover only the current native action. Generic latest IDs never stand in
	// for an uncertain prepare/review/approve response.
	g.withdrawal.mu.Lock()
	a := g.withdrawal.action
	unknown := a != nil && a.state == "unknown"
	g.withdrawal.mu.Unlock()
	if a != nil && a.helper == g.poolHelper && unknown {
		out := a.helper.WithdrawalRequest(context.Background(), pilot.WithdrawalRequest{Command: "withdrawal_action", ViewID: a.request.ViewID, OperationID: a.request.OperationID, ActionID: a.request.ActionID})
		g.withdrawal.mu.Lock()
		if g.withdrawal.action == a {
			if out.OK && out.Action != nil {
				a.state = "admitted"
				a.terminal = out.Action.State != "running" && out.Action.State != "review"
			} else if out.Error == "withdrawal_not_found" {
				a.state = "not_started"
				a.terminal = true
			}
		}
		g.withdrawal.mu.Unlock()
	}
	out := g.poolHelper.WithdrawalRequest(context.Background(), pilot.WithdrawalRequest{Command: "withdrawal_snapshot"})
	if out.OK && out.Snapshot != nil {
		s := *out.Snapshot
		g.withdrawal.mu.Lock()
		a = g.withdrawal.action
		if a == nil || a.helper != g.poolHelper || a.state == "not_started" || a.state == "revoked" {
			s.ActiveActionID, s.LastActionID = "", ""
		} else {
			if s.ActiveActionID != a.request.ActionID {
				s.ActiveActionID = ""
			}
			if s.LastActionID != a.request.ActionID {
				s.LastActionID = ""
			}
			if s.State == "busy" && s.ActiveActionID == "" {
				s.State = "idle"
				s.CanPrepare = false
			}
		}
		g.withdrawal.mu.Unlock()
		out.Snapshot = &s
	}
	return out, nil
}
func (g *Gym) nativeWithdrawalBegin(r pilot.WithdrawalRequest) (pilot.WithdrawalResponse, *nativeWithdrawalAction) {
	b, scope := g.withdrawalScope(r.ViewID)
	if !scope.OK || scope.Snapshot == nil || scope.Snapshot.Binding == nil {
		return scope, nil
	}
	id, e := withdrawalRandomID()
	if e != nil {
		return pilot.WithdrawalFailure(1, "withdrawal_unavailable"), nil
	}
	r.ActionID = id
	if r.OperationID == "" {
		r.OperationID, e = withdrawalRandomID()
		if e != nil {
			return pilot.WithdrawalFailure(1, "withdrawal_unavailable"), nil
		}
	}
	if r.Command == "withdrawal_review" {
		r.ReviewID, e = withdrawalRandomID()
		if e != nil {
			return pilot.WithdrawalFailure(1, "withdrawal_unavailable"), nil
		}
	}
	a := &nativeWithdrawalAction{helper: g.poolHelper, request: r, binding: b, state: "preparing"}
	g.withdrawal.mu.Lock()
	prior := g.withdrawal.action
	if prior != nil && prior.helper == a.helper && prior.binding.View == b.View && !prior.terminal && prior.state != "not_started" && prior.state != "revoked" {
		g.withdrawal.mu.Unlock()
		return pilot.WithdrawalFailure(1, "busy"), nil
	}
	g.withdrawal.action = a
	if e = a.helper.RememberWithdrawal(r); e != nil {
		a.state = "not_started"
		a.terminal = true
		g.withdrawal.mu.Unlock()
		return pilot.WithdrawalFailure(1, "busy"), a
	}
	g.withdrawal.mu.Unlock()
	out := a.helper.WithdrawalRequest(context.Background(), r)
	g.withdrawal.mu.Lock()
	if g.withdrawal.action == a {
		if out.OK {
			a.state = "admitted"
			if out.Action != nil {
				a.terminal = out.Action.State != "running" && out.Action.State != "review"
			}
			if out.Review != nil {
				copy := *out.Review
				a.review = &copy
			}
		} else if out.Error == "outcome_unknown" {
			a.state = "unknown"
		} else {
			a.state = "not_started"
			a.terminal = true
		}
	}
	g.withdrawal.mu.Unlock()
	return out, a
}
func (g *Gym) BeginWithdrawal(view, destination, amountText string) (pilot.WithdrawalResponse, error) {
	atoms, e := withdrawalAtoms(amountText)
	if e != nil {
		return pilot.WithdrawalFailure(1, "invalid_request"), nil
	}
	script, e := pilot.WithdrawalDestination(destination)
	if e != nil {
		return pilot.WithdrawalFailure(1, "invalid_request"), nil
	}
	out, _ := g.nativeWithdrawalBegin(pilot.WithdrawalRequest{Command: "withdrawal_prepare", ViewID: view, Intent: &pilot.WithdrawalIntent{Destination: destination, RecipientScript: script, AmountAtoms: atoms, FeeRateAtomsPerKB: 1000, FeeLimitAtoms: 100000}})
	return out, nil
}
func (g *Gym) WithdrawalOperation(view, operation string) (pilot.WithdrawalResponse, error) {
	if g.poolHelper == nil {
		return pilot.WithdrawalFailure(1, "withdrawal_unavailable"), nil
	}
	if _, s := g.withdrawalScope(view); !s.OK {
		return s, nil
	}
	return g.poolHelper.WithdrawalRequest(context.Background(), pilot.WithdrawalRequest{Command: "withdrawal_operation", ViewID: view, OperationID: operation}), nil
}
func (g *Gym) WithdrawalAction(view, operation, action string) (pilot.WithdrawalResponse, error) {
	a, operation, valid := g.withdrawalActionSelection(view, operation, action)
	if !valid {
		return pilot.WithdrawalFailure(1, "stale_view"), nil
	}
	out := a.helper.WithdrawalRequest(context.Background(), pilot.WithdrawalRequest{Command: "withdrawal_action", ViewID: view, OperationID: operation, ActionID: action})
	g.withdrawal.mu.Lock()
	if g.withdrawal.action == a && out.OK && out.Action != nil {
		a.terminal = out.Action.State != "running" && out.Action.State != "review"
		a.state = "admitted"
	}
	g.withdrawal.mu.Unlock()
	return out, nil
}
func (g *Gym) ReviewWithdrawal(view, operation string) (pilot.WithdrawalResponse, error) {
	return g.reviewWithdrawal(view, operation, "send")
}
func (g *Gym) RetryWithdrawal(view, operation string) (pilot.WithdrawalResponse, error) {
	return g.reviewWithdrawal(view, operation, "retry")
}
func (g *Gym) reviewWithdrawal(view, operation, kind string) (pilot.WithdrawalResponse, error) {
	current, e := g.WithdrawalOperation(view, operation)
	if e != nil || !current.OK || current.Operation == nil {
		return current, e
	}
	o := current.Operation
	if o.Draft == nil {
		return pilot.WithdrawalFailure(1, "operation_conflict"), nil
	}
	r := pilot.WithdrawalRequest{Command: "withdrawal_review", ViewID: view, OperationID: operation, ReviewKind: kind, DraftDigest: o.Draft.DraftDigest}
	if kind == "retry" {
		r.TxID = o.TxID
	}
	out, a := g.nativeWithdrawalBegin(r)
	if !out.OK || out.Review == nil || a == nil {
		return out, nil
	}
	g.withdrawal.mu.Lock()
	live := g.withdrawal.action == a && a.helper == g.poolHelper && a.state != "revoked" && !a.answered
	confirm := g.withdrawal.confirm
	g.withdrawal.mu.Unlock()
	if !live {
		return pilot.WithdrawalFailure(1, "stale_view"), nil
	}
	if confirm == nil {
		confirm = g.showWithdrawalReview
	}
	if e = confirm(*out.Review, func(yes bool) { go g.answerWithdrawal(a, yes) }); e != nil {
		go g.answerWithdrawal(a, false)
		return pilot.WithdrawalFailure(1, "withdrawal_unavailable"), nil
	}
	return out, nil
}
func (g *Gym) showWithdrawalReview(r pilot.WithdrawalReview, answer func(bool)) error {
	if g.app == nil || g.window == nil {
		return bridge.ErrUnavailable
	}
	message := "Candidate network payment\n\nDestination:\n" + r.Draft.Destination + "\n\nRecipient receives: " + withdrawalAmount(r.Draft.AmountAtoms) + "\nNetwork fee: " + withdrawalAmount(r.Draft.FeeAtoms) + "\nTotal wallet debit: " + withdrawalAmount(r.Draft.TotalDebitAtoms)
	d := g.app.Dialog.Question().SetTitle("Review SLUG payment").SetMessage(message).AttachToWindow(g.window)
	no := d.AddButton("Cancel").OnClick(func() { answer(false) })
	d.SetDefaultButton(no).SetCancelButton(no)
	label := "Send exact payment"
	if r.Kind == "retry" {
		label = "Retry same payment"
	}
	d.AddButton(label).OnClick(func() { answer(true) })
	d.Show()
	return nil
}
func (g *Gym) answerWithdrawal(a *nativeWithdrawalAction, yes bool) {
	r, yes, valid := g.takeWithdrawalAnswer(a, yes)
	if !valid {
		return
	}
	request := pilot.WithdrawalRequest{Command: "withdrawal_stop", ViewID: r.Binding.View.ViewID, OperationID: r.OperationID, ActionID: r.ActionID}
	if yes {
		snap, e := g.PoolSnapshot()
		if e != nil || snap.View == nil || *snap.View != r.Binding.View {
			return
		}
		g.withdrawal.mu.Lock()
		same := g.withdrawal.action == a && a.helper == g.poolHelper && a.state != "revoked"
		g.withdrawal.mu.Unlock()
		if !same {
			return
		}
		request.Command, request.ReviewID, request.DraftDigest, request.TxID = "withdrawal_approve", r.ReviewID, r.Draft.DraftDigest, r.TxID
	}
	out := a.helper.WithdrawalRequest(context.Background(), request)
	g.withdrawal.mu.Lock()
	defer g.withdrawal.mu.Unlock()
	if g.withdrawal.action != a {
		return
	}
	if out.OK && out.Action != nil {
		a.state = "admitted"
		a.terminal = out.Action.State != "running" && out.Action.State != "review"
	} else if out.Error == "outcome_unknown" {
		a.state = "unknown"
	} else {
		a.state = "not_started"
		a.terminal = true
	}
}
func (g *Gym) StopWithdrawal(view, operation, action string) (pilot.WithdrawalResponse, error) {
	g.withdrawal.mu.Lock()
	a := g.withdrawal.action
	valid := a != nil && a.helper == g.poolHelper && a.request.ViewID == view && a.request.OperationID == operation && a.request.ActionID == action
	if valid {
		a.answered = true
	}
	g.withdrawal.mu.Unlock()
	if !valid {
		return pilot.WithdrawalFailure(1, "stale_view"), nil
	}
	out := a.helper.WithdrawalRequest(context.Background(), pilot.WithdrawalRequest{Command: "withdrawal_stop", ViewID: view, OperationID: operation, ActionID: action})
	g.withdrawal.mu.Lock()
	if g.withdrawal.action == a && out.OK && out.Action != nil {
		a.terminal = out.Action.State != "running" && out.Action.State != "review"
	}
	g.withdrawal.mu.Unlock()
	return out, nil
}
func (g *Gym) AbandonWithdrawal(view, operation string) (pilot.WithdrawalResponse, error) {
	current, e := g.WithdrawalOperation(view, operation)
	if e != nil || !current.OK || current.Operation == nil {
		return current, e
	}
	r := pilot.WithdrawalRequest{Command: "withdrawal_abandon", ViewID: view, OperationID: operation}
	if current.Operation.Draft != nil {
		r.DraftDigest = current.Operation.Draft.DraftDigest
	}
	out, _ := g.nativeWithdrawalBegin(r)
	return out, nil
}
func (g *Gym) ObserveWithdrawal(view, operation string) (pilot.WithdrawalResponse, error) {
	current, e := g.WithdrawalOperation(view, operation)
	if e != nil || !current.OK || current.Operation == nil {
		return current, e
	}
	out, _ := g.nativeWithdrawalBegin(pilot.WithdrawalRequest{Command: "withdrawal_observe", ViewID: view, OperationID: operation, TxID: current.Operation.TxID})
	return out, nil
}

func (g *Gym) withdrawalActionSelection(view, operation, action string) (*nativeWithdrawalAction, string, bool) {
	g.withdrawal.mu.Lock()
	defer g.withdrawal.mu.Unlock()
	a := g.withdrawal.action
	valid := a != nil && a.helper == g.poolHelper && a.request.ViewID == view && (operation == "" || a.request.OperationID == operation) && a.request.ActionID == action && a.state != "revoked"
	if !valid {
		return nil, "", false
	}
	return a, a.request.OperationID, true
}
func (g *Gym) takeWithdrawalAnswer(a *nativeWithdrawalAction, yes bool) (pilot.WithdrawalReview, bool, bool) {
	g.withdrawal.mu.Lock()
	defer g.withdrawal.mu.Unlock()
	if a == nil || g.withdrawal.action != a || a.helper != g.poolHelper || a.answered || a.state == "revoked" || a.review == nil {
		return pilot.WithdrawalReview{}, false, false
	}
	a.answered = true
	r := *a.review
	if time.Now().UnixMilli() < r.ReviewedAtMS || time.Now().UnixMilli() >= r.ExpiresAtMS {
		yes = false
	}
	return r, yes, true
}
