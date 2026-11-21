//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// These fresh owners reuse only published keys, independent template inputs and
// the existing normal-PoW fixture leaves. Node observations and the signing
// backend are controlled: this is not the actual-node/schema16-wallet proof.
func dailyIntegrationFixture(t *testing.T) *lateWinnerOwners {
	t.Helper()
	h := &lateWinnerOwners{clock: time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)}
	t.Cleanup(func() {
		if e := h.close(); e != nil {
			t.Error(e)
		}
	})
	var e error
	h.root, e = filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(h.root, 0700); e != nil {
		t.Fatal(e)
	}
	read := func(path, hash string) []byte {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		got := sha256.Sum256(b)
		if hex.EncodeToString(got[:]) != hash {
			t.Fatal("independent fixture changed", path)
		}
		return b
	}
	var authority struct {
		Positives []struct {
			Family, Name string
			Value        json.RawMessage
		}
	}
	if e = json.Unmarshal(read("../registry/testdata/operator-reference/authority-v2.json", "7328ea2102c9ac993281bd732590c3e734eda26a3d180c2fa98bbd37da02c6b4"), &authority); e != nil {
		t.Fatal(e)
	}
	var pin registry.PublicManifestPin
	for _, c := range authority.Positives {
		if c.Family == "pin" && c.Name == "v1/operator_v2_distinct" {
			if e = json.Unmarshal(c.Value, &pin); e != nil {
				t.Fatal(e)
			}
		}
	}
	h.profile, e = registry.NewOperatorProfile(pin)
	if e != nil {
		t.Fatal(e)
	}
	genesis, e := chainhash.NewHashFromStr(pin.Source.Genesis)
	if e != nil {
		t.Fatal(e)
	}
	h.binding = operatorauthority.Binding{Version: 2, PinID: h.profile.ID(), Network: wire.BitcoinNet(pin.Source.Network), Genesis: *genesis, Keys: operatorauthority.Keys{Admission: pin.AdmissionKey, Publication: pin.PublicationKey, Cutoff: pin.CutoffKey, Checkpoint: pin.CheckpointKey}}
	paths := map[string]string{}
	for _, role := range []string{"admission", "publication", "cutoff"} {
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		paths[role] = filepath.Join(h.root, role)
		if e = os.WriteFile(paths[role], seed[:], 0600); e != nil {
			t.Fatal(e)
		}
	}
	h.seeds = operatorauthority.OnlineSeedFiles{Admission: paths["admission"], Publication: paths["publication"], Cutoff: paths["cutoff"]}
	h.owner, e = operatorauthority.OpenOnline(context.Background(), h.binding, h.seeds)
	if e != nil {
		t.Fatal(e)
	}
	d, e := fighterdescriptor.ResolveCandidate(pin.Source.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	h.issuer, e = registry.CreateOperatorManifestAuthority(filepath.Join(h.root, "issuer"), d, h.profile, h.owner)
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Sources []struct {
			Name     string
			Template prospectwork.Template
		}
	}
	if e = json.Unmarshal(read("../prospectwork/testdata/pool_share_reference/pool_share_v1.json", "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87"), &corpus); e != nil {
		t.Fatal(e)
	}
	h.node = &lateWinnerNode{winners: map[string]prospectwork.ShareCheck{}, canonical: map[string]bool{}}
	for _, s := range corpus.Sources {
		if s.Name == "coinbase-only" {
			h.node.template = s.Template
		}
	}
	c := h.node.template.Context
	if c.Version == 0 {
		t.Fatal("template absent")
	}
	ka, _, _ := settlementFixtureKey(1)
	defer ka.Zero()
	kb, _, _ := settlementFixtureKey(2)
	defer kb.Zero()
	h.sessions = lateWinnerSessions{"gym-a": hex.EncodeToString(ka.PubKey().SerializeCompressed()), "gym-b": hex.EncodeToString(kb.PubKey().SerializeCompressed())}
	_, poolScript, payout := settlementFixtureKey(3)
	h.poolConfig = poolshare.Config{Version: 2, AuthorityPin: pin.Source, Policy: prospectnode.Policy{Version: c.Version, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}, PayoutAddress: payout, PoolTarget: strings.Repeat("f", 64), MaxWorks: 4, MaxSubmissions: 4, MaxBytes: 8 << 20}
	h.roundLimits = poolround.Limits{Version: 2, MaxRounds: 4, MaxBytes: 8 << 20}
	ga, e := gymauth.GymID(c, h.sessions["gym-a"])
	if e != nil {
		t.Fatal(e)
	}
	gb, e := gymauth.GymID(c, h.sessions["gym-b"])
	if e != nil {
		t.Fatal(e)
	}
	_, payA, _ := settlementFixtureKey(4)
	_, payB, _ := settlementFixtureKey(5)
	h.settleConfig = Config{Version: 2, ChangeScript: poolScript, MaxRounds: 4, MaxBatches: 4, Destinations: []Destination{{GymID: ga, Script: payA}, {GymID: gb, Script: payB}}}
	if h.settleConfig.Destinations[0].GymID > h.settleConfig.Destinations[1].GymID {
		h.settleConfig.Destinations[0], h.settleConfig.Destinations[1] = h.settleConfig.Destinations[1], h.settleConfig.Destinations[0]
	}
	h.pool, e = poolshare.CreateDaily(context.Background(), filepath.Join(h.root, "pool"), h.poolConfig, h.issuer, h.node, h.sessions)
	if e != nil {
		t.Fatal(e)
	}
	h.rounds, e = poolround.CreateDaily(context.Background(), filepath.Join(h.root, "rounds"), h.roundLimits, h.pool, h.node)
	if e != nil {
		t.Fatal(e)
	}
	h.backend = &recoveryBackend{Backend: &settlementBackend{funding: "ready", fee: 1000, txs: map[string][]byte{}}}
	h.settlement, e = CreateDaily(context.Background(), filepath.Join(h.root, "settlement"), h.settleConfig, h.rounds, h.backend)
	if e != nil {
		t.Fatal(e)
	}
	h.settlement.now = func() time.Time { return h.clock }
	// Source/round/ledger were born V2; no legacy owner or migration is used.
	return h
}

