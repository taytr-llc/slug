// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

// PoolRecoveryArchive owns only established public journals. It cannot launch
// pool/admission/settlement services, provision a wallet or create a child.
// Verification may borrow one joined, owned node pair through BeginSource.
type PoolRecoveryArchive struct {
	r               *PoolRuntime
	bindings        map[string]recoveryvault.CurrentBinding
	owners          []io.Closer
	life            context.Context
	cancel          context.CancelFunc
	gate            chan struct{}
	once            sync.Once
	closeErr        error
	failedSource    *archiveNodeOwner
	requireChildren bool
}

func OpenPoolRecoveryArchive(ctx context.Context, config Config, accounts *gymaccount.Store, options PoolOptions, sourceID string, bindings []recoveryvault.CurrentBinding) (out *PoolRecoveryArchive, resultErr error) {
	return openPoolRecoveryArchive(ctx, config, accounts, options, sourceID, bindings, true)
}

// OpenPoolRecoverySetupAudit audits an established allocation-READY setup before
// opening installed wallets. A never-created child may have no outbox trio; any
// partial or previously established child still requires exact complete records.
func OpenPoolRecoverySetupAudit(ctx context.Context, config Config, accounts *gymaccount.Store, options PoolOptions, sourceID string, bindings []recoveryvault.CurrentBinding) (*PoolRecoveryArchive, error) {
	return openPoolRecoveryArchive(ctx, config, accounts, options, sourceID, bindings, false)
}
func openPoolRecoveryArchive(ctx context.Context, config Config, accounts *gymaccount.Store, options PoolOptions, sourceID string, bindings []recoveryvault.CurrentBinding, requireChildren bool) (out *PoolRecoveryArchive, resultErr error) {
	options, e := ResolvePoolOptions(options)
	if e != nil {
		return nil, e
	}
	if ctx == nil || ctx.Err() != nil || accounts == nil || !payoutNonzeroID(sourceID) || len(bindings) != 2 || !filepath.IsAbs(config.DataDir) || !filepath.IsAbs(config.NodeBinary) {
		return nil, ErrPoolRuntime
	}
	life, cancel := context.WithCancel(context.Background())
	r := &PoolRuntime{config: config, accounts: accounts, recipients: make(map[string]PoolRecipientBinding), gymAccounts: make(map[string]string), leagueFactories: defaultPoolLeagueFactories()}
	a := &PoolRecoveryArchive{r: r, bindings: make(map[string]recoveryvault.CurrentBinding), life: life, cancel: cancel, gate: make(chan struct{}, 1), requireChildren: requireChildren}
	defer func() {
		if resultErr != nil {
			closeErr := a.Close()
			resultErr = errors.Join(resultErr, closeErr)
			if closeErr != nil {
				out = a
			}
		}
	}()
	root := config.DataDir
	if poolRuntimeRootFiles(root) != nil || requireChainFiles(root) != nil || readManifestJSON(filepath.Join(root, poolRuntimeMarkerName), poolRuntimeMarkerLimit, &r.marker) != nil || !r.marker.validFor(accounts.AuthorityID(), options.CandidateDescriptorID) {
		return nil, ErrPoolRuntime
	}
	if readManifestJSON(filepath.Join(root, poolRuntimeSourceName), poolRuntimeMarkerLimit, &r.source) != nil || !r.marker.validSource(r.source) {
		return nil, ErrPoolRuntime
	}
	if id, e := poolservice.BindingID(r.source); e != nil || id != sourceID {
		return nil, ErrPoolRuntime
	}
	if readPoolRuntimeRecord(filepath.Join(root, poolRuntimeCoreName), poolRuntimeCore{1, sourceID}) != nil {
		return nil, ErrPoolRuntime
	}
	// Existing instance lock only; a missing namespace is not repaired.
	lock, e := leveldb.OpenFile(filepath.Join(root, "instance-lock"), &opt.Options{ErrorIfMissing: true, Strict: opt.StrictAll})
	if e != nil {
		return nil, ErrPoolRuntimeBusy
	}
	a.owners = append(a.owners, lock)
	r.issuer, e = registry.OpenManifestAuthority(filepath.Join(root, "issuer"), r.source.AuthorityPin, true)
	if e != nil {
		return nil, e
	}
	a.owners = append(a.owners, r.issuer)
	for i, f := range []string{"zero256", "incrementing256"} {
		b := bindings[i]
		identity, e := gymaccount.FixtureIdentity(f)
		if e != nil || b.Version != 1 || b.VaultBinding.AuthorityID != accounts.AuthorityID() || b.SourceBindingID != sourceID || b.VaultBinding.RootPublicKey != identity.RootPublicKey || b.VaultBinding.GymPublicKey != identity.GymPublicKey || b.VaultBinding.GymID != identity.GymID {
			return nil, ErrPoolRuntime
		}
		expected, _, e := poolRecipientExpected(PoolRecipientConfig{1, accounts.AuthorityID(), b.VaultBinding.AccountID, f, identity, r.source})
		if e != nil {
			return nil, e
		}
		gymroot := filepath.Join(root, "gyms", b.VaultBinding.AccountID)
		if readPoolRuntimeRecord(filepath.Join(gymroot, poolRuntimeGymName), poolRuntimeGym{1, accounts.AuthorityID(), b.VaultBinding.AccountID, f}) != nil || readPoolRuntimeRecord(filepath.Join(gymroot, poolRuntimeGymReadyName), expected) != nil || readPoolRecipientMarker(filepath.Join(gymroot, poolRecipientIntentName), recipientMarker(expected, "initializing")) != nil || readPoolRecipientMarker(filepath.Join(gymroot, poolRecipientReadyName), recipientMarker(expected, "ready")) != nil {
			return nil, ErrPoolRuntime
		}
		r.recipients[f], r.gymAccounts[f], a.bindings[f] = expected, b.VaultBinding.AccountID, b
	}
	if e = allocationDirectory(filepath.Join(root, "gyms"), map[string]bool{bindings[0].VaultBinding.AccountID: true, bindings[1].VaultBinding.AccountID: true}); e != nil {
		return nil, e
	}
	ready, e := r.readyRecord()
	if e != nil || readPoolRuntimeRecord(filepath.Join(root, poolRuntimeReadyName), ready) != nil {
		return nil, ErrPoolRuntime
	}
	r.pool, e = poolshare.OpenAudit(filepath.Join(root, "shares"), r.source, r.issuer)
	if e != nil {
		return nil, e
	}
	a.owners = append(a.owners, r.pool)
	r.rounds, e = poolround.OpenAudit(filepath.Join(root, "rounds"), poolround.Limits{Version: 1, MaxRounds: 256, MaxBytes: 256 << 20}, r.pool)
	if e != nil {
		return nil, e
	}
	a.owners = append(a.owners, r.rounds)
	sc := poolsettle.Config{Version: 1, ChangeScript: poolsettlefixture.PoolScript(), MaxRounds: 256, MaxBatches: 366}
	for _, b := range ready.Recipients {
		sc.Destinations = append(sc.Destinations, poolsettle.Destination{GymID: b.Config.Identity.GymID, Script: b.Recipient.Script})
	}
	r.settlement, e = poolsettle.OpenAudit(filepath.Join(root, "settlement"), sc, r.rounds)
	if e != nil {
		return nil, e
	}
	a.owners = append(a.owners, r.settlement)
	if e = a.auditLeague(ctx); e != nil {
		return nil, e
	}
	if e = a.auditWithdrawals(ctx); e != nil {
		return nil, e
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return a, nil
}
func (a *PoolRecoveryArchive) auditLeague(ctx context.Context) error {
	r := a.r
	root := filepath.Join(r.config.DataDir, poolLeagueDirectory)
	var recorded poolLeagueIntent
	if readManifestJSON(filepath.Join(root, poolLeagueIntentName), poolRuntimeMarkerLimit, &recorded) != nil {
		return ErrPoolRuntime
	}
	intent, e := r.leagueIntent(&recorded)
	if e != nil || poolLeagueFiles(root, intent) != nil {
		return ErrPoolRuntime
	}
	var ready poolLeagueReady
	if readManifestJSON(filepath.Join(root, poolLeagueReadyName), poolRuntimeMarkerLimit, &ready) != nil || ready.Version != 1 || !reflect.DeepEqual(ready.Intent, intent) || ready.EventCreatedMS <= 0 || readPoolRuntimeRecord(filepath.Join(root, poolLeagueReadyName), ready) != nil {
		return ErrPoolRuntime
	}
	r.league = &ready
	r.events, e = gymevent.OpenAudit(ctx, filepath.Join(root, "events"), intent.EventPin)
	if e != nil {
		return e
	}
	a.owners = append(a.owners, r.events)
	info, e := r.events.Info(ctx)
	if e != nil || info.StoreID != intent.EventStoreID || info.CreatedAtMS != ready.EventCreatedMS {
		return ErrPoolRuntime
	}
	if e = r.loadPipelineBootstrap(ready, true); e != nil || r.pipelineBootstrap == nil || r.pipelineBootstrap.ready == nil {
		return ErrPoolRuntime
	}
	r.pipeline = r.pipelineBootstrap.ready
	_, binding, e := derivePoolPipeline(ready)
	if e != nil {
		return e
	}
	p := r.pipeline.Prepared
	dir := filepath.Join(r.config.DataDir, poolPipelineDirectory)
	r.seeds, e = gymseed.OpenAudit(ctx, filepath.Join(dir, "seeds"), binding.SeedPin, r.events)
	if e != nil {
		return e
	}
	a.owners = append(a.owners, r.seeds)
	si, e := r.seeds.Info(ctx)
	if e != nil || p.Seed != (poolPipelineStage{1, si.StoreID, si.CreatedAtMS, ready.EventCreatedMS}) {
		return ErrPoolRuntime
	}
	r.cards, e = gymcard.OpenCareerAudit(ctx, filepath.Join(dir, "cards"), binding.CardPin, r.seeds, r.pipeline.Career)
	if e != nil {
		return e
	}
	a.owners = append(a.owners, r.cards)
	ci, e := r.cards.Info(ctx)
	if e != nil || p.Card != (poolPipelineStage{1, ci.StoreID, ci.CreatedAtMS, ci.SourceCreatedAtMS}) {
		return ErrPoolRuntime
	}
	r.bouts, e = gymbout.OpenAudit(ctx, filepath.Join(dir, "bouts"), binding.BoutPin, r.cards)
	if e != nil {
		return e
	}
	a.owners = append(a.owners, r.bouts)
	bi, e := r.bouts.Info(ctx)
	if e != nil || p.Bout != (poolPipelineStage{1, bi.StoreID, bi.CreatedAtMS, bi.SourceCreatedAtMS}) {
		return ErrPoolRuntime
	}
	r.results, e = gymresult.OpenCareerAudit(ctx, filepath.Join(dir, "results"), binding.ResultPin, r.bouts)
	if e != nil {
		return e
	}
	a.owners = append(a.owners, r.results)
	ri, e := r.results.Info(ctx)
	if e != nil || p.Result != (poolPipelineStage{1, ri.StoreID, ri.CreatedAtMS, ri.SourceCreatedAtMS}) {
		return ErrPoolRuntime
	}
	activation, e := r.results.CareerInfo(ctx)
	if e != nil || activation != r.pipeline.Career {
		return ErrPoolRuntime
	}
	r.careerReader, e = r.results.CareerReader()
	if e != nil {
		return e
	}
	if e = r.cards.AttachCareer(ctx, r.careerReader); e != nil {
		return e
	}
	for _, p := range intent.Participants {
		if readPoolRuntimeRecord(filepath.Join(root, p.AccountID+"-READY.json"), poolLeagueParticipantReady{1, intent.SourceBindingID, p}) != nil {
			return ErrPoolRuntime
		}
		dir := filepath.Join(r.config.DataDir, "gyms", p.AccountID)
		stable, e := gymstable.OpenSharedCareerAudit(filepath.Join(dir, "stable"), p.EntryPin.StablePin, r.issuer, activation)
		if e != nil {
			return e
		}
		a.owners = append(a.owners, stable)
		if e = stable.AttachCareer(ctx, r.careerReader); e != nil {
			return e
		}
		entry, e := gymentry.OpenAudit(ctx, filepath.Join(dir, "entries"), p.EntryPin, stable)
		if e != nil {
			return e
		}
		a.owners = append(a.owners, entry)
		var recipient PoolRecipientBinding
		for _, b := range r.recipients {
			if b.Config.AccountID == p.AccountID {
				recipient = b
			}
		}
		childPresent := 0
		for _, name := range []string{"SLUG-POOL-CHILD.json", "SLUG-POOL-CHILD-READY.json", "pool-outbox"} {
			found, e := poolRuntimeExists(filepath.Join(dir, name))
			if e != nil {
				return e
			}
			if found {
				childPresent++
			}
		}
		if childPresent == 0 && !a.requireChildren {
			continue
		}
		if childPresent != 3 {
			return ErrPoolRuntime
		}
		worker, e := r.archiveWorker(recipient)
		if e != nil {
			return e
		}
		marker := poolChildMarker{1, worker, recipient, p.EntryPin.StablePin}
		if readPoolRuntimeRecord(filepath.Join(dir, "SLUG-POOL-CHILD.json"), marker) != nil || readPoolRuntimeRecord(filepath.Join(dir, "SLUG-POOL-CHILD-READY.json"), marker) != nil {
			return ErrPoolRuntime
		}
		box, e := prospectworker.OpenPoolOutboxAudit(filepath.Join(dir, "pool-outbox"), worker)
		if e != nil {
			return e
		}
		a.owners = append(a.owners, box)
	}
	return nil
}
func (r *PoolRuntime) archiveWorker(b PoolRecipientBinding) (prospectworker.PoolConfig, error) {
	po, ao := poolRuntimeOrigin(r.marker.PoolPort), poolRuntimeOrigin(r.marker.AdmissionPort)
	pt, e := poolservice.TransportTrustID(poolservice.ClientConfig{Version: 1, URL: po, Token: fixtureToken, PublicKey: b.Config.Identity.GymPublicKey, Source: r.source})
	if e != nil {
		return prospectworker.PoolConfig{}, e
	}
	at, e := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: ao, Token: fixtureToken, AuthorityPin: r.source.AuthorityPin})
	if e != nil {
		return prospectworker.PoolConfig{}, e
	}
	return prospectworker.ValidatePoolConfig(prospectworker.PoolConfig{Version: 1, Source: r.source, PublicKey: b.Config.Identity.GymPublicKey, AuthorityOrigin: po, TransportTrustID: pt, AdmissionOrigin: ao, AdmissionTrustID: at, MaxRecords: 1000, MaxBytes: 256 << 20})
}
func (a *PoolRecoveryArchive) auditWithdrawals(ctx context.Context) error {
	r := a.r
	if e := r.validateWithdrawalRoot(); e != nil {
		return e
	}
	guard, e := r.withdrawalGuard()
	if e != nil {
		return e
	}
	for _, b := range r.recipients {
		root := filepath.Join(r.config.DataDir, poolWithdrawalDirectory, b.Config.AccountID)
		var init poolWithdrawalInit
		if readManifestJSON(filepath.Join(root, "INIT.json"), poolRuntimeMarkerLimit, &init) != nil {
			return ErrPoolRuntime
		}
		i := b.Config.Identity
		pin := walletwithdraw.Pin{Version: 1, Network: i.Network, Genesis: i.Genesis, AccountAuthorityID: b.Config.AccountAuthorityID, AccountID: b.Config.AccountID, Fixture: b.Config.Fixture, WalletProfile: i.Profile, RootPublicKey: i.RootPublicKey, GymPublicKey: i.GymPublicKey, GymID: i.GymID, PoolSourceBindingID: guard.SourceBindingID}
		identity := walletwithdraw.Identity{JournalID: init.JournalID, Pin: pin}
		binding, e := walletwithdraw.BindingID(identity)
		if e != nil {
			return e
		}
		want := poolWithdrawalInit{"slug-pool-withdrawal-init", 1, withdrawalRecordHash(guard), identity.JournalID, pin, binding}
		ready := poolWithdrawalReady{"slug-pool-withdrawal-ready", 1, withdrawalRecordHash(want), identity.JournalID, binding}
		if withdrawalReadRecord(filepath.Join(root, "INIT.json"), want) != nil || withdrawalReadRecord(filepath.Join(root, "READY.json"), ready) != nil {
			return ErrPoolRuntime
		}
		journal, e := walletwithdraw.OpenAudit(a.life, filepath.Join(root, "journal"), identity)
		if e != nil {
			return e
		}
		a.owners = append(a.owners, journal)
	}
	return ctx.Err()
}
func (a *PoolRecoveryArchive) Source() poolshare.Config { return a.r.source }

