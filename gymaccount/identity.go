// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

func normalizedIdentityConfig(c IdentityConfig) (IdentityConfig, error) {
	if c.Version != IdentityVersion || !validID(c.VerifierConfigHash) || c.MaxLoginAttempts == 0 || c.MaxLoginAttempts > 4096 {
		return IdentityConfig{}, ErrCorrupt
	}
	accounts, err := normalizedConfig(c.Accounts)
	if err != nil {
		return IdentityConfig{}, err
	}
	c.Accounts = accounts
	// The marker repeats the public account policy. Check the largest metadata
	// form before touching the filesystem, not after creating a partial store.
	sample := metadata{Application: application, Version: IdentityVersion, Network: FixtureNetwork, Genesis: FixtureGenesis, Profile: FixtureProfile, HoldMS: RecoveryHoldMS, AuthorityID: strings.Repeat("0", 64), Config: accounts, Identity: &c, ClockMS: math.MaxInt64, Accounts: 2, Sessions: accounts.MaxSessions, Challenges: accounts.MaxChallenges, Events: accounts.MaxEvents, LoginAttempts: c.MaxLoginAttempts, IdentitySequence: math.MaxUint64, ReservedEvents: 2}
	raw, err := json.Marshal(sample)
	if err != nil || len(raw) > MaxRecordSize {
		return IdentityConfig{}, ErrCorrupt
	}
	return c, nil
}
func CreateIdentity(path string, c IdentityConfig) (*Store, error) {
	c, err := normalizedIdentityConfig(c)
	if err != nil {
		return nil, err
	}
	return createStore(path, c.Accounts, &c)
}
func OpenIdentity(path string, c IdentityConfig) (*Store, error) {
	c, err := normalizedIdentityConfig(c)
	if err != nil {
		return nil, err
	}
	return openStore(path, c.Accounts, &c)
}

// CreateIdentityPinned creates a new schema-2 authority with the exact identity
// already committed by its owning initializer. It never adopts an existing
// directory or changes the random-ID behavior of CreateIdentity.
func CreateIdentityPinned(path string, c IdentityConfig, authorityID string) (*Store, error) {
	if !validID(authorityID) || authorityID == strings.Repeat("0", 64) {
		return nil, ErrCorrupt
	}
	c, err := normalizedIdentityConfig(c)
	if err != nil {
		return nil, err
	}
	return createStorePinnedWithClock(path, c.Accounts, &c, time.Now, authorityID)
}

// OpenIdentityPinned audits an established schema-2 authority only after its
// marker matches the owner's separately retained identity and configuration.
func OpenIdentityPinned(path string, c IdentityConfig, authorityID string) (*Store, error) {
	if !validID(authorityID) || authorityID == strings.Repeat("0", 64) {
		return nil, ErrCorrupt
	}
	c, err := normalizedIdentityConfig(c)
	if err != nil {
		return nil, err
	}
	return openStorePinnedWithClock(path, c.Accounts, &c, time.Now, authorityID)
}
func (s *Store) identityMode() error {
	if s.meta.Version != IdentityVersion || s.meta.Identity == nil {
		return ErrForbidden
	}
	return nil
}
func (s *Store) identityState(a Account) (identityState, error) {
	var state identityState
	if s.load("identity:"+a.ID, &state) != nil || state.AccountID != a.ID || state.CredentialEpoch == 0 || state.LastResetSequence > s.meta.IdentitySequence || (state.PendingEventID != "" && !validID(state.PendingEventID)) {
		return identityState{}, ErrCorrupt
	}
	if state.PendingEventID == "" {
		if a.Generation != state.CredentialEpoch {
			return identityState{}, ErrCorrupt
		}
	} else if state.CredentialEpoch == math.MaxUint64 || a.Generation != state.CredentialEpoch+1 {
		return identityState{}, ErrCorrupt
	}
	return state, nil
}
func (s *Store) identityAuthorization(a Account) error {
	if s.meta.Version == Version1 {
		return nil
	}
	if s.identityMode() != nil {
		return ErrCorrupt
	}
	state, err := s.identityState(a)
	if err != nil {
		return err
	}
	if state.PendingEventID != "" {
		return ErrForbidden
	}
	return nil
}
func advanceIdentity(next *metadata) (uint64, error) {
	if next.IdentitySequence == math.MaxUint64 {
		return 0, ErrCapacity
	}
	next.IdentitySequence++
	return next.IdentitySequence, nil
}
func (s *Store) knownIssuer(issuer string) bool {
	for _, p := range s.meta.Config.Principals {
		if p.Issuer == issuer {
			return true
		}
	}
	return false
}

