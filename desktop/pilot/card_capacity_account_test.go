//go:build slugtestclock && !slugtestpow && !race

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// This 500ms metadata performance gate runs only in the normal profile, without
// race instrumentation. Core index/cancellation/lease correctness has separate
// focused race coverage; the normal test is never skipped.
//
// Only these 256 retained histories/finality values are modeled. Authorization
// comes from actual enrolled public-fixture accounts and Store.WithSession; no
// lease callback manufactures or replaces the account returned by the authority.
// The scoped clock advances with monotonic elapsed time after construction, so
// account/entry/event/seed/card observations perform their normal durable work.

func cardRealAccountEntryFixture(t *testing.T, root string, clock func() time.Time) (*StableController, gymevent.Pin, func()) {
	t.Helper()
	ctx := t.Context()
	config := gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:card-capacity", Subject: "zero", Fixture: "zero256"}, {Issuer: "fixture:card-capacity", Subject: "incrementing", Fixture: "incrementing256"}}, MaxSessions: 128, MaxChallenges: 64, MaxEvents: 256}
	accounts, e := gymaccount.CreateFixtureForTestClock(filepath.Join(root, "accounts"), config, clock)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := accounts.Close(); e != nil {
			t.Error(e)
		}
	})
	sessions := map[string]gymaccount.Session{}
	for _, spec := range []struct{ name, subject, key string }{{"zero256", "zero", "9d3035e7440dd3737fe58ffb69ca3576257d424b6e6172b3199a66a8897bddf6"}, {"incrementing256", "incrementing", "948996ecaa9fa9707693267d6d4abefe0b5fb28b6ebae34f542e106244d254e1"}} {
		login, e := accounts.Login(ctx, gymaccount.Principal{Issuer: "fixture:card-capacity", Subject: spec.subject, AuthenticatedAtMS: clock().UnixMilli()})
		if e != nil {
			t.Fatal(e)
		}
		challenge, e := accounts.BeginChallenge(ctx, login.Token, "enroll")
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := hex.DecodeString(spec.key)
		key, _ := btcec.PrivKeyFromBytes(raw)
		clear(raw)
		auth, e := gymaccount.SignChallenge(challenge, key)
		key.Zero()
		if e != nil {
			t.Fatal(e)
		}
		if _, inserted, e := accounts.CompleteChallenge(ctx, login.Token, challenge.ChallengeID, auth); e != nil || !inserted {
			t.Fatal("fixture enrollment", e)
		}
		sessions[spec.name] = login
	}
	ep := eventFixturePin(t)
	for i := range ep.Participants {
		p := &ep.Participants[i]
		name := "incrementing256"
		zero, _ := gymaccount.FixtureIdentity("zero256")
		if p.StablePin.Identity == zero {
			name = "zero256"
		}
		p.StablePin.AccountAuthorityID = accounts.AuthorityID()
		p.StablePin.AccountID = sessions[name].Account.ID
		p.StableStoreID, e = gymstable.ID(p.StablePin)
		if e != nil {
			t.Fatal(e)
		}
		p.NamespaceID, e = gymentry.NamespaceID(p.StablePin.Identity.Network, p.StablePin.Identity.Genesis, accounts.AuthorityID())
		if e != nil {
			t.Fatal(e)
		}
	}
	ep, e = gymevent.ValidatePin(ep)
	if e != nil {
		t.Fatal(e)
	}
	var pin gymstable.Pin
	zero, _ := gymaccount.FixtureIdentity("zero256")
	for _, p := range ep.Participants {
		if p.StablePin.Identity == zero {
			pin = p.StablePin
		}
	}
	var corpus struct {
		Cases []struct {
			Fixture string
			Record  registry.ManifestRecord
		}
	}
	raw, e := os.ReadFile("../../gymstable/testdata/reference/stable_v1.json")
	if e != nil || json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("source fixture", e)
	}
	records := []registry.ManifestRecord{}
	for _, v := range corpus.Cases {
		if v.Fixture == "zero256" {
			records = append(records, v.Record)
		}
	}
	data := filepath.Join(root, "gym")
	if e = os.Mkdir(data, 0700); e != nil {
		t.Fatal(e)
	}
	sourcePath := filepath.Join(data, "manifest-authority")
	sourceRows(t, sourcePath, pin, records)
	source, e := registry.OpenManifestAuthority(sourcePath, pin.Source, true)
	if e != nil {
		t.Fatal(e)
	}
	st, e := gymstable.CreateForTestClock(filepath.Join(data, "stable"), pin, source, clock)
	if e != nil {
		t.Fatal(e)
	}
	login := sessions["zero256"]
	lease := func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		// Match the actual parent withCapability durable-check cost: one
		// original-session recheck before the child callback and one after.
		// Parent epoch bookkeeping is not synthesized here; the real account
		// identity and generation are passed through untouched.
		return accounts.WithSession(ctx, login.Token, cap, func(account gymaccount.Account, recheck func() error) error {
			if err := recheck(); err != nil {
				return err
			}
			if err := fn(account, recheck); err != nil {
				return err
			}
			return recheck()
		})
	}
	a := manifestTestAccount()
	a.Account.AccountID = pin.AccountID
	a.Account.AuthorityID = pin.AccountAuthorityID
	a.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return lease(ctx, gymaccount.View, fn)
	}
	base := &ManifestController{config: Config{DataDir: data}, account: &a.Account, view: a, cache: map[uint64]*manifestCached{}, snapshot: BlankManifestSnapshot(), manifest: manifestIdentity{Version: 1, Mode: ManifestMode, Phase: "ready", AccountAuthorityID: pin.AccountAuthorityID, AccountID: pin.AccountID, Fixture: "zero256", Network: pin.Source.Network, Genesis: pin.Source.Genesis, ActivationHeight: 62, RecoveryProfile: pin.Identity.Profile, DescriptorID: pin.Source.DescriptorID, Payout: walletPayout{Address: "aiEhKC3LRX8PPG5haqRFUcFvj3pXpBts2P", Purpose: 44}, AuthorityPort: 37619, AuthorityPin: pin.Source, Identity: walletIdentity{RootPublicKey: pin.Identity.RootPublicKey, GymPublicKey: pin.Identity.GymPublicKey, GymID: pin.Identity.GymID}}}
	base.manifest.TransportTrustID, e = prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: base.manifest.origin(), Token: strings.Repeat("a", 64), AuthorityPin: pin.Source})
	if e != nil {
		t.Fatal(e)
	}
	s := &StableController{ManifestController: base, stableConfig: StableAccountConfig{Manifest: a, Lease: lease}, laneGate: make(chan struct{}, 1), wake: make(chan struct{}, 1), stableStore: st, source: source, ownedSource: true, installed: true, mining: true, stableState: "syncing", needSync: true}
	base.stable = s
	status, e := st.Status(ctx, s.session())
	if e != nil {
		t.Fatal(e)
	}
	s.stableStatus = &status
	t.Cleanup(func() {
		s.stopStoppedLoop()
		s.laneGate <- struct{}{}
		if e := s.closePair(); e != nil {
			t.Error(e)
		}
		<-s.laneGate
	})
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	p, e := s.EntryPin()
	if e != nil {
		t.Fatal(e)
	}
	entries, e := gymentry.CreateForTestClock(ctx, filepath.Join(data, "entries"), p, st, clock)
	if e != nil {
		t.Fatal(e)
	}
	s.entryEnabled = true
	s.entryStore = entries
	s.entryPin = p
	s.entryStoreID, _ = gymentry.ID(p)
	refresh := func() {
		var e error
		login, e = accounts.Login(ctx, gymaccount.Principal{Issuer: "fixture:card-capacity", Subject: "zero", AuthenticatedAtMS: clock().UnixMilli()})
		if e != nil {
			t.Fatal("fresh scoped session", e)
		}
	}
	return s, ep, refresh
}

