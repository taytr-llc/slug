// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
)

func TestPoolChildSignerReleasesLeaseAndDiscardsOriginalAfterReset(t *testing.T) {
	h := newPoolDesktopHarness(t)
	entered, release := make(chan struct{}), make(chan struct{})
	signer := poolChildSigner(h.authority, func(ctx context.Context, _ prospect.Context, _ prospect.Proof) (gymauth.Authorization, error) {
		if h.sessions.active.Load() {
			t.Error("wallet signing held account lease")
		}
		close(entered)
		<-release
		return gymauth.Authorization{PublicKey: h.config.PublicKey}, nil
	})
	done := make(chan error, 1)
	go func() {
		auth, e := signer(t.Context(), prospect.Context{}, prospect.Proof{})
		if auth.PublicKey != "" && e != nil {
			t.Error("revoked signature escaped")
		}
		done <- e
	}()
	<-entered
	// A replacement token does not authorize the in-flight old-token result.
	h.sessions.mu.Lock()
	h.sessions.tokens[strings.Repeat("1", 64)] = false
	h.sessions.mu.Unlock()
	h.mu.Lock()
	h.token = strings.Repeat("2", 64)
	h.mu.Unlock()
	close(release)
	if e := <-done; e == nil {
		t.Fatal("reset during signing accepted replacement token")
	}
}

type poolChildBlockedWorker struct {
	entered, cancelled, release chan struct{}
	attempts                    uint64
}

func (w *poolChildBlockedWorker) Run(ctx context.Context) error {
	close(w.entered)
	<-ctx.Done()
	close(w.cancelled)
	<-w.release
	return nil
}
func (w *poolChildBlockedWorker) Status() prospectworker.Status {
	return prospectworker.Status{State: "mining", Attempts: w.attempts}
}

