// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

// RemotePoolController owns an existing local account/vault/schema14 session.
// Its remote child has no stable/event authority and is stopped until Start.
type RemotePoolController struct {
	mu                                sync.Mutex
	root                              string
	options                           pilot.PoolOptions
	inputs                            pilot.RemotePoolInputs
	configID                          string
	lock                              *leveldb.DB
	rootInfo, lockInfo                os.FileInfo
	init                              ownedGymInit
	ready                             ownedGymReady
	prefix                            []SetupReceipt
	accounts                          *gymaccount.Store
	coordinator                       *gymprovider.Coordinator
	driver                            *hostedFixtureDriver
	vault                             *recoveryvault.HostedVault
	wallets                           *pilot.OwnedWalletSetup
	wallet                            *pilot.OwnedWalletSession
	attachment                        *pilot.RemotePoolAttachment
	session                           gymaccount.Session
	fixture, state, viewID, lastError string
	authenticatedAtMS                 int64
	life                              context.Context
	cancel                            context.CancelFunc
	operation, expiring, closed       bool
	opCancel                          context.CancelFunc
	opDone                            chan struct{}
	active                            sync.WaitGroup
	expiry                            *time.Timer
	expiryDone                        chan struct{}
	shutdownErr                       error
	closeOnce                         sync.Once
	closeErr                          error
}

// This DTO contains correlation and public account/worker projections only.
// It is not a capability, ownership-history source or a wallet getter.
type RemotePoolControllerSnapshot struct {
	Version  uint32                    `json:"version"`
	ConfigID string                    `json:"config_id"`
	State    string                    `json:"state"`
	Account  pilot.AccountSnapshot     `json:"account"`
	View     *pilot.RemotePoolView     `json:"view,omitempty"`
	Remote   *pilot.RemotePoolSnapshot `json:"remote,omitempty"`
	Error    string                    `json:"error,omitempty"`
}