// IssueLogin stores a one-use, authority-bound nonce. Its durable sequence is
// compared to the eventual principal's reset sequence; that principal need not
// be known until the verifier authenticates it.
func (s *Store) IssueLogin(ctx context.Context, issuer string) (LoginAttempt, error) {
	if err := s.lock(ctx); err != nil {
		return LoginAttempt{}, err
	}
	defer s.unlock()
	if err := s.identityMode(); err != nil {
		return LoginAttempt{}, err
	}
	if !s.knownIssuer(issuer) {
		return LoginAttempt{}, ErrUnauthorized
	}
	if s.meta.LoginAttempts >= s.meta.Identity.MaxLoginAttempts {
		return LoginAttempt{}, ErrCapacity
	}
	now, err := s.observe()
	if err != nil {
		return LoginAttempt{}, err
	}
	id, err := randomID(s.random)
	if err != nil {
		return LoginAttempt{}, err
	}
	exists, err := s.db.Has([]byte("attempt:"+id), nil)
	if err != nil {
		return LoginAttempt{}, ErrCorrupt
	}
	if exists {
		return LoginAttempt{}, ErrConflict
	}
	nonce, err := randomID(s.random)
	if err != nil {
		return LoginAttempt{}, err
	}
	next := s.meta
	seq, err := advanceIdentity(&next)
	if err != nil {
		return LoginAttempt{}, err
	}
	attempt := LoginAttempt{Version: IdentityVersion, ID: id, AuthorityID: s.authorityID, Issuer: issuer, Nonce: nonce, Sequence: seq, IssuedAtMS: now, ExpiresAtMS: now + FreshAuthenticationMS}
	next.LoginAttempts++
	batch := new(leveldb.Batch)
	if put(batch, "attempt:"+id, loginAttemptRecord{Attempt: attempt}) != nil {
		return LoginAttempt{}, ErrCorrupt
	}
	if err = s.commitMutation(ctx, batch, next); err != nil {
		return LoginAttempt{}, err
	}
	return attempt, nil
}
func (s *Store) verifiedTimes(verified, issued, expires, now int64) bool {
	return verified > 0 && verified <= issued && issued <= now && expires > issued && expires-issued <= FreshAuthenticationMS && now < expires && now-verified <= FreshAuthenticationMS
}

