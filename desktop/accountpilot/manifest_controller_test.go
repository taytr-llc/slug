// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type manifestFakeChild struct {
	config                         pilot.Config
	account                        pilot.ManifestAccountConfig
	key                            *btcec.PrivateKey
	loads, starts, exports, closes atomic.Int32
	once                           sync.Once
	snapshotGate                   func()
	stopGate                       func()
}

func (f *manifestFakeChild) AccountID() string { return f.account.Account.AccountID }
func (f *manifestFakeChild) Prepare(context.Context) error {
	return os.MkdirAll(f.config.DataDir, 0700)
}
func (f *manifestFakeChild) SignAccountChallenge(_ context.Context, c gymaccount.EnrollmentChallenge) (gymaccount.Authorization, error) {
	return gymaccount.SignChallenge(c, f.key)
}
func (f *manifestFakeChild) Snapshot() pilot.ManifestSnapshot {
	if f.snapshotGate != nil {
		f.snapshotGate()
	}
	identity, _ := gymaccount.FixtureIdentity(f.account.Account.Fixture)
	descriptor, _ := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	authority := sha256.Sum256([]byte("public test manifest authority:" + f.AccountID()))
	trust := sha256.Sum256([]byte("public test transport"))
	s := pilot.BlankManifestSnapshot()
	s.WalletState = "ready"
	s.GymID = identity.GymID
	s.GymPublicKey = identity.GymPublicKey
	s.View = &pilot.ManifestViewPin{Version: 1, ViewID: f.account.ViewID, AccountAuthorityID: f.account.Account.AuthorityID, AccountID: f.AccountID(), Generation: f.account.Generation, AuthorityPin: registry.ManifestAuthorityPin{Version: 5, Network: identity.Network, Genesis: identity.Genesis, AuthorityID: hex.EncodeToString(authority[:]), DescriptorID: descriptor}, AuthorityOrigin: "http://127.0.0.1:12345", TransportTrustID: hex.EncodeToString(trust[:])}
	s.Projection.State = "ready"
	s.Projection.CompleteRecent = true
	return s
}
func (f *manifestFakeChild) Start() error { f.starts.Add(1); return nil }
func (f *manifestFakeChild) Stop() {
	if f.stopGate != nil {
		f.stopGate()
	}
}
func (f *manifestFakeChild) Close() error { f.once.Do(func() { f.closes.Add(1) }); return nil }
func (f *manifestFakeChild) LoadManifestProspects(ctx context.Context) error {
	return f.account.ViewLease(ctx, func(a gymaccount.Account, recheck func() error) error {
		if a.Enrollment == nil || a.ID != f.AccountID() {
			return gymaccount.ErrForbidden
		}
		if err := recheck(); err != nil {
			return err
		}
		f.loads.Add(1)
		return nil
	})
}
func (f *manifestFakeChild) RefreshManifestProspects(ctx context.Context) error {
	return f.LoadManifestProspects(ctx)
}
func (f *manifestFakeChild) ManifestPortrait(ctx context.Context, _ uint64) (pilot.ManifestPortrait, error) {
	return pilot.ManifestPortrait{}, f.account.ViewLease(ctx, func(gymaccount.Account, func() error) error { return pilot.ErrManifestProspect })
}
func (f *manifestFakeChild) ManifestReceipt(ctx context.Context, _ uint64) (pilot.ManifestReceipt, error) {
	return pilot.ManifestReceipt{}, f.account.ViewLease(ctx, func(gymaccount.Account, func() error) error { return pilot.ErrManifestProspect })
}
func (f *manifestFakeChild) ExportManifestReceipt(context.Context, uint64, string, string, string) error {
	return pilot.ErrManifestReceipt
}
func (f *manifestFakeChild) ReleaseManifestReceipt(context.Context, string, string) error { return nil }
func (f *manifestFakeChild) ExportRecoveryChecked(_ context.Context, _ string, guard func() error) error {
	f.exports.Add(1)
	return guard()
}
func (f *manifestFakeChild) VerifyBackup(context.Context, string) error { return nil }