func OpenRemotePoolController(ctx context.Context, existingRoot string, options pilot.PoolOptions, inputs pilot.RemotePoolInputs) (out *RemotePoolController, resultErr error) {
	if ctx == nil || wire.TestPoW {
		return nil, pilot.ErrRemotePoolConfig
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	options, e := pilot.ResolvePoolOptions(options)
	if e != nil {
		return nil, e
	}
	// Marshal/parse supplies an owned deep copy of all selected public slices.
	raw, e := pilot.MarshalRemotePoolConfig(inputs.Config)
	if e != nil {
		return nil, e
	}
	inputs.Config, e = pilot.ParseRemotePoolConfig(raw)
	if e != nil {
		return nil, e
	}
	id, e := pilot.RemotePoolConfigID(inputs.Config)
	if e != nil {
		return nil, e
	}
	if e = remotePoolRoots(existingRoot, inputs.AttachmentRoot); e != nil {
		return nil, e
	}
	rootInfo, e := os.Lstat(existingRoot)
	if e != nil {
		return nil, e
	}
	lockInfo, e := os.Lstat(filepath.Join(existingRoot, "instance-lock"))
	if e != nil {
		return nil, e
	}
	lock, e := openOwnedRootMode(existingRoot, true)
	if e != nil {
		return nil, e
	}
	// Open ctx bounds admission only. The retained owner ends at Close, not
	// when a successful caller releases its construction timeout.
	life, cancel := context.WithCancel(context.Background())
	c := &RemotePoolController{root: existingRoot, options: options, inputs: inputs, configID: id, lock: lock, rootInfo: rootInfo, lockInfo: lockInfo, life: life, cancel: cancel, fixture: inputs.Config.ExpectedLocal.Recipient.Config.Fixture, state: "signed_out"}
	defer func() {
		if resultErr != nil {
			ce := c.Close()
			resultErr = errors.Join(resultErr, ce)
			if ce != nil {
				out = c
			}
		}
	}()
	work, stop := context.WithTimeout(ctx, 60*time.Second)
	defer stop()
	stopLife := context.AfterFunc(life, stop)
	defer stopLife()
	if e = c.auditControls(); e != nil {
		return nil, e
	}
	verifier, e := gymidentity.NewVerifier(gymprovider.FixtureVerifierConfig())
	if e != nil {
		return nil, e
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
	c.accounts, e = gymaccount.OpenIdentityPinned(filepath.Join(existingRoot, "authority"), ic, h.AuthorityID)
	if e != nil {
		return nil, e
	}
	c.coordinator, e = gymprovider.OpenPinned(work, filepath.Join(existingRoot, "coordinator"), pc, c.accounts, verifier, h.CoordinatorID)
	if e != nil {
		return nil, e
	}
	c.driver, e = openHostedDriver(work, filepath.Join(existingRoot, "fixture-provider"), pc, c.accounts, c.coordinator, false)
	if e != nil {
		return nil, e
	}
	if e = c.refusePendingReset(work); e != nil {
		return nil, e
	}
	if e = requireRemoteHostedVaultMarker(existingRoot); e != nil {
		return nil, e
	}
	c.vault, e = recoveryvault.OpenHosted(work, filepath.Join(existingRoot, "hosted-vault"), hostedVaultConfig(), c.accounts, hostedFixtureWrapper{}, h.HostedVaultID)
	if e != nil {
		return nil, e
	}
	fixtures := make([]pilot.OwnedWalletFixture, 0, 2)
	for _, p := range c.init.Wallets {
		fixtures = append(fixtures, pilot.OwnedWalletFixture{Pin: p, ChildRoot: filepath.Join(existingRoot, "shared", "gyms", p.Fixture), RequireExisting: true})
	}
	c.wallets, e = pilot.OpenOwnedWalletSetup(life, c.accounts, c.vault, pilot.OwnedWalletConfig{Source: inputs.Config.ExpectedLocal.Recipient.Config.Source, Ownership: c.init.Source.Ownership, Fixtures: fixtures})
	if e != nil {
		return nil, e
	}
	if e = c.auditControls(); e != nil {
		return nil, e
	}
	if e = work.Err(); e != nil {
		return nil, e
	}
	return c, nil
}

func (c *RemotePoolController) refusePendingReset(ctx context.Context) error {
	for _, fixture := range hostedFixtures {
		_, e := c.driver.PendingReset(ctx, fixture)
		if e == nil {
			return errHostedPending
		}
		if !errors.Is(e, gymprovider.ErrNotFound) {
			return e
		}
	}
	return nil
}

// admit joins every synchronous API with Close; it never holds the account
// mutex over node, wallet, transport or hashing work.
func (c *RemotePoolController) admit(ctx context.Context, view string) (context.Context, func(), error) {
	return c.admitMode(ctx, view, false)
}

func (c *RemotePoolController) admitMode(ctx context.Context, view string, signingOut bool) (context.Context, func(), error) {
	if c == nil || ctx == nil {
		return nil, nil, gymstable.ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	if e := c.auditRootIdentity(); e != nil {
		return nil, nil, e
	}
	c.mu.Lock()
	if c.closed || c.shutdownErr != nil || c.life.Err() != nil {
		c.mu.Unlock()
		return nil, nil, gymaccount.ErrUnauthorized
	}
	if c.operation || c.expiring {
		c.mu.Unlock()
		return nil, nil, gymstable.ErrBusy
	}
	if view != "" && (!hostedID(view) || c.viewID != view || c.state != "signed_in" || c.attachment == nil || c.wallet == nil) {
		c.mu.Unlock()
		return nil, nil, gymstable.ErrStaleView
	}
	work, cancel := context.WithTimeout(ctx, 2*time.Minute)
	stop := context.AfterFunc(c.life, cancel)
	done := make(chan struct{})
	c.operation, c.opCancel, c.opDone = true, cancel, done
	if signingOut {
		c.viewID = ""
		c.state = "signing_out"
	}
	c.active.Add(1)
	c.mu.Unlock()
	finish := func() {
		stop()
		cancel()
		c.mu.Lock()
		c.operation = false
		c.opCancel = nil
		close(done)
		c.mu.Unlock()
		c.active.Done()
	}
	return work, finish, nil
}

func (c *RemotePoolController) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.viewID = ""
		c.cancel()
		c.mu.Unlock()
		c.active.Wait()
		c.stopExpiry()
		if e := c.detach(); e != nil {
			c.closeErr = e
			return
		}
		c.mu.Lock()
		prior := c.shutdownErr
		c.mu.Unlock()
		if prior != nil {
			c.closeErr = prior
			return
		}
		for _, close := range []func() error{
			func() error {
				if c.wallets != nil {
					return c.wallets.Close()
				}
				return nil
			},
			func() error {
				if c.vault != nil {
					return c.vault.Close()
				}
				return nil
			},
			func() error {
				if c.driver != nil {
					return c.driver.Close()
				}
				return nil
			},
			func() error {
				if c.coordinator != nil {
					return c.coordinator.Close()
				}
				return nil
			},
			func() error {
				if c.accounts != nil {
					return c.accounts.Close()
				}
				return nil
			},
			func() error {
				if c.lock != nil {
					return c.lock.Close()
				}
				return nil
			},
		} {
			if e := close(); e != nil {
				c.closeErr = e
				return
			}
		}
	})
	return c.closeErr
}
