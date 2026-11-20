// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"
)

type balanceFenceSessions struct {
	gate     chan struct{}
	captured chan struct{}
	once     sync.Once
	key      string
	revoked  bool
}

func (a *balanceFenceSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	select {
	case a.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-a.gate }()
	check := func() error {
		if a.revoked || token != settlementTestToken {
			return errors.New("fixture session revoked")
		}
		return ctx.Err()
	}
	if e := check(); e != nil {
		return e
	}
	a.once.Do(func() { close(a.captured) })
	return fn(a.key, check)
}

// A real registry gate held by another source operation makes verification
// wait. That wait must not retain the account lease or prevent revocation.
func TestSettlementBalanceSourceVerificationReleasesAccountLease(t *testing.T) {
	h := newSettlementHarness(t)
	key, _, _ := settlementFixtureKey(1)
	defer key.Zero()
	public := hex.EncodeToString(key.PubKey().SerializeCompressed())
	entered, release, sourceDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, e := h.issuer.IssueManifestJob(h.node.template.Context, public, func() error {
			close(entered)
			<-release
			return errors.New("fixture registry stall finished without issuance")
		})
		sourceDone <- e
	}()
	<-entered
	a := &balanceFenceSessions{gate: make(chan struct{}, 1), captured: make(chan struct{}), key: public}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type result struct {
		balance Balance
		err     error
	}
	done := make(chan result, 1)
	go func() { b, _, e := h.store.MinerBalance(ctx, a, settlementTestToken); done <- result{b, e} }()
	var released sync.Once
	joined := false
	defer func() {
		released.Do(func() { close(release) })
		if !joined {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("balance failed to drain")
			}
		}
		<-sourceDone
	}()
	select {
	case <-a.captured:
	case <-ctx.Done():
		t.Fatal("balance did not capture original account")
	}
	select {
	case a.gate <- struct{}{}:
		a.revoked = true
		<-a.gate
	case <-time.After(time.Second):
		t.Fatal("source verification held account lease")
	}
	select {
	case r := <-done:
		joined = true
		t.Fatal("balance bypassed blocked concrete source", r.err)
	default:
	}
	released.Do(func() { close(release) })
	select {
	case r := <-done:
		joined = true
		if r.err == nil || r.balance.GymID != "" {
			t.Fatal("revoked balance disclosed", r.err)
		}
	case <-ctx.Done():
		t.Fatal("balance verification failed to join")
	}
}