func manifestFixtureKeys(t *testing.T) map[string]*btcec.PrivateKey {
	t.Helper()
	raw, err := os.ReadFile("../../gymaccount/testdata/enrollment_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []struct {
			Name    string `json:"fixture_name"`
			Private string `json:"test_only_private_key"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &f) != nil {
		t.Fatal("fixture decode")
	}
	keys := map[string]*btcec.PrivateKey{}
	for _, row := range f.Cases {
		if keys[row.Name] != nil {
			continue
		}
		b, err := hex.DecodeString(row.Private)
		if err != nil || len(b) != 32 {
			t.Fatal("published fixture key")
		}
		key, _ := btcec.PrivKeyFromBytes(b)
		clear(b)
		keys[row.Name] = key
		t.Cleanup(key.Zero)
	}
	return keys
}
func manifestTestController(t *testing.T) (*ManifestController, *[]*manifestFakeChild) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("fixture authority requires normal profile")
	}
	c, err := NewManifest(pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/public-fixture/slugd"})
	if err != nil {
		t.Fatal(err)
	}
	keys := manifestFixtureKeys(t)
	children := []*manifestFakeChild{}
	c.newChild = func(cfg pilot.Config, a pilot.ManifestAccountConfig) (manifestChild, error) {
		f := &manifestFakeChild{config: cfg, account: a, key: keys[a.Account.Fixture]}
		children = append(children, f)
		return f, nil
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	return c, &children
}
func manifestWaitAccount(t *testing.T, c *ManifestController) {
	t.Helper()
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	if done == nil {
		t.Fatal("no operation")
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("account operation stuck")
	}
}
func manifestLogin(t *testing.T, c *ManifestController, name string) pilot.ManifestSnapshot {
	t.Helper()
	if err := c.LoginFixture(name); err != nil {
		t.Fatal(err)
	}
	manifestWaitAccount(t, c)
	s := c.Snapshot()
	if s.Account.State != "signed_in" || s.View == nil {
		t.Fatalf("login failed: %+v", s.Account)
	}
	return s
}

func TestManifestAccountIdleLoginResetAndIsolation(t *testing.T) {
	c, children := manifestTestController(t)
	a := manifestLogin(t, c, "zero256")
	first := (*children)[0]
	if first.loads.Load() != 1 || first.starts.Load() != 0 || a.Projection.State != "ready" {
		t.Fatal("login did not perform only cold load")
	}
	originalLease := first.account.ViewLease
	if err := c.ResetFixtureAccount(); err != nil {
		t.Fatal(err)
	}
	manifestWaitAccount(t, c)
	held := c.Snapshot()
	if held.View == nil || held.View.ViewID == a.View.ViewID || held.View.AccountID != a.View.AccountID || held.GymID != a.GymID || held.View.AuthorityPin != a.View.AuthorityPin || !held.Account.Held || held.Account.CanExport || held.Account.Generation != a.Account.Generation+1 {
		t.Fatalf("reset identity/hold: %+v", held)
	}
	called := false
	if err := originalLease(context.Background(), func(gymaccount.Account, func() error) error { called = true; return nil }); err == nil || called {
		t.Fatal("old view lease reused new credential")
	}
	if err := c.ExportRecovery(context.Background(), filepath.Join(t.TempDir(), "backup.json")); !errors.Is(err, gymaccount.ErrHeld) {
		t.Fatal("held recovery export", err)
	}
	second := (*children)[1]
	if second.exports.Load() != 0 {
		t.Fatal("held export entered child")
	}
	if err := c.Start(); err != nil {
		t.Fatal("held existing-wallet mining denied", err)
	}
	if second.starts.Load() != 1 {
		t.Fatal("start not explicit")
	}
	if err := c.Logout(); err != nil {
		t.Fatal(err)
	}
	manifestWaitAccount(t, c)
	blank := c.Snapshot()
	if blank.View != nil || blank.GymID != "" || len(blank.Projection.Items) != 0 {
		t.Fatal("logout leaked view")
	}
	b := manifestLogin(t, c, "incrementing256")
	if b.View.AccountID == a.View.AccountID || b.GymID == a.GymID || b.View.AuthorityPin == a.View.AuthorityPin {
		t.Fatal("fixture identities mixed")
	}
	if err := c.ExportManifestReceipt(context.Background(), 1, a.View.ViewID, "", filepath.Join(t.TempDir(), "receipt.json")); !errors.Is(err, gymaccount.ErrUnauthorized) {
		t.Fatal("foreign native selection accepted", err)
	}
}

func TestManifestAccountSnapshotRechecksAfterDetach(t *testing.T) {
	c, children := manifestTestController(t)
	manifestLogin(t, c, "zero256")
	child := (*children)[0]
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	child.snapshotGate = func() { once.Do(func() { close(entered) }); <-release }
	result := make(chan pilot.ManifestSnapshot, 1)
	go func() { result <- c.Snapshot() }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("snapshot not entered")
	}
	if err := c.Logout(); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case s := <-result:
		if s.View != nil || s.GymID != "" || s.AcceptedCount != 0 {
			t.Fatal("stale copy escaped final lease")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("snapshot/detach deadlock")
	}
	manifestWaitAccount(t, c)
	if child.closes.Load() != 1 {
		t.Fatal("child not drained")
	}
}

func TestManifestAccountExpiredViewDrainsAndCannotRefresh(t *testing.T) {
	c, children := manifestTestController(t)
	s := manifestLogin(t, c, "zero256")
	child := (*children)[0]
	c.expireView(s.View.ViewID, child)
	if child.closes.Load() != 1 || c.Snapshot().View != nil {
		t.Fatal("expiry retained disclosed/live child")
	}
	called := false
	if err := child.account.ViewLease(context.Background(), func(gymaccount.Account, func() error) error { called = true; return nil }); err == nil || called {
		t.Fatal("expired epoch accepted")
	}
}

func TestManifestAccountEstablishedMissingDirectoryIsNotReplaced(t *testing.T) {
	c, _ := manifestTestController(t)
	s := manifestLogin(t, c, "zero256")
	if err := c.Logout(); err != nil {
		t.Fatal(err)
	}
	manifestWaitAccount(t, c)
	dir := filepath.Join(c.config.DataDir, "gyms", s.Account.ID)
	if err := os.Rename(dir, dir+"-retained"); err != nil {
		t.Fatal(err)
	}
	if err := c.LoginFixture("zero256"); err != nil {
		t.Fatal(err)
	}
	manifestWaitAccount(t, c)
	if c.Snapshot().Account.State != "failed" {
		t.Fatal("missing established wallet accepted")
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatal("missing established directory replaced")
	}
}

func TestManifestAccountCheapProfileRefusesBeforeStorage(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-build refusal")
	}
	path := filepath.Join(t.TempDir(), "absent")
	if c, err := NewManifest(pilot.Config{DataDir: path, NodeBinary: "/public-fixture/slugd"}); err == nil || c != nil {
		t.Fatal("cheap constructor accepted")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("cheap constructor touched storage")
	}
}

func TestManifestAccountStartCannotRestartDetachedChild(t *testing.T) {
	c, children := manifestTestController(t)
	manifestLogin(t, c, "zero256")
	child := (*children)[0]
	minted, continueStart := make(chan struct{}), make(chan struct{})
	stopped, continueStop := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-continueStop:
		default:
			close(continueStop)
		}
	}()
	defer func() {
		select {
		case <-continueStart:
		default:
			close(continueStart)
		}
	}()
	c.checkpoint = func(stage string) {
		if stage == "start-token" {
			close(minted)
			<-continueStart
		}
	}
	var once sync.Once
	child.stopGate = func() { once.Do(func() { close(stopped) }); <-continueStop }
	result := make(chan error, 1)
	go func() { result <- c.Start() }()
	select {
	case <-minted:
	case <-time.After(2 * time.Second):
		t.Fatal("token not captured")
	}
	if err := c.Logout(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("logout not draining")
	}
	// Logout has invalidated the view, but has not revoked the real token yet.
	close(continueStart)
	select {
	case err := <-result:
		if err == nil || child.starts.Load() != 0 {
			t.Fatal("retiring child restarted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start publication deadlock")
	}
	close(continueStop)
	manifestWaitAccount(t, c)
}
