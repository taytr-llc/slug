// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
	"sync"
)

type nativePayoutState struct {
	mu     sync.Mutex
	action *nativePayoutAction
}
type nativePayoutAction struct {
	helper               *bridge.PoolBridge
	viewID, jobID, state string
	terminal             bool
}

func payoutNativeFailure(code string) pilot.PoolPayoutResponse {
	return pilot.PoolPayoutResponse{ID: 1, Version: 1, Error: code}
}
func (g *Gym) payoutRequest(r pilot.PoolPayoutRequest) pilot.PoolPayoutResponse {
	if g.poolHelper == nil {
		return payoutNativeFailure("payout_unavailable")
	}
	return g.poolHelper.PoolPayoutRequest(context.Background(), r)
}
func (g *Gym) observePayoutAccount(helper *bridge.PoolBridge, state string, view *pilot.ManifestViewPin) {
	g.payout.mu.Lock()
	defer g.payout.mu.Unlock()
	if a := g.payout.action; a != nil && a.helper == helper && (state == "signed_out" && view == nil || state == "signed_in" && view != nil && view.ViewID != a.viewID) {
		g.payout.action = nil
	}
}
func (g *Gym) BeginPoolPayoutHistory(viewID string) (pilot.PoolPayoutResponse, error) {
	if g.poolHelper == nil {
		return payoutNativeFailure("payout_unavailable"), nil
	}
	a := &nativePayoutAction{helper: g.poolHelper, viewID: viewID, state: "preparing"}
	g.payout.mu.Lock()
	old := g.payout.action
	if old != nil && old.helper == a.helper && old.viewID == viewID && (old.state == "preparing" || old.state == "unknown" || old.state == "admitted" && !old.terminal) {
		g.payout.mu.Unlock()
		return payoutNativeFailure("busy"), nil
	}
	g.payout.action = a
	g.payout.mu.Unlock()
	defer func() {
		g.payout.mu.Lock()
		if g.payout.action == a && a.state == "preparing" {
			a.state = "not_started"
		}
		g.payout.mu.Unlock()
	}()
	s, e := g.PoolSnapshot()
	if e != nil || s.View == nil || s.View.ViewID != viewID {
		return payoutNativeFailure("stale_view"), nil
	}
	if s.BackupState == "running" {
		return payoutNativeFailure("busy"), nil
	}
	var raw [32]byte
	if _, e := rand.Read(raw[:]); e != nil {
		return payoutNativeFailure("payout_unavailable"), nil
	}
	id := hex.EncodeToString(raw[:])
	g.payout.mu.Lock()
	if g.payout.action != a {
		g.payout.mu.Unlock()
		return payoutNativeFailure("stale_view"), nil
	}
	// Registration is local and bounded. Keep it atomic with this exact native
	// action so a view switch cannot strand an intent that was never dispatched.
	if e := a.helper.RememberPoolPayout(viewID, id); e != nil {
		g.payout.mu.Unlock()
		return payoutNativeFailure("busy"), nil
	}
	a.jobID, a.state = id, "unknown"
	g.payout.mu.Unlock()
	r := g.payoutRequest(pilot.PoolPayoutRequest{Command: "pool_payout_begin", ViewID: viewID, JobID: id})
	g.payout.mu.Lock()
	if g.payout.action == a {
		if r.OK && r.Job != nil {
			a.state, a.terminal = "admitted", r.Job.State != "running"
		} else if r.Error != "outcome_unknown" {
			a.state = "not_started"
		}
	}
	g.payout.mu.Unlock()
	return r, nil
}
func (g *Gym) PoolPayoutSnapshot() (pilot.PoolPayoutResponse, error) {
	g.payout.mu.Lock()
	current := g.payout.action
	var a nativePayoutAction
	if current != nil {
		a = *current
	}
	g.payout.mu.Unlock()
	r, resolved, terminal := nativePayoutActionSnapshot(a, g.payoutRequest)
	g.payout.mu.Lock()
	same := g.payout.action == current
	if same && current != nil && resolved != "" {
		current.state, current.terminal = resolved, terminal
	}
	g.payout.mu.Unlock()
	if !same {
		return payoutNativeFailure("stale_view"), nil
	}
	return r, nil
}

// Preserve a separately known old capture for dated display, while resolving
// admission of this exact new native action. Never adopt generic latest success.
func nativePayoutActionSnapshot(a nativePayoutAction, request func(pilot.PoolPayoutRequest) pilot.PoolPayoutResponse) (pilot.PoolPayoutResponse, string, bool) {
	if a.state == "preparing" {
		return payoutNativeFailure("outcome_unknown"), "", false
	}
	resolved := ""
	terminal := a.terminal
	if a.jobID != "" && (a.state == "unknown" || a.state == "admitted") {
		r := request(pilot.PoolPayoutRequest{Command: "pool_payout_job", ViewID: a.viewID, JobID: a.jobID})
		if r.OK && r.Job != nil {
			if r.Job.JobID != a.jobID || r.Job.Binding.View.ViewID != a.viewID {
				return payoutNativeFailure("outcome_unknown"), "", false
			}
			resolved, terminal = "admitted", r.Job.State != "running"
		} else if r.Error == "payout_not_found" {
			resolved, terminal = "not_started", false
		} else {
			if a.state == "unknown" {
				return payoutNativeFailure("outcome_unknown"), "", false
			}
			return r, "", false
		}
	}
	r := request(pilot.PoolPayoutRequest{Command: "pool_payout_snapshot"})
	if r.OK && r.Snapshot != nil {
		copy := *r.Snapshot
		admitted := resolved == "admitted" || resolved == "" && a.state == "admitted"
		if !admitted || copy.LastJobID != a.jobID {
			copy.LastJobID = ""
		}
		if copy.Activity == "capture" && (!admitted || copy.ActiveJobID != a.jobID) {
			copy.State, copy.Activity, copy.ActiveJobID = "idle", "none", ""
		}
		r.Snapshot = &copy
	}
	return r, resolved, terminal
}
func (g *Gym) PoolPayoutJob(v, id string) (pilot.PoolPayoutResponse, error) {
	r := g.payoutRequest(pilot.PoolPayoutRequest{Command: "pool_payout_job", ViewID: v, JobID: id})
	g.payout.mu.Lock()
	if a := g.payout.action; a != nil && a.viewID == v && a.jobID == id && a.helper == g.poolHelper && r.OK && r.Job != nil {
		a.state, a.terminal = "admitted", r.Job.State != "running"
	}
	g.payout.mu.Unlock()
	return r, nil
}
func (g *Gym) PoolPayoutPage(v, id string, after, limit uint32) (pilot.PoolPayoutResponse, error) {
	return g.payoutRequest(pilot.PoolPayoutRequest{Command: "pool_payout_page", ViewID: v, JobID: id, After: after, Limit: limit}), nil
}
func (g *Gym) CancelPoolPayoutHistory(v, id string) (pilot.PoolPayoutResponse, error) {
	return g.payoutRequest(pilot.PoolPayoutRequest{Command: "pool_payout_cancel", ViewID: v, JobID: id}), nil
}
func (g *Gym) ReadPoolPayoutTransaction(v, id, tx string, n uint32) (pilot.PoolPayoutResponse, error) {
	return g.payoutRequest(pilot.PoolPayoutRequest{Command: "pool_payout_wallet", ViewID: v, JobID: id, TxID: tx, OutputIndex: n}), nil
}
