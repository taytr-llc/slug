//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/prospectwork"
)

// The public full-audit walk itself owns the original round gate. No private
// lock mutation, substitute source or injected scheduling callback is used.
func holdScheduleOriginalRound(t *testing.T, h *dailyScheduleHarness) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	var enterOnce, releaseOnce sync.Once
	go func() {
		_, err := h.source.WalkSettlement(ctx, func(_ []poolround.Record) error {
			enterOnce.Do(func() { close(entered) })
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		done <- err
	}()
	joined := false
	join := func() {
		if joined {
			return
		}
		releaseOnce.Do(func() { close(release) })
		select {
		case err := <-done:
			joined = true
			cancel()
			if err != nil {
				t.Error("original round walk failed", err)
			}
		case <-time.After(10 * time.Second):
			cancel()
			t.Error("original round walk failed to join")
		}
	}
	t.Cleanup(join)
	select {
	case <-entered:
	case err := <-done:
		joined = true
		cancel()
		t.Fatal("original round audit did not reach page", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("original round walk did not acquire original gate")
	}
	return join
}

func TestDailyScheduleOriginalRoundBusyRecoversWithoutNewPayment(t *testing.T) {
	if now := time.Now().UTC(); now.Hour() == 23 && now.Minute() >= 58 {
		t.Skip("same-day contention test requires two real UTC minutes before midnight")
	}
	h := newDailyScheduleHarness(t)
	h.confirmFirst(t)
	s, path, config := dailyScheduleFixture(t, h, 0)
	ctx := context.Background()
	day := time.Now().UTC().Format("2006-01-02")
	h.backend.loseAck = true
	if err := oneCycle(t, s); err != nil {
		t.Fatal(err)
	}
	if h.backend.signCalls != 1 || h.backend.feeCalls != 1 || h.backend.broadcasts != 1 {
		t.Fatal("original pending A missing")
	}
	s, before := completedRefreshReopen(t, h, s, path, config)
	bodyA, creditA := before[completedRefreshKey("b/", 1)], before[completedRefreshKey("c/", 1)]
	if len(bodyA) == 0 || len(creditA) == 0 {
		t.Fatal("missing original immutable pending evidence")
	}
	originalSend := append([]byte(nil), h.backend.broadcastBytes[0]...)
	h.backend.confirmations = 1
	runAcrossBusy := func(label string) {
		t.Helper()
		prior, err := s.Status(ctx)
		if err != nil {
			t.Fatal(err)
		}
		release := holdScheduleOriginalRound(t, h)
		// Explicit DAILY scheduling now waits on this same audited owner within
		// the caller's bound; ordinary public/full-read APIs remain nonwaiting.
		bindingCtx, bindingCancel := context.WithTimeout(ctx, 50*time.Millisecond)
		binding, bindingErr := DailySettlementBinding(bindingCtx, h.store)
		bindingCancel()
		if binding != "" || !errors.Is(bindingErr, context.DeadlineExceeded) {
			t.Fatal(label, "original binding ignored bounded contention", binding, bindingErr)
		}
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- s.Run(runCtx) }()
		joined := false
		stop := func() {
			if joined {
				return
			}
			cancel()
			select {
			case err := <-done:
				joined = true
				if !errors.Is(err, context.Canceled) {
					t.Error(label, "unexpected scheduler result", err)
				}
			case <-time.After(10 * time.Second):
				t.Error(label, "scheduler did not join")
			}
		}
		t.Cleanup(stop)
		// The same gate remains occupied beyond the immutable one-second retry
		// interval. The old implementation returned round Busy fatally here.
		select {
		case err := <-done:
			joined = true
			cancel()
			release()
			t.Fatal(label, "ordinary contention stopped Run", err)
		case <-time.After(1200 * time.Millisecond):
		}
		during, err := s.Status(ctx)
		if err != nil || !during.Running || !reflect.DeepEqual(prior.State, during.State) {
			t.Fatal(label, "pre-dispatch Busy changed durable attempt/cursor", during, err)
		}
		if h.backend.signCalls != 1 || h.backend.feeCalls != 1 || h.backend.broadcasts != 1 {
			t.Fatal(label, "busy caused payment effects")
		}
		release()
		deadline := time.NewTimer(15 * time.Second)
		defer deadline.Stop()
		for {
			st, err := s.Status(ctx)
			if err == nil && st.State.Revision > prior.State.Revision && st.State.DailyCursor == day && st.State.Attempt != nil && st.State.Attempt.Kind == "daily" && st.State.Attempt.Result == "returned" && st.State.Attempt.ReportReason == "daily-batch-complete" {
				break
			}
			if err != nil && err != ErrBusy {
				t.Fatal(label, err)
			}
			select {
			case err := <-done:
				joined = true
				cancel()
				t.Fatal(label, "Run stopped after gate release", err)
			case <-deadline.C:
				t.Fatal(label, "original scheduler did not recover after gate release")
			case <-time.After(10 * time.Millisecond):
			}
		}
		stop()
		if h.backend.signCalls != 1 || h.backend.feeCalls != 1 || h.backend.broadcasts != 1 || len(h.backend.broadcastBytes) != 1 || !bytes.Equal(originalSend, h.backend.broadcastBytes[0]) {
			t.Fatal(label, "recovery signed/quoted/sent replacement A")
		}
	}
	runAcrossBusy("pending A binding")
	s, completed := completedRefreshReopen(t, h, s, path, config)
	if !bytes.Equal(bodyA, completed[completedRefreshKey("b/", 1)]) || !bytes.Equal(creditA, completed[completedRefreshKey("c/", 1)]) {
		t.Fatal("A recovery rewrote immutable evidence")
	}
	// Add a later actually hashed original B after A's day cursor is occupied.
	h.node.template.Context.ParentHeight += 61
	h.node.template.Context.ParentHash = strings.Repeat("a", 64)
	h.node.template.Block.Height = int64(h.node.template.Context.ParentHeight) + 1
	h.node.template.Block.PreviousHash = h.node.template.Context.ParentHash
	work, err := h.pool.IssueMiningWork(ctx, settlementTestToken)
	if err != nil {
		t.Fatal(err)
	}
	_, _, payout := settlementFixtureKey(3)
	prepared, err := prospectwork.PrepareManifestWork(work.Job, work.Template, payout, 0, strings.Repeat("f", 64))
	if err != nil {
		t.Fatal(err)
	}
	var winner prospectwork.ShareCheck
	for nonce := uint32(0); nonce < 64; nonce++ {
		candidate, err := prepared.CheckNonce(ctx, nonce)
		if err != nil {
			t.Fatal(err)
		}
		if candidate.Block {
			winner = candidate
			break
		}
	}
	if winner.Candidate == nil {
		t.Fatal("bounded normal B work exhausted")
	}
	h.node.winner = winner
	if r, err := h.pool.Submit(ctx, settlementTestToken, work.Job.ID, winner.Nonce); err != nil || r.AcceptedSequence != 2 {
		t.Fatal(r, err)
	}
	if _, err = h.source.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if r, err := h.source.Refresh(ctx, winner.WorkHash); err != nil || r.Confirmation == nil {
		t.Fatal(r, err)
	}
	runAcrossBusy("completed-day B refresh binding")
	s, after := completedRefreshReopen(t, h, s, path, config)
	if !bytes.Equal(bodyA, after[completedRefreshKey("b/", 1)]) || !bytes.Equal(creditA, after[completedRefreshKey("c/", 1)]) {
		t.Fatal("B refresh rewrote immutable A")
	}
	var head struct {
		Credits, Batches, Events uint64
		Pending                  uint64 `json:"pending_batch"`
	}
	if json.Unmarshal(after["head"], &head) != nil || head.Credits != 2 || head.Batches != 1 || head.Events != 3 || head.Pending != 0 {
		t.Fatal("exact once-only recovered ledger", head)
	}
	var credit struct {
		RoundHash string `json:"round_hash"`
	}
	if json.Unmarshal(after[completedRefreshKey("c/", 2)], &credit) != nil || credit.RoundHash != winner.WorkHash {
		t.Fatal("later original B was not imported", credit)
	}
	if err = oneCycle(t, s); err != nil {
		t.Fatal(err)
	}
	s, final := completedRefreshReopen(t, h, s, path, config)
	if !bytes.Equal(bodyA, final[completedRefreshKey("b/", 1)]) || !bytes.Equal(after[completedRefreshKey("c/", 2)], final[completedRefreshKey("c/", 2)]) || final[completedRefreshKey("c/", 3)] != nil || final[completedRefreshKey("b/", 2)] != nil {
		t.Fatal("repeat produced duplicate financial state")
	}
	if h.backend.signCalls != 1 || h.backend.feeCalls != 1 || h.backend.broadcasts != 1 {
		t.Fatal("repeat produced a payment")
	}
}
