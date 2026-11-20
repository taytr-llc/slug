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

type nativeWalletActivityState struct {
	mu     sync.Mutex
	action *nativeWalletActivityAction
}
type nativeWalletActivityAction struct {
	helper               *bridge.PoolBridge
	viewID, jobID, state string
	terminal             bool
}

func walletActivityNativeFailure(code string) pilot.WalletActivityResponse {
	return pilot.WalletActivityResponse{ID: 1, Version: 1, Error: code}
}
func (g *Gym) walletActivityRequest(r pilot.WalletActivityRequest) pilot.WalletActivityResponse {
	return walletActivityHelperRequest(g.poolHelper, r)
}
func walletActivityHelperRequest(helper *bridge.PoolBridge, r pilot.WalletActivityRequest) pilot.WalletActivityResponse {
	if helper == nil {
		return walletActivityNativeFailure("activity_unavailable")
	}
	return helper.WalletActivityRequest(context.Background(), r)
}
func (g *Gym) observeWalletActivityAccount(helper *bridge.PoolBridge, state string, view *pilot.ManifestViewPin) {
	g.walletActivity.mu.Lock()
	defer g.walletActivity.mu.Unlock()
	if a := g.walletActivity.action; a != nil && a.helper == helper && (state == "signed_out" && view == nil || state == "signed_in" && view != nil && view.ViewID != a.viewID) {
		g.walletActivity.action = nil
	}
}
func (g *Gym) BeginWalletActivity(viewID string) (pilot.WalletActivityResponse, error) {
	if g.poolHelper == nil {
		return walletActivityNativeFailure("activity_unavailable"), nil
	}
	a := &nativeWalletActivityAction{helper: g.poolHelper, viewID: viewID, state: "preparing"}
	g.walletActivity.mu.Lock()
	old := g.walletActivity.action
	if old != nil && old.helper == a.helper && old.viewID == viewID && nativeWalletActivityPending(old) {
		g.walletActivity.mu.Unlock()
		return walletActivityNativeFailure("busy"), nil
	}
	g.walletActivity.action = a
	g.walletActivity.mu.Unlock()
	defer func() {
		g.walletActivity.mu.Lock()
		if g.walletActivity.action == a && a.state == "preparing" {
			a.state = "not_started"
		}
		g.walletActivity.mu.Unlock()
	}()
	s, e := g.PoolSnapshot()
	if e != nil || s.View == nil || s.View.ViewID != viewID {
		return walletActivityNativeFailure("stale_view"), nil
	}
	if s.BackupState == "running" {
		return walletActivityNativeFailure("busy"), nil
	}
	var raw [32]byte
	if _, e := rand.Read(raw[:]); e != nil {
		return walletActivityNativeFailure("activity_unavailable"), nil
	}
	id := hex.EncodeToString(raw[:])
	g.walletActivity.mu.Lock()
	if g.walletActivity.action != a || g.poolHelper != a.helper {
		g.walletActivity.mu.Unlock()
		return walletActivityNativeFailure("stale_view"), nil
	}
	// Registration is local and bounded. Keep it atomic with this exact native
	// action so a view switch cannot strand an intent that was never dispatched.
	if e := a.helper.RememberWalletActivity(viewID, id); e != nil {
		g.walletActivity.mu.Unlock()
		return walletActivityNativeFailure("busy"), nil
	}
	a.jobID, a.state = id, "unknown"
	g.walletActivity.mu.Unlock()
	r := a.helper.WalletActivityRequest(context.Background(), pilot.WalletActivityRequest{Command: "wallet_activity_begin", ViewID: viewID, JobID: id})
	g.walletActivity.mu.Lock()
	if g.walletActivity.action == a {
		if r.OK && r.Job != nil {
			a.state, a.terminal = "admitted", r.Job.State != "running"
		} else if r.Error != "outcome_unknown" {
			a.state = "not_started"
		}
	}
	g.walletActivity.mu.Unlock()
	return r, nil
}
func (g *Gym) WalletActivitySnapshot() (pilot.WalletActivityResponse, error) {
	helper := g.poolHelper
	g.walletActivity.mu.Lock()
	current := g.walletActivity.action
	var a nativeWalletActivityAction
	if current != nil {
		a = *current
	}
	g.walletActivity.mu.Unlock()
	if a.helper != nil && a.helper != helper {
		return walletActivityNativeFailure("stale_view"), nil
	}
	r, resolved, terminal := nativeWalletActivityActionSnapshot(a, func(r pilot.WalletActivityRequest) pilot.WalletActivityResponse {
		return walletActivityHelperRequest(helper, r)
	})
	g.walletActivity.mu.Lock()
	same := g.walletActivity.action == current && g.poolHelper == helper
	if same && current != nil && resolved != "" {
		current.state, current.terminal = resolved, terminal
	}
	g.walletActivity.mu.Unlock()
	if !same {
		return walletActivityNativeFailure("stale_view"), nil
	}
	return r, nil
}

