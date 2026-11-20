// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"sync"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/wallet"
	"github.com/taytr-llc/slugwallet/walletdb"
)

var errPoolBackupRestore = errors.New("bounded backup restoration unavailable")

// The verification goroutine owns this lifecycle from construction through
// cleanup. Cancellation joins that goroutine before releasing the child. Keep
// every acquired owner, including failed construction, until closure is known.
type poolBackupRestore struct {
	owners   []io.Closer
	runtime  *walletRuntime
	started  bool
	closed   bool
	once     sync.Once
	closeErr error
}

// Retaining the error also retains the failed cleanup owner. No later close can
// turn its first uncertain result into permission to remove the scratch data.
type poolBackupRestoreCloseError struct {
	owner *poolBackupRestore
	cause error
}

func (*poolBackupRestoreCloseError) Error() string   { return "backup restore cleanup failed" }
func (e *poolBackupRestoreCloseError) Unwrap() error { return e.cause }

func (r *poolBackupRestore) Close() error {
	r.once.Do(func() {
		r.closed = true
		var err error
		for i := len(r.owners) - 1; i >= 0; i-- {
			err = errors.Join(err, r.owners[i].Close())
		}
		if err != nil {
			r.closeErr = &poolBackupRestoreCloseError{owner: r, cause: err}
		}
	})
	return r.closeErr
}

func (r *poolBackupRestore) constructionError(err error) error {
	var cleanup *wallet.PhaseARecoveryCleanupError
	if errors.As(err, &cleanup) {
		r.owners = append(r.owners, cleanup)
	}
	return errPoolBackupRestore
}

// The selected file never supplies a node or transport identity. Convert only
// the concrete child-owned loopback endpoint, preserving its private credential.
func poolBackupRecoveryRPC(c prospectnode.RPCConfig) (wallet.RecoveryRPCConfig, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return wallet.RecoveryRPCConfig{}, errPoolBackupRestore
	}
	defer clear(raw)
	if _, err = prospectnode.ParseRPCConfig(raw); err != nil {
		return wallet.RecoveryRPCConfig{}, errPoolBackupRestore
	}
	u, err := url.Parse(c.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || c.CACertificate != "" {
		return wallet.RecoveryRPCConfig{}, errPoolBackupRestore
	}
	u.Scheme, u.Path = "ws", "/ws"
	return wallet.RecoveryRPCConfig{URL: u.String(), Username: c.Username, Password: c.Password}, nil
}

type poolBackupRecoverySteps interface {
	Status(context.Context) (wallet.RecoveryProgress, error)
	Step(context.Context, wallet.RecoveryBlockSource) (wallet.RecoveryProgress, error)
}

// Address derivation alone is not completion. Retain the exact canonical
// checkpoint returned by the audited scanner and recheck the owned node.
func completePoolBackupRecovery(ctx context.Context, session poolBackupRecoverySteps, source wallet.RecoveryBlockSource) error {
	if ctx == nil || ctx.Err() != nil || session == nil || source == nil {
		return errPoolBackupRestore
	}
	progress, err := session.Status(ctx)
	for err == nil && !progress.Ready {
		if err = ctx.Err(); err != nil {
			return err
		}
		progress, err = session.Step(ctx, source)
	}
	if err != nil {
		return err
	}
	if progress.Version != 1 || progress.Phase != "complete" || progress.Derived != progress.Target || progress.ScannedHeight < 0 || progress.ScannedHeight != progress.TargetHeight || progress.ScannedHash != progress.TargetHash || !payoutLowerHex(progress.ScannedHash, 64, true) {
		return errPoolBackupRestore
	}
	tip, err := source.BestBlock(ctx)
	if err != nil || ctx.Err() != nil || tip.Height != progress.ScannedHeight || tip.Hash.String() != progress.ScannedHash {
		return errPoolBackupRestore
	}
	return nil
}