func cardRealAccountCapacityFixture(t *testing.T) (*StableController, []string) {
	t.Helper()
	ctx := t.Context()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UnixMilli()
	var advanceAt time.Time
	clock := func() time.Time {
		if advanceAt.IsZero() {
			return time.UnixMilli(now)
		}
		return time.UnixMilli(now).Add(time.Since(advanceAt))
	}
	s, ep, refresh := cardRealAccountEntryFixture(t, root, clock)
	events, e := gymevent.CreateForTestClock(ctx, filepath.Join(root, "events"), ep, clock)
	if e != nil {
		t.Fatal(e)
	}
	attachEventFixture(t, s, events)
	sp, e := gymseed.CandidatePin(ep)
	if e != nil {
		t.Fatal(e)
	}
	seedPath, cardPath := filepath.Join(root, "seeds"), filepath.Join(root, "cards")
	seeds, e := gymseed.CreateForTestClock(ctx, seedPath, sp, events, clock)
	if e != nil {
		t.Fatal(e)
	}
	si, e := seeds.Info(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p, e := gymcard.CandidatePin(sp)
	if e != nil {
		t.Fatal(e)
	}
	cards, e := gymcard.CreateForTestClock(ctx, cardPath, p, seeds, clock)
	if e != nil {
		t.Fatal(e)
	}
	ci, e := cards.Info(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = cards.Close(); e != nil {
		t.Fatal(e)
	}
	if e = seeds.Close(); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../../gymevent/testdata/reference/event_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a" {
		t.Fatal("frozen old source fixture changed")
	}
	var old struct {
		Records []struct{ Value gymevent.SnapshotReceipt }
	}
	if json.Unmarshal(raw, &old) != nil {
		t.Fatal("template")
	}
	template := old.Records[10].Value
	for i := range template.Input.Participants {
		part := &template.Input.Participants[i]
		p := ep.Participants[i]
		sid, err := gymentry.ID(p)
		if err != nil {
			t.Fatal(err)
		}
		part.Owner.AccountAuthorityID = p.StablePin.AccountAuthorityID
		part.Owner.AccountID = p.StablePin.AccountID
		part.EntryStoreID = sid
		part.Entry.StoreID = sid
		part.Entry.LatestReceipt.StoreID = sid
		part.Roster.StoreID = p.StableStoreID
		template.Captures[i].StableFence.StoreID = p.StableStoreID
		template.Captures[i].EntryFence.StoreID = sid
	}
	second := &template.Input.Participants[1]
	rename := map[string]string{}
	for i := range second.Members {
		oldID := second.Members[i].Assignment.ProspectID
		id := cardCapacityHash("capacity-only distinct prospect", cardCapacityRawID(second.Owner.GymID), cardCapacityRawID(oldID))
		rename[oldID] = id
		second.Members[i].Assignment.ProspectID = id
	}
	for i := range second.Entry.LatestReceipt.Members {
		a := &second.Entry.LatestReceipt.Members[i].Assignment
		a.ProspectID = rename[a.ProspectID]
	}
	for i, id := range second.Roster.Slots {
		if id != "" {
			second.Roster.Slots[i] = rename[id]
		}
	}
	sort.Slice(second.Members, func(i, j int) bool {
		return second.Members[i].Assignment.ProspectID < second.Members[j].Assignment.ProspectID
	})
	sort.Slice(second.Entry.LatestReceipt.Members, func(i, j int) bool {
		return second.Entry.LatestReceipt.Members[i].Assignment.ProspectID < second.Entry.LatestReceipt.Members[j].Assignment.ProspectID
	})
	seedBatch, cardBatch := new(leveldb.Batch), new(leveldb.Batch)
	ids := []string{}
	date, _ := time.Parse("2006-01-02", "2026-11-21")
	for i := 0; i < 256; i++ {
		r := cardCapacitySeedRecord(t, p, template, date.AddDate(0, 0, 7*i).Format("2006-01-02"))
		source := gymseed.CardSource{Version: 1, StoreID: r.StoreID, EventKey: r.EventKey, SourceReceipt: r.SourceReceipt, Declaration: r.Declaration, Anchor: *r.Anchor, Target: *r.Target, Finality: *r.Finality}
		plan, e := gymcard.DerivePlan(p, source)
		if e != nil {
			t.Fatal(e)
		}
		sd, e := gymseed.CardSourceDigest(sp, source)
		if e != nil {
			t.Fatal(e)
		}
		record := gymcard.Record{Version: 1, StoreID: ci.StoreID, CardID: plan.CardID, Source: source, SourceDigest: sd, Plan: plan, CreatedAtMS: max(ci.CreatedAtMS, r.Finality.ObservedAtMS)}
		b, e := gymseed.MarshalRecord(sp, r)
		if e != nil {
			t.Fatal(e)
		}
		seedBatch.Put(append([]byte{'r'}, cardCapacityRawID(r.EventKey)...), b)
		b, e = gymcard.MarshalRecord(p, record)
		if e != nil {
			t.Fatal(e)
		}
		cardBatch.Put(append([]byte{'r'}, cardCapacityRawID(record.CardID)...), b)
		ids = append(ids, record.CardID)
		now = record.CreatedAtMS
	}
	for _, entry := range []struct {
		path  string
		batch *leveldb.Batch
	}{{seedPath, seedBatch}, {cardPath, cardBatch}} {
		db, e := leveldb.OpenFile(filepath.Join(entry.path, "db"), nil)
		if e != nil {
			t.Fatal(e)
		}
		meta, e := db.Get([]byte("meta"), nil)
		if e != nil {
			t.Fatal(e)
		}
		var old map[string]json.RawMessage
		if json.Unmarshal(meta, &old) != nil {
			t.Fatal("meta")
		}
		for k, v := range map[string]any{"clock_ms": now, "event_count": 256} {
			n, _ := json.Marshal(v)
			needle := append([]byte(`"`+k+`":`), old[k]...)
			if bytes.Count(meta, needle) != 1 {
				t.Fatal("bounded meta patch")
			}
			meta = bytes.Replace(meta, needle, append([]byte(`"`+k+`":`), n...), 1)
		}
		entry.batch.Put([]byte("meta"), meta)
		if e = db.Write(entry.batch, &opt.WriteOptions{Sync: true}); e != nil {
			t.Fatal(e)
		}
		if e = db.Close(); e != nil {
			t.Fatal(e)
		}
	}
	advanceAt = time.Now()
	refresh() // A fresh original session at the retained fixture time, before timing begins.
	seeds, e = gymseed.OpenForTestClock(ctx, seedPath, sp, events, clock)
	if e != nil {
		t.Fatal("seed full audit", e)
	}
	cards, e = gymcard.OpenForTestClock(ctx, cardPath, p, seeds, clock)
	if e != nil {
		t.Fatal("card full audit", e)
	}
	if e = s.entryStore.Close(); e != nil {
		t.Fatal(e)
	}
	s.entryStore, e = gymentry.OpenForTestClock(ctx, filepath.Join(s.config.DataDir, "entries"), s.entryPin, s.stableStore, clock)
	if e != nil {
		t.Fatal(e)
	}
	s.seedEnabled, s.seedAttached, s.cardEnabled, s.cardAttached = true, true, true, true
	s.seedStore, s.cardStore = seeds, cards
	s.seedPin, s.cardPin = sp, p
	s.seedID, s.cardID = si.StoreID, ci.StoreID
	s.seedCreated, s.cardCreated, s.cardSourceCreated = si.CreatedAtMS, ci.CreatedAtMS, ci.SourceCreatedAtMS
	s.seedBytes, s.cardBytes = MaxSeedMetadataSize, MaxCardMetadataSize
	t.Cleanup(func() { _ = cards.Close(); _ = seeds.Close() })
	return s, ids
}

func TestCardHelperHistoryCapacityRealAccountAdvancingClock(t *testing.T) {
	s, ids := cardRealAccountCapacityFixture(t)
	v := s.view.ViewID
	for _, start := range []int{0, 248, 256} {
		after := ""
		if start > 0 {
			after = ids[start-1]
		}
		begin := time.Now()
		page, e := s.CardHistory(t.Context(), v, after, 8)
		elapsed := time.Since(begin)
		if e != nil {
			t.Fatalf("real-account advancing-clock modeled256 page%d %s: %v", start, elapsed, e)
		}
		want := min(8, 256-start)
		if len(page.Cards) != want || page.HasMore != (start+want < 256) {
			t.Fatal("page", start, len(page.Cards))
		}
		t.Logf("real-account advancing-clock modeled256 page%d count%d %s", start, want, elapsed)
	}
}
