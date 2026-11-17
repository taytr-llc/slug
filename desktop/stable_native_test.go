// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"github.com/taytr-llc/slug/desktop/pilot"
	"testing"
)

func TestStableNativeWrongModeKeepsTypedFailure(t *testing.T) {
	g := &Gym{}
	results := []pilot.StableResponse{g.StableSnapshot(), g.BeginStableRecent(""), g.BeginStableSync(""), g.BeginStableInventory("", 0, 8), g.BeginStableRoster("", 1, 8), g.BeginStablePrepare("", 0, 1, ""), g.BeginStableApply("", ""), g.StableJob("", ""), g.StableOperation("", ""), g.StablePending(""), g.AbandonStableOperation("", ""), g.CancelStableJob("", "")}
	for _, r := range results {
		if r.ID == 0 || r.OK || r.Error != "stable_unavailable" || r.Job != nil || r.Snapshot != nil || r.Operation != nil {
			t.Fatal("closed typed failure changed", r)
		}
	}
}
