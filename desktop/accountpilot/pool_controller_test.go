// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type poolParentChild struct {
	pilot.PoolChildAPI
	a              pilot.ManifestAccountConfig
	source         string
	starts, closes atomic.Int32
	once           sync.Once
	stopGate       func()
	tamper         bool
}

func (f *poolParentChild) AccountID() string { return f.a.Account.AccountID }
func (f *poolParentChild) Prepare(ctx context.Context) error {
	return f.a.ViewLease(ctx, func(_ gymaccount.Account, check func() error) error { return check() })
}
func (f *poolParentChild) Start() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return f.a.ViewLease(ctx, func(_ gymaccount.Account, check func() error) error {
		if e := check(); e != nil {
			return e
		}
		f.starts.Add(1)
		return nil
	})
}
func (f *poolParentChild) Stop() {
	if f.stopGate != nil {
		f.stopGate()
	}
}
func (f *poolParentChild) Close() error { f.once.Do(func() { f.closes.Add(1) }); return nil }
func (f *poolParentChild) Snapshot() pilot.PoolSnapshot {
	identity, _ := gymaccount.FixtureIdentity(f.a.Account.Fixture)
	descriptor, _ := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	pin := registry.ManifestAuthorityPin{Version: 5, Network: identity.Network, Genesis: identity.Genesis, AuthorityID: strings.Repeat("a", 64), DescriptorID: descriptor}
	origin := "http://127.0.0.1:39111"
	trust, _ := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: origin, Token: strings.Repeat("0", 64), AuthorityPin: pin})
	s := pilot.BlankPoolSnapshot()
	s.WalletState = "ready"
	s.GymID = identity.GymID
	s.GymPublicKey = identity.GymPublicKey
	s.SourceBindingID = f.source
	// Published Set58 recipient used only as a transport-shaped test value.
	s.PayoutAddress = "slug1qzmtrqsfuaf6l6kkcsseumq26ukaphfj9s9ptme"
	s.View = &pilot.ManifestViewPin{Version: 1, ViewID: f.a.ViewID, AccountID: f.AccountID(), AccountAuthorityID: f.a.Account.AuthorityID, Generation: f.a.Generation, AuthorityPin: pin, AuthorityOrigin: origin, TransportTrustID: trust}
	if f.tamper {
		s.GymID = strings.Repeat("e", 64)
	}
	return s
}
func (f *poolParentChild) RefreshPoolBalance(ctx context.Context) error { return f.Prepare(ctx) }
func poolParent(t *testing.T) (*PoolController, *[]*poolParentChild) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal account identity")
	}
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(parent, "LocalPoolPilot")
	store, lock, e := openPoolAccountStore(root)
	if e != nil {
		t.Fatal(e)
	}
	c := &PoolController{config: pilot.Config{DataDir: root}, store: store, lock: lock, state: "signed_out", sourceID: strings.Repeat("b", 64)}
	keys := manifestFixtureKeys(t)
	c.provision = func(ctx context.Context, f string, login gymaccount.Session) (pilot.PoolRecipientBinding, error) {
		if login.Account.Enrollment == nil || login.Account.RecoveryRequired {
			action := "enroll"
			if login.Account.RecoveryRequired {
				action = "recover"
			}
			challenge, e := store.BeginChallenge(ctx, login.Token, action)
			if e != nil {
				return pilot.PoolRecipientBinding{}, e
			}
			auth, e := gymaccount.SignChallenge(challenge, keys[f])
			if e != nil {
				return pilot.PoolRecipientBinding{}, e
			}
			if _, _, e = store.CompleteChallenge(ctx, login.Token, challenge.ChallengeID, auth); e != nil {
				return pilot.PoolRecipientBinding{}, e
			}
		}
		return pilot.PoolRecipientBinding{}, nil
	}
	children := []*poolParentChild{}
	c.newChild = func(_ context.Context, _ *pilot.PoolRuntime, a pilot.StableAccountConfig) (poolChild, error) {
		f := &poolParentChild{a: a.Manifest, source: c.sourceID}
		children = append(children, f)
		return f, nil
	}
	t.Cleanup(func() {
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	})
	return c, &children
}
func poolParentWait(t *testing.T, c *PoolController) {
	t.Helper()
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pool parent operation stuck")
	}
}
func poolParentLogin(t *testing.T, c *PoolController, f string) pilot.PoolSnapshot {
	t.Helper()
	if e := c.LoginFixture(f); e != nil {
		t.Fatal(e)
	}
	poolParentWait(t, c)
	s := c.Snapshot()
	if s.Account.State != "signed_in" || s.View == nil {
		t.Fatalf("login: %+v", s)
	}
	return s
}
func TestPoolParentOriginalViewLifecycle(t *testing.T) {
	c, children := poolParent(t)
	a := poolParentLogin(t, c, "zero256")
	first := (*children)[0]
	if first.starts.Load() != 0 {
		t.Fatal("login started mining")
	}
	if e := c.Start(); e != nil || first.starts.Load() != 1 {
		t.Fatal("Start nested a lease or failed", e)
	}
	if e := c.ResetFixtureAccount(); e != nil {
		t.Fatal(e)
	}
	poolParentWait(t, c)
	b := c.Snapshot()
	if b.View == nil || b.View.ViewID == a.View.ViewID || b.Account.Generation != a.Account.Generation+1 || !b.Account.Held || b.Account.CanExport || b.SourceBindingID != a.SourceBindingID || first.closes.Load() != 1 {
		t.Fatal("reset did not retain source and revoke child")
	}
	if e := first.Start(); e == nil {
		t.Fatal("old child restarted after reset")
	}
	if e := c.Logout(); e != nil {
		t.Fatal(e)
	}
	poolParentWait(t, c)
	blank := c.Snapshot()
	if pilot.ValidatePoolSnapshot(blank) != nil || blank.SourceBindingID != "" {
		t.Fatal("logout disclosed source data")
	}
	other := poolParentLogin(t, c, "incrementing256")
	if other.Account.ID == a.Account.ID || other.SourceBindingID != a.SourceBindingID {
		t.Fatal("gym isolation or shared source")
	}
}
func TestPoolParentExpiryDrainsOutsideAccountLeaseThenRelogin(t *testing.T) {
	c, children := poolParent(t)
	a := poolParentLogin(t, c, "zero256")
	first := (*children)[0]
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	first.stopGate = func() { once.Do(func() { close(entered); <-release }) }
	defer close(release)
	done := make(chan struct{})
	go func() { c.expireView(a.View.ViewID, first); close(done) }()
	<-entered
	if s := c.Snapshot(); s.View != nil || s.Account.State != "expired" {
		t.Fatal("expiry exposed draining child")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p, e := verifyFixture(ctx, "incrementing256", false)
	if e != nil {
		t.Fatal(e)
	}
	session, e := c.store.Login(ctx, p)
	if e != nil {
		t.Fatal("child drain held account lease", e)
	}
	_ = c.store.Logout(ctx, session.Token)
	// A second close would panic, so finish the gate with a separately guarded send.
	first.stopGate = nil
	release <- struct{}{}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expiry drain stuck")
	}
	again := poolParentLogin(t, c, "zero256")
	if again.View.ViewID == a.View.ViewID || first.closes.Load() != 1 {
		t.Fatal("expiry did not permit a clean new view")
	}
}
func TestPoolParentRejectsCrossGymCacheAndConstructorFailure(t *testing.T) {
	c, children := poolParent(t)
	poolParentLogin(t, c, "zero256")
	f := (*children)[0]
	f.tamper = true
	if c.Snapshot().View != nil {
		t.Fatal("foreign gym cache disclosed")
	}
	// Explicitly drain the expired view before attempting a failing replacement.
	c.expireView(f.a.ViewID, f)
	c.mu.Lock()
	done, busy := c.done, c.operation
	c.mu.Unlock()
	if busy {
		<-done
	}
	c.newChild = func(context.Context, *pilot.PoolRuntime, pilot.StableAccountConfig) (poolChild, error) {
		return (*poolParentChild)(nil), errors.New("construction failed")
	}
	if e := c.LoginFixture("zero256"); e != nil {
		t.Fatal(e)
	}
	poolParentWait(t, c)
	if c.Snapshot().View != nil {
		t.Fatal("failed constructor published a view")
	}
}