func dailyIntegrationReopen(t *testing.T, h *lateWinnerOwners) {
	t.Helper()
	ctx := context.Background()
	oldPool, oldStore := h.pool, h.settlement
	if err := h.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := oldStore.RecoverPending(ctx); err != ErrClosed {
		t.Fatal("old ledger owner survived", err)
	}
	if _, err := oldPool.Issue(ctx, "gym-a"); err != poolshare.ErrClosed {
		t.Fatal("old source owner survived", err)
	}
	var err error
	h.owner, err = operatorauthority.OpenOnline(ctx, h.binding, h.seeds)
	if err != nil {
		t.Fatal(err)
	}
	h.issuer, err = registry.OpenOperatorManifestAuthority(filepath.Join(h.root, "issuer"), h.profile, h.owner)
	if err != nil {
		t.Fatal(err)
	}
	// Even a caller changing the version field cannot adopt this V2 source via
	// legacy Open. The root is closed, so this cannot pass through a DB lock error.
	oldConfig := h.poolConfig
	oldConfig.Version = 1
	if unexpected, e := poolshare.Open(filepath.Join(h.root, "pool"), oldConfig, h.issuer, h.node, h.sessions); e == nil || unexpected != nil {
		if unexpected != nil {
			_ = unexpected.Close()
		}
		t.Fatal("legacy source adopted V2 root", e)
	}
	h.pool, err = poolshare.OpenDaily(ctx, filepath.Join(h.root, "pool"), h.poolConfig, h.issuer, h.node, h.sessions)
	if err != nil {
		t.Fatal(err)
	}
	oldLimits := h.roundLimits
	oldLimits.Version = 1
	if unexpected, e := poolround.Open(filepath.Join(h.root, "rounds"), oldLimits, h.pool, h.node); e == nil || unexpected != nil {
		if unexpected != nil {
			_ = unexpected.Close()
		}
		t.Fatal("legacy rounds adopted V2 source", e)
	}
	h.rounds, err = poolround.OpenDaily(ctx, filepath.Join(h.root, "rounds"), h.roundLimits, h.pool, h.node)
	if err != nil {
		t.Fatal(err)
	}
	oldSettle := h.settleConfig
	oldSettle.Version = 1
	if unexpected, e := Open(filepath.Join(h.root, "settlement"), oldSettle, h.rounds, h.backend); e == nil || unexpected != nil {
		if unexpected != nil {
			_ = unexpected.Close()
		}
		t.Fatal("legacy settlement adopted V2 source", e)
	}
	h.settlement, err = OpenDaily(ctx, filepath.Join(h.root, "settlement"), h.settleConfig, h.rounds, h.backend)
	if err != nil {
		t.Fatal(err)
	}
	h.settlement.now = func() time.Time { return h.clock }
}

