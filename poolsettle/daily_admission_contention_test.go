//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolround"
)

// Hold the actual concrete round gate through its public authenticated walk.
// The callback supplies no source data or authority; it only delays return.
func holdDailyAdmissionRound(t *testing.T, h *lateWinnerOwners) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	var enterOnce, releaseOnce sync.Once
	go func() {
		_, err := h.rounds.WalkSettlement(ctx, func(_ []poolround.Record) error {
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
				t.Error("original round holder", err)
			}
		case <-time.After(10 * time.Second):
			cancel()
			t.Error("original round holder did not join")
		}
	}
	t.Cleanup(join)
	select {
	case <-entered:
	case err := <-done:
		joined = true
		cancel()
		t.Fatal("original round walk never entered", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("original round holder admission")
	}
	return join
}

func confirmDailyAdmissionA(t *testing.T, h *lateWinnerOwners) {
	t.Helper()
	ctx := context.Background()
	work, err := h.pool.Issue(ctx, "gym-a")
	if err != nil {
		t.Fatal(err)
	}
	winner, _ := lateWinnerSearch(t, h, work, false)
	h.node.canonical[winner.WorkHash] = true
	h.node.tip = work.Job.Context.ParentHeight + 61
	if _, err = h.pool.Submit(ctx, "gym-a", work.Job.ID, winner.Nonce); err != nil {
		t.Fatal(err)
	}
	if _, err = h.rounds.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if row, err := h.rounds.Refresh(ctx, winner.WorkHash); err != nil || row.Confirmation == nil {
		t.Fatal(row, err)
	}
}

