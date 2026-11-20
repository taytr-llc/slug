// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/rand"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

// OwnedGymController is selected explicitly. It never substitutes an owned
// stable into the legacy PoolController or changes an existing root's mode.
type OwnedGymController struct {
	mu                                                            sync.Mutex
	miningMu                                                      sync.Mutex
	config                                                        pilot.Config
	options                                                       pilot.PoolOptions
	lock                                                          *leveldb.DB
	init                                                          ownedGymInit
	ready                                                         ownedGymReady
	hadReady                                                      bool
	prefix                                                        []SetupReceipt
	accounts                                                      *gymaccount.Store
	coordinator                                                   *gymprovider.Coordinator
	driver                                                        *hostedFixtureDriver
	vault                                                         *recoveryvault.HostedVault
	wallets                                                       *pilot.OwnedWalletSetup
	runtime                                                       *pilot.PoolRuntime
	league                                                        *pilot.OwnedLeague
	child                                                         *pilot.OwnedGymChild
	failedChild                                                   *pilot.OwnedGymChild
	failedWallet                                                  *pilot.OwnedWalletSession
	session, mining                                               gymaccount.Session
	fixture, state, lastError, viewID, setupEpoch, pendingResetID string
	operation, closed                                             bool
	cancel                                                        context.CancelFunc
	done                                                          chan struct{}
	expiry                                                        *time.Timer
	expiryDone                                                    chan struct{}
	evidencePermit                                                pilot.EvidencePermit
	shutdownErr                                                   error
	closeOnce                                                     sync.Once
	closeErr                                                      error
}

var ownedFailedOwners struct {
	sync.Mutex
	roots map[string]*OwnedGymController
}