// Preserve a separately known old capture for dated display, while resolving
// admission of this exact new native action. Never adopt generic latest success.
func nativeWalletActivityActionSnapshot(a nativeWalletActivityAction, request func(pilot.WalletActivityRequest) pilot.WalletActivityResponse) (pilot.WalletActivityResponse, string, bool) {
	if a.state == "preparing" {
		return walletActivityNativeFailure("outcome_unknown"), "", false
	}
	resolved := ""
	terminal := a.terminal
	if a.jobID != "" && (a.state == "unknown" || a.state == "admitted") {
		r := request(pilot.WalletActivityRequest{Command: "wallet_activity_job", ViewID: a.viewID, JobID: a.jobID})
		if r.OK && r.Job != nil {
			if r.Job.JobID != a.jobID || r.Job.Binding.View.ViewID != a.viewID {
				return walletActivityNativeFailure("outcome_unknown"), "", false
			}
			resolved, terminal = "admitted", r.Job.State != "running"
		} else if r.Error == "activity_not_found" {
			resolved, terminal = "not_started", false
		} else {
			if a.state == "unknown" {
				return walletActivityNativeFailure("outcome_unknown"), "", false
			}
			return r, "", false
		}
	}
	r := request(pilot.WalletActivityRequest{Command: "wallet_activity_snapshot"})
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
func nativeWalletActivityPending(a *nativeWalletActivityAction) bool {
	return a != nil && (a.state == "preparing" || a.state == "unknown" || a.state == "admitted" && !a.terminal)
}

// Exact terminal reads/cancellation retire the native reservation as well as
// the UI task. Otherwise the UI can stop polling while native stays busy.
func (g *Gym) observeWalletActivityJob(helper *bridge.PoolBridge, view, id string, r pilot.WalletActivityResponse) {
	g.walletActivity.mu.Lock()
	defer g.walletActivity.mu.Unlock()
	a := g.walletActivity.action
	if a == nil || a.helper != helper || g.poolHelper != helper || a.viewID != view || a.jobID != id {
		return
	}
	if r.OK && r.Job != nil && r.Job.JobID == id && r.Job.Binding.View.ViewID == view {
		a.state, a.terminal = "admitted", r.Job.State != "running"
	} else if !r.OK && r.Error == "activity_not_found" {
		a.state, a.terminal = "not_started", false
	}
}
func (g *Gym) WalletActivityJob(v, id string) (pilot.WalletActivityResponse, error) {
	helper := g.poolHelper
	r := walletActivityHelperRequest(helper, pilot.WalletActivityRequest{Command: "wallet_activity_job", ViewID: v, JobID: id})
	g.observeWalletActivityJob(helper, v, id, r)
	return r, nil
}
func (g *Gym) WalletActivityPage(v, id string, after, limit uint32) (pilot.WalletActivityResponse, error) {
	return g.walletActivityRequest(pilot.WalletActivityRequest{Command: "wallet_activity_page", ViewID: v, JobID: id, After: after, Limit: limit}), nil
}
func (g *Gym) CancelWalletActivity(v, id string) (pilot.WalletActivityResponse, error) {
	helper := g.poolHelper
	r := walletActivityHelperRequest(helper, pilot.WalletActivityRequest{Command: "wallet_activity_cancel", ViewID: v, JobID: id})
	g.observeWalletActivityJob(helper, v, id, r)
	return r, nil
}
func (g *Gym) WalletActivityDetail(v, id, tx string) (pilot.WalletActivityResponse, error) {
	return g.walletActivityRequest(pilot.WalletActivityRequest{Command: "wallet_activity_detail", ViewID: v, JobID: id, TxID: tx}), nil
}
