// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
)

type poolLeagueSnapshots struct {
	pool   pilot.PoolSnapshot
	stable pilot.StableSnapshot
	entry  pilot.EntrySnapshot
	event  pilot.EventSnapshot
}

// These are new empty structural contexts from published identities, not
// modified source-bound receipts. Assertions compare relationships and refusal
// behavior; no expected digest is harvested from the implementation.
func poolLeagueFixture(t *testing.T) poolLeagueSnapshots {
	t.Helper()
	m := poolBridgeSnapshot(t)
	s := stableBridgeSnapshot(t)
	s.Status.Pin.Version = gymstable.SharedPinVersion2
	s.Status.StoreID, _ = gymstable.ID(s.Status.Pin)
	participants := make([]gymentry.Pin, 0, 2)
	entryStore := ""
	for _, fixture := range []string{"zero256", "incrementing256"} {
		p := s.Status.Pin
		p.Identity, _ = gymaccount.FixtureIdentity(fixture)
		if fixture != "zero256" {
			p.AccountID = strings.Repeat("e", 64)
		}
		sid, err := gymstable.ID(p)
		if err != nil {
			t.Fatal(err)
		}
		ns, err := gymentry.NamespaceID(p.Source.Network, p.Source.Genesis, p.AccountAuthorityID)
		if err != nil {
			t.Fatal(err)
		}
		pin := gymentry.Pin{Version: 1, NamespaceID: ns, CalendarID: gymentry.CalendarID(), RulesID: gymentry.RulesID(), StableStoreID: sid, StablePin: p}
		id, err := gymentry.ID(pin)
		if err != nil {
			t.Fatal(err)
		}
		if fixture == "zero256" {
			entryStore = id
		}
		participants = append(participants, pin)
	}
	slices.SortFunc(participants, func(a, b gymentry.Pin) int {
		return strings.Compare(a.StablePin.Identity.GymID, b.StablePin.Identity.GymID)
	})
	p := gymevent.Pin{Version: 1, CalendarID: gymentry.CalendarID(), RulesID: gymevent.RulesID(), Participants: participants}
	id, err := gymevent.ID(p)
	if err != nil {
		t.Fatal(err)
	}
	e := pilot.EntrySnapshot{Version: 1, State: "idle", View: m.View, StoreID: entryStore}
	v := pilot.EventSnapshot{Version: 1, State: "ready", View: m.View, StoreID: id, Pin: &p, CoordinatorCreatedAtMS: 1795000000000, ParticipantID: m.GymID}
	return poolLeagueSnapshots{m, s, e, v}
}

func TestPoolLeagueBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_POOL_LEAGUE_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	f := poolLeagueFixture(t)
	var commands []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var selector struct {
			ID      uint64 `json:"id"`
			Command string `json:"command"`
		}
		if json.Unmarshal(scanner.Bytes(), &selector) != nil {
			os.Exit(2)
		}
		commands = append(commands, selector.Command)
		if err := os.WriteFile(os.Getenv("SLUG_POOL_LEAGUE_COMMANDS"), []byte(strings.Join(commands, ",")), 0600); err != nil {
			os.Exit(2)
		}
		var raw []byte
		var err error
		switch selector.Command {
		case "snapshot":
			if _, err = pilot.ParsePoolRequest(scanner.Bytes()); err != nil {
				os.Exit(2)
			}
			if mode == "signed-out" {
				f.pool = pilot.BlankPoolSnapshot()
			}
			raw, err = json.Marshal(pilot.PoolResponse{ID: selector.ID, OK: true, Snapshot: &f.pool})
		case "stable_snapshot":
			if _, err = pilot.ParseStableRequest(scanner.Bytes()); err != nil {
				os.Exit(2)
			}
			if mode == "stable-view" {
				v := *f.stable.View
				v.ViewID = strings.Repeat("1", 64)
				f.stable.View = &v
			}
			if mode == "pin1" {
				f.stable.Status.Pin.Version = 1
				f.stable.Status.StoreID, _ = gymstable.ID(f.stable.Status.Pin)
			}
			raw, err = json.Marshal(pilot.StableResponse{ID: selector.ID, OK: true, Snapshot: &f.stable})
		case "entry_snapshot":
			if _, err = pilot.ParseEntryRequest(scanner.Bytes()); err != nil {
				os.Exit(2)
			}
			if mode == "entry-view" {
				v := *f.entry.View
				v.ViewID = strings.Repeat("2", 64)
				f.entry.View = &v
			}
			raw, err = pilot.MarshalEntryResponse(pilot.EntryResponse{ID: selector.ID, OK: true, View: f.entry.View, StoreID: f.entry.StoreID, Snapshot: &f.entry})
		case "event_snapshot":
			if _, err = pilot.ParseEventRequest(scanner.Bytes()); err != nil {
				os.Exit(2)
			}
			if mode == "signed-out" {
				f.event = pilot.EventSnapshot{Version: 1, State: "unavailable", Error: "event_unavailable"}
			} else if mode == "foreign-source" || mode == "mixed-pin" {
				// The other participant differs; the current one still matches.
				for i := range f.event.Pin.Participants {
					p := &f.event.Pin.Participants[i]
					if p.StablePin.Identity.GymID == f.pool.GymID {
						continue
					}
					if mode == "foreign-source" {
						p.StablePin.Source.AuthorityID = strings.Repeat("3", 64)
					} else {
						p.StablePin.Version = 1
					}
					p.StableStoreID, _ = gymstable.ID(p.StablePin)
				}
				f.event.StoreID, err = gymevent.ID(*f.event.Pin)
				if err != nil {
					os.Exit(2)
				}
			}
			if mode == "wrong-family" {
				raw, err = json.Marshal(pilot.PoolResponse{ID: selector.ID, OK: true, Snapshot: &f.pool})
			} else {
				raw, err = pilot.MarshalEventResponse(pilot.EventResponse{ID: selector.ID, OK: true, View: f.event.View, StoreID: f.event.StoreID, Snapshot: &f.event})
			}
		default:
			os.Exit(3)
		}
		if err != nil {
			os.Exit(2)
		}
		fmt.Println(string(raw))
	}
	os.Exit(0)
}

func TestPoolLeagueBridgeUsesPoolContextAndRejectsSubstitution(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal public candidate identities")
	}
	for _, mode := range []string{"valid", "signed-out", "stable-view", "entry-view", "pin1", "foreign-source", "mixed-pin", "wrong-family"} {
		t.Run(mode, func(t *testing.T) {
			path := t.TempDir() + "/commands"
			cmd := exec.Command(os.Args[0], "-test.run=^TestPoolLeagueBridgeProcess$", "-test.count=1")
			cmd.Env = append(os.Environ(), "SLUG_POOL_LEAGUE_BRIDGE_PROCESS="+mode, "SLUG_POOL_LEAGUE_COMMANDS="+path)
			base, err := startStable(cmd)
			if err != nil {
				t.Fatal(err)
			}
			base.poolMode, base.entriesEnabled, base.eventsEnabled = true, true, true
			b := &PoolBridge{base}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			r := b.EventRequest(ctx, pilot.EventRequest{Command: "event_snapshot"})
			closeErr := b.Close()
			if closeErr != nil {
				t.Fatal("helper did not drain", closeErr)
			}
			if r.OK != (mode == "valid" || mode == "signed-out") {
				t.Fatal("wrong context acceptance", r.Error)
			}
			if mode == "valid" && (r.Snapshot == nil || r.Snapshot.Pin == nil || b.eventPin == nil) {
				t.Fatal("missing coordinator authority")
			}
			if mode == "signed-out" && (r.Snapshot == nil || r.Snapshot.View != nil || r.Snapshot.Pin != nil) {
				t.Fatal("signed-out coordinator disclosure")
			}
			if !r.OK && (r.View != nil || r.Snapshot != nil || r.StoreID != "") {
				t.Fatal("refused response disclosed payload")
			}
			commands, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := "snapshot,stable_snapshot,entry_snapshot,event_snapshot"
			switch mode {
			case "signed-out":
				want = "snapshot,event_snapshot"
			case "stable-view", "pin1":
				want = "snapshot,stable_snapshot"
			case "entry-view":
				want = "snapshot,stable_snapshot,entry_snapshot"
			}
			if string(commands) != want {
				t.Fatalf("family preparation %q, want %q", commands, want)
			}
		})
	}
}

