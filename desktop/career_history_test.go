// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import "testing"

func TestCareerHistoryUnavailableBindings(t *testing.T) {
	g := &Gym{}
	responses := []bool{g.CareerHistorySnapshot().OK, g.BeginCareerHistory("view", "fighter").OK, g.CareerHistoryJob("view", "job").OK, g.CancelCareerHistory("view", "job").OK, g.CareerHistoryPage("view", "job", 0, 8).OK, g.CareerHistoryResult("view", "job", 1).OK}
	for _, ok := range responses {
		if ok {
			t.Fatal("missing helper succeeded")
		}
	}
	if r := g.CareerHistorySnapshot(); r.Version != 1 || r.ID != 1 || r.Error != "career_inactive" || r.Snapshot != nil || r.Job != nil || r.Page != nil {
		t.Fatal(r)
	}
}
