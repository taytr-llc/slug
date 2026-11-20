// Copyright (c) 2026 The SLUG developers. ISC license.

package gymaccount

import "context"

// GetIdentityAccount returns an owned public record for a configured schema-2
// principal. It grants no session or capability and never advances the clock.
// This local coordinator lookup is not an unauthenticated public API.
func (s *Store) GetIdentityAccount(ctx context.Context, issuer, subject string) (Account, error) {
	if err := s.lock(ctx); err != nil {
		return Account{}, err
	}
	defer s.unlock()
	a, _, err := s.lookupIdentityAccount(issuer, subject)
	return copyAccount(a), err
}

func (s *Store) lookupIdentityAccount(issuer, subject string) (Account, identityState, error) {
	if err := s.identityMode(); err != nil {
		return Account{}, identityState{}, err
	}
	principal, err := PrincipalID(issuer, subject)
	if err != nil {
		return Account{}, identityState{}, ErrUnauthorized
	}
	if _, err = s.fixture(principal); err != nil {
		return Account{}, identityState{}, err
	}
	var id string
	if err = s.load("principal:"+principal, &id); err != nil {
		return Account{}, identityState{}, err
	}
	if !validID(id) {
		return Account{}, identityState{}, ErrCorrupt
	}
	a, err := s.account(id)
	if err != nil || a.PrincipalID != principal {
		return Account{}, identityState{}, ErrCorrupt
	}
	state, err := s.identityState(a)
	if err != nil {
		return Account{}, identityState{}, err
	}
	return a, state, nil
}

// GetResetBarrier reads the exact principal-bound event, including a historical
// completed barrier. rawEvent is the original provider event ID, not a barrier
// hash or a request to search for the latest reset. ErrNotFound is returned only
// for an absent configured account or exact barrier; it grants no reset permit.
func (s *Store) GetResetBarrier(ctx context.Context, issuer, subject, rawEvent string) (ResetBarrier, error) {
	if !validID(rawEvent) {
		return ResetBarrier{}, ErrUnauthorized
	}
	if err := s.lock(ctx); err != nil {
		return ResetBarrier{}, err
	}
	defer s.unlock()
	a, state, err := s.lookupIdentityAccount(issuer, subject)
	if err != nil {
		return ResetBarrier{}, err
	}
	id, err := resetID(a.PrincipalID, rawEvent)
	if err != nil {
		return ResetBarrier{}, err
	}
	var barrier ResetBarrier
	if err = s.load("barrier:"+id, &barrier); err != nil {
		return ResetBarrier{}, err
	}
	if s.validateBarrier(barrier) != nil || barrier.EventID != id || barrier.AccountID != a.ID || barrier.PrincipalID != a.PrincipalID {
		return ResetBarrier{}, ErrCorrupt
	}
	if barrier.Status == "pending" {
		if state.PendingEventID != id || a.Generation != barrier.Generation || state.CredentialEpoch != barrier.FromEpoch {
			return ResetBarrier{}, ErrCorrupt
		}
	} else if state.PendingEventID == id {
		return ResetBarrier{}, ErrCorrupt
	}
	return barrier, nil
}
