// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
)

func TestEntryNativeWrongModeKeepsTypedFailure(t *testing.T) {
	g := &Gym{}
	for name, call := range map[string]func() pilot.EntryResponse{
		"snapshot":  g.EntrySnapshot,
		"upcoming":  func() pilot.EntryResponse { return g.EntryUpcoming("") },
		"status":    func() pilot.EntryResponse { return g.EntryStatus("", "") },
		"history":   func() pilot.EntryResponse { return g.EntryHistory("", "", 8) },
		"prepare":   func() pilot.EntryResponse { return g.BeginEntryPrepare("", gymentry.Request{}) },
		"apply":     func() pilot.EntryResponse { return g.BeginEntryApply("", "") },
		"check":     func() pilot.EntryResponse { return g.BeginEntryCheck("", "") },
		"job":       func() pilot.EntryResponse { return g.EntryJob("", "") },
		"cancel":    func() pilot.EntryResponse { return g.CancelEntryJob("", "") },
		"operation": func() pilot.EntryResponse { return g.EntryOperation("", "") },
		"pending":   func() pilot.EntryResponse { return g.EntryPending("") },
		"abandon":   func() pilot.EntryResponse { return g.AbandonEntryOperation("", "") },
	} {
		t.Run(name, func(t *testing.T) {
			r := call()
			if r.ID != 1 || r.OK || r.Error != "entry_unavailable" || r.View != nil || r.StoreID != "" || r.Snapshot != nil || r.Status != nil || r.History != nil || r.Job != nil || r.Operation != nil {
				t.Fatal("wrong mode leaked entry state", r)
			}
		})
	}
}
