// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"github.com/taytr-llc/slug/desktop/pilot"
	"strings"
	"testing"
)

func TestWalletActivityNativeNewActionNeverAdoptsOldCapture(t *testing.T) {
	a := nativeWalletActivityAction{viewID: strings.Repeat("a", 64), jobID: strings.Repeat("b", 64), state: "unknown"}
	old := strings.Repeat("c", 64)
	snapshot := pilot.WalletActivitySnapshot{Version: 1, State: "idle", Activity: "none", LastJobID: old, CaptureJobID: old}
	for _, phase := range []string{"unknown", "not_started", ""} {
		t.Run(phase, func(t *testing.T) {
			current := a
			current.state = phase
			lookups := 0
			r, state, _ := nativeWalletActivityActionSnapshot(current, func(q pilot.WalletActivityRequest) pilot.WalletActivityResponse {
				if q.Command == "wallet_activity_job" {
					lookups++
					if q.JobID != a.jobID || q.ViewID != a.viewID {
						t.Fatal("lookup substituted old action")
					}
					return walletActivityNativeFailure("activity_not_found")
				}
				if q.Command != "wallet_activity_snapshot" {
					t.Fatal(q.Command)
				}
				return pilot.WalletActivityResponse{ID: 1, Version: 1, OK: true, Snapshot: &snapshot}
			})
			if !r.OK || r.Snapshot == nil || r.Snapshot.LastJobID != "" || r.Snapshot.CaptureJobID != old || snapshot.LastJobID != old {
				t.Fatal("new action adopted prior success or erased dated capture")
			}
			if phase == "unknown" && (lookups != 1 || state != "not_started") {
				t.Fatal("uncertain exact lookup unresolved")
			}
		})
	}
	calls := 0
	r, resolved, terminal := nativeWalletActivityActionSnapshot(a, func(q pilot.WalletActivityRequest) pilot.WalletActivityResponse {
		calls++
		if q.Command == "wallet_activity_job" {
			return pilot.WalletActivityResponse{ID: 1, Version: 1, OK: true, Job: &pilot.WalletActivityJob{JobID: a.jobID, State: "done", Binding: pilot.WalletActivityBinding{View: pilot.ManifestViewPin{ViewID: a.viewID}}}}
		}
		s := snapshot
		s.LastJobID, s.CaptureJobID = a.jobID, a.jobID
		return pilot.WalletActivityResponse{ID: 1, Version: 1, OK: true, Snapshot: &s}
	})
	if calls != 2 || resolved != "admitted" || !terminal || r.Snapshot.LastJobID != a.jobID {
		t.Fatal("exact latest action not recovered")
	}
	a.state = "preparing"
	r, _, _ = nativeWalletActivityActionSnapshot(a, func(pilot.WalletActivityRequest) pilot.WalletActivityResponse {
		t.Fatal("preflight queried old success")
		return pilot.WalletActivityResponse{}
	})
	if r.Error != "outcome_unknown" || r.OK {
		t.Fatal("preflight state mislabeled")
	}
}

func TestWalletActivityNativeUncertainLookupCannotSubstituteViewOrJob(t *testing.T) {
	a := nativeWalletActivityAction{viewID: strings.Repeat("a", 64), jobID: strings.Repeat("b", 64), state: "unknown"}
	for _, wrongView := range []bool{false, true} {
		calls := 0
		r, resolved, terminal := nativeWalletActivityActionSnapshot(a, func(q pilot.WalletActivityRequest) pilot.WalletActivityResponse {
			calls++
			j := pilot.WalletActivityJob{JobID: a.jobID, State: "done", Binding: pilot.WalletActivityBinding{View: pilot.ManifestViewPin{ViewID: a.viewID}}}
			if wrongView {
				j.Binding.View.ViewID = strings.Repeat("c", 64)
			} else {
				j.JobID = strings.Repeat("c", 64)
			}
			return pilot.WalletActivityResponse{ID: 1, Version: 1, OK: true, Job: &j}
		})
		if calls != 1 || r.OK || r.Error != "outcome_unknown" || resolved != "" || terminal {
			t.Fatal("foreign job resolved an unknown action")
		}
	}
	for _, code := range []string{"busy", "activity_unavailable", "outcome_unknown"} {
		calls := 0
		r, resolved, terminal := nativeWalletActivityActionSnapshot(a, func(pilot.WalletActivityRequest) pilot.WalletActivityResponse {
			calls++
			return walletActivityNativeFailure(code)
		})
		if calls != 1 || r.Error != "outcome_unknown" || resolved != "" || terminal {
			t.Fatal("uncertain lookup retired original intent")
		}
	}
}

func TestWalletActivityNativeTerminalCancelAndMissingReadReleaseReservation(t *testing.T) {
	view, id := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, state := range []string{"cancelled", "done", "failed", "missing"} {
		g := &Gym{}
		a := &nativeWalletActivityAction{viewID: view, jobID: id, state: "admitted"}
		g.walletActivity.action = a
		r := pilot.WalletActivityResponse{ID: 1, Version: 1, OK: true, Job: &pilot.WalletActivityJob{JobID: id, State: state, Binding: pilot.WalletActivityBinding{View: pilot.ManifestViewPin{ViewID: view}}}}
		if state == "missing" {
			r = walletActivityNativeFailure("activity_not_found")
		}
		g.observeWalletActivityJob(nil, view, strings.Repeat("c", 64), r)
		if !nativeWalletActivityPending(a) {
			t.Fatal("foreign lookup retired original action")
		}
		g.observeWalletActivityJob(nil, view, id, r)
		if nativeWalletActivityPending(a) {
			t.Fatal("terminal cancellation/read blocks later explicit refresh", state)
		}
	}
	g := &Gym{}
	a := &nativeWalletActivityAction{viewID: view, jobID: id, state: "unknown"}
	g.walletActivity.action = a
	g.observeWalletActivityJob(nil, view, id, walletActivityNativeFailure("busy"))
	if !nativeWalletActivityPending(a) {
		t.Fatal("busy resolved dispatched uncertainty")
	}
	g.observeWalletActivityJob(nil, view, id, pilot.WalletActivityResponse{OK: true, Job: &pilot.WalletActivityJob{JobID: id, State: "running", Binding: pilot.WalletActivityBinding{View: pilot.ManifestViewPin{ViewID: view}}}})
	if !nativeWalletActivityPending(a) {
		t.Fatal("cancellation acknowledgment did not join the running task")
	}
}
