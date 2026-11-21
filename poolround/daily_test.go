// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestDailyRoundIndependentBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/daily-v2/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "bc843808b12bc731d73fbcb943b4fc43ff5e9f63e21185c1ecd42c4b3e1e2076" {
		t.Fatal("independent round reference changed")
	}
	var corpus struct {
		Cases []struct {
			Name   string           `json:"name"`
			Limits Limits           `json:"limits"`
			Source poolshare.Config `json:"source"`
			Marker string           `json:"marker"`
			Meta   string           `json:"meta"`
			Prefix string           `json:"initial_prefix"`
		} `json:"cases"`
		Keys []struct {
			Sequence uint64 `json:"sequence"`
			Hex      string `json:"hex"`
		} `json:"index_keys"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Cases) != 4 {
		t.Fatal("reference grammar")
	}
	for _, c := range corpus.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if validLimits(c.Limits) != (c.Limits.Version == Version1) || validDailyLimits(c.Limits) != (c.Limits.Version == Version2) {
				t.Fatal("profile limit crossover")
			}
			markerRaw, _ := json.Marshal(marker{application, c.Limits.Version, c.Limits, c.Source})
			metaRaw, _ := json.Marshal(metadata{Application: application, Version: c.Limits.Version, Limits: c.Limits, Source: c.Source, Prefix: initialPrefix(c.Source)})
			if string(markerRaw) != c.Marker || string(metaRaw) != c.Meta || initialPrefix(c.Source) != c.Prefix {
				t.Fatal("independent canonical bytes differ")
			}
		})
	}
	for _, k := range corpus.Keys {
		if hex.EncodeToString(indexKey(k.Sequence)) != k.Hex {
			t.Fatal("global index was reset or recoded")
		}
	}
	for _, l := range []Limits{{2, 0, 4096}, {2, 16385, 4096}, {2, 1, 4095}, {2, 1, 8<<30 + 1}, {1, 721, 8 << 30}, {3, 1, 4096}} {
		if validDailyLimits(l) {
			t.Fatalf("invalid selected limits: %+v", l)
		}
	}
}

// Conditional arithmetic only: symbolic work does not enter a concrete source.
// The independently specified invariant is each two-share round splits its ten
// SLUG reward equally; global sequence and duplicate guards span all windows.
func dailyModeledEntry(n uint64, winner bool) poolshare.AccountingEntry {
	gym := fmt.Sprintf("%064x", n%2+1)
	j := prospectjob.ManifestJob{ID: fmt.Sprintf("%064x", n+100000), GymID: gym, IssuedAtMS: 1000, ExpiresAtMS: 301000, Context: prospect.Context{ParentHeight: 61}}
	header := strings.Repeat("00", 80)
	r := poolshare.Submission{Version: 1, Sequence: n, AcceptedSequence: n, JobID: j.ID, GymID: gym, PoolTarget: strings.Repeat("f", 64), Header: header, WorkHash: fmt.Sprintf("%064x", n+200000), DuplicateKey: fmt.Sprintf("%064x", n+300000), Pool: true, Block: winner, ReceivedAtMS: 1001, AcceptedAtMS: 1001, Acceptance: "current-parent"}
	e := poolshare.AccountingEntry{Share: r, Job: j}
	if winner {
		e.Share.BlockHeight = 62
		e.RewardAtoms = 1000000000
		e.Proof = &prospect.Proof{Version: 2, JobID: j.ID, GymID: gym, Header: header, MerkleBranch: []string{}}
	}
	return e
}

func TestDailyRoundLifetimeAndWindowBounds(t *testing.T) {
	cfg := poolshare.Config{Version: 2, PoolTarget: strings.Repeat("f", 64), PayoutAddress: "conditional-only"}
	b := newRoundDerivation(cfg)
	for n := uint64(1); n <= maxDailyAccepted; n++ {
		r, err := b.add(dailyModeledEntry(n, n%2 == 0))
		if err != nil {
			t.Fatal(n, err)
		}
		if n%2 != 0 {
			if r != nil {
				t.Fatal("ordinary share closed round")
			}
			continue
		}
		if r == nil || r.Version != Version1 || r.Sequence != n/2 || r.FirstAccepted != n-1 || r.LastAccepted != n || len(r.Allocations) != 2 || b.overWindow {
			t.Fatal("global sequence/window changed")
		}
		for _, a := range r.Allocations {
			if a.Shares != 1 || a.Numerator != "500000000" || a.Denominator != "1" {
				t.Fatal("two-way exact allocation changed")
			}
		}
	}
	if _, err := b.add(dailyModeledEntry(maxDailyAccepted+1, true)); !errors.Is(err, ErrSourceChanged) {
		t.Fatal("global source limit lost", err)
	}
	legacy := cfg
	legacy.Version = 1
	old := newRoundDerivation(legacy)
	for n := uint64(1); n <= 4096; n++ {
		if _, err := old.add(dailyModeledEntry(n, true)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := old.add(dailyModeledEntry(4097, true)); !errors.Is(err, ErrSourceChanged) {
		t.Fatal("V1 lifetime widened")
	}
	entries := make([]poolshare.AccountingEntry, 4098)
	for i := range entries {
		entries[i] = dailyModeledEntry(uint64(i+1), i == len(entries)-1)
	}
	if _, err := derive(cfg, entries); !errors.Is(err, ErrCapacity) {
		t.Fatal("long window escaped fixed bound", err)
	}
	entries[len(entries)-1].Share.DuplicateKey = entries[0].Share.DuplicateKey
	if _, err := derive(cfg, entries); !errors.Is(err, ErrSourceChanged) {
		t.Fatal("capacity hid a late duplicate", err)
	}
}

func TestDailyRoundOriginalSourceRestartAndFences(t *testing.T) {
	h := newRoundProfileHarness(t, false, Version2)
	ctx := context.Background()
	if limits, source, err := h.store.DailyBinding(ctx); err != nil || limits != h.limits || source != h.sourceConfig {
		t.Fatal("original daily limits/source binding", limits, source, err)
	}
	if got, err := h.store.Sync(ctx); err != nil || got.Through != 3 || got.Rounds != 1 {
		t.Fatal(got, err)
	}
	h.observer.depth = 60
	before, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil || before.Confirmation == nil {
		t.Fatal(err)
	}
	if _, err = Create(filepath.Join(t.TempDir(), "legacy"), Limits{1, 16, 32 << 20}, h.source, h.observer); !errors.Is(err, ErrConfig) {
		t.Fatal("legacy source crossover", err)
	}
	if err = h.store.Close(); err != nil {
		t.Fatal(err)
	}
	if got, e := Open(h.path, h.limits, h.source, h.observer); got != nil || !errors.Is(e, ErrConfig) {
		t.Fatal("legacy reopen crossover", e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if got, e := OpenDaily(canceled, h.path, h.limits, h.source, h.observer); got != nil || !errors.Is(e, context.Canceled) {
		t.Fatal("canceled open disclosed owner", e)
	}
	changed := h.limits
	changed.MaxRounds++
	if got, e := OpenDaily(ctx, h.path, changed, h.source, h.observer); got != nil || !errors.Is(e, ErrSourceChanged) {
		t.Fatal("changed selected bounds adopted", e)
	}
	calls := h.observer.calls
	h.store, err = OpenDaily(ctx, h.path, h.limits, h.source, h.observer)
	if err != nil {
		t.Fatal(err)
	}
	after, err := h.store.Get(ctx, h.winner.Hash)
	if err != nil || !reflect.DeepEqual(before, after) || h.observer.calls != calls {
		t.Fatal("reopen changed original round/observation", err)
	}
	if err = h.store.Close(); err != nil {
		t.Fatal(err)
	}
	audit, err := OpenDailyAudit(ctx, h.path, h.limits, h.source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = audit.Sync(ctx); !errors.Is(err, ErrInvalid) {
		t.Fatal("audit mutated", err)
	}
	if err = audit.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := leveldb.OpenFile(filepath.Join(h.path, "db"), &opt.Options{ErrorIfMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Put(indexKey(721), []byte(h.winner.Hash), &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if got, e := OpenDailyAudit(ctx, h.path, h.limits, h.source); got != nil || !errors.Is(e, ErrCorrupt) {
		t.Fatal("extra global index accepted", e)
	}
}

func TestDailyRoundProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal")
	}
	path := filepath.Join(t.TempDir(), "must-not-exist")
	for _, run := range []func() (*Store, error){
		func() (*Store, error) {
			return CreateDaily(context.Background(), path, Limits{2, 721, 8 << 30}, nil, nil)
		},
		func() (*Store, error) {
			return OpenDaily(context.Background(), path, Limits{2, 721, 8 << 30}, nil, nil)
		},
		func() (*Store, error) {
			return OpenDailyAudit(context.Background(), path, Limits{2, 721, 8 << 30}, nil)
		},
	} {
		if s, err := run(); s != nil || !errors.Is(err, ErrProfile) {
			t.Fatal(s, err)
		}
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("profile refusal touched storage", err)
	}
	var absent *Store
	if limits, source, err := absent.DailyBinding(context.Background()); !errors.Is(err, ErrProfile) || limits.Version != 0 || source.Version != 0 {
		t.Fatal("cheap binding disclosed a profile", limits, source, err)
	}
}
