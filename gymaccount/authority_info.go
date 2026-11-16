// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"context"
	"sync"
)

// AuthorityInfo contains only the immutable public schema2 trust binding. It
// contains no principals, provider assertions, sessions or wallet material.
type AuthorityInfo struct {
	Version            uint32 `json:"version"`
	AuthorityID        string `json:"authority_id"`
	VerifierConfigHash string `json:"verifier_config_hash"`
	Network            uint32 `json:"network"`
	Genesis            string `json:"genesis"`
	Profile            string `json:"profile"`
}

// AuthorityInfo reads a coherent public copy under the store gate. Legacy
// authorities cannot be implicitly relabelled as identity-provider authorities.
func (s *Store) AuthorityInfo(ctx context.Context) (AuthorityInfo, error) {
	if err := s.lock(ctx); err != nil {
		return AuthorityInfo{}, err
	}
	defer s.unlock()
	if err := s.identityMode(); err != nil {
		return AuthorityInfo{}, err
	}
	return AuthorityInfo{Version: IdentityVersion, AuthorityID: s.authorityID, VerifierConfigHash: s.meta.Identity.VerifierConfigHash, Network: s.meta.Network, Genesis: s.meta.Genesis, Profile: s.meta.Profile}, nil
}

// EnrollmentIdentity returns the configured public fixture identity for this
// authenticated account. It creates no challenge and accepts no caller-selected
// root/gym tuple. A coordinator must reauthorize the original session/generation
// before committing a provisioning result after external work.
func (s *Store) EnrollmentIdentity(ctx context.Context, loginToken string) (Account, Identity, error) {
	if err := s.lock(ctx); err != nil {
		return Account{}, Identity{}, err
	}
	defer s.unlock()
	if err := s.identityMode(); err != nil {
		return Account{}, Identity{}, err
	}
	_, session, account, _, err := s.authorize(loginToken, Enrollment)
	if err != nil {
		return Account{}, Identity{}, err
	}
	if session.Kind != "login" {
		return Account{}, Identity{}, ErrForbidden
	}
	identity, err := s.fixture(account.PrincipalID)
	if err != nil {
		return Account{}, Identity{}, err
	}
	return copyAccount(account), identity, nil
}

// WithProvisioning is a schema2-only preparation/publication lease. Unlike gym
// enrollment, full retained-material preparation requires fresh authentication
// and no active recovery hold even before the first enrollment exists. Provider
// work must stay outside the lease. Recheck immediately before a protected commit.
func (s *Store) WithProvisioning(ctx context.Context, loginToken string, fn func(Account, Identity, func() error) error) error {
	if fn == nil {
		return ErrForbidden
	}
	if _, err := s.AuthorityInfo(ctx); err != nil {
		return err
	}
	return s.WithSession(ctx, loginToken, Enrollment, func(_ Account, baseCheck func() error) error {
		if err := s.identityMode(); err != nil {
			return err
		}
		var checkMu sync.Mutex
		active := true
		defer func() { checkMu.Lock(); active = false; checkMu.Unlock() }()
		check := func() (Account, Identity, error) {
			checkMu.Lock()
			defer checkMu.Unlock()
			if !active {
				return Account{}, Identity{}, ErrClosed
			}
			if err := baseCheck(); err != nil {
				return Account{}, Identity{}, err
			}
			_, session, a, now, err := s.authorize(loginToken, Enrollment)
			if err != nil {
				return Account{}, Identity{}, err
			}
			if session.Kind != "login" || now-session.AuthenticatedAtMS > FreshAuthenticationMS {
				return Account{}, Identity{}, ErrUnauthorized
			}
			if now < a.HoldUntilMS {
				return Account{}, Identity{}, ErrHeld
			}
			identity, err := s.fixture(a.PrincipalID)
			return copyAccount(a), identity, err
		}
		a, identity, err := check()
		if err != nil {
			return err
		}
		return fn(a, identity, func() error { _, _, err := check(); return err })
	})
}

// ProvisioningIdentity is the public-value preflight corresponding to the final
// WithProvisioning lease; it never grants a reusable decryption capability.
func (s *Store) ProvisioningIdentity(ctx context.Context, loginToken string) (Account, Identity, error) {
	var account Account
	var identity Identity
	err := s.WithProvisioning(ctx, loginToken, func(a Account, i Identity, _ func() error) error { account = a; identity = i; return nil })
	return account, identity, err
}
