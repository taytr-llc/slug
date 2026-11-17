// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymprovider

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// The verifier owns the real clock. Returning a captured time from observe only
// after the signed next second begins deterministically models a request that
// crosses a second during verification, without an exported clock override.
func providerCrossSecond(c *Coordinator) (int64, func() time.Time) {
	observed := time.Now()
	target := observed.Unix() + 1
	return target, func() time.Time {
		if wait := time.Until(time.Unix(target, 0)); wait > 0 {
			time.Sleep(wait)
		}
		return observed
	}
}
func TestPreparedEvidenceCrossSecondRemainsReopenable(t *testing.T) {
	for _, kind := range []string{"login", "reset"} {
		t.Run(kind, func(t *testing.T) {
			f := newProviderFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var id string
			var evidence string
			var expectedPhase string
			var target int64
			if kind == "login" {
				flow, e := f.Coordinator.IssueLogin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				id = flow.ID
				target, f.Coordinator.now = providerCrossSecond(f.Coordinator)
				claims := providerLoginClaims(flow, "alice", strings.Repeat("71", 32), flow.ID, "fresh", 1, target*1000)
				claims["iat"] = target
				claims["exp"] = target + 250
				evidence = signProviderClaims(t, false, claims)
				expectedPhase = "submitting"
			} else {
				_, _ = coreLogin(t, f, "alice", strings.Repeat("72", 32), 1)
				req, e := f.Coordinator.IssueReset(ctx)
				if e != nil {
					t.Fatal(e)
				}
				id = req.EventID
				target, f.Coordinator.now = providerCrossSecond(f.Coordinator)
				claims := providerResetClaims(req, "alice", strings.Repeat("73", 32), "begin", 1, 2, target*1000)
				claims["iat"] = target
				claims["exp"] = target + 250
				evidence = signProviderClaims(t, true, claims)
				expectedPhase = "begin_prepared"
			}
			reached := false
			f.Coordinator.checkpoint = func(stage string) {
				if stage == "submitting-login" || stage == "begin-prepared" {
					reached = true
					cancel()
				}
			}
			var e error
			if kind == "login" {
				_, e = f.Coordinator.FinishLogin(ctx, id, evidence)
			} else {
				_, e = f.Coordinator.BeginReset(ctx, id, evidence)
			}
			if !reached || !errors.Is(e, context.Canceled) {
				t.Fatal("did not stop at prepared boundary", e)
			}
			if f.Coordinator.meta.ClockMS < target*1000 {
				t.Fatal("verified timestamp exceeded durable clock")
			}
			if e := f.Coordinator.Close(); e != nil {
				t.Fatal(e)
			}
			f.Coordinator, e = Open(context.Background(), filepath.Join(f.Root, "coordinator"), f.Config, f.Authority, f.Verifier)
			if e != nil {
				t.Fatal("legitimate prepared state became corrupt", e)
			}
			status, e := f.Coordinator.Status(context.Background(), kind, id)
			if e != nil || status.Phase != expectedPhase {
				t.Fatal("prepared state changed", e)
			}
		})
	}
}
func TestCoordinatorCloseDrainsAndPreservesBorrowedAuthority(t *testing.T) {
	f := newProviderFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	done := make(chan error, 1)
	f.Coordinator.checkpoint = func(stage string) {
		if stage == "reserve-login" {
			close(entered)
			<-release
		}
	}
	go func() { _, e := f.Coordinator.IssueLogin(ctx); done <- e }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("reservation not reached")
	}
	closed := make(chan error, 1)
	go func() { closed <- f.Coordinator.Close() }()
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !f.Coordinator.closing.Load() {
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatal("close did not stop new work")
		}
	}
	if _, e := f.Coordinator.IssueReset(context.Background()); !errors.Is(e, ErrClosed) {
		t.Fatal("work admitted after close began", e)
	}
	select {
	case e := <-closed:
		t.Fatal("close failed to drain accepted operation", e)
	default:
	}
	cancel()
	releaseOnce.Do(func() { close(release) })
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal("accepted operation ignored cancellation", e)
	}
	if e := <-closed; e != nil {
		t.Fatal(e)
	}
	if _, e := f.Authority.IssueLogin(context.Background(), FixtureIssuer); e != nil {
		t.Fatal("coordinator closed borrowed authority", e)
	}
}

// The request can expire during verification even though observation began in
// its last second. Such evidence must never create an unopenable prepared row.
func TestResetVerifiedAtRequestExpiryNeverPrepares(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	_, _ = coreLogin(t, f, "alice", strings.Repeat("81", 32), 1)
	req, err := f.Coordinator.IssueReset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	target, clock := providerCrossSecond(f.Coordinator)
	var row resetRecord
	if err := f.Coordinator.load("reset:"+req.EventID, &row); err != nil {
		t.Fatal(err)
	}
	row.Request.ExpiresAtMS = target * 1000
	row.Request.IssuedAtMS = row.Request.ExpiresAtMS - LifetimeMS
	batch := new(leveldb.Batch)
	if err := put(batch, "reset:"+req.EventID, row); err != nil {
		t.Fatal(err)
	}
	if err := f.Coordinator.db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	f.Coordinator.now = clock
	claims := providerResetClaims(row.Request, "alice", strings.Repeat("82", 32), "begin", 1, 2, target*1000)
	claims["iat"], claims["exp"] = target, target+250
	if _, err := f.Coordinator.BeginReset(ctx, req.EventID, signProviderClaims(t, true, claims)); !errors.Is(err, ErrExpired) {
		t.Fatal("expiry crossing was admitted", err)
	}
	var after resetRecord
	if err := f.Coordinator.load("reset:"+req.EventID, &after); err != nil {
		t.Fatal(err)
	}
	if after.Phase != "issued" || after.Semantic != nil || after.OperationKey != "" {
		t.Fatal("expired verification prepared reset state")
	}
	f.Coordinator.now = time.Now
	if err := f.Coordinator.Close(); err != nil {
		t.Fatal(err)
	}
	f.Coordinator, err = Open(ctx, filepath.Join(f.Root, "coordinator"), f.Config, f.Authority, f.Verifier)
	if err != nil {
		t.Fatal("rejected expiry damaged journal", err)
	}
}
