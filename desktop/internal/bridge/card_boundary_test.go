// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestCardBridgeWrongMode(t *testing.T) {
	b := &StableBridge{}
	for _, command := range []string{"card_snapshot", "card_status", "card_record", "card_history", "card_build", "card_build", "card_build", "card_build", "card_job", "card_cancel"} {
		r := b.CardRequest(context.Background(), pilot.CardRequest{Command: command})
		if r.OK || r.Error != "card_unavailable" {
			t.Fatal("old mode accepted card command")
		}
	}
}

func TestCardBridgeInvalidRequestBeforePipe(t *testing.T) {
	b := &StableBridge{cardsEnabled: true}
	for _, req := range []pilot.CardRequest{{Command: "card_build"}, {Command: "card_snapshot", Saturday: "2026-11-21"}, {Command: "card_status", ViewID: "bad", Saturday: "2026-11-21"}, {Command: "card_history", Limit: 9}} {
		r := b.CardRequest(context.Background(), req)
		if r.OK || r.Error != "invalid_request" {
			t.Fatal("invalid request crossed native boundary")
		}
	}
}

// Polling cannot import a node or invoke source-store validation/projection.
func TestCardNativePollingDependencyBoundary(t *testing.T) {
	f, e := parser.ParseFile(token.NewFileSet(), "card_bridge.go", nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	allowed := map[string]bool{"CandidatePin": true, "ValidatePin": true, "MarshalPin": true, "ID": true, "CardID": true}
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		s, ok := c.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		p, ok := s.X.(*ast.Ident)
		if !ok {
			return true
		}
		if p.Name == "gymcard" && !allowed[s.Sel.Name] {
			t.Errorf("native card poll invokes non-transport core operation %s", s.Sel.Name)
		}
		if p.Name == "gymseed" && s.Sel.Name != "EventKey" {
			t.Errorf("native card poll invokes non-transport seed operation %s", s.Sel.Name)
		}
		if p.Name == "gymevent" && s.Sel.Name != "EventForDate" {
			t.Errorf("native card poll invokes source operation %s", s.Sel.Name)
		}
		return true
	})
	for _, i := range f.Imports {
		if i.Path.Value == "\"github.com/taytr-llc/slug/seednode\"" || i.Path.Value == "\"github.com/taytr-llc/slug/fightercondition\"" || i.Path.Value == "\"github.com/taytr-llc/slug/fightermanifest\"" {
			t.Fatal("native polling imports execution dependency", i.Path.Value)
		}
	}
}
