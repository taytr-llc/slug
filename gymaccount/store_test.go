// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/btcec/v2"
)

func accountTestConfig() Config {
	return Config{Version: 1, Principals: []FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 128, MaxChallenges: 64, MaxEvents: 256}
}
func accountTestStore(t *testing.T) (*Store, *atomic.Int64, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "accounts")
	s, err := Create(path, accountTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	clock := new(atomic.Int64)
	clock.Store(1700000000000)
	s.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	t.Cleanup(func() { s.Close() })
	return s, clock, path
}
func accountTestPrincipal(subject string, clock *atomic.Int64) Principal {
	return Principal{Issuer: "fixture:local", Subject: subject, AuthenticatedAtMS: clock.Load()}
}
func accountTestLogin(t *testing.T, s *Store, subject string, clock *atomic.Int64) Session {
	t.Helper()
	session, err := s.Login(context.Background(), accountTestPrincipal(subject, clock))
	if err != nil {
		t.Fatal(err)
	}
	return session
}
func accountTestKey(t *testing.T, subject string) *btcec.PrivateKey {
	t.Helper()
	scalar := "9d3035e7440dd3737fe58ffb69ca3576257d424b6e6172b3199a66a8897bddf6"
	if subject == "bob" {
		scalar = "948996ecaa9fa9707693267d6d4abefe0b5fb28b6ebae34f542e106244d254e1"
	}
	raw, err := hex.DecodeString(scalar)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := btcec.PrivKeyFromBytes(raw)
	clear(raw)
	t.Cleanup(key.Zero)
	return key
}
func accountTestEnroll(t *testing.T, s *Store, session Session, subject, action string) (EnrollmentChallenge, Authorization, EnrollmentResult) {
	t.Helper()
	c, err := s.BeginChallenge(context.Background(), session.Token, action)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := SignChallenge(c, accountTestKey(t, subject))
	if err != nil {
		t.Fatal(err)
	}
	result, inserted, err := s.CompleteChallenge(context.Background(), session.Token, c.ChallengeID, auth)
	if err != nil || !inserted {
		t.Fatalf("complete inserted=%v err=%v", inserted, err)
	}
	return c, auth, result
}
func accountTestRead(t *testing.T, s *Store, token string) Account {
	t.Helper()
	var a Account
	if err := s.WithSession(context.Background(), token, View, func(account Account, check func() error) error { a = account; return check() }); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestStoreConcurrentLoginEnrollmentAndRestart(t *testing.T) {
	s, clock, path := accountTestStore(t)
	var wg sync.WaitGroup
	results := make(chan Session, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session, err := s.Login(context.Background(), accountTestPrincipal("alice", clock))
			results <- session
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first Session
	for session := range results {
		if first.Token == "" {
			first = session
		}
		if session.Account.ID != first.Account.ID {
			t.Fatal("duplicate principal account")
		}
	}
	c, auth, result := accountTestEnroll(t, s, first, "alice", "enroll")
	again, inserted, err := s.CompleteChallenge(context.Background(), first.Token, c.ChallengeID, auth)
	if err != nil || inserted || again != result {
		t.Fatal("successful replay changed enrollment")
	}
	bob := accountTestLogin(t, s, "bob", clock)
	if _, _, err = s.CompleteChallenge(context.Background(), bob.Token, c.ChallengeID, auth); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cross-account replay: %v", err)
	}
	b, err := s.BeginChallenge(context.Background(), bob.Token, "enroll")
	if err != nil {
		t.Fatal(err)
	}
	bad := b
	bad.RootPublicKey = c.RootPublicKey
	bad.GymPublicKey = c.GymPublicKey
	bad.GymID = c.GymID
	badAuth, err := SignChallenge(bad, accountTestKey(t, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.CompleteChallenge(context.Background(), bob.Token, b.ChallengeID, badAuth); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("transplanted identity accepted: %v", err)
	}
	bAuth, _ := SignChallenge(b, accountTestKey(t, "bob"))
	if _, _, err = s.CompleteChallenge(context.Background(), bob.Token, b.ChallengeID, bAuth); err != nil {
		t.Fatal(err)
	}
	before := accountTestRead(t, s, first.Token)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, accountTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopened.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	after := accountTestRead(t, reopened, first.Token)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("restart changed binding")
	}
	projected, _ := json.Marshal(first)
	if strings.Contains(string(projected), first.Token) || strings.Contains(fmt.Sprintf("%v %+v %#v", first, first, first), first.Token) {
		t.Fatal("session credential leaked")
	}
	// Returned account containers are independently owned.
	after.Enrollment.GymID = strings.Repeat("0", 64)
	if accountTestRead(t, reopened, first.Token).Enrollment.GymID == after.Enrollment.GymID {
		t.Fatal("caller mutated retained binding")
	}
}

func TestStoreMiningSessionsRotationAndResetHold(t *testing.T) {
	s, clock, path := accountTestStore(t)
	ctx := context.Background()
	login := accountTestLogin(t, s, "alice", clock)
	_, _, original := accountTestEnroll(t, s, login, "alice", "enroll")
	mining, err := s.MintMining(ctx, login.Token)
	if err != nil {
		t.Fatal(err)
	}
	use := func(token string, cap Capability) error {
		return s.WithSession(ctx, token, cap, func(a Account, check func() error) error { return check() })
	}
	if err = use(mining.Token, Mining); err != nil {
		t.Fatal(err)
	}
	for _, cap := range []Capability{View, Enrollment, RecoveryExport} {
		if err = use(mining.Token, cap); !errors.Is(err, ErrForbidden) {
			t.Fatalf("mining escalation %s: %v", cap, err)
		}
	}
	if err = use(login.Token, Mining); !errors.Is(err, ErrForbidden) {
		t.Fatal("login credential used as mining credential")
	}
	if _, err = s.MintMining(ctx, mining.Token); !errors.Is(err, ErrForbidden) {
		t.Fatal("mining credential minted a child")
	}
	rotated, err := s.Rotate(ctx, login.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = use(mining.Token, Mining); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("rotation retained mining authority")
	}
	if err = use(login.Token, View); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("rotation retained parent")
	}
	if rotated.ExpiresAtMS != login.ExpiresAtMS {
		t.Fatal("rotation extended authentication")
	}
	mining, err = s.MintMining(ctx, rotated.Token)
	if err != nil {
		t.Fatal(err)
	}
	reset := accountTestPrincipal("alice", clock)
	reset.RecoveryVerified = true
	reset.RecoveryEventID = "fixture-reset-one"
	result, inserted, err := s.ApplyReset(ctx, reset)
	if err != nil || !inserted {
		t.Fatal(err)
	}
	if err = use(mining.Token, Mining); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("reset retained prior authority")
	}
	login = accountTestLogin(t, s, "alice", clock)
	a := accountTestRead(t, s, login.Token)
	if a.Enrollment == nil || *a.Enrollment != original.Identity || !a.RecoveryRequired || a.HoldUntilMS != clock.Load()+RecoveryHoldMS {
		t.Fatal("reset replaced identity or omitted hold")
	}
	if _, err = s.MintMining(ctx, login.Token); !errors.Is(err, ErrForbidden) {
		t.Fatal("reset bypassed original key proof")
	}
	accountTestEnroll(t, s, login, "alice", "recover")
	mining, err = s.MintMining(ctx, login.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = use(mining.Token, Mining); err != nil {
		t.Fatalf("hold blocked existing-wallet mining: %v", err)
	}
	for _, cap := range []Capability{RecoveryExport, Withdrawal, FighterTransfer, RecoveryAuthority, MarketplacePurchase, MarketplaceListing} {
		if err = use(login.Token, cap); !errors.Is(err, ErrHeld) {
			t.Fatalf("hold missing for %s: %v", cap, err)
		}
	}
	clock.Add(1000)
	reset.AuthenticatedAtMS = clock.Load()
	again, newReset, err := s.ApplyReset(ctx, reset)
	if err != nil || newReset || again != result {
		t.Fatal("reset replay changed hold/generation")
	}
	reset.RecoveryEventID = "fixture-reset-two"
	second, _, err := s.ApplyReset(ctx, reset)
	if err != nil || second.HoldUntilMS != clock.Load()+RecoveryHoldMS || second.Generation != result.Generation+1 {
		t.Fatal("new reset did not restart hold")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, accountTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	login = accountTestLogin(t, s, "alice", clock)
	accountTestEnroll(t, s, login, "alice", "recover")
	clock.Store(second.HoldUntilMS - 1)
	login = accountTestLogin(t, s, "alice", clock)
	if err = use(login.Token, RecoveryExport); !errors.Is(err, ErrHeld) {
		t.Fatalf("hold ended early: %v", err)
	}
	clock.Add(1)
	if err = use(login.Token, RecoveryExport); err != nil {
		t.Fatalf("hold boundary not released: %v", err)
	}
	if err = s.Logout(ctx, login.Token); err != nil {
		t.Fatal(err)
	}
	if err = s.Logout(ctx, login.Token); err != nil {
		t.Fatal("logout not idempotent")
	}
}

func TestStoreExpiryClockRollbackAndLease(t *testing.T) {
	s, clock, _ := accountTestStore(t)
	ctx := context.Background()
	login := accountTestLogin(t, s, "alice", clock)
	c, err := s.BeginChallenge(ctx, login.Token, "enroll")
	if err != nil {
		t.Fatal(err)
	}
	auth, _ := SignChallenge(c, accountTestKey(t, "alice"))
	clock.Store(c.ExpiresAtMS)
	fresh := accountTestLogin(t, s, "alice", clock)
	if _, _, err = s.CompleteChallenge(ctx, fresh.Token, c.ChallengeID, auth); !errors.Is(err, ErrExpired) {
		t.Fatalf("challenge expiry: %v", err)
	}
	clock.Add(-1)
	if err = s.WithSession(ctx, fresh.Token, View, func(Account, func() error) error { return nil }); !errors.Is(err, ErrClock) {
		t.Fatalf("clock rollback accepted: %v", err)
	}
	clock.Add(1)
	accountTestEnroll(t, s, fresh, "alice", "enroll")
	mining, err := s.MintMining(ctx, fresh.Token)
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	var escaped func() error
	err = s.WithSession(ctx, mining.Token, Mining, func(a Account, check func() error) error {
		escaped = check
		clock.Store(mining.ExpiresAtMS)
		if err := check(); err != nil {
			return err
		}
		committed = true
		return nil
	})
	if !errors.Is(err, ErrExpired) || committed {
		t.Fatal("expired post-work lease allowed commit")
	}
	if !errors.Is(escaped(), ErrClosed) {
		t.Fatal("escaped callback retained lease")
	}
}

func TestStoreResetSerializesProtectedCommit(t *testing.T) {
	s, clock, _ := accountTestStore(t)
	ctx := context.Background()
	login := accountTestLogin(t, s, "alice", clock)
	accountTestEnroll(t, s, login, "alice", "enroll")
	mining, err := s.MintMining(ctx, login.Token)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.WithSession(ctx, mining.Token, Mining, func(Account, func() error) error { close(entered); <-release; return nil })
	}()
	<-entered
	reset := accountTestPrincipal("alice", clock)
	reset.RecoveryVerified = true
	reset.RecoveryEventID = "concurrent-reset"
	resetDone := make(chan error, 1)
	go func() { _, _, err := s.ApplyReset(ctx, reset); resetDone <- err }()
	select {
	case err := <-resetDone:
		t.Fatalf("reset passed active protected operation: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err = s.WithSession(canceled, mining.Token, Mining, func(Account, func() error) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal("contended gate ignored cancellation")
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = <-resetDone; err != nil {
		t.Fatal(err)
	}
	if err = s.WithSession(ctx, mining.Token, Mining, func(Account, func() error) error { return nil }); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old credential survived serialized reset")
	}
}

func TestStoreUncertainWritesReconcile(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprint(after), func(t *testing.T) {
			s, clock, path := accountTestStore(t)
			login := accountTestLogin(t, s, "alice", clock)
			c, err := s.BeginChallenge(context.Background(), login.Token, "enroll")
			if err != nil {
				t.Fatal(err)
			}
			auth, _ := SignChallenge(c, accountTestKey(t, "alice"))
			original := s.writeBatch
			s.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
				if after {
					if err := original(batch, options); err != nil {
						return err
					}
				}
				return errors.New("injected write failure")
			}
			if _, _, err = s.CompleteChallenge(context.Background(), login.Token, c.ChallengeID, auth); !errors.Is(err, ErrPoisoned) {
				t.Fatal("uncertain write not poisoned")
			}
			if err = s.WithSession(context.Background(), login.Token, View, func(Account, func() error) error { return nil }); !errors.Is(err, ErrPoisoned) {
				t.Fatal("poisoned handle usable")
			}
			s.Close()
			recovered, err := Open(path, accountTestConfig())
			if err != nil {
				t.Fatal(err)
			}
			defer recovered.Close()
			recovered.now = func() time.Time { return time.UnixMilli(clock.Load()) }
			_, inserted, err := recovered.CompleteChallenge(context.Background(), login.Token, c.ChallengeID, auth)
			if err != nil || inserted == after {
				t.Fatalf("reconcile inserted=%v after=%v err=%v", inserted, after, err)
			}
		})
	}
}

