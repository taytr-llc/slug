// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slugwallet/wallet"
)

type remotePoolLocal struct {
	session  *OwnedWalletSession
	expected RemotePoolLocalBinding
	mu       sync.Mutex
	miningMu sync.Mutex
	closed   bool
	mining   gymaccount.Session
}

func claimRemotePoolLocal(ctx context.Context, s *OwnedWalletSession, expected RemotePoolLocalBinding) (*remotePoolLocal, error) {
	if s == nil || s.entry == nil || s.runtime == nil || s.setup == nil || ctx == nil || !s.opMu.TryLock() {
		return nil, ErrRemotePoolBusy
	}
	defer s.opMu.Unlock()
	if s.remoteClaimed || !s.ready.Load() || !s.entry.config.RequireExisting || s.check(ctx) != nil {
		return nil, ErrRemotePoolUnavailable
	}
	l := &remotePoolLocal{session: s, expected: expected}
	if e := l.identity(); e != nil {
		return nil, e
	}
	s.runtime.mu.Lock()
	attached := s.runtime.closed || s.runtime.backend != nil
	s.runtime.mu.Unlock()
	if attached {
		return nil, ErrRemotePoolUnavailable
	}
	s.remoteClaimed = true
	return l, nil
}
func (l *remotePoolLocal) identity() error {
	if l == nil || l.session == nil {
		return ErrRemotePoolUnavailable
	}
	s, e := l.session, l.expected
	if s.setup == nil || s.runtime == nil || s.entry == nil || e.Version != 1 || e.ManagerSchema != 14 || !s.ready.Load() || s.closed.Load() {
		return ErrRemotePoolUnavailable
	}
	id, err := wallet.AllocationBindingID2(s.intent.Binding)
	if err != nil || id != e.AllocationBindingID || s.intent.BindingID != id || s.intent.Pin.WalletOwnerID != e.WalletOwnerID || s.intent.SourceBindingID != e.OriginalSourceBindingID || s.intent.Source != e.Recipient.Config.Source || s.intent.Pin.Fixture != e.Recipient.Config.Fixture || s.login.Account.ID != e.Recipient.Config.AccountID || s.setup.accounts.AuthorityID() != e.Recipient.Config.AccountAuthorityID || ValidatePoolRecipientBinding(e.Recipient) != nil {
		return ErrRemotePoolChanged
	}
	want := walletIdentity{RootPublicKey: e.Recipient.Config.Identity.RootPublicKey, GymPublicKey: e.Recipient.Config.Identity.GymPublicKey, GymID: e.Recipient.Config.Identity.GymID}
	if s.runtime.Identity() != want {
		return ErrRemotePoolChanged
	}
	return nil
}
func (l *remotePoolLocal) check(ctx context.Context) error {
	if l == nil || ctx == nil || ctx.Err() != nil {
		return ErrRemotePoolUnavailable
	}
	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return ErrRemotePoolUnavailable
	}
	if e := l.identity(); e != nil {
		return e
	}
	return l.session.check(ctx)
}
func (l *remotePoolLocal) withView(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
	if l == nil || fn == nil {
		return ErrRemotePoolUnavailable
	}
	s := l.session
	return s.setup.accounts.WithSession(ctx, s.login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		if a.ID != s.login.Account.ID || a.Generation != s.login.Account.Generation || a.PrincipalID != s.intent.Binding.PrincipalID || check == nil {
			return gymaccount.ErrUnauthorized
		}
		return s.epoch(ctx, func(parent func() error) error {
			if parent == nil {
				return gymaccount.ErrUnauthorized
			}
			guard := func() error {
				l.mu.Lock()
				closed := l.closed
				l.mu.Unlock()
				if closed || s.closed.Load() {
					return gymaccount.ErrUnauthorized
				}
				return errors.Join(ctx.Err(), check(), parent())
			}
			if e := guard(); e != nil {
				return e
			}
			if e := fn(a, guard); e != nil {
				return e
			}
			return guard()
		})
	})
}
func (l *remotePoolLocal) token(ctx context.Context) (string, error) {
	if e := l.check(ctx); e != nil {
		return "", e
	}
	l.miningMu.Lock()
	defer l.miningMu.Unlock()
	l.mu.Lock()
	closed, m := l.closed, l.mining
	l.mu.Unlock()
	if closed {
		return "", gymaccount.ErrUnauthorized
	}
	if m.Token != "" && time.Now().UnixMilli()+30000 < m.ExpiresAtMS {
		return m.Token, nil
	}
	m, e := l.session.setup.accounts.MintMining(ctx, l.session.login.Token)
	if e != nil {
		return "", e
	}
	if m.Account.ID != l.session.login.Account.ID || m.Account.Generation != l.session.login.Account.Generation || ctx.Err() != nil {
		return "", gymaccount.ErrUnauthorized
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return "", gymaccount.ErrUnauthorized
	}
	l.mining = m
	return m.Token, nil
}
func (l *remotePoolLocal) capture(ctx context.Context) (*mininglease.Lease, string, error) {
	token, e := l.token(ctx)
	if e != nil {
		return nil, "", e
	}
	lease, e := mininglease.Capture(ctx, remotePoolMiningSessions{l}, token)
	if e != nil || lease.PublicKey() != l.expected.Recipient.Config.Identity.GymPublicKey {
		return nil, "", gymaccount.ErrUnauthorized
	}
	if e = l.check(ctx); e != nil {
		return nil, "", e
	}
	return lease, token, nil
}

