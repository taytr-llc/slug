// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"github.com/taytr-llc/slug/desktop/pilot"
	"strings"
	"testing"
)

func TestPoolPayoutNativeNewActionNeverAdoptsOldCapture(t *testing.T) {
	a := nativePayoutAction{viewID: strings.Repeat("a", 64), jobID: strings.Repeat("b", 64), state: "unknown"}
	old := strings.Repeat("c", 64)
	snapshot := pilot.PoolPayoutSnapshot{Version: 1, State: "idle", Activity: "none", LastJobID: old, CaptureJobID: old}
	for _, phase := range []string{"unknown", "not_started", ""} {
		t.Run(phase, func(t *testing.T) {
			current := a
			current.state = phase
			lookups := 0
			r, state, _ := nativePayoutActionSnapshot(current, func(q pilot.PoolPayoutRequest) pilot.PoolPayoutResponse {
				if q.Command == "pool_payout_job" {
					lookups++
					if q.JobID != a.jobID || q.ViewID != a.viewID {
						t.Fatal("lookup substituted old action")
					}
					return payoutNativeFailure("payout_not_found")
				}
				if q.Command != "pool_payout_snapshot" {
					t.Fatal(q.Command)
				}
				return pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Snapshot: &snapshot}
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
	r, resolved, terminal := nativePayoutActionSnapshot(a, func(q pilot.PoolPayoutRequest) pilot.PoolPayoutResponse {
		calls++
		if q.Command == "pool_payout_job" {
			return pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Job: &pilot.PoolPayoutJob{JobID: a.jobID, State: "done", Binding: pilot.PoolPayoutBinding{View: pilot.ManifestViewPin{ViewID: a.viewID}}}}
		}
		s := snapshot
		s.LastJobID, s.CaptureJobID = a.jobID, a.jobID
		return pilot.PoolPayoutResponse{ID: 1, Version: 1, OK: true, Snapshot: &s}
	})
	if calls != 2 || resolved != "admitted" || !terminal || r.Snapshot.LastJobID != a.jobID {
		t.Fatal("exact latest action not recovered")
	}
	a.state = "preparing"
	r, _, _ = nativePayoutActionSnapshot(a, func(pilot.PoolPayoutRequest) pilot.PoolPayoutResponse {
		t.Fatal("preflight queried old success")
		return pilot.PoolPayoutResponse{}
	})
	if r.Error != "outcome_unknown" || r.OK {
		t.Fatal("preflight state mislabeled")
	}
}