// Run restores only a disposable schema-12 verification wallet. The same Loader
// retains the validated profile through session closure and ordinary ready Open.
// The caller must Close this owner on every return before considering deletion.
func (r *poolBackupRestore) Run(ctx context.Context, dir, fixture string, bundle recovery.Export, config prospectnode.RPCConfig) error {
	if ctx == nil || ctx.Err() != nil || wire.TestPoW || r.started || r.closed {
		return errPoolBackupRestore
	}
	r.started = true
	identity, err := gymaccount.FixtureIdentity(fixture)
	if err != nil {
		return errPoolBackupRestore
	}
	expected := walletIdentity{RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID}
	if !poolBackupBundleMatches(bundle, expected) {
		return errPoolBackupRestore
	}
	rpc, err := poolBackupRecoveryRPC(config)
	if err != nil {
		return err
	}
	source, err := wallet.NewRecoveryRPCSource(rpc)
	if err != nil {
		return errPoolBackupRestore
	}
	r.owners = append(r.owners, source)
	if err = prepareWalletDirectory(dir, true); err != nil {
		return errPoolBackupRestore
	}
	// The bounded scanner supplies the complete recovery inventory. Do not run
	// the inherited second recovery-window algorithm after audited completion.
	loader := wallet.NewLoader(&chaincfg.MainNetParams, dir, true, 5*time.Second, 0)
	session, err := loader.BeginPhaseARecovery(ctx, []byte(fixtureWalletPublicPass), []byte(fixtureWalletPrivatePass), bundle)
	if err != nil {
		return r.constructionError(err)
	}
	r.owners = append(r.owners, session)
	if err = completePoolBackupRecovery(ctx, session, source); err != nil {
		return errPoolBackupRestore
	}
	if err = session.Close(); err != nil {
		return errPoolBackupRestore
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	loaded, err := loader.OpenExistingWallet([]byte(fixtureWalletPublicPass), false)
	if err != nil {
		return r.constructionError(err)
	}
	// Own the runtime BEFORE identity checks; a failed identity check must not
	// discard a failed Close owner through the legacy convenience constructor.
	r.runtime = &walletRuntime{loader: loader, wallet: loaded}
	r.owners = append(r.owners, r.runtime)
	gym, err := loaded.GymIdentity(ctx)
	if err != nil {
		return errPoolBackupRestore
	}
	r.runtime.identity.GymPublicKey, r.runtime.identity.GymID = gym.PublicKey, gym.GymID
	err = walletdb.View(loaded.Database(), func(tx walletdb.ReadTx) error {
		var e error
		r.runtime.identity.RootPublicKey, e = loaded.Manager.RecoveryRootPublicKey(tx.ReadBucket([]byte("waddrmgr")))
		return e
	})
	if err != nil || r.runtime.identity != expected || ctx.Err() != nil {
		return errPoolBackupRestore
	}
	return nil
}

// A private sample, separate from the unchanged display DTO. The exact hash
// matters even when a fork has the same height and the wallet says synced.
func (r *walletRuntime) poolBackupSyncedTip(ctx context.Context) (localTip, error) {
	if ctx == nil || ctx.Err() != nil || r == nil || !r.mu.TryLock() {
		return localTip{}, errPoolBackupRestore
	}
	defer r.mu.Unlock()
	if r.closed || r.wallet == nil || r.wallet.Manager == nil || r.backend == nil || r.backend.Disconnected() || !r.wallet.ChainSynced() || ctx.Err() != nil {
		return localTip{}, errPoolBackupRestore
	}
	stamp := r.wallet.Manager.SyncedTo()
	if stamp.Height < 0 || !payoutLowerHex(stamp.Hash.String(), 64, true) || !r.wallet.ChainSynced() || r.backend.Disconnected() || ctx.Err() != nil {
		return localTip{}, errPoolBackupRestore
	}
	return localTip{Hash: stamp.Hash.String(), Height: stamp.Height}, nil
}

func (c *PoolChild) poolBackupWalletAt(ctx context.Context, wallet *walletRuntime, expected localTip) bool {
	actual, err := wallet.poolBackupSyncedTip(ctx)
	return err == nil && actual == expected
}