func NewOwnedGymPool(ctx context.Context, config pilot.Config, options pilot.PoolOptions) (_ *OwnedGymController, resultErr error) {
	options, e := pilot.ResolvePoolOptions(options)
	if e != nil {
		return nil, e
	}
	if wire.TestPoW || ctx == nil || ctx.Err() != nil || !filepath.IsAbs(config.NodeBinary) || filepath.Clean(config.NodeBinary) != config.NodeBinary {
		return nil, pilot.ErrPoolRuntime
	}
	work, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	lock, e := openOwnedRoot(config.DataDir)
	if e != nil {
		return nil, e
	}
	c := &OwnedGymController{config: config, options: options, lock: lock, state: "signed_out"}
	defer func() {
		if resultErr != nil {
			ce := c.Close()
			resultErr = errors.Join(resultErr, ce)
			if ce != nil {
				ownedFailedOwners.Lock()
				if ownedFailedOwners.roots == nil {
					ownedFailedOwners.roots = map[string]*OwnedGymController{}
				}
				ownedFailedOwners.roots[config.DataDir] = c
				ownedFailedOwners.Unlock()
			}
		}
	}()
	verifier, e := gymidentity.NewVerifier(gymprovider.FixtureVerifierConfig())
	if e != nil {
		return nil, e
	}
	exists, e := readHostedControl(filepath.Join(config.DataDir, ownedInitName), ownedControlLimit, &c.init)
	if e != nil {
		return nil, e
	}
	if !exists {
		entries, e := hostedDirectoryEntries(config.DataDir, 9)
		if e != nil {
			return nil, e
		}
		for _, de := range entries {
			if de.Name() != "instance-lock" {
				return nil, pilot.ErrManifestInitialization
			}
		}
		c.init, e = newOwnedInit(options, verifier.ConfigDigest())
		if e != nil {
			return nil, e
		}
		if e = ownedWrite(filepath.Join(config.DataDir, ownedInitName), c.init); e != nil {
			return nil, e
		}
	}
	if !validOwnedInit(c.init, options) {
		return nil, pilot.ErrManifestInitialization
	}
	h := c.init.Hosted
	pc, e := hostedProviderConfig(h.AuthorityID)
	if e != nil {
		return nil, e
	}
	ic := hostedIdentityConfig(verifier.ConfigDigest())
	if h.VerifierConfigHash != verifier.ConfigDigest() || h.IdentityConfigSHA256 != hostedCanonicalHash(ic) || h.ProviderConfigSHA256 != hostedCanonicalHash(pc) || h.VaultConfigSHA256 != hostedCanonicalHash(hostedVaultConfig()) {
		return nil, pilot.ErrManifestInitialization
	}
	c.prefix, e = readOwnedPrefix(config.DataDir, c.init)
	if e != nil {
		return nil, e
	}
	c.hadReady, e = readHostedControl(filepath.Join(config.DataDir, ownedReadyName), ownedControlLimit, &c.ready)
	if e != nil {
		return nil, e
	}
	if c.hadReady && (len(c.prefix) != len(ownedStages) || c.ready.Version != 1 || c.ready.InitSHA256 != hostedCanonicalHash(c.init) || !hostedID(c.ready.SourceBindingID) || !hostedHash(c.ready.LeagueSHA256)) {
		return nil, pilot.ErrManifestInitialization
	}
	path, has, e := c.component("authority", 0)
	if e != nil {
		return nil, e
	}
	if has {
		c.accounts, e = gymaccount.OpenIdentityPinned(path, ic, h.AuthorityID)
	} else {
		c.accounts, e = gymaccount.CreateIdentityPinned(path, ic, h.AuthorityID)
	}
	if e != nil {
		return nil, e
	}
	if e = c.stage("authority", hostedInitialStageHash(h, "authority")); e != nil {
		return nil, e
	}
	path, has, e = c.component("coordinator", 1)
	if e != nil {
		return nil, e
	}
	if has {
		c.coordinator, e = gymprovider.OpenPinned(work, path, pc, c.accounts, verifier, h.CoordinatorID)
	} else {
		c.coordinator, e = gymprovider.CreatePinned(work, path, pc, c.accounts, verifier, h.CoordinatorID)
	}
	if e != nil {
		return nil, e
	}
	if e = c.stage("coordinator", hostedInitialStageHash(h, "coordinator")); e != nil {
		return nil, e
	}
	path, _, e = c.component("fixture-provider", 2)
	if e != nil {
		return nil, e
	}
	c.driver, e = openHostedDriver(work, path, pc, c.accounts, c.coordinator, !c.hadReady && len(c.prefix) <= 2)
	if e != nil {
		return nil, e
	}
	if e = c.stage("driver", hostedInitialStageHash(h, "driver")); e != nil {
		return nil, e
	}
	if pending, e := c.pendingReset(work); e != nil {
		return nil, e
	} else if pending != nil {
		c.fixture = pending.Fixture
		c.pendingResetID = pending.Request.EventID
		c.lastError = "hosted_reset_pending"
		return c, nil
	}
	if e = c.resumeSetup(work); e != nil {
		return nil, e
	}
	return c, nil
}
func (c *OwnedGymController) pendingReset(ctx context.Context) (*FixturePendingReset, error) {
	var out *FixturePendingReset
	for _, fixture := range hostedFixtures {
		p, e := c.driver.PendingReset(ctx, fixture)
		if errors.Is(e, gymprovider.ErrNotFound) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if out != nil {
			return nil, pilot.ErrManifestInitialization
		}
		out = p
	}
	return out, nil
}
func (c *OwnedGymController) signedLogin(ctx context.Context, fixture string) (gymaccount.Session, int64, error) {
	if _, e := c.driver.PendingReset(ctx, fixture); e == nil {
		return gymaccount.Session{}, 0, errHostedPending
	} else if !errors.Is(e, gymprovider.ErrNotFound) {
		return gymaccount.Session{}, 0, e
	}
	flow, e := c.coordinator.IssueLogin(ctx)
	if e != nil {
		return gymaccount.Session{}, 0, e
	}
	token, e := c.driver.FreshLogin(ctx, flow, fixture)
	if e != nil {
		return gymaccount.Session{}, 0, e
	}
	session, e := c.coordinator.FinishLogin(ctx, flow.ID, token)
	token = ""
	if e != nil {
		return gymaccount.Session{}, 0, e
	}
	c.driver.mu.Lock()
	at := int64(0)
	for _, row := range c.driver.state.Auth {
		if row.Flow == flow && row.Fixture == fixture {
			at = row.AuthenticatedAtMS
		}
	}
	c.driver.mu.Unlock()
	if at == 0 {
		return gymaccount.Session{}, 0, errors.Join(gymprovider.ErrCorrupt, c.accounts.Logout(context.Background(), session.Token))
	}
	return session, at, nil
}
func (c *OwnedGymController) setupFence(epoch string) recoveryvault.EpochFence {
	return func(ctx context.Context, fn func(func() error) error) error {
		if ctx == nil || fn == nil || !c.mu.TryLock() {
			return gymaccount.ErrUnauthorized
		}
		defer c.mu.Unlock()
		check := func() error {
			if ctx.Err() != nil || c.closed || c.shutdownErr != nil || epoch == "" || c.setupEpoch != epoch {
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
func (c *OwnedGymController) resumeSetup(ctx context.Context) error {
	epoch, e := hostedRandomID(rand.Reader)
	if e != nil {
		return e
	}
	c.mu.Lock()
	c.setupEpoch = epoch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.setupEpoch = ""; c.mu.Unlock() }()
	fence := c.setupFence(epoch)
	if c.vault == nil {
		path, has, e := c.component("hosted-vault", 3)
		if e != nil {
			return e
		}
		if has {
			c.vault, e = recoveryvault.OpenHosted(ctx, path, hostedVaultConfig(), c.accounts, hostedFixtureWrapper{}, c.init.Hosted.HostedVaultID)
		} else {
			c.vault, e = recoveryvault.CreateHosted(ctx, path, hostedVaultConfig(), c.accounts, hostedFixtureWrapper{}, c.init.Hosted.HostedVaultID)
		}
		if e != nil {
			return e
		}
	}
	if e = c.stage("vault", hostedInitialStageHash(c.init.Hosted, "vault")); e != nil {
		return e
	}
	if c.runtime == nil {
		path, _, e := c.component("shared", 4)
		if e != nil {
			return e
		}
		c.runtime, e = pilot.NewOwnedGymRuntime(ctx, pilot.Config{DataDir: path, NodeBinary: c.config.NodeBinary}, c.accounts, c.options, c.init.Source, c.init.Wallets[:])
		if e != nil {
			return e
		}
	}
	source := c.runtime.SourceConfig()
	sourceID, e := poolservice.BindingID(source)
	if e != nil {
		return e
	}
	if e = c.stage("runtime", hostedCanonicalHash(struct {
		Source  pilot.OwnedSourceConfig `json:"source"`
		Binding string                  `json:"source_binding_id"`
	}{c.init.Source, sourceID})); e != nil {
		return e
	}
	if c.wallets == nil {
		fixtures := make([]pilot.OwnedWalletFixture, 0, 2)
		for i, p := range c.init.Wallets {
			root := filepath.Join(c.config.DataDir, "shared", "gyms", p.Fixture)
			has, e := hostedComponentExists(root)
			if e != nil {
				return e
			}
			established := c.hadReady || len(c.prefix) > 5+i
			if !has {
				if established {
					return pilot.ErrManifestInitialization
				}
				if e = ownedMakeDirectory(root); e != nil {
					return e
				}
			}
			fixtures = append(fixtures, pilot.OwnedWalletFixture{Pin: p, ChildRoot: root, RequireExisting: established})
		}
		c.wallets, e = pilot.OpenOwnedWalletSetup(context.Background(), c.accounts, c.vault, pilot.OwnedWalletConfig{Source: source, Ownership: c.init.Source.Ownership, Fixtures: fixtures})
		if e != nil {
			return e
		}
	}
	bindings := [2]recoveryvault.CurrentBinding{}
	for i, fixture := range hostedFixtures {
		login, _, e := c.signedLogin(ctx, fixture)
		if e != nil {
			return e
		}
		current, e := c.enroll(ctx, fixture, &login, fence)
		var session *pilot.OwnedWalletSession
		if e == nil {
			session, e = c.wallets.OpenFixture(ctx, fixture, login, fence, current)
		}
		if e == nil {
			_, e = c.runtime.PrepareOwnedWallet(ctx, session)
		}
		if session != nil {
			ce := session.Close()
			if ce != nil {
				c.failedWallet = session
				c.shutdownErr = errors.Join(c.shutdownErr, ce)
			}
			e = errors.Join(e, ce)
		}
		e = errors.Join(e, c.accounts.Logout(context.Background(), login.Token))
		if e != nil {
			return e
		}
		bindings[i] = current
		if e = c.stage("fixture-"+fixture, hostedCanonicalHash(current)); e != nil {
			return e
		}
	}
	if !c.runtime.Ready() {
		// Owned league factories require a verified complete source head. A new
		// process starts with an unready core even when its stored rows audit.
		if e = c.runtime.PrepareOwnedSource(ctx, c.init.Source); e != nil {
			return e
		}
		if e = c.runtime.StartServices(ctx); e != nil {
			return e
		}
	}
	c.league, e = c.runtime.OwnedLeague()
	if e != nil {
		return e
	}
	leagueHash := hostedCanonicalHash(c.league.Config())
	if e = c.stage("league", leagueHash); e != nil {
		return e
	}
	ready := ownedGymReady{Version: 1, InitSHA256: hostedCanonicalHash(c.init), SourceBindingID: sourceID, Fixtures: bindings, LeagueSHA256: leagueHash}
	if c.hadReady && !reflect.DeepEqual(c.ready, ready) {
		return pilot.ErrManifestInitialization
	}
	if e = ownedWrite(filepath.Join(c.config.DataDir, ownedReadyName), ready); e != nil {
		return e
	}
	c.ready, c.hadReady = ready, true
	return nil
}

// Enrollment recovery reuses the exact signed fixture/vault challenge. Held,
// already enrolled accounts take the verify-only path without provisioning.
func (c *OwnedGymController) enroll(ctx context.Context, fixture string, login *gymaccount.Session, epoch recoveryvault.EpochFence) (recoveryvault.CurrentBinding, error) {
	var zero recoveryvault.CurrentBinding
	identity, e := gymaccount.FixtureIdentity(fixture)
	if e != nil {
		return zero, e
	}
	var account gymaccount.Account
	if e = c.accounts.WithSession(ctx, login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error { account = a; return check() }); e != nil {
		return zero, e
	}
	if account.Enrollment != nil && *account.Enrollment != identity {
		return zero, gymaccount.ErrUnauthorized
	}
	status, e := c.vault.Status(ctx, login.Token, epoch)
	if errors.Is(e, recoveryvault.ErrNotFound) || e == nil && status.Receipt == nil {
		if account.Enrollment != nil {
			return zero, pilot.ErrManifestInitialization
		}
		_, _, e = c.vault.ProvisionFixture(ctx, login.Token, epoch)
	}
	if e != nil {
		return zero, e
	}
	if account.Enrollment == nil || account.RecoveryRequired {
		action := "enroll"
		if account.RecoveryRequired {
			action = "recover"
		}
		challenge, e := c.accounts.BeginChallenge(ctx, login.Token, action)
		if e != nil {
			return zero, e
		}
		auth, e := c.vault.SignAccountChallenge(ctx, login.Token, challenge, epoch)
		if e != nil {
			return zero, e
		}
		if _, _, e = c.accounts.CompleteChallenge(ctx, login.Token, challenge.ChallengeID, auth); e != nil {
			return zero, e
		}
	}
	receipt, e := c.vault.EnsureEnrollment(ctx, login.Token, epoch)
	if e != nil {
		return zero, e
	}
	if receipt.Binding.AccountID != login.Account.ID || receipt.Binding.AuthorityID != c.accounts.AuthorityID() || receipt.Binding.RootPublicKey != identity.RootPublicKey || receipt.Binding.GymPublicKey != identity.GymPublicKey || receipt.Binding.GymID != identity.GymID {
		return zero, pilot.ErrManifestInitialization
	}
	if e = c.accounts.WithSession(ctx, login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error { login.Account = a; return check() }); e != nil {
		return zero, e
	}
	sourceID, e := poolservice.BindingID(c.runtime.SourceConfig())
	if e != nil {
		return zero, e
	}
	owner := ""
	for _, p := range c.init.Wallets {
		if p.Fixture == fixture {
			owner = p.WalletOwnerID
		}
	}
	return recoveryvault.CurrentBinding{Version: 1, HostedVaultID: c.init.Hosted.HostedVaultID, VaultBinding: receipt.Binding, RetentionID: receipt.RetentionID, WalletOwnerID: owner, SourceBindingID: sourceID}, nil
}
