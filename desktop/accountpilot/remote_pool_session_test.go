// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slug/wire/v2"
)

// Real signed fixture/account owners, but no wallet or attachment is invented.
// These tests qualify the parent epoch/gate only; the external probe owns the
// separate actual schema14/node/TLS Snapshot+Start acceptance.
func remoteControllerSignedAccount(t *testing.T) *RemotePoolController {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal signed fixture account")
	}
	root := filepath.Join(remoteControllerTestParent(t), "account")
	lock, e := openOwnedRoot(root)
	if e != nil {
		t.Fatal(e)
	}
	life, cancel := context.WithCancel(context.Background())
	c := &RemotePoolController{root: root, lock: lock, life: life, cancel: cancel, state: "signing_in", fixture: "zero256"}
	t.Cleanup(func() {
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	})
	c.rootInfo, e = os.Lstat(root)
	if e != nil {
		t.Fatal(e)
	}
	c.lockInfo, e = os.Lstat(filepath.Join(root, "instance-lock"))
	if e != nil {
		t.Fatal(e)
	}
	verifier, e := gymidentity.NewVerifier(gymprovider.FixtureVerifierConfig())
	if e != nil {
		t.Fatal(e)
	}
	authority := strings.Repeat("1", 64)
	c.accounts, e = gymaccount.CreateIdentityPinned(filepath.Join(root, "authority"), hostedIdentityConfig(verifier.ConfigDigest()), authority)
	if e != nil {
		t.Fatal(e)
	}
	pc, e := hostedProviderConfig(authority)
	if e != nil {
		t.Fatal(e)
	}
	c.coordinator, e = gymprovider.CreatePinned(context.Background(), filepath.Join(root, "coordinator"), pc, c.accounts, verifier, strings.Repeat("2", 64))
	if e != nil {
		t.Fatal(e)
	}
	c.driver, e = openHostedDriver(context.Background(), filepath.Join(root, "fixture-provider"), pc, c.accounts, c.coordinator, true)
	if e != nil {
		t.Fatal(e)
	}
	c.session, c.authenticatedAtMS, e = signedOwnedFixtureLogin(context.Background(), c.accounts, c.coordinator, c.driver, "zero256")
	if e != nil {
		t.Fatal(e)
	}
	if c.session.Kind != "login" || c.session.Token == "" || c.authenticatedAtMS <= 0 {
		t.Fatal("not the original signed login")
	}
	c.viewID = strings.Repeat("3", 64)
	c.state = "signed_in"
	return c
}
func remoteControllerCheckView(c *RemotePoolController, view string) error {
	return c.withView(context.Background(), view, nil, func(_ gymaccount.Account, check func() error) error { return check() })
}
func TestRemotePoolOriginalSignedAccountEpoch(t *testing.T) {
	c := remoteControllerSignedAccount(t)
	login, view := c.session, c.viewID
	if e := remoteControllerCheckView(c, view); e != nil {
		t.Fatal(e)
	}
	epoch := c.epoch(login, view)
	if e := epoch(context.Background(), func(check func() error) error { return check() }); e != nil {
		t.Fatal(e)
	}
	c.mu.Lock()
	c.viewID = strings.Repeat("4", 64)
	c.mu.Unlock()
	if e := epoch(context.Background(), func(check func() error) error { return check() }); e == nil {
		t.Fatal("old epoch adopted replacement view")
	}
	c.mu.Lock()
	c.viewID = view
	c.mu.Unlock()
	if e := c.accounts.Logout(context.Background(), login.Token); e != nil {
		t.Fatal(e)
	}
	if e := remoteControllerCheckView(c, view); e == nil {
		t.Fatal("original signed login survived revocation")
	}
}
func TestRemotePoolAdmissionContextDoesNotEndOwner(t *testing.T) {
	c := remoteControllerSignedAccount(t)
	view := c.viewID
	ctx, cancel := context.WithCancel(context.Background())
	work, done, e := c.admit(ctx, "")
	if e != nil {
		cancel()
		t.Fatal(e)
	}
	cancel()
	done()
	if work.Err() == nil || c.life.Err() != nil {
		t.Fatal("admission and retained owner lifetime conflated")
	}
	if e = remoteControllerCheckView(c, view); e != nil {
		t.Fatal("original actual account lost after admission joined", e)
	}
}
func TestRemotePoolExpiryInvalidatesAndJoinsAdmittedAction(t *testing.T) {
	c := remoteControllerSignedAccount(t)
	login, view := c.session, c.viewID
	work, done, e := c.admit(context.Background(), "")
	if e != nil {
		t.Fatal(e)
	}
	joined := false
	defer func() {
		if !joined {
			done()
		}
	}()
	expired := make(chan struct{})
	go func() { c.expire(login, view, nil); close(expired) }()
	// Every test exit releases the admitted operation before joining expiry.
	defer func() {
		if !joined {
			done()
			joined = true
		}
		<-expired
	}()
	select {
	case <-work.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("expiry did not cancel original operation")
	}
	c.mu.Lock()
	valid := c.viewID == "" && c.expiring
	c.mu.Unlock()
	if !valid {
		t.Fatal("expiry did not invalidate before join")
	}
	select {
	case <-expired:
		t.Fatal("expiry returned before original action joined")
	default:
	}
	done()
	joined = true
	select {
	case <-expired:
	case <-time.After(2 * time.Second):
		t.Fatal("expiry did not join")
	}
	if e = remoteControllerCheckView(c, view); e == nil {
		t.Fatal("expired view authorized")
	}
}
func TestRemotePoolLogoutAdmissionInvalidatesBeforeExpiry(t *testing.T) {
	c := remoteControllerSignedAccount(t)
	login, view := c.session, c.viewID
	// A public Logout additionally requires a real attachment; this control
	// exercises the atomic transition with the same concrete original account.
	_, done, e := c.admitMode(context.Background(), "", true)
	if e != nil {
		t.Fatal(e)
	}
	defer done()
	finished := make(chan struct{})
	go func() { c.expire(login, view, nil); close(finished) }()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("expiry waited for an operation after logout invalidation")
	}
	c.mu.Lock()
	valid := c.viewID == "" && c.state == "signing_out"
	c.mu.Unlock()
	if !valid {
		t.Fatal("logout admission left original view live")
	}
}
