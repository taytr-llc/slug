// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"github.com/taytr-llc/slug/desktop/pilot"
	"testing"
)

func TestSeedNativeWrongMode(t *testing.T) {
	g := &Gym{}
	for name, fn := range map[string]func() pilot.SeedResponse{
		"snapshot":  g.SeedSnapshot,
		"status":    func() pilot.SeedResponse { return g.SeedStatus("", "") },
		"record":    func() pilot.SeedResponse { return g.SeedRecord("", "") },
		"history":   func() pilot.SeedResponse { return g.SeedHistory("", "", 8) },
		"prepare":   func() pilot.SeedResponse { return g.BeginSeedPrepare("", "") },
		"publish":   func() pilot.SeedResponse { return g.BeginSeedPublish("", "") },
		"reconcile": func() pilot.SeedResponse { return g.BeginSeedReconcile("", "") },
		"attest":    func() pilot.SeedResponse { return g.BeginSeedAttest("", "") },
		"job":       func() pilot.SeedResponse { return g.SeedJob("", "") },
		"cancel":    func() pilot.SeedResponse { return g.CancelSeedJob("", "") },
	} {
		t.Run(name, func(t *testing.T) {
			r := fn()
			if r.OK || r.Error != "seed_unavailable" || r.View != nil || r.StoreID != "" || r.Snapshot != nil || r.Record != nil || r.Status != nil || r.History != nil || r.Job != nil {
				t.Fatal("wrong mode leaked seed data")
			}
		})
	}
}