func poolChildLifecycleFixture(t *testing.T) (*PoolChild, *registry.ManifestStore, *poolDesktopHarness, *atomic.Int32) {
	t.Helper()
	h := newPoolDesktopHarness(t)
	root := t.TempDir()
	issuer, e := registry.CreateManifestAuthority(filepath.Join(root, "issuer"), policy().Network, genesis, manifestDescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = issuer.Close() })
	info, e := issuer.Info()
	if e != nil {
		t.Fatal(e)
	}
	identity, _ := gymaccount.FixtureIdentity("zero256")
	pin := gymstable.Pin{Version: gymstable.SharedPinVersion2, AccountAuthorityID: h.account.Account.AuthorityID, AccountID: h.account.Account.AccountID, Identity: identity, Source: info.Pin, ConditionVersion: fightercondition.Version1, ProjectorID: fightercondition.ProjectorID}
	store, e := gymstable.CreateShared(filepath.Join(root, "stable"), pin, issuer)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	base := &ManifestController{config: Config{DataDir: root}, account: &h.account.Account, view: h.account, manifest: manifestIdentity{Phase: "ready", AuthorityPin: info.Pin, Identity: walletIdentity{RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID}}, cache: make(map[uint64]*manifestCached), snapshot: BlankManifestSnapshot()}
	lease := func(ctx context.Context, _ gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		return h.account.ViewLease(ctx, func(a gymaccount.Account, check func() error) error { a.Enrollment = &identity; return fn(a, check) })
	}
	s := &StableController{ManifestController: base, stableConfig: StableAccountConfig{Manifest: h.account, Lease: lease}, sharedPin: &pin, sharedSource: issuer, laneGate: make(chan struct{}, 1), wake: make(chan struct{}, 1)}
	base.stable = s
	if e = s.openPair(nil); e != nil {
		t.Fatal(e)
	}
	status, e := s.stableStore.Status(t.Context(), s.session())
	if e != nil {
		t.Fatal(e)
	}
	s.stableStatus = &status
	s.installed = true
	ctx, cancel := context.WithCancel(context.Background())
	released := new(atomic.Int32)
	fence, e := NewPoolSessionAuthority(h.config, h.account)
	if e != nil {
		t.Fatal(e)
	}
	c := &PoolChild{stable: s, account: s.stableConfig, authority: h.authority, admission: &poolChildAdmission{fence: fence}, ctx: ctx, cancel: cancel, closeDone: make(chan struct{}), refreshDone: make(chan struct{}), prepared: true, snapshot: BlankPoolSnapshot(), resources: poolChildResources{Release: func() { released.Add(1) }, WorkerConfig: prospectworker.PoolConfig{MaxRecords: 1000}}}
	c.signer = func(context.Context) (prospectworker.Signer, error) {
		return func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
			return gymauth.Authorization{}, nil
		}, nil
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, issuer, h, released
}
func newPoolChildBlockedWorker(n uint64) *poolChildBlockedWorker {
	return &poolChildBlockedWorker{entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{}), attempts: n}
}
func TestPoolChildStopRestartAndCloseJoinOwnedRun(t *testing.T) {
	c, issuer, _, released := poolChildLifecycleFixture(t)
	first, second := newPoolChildBlockedWorker(3), newPoolChildBlockedWorker(2)
	var calls int
	c.newWorker = func(prospectworker.Signer) (poolChildWorker, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return second, nil
	}
	if e := c.Start(); e != nil {
		t.Fatal(e)
	}
	<-first.entered
	c.mu.Lock()
	c.copyWorkerLocked(c.run, prospectworker.Status{State: "hashing", Attempts: 3, LastError: "not_found"})
	c.mu.Unlock()
	if got := c.Snapshot(); got.State != "running" || !PoolStatusCode(got.LastError) {
		t.Fatal("worker status cannot cross the Pool protocol", got.State, got.LastError)
	}
	stopped := make(chan struct{})
	go func() { c.Stop(); close(stopped) }()
	<-first.cancelled
	select {
	case <-stopped:
		t.Fatal("Stop returned before in-flight work drained")
	case <-time.After(20 * time.Millisecond):
	}
	if released.Load() != 0 {
		t.Fatal("Stop released parent resource")
	}
	close(first.release)
	<-stopped
	if got := c.Snapshot(); got.Attempts != 3 || got.State != "stopped" {
		t.Fatal("stopped accounting", got.Attempts, got.State)
	}
	if _, e := issuer.Info(); e != nil {
		t.Fatal("Stop closed shared issuer", e)
	}
	if e := c.Start(); e != nil {
		t.Fatal(e)
	}
	<-second.entered
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	<-second.cancelled
	select {
	case <-closed:
		t.Fatal("Close returned before work drained")
	case <-time.After(20 * time.Millisecond):
	}
	if released.Load() != 0 {
		t.Fatal("parent borrow released while worker active")
	}
	close(second.release)
	if e := <-closed; e != nil {
		t.Fatal(e)
	}
	if released.Load() != 1 {
		t.Fatal("borrow not released exactly once")
	}
	if _, e := issuer.Info(); e != nil {
		t.Fatal("child closed borrowed issuer", e)
	}
	if e := c.Close(); e != nil || released.Load() != 1 {
		t.Fatal("repeated Close changed ownership", e)
	}
	if got := c.Snapshot(); got.View != nil || got.GymID != "" || got.Attempts != 0 {
		t.Fatal("closed child republished stale data")
	}
}
func TestPoolChildStaleStartDoesNotConstructWorker(t *testing.T) {
	c, _, h, _ := poolChildLifecycleFixture(t)
	var calls atomic.Int32
	c.newWorker = func(prospectworker.Signer) (poolChildWorker, error) {
		calls.Add(1)
		return nil, errors.New("should not execute")
	}
	h.mu.Lock()
	h.view = false
	h.mu.Unlock()
	if e := c.Start(); e == nil || calls.Load() != 0 {
		t.Fatal("stale original view started mining", e)
	}
}

func TestPoolChildTimedOutBalancePublishesOnlyFailureToOriginalView(t *testing.T) {
	c, _, h, _ := poolChildLifecycleFixture(t)
	h.hook = func(_ http.ResponseWriter, request *http.Request) { <-request.Context().Done() }
	c.snapshot.Balance = PoolBalanceSnapshot{State: "ready", Value: &h.response}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := c.RefreshPoolBalance(ctx); err == nil || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("blocked balance did not time out", err)
	}
	got := c.Snapshot().Balance
	if got.State != "failed" || got.Error != "balance_unavailable" || got.Value != nil {
		t.Fatal("timeout retained a stale balance payload", got.State, got.Error)
	}
}