// LoginWithAssertion consumes an attempt and creates its sole session in the
// same synchronous batch. It accepts only trusted, pinned verifier output.
func (s *Store) LoginWithAssertion(ctx context.Context, id string, p VerifiedLogin) (Session, error) {
	if err := s.lock(ctx); err != nil {
		return Session{}, err
	}
	defer s.unlock()
	if err := s.identityMode(); err != nil {
		return Session{}, err
	}
	if !validID(id) || p.VerifierConfigHash != s.meta.Identity.VerifierConfigHash || p.CredentialEpoch == 0 {
		return Session{}, ErrUnauthorized
	}
	principal, err := PrincipalID(p.Issuer, p.Subject)
	if err != nil {
		return Session{}, ErrUnauthorized
	}
	if _, err = s.fixture(principal); err != nil {
		return Session{}, err
	}
	var record loginAttemptRecord
	if s.load("attempt:"+id, &record) != nil {
		return Session{}, ErrUnauthorized
	}
	if s.validateAttempt(record) != nil || record.Attempt.ID != id || record.Attempt.Issuer != p.Issuer || record.Attempt.Nonce != p.Nonce || record.ConsumedAtMS != 0 {
		return Session{}, ErrUnauthorized
	}
	now, err := s.observe()
	if err != nil {
		return Session{}, err
	}
	if !s.verifiedTimes(p.AuthenticatedAtMS, p.IssuedAtMS, p.ExpiresAtMS, now) || now >= record.Attempt.ExpiresAtMS || p.IssuedAtMS/1000 < record.Attempt.IssuedAtMS/1000 {
		return Session{}, ErrExpired
	}
	next := s.meta
	batch := new(leveldb.Batch)
	var accountID string
	var a Account
	var state identityState
	err = s.load("principal:"+principal, &accountID)
	switch {
	case errors.Is(err, ErrNotFound):
		if next.Accounts >= 2 {
			return Session{}, ErrCapacity
		}
		if p.CredentialEpoch != 1 {
			return Session{}, ErrUnauthorized
		}
		accountID, err = randomID(s.random)
		if err != nil {
			return Session{}, err
		}
		exists, e := s.db.Has([]byte("account:"+accountID), nil)
		if e != nil {
			return Session{}, ErrCorrupt
		}
		if exists {
			return Session{}, ErrConflict
		}
		a = Account{Version: Version1, ID: accountID, PrincipalID: principal, Generation: 1}
		state = identityState{AccountID: accountID, CredentialEpoch: 1}
		next.Accounts++
		if put(batch, "principal:"+principal, accountID) != nil || put(batch, "account:"+accountID, a) != nil || put(batch, "identity:"+accountID, state) != nil {
			return Session{}, ErrCorrupt
		}
	case err != nil:
		return Session{}, err
	default:
		a, err = s.account(accountID)
		if err != nil {
			return Session{}, err
		}
		state, err = s.identityState(a)
		if err != nil {
			return Session{}, err
		}
	}
	if state.PendingEventID != "" || state.CredentialEpoch != p.CredentialEpoch || record.Attempt.Sequence <= state.LastResetSequence {
		return Session{}, ErrUnauthorized
	}
	result, err := s.newSession(batch, &next, a, "login", "", p.AuthenticatedAtMS, now, now+SessionLifetimeMS)
	if err != nil {
		return Session{}, err
	}
	sessionID, _ := tokenID(result.Token)
	record.PrincipalID = principal
	record.SessionID = sessionID
	record.Generation = a.Generation
	record.ConsumedAtMS = now
	if put(batch, "attempt:"+id, record) != nil {
		return Session{}, ErrCorrupt
	}
	if err = s.event(batch, &next, "identity-login", a, now, id); err != nil {
		return Session{}, err
	}
	if err = s.commitMutation(ctx, batch, next); err != nil {
		return Session{}, err
	}
	return result, nil
}
func (s *Store) validateResetInput(p VerifiedReset, stage string, now int64) (string, string, error) {
	if p.VerifierConfigHash != s.meta.Identity.VerifierConfigHash || p.Stage != stage || !validID(p.EventID) || p.FromEpoch == 0 || p.FromEpoch == math.MaxUint64 || p.ToEpoch != p.FromEpoch+1 || !(p.VerifiedAtMS > 0 && p.VerifiedAtMS <= p.IssuedAtMS && p.IssuedAtMS <= now && p.ExpiresAtMS > p.IssuedAtMS && p.ExpiresAtMS-p.IssuedAtMS <= FreshAuthenticationMS && now < p.ExpiresAtMS) {
		return "", "", ErrUnauthorized
	}
	principal, err := PrincipalID(p.Issuer, p.Subject)
	if err != nil {
		return "", "", ErrUnauthorized
	}
	if _, err = s.fixture(principal); err != nil {
		return "", "", err
	}
	event, err := resetID(principal, p.EventID)
	return principal, event, err
}
func resetBinding(r ResetBarrier, principal string, p VerifiedReset) bool {
	return r.PrincipalID == principal && r.FromEpoch == p.FromEpoch && r.ToEpoch == p.ToEpoch && r.VerifiedAtMS == p.VerifiedAtMS
}

