//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"strings"
	"testing"
)

func TestResultConcurrentSelectionsReserveOneCoinPermanently(t *testing.T) {
	f := newResultFixture(t)
	second := addModeledSecondBooking(t, f)
	selections := []sourceCall{f.call(), second}
	var records [2]Record
	var baselines [2][64]byte
	checkKeys := func(stage string) {
		t.Helper()
		for i := range records {
			if records[i].ResultKey != "" && records[i].ResultKey != string(baselines[i][:]) {
				t.Fatalf("returned key changed at %s selection%d: got=%q baseline=%q indexed=%+v", stage, i, records[i].ResultKey, baselines[i], f.s.indexed(string(baselines[i][:])))
			}
		}
	}
	for i, call := range selections {
		c, done, e := f.s.call(call)
		if e != nil {
			t.Fatal(e)
		}
		out, e := f.s.prepare(c)
		done()
		if e != nil {
			t.Fatal(e)
		}
		records[i] = out.Record
		copy(baselines[i][:], out.Record.ResultKey)
		checkKeys("prepare")
	}
	m := fundedPublisher(t, f, records[0])
	m.dynamicCarrier = true
	checkKeys("fundedPublisher")
	for _, r := range records {
		src := r.Statement.Statement.Source.Source
		m.hashes[src.TargetHeight] = src.TargetHash
	}
	start := make(chan struct{})
	type answer struct {
		index int
		out   ActionResult
		err   error
	}
	answers := make(chan answer, 2)
	for i, call := range selections {
		go func(i int, call sourceCall) {
			<-start
			c, done, e := f.s.call(call)
			if e != nil {
				answers <- answer{index: i, err: e}
				return
			}
			out, e := f.s.publish(c, m)
			done()
			answers <- answer{i, out, e}
		}(i, call)
	}
	close(start)
	winner, loser := -1, -1
	for range selections {
		a := <-answers
		if a.err == nil {
			winner = a.index
		} else if errors.Is(a.err, ErrWaitingForFunds) {
			loser = a.index
		} else {
			t.Fatal("competing publish", a.err)
		}
	}
	if winner < 0 || loser < 0 || m.builds != 1 || m.sends != 1 {
		t.Fatal("coin selected twice", winner, loser, m.builds, m.sends)
	}
	checkKeys("publish answers")
	auditCurrent(t, f)
	checkKeys("auditCurrent")
	first, _, e := f.s.record(records[winner].ResultKey)
	if e != nil {
		t.Fatal(e)
	}
	original := *first.Publication
	t.Logf("winner=%d loser=%d keys=%s/%s", winner, loser, records[0].ResultKey, records[1].ResultKey)
	// A funding-only reorg atomically clears inventory and preserves the input
	// reservation. Rebuilding that same coin cannot free it for the other result.
	m.tip.Hash = strings.Repeat("d", 64)
	m.hashes[m.tip.Height] = m.tip.Hash
	c, done, e := f.s.call(selections[loser])
	if e != nil {
		t.Fatal(e)
	}
	out, e := f.s.reconcile(c, m)
	done()
	checkKeys("reconcile")
	if e != nil || out.Status.FundingState != "scanning" {
		t.Fatal("funding restart", e)
	}
	if _, ok, e := f.s.raw(coinKey('u', original.Coin.Outpoint), 256); e != nil || !ok {
		t.Fatal("reservation released on reorg", e)
	}
	next := f.s.meta
	next.FundingComplete = true
	next.FundingCursor.Height = int32(m.tip.Height)
	next.FundingCursor.Hash = m.tip.Hash
	next.FundingCursor.CompleteToHeight = int32(m.tip.Height)
	next.FundingCursor.CompleteToHash = m.tip.Hash
	next.FundingCursor.ScanDigest = strings.Repeat("c", 64)
	b := new(leveldb.Batch)
	if e = put(b, coinKey('c', original.Coin.Outpoint), original.Coin, 1024); e != nil {
		t.Fatal(e)
	}
	if e = f.s.commitLocked(b, next); e != nil {
		t.Fatal(e)
	}
	checkKeys("reintroduced funding sync")
	c, done, e = f.s.call(selections[loser])
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.publish(c, m)
	done()
	checkKeys("loser repeated publish")
	if !errors.Is(e, ErrWaitingForFunds) || m.sends != 1 {
		t.Fatal("reintroduced reserved coin spent", e)
	}
	first, found, readErr := f.s.record(records[winner].ResultKey)
	e = readErr
	if e != nil || first.Publication == nil || !equal(*first.Publication, original) {
		raw, getErr := f.s.db.Get(key('r', records[winner].ResultKey), nil)
		t.Fatalf("original publication replaced: error=%v found=%v requested=%q winner=%d actualKey=%s publication=%v directGet=%v/%d index=%+v", e, found, records[winner].ResultKey, winner, first.ResultKey, first.Publication, getErr, len(raw), f.s.index)
	}
	auditCurrent(t, f)
}
