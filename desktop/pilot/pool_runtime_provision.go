// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
)

// ProvisionFixture verifies a concrete retained wallet under one original
// login. It is used for temporary setup sessions and for each visible login;
// it never substitutes a successor session, starts mining, or leaves a wallet
// open for a child. The account parent owns revocation of setup login sessions.
func (r *PoolRuntime) ProvisionFixture(ctx context.Context, fixture string, login gymaccount.Session) (binding PoolRecipientBinding, resultErr error) {
	work, end, e := r.begin(ctx)
	if e != nil {
		return binding, e
	}
	defer end()
	identity, e := gymaccount.FixtureIdentity(fixture)
	if e != nil || login.Kind != "login" || !validAccountID(login.Account.ID) || login.Account.Generation == 0 {
		return binding, ErrPoolRuntime
	}
	r.mu.Lock()
	for b := range r.borrows {
		if b.account == login.Account.ID {
			r.mu.Unlock()
			return binding, ErrPoolRuntimeBusy
		}
	}
	r.verified[fixture] = false
	r.mu.Unlock()
	var account gymaccount.Account
	lease := func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return r.accounts.WithSession(ctx, login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error {
			if a.ID != login.Account.ID || a.Generation != login.Account.Generation || check == nil || check() != nil || work.Err() != nil {
				return gymaccount.ErrUnauthorized
			}
			if e := fn(a, check); e != nil {
				return e
			}
			return check()
		})
	}
	if e = lease(work, func(a gymaccount.Account, _ func() error) error { account = a; return nil }); e != nil {
		return binding, e
	}
	if account.Enrollment != nil && *account.Enrollment != identity {
		return binding, ErrPoolRuntime
	}
	var challenge gymaccount.EnrollmentChallenge
	if account.Enrollment == nil || account.RecoveryRequired {
		action := "enroll"
		if account.RecoveryRequired {
			action = "recover"
		}
		challenge, e = r.accounts.BeginChallenge(work, login.Token, action)
		if e != nil {
			return binding, e
		}
		if challenge.AccountID != account.ID || challenge.Generation != account.Generation || challenge.GymID != identity.GymID || challenge.GymPublicKey != identity.GymPublicKey || challenge.RootPublicKey != identity.RootPublicKey || challenge.AuthorityID != r.marker.AccountAuthorityID {
			return binding, ErrPoolRuntime
		}
	}
	root := filepath.Join(r.config.DataDir, "gyms", account.ID)
	marker := poolRuntimeGym{1, r.marker.AccountAuthorityID, account.ID, fixture}
	readyPath := filepath.Join(root, poolRuntimeGymReadyName)
	rootExists, e := poolRuntimeExists(root)
	if e != nil {
		return binding, e
	}
	if e = lease(work, func(_ gymaccount.Account, _ func() error) error {
		if !rootExists {
			if e := os.Mkdir(root, 0700); e != nil {
				return e
			}
			if e := syncDirectory(filepath.Dir(root)); e != nil {
				return e
			}
		}
		return writePoolRuntimeRecord(filepath.Join(root, poolRuntimeGymName), marker)
	}); e != nil {
		return binding, e
	}
	hasReady, e := poolRuntimeExists(readyPath)
	if e != nil {
		return binding, e
	}
	walletDir := filepath.Join(root, "wallet")
	walletExists, e := poolRuntimeExists(walletDir)
	if e != nil {
		return binding, e
	}
	if !walletExists && (hasReady || account.Enrollment != nil) {
		return binding, ErrPoolRuntime
	}
	w, e := openAccountFixtureWallet(walletDir, fixture, !walletExists)
	if e != nil {
		return binding, e
	}
	walletClosed := false
	defer func() {
		if !walletClosed {
			resultErr = errors.Join(resultErr, w.Close())
		}
	}()
	if e = w.Attach(work, r.nodes.rpcConfig()); e != nil {
		return binding, e
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		view, e := w.Snapshot(work)
		if e == nil && view.Synced {
			break
		}
		select {
		case <-work.Done():
			return binding, work.Err()
		case <-ticker.C:
		}
	}
	if challenge.ChallengeID != "" {
		if e = lease(work, func(_ gymaccount.Account, _ func() error) error { return nil }); e != nil {
			return binding, e
		}
		auth, e := w.SignAccountChallenge(work, r.marker.AccountAuthorityID, challenge)
		if e != nil {
			return binding, e
		}
		if _, _, e = r.accounts.CompleteChallenge(work, login.Token, challenge.ChallengeID, auth); e != nil {
			return binding, e
		}
	}
	mining, e := r.accounts.MintMining(work, login.Token)
	if e != nil {
		return binding, e
	}
	var random [32]byte
	if _, e = rand.Read(random[:]); e != nil {
		return binding, e
	}
	view := ManifestAccountConfig{Account: AccountConfig{Fixture: fixture, AccountID: account.ID, AuthorityID: r.marker.AccountAuthorityID, RequireExisting: hasReady, Authorizer: poolRuntimeSessions{r.accounts}, MiningToken: func(context.Context) (string, error) { return mining.Token, nil }}, ViewID: hex.EncodeToString(random[:]), Generation: account.Generation, ViewLease: lease}
	c := PoolRecipientConfig{Version: 1, AccountAuthorityID: r.marker.AccountAuthorityID, AccountID: account.ID, Fixture: fixture, Identity: identity, Source: r.source}
	binding, e = ensurePoolRecipientBinding(work, root, c, view, w, !hasReady)
	if e != nil {
		return PoolRecipientBinding{}, e
	}
	if e = lease(work, func(gymaccount.Account, func() error) error { return nil }); e != nil {
		return PoolRecipientBinding{}, e
	}
	withdrawal, e := r.ensureWithdrawalScope(work, binding, w)
	if e != nil {
		return PoolRecipientBinding{}, e
	}
	// Successful shutdown is part of provisioning: the child must be able to
	// open this wallet exclusively before the runtime advertises it as ready.
	e = w.Close()
	walletClosed = true
	if e != nil {
		return PoolRecipientBinding{}, e
	}
	if e = lease(work, func(a gymaccount.Account, check func() error) error {
		if a.Enrollment == nil || *a.Enrollment != identity || a.RecoveryRequired {
			return gymaccount.ErrUnauthorized
		}
		if e := writePoolRuntimeRecord(readyPath, binding); e != nil {
			return e
		}
		if e := check(); e != nil {
			return e
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.closed || work.Err() != nil {
			return ErrPoolRuntime
		}
		if previous, ok := r.recipients[fixture]; ok && previous != binding {
			return ErrPoolRuntime
		}
		for name, b := range r.recipients {
			if name != fixture && b.Config.AccountID == account.ID {
				return ErrPoolRuntime
			}
		}
		r.recipients[fixture] = binding
		if r.withdrawals == nil {
			r.withdrawals = make(map[string]poolWithdrawalScope)
		}
		r.withdrawals[fixture] = withdrawal
		if r.gymAccounts == nil {
			r.gymAccounts = make(map[string]string)
		}
		r.gymAccounts[fixture] = account.ID
		r.verified[fixture] = true
		return nil
	}); e != nil {
		return PoolRecipientBinding{}, e
	}
	return binding, nil
}
