// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"sync"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
)

type nativeHostedReceiveState struct {
	mu     sync.Mutex
	action *nativeHostedReceiveAction
}
type nativeHostedReceiveAction struct {
	helper                   *bridge.PoolBridge
	owner                    bridge.HostedRecoveryContext
	action, operation, state string
	refusal                  string
	job                      *pilot.HostedReceiveJob
}

func (g *Gym) HostedRecoverySnapshot() (pilot.HostedRecoverySnapshot, error) {
	b := g.poolHelper
	if b == nil || b.Mode() != pilot.HostedRecoveryPoolMode {
		return pilot.HostedRecoverySnapshot{}, bridge.ErrUnavailable
	}
	r, e := b.HostedRecoveryRequest(context.Background(), pilot.HostedRecoveryRequest{Command: "hosted_recovery_snapshot"})
	if e != nil || r.Snapshot == nil || g.poolHelper != b {
		return pilot.HostedRecoverySnapshot{}, bridge.ErrUnavailable
	}
	s := *r.Snapshot
	childState, view := hostedChildObservation(s)
	if e = g.observeEvidenceAccount(b.StableBridge, childState, view); e != nil {
		return pilot.HostedRecoverySnapshot{}, e
	}
	g.observePayoutAccount(b, childState, view)
	g.observeWalletActivityAccount(b, childState, view)
	g.observeWithdrawalAccount(b, childState, view)
	owner, oe := bridge.HostedContextForSnapshot(s)
	g.hostedReceive.mu.Lock()
	if a := g.hostedReceive.action; a != nil && (a.helper != b || oe != nil || a.owner.ViewID != "" && a.owner != owner) {
		g.hostedReceive.action = nil
	}
	g.hostedReceive.mu.Unlock()
	return s, nil
}

// A validated signed-in recovery-only owner follows joined replacement of any
// healthy child. Inactive transitional/error states do not prove that join.
func hostedChildObservation(s pilot.HostedRecoverySnapshot) (string, *pilot.ManifestViewPin) {
	if s.Healthy != nil {
		return s.Account.State, s.Healthy.Pool.View
	}
	if s.Kind == "recovery_only" && s.Recovery != nil && s.Account.State == "signed_in" {
		return "signed_out", nil
	}
	return s.Account.State, nil
}

// The full selected owner is captured before the dialog. Native and helper keep
// their own matching descriptors; file contents never enter the webview.
func hostedRecoveryAction(ctx context.Context, b *bridge.PoolBridge, current func() bool, choose func() (string, error), protected []string, export bool) (result string, resultErr error) {
	if b == nil || current == nil || !current() {
		return "", bridge.ErrUnavailable
	}
	r, e := b.HostedRecoveryRequest(ctx, pilot.HostedRecoveryRequest{Command: "hosted_recovery_snapshot"})
	if e != nil || r.Snapshot == nil {
		return "", bridge.ErrUnavailable
	}
	s := *r.Snapshot
	owner, e := bridge.HostedContextForSnapshot(s)
	if e != nil {
		return "", e
	}
	if s.Healthy != nil && ((s.Healthy.Pool.State != "stopped" && s.Healthy.Pool.State != "failed") || s.Healthy.Pool.BackupState == "running") || s.Recovery != nil && s.Recovery.Backup.State == "running" {
		return "", bridge.ErrBusy
	}
	if export && !s.Account.CanExport {
		if s.Account.Held {
			return "", errors.New("Recovery export is unavailable during the 24-hour security hold.")
		}
		return "", errors.New("Sign out and sign in again before exporting recovery.")
	}
	path, e := choose()
	if e != nil {
		return "", errors.New("The file dialog is unavailable.")
	}
	if path == "" {
		return "", nil
	}
	if !current() || !b.HostedContextMatches(owner) {
		return "", bridge.ErrUnavailable
	}
	mode, command := recoveryfile.ReadMode, "hosted_recovery_verify_backup"
	if export {
		mode, command = recoveryfile.CreateMode, "hosted_recovery_export_recovery"
	}
	selection, e := recoveryfile.Capture(path, protected, mode)
	if e != nil {
		return "", errors.New("Choose a recovery file outside application data.")
	}
	defer func() {
		if e := selection.Close(); e != nil {
			result = ""
			if export {
				resultErr = bridge.ErrRecoveryOutcomeUnknown
			} else {
				resultErr = errors.New("The backup selection could not be closed safely.")
			}
		}
	}()
	if !current() || !b.HostedContextMatches(owner) {
		return "", bridge.ErrUnavailable
	}
	pin := selection.Pin()
	_, e = b.HostedRecoveryRequest(ctx, pilot.HostedRecoveryRequest{Command: command, OwnerKind: owner.Kind, ViewID: owner.ViewID, Selection: &pin})
	if e != nil || !current() {
		if export {
			return "", bridge.ErrRecoveryOutcomeUnknown
		}
		return "", errors.New("Backup verification could not start. Choose this signed-in wallet's recovery file again.")
	}
	if export {
		return "Test wallet recovery file saved.", nil
	}
	return "Backup verification started.", nil
}

