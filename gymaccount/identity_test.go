// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func identityTestConfig() IdentityConfig {
	return IdentityConfig{Version: IdentityVersion, Accounts: accountTestConfig(), VerifierConfigHash: strings.Repeat("a", 64), MaxLoginAttempts: 128}
}
func identityTestStore(t *testing.T) (*Store, *atomic.Int64, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "identity")
	s, err := CreateIdentity(path, identityTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	clock := new(atomic.Int64)
	clock.Store(1700000000123)
	s.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	t.Cleanup(func() { s.Close() })
	return s, clock, path
}
func identityTestAttempt(t *testing.T, s *Store, clock *atomic.Int64, subject string, epoch uint64) (LoginAttempt, VerifiedLogin) {
	t.Helper()
	attempt, err := s.IssueLogin(context.Background(), "fixture:local")
	if err != nil {
		t.Fatal(err)
	}
	now := clock.Load() / 1000 * 1000
	return attempt, VerifiedLogin{VerifierConfigHash: identityTestConfig().VerifierConfigHash, Issuer: "fixture:local", Subject: subject, Nonce: attempt.Nonce, AuthenticatedAtMS: now, IssuedAtMS: now, ExpiresAtMS: now + FreshAuthenticationMS, CredentialEpoch: epoch}
}
func identityTestLogin(t *testing.T, s *Store, clock *atomic.Int64, subject string, epoch uint64) Session {
	t.Helper()
	a, p := identityTestAttempt(t, s, clock, subject, epoch)
	session, err := s.LoginWithAssertion(context.Background(), a.ID, p)
	if err != nil {
		t.Fatal(err)
	}
	return session
}
func identityTestReset(clock *atomic.Int64, subject, event, stage string, epoch uint64) VerifiedReset {
	now := clock.Load() / 1000 * 1000
	return VerifiedReset{VerifierConfigHash: identityTestConfig().VerifierConfigHash, Issuer: "fixture:local", Subject: subject, EventID: strings.Repeat(event, 64), Stage: stage, FromEpoch: epoch, ToEpoch: epoch + 1, VerifiedAtMS: now, IssuedAtMS: now, ExpiresAtMS: now + FreshAuthenticationMS}
}
func identityRefreshReset(p VerifiedReset, clock *atomic.Int64, stage string) VerifiedReset {
	p.Stage = stage
	p.IssuedAtMS = clock.Load() / 1000 * 1000
	p.ExpiresAtMS = p.IssuedAtMS + FreshAuthenticationMS
	return p
}
func identityReopen(t *testing.T, s *Store, path string, clock *atomic.Int64) *Store {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := OpenIdentity(path, identityTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	next.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	t.Cleanup(func() { next.Close() })
	return next
}

func TestIdentityResetRecoveryAndRestart(t *testing.T) {
	s, clock, path := identityTestStore(t)
	ctx := context.Background()
	login := identityTestLogin(t, s, clock, "alice", 1)
	accountTestEnroll(t, s, login, "alice", "enroll")
	mining, err := s.MintMining(ctx, login.Token)
	if err != nil {
		t.Fatal(err)
	}
	pre, p := identityTestAttempt(t, s, clock, "alice", 1)
	reset := identityTestReset(clock, "alice", "b", "begin", 1)
	begin, changed, err := s.BeginReset(ctx, reset)
	if err != nil || !changed || begin.Generation != 2 || begin.Status != "pending" {
		t.Fatalf("begin: %+v %v %v", begin, changed, err)
	}
	for _, token := range []string{login.Token, mining.Token} {
		if err = s.WithSession(ctx, token, View, func(Account, func() error) error { return nil }); err == nil {
			t.Fatal("pending authorized old session")
		}
	}
	if session, err := s.LoginWithAssertion(ctx, pre.ID, p); err == nil || session.Token != "" {
		t.Fatal("pre-reset principal minted pending session")
	}
	s = identityReopen(t, s, path, clock)
	// A long provider outage does not silently release pending. Fresh transport
	// attests the original verified event rather than inventing a new reset.
	clock.Add(2 * FreshAuthenticationMS)
	if err = s.WithSession(ctx, login.Token, View, func(Account, func() error) error { return nil }); err == nil {
		t.Fatal("pending timed out open")
	}
	during, duringProof := identityTestAttempt(t, s, clock, "alice", 2)
	completeProof := identityRefreshReset(reset, clock, "complete")
	complete, changed, err := s.CompleteReset(ctx, completeProof)
	if err != nil || !changed || complete.Generation != begin.Generation || complete.Status != "completed" || complete.HoldUntilMS != clock.Load()+RecoveryHoldMS {
		t.Fatalf("complete: %+v %v %v", complete, changed, err)
	}
	if _, err = s.LoginWithAssertion(ctx, during.ID, duringProof); err == nil {
		t.Fatal("attempt issued during pending crossed completion")
	}
	s = identityReopen(t, s, path, clock)
	// A new nonce with refreshed transport but the OLD authentication epoch is
	// equally invalid. Freshness/nonce alone must not recreate authority.
	staleAttempt, stale := identityTestAttempt(t, s, clock, "alice", 1)
	if _, err = s.LoginWithAssertion(ctx, staleAttempt.ID, stale); err == nil {
		t.Fatal("old authentication epoch accepted after reset")
	}
	recovered := identityTestLogin(t, s, clock, "alice", 2)
	a := accountTestRead(t, s, recovered.Token)
	expected, _ := FixtureIdentity("zero256")
	if a.Enrollment == nil || *a.Enrollment != expected || !a.RecoveryRequired || a.Generation != 2 {
		t.Fatal("reset changed wallet identity or omitted recovery proof")
	}
	if _, err = s.MintMining(ctx, recovered.Token); err == nil {
		t.Fatal("mining allowed before original gym proof")
	}
	accountTestEnroll(t, s, recovered, "alice", "recover")
	freshMining, err := s.MintMining(ctx, recovered.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WithSession(ctx, freshMining.Token, Mining, func(_ Account, check func() error) error { return check() }); err != nil {
		t.Fatal(err)
	}
	for _, cap := range []Capability{Withdrawal, FighterTransfer, RecoveryExport, RecoveryAuthority, MarketplacePurchase, MarketplaceListing} {
		if err = s.WithSession(ctx, recovered.Token, cap, func(Account, func() error) error { return nil }); !errors.Is(err, ErrHeld) {
			t.Fatalf("%s not held: %v", cap, err)
		}
	}
	replay, changed, err := s.CompleteReset(ctx, completeProof)
	if err != nil || changed || replay != complete {
		t.Fatal("complete replay changed receipt")
	}
	again, changed, err := s.BeginReset(ctx, identityRefreshReset(reset, clock, "begin"))
	if err != nil || changed || again != complete {
		t.Fatal("begin replay did not return original completed receipt")
	}
	clock.Store(complete.HoldUntilMS - 1)
	near := identityTestLogin(t, s, clock, "alice", 2)
	if err = s.WithSession(ctx, near.Token, RecoveryExport, func(Account, func() error) error { return nil }); !errors.Is(err, ErrHeld) {
		t.Fatal("hold released early")
	}
	clock.Add(1)
	if err = s.WithSession(ctx, near.Token, RecoveryExport, func(_ Account, check func() error) error { return check() }); err != nil {
		t.Fatal("exact hold boundary remained blocked", err)
	}
	nextReset := identityTestReset(clock, "alice", "c", "begin", 2)
	if _, changed, err = s.BeginReset(ctx, nextReset); err != nil || !changed {
		t.Fatal(err)
	}
	oldReplay, changed, err := s.CompleteReset(ctx, identityRefreshReset(reset, clock, "complete"))
	if err != nil || changed || oldReplay != complete {
		t.Fatal("superseded event replay changed hold or failed")
	}
	nextComplete, changed, err := s.CompleteReset(ctx, identityRefreshReset(nextReset, clock, "complete"))
	if err != nil || !changed || nextComplete.HoldUntilMS <= complete.HoldUntilMS {
		t.Fatal("distinct reset did not restart hold", err)
	}
	s = identityReopen(t, s, path, clock)
	final := identityTestLogin(t, s, clock, "alice", 3)
	if accountTestRead(t, s, final.Token).Generation != 3 {
		t.Fatal("generation bumped twice")
	}
}

func TestIdentityNonceAndVerifierBinding(t *testing.T) {
	mutations := map[string]func(*VerifiedLogin){
		"config": func(p *VerifiedLogin) { p.VerifierConfigHash = strings.Repeat("b", 64) }, "nonce": func(p *VerifiedLogin) { p.Nonce = strings.Repeat("0", 64) }, "issuer": func(p *VerifiedLogin) { p.Issuer = "other" }, "subject": func(p *VerifiedLogin) { p.Subject = "unlisted" }, "zero_epoch": func(p *VerifiedLogin) { p.CredentialEpoch = 0 }, "wrong_epoch": func(p *VerifiedLogin) { p.CredentialEpoch = 2 }, "future_auth": func(p *VerifiedLogin) { p.AuthenticatedAtMS = p.IssuedAtMS + 1000 }, "old_auth": func(p *VerifiedLogin) { p.AuthenticatedAtMS -= FreshAuthenticationMS + 1000 }, "future_issue": func(p *VerifiedLogin) { p.IssuedAtMS += 1000 }, "expired": func(p *VerifiedLogin) { p.ExpiresAtMS = p.IssuedAtMS }, "excess_lifetime": func(p *VerifiedLogin) { p.ExpiresAtMS += 1 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			s, clock, _ := identityTestStore(t)
			a, p := identityTestAttempt(t, s, clock, "alice", 1)
			bad := p
			mutate(&bad)
			if result, err := s.LoginWithAssertion(context.Background(), a.ID, bad); err == nil || result.Token != "" {
				t.Fatal("bad assertion authorized")
			}
			if s.meta.Sessions != 0 {
				t.Fatal("rejected assertion wrote session")
			}
			if _, err := s.LoginWithAssertion(context.Background(), a.ID, p); err != nil {
				t.Fatal("rejection consumed correct nonce", err)
			}
		})
	}
	s, clock, path := identityTestStore(t)
	a, p := identityTestAttempt(t, s, clock, "alice", 1)
	fetched, err := s.GetLoginAttempt(context.Background(), a.ID)
	if err != nil || fetched != a {
		t.Fatal("persisted attempt lookup mismatch")
	}
	login, err := s.LoginWithAssertion(context.Background(), a.ID, p)
	if err != nil {
		t.Fatal("same-second NumericDate was rejected", err)
	}
	if _, err = s.LoginWithAssertion(context.Background(), a.ID, p); err == nil {
		t.Fatal("consumed assertion replay minted another token")
	}
	s = identityReopen(t, s, path, clock)
	if _, err = s.LoginWithAssertion(context.Background(), a.ID, p); err == nil {
		t.Fatal("reopen forgot nonce consumption")
	}
	if accountTestRead(t, s, login.Token).ID != login.Account.ID {
		t.Fatal("first login lost")
	}
}

func TestIdentitySchemaAndPinnedConfiguration(t *testing.T) {
	s, clock, path := identityTestStore(t)
	ctx := context.Background()
	if _, err := s.Login(ctx, accountTestPrincipal("alice", clock)); !errors.Is(err, ErrForbidden) {
		t.Fatal("legacy login bypass", err)
	}
	legacyReset := accountTestPrincipal("alice", clock)
	legacyReset.RecoveryVerified = true
	legacyReset.RecoveryEventID = "event"
	if _, _, err := s.ApplyReset(ctx, legacyReset); !errors.Is(err, ErrForbidden) {
		t.Fatal("legacy reset bypass", err)
	}
	if _, err := OpenIdentity(path, identityTestConfig()); err == nil {
		t.Fatal("second writer acquired live authority")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(path, markerName))
	if err != nil {
		t.Fatal(err)
	}
	bad := identityTestConfig()
	bad.VerifierConfigHash = strings.Repeat("b", 64)
	if _, err = OpenIdentity(path, bad); err == nil {
		t.Fatal("changed trust configuration accepted")
	}
	if _, err = Open(path, accountTestConfig()); err == nil {
		t.Fatal("schema1 adopted schema2")
	}
	after, _ := os.ReadFile(filepath.Join(path, markerName))
	if string(after) != string(before) {
		t.Fatal("refused open changed marker")
	}
	legacyPath := filepath.Join(t.TempDir(), "legacy")
	old, err := Create(legacyPath, accountTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := old.db.Get([]byte("meta"), nil)
	if strings.Contains(string(raw), "identity") || strings.Contains(string(raw), "reserved_events") {
		t.Fatal("schema1 serialized new fields")
	}
	if _, err = old.IssueLogin(ctx, "fixture:local"); !errors.Is(err, ErrForbidden) {
		t.Fatal("schema1 accepted new API")
	}
	old.Close()
	if _, err = OpenIdentity(legacyPath, identityTestConfig()); err == nil {
		t.Fatal("schema2 silently upgraded schema1")
	}
	reopened, err := Open(legacyPath, accountTestConfig())
	if err != nil {
		t.Fatal("legacy store changed", err)
	}
	reopened.Close()
	huge := identityTestConfig()
	for i := range huge.Accounts.Principals {
		huge.Accounts.Principals[i].Issuer = strings.Repeat("x", 1024)
		huge.Accounts.Principals[i].Subject = strings.Repeat(string(rune('a'+i)), 1024)
	}
	oversized := filepath.Join(t.TempDir(), "oversized")
	if _, err = CreateIdentity(oversized, huge); err == nil {
		t.Fatal("oversized schema created")
	}
	if _, err = os.Stat(oversized); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid config touched filesystem")
	}
}

func TestIdentityReservedCompletionAndResetCAS(t *testing.T) {
	ctx := context.Background()
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
	alice := identityTestLogin(t, s, clock, "alice", 1)
	reset := identityTestReset(clock, "alice", "d", "begin", 1)
	begin, changed, err := s.BeginReset(ctx, reset)
	if err != nil || !changed {
		t.Fatal(err)
	}
	identityTestLogin(t, s, clock, "bob", 1) // The only unreserved audit slot.
	if _, err = s.Rotate(ctx, alice.Token); err == nil {
		t.Fatal("pending old login rotated")
	}
	extra := identityTestReset(clock, "alice", "e", "begin", 1)
	if _, _, err = s.BeginReset(ctx, extra); !errors.Is(err, ErrConflict) {
		t.Fatal("distinct pending reset accepted", err)
	}
	altered := identityRefreshReset(reset, clock, "complete")
	altered.VerifiedAtMS--
	if _, _, err = s.CompleteReset(ctx, altered); !errors.Is(err, ErrConflict) {
		t.Fatal("reset verification time was not immutable", err)
	}
	complete, changed, err := s.CompleteReset(ctx, identityRefreshReset(reset, clock, "complete"))
	if err != nil || !changed || complete.Generation != begin.Generation || s.meta.Events != 4 || s.meta.ReservedEvents != 0 {
		t.Fatal("reserved completion failed", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenIdentity(path, config)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
}

func TestIdentityPendingSerializesProtectedCommit(t *testing.T) {
	s, clock, _ := identityTestStore(t)
	session := identityTestLogin(t, s, clock, "alice", 1)
	entered := make(chan struct{})
	release := make(chan struct{})
	callbackDone := make(chan error, 1)
	go func() {
		callbackDone <- s.WithSession(context.Background(), session.Token, View, func(_ Account, check func() error) error { close(entered); <-release; return check() })
	}()
	<-entered
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.BeginReset(cancelCtx, identityTestReset(clock, "alice", "f", "begin", 1)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled wait did not stop", err)
	}
	resetDone := make(chan error, 1)
	go func() {
		_, _, err := s.BeginReset(context.Background(), identityTestReset(clock, "alice", "f", "begin", 1))
		resetDone <- err
	}()
	select {
	case err := <-resetDone:
		t.Fatal("reset crossed protected callback", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-callbackDone; err != nil {
		t.Fatal(err)
	}
	if err := <-resetDone; err != nil {
		t.Fatal(err)
	}
	if err := s.WithSession(context.Background(), session.Token, View, func(Account, func() error) error { return nil }); err == nil {
		t.Fatal("new capability crossed acknowledged pending")
	}
}

func TestIdentityUncertainWritesReconcile(t *testing.T) {
	for _, stage := range []string{"login", "begin", "complete"} {
		for _, afterCommit := range []bool{false, true} {
			t.Run(stage+map[bool]string{false: "Before", true: "After"}[afterCommit], func(t *testing.T) {
				s, clock, path := identityTestStore(t)
				ctx := context.Background()
				attempt, proof := identityTestAttempt(t, s, clock, "alice", 1)
				reset := identityTestReset(clock, "alice", "a", "begin", 1)
				if stage != "login" {
					if _, err := s.LoginWithAssertion(ctx, attempt.ID, proof); err != nil {
						t.Fatal(err)
					}
				}
				if stage == "complete" {
					if _, _, err := s.BeginReset(ctx, reset); err != nil {
						t.Fatal(err)
					}
				}
				realWrite := s.writeBatch
				s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
					if afterCommit {
						if err := realWrite(b, o); err != nil {
							return err
						}
					}
					return errors.New("injected uncertain write")
				}
				var err error
				switch stage {
				case "login":
					var result Session
					result, err = s.LoginWithAssertion(ctx, attempt.ID, proof)
					if result.Token != "" {
						t.Fatal("uncertain login exposed token")
					}
				case "begin":
					_, _, err = s.BeginReset(ctx, reset)
				case "complete":
					_, _, err = s.CompleteReset(ctx, identityRefreshReset(reset, clock, "complete"))
				}
				if !errors.Is(err, ErrPoisoned) {
					t.Fatal("write failure was not uncertain", err)
				}
				if _, err = s.IssueLogin(ctx, "fixture:local"); !errors.Is(err, ErrPoisoned) {
					t.Fatal("poisoned store remained usable")
				}
				s.Close()
				reopened, err := OpenIdentity(path, identityTestConfig())
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				reopened.now = func() time.Time { return time.UnixMilli(clock.Load()) }
				switch stage {
				case "login":
					_, err = reopened.LoginWithAssertion(ctx, attempt.ID, proof)
					if (err != nil) != afterCommit {
						t.Fatalf("login replay differs from commit outcome: %v", err)
					}
				case "begin":
					_, changed, e := reopened.BeginReset(ctx, reset)
					if e != nil || changed == afterCommit {
						t.Fatal("begin reconciliation", changed, e)
					}
				case "complete":
					r, changed, e := reopened.CompleteReset(ctx, identityRefreshReset(reset, clock, "complete"))
					if e != nil || changed == afterCommit || r.HoldUntilMS != clock.Load()+RecoveryHoldMS {
						t.Fatal("completion reconciliation", changed, e)
					}
				}
			})
		}
	}
}

func TestIdentityCorruptStateFailsClosed(t *testing.T) {
	mutations := map[string]func(*Store, Session, ResetBarrier){
		"missing_identity": func(s *Store, a Session, r ResetBarrier) { s.db.Delete([]byte("identity:"+a.Account.ID), nil) },
		"missing_barrier":  func(s *Store, a Session, r ResetBarrier) { s.db.Delete([]byte("barrier:"+r.EventID), nil) },
		"lost_pending": func(s *Store, a Session, r ResetBarrier) {
			var state identityState
			s.load("identity:"+a.Account.ID, &state)
			state.PendingEventID = ""
			b := new(leveldb.Batch)
			put(b, "identity:"+a.Account.ID, state)
			s.db.Write(b, nil)
		},
		"epoch_rewind": func(s *Store, a Session, r ResetBarrier) {
			var state identityState
			s.load("identity:"+a.Account.ID, &state)
			state.CredentialEpoch = 2
			b := new(leveldb.Batch)
			put(b, "identity:"+a.Account.ID, state)
			s.db.Write(b, nil)
		},
		"missing_reservation": func(s *Store, a Session, r ResetBarrier) {
			meta := s.meta
			meta.ReservedEvents = 0
			b := new(leveldb.Batch)
			put(b, "meta", meta)
			s.db.Write(b, nil)
		},
		"sequence_gap": func(s *Store, a Session, r ResetBarrier) {
			meta := s.meta
			meta.IdentitySequence++
			b := new(leveldb.Batch)
			put(b, "meta", meta)
			s.db.Write(b, nil)
		},
		"changed_verifier": func(s *Store, a Session, r ResetBarrier) {
			meta := s.meta
			copyConfig := *meta.Identity
			copyConfig.VerifierConfigHash = strings.Repeat("b", 64)
			meta.Identity = &copyConfig
			b := new(leveldb.Batch)
			put(b, "meta", meta)
			s.db.Write(b, nil)
		},
		"consumption_removed": func(s *Store, a Session, r ResetBarrier) {
			it := s.db.NewIterator(nil, nil)
			defer it.Release()
			for it.Next() {
				if strings.HasPrefix(string(it.Key()), "attempt:") {
					var x loginAttemptRecord
					json.Unmarshal(it.Value(), &x)
					x.ConsumedAtMS = 0
					x.PrincipalID = ""
					x.SessionID = ""
					x.Generation = 0
					b := new(leveldb.Batch)
					put(b, string(it.Key()), x)
					s.db.Write(b, nil)
					return
				}
			}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			s, clock, path := identityTestStore(t)
			login := identityTestLogin(t, s, clock, "alice", 1)
			r, _, err := s.BeginReset(context.Background(), identityTestReset(clock, "alice", "d", "begin", 1))
			if err != nil {
				t.Fatal(err)
			}
			mutate(s, login, r)
			s.Close()
			if broken, err := OpenIdentity(path, identityTestConfig()); err == nil {
				broken.Close()
				t.Fatal("corrupt authority opened")
			}
		})
	}
}

func TestIdentityConfigAndPublicRecordsOwnCopies(t *testing.T) {
	config := identityTestConfig()
	path := filepath.Join(t.TempDir(), "identity")
	s, err := CreateIdentity(path, config)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	config.Accounts.Principals[0].Subject = "changed"
	if reflect.DeepEqual(s.meta.Config.Principals, config.Accounts.Principals) {
		t.Fatal("caller retained mutable policy alias")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenIdentity(path, identityTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

func TestIdentityExpiredLookupPersistsClockAcrossReopen(t *testing.T) {
	s, clock, path := identityTestStore(t)
	attempt, proof := identityTestAttempt(t, s, clock, "alice", 1)
	clock.Store(attempt.ExpiresAtMS)
	if _, err := s.GetLoginAttempt(context.Background(), attempt.ID); !errors.Is(err, ErrExpired) {
		t.Fatal("lookup accepted expired attempt", err)
	}
	s = identityReopen(t, s, path, clock)
	clock.Store(attempt.IssuedAtMS + 1000)
	if _, err := s.GetLoginAttempt(context.Background(), attempt.ID); !errors.Is(err, ErrClock) {
		t.Fatal("rollback resurrected expired lookup", err)
	}
	if session, err := s.LoginWithAssertion(context.Background(), attempt.ID, proof); !errors.Is(err, ErrClock) || session.Token != "" {
		t.Fatal("rollback resurrected authentication", err)
	}
}

func TestIdentityResetRejectsWrongBindingAndOldVerification(t *testing.T) {
	s, clock, _ := identityTestStore(t)
	identityTestLogin(t, s, clock, "alice", 1)
	good := identityTestReset(clock, "alice", "1", "begin", 1)
	mutations := map[string]func(*VerifiedReset){
		"config": func(p *VerifiedReset) { p.VerifierConfigHash = strings.Repeat("b", 64) }, "stage": func(p *VerifiedReset) { p.Stage = "complete" }, "event": func(p *VerifiedReset) { p.EventID = "opaque" }, "epoch_gap": func(p *VerifiedReset) { p.ToEpoch++ }, "subject": func(p *VerifiedReset) { p.Subject = "bob" }, "future_verification": func(p *VerifiedReset) { p.VerifiedAtMS += 1000 }, "old_verification": func(p *VerifiedReset) { p.VerifiedAtMS -= FreshAuthenticationMS + 1000 }, "expired_transport": func(p *VerifiedReset) { p.ExpiresAtMS = p.IssuedAtMS },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			bad := good
			mutate(&bad)
			if _, _, err := s.BeginReset(context.Background(), bad); err == nil {
				t.Fatal("bad begin accepted")
			}
		})
	}
	begin, changed, err := s.BeginReset(context.Background(), good)
	if err != nil || !changed || begin.Generation != 2 {
		t.Fatal("invalid evidence mutated generation", err)
	}
	wrongEpoch := identityRefreshReset(good, clock, "complete")
	wrongEpoch.FromEpoch = 2
	wrongEpoch.ToEpoch = 3
	if _, _, err = s.CompleteReset(context.Background(), wrongEpoch); !errors.Is(err, ErrConflict) {
		t.Fatal("epoch-swapped completion accepted", err)
	}
	futureEvent := identityRefreshReset(good, clock, "complete")
	futureEvent.EventID = strings.Repeat("2", 64)
	if _, _, err = s.CompleteReset(context.Background(), futureEvent); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown completion accepted", err)
	}
	if _, _, err = s.CompleteReset(context.Background(), identityRefreshReset(good, clock, "complete")); err != nil {
		t.Fatal(err)
	}
	delayed := good
	delayed.EventID = strings.Repeat("3", 64)
	if _, _, err = s.BeginReset(context.Background(), delayed); !errors.Is(err, ErrConflict) {
		t.Fatal("out-of-order old reset accepted", err)
	}
}
