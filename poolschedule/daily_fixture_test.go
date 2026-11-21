// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type dailyScheduleHarness struct {
	*settlementHarness
	work poolshare.MiningWork
}

// Same published Set52 template and scalar fixture owners as fixture_test.go,
// but fresh explicitly selected daily journals. No existing V1 root is adopted.
// Empty construction performs actual immutable issuance, not work hashing.
func newDailyScheduleHarness(t *testing.T) *dailyScheduleHarness {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal source; separate cheap refusal")
	}
	raw, err := os.ReadFile("../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("independent template changed")
	}
	var corpus struct {
		Sources []struct {
			Name     string                `json:"name"`
			Template prospectwork.Template `json:"template"`
		} `json:"sources"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("template corpus")
	}
	var template prospectwork.Template
	for _, source := range corpus.Sources {
		if source.Name == "coinbase-only" {
			template = source.Template
		}
	}
	if template.Context.Version == 0 {
		t.Fatal("missing original template")
	}
	descriptor, err := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := registry.CreateManifestAuthority(filepath.Join(t.TempDir(), "issuer"), template.Context.Network, template.Context.Genesis, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := issuer.Close(); e != nil {
			t.Error(e)
		}
	})
	info, err := issuer.Info()
	if err != nil {
		t.Fatal(err)
	}
	gym, _, _ := settlementFixtureKey(1)
	_, script, address := settlementFixtureKey(3)
	_, payScript, _ := settlementFixtureKey(4)
	node := &settlementSourceNode{template: template}
	c := template.Context
	config := poolshare.Config{Version: poolshare.Version2, AuthorityPin: info.Pin, Policy: prospectnode.Policy{Version: 1, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}, PayoutAddress: address, PoolTarget: strings.Repeat("f", 64), MaxWorks: 8, MaxSubmissions: 8, MaxBytes: 8 << 20}
	ctx := context.Background()
	pool, err := poolshare.CreateDaily(ctx, filepath.Join(t.TempDir(), "pool"), config, issuer, node, settlementSessions{hex.EncodeToString(gym.PubKey().SerializeCompressed())})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := pool.Close(); e != nil {
			t.Error(e)
		}
	})
	work, err := pool.IssueMiningWork(ctx, settlementTestToken)
	if err != nil {
		t.Fatal(err)
	}
	rounds, err := poolround.CreateDaily(ctx, filepath.Join(t.TempDir(), "rounds"), poolround.Limits{Version: poolround.Version2, MaxRounds: 8, MaxBytes: 8 << 20}, pool, node)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := rounds.Close(); e != nil {
			t.Error(e)
		}
	})
	h := &settlementHarness{issuer: issuer, pool: pool, source: rounds, node: node, backend: &settlementBackend{funding: "ready", fee: 1000, txs: make(map[string][]byte)}, config: poolsettle.Config{Version: poolsettle.Version2, ChangeScript: script, Destinations: []poolsettle.Destination{{GymID: work.Job.GymID, Script: payScript}}, MaxRounds: 8, MaxBatches: 8}, path: filepath.Join(t.TempDir(), "settlement"), gym: work.Job.GymID}
	h.store, err = poolsettle.CreateDaily(ctx, h.path, h.config, rounds, h.backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := h.store.Close(); e != nil {
			t.Error(e)
		}
	})
	return &dailyScheduleHarness{settlementHarness: h, work: work}
}

// One actual normal-PoW winner, confirmed through the existing controlled node
// observer. This does not claim an actual external node or daily-scale ledger.
func (h *dailyScheduleHarness) confirmFirst(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, _, payout := settlementFixtureKey(3)
	prepared, err := prospectwork.PrepareManifestWork(h.work.Job, h.work.Template, payout, 0, strings.Repeat("f", 64))
	if err != nil {
		t.Fatal(err)
	}
	for nonce := uint32(0); nonce < 64; nonce++ {
		check, e := prepared.CheckNonce(ctx, nonce)
		if e != nil {
			t.Fatal(e)
		}
		if check.Block {
			h.node.winner = check
			break
		}
	}
	if h.node.winner.Candidate == nil {
		t.Fatal("fixed normal nonce-search bound exhausted")
	}
	receipt, err := h.pool.Submit(ctx, settlementTestToken, h.work.Job.ID, h.node.winner.Nonce)
	if err != nil || receipt.AcceptedSequence != 1 {
		t.Fatal("original winner", err)
	}
	if _, err = h.source.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	round, err := h.source.Refresh(ctx, h.node.winner.WorkHash)
	if err != nil || round.Confirmation == nil {
		t.Fatal("original confirmed round", err)
	}
}

func dailyScheduleFixture(t *testing.T, h *dailyScheduleHarness, minute uint32) (*Scheduler, string, Config) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	id, err := DailySettlementBinding(ctx, h.store)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	c := Config{Version: Version2, SchedulerID: repeatID('4'), RuntimeID: repeatID('5'), SettlementBindingID: id, DailyUTCMinute: minute, RetryMS: 1000}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	path := filepath.Join(parent, "schedule")
	s, err := CreateDaily(ctx, path, c, h.store)
	cancel() // successful constructor admission does not govern Run lifetime
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := s.Close(context.Background()); e != nil {
			t.Error(e)
		}
	})
	return s, path, c
}
