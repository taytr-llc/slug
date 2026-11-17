// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
)

func TestEventNativeWrongModeKeepsTypedFailure(t *testing.T) {
	g := &Gym{}
	for name, call := range map[string]func() pilot.EventResponse{
		"snapshot": g.EventSnapshot,
		"status":   func() pilot.EventResponse { return g.EventStatus("", "") },
		"record":   func() pilot.EventResponse { return g.EventRecord("", "") },
		"history":  func() pilot.EventResponse { return g.EventHistory("", "", 8) },
		"capture":  func() pilot.EventResponse { return g.BeginEventCapture("", "") },
		"job":      func() pilot.EventResponse { return g.EventJob("", "") },
		"cancel":   func() pilot.EventResponse { return g.CancelEventJob("", "") },
	} {
		t.Run(name, func(t *testing.T) {
			r := call()
			if r.ID != 1 || r.OK || r.Error != "event_unavailable" || r.View != nil || r.StoreID != "" || r.Snapshot != nil || r.Status != nil || r.Record != nil || r.History != nil || r.Job != nil {
				t.Fatal("wrong mode leaked event state")
			}
		})
	}
}