func dailyIntegrationView(t *testing.T, h *lateWinnerOwners, credits, batches, pending uint64) *dailyView {
	t.Helper()
	v, err := h.settlement.auditDaily(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.head.Credits != credits || v.head.Batches != batches || v.head.PendingBatch != pending {
		t.Fatal("daily original counts", v.head)
	}
	if len(h.settlement.state.Credits) != 0 || len(h.settlement.state.Batches) != 0 {
		t.Fatal("indexed owner materialized all legacy history")
	}
	return v
}
func dailyIntegrationRaw(t *testing.T, s *Store, key []byte) []byte {
	t.Helper()
	b, e := s.db.Get(key, nil)
	if e != nil {
		t.Fatal(e)
	}
	return append([]byte(nil), b...)
}
func dailyIntegrationBalance(t *testing.T, r Report, gym string, unpaid, available string, reserved, paid int64) {
	t.Helper()
	for _, b := range r.Balances {
		if b.GymID == gym {
			if b.Confirmed != (Fraction{unpaid, "1"}) || b.Available != (Fraction{available, "1"}) || b.ReservedAtoms != reserved || b.PaidAtoms != paid {
				t.Fatal("exact independent 99/1 balance", b)
			}
			return
		}
	}
	t.Fatal("missing original gym balance", gym)
}

func TestDailyOperatorSourcePendingRecoveryAndNextDay(t *testing.T) {
	h := dailyIntegrationFixture(t)
	ctx := context.Background()
	a, e := h.pool.Issue(ctx, "gym-a")
	if e != nil {
		t.Fatal(e)
	}
	wa, _ := lateWinnerSearch(t, h, a, false)
	h.node.canonical[wa.WorkHash] = true
	h.node.tip = a.Job.Context.ParentHeight + 61
	ra, e := h.pool.Submit(ctx, "gym-a", a.Job.ID, wa.Nonce)
	if e != nil || ra.Sequence != 1 || ra.AcceptedSequence != 1 {
		t.Fatal(ra, e)
	}
	if _, e = h.rounds.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	if r, e := h.rounds.Refresh(ctx, wa.WorkHash); e != nil || r.Confirmation == nil {
		t.Fatal(r, e)
	}
	base := h.backend.Backend.(*settlementBackend)
	// Observe the original sync writes, then inspect their bytes before the
	// backend can transmit. No mock ledger, captured DTO or replacement DB.
	write := h.settlement.writeBatch
	synced := 0
	h.settlement.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
		if o == nil || !o.Sync {
			t.Fatal("daily mutation was not synced")
		}
		e := write(b, o)
		if e == nil {
			synced++
		}
		return e
	}
	beforeSends := 0
	base.beforeBroadcast = func() {
		beforeSends++
		var head dailyHeader
		var body dailyBatchBody
		var delivery dailyDelivery
		if synced == 0 || dailyDecode(dailyIntegrationRaw(t, h.settlement, []byte("head")), dailyHeadLimit, &head) != nil || head.PendingBatch != 1 || head.Batches != 1 {
			t.Fatal("broadcast preceded durable pending header")
		}
		if dailyDecode(dailyIntegrationRaw(t, h.settlement, dailyNumber("b/", 1)), dailyBatchLimit, &body) != nil || dailyDecode(dailyIntegrationRaw(t, h.settlement, dailyNumber("bs/", 1)), dailyReferenceLimit, &delivery) != nil || delivery.State != "prepared" {
			t.Fatal("broadcast preceded exact prepared body")
		}
		if head.Credits != 1 || body.TxID == "" || body.Raw == "" || body.Sequence != 1 {
			t.Fatal("incomplete prepared batch")
		}
	}
	base.loseAck = true
	r, e := h.settlement.RunDaily(ctx)
	if e != nil || r.Reason != "broadcast-unknown" || r.State != "pending" || beforeSends != 1 {
		t.Fatal(r, e, beforeSends)
	}
	base.beforeBroadcast = nil
	v := dailyIntegrationView(t, h, 1, 1, 1)
	if v.pending == nil {
		t.Fatal("missing original pending A")
	}
	original := clone(*v.pending)
	originalBody := dailyIntegrationRaw(t, h.settlement, dailyNumber("b/", 1))
	if len(base.broadcastBytes) != 1 || hex.EncodeToString(base.broadcastBytes[0]) != original.Raw {
		t.Fatal("broadcast not exact durable A")
	}
	if original.FeeAtoms != 1000 || original.Change == nil || original.Change.Atoms != 9999000 || len(original.Inputs) != 1 || len(original.Payments) != 1 || original.Payments[0].Atoms != 990000000 || original.Payments[0].GymID != a.Job.GymID {
		t.Fatal("A independent arithmetic", original)
	}
	dailyIntegrationBalance(t, r, a.Job.GymID, "990000000", "0", 990000000, 0)
	assertA := func(pending bool) {
		t.Helper()
		if !bytes.Equal(originalBody, dailyIntegrationRaw(t, h.settlement, dailyNumber("b/", 1))) {
			t.Fatal("immutable A body changed")
		}
		if h.backend.signs != 1 || h.backend.quotes != 1 || base.signCalls != 1 {
			t.Fatal("recovery signed or quoted replacement")
		}
		if pending {
			v, e := h.settlement.auditDaily(ctx)
			if e != nil || v.pending == nil {
				t.Fatal("pending A audit", e)
			}
			got := clone(*v.pending)
			got.State = original.State
			if !reflect.DeepEqual(got, original) {
				t.Fatal("A inputs/claims/payment/fee/raw changed")
			}
		}
	}
	// A second original work creates a distinct reward while A remains reserved.
	h.node.template.Context.ParentHash = strings.Repeat("a", 64)
	h.node.template.Context.ParentHeight = h.node.tip
	h.node.template.Block.PreviousHash = h.node.template.Context.ParentHash
	h.node.template.Block.Height = int64(h.node.tip) + 1
	b, e := h.pool.Issue(ctx, "gym-b")
	if e != nil {
		t.Fatal(e)
	}
	wb, _ := lateWinnerSearch(t, h, b, false)
	h.node.canonical[wb.WorkHash] = true
	h.node.tip = b.Job.Context.ParentHeight + 61
	rb, e := h.pool.Submit(ctx, "gym-b", b.Job.ID, wb.Nonce)
	if e != nil || rb.Sequence != 2 || rb.AcceptedSequence != 2 {
		t.Fatal(rb, e)
	}
	if _, e = h.rounds.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	if row, e := h.rounds.Refresh(ctx, wb.WorkHash); e != nil || row.Confirmation == nil {
		t.Fatal(row, e)
	}
	dailyIntegrationView(t, h, 1, 1, 1)
	assertA(true)
	if r, e = h.settlement.RunDaily(ctx); e != nil || r.State != "pending" {
		t.Fatal(r, e)
	}
	dailyIntegrationView(t, h, 1, 1, 1)
	assertA(true)
	dailyIntegrationReopen(t, h)
	dailyIntegrationView(t, h, 1, 1, 1)
	assertA(true)
	// Exact saved observation recovers the lost acknowledgement without import,
	// another quote/signature, or another broadcast.
	if r, e = h.settlement.RecoverPending(ctx); e != nil || r.State != "pending" {
		t.Fatal(r, e)
	}
	dailyIntegrationView(t, h, 1, 1, 1)
	assertA(true)
	if base.broadcasts != 1 {
		t.Fatal("lost acknowledgement blindly resent")
	}
	base.confirmations = 1
	if r, e = h.settlement.RecoverPending(ctx); e != nil || r.State != "complete" || r.Reason != "" {
		t.Fatal(r, e)
	}
	dailyIntegrationView(t, h, 1, 1, 0)
	assertA(false)
	dailyIntegrationBalance(t, r, a.Job.GymID, "0", "0", 0, 990000000)
	// Recovery never imports B. Ordinary same-day work imports exactly once but
	// the original daily slot prohibits preparing a second transaction.
	if r, e = h.settlement.RunDaily(ctx); e != nil || r.Reason != "daily-batch-complete" {
		t.Fatal(r, e)
	}
	dailyIntegrationView(t, h, 2, 1, 0)
	assertA(false)
	dailyIntegrationBalance(t, r, a.Job.GymID, "0", "0", 0, 990000000)
	dailyIntegrationBalance(t, r, b.Job.GymID, "990000000", "990000000", 0, 0)
	var credit Credit
	if dailyDecode(dailyIntegrationRaw(t, h.settlement, dailyNumber("c/", 2)), dailyCreditLimit, &credit) != nil || credit.RoundHash != wb.WorkHash || credit.Coin.TxID == original.Inputs[0].TxID {
		t.Fatal("B credit source identity", credit)
	}
	wantClaims := map[string]Fraction{"": {"10000000", "1"}, b.Job.GymID: {"990000000", "1"}}
	if len(credit.Coin.Claims) != len(wantClaims) {
		t.Fatal("B allocation count")
	}
	for _, claim := range credit.Coin.Claims {
		if claim.RoundHash != wb.WorkHash || claim.Amount != wantClaims[claim.GymID] {
			t.Fatal("B independent 99/1 allocation", claim)
		}
		delete(wantClaims, claim.GymID)
	}
	if len(wantClaims) != 0 {
		t.Fatal("B allocation omitted")
	}
	if r, e = h.settlement.RunDaily(ctx); e != nil || r.Reason != "daily-batch-complete" {
		t.Fatal(r, e)
	}
	dailyIntegrationView(t, h, 2, 1, 0)
	assertA(false)
	h.clock = h.clock.Add(24 * time.Hour)
	if r, e = h.settlement.RunDaily(ctx); e != nil || r.State != "pending" {
		t.Fatal(r, e)
	}
	v = dailyIntegrationView(t, h, 2, 2, 2)
	if v.pending == nil {
		t.Fatal("missing next-day B")
	}
	next := clone(*v.pending)
	if next.Day <= original.Day || len(next.Inputs) != 2 || next.Inputs[0].TxID != original.TxID || len(next.Payments) != 1 || next.Payments[0].GymID != b.Job.GymID || next.Payments[0].Atoms != 990000000 || next.FeeAtoms != 1000 || next.Change == nil || next.Change.Atoms != 19998000 || h.backend.signs != 2 || h.backend.quotes != 2 {
		t.Fatal("B independent carry/payment/fee", next)
	}
	if !bytes.Equal(originalBody, dailyIntegrationRaw(t, h.settlement, dailyNumber("b/", 1))) {
		t.Fatal("next-day batch rewrote A")
	}
	if len(next.Change.Claims) != 2 {
		t.Fatal("operator carry lost original origins")
	}
	origins := map[string]bool{}
	for _, claim := range next.Change.Claims {
		if claim.GymID != "" || claim.Amount.Denominator != "1" {
			t.Fatal("operator carry contains miner principal", claim)
		}
		origins[claim.RoundHash] = true
	}
	if !origins[wa.WorkHash] || !origins[wb.WorkHash] {
		t.Fatal("carry origins changed")
	}
	dailyIntegrationBalance(t, r, a.Job.GymID, "0", "0", 0, 990000000)
	dailyIntegrationBalance(t, r, b.Job.GymID, "990000000", "0", 990000000, 0)
	if r, e = h.settlement.RecoverPending(ctx); e != nil || r.State != "complete" {
		t.Fatal(r, e)
	}
	dailyIntegrationView(t, h, 2, 2, 0)
	dailyIntegrationBalance(t, r, a.Job.GymID, "0", "0", 0, 990000000)
	dailyIntegrationBalance(t, r, b.Job.GymID, "0", "0", 0, 990000000)
	if base.broadcasts != 2 || h.backend.signs != 2 || !bytes.Equal(originalBody, dailyIntegrationRaw(t, h.settlement, dailyNumber("b/", 1))) {
		t.Fatal("duplicate payment or changed A")
	}
	if info, e := h.issuer.Info(); e != nil || info.Records != 0 {
		t.Fatal("pool settlement created a fighter", info, e)
	}
}