type remotePoolMiningSessions struct{ local *remotePoolLocal }

func (s remotePoolMiningSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	if s.local == nil || s.local.session == nil || fn == nil {
		return gymaccount.ErrUnauthorized
	}
	l := s.local
	w := l.session
	return w.setup.accounts.WithSession(ctx, token, gymaccount.Mining, func(a gymaccount.Account, check func() error) error {
		if a.ID != w.login.Account.ID || a.Generation != w.login.Account.Generation || a.Enrollment == nil || *a.Enrollment != l.expected.Recipient.Config.Identity || a.RecoveryRequired || check == nil {
			return gymaccount.ErrUnauthorized
		}
		return fn(a.Enrollment.GymPublicKey, check)
	})
}
func (l *remotePoolLocal) attach(ctx context.Context, rpc prospectnode.RPCConfig) error {
	s := l.session
	if !s.opMu.TryLock() {
		return ErrRemotePoolBusy
	}
	defer s.opMu.Unlock()
	if e := l.check(ctx); e != nil {
		return e
	}
	if e := s.runtime.Attach(ctx, rpc); e != nil {
		return ErrRemotePoolUnavailable
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if e := l.check(ctx); e != nil {
			return e
		}
		ready, e := s.runtime.ownedStartupSynced(ctx)
		if e != nil {
			return e
		}
		if ready {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	if e := l.auditRecipient(ctx); e != nil {
		return e
	}
	return l.check(ctx)
}
func (l *remotePoolLocal) auditRecipient(ctx context.Context) error {
	s := l.session
	want := ownedWalletReady{1, allocationJSONHash(s.intent), s.intent.BindingID, s.intent.CarrierBindingID, s.intent.WithdrawalBindingID, l.expected.Recipient}
	if e := ownedWalletInventory(s.root); e != nil {
		return e
	}
	if e := readPoolRuntimeRecord(filepath.Join(s.root, ownedWalletReadyName), want); e != nil {
		return e
	}
	lease, token, e := l.capture(ctx)
	if e != nil {
		return e
	}
	// This private ViewID is correlation inside the existing recipient auditor;
	// the actual outer View remains the retained session epoch, not this string.
	a := ManifestAccountConfig{Account: AccountConfig{Fixture: s.intent.Pin.Fixture, AccountID: s.login.Account.ID, AuthorityID: s.setup.accounts.AuthorityID(), RequireExisting: true, Authorizer: remotePoolMiningSessions{l}, MiningToken: func(context.Context) (string, error) { return token, nil }}, ViewID: l.expected.WalletOwnerID, Generation: s.login.Account.Generation, ViewLease: l.withView}
	got, e := auditExistingPoolRecipientBinding(ctx, s.root, l.expected.Recipient.Config, a, &ownedRecipientWallet{s})
	if e != nil {
		return e
	}
	if got != l.expected.Recipient {
		return ErrRemotePoolChanged
	}
	if e = readPoolRuntimeRecord(filepath.Join(s.root, ownedWalletReadyName), want); e != nil {
		return e
	}
	if e = lease.Check(); e != nil {
		return e
	}
	if e = l.check(ctx); e != nil {
		return e
	}
	s.recipient = got
	return nil
}
func (l *remotePoolLocal) close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.closed = true
	l.mining = gymaccount.Session{}
	l.mu.Unlock()
}