// BeginReset revokes old-generation sessions and persists a fail-closed barrier
// BEFORE the provider may grant changed-credential access. The next audit slot
// is reserved for completion; ordinary operations cannot consume that slot.
func (s *Store) BeginReset(ctx context.Context, p VerifiedReset) (ResetBarrier, bool, error) {
	if err := s.lock(ctx); err != nil {
		return ResetBarrier{}, false, err
	}
	defer s.unlock()
	if err := s.identityMode(); err != nil {
		return ResetBarrier{}, false, err
	}
	// Reject untrusted authority/principal fields before observing the clock.
	if p.VerifierConfigHash != s.meta.Identity.VerifierConfigHash {
		return ResetBarrier{}, false, ErrUnauthorized
	}
	principal, err := PrincipalID(p.Issuer, p.Subject)
	if err != nil {
		return ResetBarrier{}, false, ErrUnauthorized
	}
	if _, err = s.fixture(principal); err != nil {
		return ResetBarrier{}, false, err
	}
	now, err := s.observe()
	if err != nil {
		return ResetBarrier{}, false, err
	}
	principal, id, err := s.validateResetInput(p, "begin", now)
	if err != nil {
		return ResetBarrier{}, false, err
	}
	var old ResetBarrier
	err = s.load("barrier:"+id, &old)
	if err == nil {
		if s.validateBarrier(old) != nil || old.EventID != id {
			return ResetBarrier{}, false, ErrCorrupt
		}
		if !resetBinding(old, principal, p) {
			return ResetBarrier{}, false, ErrConflict
		}
		return old, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return ResetBarrier{}, false, err
	}
	var accountID string
	if err = s.load("principal:"+principal, &accountID); err != nil {
		return ResetBarrier{}, false, err
	}
	a, err := s.account(accountID)
	if err != nil {
		return ResetBarrier{}, false, err
	}
	state, err := s.identityState(a)
	if err != nil {
		return ResetBarrier{}, false, err
	}
	if now-p.VerifiedAtMS > FreshAuthenticationMS {
		return ResetBarrier{}, false, ErrExpired
	}
	if state.PendingEventID != "" || state.CredentialEpoch != p.FromEpoch {
		return ResetBarrier{}, false, ErrConflict
	}
	if a.Generation == math.MaxUint64 || uint64(s.meta.Events)+uint64(s.meta.ReservedEvents)+2 > uint64(s.meta.Config.MaxEvents) {
		return ResetBarrier{}, false, ErrCapacity
	}
	next := s.meta
	seq, err := advanceIdentity(&next)
	if err != nil {
		return ResetBarrier{}, false, err
	}
	a.Generation++
	a.RecoveryRequired = a.Enrollment != nil
	state.PendingEventID = id
	r := ResetBarrier{Version: IdentityVersion, EventID: id, AccountID: a.ID, PrincipalID: principal, Generation: a.Generation, FromEpoch: p.FromEpoch, ToEpoch: p.ToEpoch, Status: "pending", VerifiedAtMS: p.VerifiedAtMS, BeganAtMS: now, BeginSequence: seq}
	batch := new(leveldb.Batch)
	if put(batch, "account:"+a.ID, a) != nil || put(batch, "identity:"+a.ID, state) != nil || put(batch, "barrier:"+id, r) != nil {
		return ResetBarrier{}, false, ErrCorrupt
	}
	if err = s.event(batch, &next, "reset-begin", a, now, id); err != nil {
		return ResetBarrier{}, false, err
	}
	next.ReservedEvents++
	if err = s.commitMutation(ctx, batch, next); err != nil {
		return ResetBarrier{}, false, err
	}
	return r, true, nil
}