func TestDailyAdmissionOriginalLedgerWaitCancellationAndClose(t *testing.T) {
	h := dailyIntegrationFixture(t)
	s := h.settlement
	before := dailyRows(t, s)
	// Repeated independent holders exercise progress on the same original owner.
	// No synthetic Busy value or replacement gate enters the public binding API.
	for repeat := 0; repeat < 3; repeat++ {
		end, err := s.begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(end) }
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		done := make(chan error, 1)
		go func() {
			c, p, e := s.DailySchedulerBinding(ctx)
			if e == nil && (!reflect.DeepEqual(c, h.settleConfig) || p != h.poolConfig) {
				e = errors.New("original binding changed")
			}
			done <- e
		}()
		joined := false
		cleanup := func() {
			cancel()
			release()
			if !joined {
				select {
				case <-done:
					joined = true
				case <-time.After(10 * time.Second):
					t.Error("original binding did not join")
				}
			}
		}
		t.Cleanup(cleanup)
		// This original public reader remains active across release, competing
		// through the unchanged nonblocking gate instead of a quiet handoff.
		readCtx, stopReads := context.WithCancel(context.Background())
		readDone := make(chan error, 1)
		readSeen := make(chan struct{}, 2)
		go func() {
			for {
				_, e := s.View(readCtx)
				if readCtx.Err() != nil {
					readDone <- nil
					return
				}
				if e != nil && e != ErrBusy {
					readDone <- e
					return
				}
				select {
				case readSeen <- struct{}{}:
				default:
				}
				select {
				case <-readCtx.Done():
					readDone <- nil
					return
				case <-time.After(time.Millisecond):
				}
			}
		}()
		readsJoined := false
		joinReads := func() {
			if readsJoined {
				return
			}
			stopReads()
			select {
			case e := <-readDone:
				readsJoined = true
				if e != nil {
					t.Error("original public reader", e)
				}
			case <-time.After(10 * time.Second):
				t.Error("original public reader failed to join")
			}
		}
		t.Cleanup(joinReads)
		for seen := 0; seen < 2; seen++ {
			select {
			case <-readSeen:
			case <-ctx.Done():
				t.Fatal("recurring original reads never ran", ctx.Err())
			}
		}
		select {
		case e := <-done:
			joined = true
			cleanup()
			t.Fatal("ledger contention did not queue", e)
		case <-time.After(25 * time.Millisecond):
		}
		// Existing public reads still refuse immediately while the gate is held.
		if _, err = s.View(context.Background()); err != ErrBusy {
			t.Fatal("public read gate changed", err)
		}
		release()
		select {
		case err = <-done:
			joined = true
		case <-ctx.Done():
			t.Fatal("queued original binding did not progress", ctx.Err())
		}
		cancel()
		joinReads()
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, dailyRows(t, s)) {
		t.Fatal("configuration admission wrote ledger")
	}
	for _, deadline := range []bool{false, true} {
		end, err := s.begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var once sync.Once
		release := func() { once.Do(end) }
		ctx, cancel := context.WithCancel(context.Background())
		if deadline {
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
		}
		done := make(chan error, 1)
		go func() { _, _, e := s.DailySchedulerBinding(ctx); done <- e }()
		joined := false
		t.Cleanup(func() {
			cancel()
			release()
			if !joined {
				select {
				case <-done:
					joined = true
				case <-time.After(10 * time.Second):
					t.Error("canceled binding did not join")
				}
			}
		})
		if !deadline {
			select {
			case err = <-done:
				joined = true
				t.Fatal("cancellable admission did not wait", err)
			case <-time.After(25 * time.Millisecond):
			}
			cancel()
		}
		select {
		case err = <-done:
			joined = true
		case <-time.After(5 * time.Second):
			t.Fatal("caller bound ignored")
		}
		want := context.Canceled
		if deadline {
			want = context.DeadlineExceeded
		}
		if !errors.Is(err, want) {
			t.Fatal("caller cause lost", err)
		}
		release()
		cancel()
		if !reflect.DeepEqual(before, dailyRows(t, s)) {
			t.Fatal("canceled binding wrote ledger")
		}
	}
	// A background caller still obeys the invocation's own finite contention
	// budget. The private short deadline models elapsed waiting; production's
	// selected 30s constant and every financial clock remain unchanged.
	endBudget, err := s.begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var budgetRelease sync.Once
	releaseBudget := func() { budgetRelease.Do(endBudget) }
	t.Cleanup(releaseBudget)
	budget := dailyContention{deadline: time.Now().Add(25 * time.Millisecond)}
	queuedEnd, err := s.beginDailyContention(context.Background(), &budget)
	if queuedEnd != nil {
		queuedEnd()
		t.Fatal("expired budget acquired original owner", err)
	}
	if err != context.DeadlineExceeded {
		t.Fatal("background queue ignored finite budget", err)
	}
	releaseBudget()
	if !reflect.DeepEqual(before, dailyRows(t, s)) {
		t.Fatal("expired background admission wrote ledger")
	}
	// Close must join the original holder, while queued/new operations never
	// acquire authority after the same owner's closing flag is published.
	end, err := s.begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() { once.Do(end) }
	waiterCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	waiter := make(chan error, 1)
	closed := make(chan error, 1)
	go func() { _, _, e := s.DailySchedulerBinding(waiterCtx); waiter <- e }()
	go func() { closed <- s.Close() }()
	waiterJoined, closeJoined := false, false
	t.Cleanup(func() {
		cancel()
		release()
		if !waiterJoined {
			select {
			case <-waiter:
				waiterJoined = true
			case <-time.After(10 * time.Second):
				t.Error("closing waiter did not join")
			}
		}
		if !closeJoined {
			select {
			case <-closed:
				closeJoined = true
			case <-time.After(10 * time.Second):
				t.Error("Close did not join")
			}
		}
	})
	until := time.NewTimer(5 * time.Second)
	defer until.Stop()
	for {
		s.lifecycle.Lock()
		closing := s.closing
		s.lifecycle.Unlock()
		if closing {
			break
		}
		select {
		case <-until.C:
			t.Fatal("original Close did not begin")
		case <-time.After(time.Millisecond):
		}
	}
	select {
	case e := <-closed:
		closeJoined = true
		t.Fatal("Close bypassed original holder", e)
	default:
	}
	if _, _, err = s.DailySchedulerBinding(context.Background()); err != ErrClosed {
		t.Fatal("closing owner admitted", err)
	}
	release()
	select {
	case err = <-waiter:
		waiterJoined = true
	case <-waiterCtx.Done():
		t.Fatal("closing waiter failed to join")
	}
	if err != ErrClosed {
		t.Fatal("queued operation survived Close", err)
	}
	select {
	case err = <-closed:
		closeJoined = true
	case <-time.After(5 * time.Second):
		t.Fatal("Close failed to join")
	}
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if h.backend.quotes != 0 || h.backend.signs != 0 || h.backend.broadcasts != 0 {
		t.Fatal("binding/close caused payment effects")
	}
}

