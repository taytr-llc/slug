// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// A new schema-2 reset requires both its begin event and a reserved completion
// event. A session must not remain usable when only one audit slot remains:
// otherwise a verified reset is refused while old credentials retain power.
func TestIdentityOneFreeAuditSlotCannotLeaveUnrevocableSession(t *testing.T) {
	config := identityTestConfig()
	config.Accounts.MaxEvents = 4
	path := filepath.Join(t.TempDir(), "identity")
	s, err := CreateIdentity(path, config)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	clock := new(atomic.Int64)
	clock.Store(1700000000000)
	s.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	ctx := context.Background()
	login := identityTestLogin(t, s, clock, "alice", 1)
	accountTestEnroll(t, s, login, "alice", "enroll")
	if s.meta.Events != 3 || s.meta.ReservedEvents != 0 {
		t.Fatal("fixture did not reach exactly one unreserved audit slot")
	}
	reset := identityTestReset(clock, "alice", "f", "begin", 1)
	if _, changed, err := s.BeginReset(ctx, reset); !errors.Is(err, ErrCapacity) || changed {
		t.Fatalf("reset must refuse without room for durable completion: changed=%v error=%v", changed, err)
	}
	for _, capability := range []Capability{View, Enrollment, RecoveryExport, Withdrawal,
		FighterTransfer, RecoveryAuthority, MarketplacePurchase, MarketplaceListing} {
		entered := false
		err := s.WithSession(ctx, login.Token, capability, func(Account, func() error) error {
			entered = true
			return nil
		})
		if !errors.Is(err, ErrCapacity) || entered {
			t.Errorf("unrevocable %s session remained usable: entered=%v error=%v", capability, entered, err)
		}
	}
	if _, err := s.MintMining(ctx, login.Token); !errors.Is(err, ErrCapacity) {
		t.Fatalf("unrevocable session minted mining credential: %v", err)
	}
	if _, err := s.Rotate(ctx, login.Token); !errors.Is(err, ErrCapacity) {
		t.Fatalf("unrevocable session rotated: %v", err)
	}
}
