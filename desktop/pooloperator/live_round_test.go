//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
)

// Real original pool/round/ledger and OperatorV2 issuer; authentication and node
// observations are controlled published fixtures. This does not manufacture a
// READY runtime, run listeners/scheduler, or claim actual wallet/node payment.
type opLiveNode struct {
	template  prospectwork.Template
	winners   map[string]prospectwork.ShareCheck
	canonical map[string]bool
	tip       uint32
	observed  []string
}

func (n *opLiveNode) ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error) {
	return n.template, nil
}
func (n *opLiveNode) Proposal(context.Context, prospect.Context, string) error { return nil }
func (n *opLiveNode) RecheckContext(ctx context.Context, c prospect.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if c.ParentHash != n.template.Context.ParentHash || c.ParentHeight != n.template.Context.ParentHeight {
		return errors.New("fixture parent changed")
	}
	return nil
}
func (n *opLiveNode) SubmitBlock(ctx context.Context, _ prospect.Context, raw string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	for _, w := range n.winners {
		if w.Candidate != nil && w.Candidate.BlockHex == raw {
			return nil
		}
	}
	return errors.New("unexpected fixture block")
}
func (n *opLiveNode) CanonicalBlock(ctx context.Context, _ prospect.Context, p prospect.Proof, hash string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	w, ok := n.winners[hash]
	if !ok || !n.canonical[hash] || w.Candidate == nil || !reflect.DeepEqual(w.Candidate.Proof, p) {
		return errors.New("not exact canonical fixture")
	}
	return nil
}
func (n *opLiveNode) ObservePoolBlock(ctx context.Context, c prospect.Context, p prospect.Proof, hash string) (prospectnode.PoolBlockObservation, error) {
	if e := ctx.Err(); e != nil {
		return prospectnode.PoolBlockObservation{}, e
	}
	w, ok := n.winners[hash]
	if !ok || w.Candidate == nil || !reflect.DeepEqual(w.Candidate.Proof, p) {
		return prospectnode.PoolBlockObservation{}, errors.New("not original winner")
	}
	n.observed = append(n.observed, hash)
	o := prospectnode.PoolBlockObservation{BlockHash: hash, BlockHeight: c.ParentHeight + 1, TipHash: strings.Repeat("a", 64), TipHeight: n.tip, Canonical: n.canonical[hash]}
	if o.Canonical {
		o.Descendants = o.TipHeight - o.BlockHeight
	}
	return o, nil
}

type opLiveSessions string