func TestDailyAdmissionPostSignRoundContentionUsesOriginalAttempt(t *testing.T) {
	for _, abort := range []bool{false, true} {
		t.Run(map[bool]string{false: "release", true: "cancel"}[abort], func(t *testing.T) {
			h := dailyIntegrationFixture(t)
			confirmDailyAdmissionA(t, h)
			s := h.settlement
			base := h.backend.Backend.(*settlementBackend)
			before := dailyRows(t, s)
			signed, resume := make(chan struct{}), make(chan struct{})
			var resumeOnce sync.Once
			resumeSign := func() { resumeOnce.Do(func() { close(resume) }) }
			base.afterSign = func() { close(signed); <-resume }
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			type result struct {
				report Report
				err    error
			}
			done := make(chan result, 1)
			go func() { r, e := s.RunDaily(ctx); done <- result{r, e} }()
			joined := false
			t.Cleanup(func() {
				cancel()
				resumeSign()
				if !joined {
					select {
					case <-done:
						joined = true
					case <-time.After(10 * time.Second):
						t.Error("post-sign operation did not join")
					}
				}
			})
			select {
			case <-signed:
			case r := <-done:
				joined = true
				t.Fatal("original signer not reached", r)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			release := holdDailyAdmissionRound(t, h)
			resumeSign()
			// Original post-sign read has no mutation authority. While its exact source
			// gate stays occupied, it may not return Busy and cause whole-operation retry.
			select {
			case r := <-done:
				joined = true
				t.Fatal("post-sign source contention escaped", r)
			case <-time.After(75 * time.Millisecond):
			}
			if abort {
				cancel()
			} else {
				release()
			}
			var got result
			select {
			case got = <-done:
				joined = true
			case <-time.After(5 * time.Second):
				t.Fatal("post-sign source operation did not join")
			}
			release()
			cancel()
			base.afterSign = nil
			if h.backend.quotes != 1 || h.backend.signs != 1 || base.signCalls != 1 {
				t.Fatal("post-sign retry repeated quote/sign", h.backend.quotes, h.backend.signs, base.signCalls)
			}
			if abort {
				if !errors.Is(got.err, context.Canceled) || !reflect.DeepEqual(got.report, Report{}) || !reflect.DeepEqual(before, dailyRows(t, s)) || h.backend.broadcasts != 0 {
					t.Fatal("canceled signed attempt committed or sent", got)
				}
				return
			}
			if got.err != nil || got.report.State != "pending" || h.backend.broadcasts != 1 || base.broadcasts != 1 || len(base.broadcastBytes) != 1 {
				t.Fatal("same signed attempt did not complete once", got)
			}
			bodyA := dailyIntegrationRaw(t, s, dailyNumber("b/", 1))
			creditA := dailyIntegrationRaw(t, s, dailyNumber("c/", 1))
			sent := append([]byte(nil), base.broadcastBytes[0]...)
			if len(bodyA) == 0 || len(creditA) == 0 {
				t.Fatal("original committed evidence absent")
			}
			base.confirmations = 1
			if r, e := s.RecoverPending(context.Background()); e != nil || r.State != "complete" {
				t.Fatal(r, e)
			}
			if r, e := s.RunDaily(context.Background()); e != nil || r.Reason != "daily-batch-complete" {
				t.Fatal(r, e)
			}
			if !bytes.Equal(bodyA, dailyIntegrationRaw(t, s, dailyNumber("b/", 1))) || !bytes.Equal(creditA, dailyIntegrationRaw(t, s, dailyNumber("c/", 1))) || h.backend.quotes != 1 || h.backend.signs != 1 || h.backend.broadcasts != 1 || !bytes.Equal(sent, base.broadcastBytes[0]) {
				t.Fatal("recovery changed original signed attempt")
			}
		})
	}
}
