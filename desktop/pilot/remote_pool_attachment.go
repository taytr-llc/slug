// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/wire/v2"
)

var (
	ErrRemotePoolUnavailable = errors.New("remote pool attachment unavailable")
	ErrRemotePoolBusy        = errors.New("remote pool attachment busy")
	ErrRemotePoolChanged     = errors.New("remote pool attachment identity changed")
	ErrRemotePoolCleanup     = errors.New("remote pool attachment cleanup incomplete")
	ErrRemotePoolProfile     = errors.New("remote pool attachment requires production PoW")
)

// Constructor inputs remain inside the helper. NodeRPC is private local input.
type RemotePoolInputs struct {
	AttachmentRoot          string
	Config                  RemotePoolConfig
	NodeRPC                 prospectnode.RPCConfig
	PoolCredentialPath      string
	AdmissionCredentialPath string
}

type RemotePoolView struct {
	Version            uint32 `json:"version"`
	ViewID             string `json:"view_id"`
	AccountAuthorityID string `json:"account_authority_id"`
	AccountID          string `json:"account_id"`
	Generation         uint64 `json:"generation"`
	Fixture            string `json:"fixture"`
	RootPublicKey      string `json:"root_public_key"`
	GymPublicKey       string `json:"gym_public_key"`
	GymID              string `json:"gym_id"`
}

type RemotePoolSnapshot struct {
	Version                 uint32         `json:"version"`
	ConfigID                string         `json:"config_id"`
	View                    RemotePoolView `json:"view"`
	SourceBindingID         string         `json:"source_binding_id"`
	OriginalSourceBindingID string         `json:"original_source_binding_id"`
	State                   string         `json:"state"`
	WorkerState             string         `json:"worker_state"`
	LastError               string         `json:"last_error"`
	Attempts                uint64         `json:"attempts"`
	HashesPerSecondMilli    uint64         `json:"hashes_per_second_milli"`
	Records                 uint64         `json:"records"`
	Pending                 uint64         `json:"pending"`
	WalletState             string         `json:"wallet_state"`
	WalletHeight            uint32         `json:"wallet_height"`
	SpendableAtoms          int64          `json:"spendable_atoms"`
	ImmatureAtoms           int64          `json:"immature_atoms"`
	UnconfirmedAtoms        int64          `json:"unconfirmed_atoms"`
	PayoutAddress           string         `json:"payout_address"`
}

type remotePoolRun struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	worker *prospectworker.PoolWorker
}

// The session is borrowed, never closed here. Controller Close first joins this
// owner, then closes its original wallet session/setup/vault/account/root owners.
type RemotePoolAttachment struct {
	recovery          *RecoverableRemotePoolConfig
	recoveryWorker    prospectworker.RecoverablePoolConfig
	config            RemotePoolConfig
	configID          string
	view              RemotePoolView
	workerConfig      prospectworker.PoolConfig
	local             *remotePoolLocal
	node              *remotePoolNode
	credentials       *remotePoolCredentials
	files             *remotePoolFiles
	box               *prospectworker.PoolOutbox
	life              context.Context
	cancel            context.CancelFunc
	mu                sync.Mutex
	closing, ready    bool
	gate              chan struct{}
	active            sync.WaitGroup
	run               *remotePoolRun
	dailyRead         *remoteDailyPayoutOperation
	dailyReadLast     RemoteDailyPayoutJob
	dailyReadSequence uint64
	dailyPayout       *RemoteDailyPayoutCapture
	last              prospectworker.Status
	state, lastError  string
	fileMu            sync.Mutex
	closeOnce         sync.Once
	closeDone         chan struct{}
	closeErr          error
}

