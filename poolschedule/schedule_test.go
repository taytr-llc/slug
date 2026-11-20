// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
)

func scheduleFixture(t *testing.T, h *settlementHarness, minute uint32) (*Scheduler, string, Config) {
	t.Helper()
	ctx := context.Background()
	id, e := SettlementBinding(ctx, h.store)
	if e != nil {
		t.Fatal(e)
	}
	c := Config{Version: 1, SchedulerID: repeatID('4'), RuntimeID: repeatID('5'), SettlementBindingID: id, DailyUTCMinute: minute, RetryMS: 1000}
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(parent, "schedule")
	s, e := Create(ctx, path, c, h.store)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(context.Background()); e != nil {
			t.Error(e)
		}
	})
	return s, path, c
}

// Execute exactly one production recovery-first cycle. No injected clock,
// replacement store or generic settlement callback enters production code.
func oneCycle(t *testing.T, s *Scheduler) error {
	t.Helper()
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.running = false; s.mu.Unlock() }()
	return s.cycle(context.Background())
}
func TestScheduleBeforeDueAndExactRecovery(t *testing.T) {
	h := newSettlementHarness(t)
	now := time.Now().UTC()
	minute := now.Hour()*60 + now.Minute()
	if minute >= 1438 {
		t.Skip("pre-due test requires two real UTC minutes before midnight")
	}
	s, path, c := scheduleFixture(t, h, uint32(minute+2))
	if e := oneCycle(t, s); e != nil {
		t.Fatal(e)
	}
	if h.backend.feeCalls != 0 || h.backend.signCalls != 0 || h.backend.broadcasts != 0 {
		t.Fatal("pre-due dispatched new financial work")
	}
	st, e := s.Status(context.Background())
	if e != nil || st.State.DailyCursor != "" || st.State.Attempt.Kind != "recovery" || st.State.Attempt.ReportState != "complete" || st.State.Attempt.ReportReason != "" {
		t.Fatalf("recovery status %+v %v", st, e)
	}
	// A real settlement intent loses its first acknowledgement. Restarting the
	// schedule before due may only observe/rebroadcast those exact retained bytes.
	h.backend.loseAck = true
	r, e := h.store.RunDaily(context.Background())
	if e != nil || r.State != "pending" || len(h.backend.broadcastBytes) != 1 {
		t.Fatalf("lost ack %+v %v", r, e)
	}
	signed := append([]byte(nil), h.backend.broadcastBytes[0]...)
	h.backend.txs = make(map[string][]byte)
	if e = s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	s, e = Open(context.Background(), path, c, h.store)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(context.Background()); e != nil {
			t.Error(e)
		}
	})
	if e = oneCycle(t, s); e != nil {
		t.Fatal(e)
	}
	if h.backend.signCalls != 1 || h.backend.feeCalls != 1 || len(h.backend.broadcastBytes) != 2 || !bytes.Equal(signed, h.backend.broadcastBytes[1]) {
		t.Fatal("recovery changed retained payment")
	}
	h.backend.confirmations = 1
	h.addConfirmedRound(t)
	if e = oneCycle(t, s); e != nil {
		t.Fatal(e)
	}
	if h.backend.signCalls != 1 || h.backend.feeCalls != 1 {
		t.Fatal("new mature credit paid before due")
	}
	st, e = s.Status(context.Background())
	if e != nil || st.State.DailyCursor != "" {
		t.Fatal("recovery occupied daily schedule")
	}
}
func TestScheduleDueCompletionUncertainty(t *testing.T) {
	h := newSettlementHarness(t)
	s, path, c := scheduleFixture(t, h, 0)
	h.backend.beforeBroadcast = func() {
		if e := s.db.Close(); e != nil {
			t.Error(e)
		}
	}
	if e := oneCycle(t, s); !errors.Is(e, ErrPoisoned) {
		t.Fatalf("uncertain completion %v", e)
	}
	if h.backend.signCalls != 1 || len(h.backend.broadcastBytes) != 1 {
		t.Fatal("expected one retained signed payment")
	}
	signed := append([]byte(nil), h.backend.broadcastBytes[0]...)
	// The injected close has already joined the DB. Mark that exact fact solely
	// for test cleanup; production does not repair/reopen an uncertain owner.
	s.mu.Lock()
	s.dbClosed = true
	s.mu.Unlock()
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	h.backend.beforeBroadcast = nil
	h.backend.txs = make(map[string][]byte)
	s, e := Open(context.Background(), path, c, h.store)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(context.Background()); e != nil {
			t.Error(e)
		}
	})
	before, e := s.Status(context.Background())
	if e != nil || before.State.Attempt.Kind != "daily" || before.State.Attempt.Result != "running" {
		t.Fatal("missing interrupted daily intent")
	}
	if e = oneCycle(t, s); e != nil {
		t.Fatal(e)
	}
	if h.backend.signCalls != 1 || len(h.backend.broadcastBytes) != 2 || !bytes.Equal(signed, h.backend.broadcastBytes[1]) {
		t.Fatal("completion loss signed twice")
	}
	h.backend.confirmations = 1
	if e = oneCycle(t, s); e != nil {
		t.Fatal(e)
	}
	st, e := s.Status(context.Background())
	if e != nil || st.State.DailyCursor != time.Now().UTC().Format("2006-01-02") {
		t.Fatalf("occupied day %+v %v", st, e)
	}
}
func TestScheduleOriginalOwnerAndJoinedClose(t *testing.T) {
	h := newSettlementHarness(t)
	s, path, c := scheduleFixture(t, h, 0)
	other := filepath.Join(filepath.Dir(path), "other")
	if _, e := Create(context.Background(), other, c, h.store); !errors.Is(e, ErrBusy) {
		t.Fatal("duplicate pointer owner")
	}
	if _, e := os.Lstat(other); !os.IsNotExist(e) {
		t.Fatal("duplicate claim had filesystem effects")
	}
	// Real production clock and timer: an immature daily attempt retries through
	// recovery after the immutable interval, without fee/signing work.
	h.backend.funding = "immature"
	retryCtx, retryCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer retryCancel()
	inspections := 0
	h.backend.beforeInspect = func(context.Context) {
		inspections++
		if inspections == 2 {
			retryCancel()
		}
	}
	if e := s.Run(retryCtx); !errors.Is(e, context.Canceled) {
		t.Fatal("deferred retry did not join", e)
	}
	if inspections != 2 || h.backend.signCalls != 0 || h.backend.feeCalls != 0 {
		t.Fatal("deferred retry escaped recovery-only ordering")
	}
	h.backend.funding = "ready"
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	var enteredOnce sync.Once
	h.backend.beforeInspect = func(context.Context) { enteredOnce.Do(func() { close(entered) }); <-release }
	go func() { done <- s.Run(context.Background()) }()
	released := false
	joined := false
	defer func() {
		if !released {
			close(release)
		}
		if !joined {
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("unjoined schedule")
			}
		}
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("backend not reached")
	}
	if _, e := s.Status(context.Background()); e != nil {
		t.Fatal("status waited for backend", e)
	}
	if e := s.Run(context.Background()); !errors.Is(e, ErrBusy) {
		t.Fatal("concurrent Run")
	}
	bounded, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	e := s.Close(bounded)
	cancel()
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("close released active call", e)
	}
	if _, e = Open(context.Background(), path, c, h.store); !errors.Is(e, ErrBusy) && !errors.Is(e, poolsettle.ErrBusy) {
		t.Fatal("unjoined close released process claim", e)
	}
	close(release)
	released = true
	select {
	case e = <-done:
		joined = true
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not join")
	}
	if e = s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, _, e = h.store.SchedulerBinding(context.Background()); e != nil {
		t.Fatal("scheduler closed borrowed settlement", e)
	}
}
func TestScheduleBindingFilesAndRefusals(t *testing.T) {
	h := newSettlementHarness(t)
	s, path, c := scheduleFixture(t, h, 0)
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	changed := c
	changed.RuntimeID = repeatID('6')
	if _, e := Open(context.Background(), path, changed, h.store); !errors.Is(e, ErrInvalid) {
		t.Fatal("marker expected-ID adoption", e)
	}
	if e := os.WriteFile(filepath.Join(path, "unexpected"), []byte("public"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(context.Background(), path, c, h.store); e == nil {
		t.Fatal("unexpected root entry")
	}
	if e := os.Remove(filepath.Join(path, "unexpected")); e != nil {
		t.Fatal(e)
	}
	savedDB := filepath.Join(filepath.Dir(path), "saved-db")
	if e := os.Rename(filepath.Join(path, "db"), savedDB); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(context.Background(), path, c, h.store); e == nil {
		t.Fatal("missing established DB accepted")
	}
	if _, e := os.Lstat(filepath.Join(path, "db")); !os.IsNotExist(e) {
		t.Fatal("missing DB recreated")
	}
	if e := os.Rename(savedDB, filepath.Join(path, "db")); e != nil {
		t.Fatal(e)
	}
	marker := filepath.Join(path, markerName)
	savedMarker := filepath.Join(filepath.Dir(path), "saved-marker")
	if e := os.Rename(marker, savedMarker); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(savedMarker, marker); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(context.Background(), path, c, h.store); e == nil {
		t.Fatal("marker symlink accepted")
	}
	if e := os.Remove(marker); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(savedMarker, marker); e != nil {
		t.Fatal(e)
	}
	s, e := Open(context.Background(), path, c, h.store)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(context.Background()); e != nil {
			t.Error(e)
		}
	})
	st, e := s.Status(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	st.State.Config.RuntimeID = repeatID('7')
	again, e := s.Status(context.Background())
	if e != nil || again.State.Config != c {
		t.Fatal("status aliases state")
	}
	if e = h.store.Close(); e != nil {
		t.Fatal(e)
	}
	if e = s.Run(context.Background()); !errors.Is(e, poolsettle.ErrClosed) {
		t.Fatal("closed original store", e)
	}
}
func TestScheduleRejectsCheapProfile(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	c := Config{}
	if _, e := Create(context.Background(), "unused", c, nil); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	if _, e := Open(context.Background(), "unused", c, nil); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	if _, e := SettlementBinding(context.Background(), nil); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	var s *Scheduler
	if e := s.Run(context.Background()); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
}
