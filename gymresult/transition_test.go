//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
	"strings"
	"testing"
)

func TestResultCompletedBodySurvivesLaterClose(t *testing.T) {
	f := newResultFixture(t)
	c, done := fixtureCall(t, f)
	closed := make(chan error, 1)
	f.s.checkpoint = func(stage string) {
		if stage == "guarded-body-complete" {
			go func() { closed <- f.s.Close() }()
			<-f.s.life.Done()
		}
	}
	_, st, e := f.s.read(c, gymaccount.View, false)
	done()
	if e != nil || st.Exists {
		t.Fatal("completed read replaced", e)
	}
	if e = <-closed; e != nil {
		t.Fatal(e)
	}
}

// This models only the already-verified chain-reader boundary of the private
// reconciliation algorithm. Public Reconcile still requires a concrete owned
// ResultPublisher. It asserts real source authorization, durable journal state,
// batch inventory changes and response coherence, not a PoW/network verdict.
type modelResultChain struct {
	tip    Tip
	hashes map[uint32]string
	blocks map[uint32]*wire.MsgBlock
	err    error
	reads  int
}

func (m *modelResultChain) Observe(context.Context) (Tip, error) { return m.tip, m.err }
func (m *modelResultChain) CanonicalHash(_ context.Context, h uint32) (string, error) {
	m.reads++
	return m.hashes[h], m.err
}
func (m *modelResultChain) Block(_ context.Context, h uint32) (*wire.MsgBlock, error) {
	return m.blocks[h], m.err
}
func preparedFixture(t *testing.T) (*resultFixture, sourceCall, func(), Record) {
	f := newResultFixture(t)
	c, done := fixtureCall(t, f)
	a, e := f.s.prepare(c)
	if e != nil {
		done()
		t.Fatal(e)
	}
	return f, c, done, a.Record
}
func modelTip(r Record) *modelResultChain {
	h := max(r.Statement.Statement.Source.Source.TargetHeight+1, 150)
	tip := Tip{Height: h, Hash: strings.Repeat("a", 64)}
	return &modelResultChain{tip: tip, hashes: map[uint32]string{h: tip.Hash, r.Statement.Statement.Source.Source.TargetHeight: r.Statement.Statement.Source.Source.TargetHash}, blocks: map[uint32]*wire.MsgBlock{}}
}
func TestResultReconcileReturnsCommittedFundingState(t *testing.T) {
	f, c, done, r := preparedFixture(t)
	defer done()
	m := modelTip(r)
	prev := strings.Repeat("b", 64)
	m.hashes[m.tip.Height-1] = prev
	next := f.s.meta
	next.FundingCursor.Height = int32(m.tip.Height) - 1
	next.FundingCursor.Hash = prev
	next.FundingCursor.ScanDigest = strings.Repeat("c", 64)
	next.LastFundingTip = &m.tip
	if e := f.s.commitLocked(new(leveldb.Batch), next); e != nil {
		t.Fatal(e)
	}
	ph, _ := chainhash.NewHashFromStr(prev)
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: *ph, Index: 1}, nil, nil))
	tx.AddTxOut(wire.NewTxOut(5000, rawID(f.pin.FundingScript)))
	m.blocks[m.tip.Height] = &wire.MsgBlock{Header: wire.BlockHeader{PrevBlock: *ph}, Transactions: []*wire.MsgTx{tx}}
	got, e := f.s.reconcile(c, m)
	if e != nil {
		t.Fatal(e)
	}
	if got.Status.FundingState != "available" || !f.s.meta.FundingComplete {
		t.Fatal("stale completed inventory", got.Status.FundingState)
	}
	_, fresh, e := f.s.read(c, gymaccount.View, true)
	if e != nil || fresh.FundingState != got.Status.FundingState {
		t.Fatal("read disagrees", e, fresh.FundingState)
	}
	// A canonical funding-cursor replacement resets the inventory in the same
	// transition and must return scanning immediately, preserving reservations.
	m.tip.Hash = strings.Repeat("d", 64)
	m.hashes[m.tip.Height] = m.tip.Hash
	got, e = f.s.reconcile(c, m)
	if e != nil {
		t.Fatal(e)
	}
	if got.Status.FundingState != "scanning" || f.s.meta.FundingComplete {
		t.Fatal("stale reset inventory", got.Status.FundingState)
	}
}
func TestResultFreshSavedTargetConflictIsDurable(t *testing.T) {
	f, c, done, r := preparedFixture(t)
	defer done()
	m := modelTip(r)
	if _, e := f.s.checkSavedTarget(c, r, m); e != nil {
		t.Fatal("matching target", e)
	}
	m.hashes[r.Statement.Statement.Source.Source.TargetHeight] = strings.Repeat("e", 64)
	if _, e := f.s.checkSavedTarget(c, r, m); !errors.Is(e, ErrSourceConflict) {
		t.Fatal("changed target", e)
	}
	row, ok, e := f.s.record(r.ResultKey)
	if e != nil || !ok || len(row.Conflicts) != 1 || row.Conflicts[0].Kind != "source_chain_conflict" || row.Publication != nil {
		t.Fatal("conflict lost or dispatched", e)
	}
	if _, e = f.s.checkSavedTarget(c, row, m); !errors.Is(e, ErrSourceConflict) {
		t.Fatal(e)
	}
	row, _, _ = f.s.record(r.ResultKey)
	if len(row.Conflicts) != 1 {
		t.Fatal("duplicate conflict")
	}
}
func TestResultTargetRPCFailureIsNotConflict(t *testing.T) {
	f, c, done, r := preparedFixture(t)
	defer done()
	m := modelTip(r)
	m.err = errors.New("test unavailable")
	if _, e := f.s.checkSavedTarget(c, r, m); e == nil {
		t.Fatal("accepted failed observation")
	}
	row, _, e := f.s.record(r.ResultKey)
	if e != nil || len(row.Conflicts) != 0 || row.Publication != nil {
		t.Fatal("failure became conflict", e)
	}
}
