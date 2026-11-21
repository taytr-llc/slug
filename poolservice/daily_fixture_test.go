// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// Fresh daily owners use the published legacy template and controlled node.
// This fixture proves service composition, not operator signature custody.
func newDailyServerHarness(t *testing.T) *serverHarness {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-PoW concrete source; refusal tested separately")
	}
	raw, e := os.ReadFile("../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("independent template fixture changed")
	}
	var corpus struct {
		Sources []struct {
			Name     string                `json:"name"`
			Template prospectwork.Template `json:"template"`
		} `json:"sources"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("invalid frozen template")
	}
	var template prospectwork.Template
	for _, s := range corpus.Sources {
		if s.Name == "coinbase-only" {
			template = s.Template
		}
	}
	if template.Context.Version == 0 {
		t.Fatal("missing frozen source")
	}
	descriptor, e := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if e != nil {
		t.Fatal(e)
	}
	issuer, e := registry.CreateManifestAuthority(filepath.Join(t.TempDir(), "issuer"), template.Context.Network, template.Context.Genesis, descriptor)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := issuer.Close(); e != nil {
			t.Error(e)
		}
	})
	info, e := issuer.Info()
	if e != nil {
		t.Fatal(e)
	}
	publicA, _, _ := serverFixtureKey(1)
	publicB, _, _ := serverFixtureKey(2)
	_, poolScript, poolAddress := serverFixtureKey(3)
	_, destA, _ := serverFixtureKey(4)
	_, destB, _ := serverFixtureKey(5)
	ctx := template.Context
	gymA, e := gymauth.GymID(ctx, publicA)
	if e != nil {
		t.Fatal(e)
	}
	gymB, e := gymauth.GymID(ctx, publicB)
	if e != nil {
		t.Fatal(e)
	}
	sessions := &serverSessions{keys: map[string]string{serverTokenA: publicA, serverTokenB: publicB, serverTokenRenewed: publicA}, revoked: make(map[string]bool)}
	node := &serverSourceNode{template: template}
	config := poolshare.Config{Version: 2, AuthorityPin: info.Pin, Policy: prospectnode.Policy{Version: 1, Network: ctx.Network, Genesis: ctx.Genesis, DNAVersion: ctx.DNAVersion, ProspectTarget: ctx.ProspectTarget}, PayoutAddress: poolAddress, PoolTarget: strings.Repeat("f", 64), MaxWorks: 16, MaxSubmissions: 16, MaxBytes: 16 << 20}
	pool, e := poolshare.CreateDaily(context.Background(), filepath.Join(t.TempDir(), "pool"), config, issuer, node, sessions)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := pool.Close(); e != nil {
			t.Error(e)
		}
	})
	rounds, e := poolround.CreateDaily(context.Background(), filepath.Join(t.TempDir(), "rounds"), poolround.Limits{Version: 2, MaxRounds: 16, MaxBytes: 16 << 20}, pool, node)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := rounds.Close(); e != nil {
			t.Error(e)
		}
	})
	dests := []poolsettle.Destination{{GymID: gymA, Script: destA}, {GymID: gymB, Script: destB}}
	sort.Slice(dests, func(i, j int) bool { return dests[i].GymID < dests[j].GymID })
	settlement, e := poolsettle.CreateDaily(context.Background(), filepath.Join(t.TempDir(), "settlement"), poolsettle.Config{Version: 2, ChangeScript: poolScript, Destinations: dests, MaxRounds: 16, MaxBatches: 4}, rounds, serverNoPayment{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := settlement.Close(); e != nil {
			t.Error(e)
		}
	})
	server, e := NewDailyServer(context.Background(), ServerConfig{Version: 2, Listen: "127.0.0.1:0"}, pool, settlement, sessions)
	if e != nil {
		t.Fatal(e)
	}
	return &serverHarness{server: server, pool: pool, rounds: rounds, settlement: settlement, sessions: sessions, node: node, source: config, publicA: publicA, publicB: publicB, gymA: gymA, gymB: gymB}
}
