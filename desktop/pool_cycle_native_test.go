// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"testing"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
)

func TestPoolCycleNativeUninitializedRefusesWithoutFallback(t *testing.T) {
	g := &Gym{poolHelper: &bridge.PoolBridge{}, stableHelper: &bridge.StableBridge{}}
	seed, card, bout, result := g.SeedSnapshot(), g.CardSnapshot(), g.BoutSnapshot(), g.ResultSnapshot()
	if seed.OK || seed.Error != "seed_unavailable" || card.OK || card.Error != "card_unavailable" || bout.OK || bout.Error != "bout_unavailable" || result.OK || result.Error != "result_unavailable" {
		t.Fatal("uninitialized pool pipeline borrowed another mode")
	}
	if g.CareerPreviewSnapshot().Error != "career_inactive" || g.CareerFightPreviewPortrait("", "", "").Error != "career_inactive" {
		t.Fatal("pool route enabled practice")
	}
	if g.CareerMode() || g.CareerHistorySnapshot().Error != "career_inactive" || g.CareerInventory("", "", 0, 8).Error != "career_inactive" || g.CareerRoster("", "", 1, 8).Error != "career_inactive" {
		t.Fatal("uninitialized pool acquired career context")
	}
	if g.BoutPortrait("", "", "", "").Error != "bout_unavailable" || g.CareerHistoryResult("", "", 1).Error != "result_unavailable" {
		t.Fatal("uninitialized retained evidence escaped its mode")
	}
}