func (s opLiveSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	if token != "published-live-round-session" {
		return errors.New("fixture session refused")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	return fn(string(s), ctx.Err)
}

type opLiveFunding struct {
	t           *testing.T
	inspections int
}

func (b *opLiveFunding) Inspect(ctx context.Context, coins []poolsettle.Coin) ([]poolsettle.Funding, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	b.inspections++
	out := make([]poolsettle.Funding, len(coins))
	for i, c := range coins {
		out[i] = poolsettle.Funding{TxID: c.TxID, Index: c.Index, State: "immature"}
	}
	return out, nil
}
func (b *opLiveFunding) EstimateFee(context.Context, []poolsettle.Coin, []poolsettle.Payment, string) (int64, error) {
	b.t.Error("immature fixture requested fee")
	return 0, ErrUnavailable
}
func (b *opLiveFunding) Sign(context.Context, []poolsettle.Coin, []byte) ([]byte, error) {
	b.t.Error("maintenance granted signing")
	return nil, ErrUnavailable
}
func (b *opLiveFunding) ObserveTransaction(context.Context, string) (poolsettle.TransactionObservation, error) {
	b.t.Error("no pending payment expected")
	return poolsettle.TransactionObservation{}, ErrUnavailable
}
func (b *opLiveFunding) Broadcast(context.Context, []byte) (string, error) {
	b.t.Error("maintenance broadcast payment")
	return "", ErrUnavailable
}

func opLiveFixture(t *testing.T) (*Runtime, *opLiveNode, *opLiveFunding) {
	t.Helper()
	r, _ := opActionTestRuntime(t)
	raw, e := os.ReadFile("../../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("independent template changed")
	}
	var f struct {
		Sources []struct {
			Name     string
			Template prospectwork.Template
		}
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	n := &opLiveNode{winners: map[string]prospectwork.ShareCheck{}, canonical: map[string]bool{}}
	for _, v := range f.Sources {
		if v.Name == "coinbase-only" {
			n.template = v.Template
		}
	}
	if n.template.Context.Version == 0 {
		t.Fatal("missing original template")
	}
	path, fresh, e := r.files.child("pool")
	if e != nil || !fresh {
		t.Fatal(e)
	}
	r.pool, e = poolshare.Create(path, r.setup.Pool, r.issuer, n, opLiveSessions(r.setup.Miners.Enrollments[0].PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	if e = r.files.adopt("pool"); e != nil {
		t.Fatal(e)
	}
	path, fresh, e = r.files.child("rounds")
	if e != nil || !fresh {
		t.Fatal(e)
	}
	r.rounds, e = poolround.Create(path, r.setup.RoundLimits, r.pool, n)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.files.adopt("rounds"); e != nil {
		t.Fatal(e)
	}
	b := &opLiveFunding{t: t}
	path, fresh, e = r.files.child("settlement")
	if e != nil || !fresh {
		t.Fatal(e)
	}
	r.settlement, e = poolsettle.Create(path, r.setup.Settlement, r.rounds, b)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.files.adopt("settlement"); e != nil {
		t.Fatal(e)
	}
	r.round = roundMaintenance{store: r.rounds}
	return r, n, b
}

func opLiveWinner(t *testing.T, r *Runtime, n *opLiveNode) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	w, e := r.pool.IssueMiningWork(ctx, "published-live-round-session")
	if e != nil {
		t.Fatal(e)
	}
	p, e := prospectwork.PrepareManifestWork(w.Job, w.Template, r.setup.Pool.PayoutAddress, 0, r.setup.Pool.PoolTarget)
	if e != nil {
		t.Fatal(e)
	}
	var winner prospectwork.ShareCheck
	for nonce := uint32(0); nonce < 64; nonce++ {
		c, e := p.CheckNonce(ctx, nonce)
		if e != nil {
			t.Fatal(e)
		}
		if c.Block {
			winner = c
			break
		}
	}
	if winner.Candidate == nil {
		t.Fatal("bounded normal-PoW search exhausted")
	}
	n.winners[winner.WorkHash] = winner
	n.canonical[winner.WorkHash] = true
	n.tip = w.Job.Context.ParentHeight + 61
	receipt, e := r.pool.Submit(ctx, "published-live-round-session", w.Job.ID, winner.Nonce)
	if e != nil || receipt.AcceptedSequence == 0 || !receipt.Block {
		t.Fatal(receipt, e)
	}
	if receipt.GymID != r.setup.Miners.Enrollments[0].GymID {
		t.Fatal("source changed enrolled gym")
	}
	n.template.Context.ParentHash = winner.WorkHash
	n.template.Context.ParentHeight = w.Job.Context.ParentHeight + 1
	n.template.Block.PreviousHash = winner.WorkHash
	n.template.Block.Height = int64(n.template.Context.ParentHeight) + 1
	return winner.WorkHash
}
func opLiveBalance(t *testing.T, r *Runtime, winners int) {
	t.Helper()
	v, e := r.settlement.RunDaily(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if len(v.Balances) != len(r.setup.Settlement.Destinations) {
		t.Fatal("wrong original ledger balance count", v)
	}
	zero := poolsettle.Fraction{Numerator: "0", Denominator: "1"}
	seen := make(map[string]bool)
	for _, b := range v.Balances {
		configured := false
		for _, destination := range r.setup.Settlement.Destinations {
			configured = configured || destination.GymID == b.GymID
		}
		want := zero
		if b.GymID == r.setup.Miners.Enrollments[0].GymID {
			want.Numerator = strconv.FormatInt(int64(winners)*990000000, 10)
		}
		if !configured || seen[b.GymID] || b.Confirmed != want || b.ReservedAtoms != 0 || b.PaidAtoms != 0 || b.Available != zero {
			t.Fatal("original ledger failed exact enrolled balance/immature gate", b)
		}
		seen[b.GymID] = true
	}
}

func TestOperatorLiveRoundOriginalCatchupAndDefault(t *testing.T) {
	r, n, backend := opLiveFixture(t)
	ctx := context.Background()
	pool, rounds, ledger := r.pool, r.rounds, r.settlement
	setup := r.Setup()
	admission, cancel := context.WithCancel(ctx)
	if e := r.EnableLiveRoundMaintenance(admission); e != nil {
		t.Fatal(e)
	}
	cancel()
	original := r.round.live
	hashes := make([]string, 10)
	for i := range hashes {
		hashes[i] = opLiveWinner(t, r, n)
	}
	summary, e := r.round.step(ctx)
	if e != nil || summary != (poolround.Summary{Through: 10, Rounds: 10}) || r.round.cursor != 8 || len(n.observed) != 8 {
		t.Fatal("full Sync did not precede bounded live refresh", summary, r.round.cursor, e)
	}
	opLiveBalance(t, r, 8)
	if _, e = r.round.step(ctx); e != nil || r.round.cursor != 10 || len(n.observed) != 10 {
		t.Fatal("rotating tail not refreshed", e)
	}
	opLiveBalance(t, r, 10)
	before, e := rounds.List(ctx, 0, poolround.MaxPage)
	if e != nil {
		t.Fatal(e)
	}
	legacy := roundMaintenance{store: rounds}
	got, e := legacy.step(ctx)
	if e != nil || got != summary || legacy.live != nil || legacy.cursor != 8 {
		t.Fatal("default maintenance changed", got, e)
	}
	after, e := rounds.List(ctx, 0, poolround.MaxPage)
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("default and selected observations diverged", e)
	}
	if r.pool != pool || r.rounds != rounds || r.round.store != rounds || r.settlement != ledger || r.round.live != original || !reflect.DeepEqual(setup, r.Setup()) || backend.inspections == 0 {
		t.Fatal("selected maintenance replaced original authority/config")
	}
	// Private test corruption of the handle is a stale-selection control, not
	// a public setter or replacement Store. The reanchor uses the same owner.
	r.round.live = &poolround.Live{}
	if e = r.round.refresh(ctx); e != nil || r.round.live != original {
		t.Fatal("explicit same-owner reanchor", e)
	}
	first, e := rounds.Get(ctx, hashes[0])
	if e != nil || first.Confirmation == nil {
		t.Fatal(e)
	}
	n.canonical[hashes[0]] = false
	r.round.cursor = 0
	if e = r.round.refresh(ctx); e != nil {
		t.Fatal(e)
	}
	conflict, e := rounds.Get(ctx, hashes[0])
	if e != nil || conflict.Status() != "funding-conflict" || !reflect.DeepEqual(conflict.Round, first.Round) || !reflect.DeepEqual(conflict.Confirmation, first.Confirmation) {
		t.Fatal("old round lost retained confirmation/allocation", e)
	}
}

func TestOperatorLiveRoundCapacityRefresh(t *testing.T) {
	r, n, _ := opLiveFixture(t)
	ctx := context.Background()
	first := opLiveWinner(t, r, n)
	// A separately selected smaller round accountant exercises the unchanged
	// capacity branch without changing the real runtime's frozen Setup.
	s, e := poolround.Create(filepath.Join(t.TempDir(), "bounded-rounds"), poolround.Limits{Version: 1, MaxRounds: 1, MaxBytes: 8 << 20}, r.pool, n)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	m := roundMaintenance{store: s}
	m.live, e = s.BeginLive(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.step(ctx); e != nil {
		t.Fatal(e)
	}
	opLiveWinner(t, r, n)
	if _, e = m.step(ctx); e != poolround.ErrCapacity {
		t.Fatal("full Sync did not retain capacity gate", e)
	}
	n.canonical[first] = false
	if e = m.refresh(ctx); e != nil {
		t.Fatal("capacity blocked original retained refresh", e)
	}
	rows, e := s.List(ctx, 0, poolround.MaxPage)
	if e != nil || len(rows) != 1 || rows[0].Round.BlockHash != first || rows[0].Status() != "funding-conflict" {
		t.Fatal("capacity admitted new tail or erased old confirmation", e)
	}
}

func TestOperatorLiveRoundSelectionRefusals(t *testing.T) {
	r, _, _ := opLiveFixture(t)
	ctx := context.Background()
	if e := r.EnableLiveRoundMaintenance(ctx); e != nil {
		t.Fatal(e)
	}
	original := r.round.live
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if e := r.EnableLiveRoundMaintenance(cancelled); !errors.Is(e, context.Canceled) || r.round.live != original {
		t.Fatal("canceled selection changed original mode", e)
	}
	r.localGate <- struct{}{}
	e := r.EnableLiveRoundMaintenance(ctx)
	<-r.localGate
	if e != ErrActionBusy || r.round.live != original {
		t.Fatal("selection bypassed original local operation", e)
	}
	r.mu.Lock()
	r.selectingLiveRounds = true
	r.mu.Unlock()
	e = r.Run(ctx, nil)
	r.mu.Lock()
	r.selectingLiveRounds = false
	started := r.started
	r.mu.Unlock()
	if e != ErrActionBusy || started {
		t.Fatal("Run consumed during selection", e)
	}
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
	e = r.EnableLiveRoundMaintenance(ctx)
	r.mu.Lock()
	r.started = false
	r.mu.Unlock()
	if e != ErrUnavailable || r.round.live != original {
		t.Fatal("mode changed after first Run", e)
	}
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = original.List(ctx, 0); e == nil {
		t.Fatal("original closed owner handle revived")
	}
	if e = r.EnableLiveRoundMaintenance(ctx); e != ErrUnavailable {
		t.Fatal("closed runtime selected a replacement", e)
	}
}

func TestOperatorLiveRoundCloseJoinsSelection(t *testing.T) {
	r, _, _ := opLiveFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r.inputMu.Lock()
	locked := true
	defer func() {
		if locked {
			r.inputMu.Unlock()
		}
	}()
	selected := make(chan error, 1)
	go func() { selected <- r.EnableLiveRoundMaintenance(ctx) }()
	for len(r.localGate) == 0 {
		select {
		case <-ctx.Done():
			t.Fatal("selection was not admitted")
		case <-time.After(time.Millisecond):
		}
	}
	closed := make(chan error, 1)
	go func() { closed <- r.Close() }()
	select {
	case <-r.life.Done():
	case <-ctx.Done():
		t.Fatal("Close did not cancel original selection")
	}
	select {
	case e := <-closed:
		t.Fatal("Close released owners before selection joined", e)
	default:
	}
	if _, e := r.pool.AccountingPage(context.Background(), 0, 0, 1); e != nil {
		t.Fatal("source closed before admitted selection joined", e)
	}
	r.inputMu.Unlock()
	locked = false
	select {
	case e := <-selected:
		if e == nil {
			t.Fatal("cancelled selection published handle")
		}
	case <-ctx.Done():
		t.Fatal("selection did not join")
	}
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("Close did not join")
	}
	if r.round.live != nil || r.selectingLiveRounds {
		t.Fatal("failed selection retained a new mode")
	}
}
