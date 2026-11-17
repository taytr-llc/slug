// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"context"
	"errors"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
)

func (s *Store) newSession(batch *leveldb.Batch, next *metadata, a Account, kind, parent string, auth, now, expires int64) (Session, error) {
	if next.Sessions >= next.Config.MaxSessions {
		return Session{}, ErrCapacity
	}
	token, err := randomID(s.random)
	if err != nil {
		return Session{}, err
	}
	id, _ := tokenID(token)
	exists, err := s.db.Has([]byte("session:"+id), nil)
	if err != nil {
		return Session{}, ErrCorrupt
	}
	if exists {
		return Session{}, ErrConflict
	}
	record := sessionRecord{AccountID: a.ID, Generation: a.Generation, Kind: kind, Parent: parent, AuthenticatedAtMS: auth, IssuedAtMS: now, ExpiresAtMS: expires}
	if put(batch, "session:"+id, record) != nil {
		return Session{}, ErrCorrupt
	}
	next.Sessions++
	return Session{Token: token, Kind: kind, Account: copyAccount(a), ExpiresAtMS: expires}, nil
}

// Login trusts only a previously verified provider result mapped to one of the
// configured public fixtures. An unacknowledged token cannot be recovered; a
// fresh verified login may create another independently revocable credential.
func (s *Store) Login(ctx context.Context, p Principal) (Session, error) {
	if err := s.lock(ctx); err != nil {
		return Session{}, err
	}
	defer s.unlock()
	if s.meta.Version != Version1 {
		return Session{}, ErrForbidden
	}
	// Reject unknown principals before any clock write.
	id, err := PrincipalID(p.Issuer, p.Subject)
	if err != nil {
		return Session{}, ErrUnauthorized
	}
	if _, err = s.fixture(id); err != nil {
		return Session{}, err
	}
	now, err := s.observe()
	if err != nil {
		return Session{}, err
	}
	if _, err = s.principal(p, now); err != nil {
		return Session{}, err
	}
	var accountID string
	err = s.load("principal:"+id, &accountID)
	batch := new(leveldb.Batch)
	next := s.meta
	var account Account
	switch {
	case errors.Is(err, ErrNotFound):
		if next.Accounts >= 2 {
			return Session{}, ErrCapacity
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
		account = Account{Version: Version1, ID: accountID, PrincipalID: id, Generation: 1}
		next.Accounts++
		if put(batch, "principal:"+id, accountID) != nil || put(batch, "account:"+accountID, account) != nil {
			return Session{}, ErrCorrupt
		}
	case err != nil:
		return Session{}, err
	default:
		account, err = s.account(accountID)
		if err != nil {
			return Session{}, err
		}
	}
	result, err := s.newSession(batch, &next, account, "login", "", p.AuthenticatedAtMS, now, now+SessionLifetimeMS)
	if err != nil {
		return Session{}, err
	}
	if err = s.event(batch, &next, "login", account, now, ""); err != nil {
		return Session{}, err
	}
	if err = s.commitMutation(ctx, batch, next); err != nil {
		return Session{}, err
	}
	return result, nil
}
func (s *Store) loadSession(token string) (string, sessionRecord, Account, error) {
	id, err := tokenID(token)
	if err != nil {
		return "", sessionRecord{}, Account{}, err
	}
	var session sessionRecord
	if err = s.load("session:"+id, &session); err != nil {
		if errors.Is(err, ErrNotFound) {
			err = ErrUnauthorized
		}
		return "", sessionRecord{}, Account{}, err
	}
	account, err := s.account(session.AccountID)
	if err != nil {
		return "", sessionRecord{}, Account{}, err
	}
	if s.validateSession(session, account) != nil {
		return "", sessionRecord{}, Account{}, ErrCorrupt
	}
	return id, session, account, nil
}
func (s *Store) validateSession(session sessionRecord, a Account) error {
	if session.AccountID != a.ID || session.Generation == 0 || session.Generation > a.Generation || session.AuthenticatedAtMS <= 0 || session.AuthenticatedAtMS > session.IssuedAtMS || session.IssuedAtMS > s.meta.ClockMS || session.ExpiresAtMS <= session.IssuedAtMS {
		return ErrCorrupt
	}
	switch session.Kind {
	case "login":
		if session.Parent != "" || session.ExpiresAtMS-session.IssuedAtMS != SessionLifetimeMS {
			return ErrCorrupt
		}
	case "mining":
		if !validID(session.Parent) || session.ExpiresAtMS-session.IssuedAtMS > MiningLifetimeMS {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	return nil
}
func validCapability(c Capability) bool {
	switch c {
	case View, Enrollment, Mining, RosterManage, RecoveryExport, Withdrawal, FighterTransfer, RecoveryAuthority, MarketplacePurchase, MarketplaceListing:
		return true
	}
	return false
}
func heldCapability(c Capability) bool {
	switch c {
	case RecoveryExport, Withdrawal, FighterTransfer, RecoveryAuthority, MarketplacePurchase, MarketplaceListing:
		return true
	}
	return false
}
func (s *Store) authorize(token string, capability Capability) (string, sessionRecord, Account, int64, error) {
	id, session, a, err := s.loadSession(token)
	if err != nil {
		return "", sessionRecord{}, Account{}, 0, err
	}
	now, err := s.observe()
	if err != nil {
		return "", sessionRecord{}, Account{}, 0, err
	}
	// Keep enough unreserved audit capacity to revoke an active authority.
	// Schema 2 reset needs both its begin event and a reserved completion event;
	// even one free slot is insufficient. Schema 1 retains its original rule.
	revocationSlots := uint64(1)
	if s.meta.Version == IdentityVersion {
		revocationSlots = 2
	}
	if uint64(s.meta.Events)+uint64(s.meta.ReservedEvents)+revocationSlots > uint64(s.meta.Config.MaxEvents) {
		return "", sessionRecord{}, Account{}, 0, ErrCapacity
	}
	if err := s.identityAuthorization(a); err != nil {
		return "", sessionRecord{}, Account{}, 0, err
	}
	if session.Revoked || session.Generation != a.Generation {
		return "", sessionRecord{}, Account{}, 0, ErrUnauthorized
	}
	if now >= session.ExpiresAtMS {
		return "", sessionRecord{}, Account{}, 0, ErrExpired
	}
	if !validCapability(capability) {
		return "", sessionRecord{}, Account{}, 0, ErrForbidden
	}
	if session.Kind == "mining" {
		if capability != Mining {
			return "", sessionRecord{}, Account{}, 0, ErrForbidden
		}
		var parent sessionRecord
		if s.load("session:"+session.Parent, &parent) != nil || s.validateSession(parent, a) != nil || parent.Kind != "login" || parent.AccountID != a.ID || parent.Generation != session.Generation || session.ExpiresAtMS > parent.ExpiresAtMS {
			return "", sessionRecord{}, Account{}, 0, ErrCorrupt
		}
		if parent.Revoked || now >= parent.ExpiresAtMS {
			return "", sessionRecord{}, Account{}, 0, ErrUnauthorized
		}
	} else if capability == Mining {
		return "", sessionRecord{}, Account{}, 0, ErrForbidden
	}
	if capability != View && capability != Enrollment && (a.Enrollment == nil || a.RecoveryRequired) {
		return "", sessionRecord{}, Account{}, 0, ErrForbidden
	}
	if heldCapability(capability) {
		if now < a.HoldUntilMS {
			return "", sessionRecord{}, Account{}, 0, ErrHeld
		}
		if now-session.AuthenticatedAtMS > FreshAuthenticationMS {
			return "", sessionRecord{}, Account{}, 0, ErrUnauthorized
		}
	}
	return id, session, a, now, nil
}

// WithSession holds authorization against logout/reset until fn returns. Call
// recheck after slow work, in the final protected commit guard. No account write
// occurs after fn. The callback/recheck must be used synchronously and must not
// reenter Store. This makes account -> registry the sole lock order.
func (s *Store) WithSession(ctx context.Context, token string, capability Capability, fn func(Account, func() error) error) error {
	if fn == nil {
		return ErrForbidden
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	_, _, a, _, err := s.authorize(token, capability)
	if err != nil {
		return err
	}
	var leaseMu sync.Mutex
	active := true
	defer func() { leaseMu.Lock(); active = false; leaseMu.Unlock() }()
	recheck := func() error {
		leaseMu.Lock()
		defer leaseMu.Unlock()
		if !active {
			return ErrClosed
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		_, _, _, _, err := s.authorize(token, capability)
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return fn(copyAccount(a), recheck)
}

func (s *Store) MintMining(ctx context.Context, token string) (Session, error) {
	if err := s.lock(ctx); err != nil {
		return Session{}, err
	}
	defer s.unlock()
	parentID, parent, a, now, err := s.authorize(token, View)
	if err != nil {
		return Session{}, err
	}
	if parent.Kind != "login" || a.Enrollment == nil || a.RecoveryRequired {
		return Session{}, ErrForbidden
	}
	expires := now + MiningLifetimeMS
	if parent.ExpiresAtMS < expires {
		expires = parent.ExpiresAtMS
	}
	batch := new(leveldb.Batch)
	next := s.meta
	result, err := s.newSession(batch, &next, a, "mining", parentID, parent.AuthenticatedAtMS, now, expires)
	if err != nil {
		return Session{}, err
	}
	if err = s.event(batch, &next, "mining-session", a, now, ""); err != nil {
		return Session{}, err
	}
	if err = s.commitMutation(ctx, batch, next); err != nil {
		return Session{}, err
	}
	return result, nil
}
func (s *Store) Logout(ctx context.Context, token string) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	id, session, a, err := s.loadSession(token)
	if err != nil {
		return err
	}
	if session.Kind != "login" {
		return ErrForbidden
	}
	// A known already-revoked login remains idempotently logged out.
	if session.Revoked {
		return nil
	}
	now, err := s.observe()
	if err != nil {
		return err
	}
	session.Revoked = true
	batch := new(leveldb.Batch)
	next := s.meta
	if put(batch, "session:"+id, session) != nil {
		return ErrCorrupt
	}
	if err = s.event(batch, &next, "logout", a, now, ""); err != nil {
		return err
	}
	return s.commitMutation(ctx, batch, next)
}
func (s *Store) Rotate(ctx context.Context, token string) (Session, error) {
	if err := s.lock(ctx); err != nil {
		return Session{}, err
	}
	defer s.unlock()
	id, old, a, now, err := s.authorize(token, View)
	if err != nil {
		return Session{}, err
	}
	if old.Kind != "login" {
		return Session{}, ErrForbidden
	}
	batch := new(leveldb.Batch)
	next := s.meta
	// Rotation is not fresh authentication and cannot extend absolute lifetime.
	result, err := s.newSession(batch, &next, a, "login", "", old.AuthenticatedAtMS, now, now+SessionLifetimeMS)
	if err != nil {
		return Session{}, err
	}
	result.ExpiresAtMS = old.ExpiresAtMS
	newID, _ := tokenID(result.Token)
	rotated := sessionRecord{AccountID: a.ID, Generation: a.Generation, Kind: "login", AuthenticatedAtMS: old.AuthenticatedAtMS, IssuedAtMS: old.IssuedAtMS, ExpiresAtMS: old.ExpiresAtMS}
	if put(batch, "session:"+newID, rotated) != nil {
		return Session{}, ErrCorrupt
	}
	old.Revoked = true
	if put(batch, "session:"+id, old) != nil {
		return Session{}, ErrCorrupt
	}
	if err = s.event(batch, &next, "rotation", a, now, ""); err != nil {
		return Session{}, err
	}
	if err = s.commitMutation(ctx, batch, next); err != nil {
		return Session{}, err
	}
	return result, nil
}
