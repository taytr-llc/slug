// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
)

// This public context has no caller-supplied constructor in the IPC route.
// Its private retained bridge copy seals the complete owner/view and scope.
type HostedRecoveryContext struct {
	Kind         pilot.HostedOwnerKind
	ViewID       string
	Allocation   pilot.HostedAllocationScope
	HealthyView  pilot.ManifestViewPin
	RecoveryView pilot.HostedRecoveryView
}
type hostedOwnerExpectation struct {
	context         HostedRecoveryContext
	pending, issued string
}
type hostedReceiveExpectation struct {
	owner                      HostedRecoveryContext
	action, operation, command string
	state                      string
}
type HostedRecoveryFailure struct{ Code string }

func (e *HostedRecoveryFailure) Error() string {
	return "The hosted recovery request could not complete (" + e.Code + ")."
}

func HostedContextForSnapshot(s pilot.HostedRecoverySnapshot) (HostedRecoveryContext, error) {
	if pilot.ValidateHostedRecoverySnapshot(s) != nil {
		return HostedRecoveryContext{}, ErrUnavailable
	}
	switch s.Kind {
	case "healthy":
		return HostedRecoveryContext{Kind: pilot.HostedHealthyOwner, ViewID: s.Healthy.Pool.View.ViewID, Allocation: s.Healthy.Allocation, HealthyView: *s.Healthy.Pool.View}, nil
	case "recovery_only":
		return HostedRecoveryContext{Kind: pilot.HostedMissingOwner, ViewID: s.Recovery.View.ViewID, Allocation: s.Recovery.Allocation, RecoveryView: s.Recovery.View}, nil
	default:
		return HostedRecoveryContext{}, ErrUnavailable
	}
}
func (b *PoolBridge) HostedContextMatches(want HostedRecoveryContext) bool {
	if b == nil || b.StableBridge == nil || b.Mode() != pilot.HostedRecoveryPoolMode {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hostedOwner != nil && b.hostedOwner.context == want
}

func (b *PoolBridge) rememberHostedRoot(s pilot.HostedRecoverySnapshot) error {
	if pilot.ValidateHostedRecoverySnapshot(s) != nil || s.Mode != b.Mode() {
		return ErrUnavailable
	}
	if s.Kind == "inactive" {
		b.clearStableView()
		b.manifestView = nil
		b.mu.Lock()
		b.hostedOwner, b.hostedReceive = nil, nil
		b.mu.Unlock()
		return nil
	}
	ctx, e := HostedContextForSnapshot(s)
	if e != nil {
		return e
	}
	b.mu.Lock()
	same := b.hostedOwner != nil && b.hostedOwner.context == ctx
	b.mu.Unlock()
	if !same {
		b.clearStableView()
		b.manifestView = nil
	}
	if s.Healthy != nil {
		p := &s.Healthy.Pool
		if !b.matchesPoolSelection(p) {
			return ErrUnavailable
		}
		b.poolSourceID = p.SourceBindingID
		pin := p.View.AuthorityPin
		b.poolSourcePin = &pin
		v := *p.View
		b.manifestView = &v
		payout, e := pilot.PoolPayoutBindingForSnapshot(*p)
		if e != nil {
			return ErrUnavailable
		}
		activity, e := pilot.WalletActivityBindingForSnapshot(*p)
		if e != nil {
			return ErrUnavailable
		}
		b.mu.Lock()
		if b.activityBinding == nil || *b.activityBinding != activity {
			b.activityAttempt, b.activityCapture, b.activityRows = nil, nil, nil
		}
		b.activityBinding = &activity
		copyPool := *p
		copyView := *p.View
		copyPool.View = &copyView
		b.withdrawalPool = &copyPool
		b.payoutBinding = &payout
		b.mu.Unlock()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !same {
		b.hostedReceive = nil
	}
	b.hostedOwner = &hostedOwnerExpectation{context: ctx}
	if s.Healthy != nil {
		b.hostedOwner.pending = s.Healthy.PendingReceiveID
		b.hostedOwner.issued = s.Healthy.IssuedReceiveID
	}
	return nil
}

func (b *PoolBridge) HostedRecoveryRequest(ctx context.Context, req pilot.HostedRecoveryRequest) (pilot.HostedRecoveryResponse, error) {
	if b == nil || b.StableBridge == nil || b.Mode() != pilot.HostedRecoveryPoolMode {
		return pilot.HostedRecoveryResponse{}, ErrUnavailable
	}
	id, e := b.begin(ctx)
	if e != nil {
		return pilot.HostedRecoveryResponse{}, e
	}
	defer func() { <-b.gate }()
	req.ID = id
	req.Version = 1
	raw, e := json.Marshal(req)
	if e != nil {
		return pilot.HostedRecoveryResponse{}, ErrUnavailable
	}
	if _, e = pilot.ParseHostedRecoveryRequest(raw); e != nil {
		return pilot.HostedRecoveryResponse{}, ErrUnavailable
	}
	command := strings.TrimPrefix(req.Command, "hosted_recovery_")
	receive := strings.HasPrefix(command, "receive_")
	backup := oneOf(command, "export_recovery", "verify_backup", "cancel_pool_backup")
	var owner HostedRecoveryContext
	if receive || backup || oneOf(command, "start", "stop") {
		b.mu.Lock()
		if b.hostedOwner != nil {
			owner = b.hostedOwner.context
		}
		b.mu.Unlock()
		if owner.ViewID == "" || owner.ViewID != req.ViewID || backup && owner.Kind != req.OwnerKind || !backup && owner.Kind != pilot.HostedHealthyOwner {
			return pilot.HostedRecoveryResponse{}, ErrUnavailable
		}
	}
	if receive {
		b.mu.Lock()
		prior := b.hostedReceive
		if oneOf(command, "receive_lookup", "receive_cancel") {
			if prior == nil || prior.owner != owner || prior.action != req.ActionID || prior.operation != req.OperationID {
				b.mu.Unlock()
				return pilot.HostedRecoveryResponse{}, ErrUnavailable
			}
		} else {
			if prior != nil && oneOf(prior.state, "running", "unknown") {
				b.mu.Unlock()
				return pilot.HostedRecoveryResponse{}, ErrBusy
			}
			b.hostedReceive = &hostedReceiveExpectation{owner: owner, action: req.ActionID, operation: req.OperationID, command: command, state: "unknown"}
		}
		b.mu.Unlock()
	}
	if oneOf(command, "login_fixture", "logout", "reset_fixture") {
		b.clearStableView()
		b.manifestView = nil
		b.mu.Lock()
		b.hostedOwner, b.hostedReceive = nil, nil
		b.mu.Unlock()
	}
	budget := 30 * time.Second
	if oneOf(command, "login_fixture", "reset_fixture") {
		budget = 120 * time.Second
	}
	b.mu.Lock()
	initial := command == "snapshot" && !b.poolHandshakeUsed
	if initial {
		b.poolHandshakeUsed = true
		b.poolHandshakePending = true
		budget = 180 * time.Second
	}
	b.mu.Unlock()
	if initial {
		defer func() { b.mu.Lock(); b.poolHandshakePending = false; b.mu.Unlock() }()
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	f, e := b.exchangeExpected(ctx, &stableExpected{id: id, hostedRecovery: true}, raw)
	if e != nil || f.hostedRecovery == nil {
		if initial {
			b.interruptStartup()
		}
		return pilot.HostedRecoveryResponse{}, ErrUnavailable
	}
	b.mu.Lock()
	b.poolStartupComplete = true
	b.mu.Unlock()
	r := *f.hostedRecovery
	if !r.OK {
		if receive && command == "receive_lookup" && oneOf(r.Error, "allocation_not_found", "allocation_resume_required") {
			b.mu.Lock()
			if p := b.hostedReceive; p != nil && p.owner == owner && p.action == req.ActionID && p.operation == req.OperationID {
				p.state = "failed"
				if r.Error == "allocation_resume_required" {
					p.state = "needs_resume"
				}
			}
			b.mu.Unlock()
		}
		if receive && !oneOf(command, "receive_lookup", "receive_cancel") && r.Error != "outcome_unknown" {
			b.mu.Lock()
			if b.hostedReceive != nil && b.hostedReceive.action == req.ActionID && b.hostedReceive.operation == req.OperationID {
				b.hostedReceive.state = "failed"
			}
			b.mu.Unlock()
		}
		if command == "export_recovery" && r.Error == "backup_export_uncertain" {
			return pilot.HostedRecoveryResponse{}, ErrRecoveryOutcomeUnknown
		}
		return pilot.HostedRecoveryResponse{}, &HostedRecoveryFailure{Code: r.Error}
	}
	if receive {
		j := r.Receive
		if j == nil || r.Snapshot != nil || j.ActionID != req.ActionID || j.OperationID != req.OperationID || j.ViewID != owner.ViewID || j.Allocation != owner.Allocation || !b.HostedContextMatches(owner) {
			b.fail(ErrUnavailable)
			return pilot.HostedRecoveryResponse{}, ErrUnavailable
		}
		b.mu.Lock()
		if b.hostedReceive == nil || b.hostedReceive.action != req.ActionID || b.hostedReceive.operation != req.OperationID {
			b.mu.Unlock()
			b.fail(ErrUnavailable)
			return pilot.HostedRecoveryResponse{}, ErrUnavailable
		}
		b.hostedReceive.state = j.State
		b.mu.Unlock()
	} else {
		if r.Snapshot == nil || r.Receive != nil {
			b.fail(ErrUnavailable)
			return pilot.HostedRecoveryResponse{}, ErrUnavailable
		}
		if backup || oneOf(command, "start", "stop") {
			observed, e := HostedContextForSnapshot(*r.Snapshot)
			if e != nil || observed != owner {
				b.fail(ErrUnavailable)
				return pilot.HostedRecoveryResponse{}, ErrUnavailable
			}
		}
		if e = b.rememberHostedRoot(*r.Snapshot); e != nil {
			b.fail(ErrUnavailable)
			return pilot.HostedRecoveryResponse{}, e
		}
	}
	return r, nil
}

func IsHostedRecoveryUnknown(err error) bool {
	var e *HostedRecoveryFailure
	return errors.Is(err, ErrUnavailable) || errors.As(err, &e) && e.Code == "outcome_unknown"
}
