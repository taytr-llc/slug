// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"testing"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
)

func TestPoolNativeLeagueUninitializedRefusesWithoutFallback(t *testing.T) {
	g := &Gym{poolHelper: &bridge.PoolBridge{}, stableHelper: &bridge.StableBridge{}}
	entry := g.EntrySnapshot()
	event := g.EventSnapshot()
	if entry.OK || entry.Error != "entry_unavailable" || entry.View != nil || entry.Snapshot != nil || event.OK || event.Error != "event_unavailable" || event.View != nil || event.Snapshot != nil {
		t.Fatal("uninitialized explicit pool route borrowed another mode")
	}
}
