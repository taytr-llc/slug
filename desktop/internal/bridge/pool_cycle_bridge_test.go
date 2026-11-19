//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
)

func TestPoolCycleBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_POOL_CYCLE_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	f := poolLeagueFixture(t)
	sp, _ := gymseed.CandidatePin(*f.event.Pin)
	sid, _ := gymseed.ID(sp)
	cp, _ := gymcard.CandidatePin(sp)
	cid, _ := gymcard.ID(cp)
	bp, _ := gymbout.CandidatePin(cp)
	bid, _ := gymbout.ID(bp)
	rp, _ := gymresult.CandidatePin(bp)
	rid, _ := gymresult.ID(rp)
	created := f.event.CoordinatorCreatedAtMS
	var commands []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var q struct {
			ID      uint64
			Command string
		}
		if json.Unmarshal(scanner.Bytes(), &q) != nil {
			os.Exit(2)
		}
		commands = append(commands, q.Command)
		if os.WriteFile(os.Getenv("SLUG_POOL_CYCLE_COMMANDS"), []byte(strings.Join(commands, ",")), 0600) != nil {
			os.Exit(2)
		}
		var raw []byte
		var err error
		switch q.Command {
		case "snapshot":
			raw, err = json.Marshal(pilot.PoolResponse{ID: q.ID, OK: true, Snapshot: &f.pool})
		case "stable_snapshot":
			raw, err = json.Marshal(pilot.StableResponse{ID: q.ID, OK: true, Snapshot: &f.stable})
		case "entry_snapshot":
			raw, err = pilot.MarshalEntryResponse(pilot.EntryResponse{ID: q.ID, OK: true, View: f.entry.View, StoreID: f.entry.StoreID, Snapshot: &f.entry})
		case "event_snapshot":
			raw, err = pilot.MarshalEventResponse(pilot.EventResponse{ID: q.ID, OK: true, View: f.event.View, StoreID: f.event.StoreID, Snapshot: &f.event})
		case "seed_snapshot":
			v := pilot.SeedSnapshot{Version: 1, State: "ready", View: f.pool.View, StoreID: sid, Pin: &sp, CreatedAtMS: created, PublisherAccountID: sp.PublisherAccountID, NodeState: "ready"}
			raw, err = pilot.MarshalSeedResponse(pilot.SeedResponse{ID: q.ID, OK: true, View: v.View, StoreID: sid, Snapshot: &v})
		case "card_snapshot":
			v := pilot.CardSnapshot{Version: 1, State: "ready", View: f.pool.View, StoreID: cid, Pin: &cp, SourceCreatedAtMS: created, CreatedAtMS: created}
			raw, err = pilot.MarshalCardResponse(pilot.CardResponse{ID: q.ID, OK: true, View: v.View, StoreID: cid, Snapshot: &v})
		case "bout_snapshot":
			v := pilot.BoutSnapshot{Version: 1, State: "ready", View: f.pool.View, StoreID: bid, Pin: &bp, SourceCreatedAtMS: created, CreatedAtMS: created}
			raw, err = pilot.MarshalBoutResponse(pilot.BoutResponse{ID: q.ID, OK: true, View: v.View, StoreID: bid, Snapshot: &v})
		case "result_snapshot":
			v := pilot.ResultSnapshot{Version: 1, State: "ready", View: f.pool.View, StoreID: rid, Pin: &rp, SourceCreatedAtMS: created, CreatedAtMS: created, PublisherAccountID: rp.PublisherAccountID, NodeState: "online", NodeOwner: "shared_pool"}
			if mode == "changed-view" {
				view := *v.View
				view.ViewID = strings.Repeat("f", 64)
				v.View = &view
			}
			raw, err = pilot.MarshalResultResponse(pilot.ResultResponse{ID: q.ID, OK: true, View: v.View, StoreID: rid, Snapshot: &v})
		case "career_history_snapshot":
			raw, err = pilot.MarshalCareerHistoryResponse(pilot.CareerHistoryResponse{ID: q.ID, Version: 1, Error: "busy"})
		default:
			os.Exit(3)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if _, err = fmt.Fprintln(os.Stdout, string(raw)); err != nil {
			os.Exit(2)
		}
	}
	os.Exit(0)
}

func poolCycleBridgeFixture(t *testing.T, mode string) (*PoolBridge, string) {
	t.Helper()
	commands := filepath.Join(t.TempDir(), "commands")
	cmd := exec.Command(os.Args[0], "-test.run=^TestPoolCycleBridgeProcess$")
	cmd.Env = append(os.Environ(), "SLUG_POOL_CYCLE_BRIDGE_PROCESS="+mode, "SLUG_POOL_CYCLE_COMMANDS="+commands)
	b, err := startStable(cmd)
	if err != nil {
		t.Fatal(err)
	}
	b.poolMode = true
	b.entriesEnabled, b.eventsEnabled, b.seedsEnabled, b.cardsEnabled, b.boutsEnabled, b.resultsEnabled, b.careersEnabled = true, true, true, true, true, true, true
	p := &PoolBridge{b}
	t.Cleanup(func() { _ = p.Close() })
	return p, commands
}

func TestPoolCycleBridgeUsesPoolChainForDiscovery(t *testing.T) {
	want := "snapshot,stable_snapshot,entry_snapshot,event_snapshot,seed_snapshot,card_snapshot,bout_snapshot,result_snapshot"
	for _, family := range []string{"result", "career", "history"} {
		t.Run(family, func(t *testing.T) {
			b, path := poolCycleBridgeFixture(t, "ready")
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			switch family {
			case "result":
				r := b.ResultRequest(ctx, pilot.ResultRequest{Command: "result_snapshot"})
				if !r.OK || r.Snapshot == nil || r.Snapshot.NodeOwner != "shared_pool" {
					t.Fatal("pool result discovery", r.Error)
				}
			case "career":
				f := poolLeagueFixture(t)
				r := b.CareerRequest(ctx, pilot.CareerRequest{Command: "career_inventory", ViewID: f.pool.View.ViewID, JobID: strings.Repeat("b", 64), Limit: 8})
				if r.OK || r.Error != "stable_not_found" {
					t.Fatal("career must discover exact pool result then require selected job", r.Error)
				}
			case "history":
				r := b.CareerHistoryRequest(ctx, pilot.CareerHistoryRequest{Command: "career_history_snapshot"})
				if r.OK || r.Error != "busy" {
					t.Fatal("history closed response", r.Error)
				}
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			expected := want
			if family == "history" {
				expected += ",career_history_snapshot"
			}
			if string(raw) != expected {
				t.Fatal("unexpected authority path", string(raw))
			}
		})
	}
}

func TestPoolCycleBridgeRejectsResultViewReplacement(t *testing.T) {
	b, _ := poolCycleBridgeFixture(t, "changed-view")
	r := b.ResultRequest(t.Context(), pilot.ResultRequest{Command: "result_snapshot"})
	if r.OK || r.Snapshot != nil || r.View != nil {
		t.Fatal("replacement result view disclosed")
	}
	if _, _, ready := b.resultContext(); ready {
		t.Fatal("replacement context retained")
	}
}