func TestStoreMarkerAndCorruptionFailClosed(t *testing.T) {
	for _, mutation := range []string{"marker", "missing-index", "unknown-row", "changed-identity", "missing-parent"} {
		t.Run(mutation, func(t *testing.T) {
			s, clock, path := accountTestStore(t)
			login := accountTestLogin(t, s, "alice", clock)
			accountTestEnroll(t, s, login, "alice", "enroll")
			mining, err := s.MintMining(context.Background(), login.Token)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			if mutation == "marker" {
				if err = os.WriteFile(filepath.Join(path, markerName), []byte(`{"version":99}`), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				db, err := leveldb.OpenFile(filepath.Join(path, "db"), nil)
				if err != nil {
					t.Fatal(err)
				}
				switch mutation {
				case "missing-index":
					db.Delete([]byte("principal:"+login.Account.PrincipalID), nil)
				case "unknown-row":
					db.Put([]byte("unexpected"), []byte(`{}`), nil)
				case "changed-identity":
					var a Account
					raw, _ := db.Get([]byte("account:"+login.Account.ID), nil)
					json.Unmarshal(raw, &a)
					a.Enrollment.GymID = strings.Repeat("0", 64)
					raw, _ = json.Marshal(a)
					db.Put([]byte("account:"+a.ID), raw, nil)
				case "missing-parent":
					id, _ := tokenID(mining.Token)
					var record sessionRecord
					raw, _ := db.Get([]byte("session:"+id), nil)
					json.Unmarshal(raw, &record)
					record.Parent = strings.Repeat("0", 64)
					raw, _ = json.Marshal(record)
					db.Put([]byte("session:"+id), raw, nil)
				}
				db.Close()
			}
			if reopened, err := Open(path, accountTestConfig()); err == nil {
				reopened.Close()
				t.Fatal("corrupt store reopened")
			}
		})
	}
	absent := filepath.Join(t.TempDir(), "absent")
	if _, err := Open(absent, accountTestConfig()); err == nil {
		t.Fatal("missing store created")
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Fatal("Open mutated absent directory")
	}
}

func TestStoreResetUnknownOutcomeAndHoldCorruption(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprint(after), func(t *testing.T) {
			s, clock, path := accountTestStore(t)
			login := accountTestLogin(t, s, "alice", clock)
			accountTestEnroll(t, s, login, "alice", "enroll")
			p := accountTestPrincipal("alice", clock)
			p.RecoveryVerified = true
			p.RecoveryEventID = "public-fixture-reset"
			write := s.writeBatch
			s.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
				if after {
					if err := write(batch, options); err != nil {
						return err
					}
				}
				return errors.New("injected commit ACK loss")
			}
			if _, _, err := s.ApplyReset(context.Background(), p); !errors.Is(err, ErrPoisoned) {
				t.Fatal("reset write outcome not marked uncertain")
			}
			s.Close()
			reopened, err := Open(path, accountTestConfig())
			if err != nil {
				t.Fatal(err)
			}
			reopened.now = func() time.Time { return time.UnixMilli(clock.Load()) }
			result, inserted, err := reopened.ApplyReset(context.Background(), p)
			if err != nil || inserted == after || result.Generation != 2 || result.HoldUntilMS != clock.Load()+RecoveryHoldMS {
				t.Fatal("reset lost-ACK recovery changed result")
			}
			reopened.Close()
			// Clearing the requirement cannot be disguised as a successful key proof.
			db, err := leveldb.OpenFile(filepath.Join(path, "db"), nil)
			if err != nil {
				t.Fatal(err)
			}
			var a Account
			raw, _ := db.Get([]byte("account:"+login.Account.ID), nil)
			json.Unmarshal(raw, &a)
			a.RecoveryRequired = false
			raw, _ = json.Marshal(a)
			db.Put([]byte("account:"+a.ID), raw, nil)
			db.Close()
			if open, err := Open(path, accountTestConfig()); err == nil {
				open.Close()
				t.Fatal("corrupt recovery requirement bypassed original key proof")
			}
		})
	}
}