func OpenRemotePoolAttachment(ctx context.Context, session *OwnedWalletSession, viewID string, in RemotePoolInputs) (*RemotePoolAttachment, error) {
	return openRemotePoolAttachment(ctx, session, viewID, in, false)
}
func openRemotePoolAttachment(ctx context.Context, session *OwnedWalletSession, viewID string, in RemotePoolInputs, daily bool) (*RemotePoolAttachment, error) {
	return openRemotePoolAttachmentSelection(ctx, session, viewID, in, daily, nil)
}
func openRemotePoolAttachmentSelection(ctx context.Context, session *OwnedWalletSession, viewID string, in RemotePoolInputs, daily bool, recovery *RecoverableRemotePoolConfig) (out *RemotePoolAttachment, resultErr error) {
	if wire.TestPoW {
		return nil, ErrRemotePoolProfile
	}
	if ctx == nil || session == nil || session.setup == nil || session.entry == nil || session.runtime == nil || !remotePoolID(viewID) {
		return nil, ErrRemotePoolUnavailable
	}
	if e := ctx.Err(); e != nil {
		if daily {
			return nil, e
		}
		return nil, ErrRemotePoolUnavailable
	}
	raw, e := marshalSelectedRemotePoolConfig(in.Config, daily)
	if e != nil {
		return nil, e
	}
	config, e := parseSelectedRemotePoolConfig(raw, daily)
	if e != nil {
		return nil, e
	}
	if e = remotePoolSelectedPaths(session, in); e != nil {
		return nil, e
	}
	if e = remotePoolValidateNode(in.NodeRPC); e != nil {
		return nil, e
	}
	id, e := selectedRemotePoolConfigID(config)
	if e != nil {
		return nil, e
	}
	wc, e := remotePoolWorkerConfig(config)
	if e != nil {
		return nil, e
	}
	var rw prospectworker.RecoverablePoolConfig
	if recovery != nil {
		if !daily || !sameRecoverableRemoteBase(*recovery, config) {
			return nil, ErrRemotePoolConfig
		}
		id, e = RecoverableRemotePoolConfigID(*recovery)
		if e != nil {
			return nil, e
		}
		rw, e = remotePoolRecoverableWorkerConfig(*recovery)
		if e != nil {
			return nil, e
		}
	}
	life, cancel := context.WithCancel(session.setup.ctx)
	a := &RemotePoolAttachment{recovery: recovery, recoveryWorker: rw, config: config, configID: id, workerConfig: wc, life: life, cancel: cancel, gate: make(chan struct{}, 1), closeDone: make(chan struct{}), state: "stopped"}
	defer func() {
		if resultErr != nil {
			ce := a.Close()
			resultErr = errors.Join(resultErr, ce)
			if ce != nil {
				out = a
			} else {
				out = nil
			}
		}
	}()
	work, stop := context.WithTimeout(ctx, 60*time.Second)
	defer stop()
	stopLife := context.AfterFunc(life, stop)
	defer stopLife()
	a.local, e = claimRemotePoolLocalMode(work, session, config.ExpectedLocal, daily)
	if e != nil {
		return nil, e
	}
	f := config.ExpectedLocal.Recipient.Config
	a.view = RemotePoolView{1, viewID, f.AccountAuthorityID, f.AccountID, session.login.Account.Generation, f.Fixture, f.Identity.RootPublicKey, f.Identity.GymPublicKey, f.Identity.GymID}
	a.files, e = openRemotePoolFilesSelection(in.AttachmentRoot, config, true, recovery)
	if e != nil {
		return nil, e
	}
	a.credentials, e = openRemotePoolCredentialsSelection(life, config, remotePoolCredentialSelection{in.PoolCredentialPath, in.AdmissionCredentialPath}, daily, recovery)
	if e != nil {
		return nil, e
	}
	a.node, e = openRemotePoolNode(life, in.NodeRPC, a.local, config.Source.Policy)
	if e != nil {
		return nil, e
	}
	if _, e = a.node.client.Resolve(work, config.Source.Policy); e != nil {
		return nil, ErrRemotePoolUnavailable
	}
	if e = a.local.attach(work, in.NodeRPC); e != nil {
		return nil, e
	}
	path, create, e := a.files.child("outbox")
	if e != nil {
		return nil, e
	}
	if recovery != nil {
		if create {
			a.box, e = prospectworker.CreateRecoverableDailyPoolOutbox(path, rw)
		} else {
			a.box, e = prospectworker.OpenRecoverableDailyPoolOutbox(path, rw)
		}
	} else if daily {
		if create {
			a.box, e = prospectworker.CreateDailyPoolOutbox(path, wc)
		} else {
			a.box, e = prospectworker.OpenDailyPoolOutbox(path, wc)
		}
	} else {
		if create {
			a.box, e = prospectworker.CreatePoolOutbox(path, wc)
		} else {
			a.box, e = prospectworker.OpenPoolOutbox(path, wc)
		}
	}
	if e != nil {
		return nil, e
	}
	if e = a.files.adopt("outbox"); e != nil {
		return nil, e
	}
	if e = a.local.check(work); e != nil {
		return nil, e
	}
	if e = a.credentials.check(work, 0); e != nil {
		return nil, e
	}
	if e = a.credentials.check(work, 1); e != nil {
		return nil, e
	}
	if e = a.files.markReady(); e != nil {
		return nil, e
	}
	if e = a.local.check(work); e != nil {
		return nil, e
	}
	if work.Err() != nil {
		return nil, work.Err()
	}
	a.ready = true
	return a, nil
}