type archiveNodeOwner struct {
	nodes  *pilotNodes
	client *prospectnode.Client
	once   sync.Once
	err    error
}

func (n *archiveNodeOwner) Close() error {
	n.once.Do(func() {
		if n.client != nil {
			n.client.Close()
		}
		if n.nodes != nil {
			if n.nodes.primary != nil {
				n.err = errors.Join(n.err, n.nodes.primary.stop())
			}
			if n.nodes.peer != nil {
				n.err = errors.Join(n.err, n.nodes.peer.stop())
			}
		}
	})
	return n.err
}
func (a *PoolRecoveryArchive) openSource(ctx context.Context) (*archiveNodeOwner, error) {
	owner := &archiveNodeOwner{nodes: &pilotNodes{}}
	ports := make([]uint16, 4)
	seen := map[uint16]bool{a.r.marker.PoolPort: true, a.r.marker.AdmissionPort: true}
	for i := range ports {
		for attempt := 0; attempt < 16; attempt++ {
			p, e := unusedPort()
			if e != nil {
				return owner, e
			}
			if !seen[p] {
				ports[i] = p
				seen[p] = true
				break
			}
		}
		if ports[i] == 0 {
			return owner, ErrPoolRuntime
		}
	}
	var e error
	owner.nodes.peer, e = startNodeProfile(a.r.config.NodeBinary, a.r.config.DataDir, "peer", ports[0], ports[1], ports[3], true, false)
	if e != nil {
		return owner, e
	}
	owner.nodes.primary, e = startNodeProfile(a.r.config.NodeBinary, a.r.config.DataDir, "primary", ports[2], 0, ports[1], false, true)
	if e != nil {
		return owner, e
	}
	if e = waitNode(ctx, owner.nodes.primary, true); e == nil {
		e = waitNode(ctx, owner.nodes.peer, false)
	}
	if e != nil {
		return owner, e
	}
	owner.client, e = prospectnode.NewClient(owner.nodes.rpcConfig())
	return owner, e
}
func (a *PoolRecoveryArchive) BeginSource(ctx context.Context) (*prospectnode.Client, prospectnode.RPCConfig, func() error, error) {
	if ctx == nil || ctx.Err() != nil || a.life.Err() != nil {
		return nil, prospectnode.RPCConfig{}, nil, ErrPoolRuntime
	}
	select {
	case a.gate <- struct{}{}:
	default:
		return nil, prospectnode.RPCConfig{}, nil, ErrPoolRuntimeBusy
	}
	if a.closeErr != nil || a.life.Err() != nil {
		<-a.gate
		return nil, prospectnode.RPCConfig{}, nil, ErrPoolRuntime
	}
	work, cancel := context.WithTimeout(ctx, 45*time.Second)
	stop := context.AfterFunc(a.life, cancel)
	owner, e := a.openSource(work)
	stop()
	cancel()
	closeOwner := func() error {
		ce := owner.Close()
		if ce != nil {
			a.failedSource = owner
			a.closeErr = errors.Join(a.closeErr, ce)
		}
		return ce
	}
	if e != nil {
		ce := closeOwner()
		<-a.gate
		return nil, prospectnode.RPCConfig{}, nil, errors.Join(e, ce)
	}
	var once sync.Once
	var ce error
	close := func() error { once.Do(func() { ce = closeOwner(); <-a.gate }); return ce }
	return owner.client, owner.nodes.rpcConfig(), close, nil
}
func (a *PoolRecoveryArchive) Close() error {
	if a == nil {
		return nil
	}
	a.once.Do(func() {
		a.cancel()
		a.gate <- struct{}{}
		defer func() { <-a.gate }()
		for i := len(a.owners) - 1; i >= 0; i-- {
			// The first owner is the dedicated lifetime lock, not semantic journal data.
			// Retain it after uncertain cleanup so no replacement can bypass this owner.
			if i == 0 && a.closeErr != nil {
				break
			}
			a.closeErr = errors.Join(a.closeErr, a.owners[i].Close())
		}
	})
	return a.closeErr
}