func nativeHostedPending(a *nativeHostedReceiveAction) bool {
	return a != nil && (a.state == "preparing" || a.state == "unknown" || a.state == "running")
}
func (g *Gym) NewReceiveAddress() (pilot.HostedReceiveJob, error) {
	return g.beginHostedReceive("receive_begin")
}
func (g *Gym) ResumeReceiveAddress() (pilot.HostedReceiveJob, error) {
	return g.beginHostedReceive("receive_resume")
}
func (g *Gym) RedisplayReceiveAddress() (pilot.HostedReceiveJob, error) {
	return g.beginHostedReceive("receive_redisplay")
}
func (g *Gym) beginHostedReceive(command string) (pilot.HostedReceiveJob, error) {
	b := g.poolHelper
	if b == nil || b.Mode() != pilot.HostedRecoveryPoolMode {
		return pilot.HostedReceiveJob{}, bridge.ErrUnavailable
	}
	g.hostedReceive.mu.Lock()
	if nativeHostedPending(g.hostedReceive.action) {
		g.hostedReceive.mu.Unlock()
		return pilot.HostedReceiveJob{}, bridge.ErrBusy
	}
	a := &nativeHostedReceiveAction{helper: b, state: "preparing"}
	g.hostedReceive.action = a
	g.hostedReceive.mu.Unlock()
	defer func() {
		g.hostedReceive.mu.Lock()
		if g.hostedReceive.action == a && a.state == "preparing" {
			a.state = "failed"
		}
		g.hostedReceive.mu.Unlock()
	}()
	r, e := b.HostedRecoveryRequest(context.Background(), pilot.HostedRecoveryRequest{Command: "hosted_recovery_snapshot"})
	if e != nil || r.Snapshot == nil || r.Snapshot.Healthy == nil || g.poolHelper != b {
		return pilot.HostedReceiveJob{}, bridge.ErrUnavailable
	}
	s := *r.Snapshot
	owner, e := bridge.HostedContextForSnapshot(s)
	if e != nil {
		return pilot.HostedReceiveJob{}, e
	}
	if s.Healthy.Pool.State != "stopped" || s.Healthy.Pool.BackupState == "running" {
		return pilot.HostedReceiveJob{}, bridge.ErrBusy
	}
	action, e := withdrawalRandomID()
	if e != nil {
		return pilot.HostedReceiveJob{}, bridge.ErrUnavailable
	}
	operation := ""
	switch command {
	case "receive_begin":
		if s.Healthy.PendingReceiveID != "" {
			return pilot.HostedReceiveJob{}, bridge.ErrBusy
		}
		operation, e = withdrawalRandomID()
	case "receive_resume":
		operation = s.Healthy.PendingReceiveID
	case "receive_redisplay":
		operation = s.Healthy.IssuedReceiveID
	default:
		return pilot.HostedReceiveJob{}, bridge.ErrUnavailable
	}
	if e != nil || operation == "" {
		return pilot.HostedReceiveJob{}, bridge.ErrUnavailable
	}
	g.hostedReceive.mu.Lock()
	if g.hostedReceive.action != a || g.poolHelper != b || !b.HostedContextMatches(owner) {
		g.hostedReceive.mu.Unlock()
		return pilot.HostedReceiveJob{}, bridge.ErrUnavailable
	}
	a.owner = owner
	a.action = action
	a.operation = operation
	a.state = "unknown"
	g.hostedReceive.mu.Unlock()
	return g.dispatchHostedReceive(a, command)
}
func (g *Gym) dispatchHostedReceive(a *nativeHostedReceiveAction, command string) (pilot.HostedReceiveJob, error) {
	if a == nil || g.poolHelper != a.helper || !a.helper.HostedContextMatches(a.owner) {
		return pilot.HostedReceiveJob{}, bridge.ErrUnavailable
	}
	r, e := a.helper.HostedRecoveryRequest(context.Background(), pilot.HostedRecoveryRequest{Command: "hosted_recovery_" + command, ViewID: a.owner.ViewID, ActionID: a.action, OperationID: a.operation})
	g.hostedReceive.mu.Lock()
	defer g.hostedReceive.mu.Unlock()
	if g.hostedReceive.action != a || g.poolHelper != a.helper || !a.helper.HostedContextMatches(a.owner) {
		return pilot.HostedReceiveJob{}, bridge.ErrUnavailable
	}
	if e != nil {
		var refusal *bridge.HostedRecoveryFailure
		if command == "receive_lookup" && errors.As(e, &refusal) {
			switch refusal.Code {
			case "allocation_not_found":
				a.state = "failed"
				a.refusal = refusal.Code
			case "allocation_resume_required":
				a.state = "needs_resume"
				a.refusal = refusal.Code
			}
		}
		if command != "receive_lookup" && command != "receive_cancel" && !bridge.IsHostedRecoveryUnknown(e) {
			a.state = "failed"
			if errors.As(e, &refusal) {
				a.refusal = refusal.Code
			} else if errors.Is(e, bridge.ErrBusy) {
				a.refusal = "busy"
			}
		}
		return pilot.HostedReceiveJob{}, e
	}
	if r.Receive == nil || r.Receive.ActionID != a.action || r.Receive.OperationID != a.operation || r.Receive.ViewID != a.owner.ViewID || r.Receive.Allocation != a.owner.Allocation {
		return pilot.HostedReceiveJob{}, bridge.ErrUnavailable
	}
	j := *r.Receive
	if j.Address != nil {
		address := *j.Address
		j.Address = &address
	}
	saved := j
	if j.Address != nil {
		address := *j.Address
		saved.Address = &address
	}
	a.job = &saved
	a.state = j.State
	a.refusal = ""
	return j, nil
}
func (g *Gym) ReceiveAddressStatus() (pilot.HostedReceiveJob, error) {
	g.hostedReceive.mu.Lock()
	a := g.hostedReceive.action
	if a == nil || a.state == "failed" && a.action == "" {
		g.hostedReceive.mu.Unlock()
		return pilot.HostedReceiveJob{}, &bridge.HostedRecoveryFailure{Code: "allocation_not_found"}
	}
	if a.refusal != "" && (a.state == "failed" || a.state == "needs_resume") {
		if g.poolHelper != a.helper || a.helper == nil || !a.helper.HostedContextMatches(a.owner) {
			g.hostedReceive.mu.Unlock()
			return pilot.HostedReceiveJob{}, bridge.ErrUnavailable
		}
		// This is the retained definitive outcome of this native invocation,
		// not a new helper lookup. No address or successful admission is implied.
		job := pilot.HostedReceiveJob{Version: 1, ActionID: a.action, OperationID: a.operation, ViewID: a.owner.ViewID, Allocation: a.owner.Allocation, State: a.state, Error: a.refusal}
		g.hostedReceive.mu.Unlock()
		return job, nil
	}
	g.hostedReceive.mu.Unlock()
	return g.dispatchHostedReceive(a, "receive_lookup")
}
func (g *Gym) CancelReceiveAddress() (pilot.HostedReceiveJob, error) {
	g.hostedReceive.mu.Lock()
	a := g.hostedReceive.action
	g.hostedReceive.mu.Unlock()
	return g.dispatchHostedReceive(a, "receive_cancel")
}
