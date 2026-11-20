// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slug/wire/v2"
)

func ownedPendingRoot(t *testing.T) (string, ownedGymInit, gymprovider.ResetRequest) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal signed fixture identity")
	}
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(parent, "owned")
	lock, e := openOwnedRoot(root)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	options, e := pilot.ResolvePoolOptions(pilot.PoolOptions{})
	if e != nil {
		t.Fatal(e)
	}
	verifier, e := gymidentity.NewVerifier(gymprovider.FixtureVerifierConfig())
	if e != nil {
		t.Fatal(e)
	}
	init, e := newOwnedInit(options, verifier.ConfigDigest())
	if e != nil {
		t.Fatal(e)
	}
	if e = ownedWrite(filepath.Join(root, ownedInitName), init); e != nil {
		t.Fatal(e)
	}
	h := init.Hosted
	pc, e := hostedProviderConfig(h.AuthorityID)
	if e != nil {
		t.Fatal(e)
	}
	accounts, e := gymaccount.CreateIdentityPinned(filepath.Join(root, "authority"), hostedIdentityConfig(verifier.ConfigDigest()), h.AuthorityID)
	if e != nil {
		t.Fatal(e)
	}
	defer accounts.Close()
	c := &OwnedGymController{config: pilot.Config{DataDir: root}, init: init, accounts: accounts, state: "signed_out"}
	if e = c.stage("authority", hostedInitialStageHash(h, "authority")); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	coordinator, e := gymprovider.CreatePinned(ctx, filepath.Join(root, "coordinator"), pc, accounts, verifier, h.CoordinatorID)
	if e != nil {
		t.Fatal(e)
	}
	defer coordinator.Close()
	c.coordinator = coordinator
	if e = c.stage("coordinator", hostedInitialStageHash(h, "coordinator")); e != nil {
		t.Fatal(e)
	}
	driver, e := openHostedDriver(ctx, filepath.Join(root, "fixture-provider"), pc, accounts, coordinator, true)
	if e != nil {
		t.Fatal(e)
	}
	defer driver.Close()
	c.driver = driver
	if e = c.stage("driver", hostedInitialStageHash(h, "driver")); e != nil {
		t.Fatal(e)
	}
	login, _, e := c.signedLogin(ctx, "zero256")
	if e != nil {
		t.Fatal(e)
	}
	view := strings.Repeat("c", 64)
	c.session = login
	c.viewID = view
	c.state = "signed_in"
	read := func() error {
		return c.withCapability(ctx, login, view, nil, gymaccount.View, func(_ gymaccount.Account, check func() error) error { return check() })
	}
	if e = read(); e != nil {
		t.Fatal("original view", e)
	}
	c.viewID = strings.Repeat("d", 64)
	if e = read(); e == nil {
		t.Fatal("replacement view authorized old callback")
	}
	c.viewID = view
	request, e := coordinator.IssueReset(ctx)
	if e != nil {
		t.Fatal(e)
	}
	evidence, e := driver.BeginResetEvidence(ctx, request, "zero256")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = coordinator.BeginReset(ctx, request.EventID, evidence); e != nil {
		t.Fatal(e)
	}
	if e = read(); e == nil {
		t.Fatal("reset barrier did not revoke original view")
	}
	return root, init, request
}
func TestOwnedGymPendingStartupIsSignedOutAndExact(t *testing.T) {
	root, init, request := ownedPendingRoot(t)
	c, e := NewOwnedGymPool(context.Background(), pilot.Config{DataDir: root, NodeBinary: "/nonexistent/slugd"}, pilot.PoolOptions{})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	snapshot := c.Snapshot()
	if snapshot.State != "signed_out" || snapshot.PendingResetID != request.EventID || snapshot.Source != init.Source || snapshot.View != nil || snapshot.Child != nil || snapshot.Error != "hosted_reset_pending" {
		t.Fatal("not a usable exact pending startup", snapshot)
	}
	for _, path := range []string{"shared", "hosted-vault"} {
		if _, e = os.Lstat(filepath.Join(root, path)); !os.IsNotExist(e) {
			t.Fatal("pending startup opened later component", path, e)
		}
	}
	if _, e = c.Scope(context.Background(), "zero256"); e == nil {
		t.Fatal("pending startup invented complete scope")
	}
	if e = c.ResumePendingReset(context.Background(), strings.Repeat("a", 64)); e == nil {
		t.Fatal("different reset ID accepted")
	}
}
func TestOwnedGymRootIsolationAndEstablishedPrefix(t *testing.T) {
	root, init, _ := ownedPendingRoot(t)
	if _, e := openHostedRoot(root, pilot.PoolOptions{}); e == nil {
		t.Fatal("legacy hosted root accepted owned namespace")
	}
	prefix, e := readOwnedPrefix(root, init)
	if e != nil || len(prefix) != 3 {
		t.Fatal(e, len(prefix))
	}
	if e = os.Rename(filepath.Join(root, "coordinator"), filepath.Join(filepath.Dir(root), "saved-coordinator")); e != nil {
		t.Fatal(e)
	}
	if _, e = readOwnedPrefix(root, init); e == nil {
		t.Fatal("missing established component accepted")
	}
	if _, e = os.Lstat(filepath.Join(root, "coordinator")); !os.IsNotExist(e) {
		t.Fatal("audit recreated missing store")
	}
}
func TestOwnedGymImmutableInitAndEpoch(t *testing.T) {
	options, e := pilot.ResolvePoolOptions(pilot.PoolOptions{})
	if e != nil {
		t.Fatal(e)
	}
	v, e := newOwnedInit(options, strings.Repeat("1", 64))
	if e != nil {
		t.Fatal(e)
	}
	if !validOwnedInit(v, options) {
		t.Fatal("valid selected init refused")
	}
	changed := v
	changed.Wallets[1].InventoryID = changed.Wallets[0].InventoryID
	if validOwnedInit(changed, options) {
		t.Fatal("duplicate independent identities accepted")
	}
	c := &OwnedGymController{setupEpoch: "epoch"}
	fence := c.setupFence("epoch")
	call := func() error { return fence(context.Background(), func(check func() error) error { return check() }) }
	if e = call(); e != nil {
		t.Fatal(e)
	}
	c.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case e = <-done:
		if e == nil {
			t.Fatal("held parent lock accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("epoch waited for mutex")
	}
	c.mu.Unlock()
	c.setupEpoch = "replacement"
	if e = call(); e == nil {
		t.Fatal("substituted epoch accepted")
	}
}
func TestOwnedGymCloseJoinsParentOperation(t *testing.T) {
	c := &OwnedGymController{state: "signed_out"}
	started := make(chan struct{})
	joined := make(chan struct{})
	if e := c.begin("signing_in", "", func(ctx context.Context) error { close(started); <-ctx.Done(); close(joined); return ctx.Err() }); e != nil {
		t.Fatal(e)
	}
	<-started
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	select {
	case <-joined:
	default:
		t.Fatal("Close returned before original operation joined")
	}
	if !c.closed || c.operation {
		t.Fatal("close state")
	}
	if e := c.LoginFixture("zero256"); e == nil {
		t.Fatal("closed controller reused")
	}
}
func TestOwnedGymControlCanonicalAndCreateOnly(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "record.json")
	value := SetupReceipt{1, "authority", strings.Repeat("a", 64), strings.Repeat("b", 64)}
	if e = ownedWrite(path, value); e != nil {
		t.Fatal(e)
	}
	if e = ownedWrite(path, value); e != nil {
		t.Fatal("exact resync", e)
	}
	changed := value
	changed.IdentitySHA256 = strings.Repeat("c", 64)
	if e = ownedWrite(path, changed); e == nil {
		t.Fatal("immutable record overwritten")
	}
	var got SetupReceipt
	found, e := readHostedControl(path, ownedControlLimit, &got)
	if e != nil || !found || !reflect.DeepEqual(value, got) {
		t.Fatal("original changed", e)
	}
	if e = os.WriteFile(path, []byte(`{"version":1,"version":1}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e = ownedWrite(path, value); !errors.Is(e, pilot.ErrManifestInitialization) {
		t.Fatal("duplicate field repaired", e)
	}
}
