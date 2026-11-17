// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"bytes"
	"github.com/taytr-llc/slug/gymentry"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestEventCodecClosedShapesAndOwnedValues(t *testing.T) {
	p := fixturePin(t)
	ev, e := EventForDate(p, "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	v := EventStatus{Version: 1, StoreID: ev.StoreID, Event: ev, Phase: "before_cutoff", CapturedParticipants: []string{}, CoordinatorCreatedAtMS: ev.CutoffMS - 2, ObservedAtMS: ev.CutoffMS - 1, SeedAuthority: "unavailable"}
	raw, e := MarshalEventStatus(p, v)
	if e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{append(append([]byte{}, raw...), []byte(" {}")...), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"\u0076ersion":1`), 1), bytes.Replace(raw, []byte(`"exists":false`), []byte(`"exists":null`), 1), bytes.Replace(raw, []byte(`"captured_participants":[]`), []byte(`"captured_participants":null`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1e0`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":-0`), 1), bytes.Replace(raw, []byte(`"exists":false`), []byte(`"extra":[],"exists":false`), 1)} {
		if _, e = ParseEventStatus(p, bad); e == nil {
			t.Fatalf("accepted malformed frame %.80s", bad)
		}
	}
	escaped := bytes.Replace(raw, []byte(`"exists"`), []byte(`"\u0065xists"`), 1)
	got, e := ParseEventStatus(p, escaped)
	if e != nil || !equal(got, v) {
		t.Fatal("escaped ASCII key transport", e)
	}
	pin, e := ValidatePin(p)
	if e != nil {
		t.Fatal(e)
	}
	pin.Participants[0].StablePin.AccountID = strings.Repeat("f", 64)
	if pin.Participants[0].StablePin.AccountID == p.Participants[0].StablePin.AccountID {
		t.Fatal("validator retained caller slice")
	}
	oversized := p
	oversized.Participants = make([]gymentry.Pin, 100000)
	if _, e = MarshalPin(oversized); e == nil {
		t.Fatal("typed bound")
	}
}
func TestEventStructuralPathsHaveNoProofOrPortraitCalls(t *testing.T) {
	for _, name := range []string{"codec.go", "json.go", "validate.go", "calendar.go", "audit.go"} {
		f, e := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if e != nil {
			t.Fatal(e)
		}
		for _, im := range f.Imports {
			for _, blocked := range []string{"fighterportrait", "/registry", "/prospect/", "argon2"} {
				if strings.Contains(im.Path.Value, blocked) {
					t.Fatalf("structural path %s imports %s", name, blocked)
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "WithCutoffSnapshot", "WithCutoffSelection", "GetManifestAdmission", "Render":
				t.Errorf("structural path %s invokes %s", name, sel.Sel.Name)
			}
			return true
		})
	}
	// The existing receipt validator is deliberately structural too.
	b, e := os.ReadFile("../gymentry/validate.go")
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(b, []byte("fighterportrait")) {
		t.Fatal("entry validation gained a portrait dependency")
	}
}
func FuzzEventStatusCodec(f *testing.F) {
	p := fixturePin(f)
	ev, e := EventForDate(p, "2026-11-21")
	if e != nil {
		f.Fatal(e)
	}
	v := EventStatus{Version: 1, StoreID: ev.StoreID, Event: ev, Phase: "before_cutoff", CapturedParticipants: []string{}, CoordinatorCreatedAtMS: ev.CutoffMS - 2, ObservedAtMS: ev.CutoffMS - 1, SeedAuthority: "unavailable"}
	raw, e := MarshalEventStatus(p, v)
	if e != nil {
		f.Fatal(e)
	}
	f.Add(raw)
	f.Add([]byte(`{"version":1,"version":1}`))
	f.Add([]byte(`{"captured_participants":[null]}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		v, e := ParseEventStatus(p, b)
		if e != nil {
			return
		}
		canonical, e := MarshalEventStatus(p, v)
		if e != nil || len(canonical) > MaxEventSize {
			t.Fatal(e)
		}
		again, e := ParseEventStatus(p, canonical)
		if e != nil || !equal(again, v) {
			t.Fatal(e)
		}
	})
}