func remotePoolSelectedPaths(s *OwnedWalletSession, in RemotePoolInputs) error {
	paths := []string{in.AttachmentRoot, in.PoolCredentialPath, in.AdmissionCredentialPath}
	for _, p := range paths {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p || len(p) > 4096 {
			return ErrRemotePoolConfig
		}
	}
	for _, p := range paths[1:] {
		if p == in.AttachmentRoot || withinDir(p, in.AttachmentRoot) || withinDir(in.AttachmentRoot, filepath.Dir(p)) {
			return ErrRemotePoolConfig
		}
	}
	// The original child belongs to an existing owned root; never add remote
	// files anywhere under that closed tree, or make a parent attachment own it.
	root := s.entry.config.ChildRoot
	for p := root; ; p = filepath.Dir(p) {
		exists, e := poolRuntimeExists(filepath.Join(p, "SLUG-OWNED-GYM-INIT.json"))
		if e != nil {
			return e
		}
		if exists {
			root = p
			break
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	for _, p := range paths {
		if p == root || withinDir(p, root) || withinDir(root, p) {
			return ErrRemotePoolConfig
		}
	}
	return nil
}

func (a *RemotePoolAttachment) check(ctx context.Context) error {
	if a == nil || ctx == nil {
		return ErrRemotePoolUnavailable
	}
	if e := ctx.Err(); e != nil {
		if a.config.Version == 2 {
			return e
		}
		return ErrRemotePoolUnavailable
	}
	a.mu.Lock()
	bad := a.closing || !a.ready || a.life == nil || a.life.Err() != nil
	a.mu.Unlock()
	if bad {
		return ErrRemotePoolUnavailable
	}
	if e := a.local.check(ctx); e != nil {
		return e
	}
	if a.config.Version == 2 {
		if e := a.local.dailyRecipientControls(ctx); e != nil {
			return e
		}
	}
	a.fileMu.Lock()
	e := a.files.check()
	a.fileMu.Unlock()
	return e
}
func (a *RemotePoolAttachment) begin(ctx context.Context, view string) (context.Context, func(), error) {
	if a == nil || ctx == nil || view != a.view.ViewID {
		return nil, nil, ErrRemotePoolUnavailable
	}
	if e := ctx.Err(); e != nil {
		if a.config.Version == 2 {
			return nil, nil, e
		}
		return nil, nil, ErrRemotePoolUnavailable
	}
	a.mu.Lock()
	if a.closing || !a.ready || a.life == nil || a.life.Err() != nil {
		a.mu.Unlock()
		return nil, nil, ErrRemotePoolUnavailable
	}
	select {
	case a.gate <- struct{}{}:
	default:
		a.mu.Unlock()
		return nil, nil, ErrRemotePoolBusy
	}
	a.active.Add(1)
	a.mu.Unlock()
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(a.life, cancel)
	end := func() { stop(); cancel(); <-a.gate; a.active.Done() }
	if e := a.check(work); e != nil {
		end()
		return nil, nil, e
	}
	return work, end, nil
}
func (a *RemotePoolAttachment) Close() error {
	if a == nil {
		return nil
	}
	if a.cancel == nil || a.closeDone == nil {
		return ErrRemotePoolUnavailable
	}
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.closing = true
		a.cancel()
		if a.run != nil {
			a.run.cancel()
		}
		a.mu.Unlock()
		a.active.Wait()
		if a.config.Version == 2 {
			a.mu.Lock()
			a.dailyPayout = nil
			a.dailyReadLast = RemoteDailyPayoutJob{}
			a.mu.Unlock()
		}
		if a.node != nil {
			a.node.close()
		}
		if a.credentials != nil {
			a.closeErr = errors.Join(a.closeErr, a.credentials.Close())
		}
		if a.box != nil {
			a.closeErr = errors.Join(a.closeErr, a.box.Close())
		}
		if a.local != nil {
			a.local.close()
		}
		// A failed child retains the original root lock and exact failed owners.
		if a.closeErr == nil && a.files != nil {
			a.closeErr = a.files.close()
		}
		close(a.closeDone)
	})
	<-a.closeDone
	return a.closeErr
}
