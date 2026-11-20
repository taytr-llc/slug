// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"reflect"
)

// The parent has already attached this exact session's wallet and retains its
// lifetime. Construction can read the provisional original view, but cannot
// authorize a transfer or disclose the child as signed in.
func NewOwnedGymChild(ctx context.Context, runtime *PoolRuntime, wallet *OwnedWalletSession, league *OwnedLeague, account StableAccountConfig) (out *OwnedGymChild, resultErr error) {
	if wire.TestPoW {
		return nil, gymstable.ErrProfile
	}
	if ctx == nil || ctx.Err() != nil || runtime == nil || wallet == nil || league == nil || account.Lease == nil || account.Manifest.Account.AdmissionSigner != nil || wallet.check(ctx) != nil || !wallet.ready.Load() {
		return nil, gymstable.ErrInvalid
	}
	a := account.Manifest
	f, e := gymaccount.FixtureIdentity(a.Account.Fixture)
	if e != nil || wallet.login.Account.ID != a.Account.AccountID || wallet.login.Account.Generation != a.Generation || wallet.intent.Pin.Fixture != a.Account.Fixture || wallet.setup.accounts != runtime.accounts {
		return nil, gymstable.ErrInvalid
	}
	view := OwnedGymView{Version: 1, ViewID: a.ViewID, AccountAuthorityID: a.Account.AuthorityID, AccountID: a.Account.AccountID, Generation: a.Generation, Fixture: a.Account.Fixture, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID}
	if ValidateOwnedGymView(view) != nil {
		return nil, gymstable.ErrInvalid
	}
	r, e := runtime.ownedGymChildResources(wallet)
	if e != nil {
		return nil, e
	}
	if r.RegisterClose == nil || r.Release == nil || r.Node == nil || r.Life == nil || r.Life.Err() != nil {
		if r.Release != nil {
			r.Release()
		}
		return nil, ErrPoolRuntime
	}
	life, cancel := context.WithCancel(context.Background())
	c := &OwnedGymChild{account: account, view: view, runtime: runtime, wallet: wallet, league: league, workerConfig: r.WorkerConfig, node: r.Node, rpc: r.RPC, config: r.Config, life: life, cancel: cancel, release: r.Release}
	c.stopRuntime = context.AfterFunc(r.Life, cancel)
	// Retain this exact owner even when failed construction cannot join cleanup.
	defer func() {
		if resultErr != nil {
			ce := c.Close()
			resultErr = errors.Join(resultErr, ce)
			if ce != nil {
				out = c
			} else {
				out = nil
			}
		}
	}()
	if e = r.RegisterClose(c.Close); e != nil {
		return nil, e
	}
	cfg := league.Config()
	var participant *OwnedLeagueParticipant
	for n := range cfg.Participants {
		if cfg.Participants[n].Fixture == view.Fixture {
			participant = &cfg.Participants[n]
		}
	}
	if participant == nil {
		return nil, ErrPoolRuntime
	}
	c.scope = OwnedGymScope{Version: 1, Fixture: view.Fixture, Source: cfg.Source, PoolSource: cfg.PoolSource, SourceBindingID: cfg.SourceBindingID, StablePin: participant.StablePin, StableStoreID: participant.StableStoreID, EntryPin: participant.EntryPin, EntryStoreID: participant.EntryStoreID, EventPin: cfg.EventPin, EventStoreID: cfg.EventStoreID, Carrier: wallet.intent.Carrier, CarrierBindingID: wallet.intent.CarrierBindingID}
	if ValidateOwnedGymScope(c.scope) != nil || !c.scope.ViewMatches(view) || r.Recipient != wallet.recipient || r.WorkerConfig.Source != cfg.PoolSource || r.WorkerConfig.PublicKey != f.GymPublicKey {
		return nil, ErrPoolRuntime
	}
	if c.stable, c.entries, e = league.Participant(view.Fixture); e != nil {
		return nil, e
	}
	c.events = league.Events()
	if c.events == nil {
		return nil, ErrPoolRuntime
	}
	clientConfig := PoolAuthorityConfig{Version: 1, URL: r.WorkerConfig.AuthorityOrigin, PublicKey: f.GymPublicKey, TransportTrustID: r.WorkerConfig.TransportTrustID, Source: cfg.PoolSource}
	if c.authority, e = NewPoolSessionAuthority(clientConfig, a); e != nil {
		return nil, e
	}
	admission, e := NewPoolSessionAuthority(clientConfig, a)
	if e != nil {
		return nil, e
	}
	c.admission = &poolChildAdmission{fence: admission, config: r.WorkerConfig}
	if c.box, e = openOwnedWorker(c.config.DataDir, r.WorkerConfig, r.Recipient, participant.StablePin); e != nil {
		return nil, e
	}
	if c.transfers, e = NewOwnedTransferActions(ctx, wallet, runtime, view.ViewID); e != nil {
		return nil, e
	}
	c.snapshot = OwnedGymChildSnapshot{State: "stopped", WalletState: "syncing", PayoutAddress: r.Recipient.Recipient.Address, StableStoreID: participant.StableStoreID, EntryStoreID: participant.EntryStoreID, EventStoreID: cfg.EventStoreID, Backup: HostedBackupSnapshot{State: "idle"}}
	if e = c.fence(ctx, func() error { return nil }); e != nil {
		return nil, e
	}
	return c, nil
}

type ownedWorkerMarker struct {
	Version   uint32                    `json:"version"`
	Worker    prospectworker.PoolConfig `json:"worker"`
	Recipient PoolRecipientBinding      `json:"recipient"`
	Stable    gymstable.OwnedPin        `json:"stable"`
}

func openOwnedWorker(root string, worker prospectworker.PoolConfig, recipient PoolRecipientBinding, stable gymstable.OwnedPin) (box *prospectworker.PoolOutbox, resultErr error) {
	want := ownedWorkerMarker{1, worker, recipient, stable}
	intent := filepath.Join(root, "SLUG-OWNED-WORKER-INTENT.json")
	ready := filepath.Join(root, "SLUG-OWNED-WORKER-READY.json")
	path := filepath.Join(root, "owned-outbox")
	check := func(name string) error {
		var got ownedWorkerMarker
		if ownedLeagueRead(name, &got) != nil || !reflect.DeepEqual(got, want) {
			return ErrPoolRuntime
		}
		return nil
	}
	ie, e := poolRuntimeExists(intent)
	if e != nil {
		return nil, e
	}
	re, e := poolRuntimeExists(ready)
	if e != nil {
		return nil, e
	}
	pe, e := poolRuntimeExists(path)
	if e != nil {
		return nil, e
	}
	if !ie {
		if re || pe {
			return nil, ErrPoolRuntime
		}
		if e = ownedLeagueWrite(intent, want); e != nil {
			return nil, e
		}
	} else if e = check(intent); e != nil {
		return nil, e
	}
	if re && (check(ready) != nil || !pe) {
		return nil, ErrPoolRuntime
	}
	if pe {
		if plainChild(path, true) != nil {
			return nil, ErrPoolRuntime
		}
		box, e = prospectworker.OpenPoolOutbox(path, worker)
	} else {
		box, e = prospectworker.CreatePoolOutbox(path, worker)
	}
	if e != nil {
		return box, e
	}
	if !re {
		if e = ownedLeagueWrite(ready, want); e != nil {
			ce := box.Close()
			if ce == nil {
				box = nil
			}
			return box, errors.Join(e, ce)
		}
	}
	return box, nil
}
