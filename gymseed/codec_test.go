// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
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
func structuralRecord(t testing.TB) (Pin, Record) {
	t.Helper()
	raw, e := os.ReadFile("../gymevent/testdata/reference/event_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var c struct {
		Pin     struct{ Value gymevent.Pin }
		Records []struct{ Value gymevent.SnapshotReceipt }
	}
	if json.Unmarshal(raw, &c) != nil || len(c.Records) == 0 {
		t.Fatal("old44 public modeled receipts")
	}
	p, e := CandidatePin(c.Pin.Value)
	if e != nil {
		t.Fatal(e)
	}
	source := c.Records[0].Value
	decl, e := declarationFor(p, source)
	if e != nil {
		t.Fatal(e)
	}
	id, _ := ID(p)
	r := Record{Version: 1, StoreID: id, EventKey: eventKey(p, source.Input.Event), SourceReceipt: source, Declaration: decl, PreparedAtMS: source.CompletedAtMS, Cursor: initialCursor(p, eventKey(p, source.Input.Event), 0), ScanRestarts: []ScanRestart{}, Conflicts: []Conflict{}, Observation: Observation{NodeState: "unobserved"}}
	return p, r
}
func TestSeedClosedCodecAndOwnedValues(t *testing.T) {
	requireNormal(t)
	p, r := structuralRecord(t)
	raw, e := MarshalRecord(p, r)
	if e != nil {
		t.Fatal("positive modeled source control", e)
	}
	parsed, e := ParseRecord(p, raw)
	if e != nil || !equal(parsed, r) {
		t.Fatal("roundtrip", e)
	}
	parsed.SourceReceipt.Captures[0].CapturedAtMS++
	if equal(parsed, r) {
		t.Fatal("alias")
	}
	for name, mutate := range map[string]func([]byte) []byte{"unknown": func(b []byte) []byte { return append([]byte(`{"unexpected":1,`), b[1:]...) }, "duplicate": func(b []byte) []byte { return append([]byte(`{"version":1,`), b[1:]...) }, "negative": func(b []byte) []byte { return bytes.Replace(b, []byte(`"generation":0`), []byte(`"generation":-1`), 1) }, "exponent": func(b []byte) []byte { return bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1e0`), 1) }, "null_array": func(b []byte) []byte {
		return bytes.Replace(b, []byte(`"scan_restarts":[]`), []byte(`"scan_restarts":null`), 1)
	}} {
		t.Run(name, func(t *testing.T) {
			if _, e := ParseRecord(p, mutate(raw)); e == nil {
				t.Fatal("accepted malformed shape")
			}
		})
	}
	bad := r
	bad.Declaration.Declaration.RulesID = strings.Repeat("a", 64)
	if _, e = MarshalRecord(p, bad); e == nil {
		t.Fatal("forged rules")
	}
	bad = r
	bad.Cursor.Height = 0
	bad.Cursor.Hash = strings.Repeat("b", 64)
	bad.Cursor.CompleteToHeight = 1
	bad.Cursor.CompleteToHash = bad.Cursor.Hash
	if _, e = MarshalRecord(p, bad); e == nil {
		t.Fatal("cursor overclaim")
	}
	bad = r
	bad.Observation.NodeState = "online"
	bad.Observation.ObservedAtMS = r.PreparedAtMS
	bad.Observation.ChainObservedAtMS = r.PreparedAtMS
	if _, e = MarshalRecord(p, bad); e == nil {
		t.Fatal("missing chain tip")
	}
}
func TestSeedInitializeOpenAndProfile(t *testing.T) {
	ctx := context.Background()
	if wire.TestPoW {
		if _, e := Create(ctx, filepath.Join(realTemp(t), "seed"), Pin{}, nil); !errors.Is(e, ErrProfile) {
			t.Fatal(e)
		}
		return
	}
	ep := eventPin(t)
	events, e := gymevent.Create(ctx, filepath.Join(realTemp(t), "events"), ep)
	if e != nil {
		t.Fatal(e)
	}
	defer events.Close()
	p, e := CandidatePin(ep)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(realTemp(t), "seed")
	s, e := Create(ctx, path, p, events)
	if e != nil {
		t.Fatal(e)
	}
	info, e := s.Info(ctx)
	if e != nil || info.FundingHeight != -1 || info.EventCount != 0 {
		t.Fatal(info, e)
	}
	s.Close()
	s, e = ResumePrepared(ctx, path, p, events)
	if e != nil {
		t.Fatal(e)
	}
	again, _ := s.Info(ctx)
	if !equal(info, again) {
		t.Fatal("creation changed")
	}
	s.Close()
	if e = os.WriteFile(filepath.Join(path, "unknown"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	if s, e = Open(ctx, path, p, events); e == nil {
		s.Close()
		t.Fatal("unknown path accepted")
	}
}
func TestSeedInitializationClockAfterPin(t *testing.T) {
	requireNormal(t)
	ctx := context.Background()
	ep := eventPin(t)
	events, e := gymevent.Create(ctx, filepath.Join(realTemp(t), "events"), ep)
	if e != nil {
		t.Fatal(e)
	}
	defer events.Close()
	p, _ := CandidatePin(ep)
	now := time.Now().UnixMilli() + 100
	after := now + 1000
	seen := false
	s, e := create(ctx, filepath.Join(realTemp(t), "seed"), p, events, func() time.Time { return time.UnixMilli(now) }, func(stage string) {
		if stage == "init-synced" {
			seen = true
			now = after
		}
	})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	info, _ := s.Info(ctx)
	if !seen || info.CreatedAtMS != after {
		t.Fatal("pre-intent time reused", info)
	}
}