func TestStoreCapacityAndCancelledMutationFailClosed(t *testing.T) {
	config := accountTestConfig()
	config.MaxEvents = 1
	path := filepath.Join(t.TempDir(), "accounts")
	s, err := Create(path, config)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	clock := new(atomic.Int64)
	clock.Store(1700000000000)
	s.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	login := accountTestLogin(t, s, "alice", clock)
	if err = s.WithSession(context.Background(), login.Token, View, func(Account, func() error) error { return nil }); !errors.Is(err, ErrCapacity) {
		t.Fatal("audit exhaustion left a usable but unrevocable session")
	}
	s2, clock2, _ := accountTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s2.Login(ctx, accountTestPrincipal("alice", clock2)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled available-gate login mutated store")
	}
	if s2.meta.Accounts != 0 || s2.meta.Events != 0 || s2.meta.ClockMS != 0 {
		t.Fatal("cancelled login had durable effects")
	}
	unknown := accountTestPrincipal("not-allowed", clock2)
	if _, err = s2.Login(context.Background(), unknown); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("unknown fixture principal accepted")
	}
	if s2.meta.ClockMS != 0 {
		t.Fatal("unknown principal caused a write")
	}
	future := accountTestPrincipal("alice", clock2)
	future.AuthenticatedAtMS++
	if _, err = s2.Login(context.Background(), future); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("future provider authentication accepted")
	}
}
