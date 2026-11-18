// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"encoding/json"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func requireNormal(t *testing.T) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal authority profile")
	}
}
func realTemp(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func eventPin(t testing.TB) gymevent.Pin {
	t.Helper()
	raw, e := os.ReadFile("../gymstable/testdata/reference/stable_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var v struct{ Pins []struct{ Pin gymstable.Pin } }
	if json.Unmarshal(raw, &v) != nil || len(v.Pins) != 2 {
		t.Fatal("existing public pins")
	}
	p := gymevent.Pin{Version: 1, CalendarID: gymentry.CalendarID(), RulesID: gymevent.RulesID(), Participants: []gymentry.Pin{}}
	for _, x := range v.Pins {
		ns, _ := gymentry.NamespaceID(x.Pin.Identity.Network, x.Pin.Identity.Genesis, x.Pin.AccountAuthorityID)
		id, _ := gymstable.ID(x.Pin)
		p.Participants = append(p.Participants, gymentry.Pin{Version: 1, NamespaceID: ns, CalendarID: gymentry.CalendarID(), RulesID: gymentry.RulesID(), StableStoreID: id, StablePin: x.Pin})
	}
	sort.Slice(p.Participants, func(i, j int) bool {
		return p.Participants[i].StablePin.Identity.GymID < p.Participants[j].StablePin.Identity.GymID
	})
	return p
}

func modelSource(t testing.TB) (Pin, gymseed.CardSource) {
	t.Helper()
	raw, e := os.ReadFile("../gymseed/testdata/reference/seed_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var c struct {
		Pin  struct{ Value gymseed.Pin }
		DTOs []struct {
			Kind  string
			Value json.RawMessage
		}
	}
	if json.Unmarshal(raw, &c) != nil {
		t.Fatal("old seed corpus")
	}
	p, e := CandidatePin(c.Pin.Value)
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range c.DTOs {
		if x.Kind != "Record" {
			continue
		}
		var r gymseed.Record
		if json.Unmarshal(x.Value, &r) != nil || r.Finality == nil || r.Anchor == nil || r.Target == nil || len(r.Conflicts) != 0 {
			continue
		}
		v := gymseed.CardSource{Version: 1, StoreID: r.StoreID, EventKey: r.EventKey, SourceReceipt: r.SourceReceipt, Declaration: r.Declaration, Anchor: *r.Anchor, Target: *r.Target, Finality: *r.Finality}
		if _, e := gymseed.ValidateCardSource(c.Pin.Value, v); e == nil {
			return p, v
		}
	}
	t.Fatal("no existing modeled finality fixture")
	return Pin{}, gymseed.CardSource{}
}
