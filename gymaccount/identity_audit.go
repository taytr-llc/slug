// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import "math"

func (s *Store) validateAttempt(r loginAttemptRecord) error {
	a := r.Attempt
	if a.Version != IdentityVersion || !validID(a.ID) || a.AuthorityID != s.authorityID || !s.knownIssuer(a.Issuer) || !validID(a.Nonce) || a.Sequence == 0 || a.Sequence > s.meta.IdentitySequence || a.IssuedAtMS <= 0 || a.IssuedAtMS > s.meta.ClockMS || a.ExpiresAtMS-a.IssuedAtMS != FreshAuthenticationMS {
		return ErrCorrupt
	}
	if r.ConsumedAtMS == 0 {
		if r.PrincipalID != "" || r.SessionID != "" || r.Generation != 0 {
			return ErrCorrupt
		}
		return nil
	}
	if !validID(r.PrincipalID) || !validID(r.SessionID) || r.Generation == 0 || r.ConsumedAtMS < a.IssuedAtMS || r.ConsumedAtMS >= a.ExpiresAtMS || r.ConsumedAtMS > s.meta.ClockMS {
		return ErrCorrupt
	}
	var accountID string
	if s.load("principal:"+r.PrincipalID, &accountID) != nil {
		return ErrCorrupt
	}
	account, err := s.account(accountID)
	if err != nil || r.Generation > account.Generation {
		return ErrCorrupt
	}
	var session sessionRecord
	if s.load("session:"+r.SessionID, &session) != nil || s.validateSession(session, account) != nil || session.AccountID != accountID || session.Generation != r.Generation || session.Kind != "login" || session.IssuedAtMS != r.ConsumedAtMS {
		return ErrCorrupt
	}
	return nil
}
func (s *Store) validateBarrier(r ResetBarrier) error {
	if r.Version != IdentityVersion || !validID(r.EventID) || !validID(r.AccountID) || !validID(r.PrincipalID) || r.Generation < 2 || r.FromEpoch == 0 || r.FromEpoch == math.MaxUint64 || r.ToEpoch != r.FromEpoch+1 || r.Generation != r.ToEpoch || r.VerifiedAtMS <= 0 || r.VerifiedAtMS > r.BeganAtMS || r.BeganAtMS <= 0 || r.BeganAtMS > s.meta.ClockMS || r.BeginSequence == 0 || r.BeginSequence > s.meta.IdentitySequence {
		return ErrCorrupt
	}
	a, err := s.account(r.AccountID)
	if err != nil || a.PrincipalID != r.PrincipalID || r.Generation > a.Generation {
		return ErrCorrupt
	}
	switch r.Status {
	case "pending":
		if r.CompletedAtMS != 0 || r.HoldUntilMS != 0 || r.CompleteSequence != 0 {
			return ErrCorrupt
		}
	case "completed":
		if r.CompletedAtMS < r.BeganAtMS || r.CompletedAtMS > s.meta.ClockMS || r.HoldUntilMS != r.CompletedAtMS+RecoveryHoldMS || r.CompleteSequence <= r.BeginSequence || r.CompleteSequence > s.meta.IdentitySequence {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	return nil
}
func (s *Store) auditIdentityAccount(a Account, state identityState, barriers map[string]ResetBarrier, completions map[uint64]string) error {
	if state.AccountID != a.ID || state.CredentialEpoch == 0 {
		return ErrCorrupt
	}
	byGeneration := map[uint64]ResetBarrier{}
	for _, r := range barriers {
		if r.AccountID == a.ID {
			if _, exists := byGeneration[r.Generation]; exists {
				return ErrCorrupt
			}
			byGeneration[r.Generation] = r
		}
	}
	if uint64(len(byGeneration))+1 != a.Generation {
		return ErrCorrupt
	}
	epoch := uint64(1)
	lastSequence := uint64(0)
	var hold, lastTime int64
	pending := ""
	for gen := uint64(2); gen <= a.Generation; gen++ {
		r, ok := byGeneration[gen]
		if !ok || r.FromEpoch != epoch || r.BeginSequence <= lastSequence || r.BeganAtMS < lastTime {
			return ErrCorrupt
		}
		if r.Status == "pending" {
			if gen != a.Generation || completions[gen] != "" {
				return ErrCorrupt
			}
			pending = r.EventID
			break
		}
		epoch = r.ToEpoch
		lastSequence = r.CompleteSequence
		hold = r.HoldUntilMS
		lastTime = r.CompletedAtMS
	}
	if state.CredentialEpoch != epoch || state.LastResetSequence != lastSequence || state.PendingEventID != pending || a.HoldUntilMS != hold {
		return ErrCorrupt
	}
	return nil
}