// CompleteReset records the original fixed hold once. Completed-event replays,
// including after later resets, return the original immutable receipt. Pending
// resets have no timeout-based unlock; uncertain outcomes require reconciliation.
func (s *Store) CompleteReset(ctx context.Context, p VerifiedReset) (ResetBarrier, bool, error) {
	if err := s.lock(ctx); err != nil {
		return ResetBarrier{}, false, err
	}
	defer s.unlock()
	if err := s.identityMode(); err != nil {
		return ResetBarrier{}, false, err
	}
	if p.VerifierConfigHash != s.meta.Identity.VerifierConfigHash {
		return ResetBarrier{}, false, ErrUnauthorized
	}
	principal, err := PrincipalID(p.Issuer, p.Subject)
	if err != nil {
		return ResetBarrier{}, false, ErrUnauthorized
	}
	if _, err = s.fixture(principal); err != nil {
		return ResetBarrier{}, false, err
	}
	now, err := s.observe()
	if err != nil {
		return ResetBarrier{}, false, err
	}
	principal, id, err := s.validateResetInput(p, "complete", now)
	if err != nil {
		return ResetBarrier{}, false, err
	}
	var r ResetBarrier
	if err = s.load("barrier:"+id, &r); err != nil {
		return ResetBarrier{}, false, err
	}
	if s.validateBarrier(r) != nil || r.EventID != id {
		return ResetBarrier{}, false, ErrCorrupt
	}
	if !resetBinding(r, principal, p) {
		return ResetBarrier{}, false, ErrConflict
	}
	if r.Status == "completed" {
		return r, false, nil
	}
	a, err := s.account(r.AccountID)
	if err != nil {
		return ResetBarrier{}, false, err
	}
	state, err := s.identityState(a)
	if err != nil {
		return ResetBarrier{}, false, err
	}
	if r.Status != "pending" || state.PendingEventID != id || a.Generation != r.Generation || state.CredentialEpoch != r.FromEpoch || s.meta.ReservedEvents == 0 {
		return ResetBarrier{}, false, ErrCorrupt
	}
	next := s.meta
	seq, err := advanceIdentity(&next)
	if err != nil {
		return ResetBarrier{}, false, err
	}
	next.ReservedEvents--
	until := now + RecoveryHoldMS
	if until < a.HoldUntilMS {
		until = a.HoldUntilMS
	}
	a.HoldUntilMS = until
	a.RecoveryRequired = a.Enrollment != nil
	state.CredentialEpoch = r.ToEpoch
	state.PendingEventID = ""
	state.LastResetSequence = seq
	r.Status = "completed"
	r.CompletedAtMS = now
	r.HoldUntilMS = until
	r.CompleteSequence = seq
	batch := new(leveldb.Batch)
	if put(batch, "account:"+a.ID, a) != nil || put(batch, "identity:"+a.ID, state) != nil || put(batch, "barrier:"+id, r) != nil {
		return ResetBarrier{}, false, ErrCorrupt
	}
	if err = s.event(batch, &next, "reset-complete", a, now, id); err != nil {
		return ResetBarrier{}, false, err
	}
	if err = s.commitMutation(ctx, batch, next); err != nil {
		return ResetBarrier{}, false, err
	}
	return r, true, nil
}

// GetLoginAttempt returns only the public persisted nonce challenge. It never
// grants authority; consumption and all reset checks occur in LoginWithAssertion.
// Its clock observation is synchronized even when the attempt has expired.
func (s *Store) GetLoginAttempt(ctx context.Context, id string) (LoginAttempt, error) {
	if err := s.lock(ctx); err != nil {
		return LoginAttempt{}, err
	}
	defer s.unlock()
	if err := s.identityMode(); err != nil {
		return LoginAttempt{}, err
	}
	if !validID(id) {
		return LoginAttempt{}, ErrUnauthorized
	}
	var record loginAttemptRecord
	if s.load("attempt:"+id, &record) != nil {
		return LoginAttempt{}, ErrUnauthorized
	}
	if s.validateAttempt(record) != nil || record.Attempt.ID != id {
		return LoginAttempt{}, ErrCorrupt
	}
	if record.ConsumedAtMS != 0 {
		return LoginAttempt{}, ErrUnauthorized
	}
	now, err := s.observe()
	if err != nil {
		return LoginAttempt{}, err
	}
	if now >= record.Attempt.ExpiresAtMS {
		return LoginAttempt{}, ErrExpired
	}
	return record.Attempt, nil
}
