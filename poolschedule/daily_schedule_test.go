// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestDailyScheduleOriginalOwnerModesAndReopen(t *testing.T) {
	h := newDailyScheduleHarness(t)
	s, path, c := dailyScheduleFixture(t, h, 0)
	ctx := context.Background()
	other := filepath.Join(filepath.Dir(path), "other")
	if _, err := CreateDaily(ctx, other, c, h.store); !errors.Is(err, ErrBusy) {
		t.Fatal("second original owner", err)
	}
	if _, err := os.Lstat(other); !os.IsNotExist(err) {
		t.Fatal("second owner touched storage")
	}
	if id, err := SettlementBinding(ctx, h.store); err == nil || id != "" {
		t.Fatal("legacy binding adopted daily store")
	}
	legacy := c
	legacy.Version = Version1
	if _, err := Create(ctx, other, legacy, h.store); err == nil {
		t.Fatal("legacy constructor adopted daily store")
	}
	if _, err := CreateDaily(ctx, other, legacy, h.store); !errors.Is(err, ErrInvalid) {
		t.Fatal("daily constructor adopted V1 config", err)
	}
	if err := oneCycle(t, s); err != nil {
		t.Fatal(err)
	}
	st, err := s.Status(ctx)
	if err != nil || st.State.Version != Version2 || st.State.Config != c || st.State.DailyCursor != "" || st.State.Attempt.Kind != "daily" || st.State.Attempt.ReportState != "deferred" {
		t.Fatal("empty due attempt", st, err)
	}
	if h.backend.feeCalls != 0 || h.backend.signCalls != 0 || h.backend.broadcasts != 0 {
		t.Fatal("empty due signed or paid")
	}
	if err = s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	changed := c
	changed.RetryMS++
	if got, e := OpenDaily(ctx, path, changed, h.store); got != nil || e == nil {
		t.Fatal("changed original selection", e)
	}
	s, err = OpenDaily(ctx, path, c, h.store)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(ctx)
	restored, err := s.Status(ctx)
	if err != nil || restored.State.Revision != st.State.Revision || restored.State.Config != c || restored.State.DailyCursor != "" {
		t.Fatal("physical schedule reopen", err)
	}
	if err = s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err = h.store.Close(); err != nil {
		t.Fatal(err)
	}
	audit, err := poolsettle.OpenDailyAudit(ctx, h.path, h.config, h.source)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	if id, e := DailySettlementBinding(ctx, audit); id != "" || e == nil {
		t.Fatal("read-only daily owner admitted", e)
	}
}

func TestDailySchedulePendingRecoveryBeforeDue(t *testing.T) {
	now := time.Now().UTC()
	minute := now.Hour()*60 + now.Minute()
	if minute >= 1438 {
		t.Skip("requires two real UTC minutes before midnight")
	}
	h := newDailyScheduleHarness(t)
	h.confirmFirst(t)
	s, path, c := dailyScheduleFixture(t, h, uint32(minute+2))
	if err := oneCycle(t, s); err != nil {
		t.Fatal(err)
	}
	if h.backend.feeCalls != 0 || h.backend.signCalls != 0 {
		t.Fatal("pre-due imported/signed new payout")
	}
	h.backend.loseAck = true
	report, err := h.store.RunDaily(context.Background())
	if err != nil || report.State != "pending" || len(h.backend.broadcastBytes) != 1 {
		t.Fatal("original lost-ack intent", report, err)
	}
	original := append([]byte(nil), h.backend.broadcastBytes[0]...)
	if err = s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = h.store.Close(); err != nil {
		t.Fatal(err)
	}
	h.store, err = poolsettle.OpenDaily(context.Background(), h.path, h.config, h.source, h.backend)
	if err != nil {
		t.Fatal(err)
	}
	h.backend.txs = make(map[string][]byte)
	s, err = OpenDaily(context.Background(), path, c, h.store)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(context.Background())
	if err = oneCycle(t, s); err != nil {
		t.Fatal(err)
	}
	if h.backend.signCalls != 1 || h.backend.feeCalls != 1 || len(h.backend.broadcastBytes) != 2 || !bytes.Equal(original, h.backend.broadcastBytes[1]) {
		t.Fatal("daily recovery re-signed or changed exact A")
	}
	st, err := s.Status(context.Background())
	if err != nil || st.State.DailyCursor != "" || st.State.Attempt.Kind != "recovery" {
		t.Fatal("pre-due recovery occupied day", err)
	}
	h.backend.confirmations = 1
	if err = oneCycle(t, s); err != nil {
		t.Fatal(err)
	}
	st, err = s.Status(context.Background())
	if err != nil || st.State.DailyCursor != "" || h.backend.signCalls != 1 {
		t.Fatal("recovery completion paid again/occupied cursor", err)
	}
}

func TestDailyScheduleRunCancelAndJoin(t *testing.T) {
	h := newDailyScheduleHarness(t)
	s, _, _ := dailyScheduleFixture(t, h, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	defer func() {
		cancel()
		if e := s.Close(context.Background()); e != nil {
			t.Error(e)
		}
	}()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		st, err := s.Status(context.Background())
		if err == nil && st.State.Attempt != nil && st.State.Attempt.Kind == "daily" && st.State.Attempt.Result == "returned" {
			break
		}
		if err != nil && !errors.Is(err, ErrBusy) {
			cancel()
			<-done
			t.Fatal(err)
		}
		select {
		case <-deadline.C:
			cancel()
			<-done
			t.Fatal("daily loop did not return empty deferral")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not join cancellation")
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.backend.feeCalls != 0 || h.backend.signCalls != 0 || h.backend.broadcasts != 0 {
		t.Fatal("empty original loop performed payment")
	}
}

func TestDailyScheduleCheapAndCanceledBeforeEffects(t *testing.T) {
	c := Config{Version: Version2, SchedulerID: repeatID('1'), RuntimeID: repeatID('2'), SettlementBindingID: repeatID('3'), DailyUTCMinute: 0, RetryMS: 1000}
	path := filepath.Join(t.TempDir(), "never")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CreateDaily(ctx, path, c, &poolsettle.Store{})
	if wire.TestPoW {
		if !errors.Is(err, ErrProfile) {
			t.Fatal(err)
		}
	} else if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("refusal touched filesystem")
	}
	if wire.TestPoW {
		if id, e := DailySettlementBinding(context.Background(), nil); id != "" || !errors.Is(e, ErrProfile) {
			t.Fatal("cheap binding", e)
		}
		if _, e := OpenDaily(context.Background(), path, c, nil); !errors.Is(e, ErrProfile) {
			t.Fatal("cheap open", e)
		}
	}
}
