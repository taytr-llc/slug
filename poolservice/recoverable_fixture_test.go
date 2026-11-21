//go:build !slugtestpow && (linux || (darwin && cgo))

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

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// Original request-only authority, source and daily ledger with public synthetic
// operator keys and a controlled node. This is service composition evidence,
// not real-node acceptance or a complete independently provisioned Setup.
func newRecoveryServerHarness(t *testing.T) (*serverHarness, registry.IssueProfile) {
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
	profile, owner := recoveryServerAuthority(t)
	descriptor := profile.Pin().Source.DescriptorID
	d, e := fighterdescriptor.ResolveCandidate(descriptor)
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
	config := poolshare.Config{Version: 2, AuthorityPin: profile.Pin().Source, Policy: prospectnode.Policy{Version: 1, Network: ctx.Network, Genesis: ctx.Genesis, DNAVersion: ctx.DNAVersion, ProspectTarget: ctx.ProspectTarget}, PayoutAddress: poolAddress, PoolTarget: strings.Repeat("f", 64), MaxWorks: 16, MaxSubmissions: 16, MaxBytes: 128 << 20}
	sourceID, e := poolshare.DailyConfigID(config)
	if e != nil {
		t.Fatal(e)
	}
	issue := registry.IssueProfile{Version: 1, BaseSetupID: strings.Repeat("b", 64), SourceConfigID: sourceID, MaxRequests: config.MaxWorks}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	issuer, e := registry.CreateRecoverableOperatorManifestAuthority(context.Background(), filepath.Join(root, "issuer"), d, profile, issue, owner)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := issuer.Close(); e != nil {
			t.Error(e)
		}
	})
	pool, e := poolshare.CreateRecoverableDaily(context.Background(), filepath.Join(root, "pool"), config, issue, issuer, node, sessions)
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
	server, e := NewRecoverableDailyServer(context.Background(), ServerConfig{Version: 3, Listen: "127.0.0.1:0"}, pool, settlement, sessions, issue)
	if e != nil {
		t.Fatal(e)
	}
	return &serverHarness{server: server, pool: pool, rounds: rounds, settlement: settlement, sessions: sessions, node: node, source: config, publicA: publicA, publicB: publicB, gymA: gymA, gymB: gymB}, issue
}

func recoveryServerAuthority(t *testing.T) (registry.OperatorProfile, *operatorauthority.Owner) {
	t.Helper()
	raw, e := os.ReadFile("../registry/testdata/operator-reference/authority-v2.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "7328ea2102c9ac993281bd732590c3e734eda26a3d180c2fa98bbd37da02c6b4" {
		t.Fatal("public operator reference")
	}
	var corpus struct {
		Positives []struct {
			Family, Name string
			Value        json.RawMessage
		}
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("operator reference")
	}
	var pin registry.PublicManifestPin
	for _, r := range corpus.Positives {
		if r.Family == "pin" && r.Name == "v1/operator_v2_distinct" {
			if json.Unmarshal(r.Value, &pin) != nil {
				t.Fatal("pin")
			}
		}
	}
	profile, e := registry.NewOperatorProfile(pin)
	if e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	paths := map[string]string{}
	for _, role := range []string{"admission", "publication", "cutoff"} {
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		paths[role] = filepath.Join(root, role)
		if e = os.WriteFile(paths[role], seed[:], 0600); e != nil {
			t.Fatal(e)
		}
	}
	genesis, e := chainhash.NewHashFromStr(pin.Source.Genesis)
	if e != nil {
		t.Fatal(e)
	}
	binding := operatorauthority.Binding{Version: 2, PinID: profile.ID(), Network: wire.BitcoinNet(pin.Source.Network), Genesis: *genesis, Keys: operatorauthority.Keys{Admission: pin.AdmissionKey, Publication: pin.PublicationKey, Cutoff: pin.CutoffKey, Checkpoint: pin.CheckpointKey}}
	owner, e := operatorauthority.OpenOnline(context.Background(), binding, operatorauthority.OnlineSeedFiles{Admission: paths["admission"], Publication: paths["publication"], Cutoff: paths["cutoff"]})
	if owner != nil {
		t.Cleanup(func() {
			if e := owner.Close(); e != nil {
				t.Error(e)
			}
		})
	}
	if e != nil {
		t.Fatal(e)
	}
	return profile, owner
}
