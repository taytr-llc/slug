// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

type hostedPoolOwner struct {
	allocation  *hostedAllocationOwner
	init        HostedInit
	ready       HostedReady
	hadReady    bool
	prefix      []SetupReceipt
	options     pilot.PoolOptions
	coordinator *gymprovider.Coordinator
	driver      *hostedFixtureDriver
	vault       *recoveryvault.HostedVault
	current     *recoveryvault.CurrentStore
	setupEpoch  string
	failedChild poolChild
}

// Failed constructor cleanup remains owned under its exact path. A replacement
// cannot bypass the retained instance lock or discard a still-active loader.
var hostedFailedOwners struct {
	sync.Mutex
	roots map[string]*PoolController
}

func hostedIdentityConfig(verifier string) gymaccount.IdentityConfig {
	principals := []gymaccount.FixturePrincipal{{Issuer: gymprovider.FixtureIssuer, Subject: "zero256", Fixture: "zero256"}, {Issuer: gymprovider.FixtureIssuer, Subject: "incrementing256", Fixture: "incrementing256"}}
	sort.Slice(principals, func(i, j int) bool {
		a, _ := gymaccount.PrincipalID(principals[i].Issuer, principals[i].Subject)
		b, _ := gymaccount.PrincipalID(principals[j].Issuer, principals[j].Subject)
		return a < b
	})
	return gymaccount.IdentityConfig{Version: 2, Accounts: gymaccount.Config{Version: 1, Principals: principals, MaxSessions: 4096, MaxChallenges: 4096, MaxEvents: 16384}, VerifierConfigHash: verifier, MaxLoginAttempts: 1024}
}
func hostedVaultConfig() recoveryvault.Config {
	return recoveryvault.Config{Version: 1, WrapperID: hostedWrapperID, RecoveryKeyRef: "fixture-recovery-v1", GymKeyRef: "fixture-gym-v1", AllowedRecoveryKeyRefs: []string{"fixture-recovery-v1"}, AllowedGymKeyRefs: []string{"fixture-gym-v1"}, MaxOperations: 128, Policy: pilot.HostedPoolPolicy()}
}
func NewHostedFixturePool(ctx context.Context, config pilot.Config, options pilot.PoolOptions) (*PoolController, error) {
	return newHostedFixturePool(ctx, config, options, false)
}
func NewHostedRecoveryPool(ctx context.Context, config pilot.Config, options pilot.PoolOptions) (*PoolController, error) {
	return newHostedFixturePool(ctx, config, options, true)
}
func newHostedFixturePool(ctx context.Context, config pilot.Config, options pilot.PoolOptions, allocation bool) (_ *PoolController, resultErr error) {
	options, e := pilot.ResolvePoolOptions(options)
	if e != nil {
		return nil, e
	}
	if wire.TestPoW || ctx == nil || ctx.Err() != nil || !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, pilot.ErrPoolRuntime
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	openRoot := openHostedRoot
	if allocation {
		openRoot = openHostedAllocationRoot
	}
	lock, e := openRoot(config.DataDir, options)
	if e != nil {
		return nil, e
	}
	c := &PoolController{config: config, lock: lock, state: "signed_out", hosted: &hostedPoolOwner{options: options}}
	defer func() {
		if resultErr != nil {
			closeErr := c.Close()
			resultErr = errors.Join(resultErr, closeErr)
			if closeErr != nil {
				hostedFailedOwners.Lock()
				if hostedFailedOwners.roots == nil {
					hostedFailedOwners.roots = make(map[string]*PoolController)
				}
				hostedFailedOwners.roots[config.DataDir] = c
				hostedFailedOwners.Unlock()
			}
		}
	}()
	h := c.hosted
	if allocation {
		h.allocation = &hostedAllocationOwner{}
	}
	v, e := gymidentity.NewVerifier(gymprovider.FixtureVerifierConfig())
	if e != nil {
		return nil, e
	}
	h.init, _, e = readHostedInit(config.DataDir)
	if e != nil {
		return nil, e
	}
	if allocation && h.init.Version == 0 {
		guard, exists, e := readHostedAllocationGuard(config.DataDir)
		if e != nil {
			return nil, e
		}
		if exists {
			h.init = guard.HostedInit
		}
	}
	if h.init.Version == 0 {
		ids := make([]string, 5)
		for i := range ids {
			ids[i], e = hostedRandomID(rand.Reader)
			if e != nil {
				return nil, e
			}
		}
		pc, e := hostedProviderConfig(ids[0])
		if e != nil {
			return nil, e
		}
		h.init = HostedInit{Version: 1, Mode: pilot.HostedPoolMode, DescriptorID: options.CandidateDescriptorID, AuthorityID: ids[0], CoordinatorID: ids[1], HostedVaultID: ids[2], ProviderDriverID: hostedDriverID, WrapperID: hostedWrapperID, VerifierConfigHash: v.ConfigDigest(), IdentityConfigSHA256: hostedCanonicalHash(hostedIdentityConfig(v.ConfigDigest())), ProviderConfigSHA256: hostedCanonicalHash(pc), VaultConfigSHA256: hostedCanonicalHash(hostedVaultConfig()), Wallets: []WalletOwnerPin{{"zero256", ids[3]}, {"incrementing256", ids[4]}}}
		if allocation {
			if e = c.prepareHostedAllocationGuard(); e != nil {
				return nil, e
			}
		}
		if e = writeHostedInit(config.DataDir, h.init); e != nil {
			return nil, e
		}
	}
	if allocation {
		if e = c.prepareHostedAllocationGuard(); e != nil {
			return nil, e
		}
		if _, exists, e := readHostedInit(config.DataDir); e != nil {
			return nil, e
		} else if !exists {
			if e = writeHostedInit(config.DataDir, h.init); e != nil {
				return nil, e
			}
		}
	}
	pc, e := hostedProviderConfig(h.init.AuthorityID)
	if e != nil {
		return nil, e
	}
	ic := hostedIdentityConfig(v.ConfigDigest())
	if h.init.DescriptorID != options.CandidateDescriptorID || h.init.VerifierConfigHash != v.ConfigDigest() || h.init.IdentityConfigSHA256 != hostedCanonicalHash(ic) || h.init.ProviderConfigSHA256 != hostedCanonicalHash(pc) || h.init.VaultConfigSHA256 != hostedCanonicalHash(hostedVaultConfig()) {
		return nil, pilot.ErrManifestInitialization
	}
	h.prefix, e = readHostedSetupPrefixMode(config.DataDir, h.init, allocation)
	if e != nil {
		return nil, e
	}
	h.ready, h.hadReady, e = readHostedReady(config.DataDir)
	if e != nil {
		return nil, e
	}
	if h.hadReady && !hostedReadyMatches(h.init, h.ready) {
		return nil, pilot.ErrManifestInitialization
	}
	path := filepath.Join(config.DataDir, "authority")
	exists, e := hostedComponentExists(path)
	if e != nil {
		return nil, e
	}
	if exists {
		c.store, e = gymaccount.OpenIdentityPinned(path, ic, h.init.AuthorityID)
	} else {
		c.store, e = gymaccount.CreateIdentityPinned(path, ic, h.init.AuthorityID)
	}
	if e != nil {
		return nil, e
	}
	if e = c.hostedStage("authority", hostedInitialStageHash(h.init, "authority")); e != nil {
		return nil, e
	}
	path = filepath.Join(config.DataDir, "coordinator")
	exists, e = hostedComponentExists(path)
	if e != nil {
		return nil, e
	}
	if exists {
		h.coordinator, e = gymprovider.OpenPinned(ctx, path, pc, c.store, v, h.init.CoordinatorID)
	} else {
		h.coordinator, e = gymprovider.CreatePinned(ctx, path, pc, c.store, v, h.init.CoordinatorID)
	}
	if e != nil {
		return nil, e
	}
	if e = c.hostedStage("coordinator", hostedInitialStageHash(h.init, "coordinator")); e != nil {
		return nil, e
	}
	h.driver, e = openHostedDriver(ctx, filepath.Join(config.DataDir, "fixture-provider"), pc, c.store, h.coordinator, len(h.prefix) <= 2)
	if e != nil {
		return nil, e
	}
	if e = c.hostedStage("driver", hostedInitialStageHash(h.init, "driver")); e != nil {
		return nil, e
	}
	if pending, e := c.hostedPending(ctx); e != nil {
		return nil, e
	} else if pending != "" {
		c.fixture = pending
		c.lastError = "hosted_reset_pending"
		return c, nil
	}
	if e = c.resumeHostedSetup(ctx); e != nil {
		return nil, e
	}
	return c, nil
}
func hostedComponentExists(path string) (bool, error) {
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, pilot.ErrManifestInitialization
	}
	return true, nil
}
func (c *PoolController) hostedStage(stage, identity string) error {
	h := c.hosted
	want := SetupReceipt{Version: 1, Stage: stage, InitSHA256: hostedCanonicalHash(h.init), IdentitySHA256: identity}
	index := -1
	for i, s := range hostedStages {
		if s == stage {
			index = i
		}
	}
	if index < 0 || !hostedHash(identity) || index > len(h.prefix) {
		return pilot.ErrManifestInitialization
	}
	if index < len(h.prefix) {
		if h.prefix[index] != want {
			return pilot.ErrManifestInitialization
		}
		return nil
	}
	if h.hadReady {
		return pilot.ErrManifestInitialization
	}
	if e := writeHostedSetupReceiptMode(c.config.DataDir, want, h.allocation != nil); e != nil {
		return e
	}
	h.prefix = append(h.prefix, want)
	return nil
}
func (c *PoolController) hostedPending(ctx context.Context) (string, error) {
	for _, f := range hostedFixtures {
		_, e := c.hosted.driver.PendingReset(ctx, f)
		if e == nil {
			return f, nil
		}
		if !errors.Is(e, gymprovider.ErrNotFound) {
			return "", e
		}
	}
	return "", nil
}
func (c *PoolController) hostedSetupFence(epoch string) recoveryvault.EpochFence {
	return func(ctx context.Context, fn func(func() error) error) error {
		if ctx == nil || fn == nil || !c.mu.TryLock() {
			return gymaccount.ErrUnauthorized
		}
		defer c.mu.Unlock()
		check := func() error {
			if ctx.Err() != nil || c.closed || c.hosted == nil || epoch == "" || c.hosted.setupEpoch != epoch || c.shutdownErr != nil {
				return gymaccount.ErrUnauthorized
			}
			return nil
		}
		if e := check(); e != nil {
			return e
		}
		if e := fn(check); e != nil {
			return e
		}
		return check()
	}
}
func (c *PoolController) hostedViewFence(session gymaccount.Session, view string, child func() poolChild) recoveryvault.EpochFence {
	return func(ctx context.Context, fn func(func() error) error) error {
		if ctx == nil || fn == nil || !c.mu.TryLock() {
			return gymaccount.ErrUnauthorized
		}
		defer c.mu.Unlock()
		check := func() error {
			expected := child()
			if ctx.Err() != nil || c.closed || c.shutdownErr != nil || c.hosted == nil || view == "" || c.viewID != view || c.session.Token != session.Token || c.session.Account.ID != session.Account.ID || c.session.Account.Generation != session.Account.Generation || expected != nil && c.child != expected {
				return gymaccount.ErrUnauthorized
			}
			return nil
		}
		if e := check(); e != nil {
			return e
		}
		if e := fn(check); e != nil {
			return e
		}
		return check()
	}
}
func (c *PoolController) hostedSignedLogin(ctx context.Context, fixture string) (gymaccount.Session, int64, error) {
	if _, e := c.hosted.driver.PendingReset(ctx, fixture); e == nil {
		return gymaccount.Session{}, 0, errHostedPending
	} else if !errors.Is(e, gymprovider.ErrNotFound) {
		return gymaccount.Session{}, 0, e
	}
	flow, e := c.hosted.coordinator.IssueLogin(ctx)
	if e != nil {
		return gymaccount.Session{}, 0, e
	}
	token, e := c.hosted.driver.FreshLogin(ctx, flow, fixture)
	if e != nil {
		return gymaccount.Session{}, 0, e
	}
	session, e := c.hosted.coordinator.FinishLogin(ctx, flow.ID, token)
	token = ""
	if e != nil {
		return gymaccount.Session{}, 0, e
	}
	c.hosted.driver.mu.Lock()
	at := int64(0)
	for _, row := range c.hosted.driver.state.Auth {
		if row.Flow == flow && row.Fixture == fixture {
			at = row.AuthenticatedAtMS
		}
	}
	c.hosted.driver.mu.Unlock()
	if at == 0 {
		_ = c.store.Logout(context.Background(), session.Token)
		return gymaccount.Session{}, 0, gymprovider.ErrCorrupt
	}
	return session, at, nil
}
func (c *PoolController) resumeHostedSetup(ctx context.Context) (resultErr error) {
	h := c.hosted
	epoch, e := hostedRandomID(rand.Reader)
	if e != nil {
		return e
	}
	c.mu.Lock()
	h.setupEpoch = epoch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); h.setupEpoch = ""; c.mu.Unlock() }()
	fence := c.hostedSetupFence(epoch)
	if h.allocation != nil && h.allocation.setup == nil {
		pins := make([]pilot.HostedInventoryPin, 0, 2)
		for _, p := range h.allocation.guard.Inventories {
			pins = append(pins, pilot.HostedInventoryPin{Fixture: p.Fixture, InventoryID: p.InventoryID})
		}
		completed := []string{}
		for _, r := range h.prefix {
			if r.Stage == "fixture-zero256" {
				completed = append(completed, "zero256")
			}
			if r.Stage == "fixture-incrementing256" {
				completed = append(completed, "incrementing256")
			}
		}
		h.allocation.setup, e = pilot.OpenHostedAllocationSetup(ctx, filepath.Join(c.config.DataDir, "allocation-recovery"), c.store, hostedCanonicalHash(h.allocation.guard), pins, h.hadReady, completed)
		if e != nil {
			return e
		}
	}
	path := filepath.Join(c.config.DataDir, "hosted-vault")
	if h.vault == nil {
		exists, e := hostedComponentExists(path)
		if e != nil {
			return e
		}
		if exists {
			h.vault, e = recoveryvault.OpenHosted(ctx, path, hostedVaultConfig(), c.store, hostedFixtureWrapper{}, h.init.HostedVaultID)
		} else {
			h.vault, e = recoveryvault.CreateHosted(ctx, path, hostedVaultConfig(), c.store, hostedFixtureWrapper{}, h.init.HostedVaultID)
		}
		if e != nil {
			return e
		}
	}
	if e = c.hostedStage("vault", hostedInitialStageHash(h.init, "vault")); e != nil {
		return e
	}
	if h.allocation != nil && h.allocation.setup.Ready() {
		if e = c.auditHostedAllocationReady(ctx); e != nil {
			return e
		}
	}
	if h.allocation != nil && h.hadReady {
		missing, e := c.classifyHostedWallets()
		if e != nil {
			return e
		}
		if missing {
			return c.openHostedRecoveryArchive(ctx)
		}
	}
	if c.runtime == nil {
		config := pilot.Config{DataDir: filepath.Join(c.config.DataDir, "shared"), NodeBinary: c.config.NodeBinary}
		if h.allocation != nil {
			c.runtime, e = pilot.NewHostedRecoveryPoolRuntime(ctx, config, c.store, h.options, h.vault, h.init.Wallets[0].WalletOwnerID, h.init.Wallets[1].WalletOwnerID, h.allocation.setup)
		} else {
			c.runtime, e = pilot.NewHostedPoolRuntime(ctx, config, c.store, h.options, h.vault, h.init.Wallets[0].WalletOwnerID, h.init.Wallets[1].WalletOwnerID)
		}
		if e != nil {
			return e
		}
	}
	c.sourceID, e = poolservice.BindingID(c.runtime.SourceConfig())
	if e != nil {
		return e
	}
	runtimeHash := hostedCanonicalHash(struct {
		Source     string `json:"source_binding_id"`
		Descriptor string `json:"descriptor_id"`
		Authority  string `json:"authority_id"`
	}{c.sourceID, h.init.DescriptorID, h.init.AuthorityID})
	if e = c.hostedStage("runtime", runtimeHash); e != nil {
		return e
	}
	bindings := make([]recoveryvault.CurrentBinding, 0, 2)
	for _, fixture := range hostedFixtures {
		session, _, e := c.hostedSignedLogin(ctx, fixture)
		if e != nil {
			if errors.Is(e, errHostedPending) {
				c.mu.Lock()
				c.fixture = fixture
				c.mu.Unlock()
			}
			return e
		}
		_, e = c.runtime.ProvisionHostedFixture(ctx, fixture, session, fence)
		logout := c.store.Logout(context.Background(), session.Token)
		if e = errors.Join(e, logout); e != nil {
			return e
		}
		binding, e := c.runtime.HostedBinding(fixture)
		if e != nil {
			return e
		}
		bindings = append(bindings, binding)
		if e = c.hostedStage("fixture-"+fixture, hostedCanonicalHash(binding)); e != nil {
			return e
		}
	}
	path = filepath.Join(c.config.DataDir, "current-recovery")
	if h.current == nil {
		exists, e := hostedComponentExists(path)
		if e != nil {
			return e
		}
		if exists {
			h.current, e = recoveryvault.OpenCurrentStore(ctx, path, h.vault, bindings)
		} else {
			h.current, e = recoveryvault.CreateCurrentStore(ctx, path, h.vault, bindings)
		}
		if e != nil {
			return e
		}
	}
	if e = c.runtime.AttachHostedCurrent(h.current, bindings); e != nil {
		return e
	}
	currentHash := hostedCanonicalHash(struct {
		Version  uint32                         `json:"version"`
		Bindings []recoveryvault.CurrentBinding `json:"bindings"`
	}{1, bindings})
	if e = c.hostedStage("current", currentHash); e != nil {
		return e
	}
	if h.allocation == nil && !c.runtime.Ready() {
		if e = c.runtime.StartServices(ctx); e != nil {
			return e
		}
	}
	if h.allocation != nil {
		if e = c.runtime.PrepareHostedRecoveryServices(ctx); e != nil {
			return e
		}
		if e = h.allocation.setup.PublishReady(c.sourceID); e != nil {
			return e
		}
	}
	ready := HostedReady{Version: 1, InitSHA256: hostedCanonicalHash(h.init), AuthorityID: h.init.AuthorityID, CoordinatorID: h.init.CoordinatorID, HostedVaultID: h.init.HostedVaultID, SourceBindingID: c.sourceID, Fixtures: []HostedFixtureReady{{"zero256", bindings[0]}, {"incrementing256", bindings[1]}}}
	if h.hadReady {
		if !reflect.DeepEqual(h.ready, ready) {
			return pilot.ErrManifestInitialization
		}
	} else {
		if e = writeHostedReadyMode(c.config.DataDir, ready, h.allocation != nil); e != nil {
			return e
		}
		h.ready, h.hadReady = ready, true
	}
	if h.allocation != nil && !c.runtime.Ready() {
		if e = c.runtime.StartServices(ctx); e != nil {
			return e
		}
	}
	c.newChild = func(ctx context.Context, r *pilot.PoolRuntime, a pilot.StableAccountConfig) (poolChild, error) {
		return pilot.NewPoolChild(ctx, r, a)
	}
	return nil
}
func (c *PoolController) loginHosted(ctx context.Context, fixture string) error {
	if _, e := c.hosted.driver.PendingReset(ctx, fixture); e == nil {
		c.mu.Lock()
		c.fixture = fixture
		c.mu.Unlock()
		return errHostedPending
	} else if !errors.Is(e, gymprovider.ErrNotFound) {
		return e
	}
	if c.hostedAllocationMode() && c.hosted.allocation.archive != nil {
		session, _, e := c.hostedSignedLogin(ctx, fixture)
		if e != nil {
			return e
		}
		if !c.hosted.allocation.missing[fixture] {
			c.mu.Lock()
			c.fixture = fixture
			c.mu.Unlock()
			return errors.Join(pilot.ErrHostedRecoveryUnavailable, c.store.Logout(context.Background(), session.Token))
		}
		return c.installRecoverySession(ctx, fixture, session)
	}
	if c.runtime == nil || !c.runtime.Ready() {
		if e := c.resumeHostedSetup(ctx); e != nil {
			return e
		}
	}
	if c.hostedAllocationMode() && c.hosted.allocation.archive != nil {
		return c.loginHosted(ctx, fixture)
	}
	session, at, e := c.hostedSignedLogin(ctx, fixture)
	if e != nil {
		return e
	}
	return c.installSession(ctx, fixture, session, at)
}
func (c *PoolController) resetHosted(ctx context.Context, fixture string) (resultErr error) {
	defer func() {
		if resultErr != nil {
			if _, e := c.hosted.driver.PendingReset(context.Background(), fixture); e == nil {
				resultErr = errors.Join(resultErr, errHostedPending)
			}
		}
	}()
	pending, e := c.hosted.driver.PendingReset(ctx, fixture)
	var request gymprovider.ResetRequest
	if e == nil {
		request = pending.Request
	} else if errors.Is(e, gymprovider.ErrNotFound) {
		request, e = c.hosted.coordinator.IssueReset(ctx)
	}
	var ack gymprovider.ResetResult
	if e == nil {
		var token string
		token, e = c.hosted.driver.BeginResetEvidence(ctx, request, fixture)
		if e == nil {
			ack, e = c.hosted.coordinator.BeginReset(ctx, request.EventID, token)
		}
	}
	cleanup := c.detach(context.Background())
	if e = errors.Join(e, cleanup); e != nil {
		return e
	}
	// A completed coordinator replay may outlive a lost final driver observation.
	if ack.Phase == "completed" {
		if e = c.hosted.driver.ObserveResetComplete(ctx, request, ack); e != nil {
			return e
		}
	} else {
		if e = c.hosted.driver.CommitReset(ctx, request, ack); e != nil {
			return e
		}
		token, e := c.hosted.driver.CompleteResetEvidence(ctx, request)
		if e != nil {
			return e
		}
		ack, e = c.hosted.coordinator.CompleteReset(ctx, request.EventID, token)
		if e != nil {
			return e
		}
		if e = c.hosted.driver.ObserveResetComplete(ctx, request, ack); e != nil {
			return e
		}
	}
	return c.loginHosted(ctx, fixture)
}
func (h *hostedPoolOwner) close() error {
	if h == nil {
		return nil
	}
	var e error
	if h.allocation != nil {
		if h.allocation.failedRecovery != nil {
			if e = h.allocation.failedRecovery.Close(); e != nil {
				return e
			}
		}
		if h.allocation.recovery != nil {
			if e = h.allocation.recovery.Close(); e != nil {
				return e
			}
		}
		if h.allocation.archive != nil {
			if e = h.allocation.archive.Close(); e != nil {
				return e
			}
		}
		if h.allocation.setup != nil {
			if e = h.allocation.setup.Close(); e != nil {
				return e
			}
		}
	}
	if h.current != nil {
		e = h.current.Close()
		if e != nil {
			return e
		}
	}
	if h.vault != nil {
		e = h.vault.Close()
		if e != nil {
			return e
		}
	}
	if h.driver != nil {
		e = h.driver.Close()
		if e != nil {
			return e
		}
	}
	if h.coordinator != nil {
		return h.coordinator.Close()
	}
	return nil
}