func TestPoolLeagueFinalGateViewAndCapabilityIsolation(t *testing.T) {
	view := pilot.ManifestViewPin{ViewID: strings.Repeat("a", 64)}
	other := view
	other.Generation++
	base := &StableBridge{poolMode: true, entriesEnabled: true, eventsEnabled: true, manifestView: &view, view: &view}
	b := &PoolBridge{base}
	if !b.poolEventPrepared(&view) || b.poolEventPrepared(&other) || b.poolEventPrepared(nil) {
		t.Fatal("prepared view correlation")
	}
	base.manifestView = &other
	if b.poolEventPrepared(&view) {
		t.Fatal("replacement pool login borrowed original context")
	}
	base.manifestView, base.view = nil, nil
	if !b.poolEventPrepared(nil) || b.poolEventPrepared(&view) {
		t.Fatal("blank context correlation")
	}
	// No pipe channels are installed: these refusals must occur before I/O.
	if r := b.PreviewRequest(t.Context(), pilot.PreviewRequest{Command: "preview_snapshot"}); r.OK || r.Error != "preview_unavailable" {
		t.Fatal("pool practice enabled")
	}
	if r := b.FightPreviewPortrait(t.Context(), "", "", ""); r.OK || r.Error != "preview_unavailable" {
		t.Fatal("pool practice portrait enabled")
	}
	if base.seedsEnabled || base.cardsEnabled || base.boutsEnabled || base.resultsEnabled || base.careersEnabled {
		t.Fatal("later pipeline enabled")
	}
	if r := b.EventRequest(t.Context(), pilot.EventRequest{Command: "event_record", ViewID: "invalid"}); r.OK || r.Error != "invalid_request" {
		t.Fatal("invalid event reached context preparation")
	}
	base.gate = make(chan struct{}, 1)
	base.gate <- struct{}{}
	if r := b.EventRequest(t.Context(), pilot.EventRequest{Command: "event_snapshot"}); r.OK || r.Error != "busy" || base.event != nil || base.eventPin != nil {
		t.Fatal("busy context preparation changed correlation")
	}
	<-base.gate
}

func TestPoolLeagueBridgeRetainsExactCoordinator(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal public candidate identities")
	}
	f := poolLeagueFixture(t)
	pin := f.stable.Status.Pin
	base := &StableBridge{poolMode: true, entriesEnabled: true, eventsEnabled: true, poolSourceID: f.pool.SourceBindingID, poolSourcePin: &pin.Source,
		manifestView: f.pool.View, view: f.stable.View, stablePin: &pin, storeID: f.stable.Status.StoreID}
	entry, store, ok := base.entryPin()
	if !ok || store != f.entry.StoreID {
		t.Fatal("entry context")
	}
	base.entry = &entryCorrelation{view: *f.entry.View, pin: entry, storeID: store}
	response := pilot.EventResponse{ID: 1, OK: true, View: f.event.View, StoreID: f.event.StoreID, Snapshot: &f.event}
	if !base.rememberEventSnapshot(response) {
		t.Fatal("exact shared coordinator refused")
	}
	f.event.CoordinatorCreatedAtMS++
	if base.rememberEventSnapshot(response) {
		t.Fatal("changed coordinator creation time adopted")
	}
	f.event.CoordinatorCreatedAtMS--
	base.clearEventView()
	if base.eventPin == nil || base.event != nil {
		t.Fatal("logout forgot authority or retained view")
	}
	if !base.rememberEventSnapshot(response) {
		t.Fatal("unchanged coordinator could not reattach")
	}
	// A record read has no authority establishment path.
	base.eventPin = nil
	if _, _, ready := base.eventContext(); ready {
		t.Fatal("cached participant fabricated coordinator authority")
	}
}
