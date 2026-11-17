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

func TestSeedBridgeWrongMode(t *testing.T) {
	b := &StableBridge{}
	for _, command := range []string{"seed_snapshot", "seed_status", "seed_record", "seed_history", "seed_prepare", "seed_publish", "seed_reconcile", "seed_attest", "seed_job", "seed_cancel"} {
		r := b.SeedRequest(context.Background(), pilot.SeedRequest{Command: command})
		if r.OK || r.Error != "seed_unavailable" {
			t.Fatal("old mode accepted seed command")
		}
	}
}

func TestSeedBridgeInvalidRequestBeforePipe(t *testing.T) {
	b := &StableBridge{seedsEnabled: true}
	for _, req := range []pilot.SeedRequest{{Command: "seed_publish"}, {Command: "seed_snapshot", Saturday: "2026-11-21"}, {Command: "seed_status", ViewID: "bad", Saturday: "2026-11-21"}, {Command: "seed_history", Limit: 9}} {
		r := b.SeedRequest(context.Background(), req)
		if r.OK || r.Error != "invalid_request" {
			t.Fatal("invalid request crossed native boundary")
		}
	}
}

// Polling cannot import a node or invoke source-store validation/projection.
func TestSeedNativePollingDependencyBoundary(t *testing.T) {
	f, e := parser.ParseFile(token.NewFileSet(), "seed_bridge.go", nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	allowed := map[string]bool{"CandidatePin": true, "ValidatePin": true, "MarshalPin": true, "ID": true, "EventKey": true, "ValidateStatus": true, "ValidateHistoryPage": true}
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
		if p.Name == "gymseed" && !allowed[s.Sel.Name] {
			t.Errorf("native seed poll invokes non-transport core operation %s", s.Sel.Name)
		}
		if p.Name == "gymevent" && s.Sel.Name != "EventForDate" {
			t.Errorf("native seed poll invokes source operation %s", s.Sel.Name)
		}
		return true
	})
	for _, i := range f.Imports {
		if i.Path.Value == "\"github.com/taytr-llc/slug/seednode\"" || i.Path.Value == "\"github.com/taytr-llc/slug/fightercondition\"" || i.Path.Value == "\"github.com/taytr-llc/slug/fightermanifest\"" {
			t.Fatal("native polling imports execution dependency", i.Path.Value)
		}
	}
}