func TestPoolChildCancelledBalanceCannotPublishIntoRevokedView(t *testing.T) {
	for _, revoke := range []string{"view", "child"} {
		t.Run(revoke, func(t *testing.T) {
			c, _, h, _ := poolChildLifecycleFixture(t)
			entered := make(chan struct{})
			h.hook = func(_ http.ResponseWriter, request *http.Request) {
				close(entered)
				<-request.Context().Done()
			}
			c.snapshot.Balance = PoolBalanceSnapshot{State: "ready", Value: &h.response}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- c.RefreshPoolBalance(ctx) }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("balance request did not start")
			}
			if revoke == "view" {
				h.mu.Lock()
				h.view = false
				h.mu.Unlock()
			} else {
				c.cancel()
			}
			cancel()
			if err := <-done; err == nil {
				t.Fatal("cancelled request succeeded")
			}
			got := c.Snapshot().Balance
			if got.State != "ready" || got.Value == nil {
				t.Fatal("failure published after original view or child was revoked", got.State)
			}
		})
	}
}

func TestPoolChildSharedReopenNeverOpensSoloSource(t *testing.T) {
	c, issuer, _, _ := poolChildLifecycleFixture(t)
	s := c.stable
	if e := s.closePair(); e != nil {
		t.Fatal(e)
	}
	if e := s.openPair(nil); e != nil {
		t.Fatal("shared reopen attempted absent solo source", e)
	}
	if s.source != issuer || s.ownedSource || s.stableStatus.Pin.Version != gymstable.SharedPinVersion2 {
		t.Fatal("shared source ownership changed")
	}
}

func TestPoolChildStorageResumesIntentButDoesNotReplaceReadyStable(t *testing.T) {
	c, issuer, h, _ := poolChildLifecycleFixture(t)
	info, e := issuer.Info()
	if e != nil {
		t.Fatal(e)
	}
	source := h.config.Source
	source.AuthorityPin = info.Pin
	poolTrust, e := poolservice.TransportTrustID(poolservice.ClientConfig{Version: 1, URL: h.config.URL, Token: h.token, PublicKey: h.config.PublicKey, Source: source})
	if e != nil {
		t.Fatal(e)
	}
	admissionTrust, e := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: h.config.URL, Token: h.token, AuthorityPin: info.Pin})
	if e != nil {
		t.Fatal(e)
	}
	r := poolChildResources{Config: Config{DataDir: t.TempDir()}, Issuer: issuer, StablePin: *c.stable.sharedPin, WorkerConfig: prospectworker.PoolConfig{Version: 1, Source: source, PublicKey: h.config.PublicKey, AuthorityOrigin: h.config.URL, TransportTrustID: poolTrust, AdmissionOrigin: h.config.URL, AdmissionTrustID: admissionTrust, MaxRecords: 1000, MaxBytes: 256 << 20}}
	box, e := openPoolChildOutbox(r)
	if e != nil {
		t.Fatal(e)
	}
	if e = box.Close(); e != nil {
		t.Fatal(e)
	}
	ready := filepath.Join(r.Config.DataDir, "SLUG-POOL-CHILD-READY.json")
	before, e := os.ReadFile(ready)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(ready); e != nil {
		t.Fatal(e)
	}
	box, e = openPoolChildOutbox(r)
	if e != nil {
		t.Fatal("interrupted child install did not resume", e)
	}
	_ = box.Close()
	after, e := os.ReadFile(ready)
	if e != nil || string(after) != string(before) {
		t.Fatal("resumed child changed immutable binding", e)
	}
	path := filepath.Join(r.Config.DataDir, "stable")
	if e = os.Rename(path, path+"-retained"); e != nil {
		t.Fatal(e)
	}
	if box, e = openPoolChildOutbox(r); !errors.Is(e, ErrManifestInitialization) || box != nil {
		t.Fatal("missing established stable replaced", e)
	}
	if _, e = os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("ready stable recreated")
	}
}
