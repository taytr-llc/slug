// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/chain"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
)

// Paths are native/CLI selections. Credentials and phrase literals are never
// accepted as command arguments or returned in a session status.
type RestoredGymConfig struct {
	Directory, RecoveryFile, PublicConfigFile, NodeConfigFile string
}
type restoredPublicConfig struct {
	Version    uint32               `json:"version"`
	Ownership  fighterownership.Pin `json:"ownership"`
	PublicRead restoredPublicRead   `json:"public_read"`
}
type RestoredGymStatus struct {
	SetupID      string                   `json:"setup_id"`
	GymID        string                   `json:"gym_id"`
	GymPublicKey string                   `json:"gym_public_key"`
	Phase        string                   `json:"phase"`
	Recovery     *wallet.RecoveryProgress `json:"recovery,omitempty"`
}

// RestoredGymSession owns a fresh, explicit file-restored namespace. It never
// borrows an account, provider, vault or old payment journal. The root lock and
// the original marker identities outlive every subordinate operation.
type RestoredGymSession struct {
	life           context.Context
	cancel         context.CancelFunc
	gate           chan struct{}
	closing        atomic.Bool
	poisoned       atomic.Bool
	closeOnce      sync.Once
	closeErr       error
	root           *restoredRoot
	dir            string
	initial        restoredSetupInit
	activation     *restoredSetupActivate
	ready          *restoredSetupReady
	nodeConfig     prospectnode.RPCConfig
	loader         *wallet.Loader
	pending        *wallet.RecoverySession
	installed      *wallet.Wallet
	carrier        *wallet.FighterCarrierOwner
	journal        *fightercarrier.Journal
	store          *fighterownership.Store
	reader         *fighterownership.PublicReader
	public         *fighterownership.PublicSource
	node           *fighterownership.OwnedChainSource
	backend        *chain.RPCClient
	recoverySource *wallet.RecoveryRPCSource
	cleanup        []io.Closer
	refresh        *restoredRefresh
	discovery      *restoredDiscovery
	selection      *restoredRecoverySelection
	prepared       *wallet.PreparedFighterTransfer
	review         *RestoredTransferReview
	deliveryReview *RestoredDeliveryReview
	deliveryNode   *withdrawalNode
	actionViewID   string
}

type RestoredGymCleanupError struct {
	owner *RestoredGymSession
	cause error
}

func (*RestoredGymCleanupError) Error() string   { return "restored gym cleanup failed" }
func (e *RestoredGymCleanupError) Unwrap() error { return e.cause }
func (e *RestoredGymCleanupError) Close() error  { return e }

type restoredCleanupCause struct{ cause error }

func (c *restoredCleanupCause) Close() error { return c.cause }
func (s *RestoredGymSession) rememberCleanup(e error) {
	var file *RestoredGymFileCleanupError
	var publication *recoveryfile.CleanupError
	var phase *wallet.PhaseARecoveryCleanupError
	var active *wallet.HostedAllocationCleanupError
	if errors.As(e, &file) || errors.As(e, &publication) || errors.As(e, &phase) || errors.As(e, &active) {
		// Retain the whole original error tree: independent failed resources may
		// occupy sibling branches, and errors.As returns only the first match.
		s.cleanup = append(s.cleanup, &restoredCleanupCause{cause: e})
		s.poisoned.Store(true)
	}
}
func (s *RestoredGymSession) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closing.Store(true)
		s.cancel()
		s.gate <- struct{}{}
		var e error
		e = errors.Join(e, s.clearRefresh(), s.clearActions(), s.clearDiscovery())
		if s.deliveryNode != nil {
			s.deliveryNode.close()
			s.deliveryNode.config = prospectnode.RPCConfig{}
		}
		if s.carrier != nil {
			e = errors.Join(e, s.carrier.Close())
		}
		if s.backend != nil {
			s.backend.Stop()
			s.backend.WaitForShutdown()
		}
		if s.pending != nil {
			e = errors.Join(e, s.pending.Close())
		}
		if s.installed != nil {
			e = errors.Join(e, s.loader.UnloadWallet())
			s.installed.Manager.Close()
		}
		if s.recoverySource != nil {
			e = errors.Join(e, s.recoverySource.Close())
		}
		if s.node != nil {
			e = errors.Join(e, s.node.Close())
		}
		if s.reader != nil {
			e = errors.Join(e, s.reader.Close())
		}
		if s.store != nil {
			e = errors.Join(e, s.store.Close())
		}
		if s.journal != nil {
			e = errors.Join(e, s.journal.Close())
		}
		for _, c := range s.cleanup {
			e = errors.Join(e, c.Close())
		}
		// A failed close retains this owner and the root's exclusion lock.
		if e == nil && s.root != nil {
			e = s.root.Close()
		}
		if e != nil {
			s.closeErr = &RestoredGymCleanupError{owner: s, cause: e}
		}
		s.nodeConfig = prospectnode.RPCConfig{}
	})
	return s.closeErr
}
func (s *RestoredGymSession) acquire(ctx context.Context, duration time.Duration) (context.Context, func(), error) {
	work, end, e := s.acquireGate(ctx, duration)
	if e != nil {
		return nil, nil, e
	}
	if s.refresh != nil {
		end()
		return nil, nil, fightercarrier.ErrBusy
	}
	return work, end, nil
}

// Only refresh progression/cancellation may enter while a refresh owns the
// original replay. All ordinary operations refuse until it completes or joins.
func (s *RestoredGymSession) acquireGate(ctx context.Context, duration time.Duration) (context.Context, func(), error) {
	if ctx == nil || s == nil || s.closing.Load() || s.poisoned.Load() {
		return nil, nil, ErrRestoredGym
	}
	select {
	case s.gate <- struct{}{}:
	default:
		return nil, nil, fightercarrier.ErrBusy
	}
	if s.closing.Load() || s.poisoned.Load() {
		<-s.gate
		return nil, nil, ErrRestoredGym
	}
	work, cancel := context.WithTimeout(ctx, duration)
	stop := context.AfterFunc(s.life, cancel)
	end := func() { stop(); cancel(); <-s.gate }
	if e := errors.Join(work.Err(), s.root.check()); e != nil {
		s.rememberCleanup(e)
		end()
		return nil, nil, errors.Join(ErrRestoredGym, e)
	}
	return work, end, nil
}
func restoredID() (string, error) {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", ErrRestoredGym
	}
	return hex.EncodeToString(b[:]), nil
}

func readRestoredConfig(ctx context.Context, c RestoredGymConfig) (restoredPublicConfig, prospectnode.RPCConfig, error) {
	var p restoredPublicConfig
	var node prospectnode.RPCConfig
	if wire.TestPoW || !restoredSupported() || ctx == nil || ctx.Err() != nil || !filepath.IsAbs(c.Directory) || filepath.Clean(c.Directory) != c.Directory {
		return p, node, ErrRestoredGym
	}
	public, e := readRestoredSelected(ctx, c.PublicConfigFile, c.Directory, 16384)
	if e != nil {
		return p, node, e
	}
	defer clear(public)
	if _, e = poolDecode(public, 16384, 10, &p); e != nil || p.Version != 1 {
		return p, node, ErrRestoredGym
	}
	if _, e = fighterownership.ValidatePin(p.Ownership); e != nil {
		return p, node, ErrRestoredGym
	}
	raw, e := readRestoredSelected(ctx, c.NodeConfigFile, c.Directory, 65536)
	if e != nil {
		return p, node, e
	}
	defer clear(raw)
	node, e = prospectnode.ParseRPCConfig(raw)
	if e != nil {
		return p, prospectnode.RPCConfig{}, ErrRestoredGym
	}
	if e = restoredValidateTransport(p.PublicRead, restoredNodeBinding{URL: node.URL, CACertificateSHA256: restoredSHA([]byte(node.CACertificate))}); e != nil {
		return p, prospectnode.RPCConfig{}, e
	}
	return p, node, nil
}
func readRestoredExport(ctx context.Context, c RestoredGymConfig) (recovery.Export, uint32, string, error) {
	raw, e := readRestoredSelected(ctx, c.RecoveryFile, c.Directory, recovery.MaxExportSize)
	if e != nil {
		return recovery.Export{}, 0, "", e
	}
	defer clear(raw)
	bundle, e := recovery.ParseExport(raw)
	if e != nil {
		return recovery.Export{}, 0, "", ErrRestoredGym
	}
	return bundle, uint32(len(raw)), restoredSHA(raw), nil
}
func newRestoredSession(ctx context.Context, c RestoredGymConfig, node prospectnode.RPCConfig) *RestoredGymSession {
	life, cancel := context.WithCancel(ctx)
	return &RestoredGymSession{life: life, cancel: cancel, gate: make(chan struct{}, 1), dir: c.Directory, nodeConfig: node, loader: wallet.NewLoader(&chaincfg.MainNetParams, filepath.Join(c.Directory, "wallet"), true, 5*time.Second, 0)}
}

func CreateRestoredGymSession(ctx context.Context, c RestoredGymConfig) (out *RestoredGymSession, resultErr error) {
	p, node, e := readRestoredConfig(ctx, c)
	if e != nil {
		return nil, e
	}
	bundle, n, fileHash, e := readRestoredExport(ctx, c)
	if e != nil {
		return nil, e
	}
	defer func() { bundle.Mnemonic = "" }()
	i := restoredSetupInit{Version: 1, Mode: "file-restored-gym", Ownership: p.Ownership, PublicRead: p.PublicRead, NodeBinding: restoredNodeBinding{node.URL, restoredSHA(nil)}, OriginalFileBytes: n, OriginalFileSHA256: fileHash, OriginalManifest: bundle.Manifest, RestoreSchema: 12, ActiveSchema: 15}
	for _, id := range []*string{&i.SetupID, &i.WalletOwnerID, &i.CarrierJournalID, &i.OwnershipStoreID} {
		*id, e = restoredID()
		if e != nil {
			return nil, e
		}
	}
	i.OwnershipPinID, e = fighterownership.ID(i.Ownership)
	if e != nil {
		return nil, e
	}
	raw, e := recovery.MarshalManifest(i.OriginalManifest)
	if e != nil {
		return nil, e
	}
	i.OriginalManifestSHA256 = restoredSHA(raw)
	f := i.OriginalManifest
	i.CarrierIdentity = restoredCarrierIdentity{i.CarrierJournalID, fightercarrier.Pin{Version: 1, OwnerMode: "restored", WalletOwnerID: i.WalletOwnerID, Network: f.Network, Genesis: f.Genesis, WalletProfile: f.Profile, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID, ManagerSchema: 15, OriginalManifestSHA256: i.OriginalManifestSHA256, OwnershipPinID: i.OwnershipPinID}}
	i.CarrierBindingID, e = fightercarrier.BindingID(i.CarrierIdentity.identity())
	if e != nil || restoredValidateInit(i, nil) != nil {
		return nil, ErrRestoredGym
	}
	s := newRestoredSession(ctx, c, node)
	defer func() {
		if resultErr != nil {
			s.rememberCleanup(resultErr)
			resultErr = errors.Join(ErrRestoredGym, resultErr, s.Close())
			out = nil
		}
	}()
	s.initial = i
	s.root, e = openRestoredRoot(c.Directory, true)
	if e != nil {
		return nil, e
	}
	if e = s.publishMarker(ctx, restoredInitName, i, restoredInitLimit); e != nil {
		return nil, e
	}
	if e = s.openSubordinates(ctx, false, false); e != nil {
		return nil, e
	}
	if _, e = s.root.walletStatus(true); e != nil {
		return nil, e
	}
	s.pending, e = s.loader.BeginPhaseARecovery(s.life, []byte(fixtureWalletPublicPass), []byte(fixtureWalletPrivatePass), bundle)
	if e != nil {
		s.rememberCleanup(e)
		return nil, e
	}
	if e = s.root.check(); e != nil {
		return nil, e
	}
	return s, nil
}

func (s *RestoredGymSession) publishMarker(ctx context.Context, name string, value any, limit int) error {
	if e := s.root.check(); e != nil {
		return e
	}
	raw, e := restoredMarkerBytes(value, limit)
	if e != nil {
		return e
	}
	selected, e := recoveryfile.Capture(filepath.Join(s.dir, name), nil, recoveryfile.CreateMode)
	if e != nil {
		return errors.Join(ErrRestoredGymStorage, e)
	}
	h, e := recoveryfile.Open(selected.Pin(), nil)
	if e != nil {
		return restoredClose(errors.Join(ErrRestoredGymStorage, e), selected)
	}
	e = h.Publish(ctx, raw, func(commit func(func() error) error) error {
		return commit(func() error { return errors.Join(ctx.Err(), s.root.check()) })
	})
	e = restoredClose(e, h, selected)
	if e != nil {
		s.rememberCleanup(e)
		s.poisoned.Store(true)
		return errors.Join(ErrRestoredGymStorage, e)
	}
	return s.root.check()
}
func (s *RestoredGymSession) readMarker(ctx context.Context, name string, limit int, out any) error {
	if e := s.root.check(); e != nil {
		return e
	}
	raw, e := readRestoredSelected(ctx, filepath.Join(s.dir, name), "", limit)
	if e != nil {
		return e
	}
	defer clear(raw)
	if e = restoredParseMarker(raw, limit, out); e != nil {
		return e
	}
	return s.root.check()
}
func (s *RestoredGymSession) openSubordinates(ctx context.Context, requireExisting, ready bool) error {
	items, e := s.root.inventory()
	if e != nil {
		return e
	}
	if requireExisting && (!items["carrier"] || !items["ownership"]) {
		return ErrRestoredGymStorage
	}
	if items["carrier"] {
		s.journal, e = fightercarrier.Open(s.life, filepath.Join(s.dir, "carrier"), s.initial.CarrierIdentity.identity())
	} else {
		s.journal, e = fightercarrier.Create(s.life, filepath.Join(s.dir, "carrier"), s.initial.CarrierIdentity.identity())
	}
	if e != nil {
		return e
	}
	j, e := s.journal.TryAcquire(ctx)
	if e != nil {
		return e
	}
	snap, e := j.Snapshot()
	ce := j.Close()
	if e != nil || ce != nil || !ready && snap.Count != 0 {
		return ErrRestoredGymStorage
	}
	id := fighterownership.StoreIdentity{StoreID: s.initial.OwnershipStoreID}
	if items["ownership"] {
		s.store, e = fighterownership.Open(filepath.Join(s.dir, "ownership"), s.initial.Ownership, id)
	} else {
		s.store, e = fighterownership.Create(filepath.Join(s.dir, "ownership"), s.initial.Ownership, id)
	}
	if e != nil {
		return e
	}
	if !ready {
		empty, e := s.store.Empty(ctx)
		if e != nil || !empty {
			return ErrRestoredGymStorage
		}
	}
	return s.root.check()
}

func OpenRestoredGymSession(ctx context.Context, c RestoredGymConfig) (out *RestoredGymSession, resultErr error) {
	p, node, e := readRestoredConfig(ctx, c)
	if e != nil {
		return nil, e
	}
	s := newRestoredSession(ctx, c, node)
	defer func() {
		if resultErr != nil {
			s.rememberCleanup(resultErr)
			resultErr = errors.Join(ErrRestoredGym, resultErr, s.Close())
			out = nil
		}
	}()
	s.root, e = openRestoredRoot(c.Directory, false)
	if e != nil {
		return nil, e
	}
	items, e := s.root.inventory()
	if e != nil || !items[restoredInitName] {
		return nil, errors.Join(ErrRestoredGymStorage, e)
	}
	if e = s.readMarker(ctx, restoredInitName, restoredInitLimit, &s.initial); e != nil || restoredValidateInit(s.initial, nil) != nil {
		return nil, errors.Join(ErrRestoredGymStorage, e)
	}
	if !restoredEqual(s.initial.Ownership, p.Ownership) || s.initial.PublicRead != p.PublicRead || s.initial.NodeBinding != (restoredNodeBinding{node.URL, restoredSHA(nil)}) {
		return nil, errors.Join(ErrRestoredGym, e)
	}
	if items[restoredActivateName] {
		s.activation = &restoredSetupActivate{}
		if e = s.readMarker(ctx, restoredActivateName, restoredActivateLimit, s.activation); e != nil || restoredValidateActivate(*s.activation, s.initial, nil) != nil {
			return nil, errors.Join(ErrRestoredGymStorage, e)
		}
	}
	if items[restoredReadyName] {
		if s.activation == nil {
			return nil, errors.Join(ErrRestoredGymStorage, e)
		}
		s.ready = &restoredSetupReady{}
		if e = s.readMarker(ctx, restoredReadyName, restoredReadyLimit, s.ready); e != nil || restoredValidateReady(*s.ready, s.initial, *s.activation) != nil {
			return nil, errors.Join(ErrRestoredGymStorage, e)
		}
	}
	var bundle recovery.Export
	if s.ready == nil || c.RecoveryFile != "" {
		var n uint32
		var h string
		bundle, n, h, e = readRestoredExport(ctx, c)
		if e != nil || n != s.initial.OriginalFileBytes || h != s.initial.OriginalFileSHA256 || !restoredEqual(bundle.Manifest, s.initial.OriginalManifest) {
			return nil, errors.Join(ErrRestoredGym, e)
		}
		defer func() { bundle.Mnemonic = "" }()
	}
	exists, e := s.root.walletStatus(false)
	if e != nil {
		return nil, e
	}
	if (s.ready != nil || s.activation != nil) && !exists {
		return nil, errors.Join(ErrRestoredGymStorage, e)
	}
	if e = s.openSubordinates(ctx, exists, s.ready != nil); e != nil {
		return nil, e
	}
	if s.activation != nil {
		s.installed, e = s.loader.OpenRestoredCarrierWallet(ctx, []byte(fixtureWalletPublicPass), s.activation.Guard, s.journal)
		var cleanup *wallet.HostedAllocationCleanupError
		if e != nil && s.ready == nil && !errors.As(e, &cleanup) && errors.Is(e, waddrmgr.ErrHostedAllocationProfile) {
			s.installed, e = s.loader.ActivateRestoredCarrierWalletExpected(ctx, []byte(fixtureWalletPublicPass), s.activation.Guard, s.journal)
		}
		if e != nil {
			s.rememberCleanup(e)
			return nil, e
		}
		if s.ready == nil {
			if e = s.publishReady(ctx); e != nil {
				return nil, e
			}
		}
		if e = s.openActiveOwner(ctx); e != nil {
			return nil, e
		}
	} else if exists {
		s.pending, e = s.loader.OpenPhaseARecovery(s.life, []byte(fixtureWalletPublicPass), s.initial.OriginalManifest)
	} else {
		if _, e = s.root.walletStatus(true); e != nil {
			return nil, e
		}
		s.pending, e = s.loader.BeginPhaseARecovery(s.life, []byte(fixtureWalletPublicPass), []byte(fixtureWalletPrivatePass), bundle)
	}
	if e != nil {
		s.rememberCleanup(e)
		return nil, e
	}
	return s, s.root.check()
}

func (s *RestoredGymSession) publishReady(ctx context.Context) error {
	a := s.activation
	if a == nil || s.installed == nil {
		return ErrRestoredGymStorage
	}
	r := restoredSetupReady{1, s.initial.SetupID, a.InitDigest, restoredDigest("activation", *a), a.GuardDigest, s.initial.CarrierBindingID, s.initial.OwnershipPinID, s.initial.OwnershipStoreID}
	if e := restoredValidateReady(r, s.initial, *a); e != nil {
		return e
	}
	if e := s.publishMarker(ctx, restoredReadyName, r, restoredReadyLimit); e != nil {
		return e
	}
	s.ready = &r
	return nil
}
func (s *RestoredGymSession) openActiveOwner(ctx context.Context) error {
	var e error
	s.carrier, e = s.installed.OpenRestoredFighterCarrier(ctx, s.journal, s.activation.Guard)
	if e != nil {
		return e
	}
	return s.root.check()
}

func (s *RestoredGymSession) Status(ctx context.Context) (RestoredGymStatus, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredGymStatus{}, e
	}
	defer end()
	v := RestoredGymStatus{SetupID: s.initial.SetupID, GymID: s.initial.OriginalManifest.GymID, GymPublicKey: s.initial.OriginalManifest.GymPublicKey, Phase: "pending"}
	if s.ready != nil {
		v.Phase = "ready"
	} else if s.pending != nil {
		p, e := s.pending.Status(work)
		if e != nil {
			return RestoredGymStatus{}, e
		}
		v.Recovery = &p
	}
	return v, work.Err()
}

func (s *RestoredGymSession) Restore(ctx context.Context) (RestoredGymStatus, error) {
	work, end, e := s.acquire(ctx, 10*time.Minute)
	if e != nil {
		return RestoredGymStatus{}, e
	}
	e = func() error {
		if s.ready != nil {
			return nil
		}
		if s.pending == nil {
			return ErrRestoredGymStorage
		}
		u, _ := url.Parse(s.nodeConfig.URL)
		u.Scheme = "ws"
		u.Path = "/ws"
		if s.recoverySource == nil {
			s.recoverySource, e = wallet.NewRecoveryRPCSource(wallet.RecoveryRPCConfig{URL: u.String(), Username: s.nodeConfig.Username, Password: s.nodeConfig.Password})
			if e != nil {
				return e
			}
		}
		for {
			p, e := s.pending.Status(work)
			if e != nil {
				return e
			}
			if p.Ready {
				break
			}
			if _, e = s.pending.Step(work, s.recoverySource); e != nil {
				return e
			}
		}
		if e = s.pending.Close(); e != nil {
			s.rememberCleanup(e)
			return e
		}
		s.pending = nil
		guard, e := s.loader.InspectRestoredCarrierActivation(work, []byte(fixtureWalletPublicPass), s.initial.OriginalManifest, s.journal)
		if e != nil {
			s.rememberCleanup(e)
			return e
		}
		digest, e := waddrmgr.RestoredCarrierGuardDigest(guard)
		if e != nil {
			return e
		}
		a := restoredSetupActivate{1, s.initial.SetupID, restoredDigest("setup", s.initial), guard, digest}
		if e = s.publishMarker(work, restoredActivateName, a, restoredActivateLimit); e != nil {
			return e
		}
		s.activation = &a
		s.installed, e = s.loader.ActivateRestoredCarrierWalletExpected(work, []byte(fixtureWalletPublicPass), guard, s.journal)
		if e != nil {
			s.rememberCleanup(e)
			s.poisoned.Store(true)
			return e
		}
		if e = s.publishReady(work); e != nil {
			return e
		}
		return s.openActiveOwner(work)
	}()
	if e != nil {
		s.rememberCleanup(e)
	}
	end()
	if e != nil {
		return RestoredGymStatus{}, errors.Join(ErrRestoredGym, e)
	}
	return s.Status(ctx)
}

// Files are opened from exact external selections; never marshal this private
// configuration or include source errors/paths in a user-visible response.
func (s *RestoredGymSession) connect(ctx context.Context) error {
	if s.ready == nil || s.installed == nil {
		return ErrRestoredGym
	}
	var e error
	if s.node == nil {
		s.node, e = fighterownership.NewOwnedChainSource(ctx, s.initial.Ownership, s.nodeConfig, s.life)
		if e != nil {
			return e
		}
	}
	if s.reader == nil {
		s.reader, e = fighterownership.NewPublicReader(ctx, s.initial.Ownership, fighterownership.PublicReadConfig{URL: s.initial.PublicRead.URL, TLSSPKISHA256: s.initial.PublicRead.TLSSPKISHA256}, s.life)
		if e != nil {
			return e
		}
	}
	if s.backend == nil {
		u, _ := url.Parse(s.nodeConfig.URL)
		backend, e := chain.NewRPCClient(&chaincfg.MainNetParams, u.Host, s.nodeConfig.Username, s.nodeConfig.Password, nil, true, 0)
		if e != nil {
			return e
		}
		if e = backend.Start(ctx); e != nil {
			backend.Stop()
			backend.WaitForShutdown()
			return e
		}
		s.backend = backend
		s.installed.SynchronizeRPC(&ownedLocalChain{RPCClient: backend})
	}
	return s.root.check()
}

// Refresh preserves the original one-shot30-second contract. The CLI uses
// BeginRefresh/RefreshStep to retain the same verification across bounded slices.
func (s *RestoredGymSession) Refresh(ctx context.Context) (head fighterownership.HeadRef, resultErr error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return fighterownership.HeadRef{}, e
	}
	defer func() { resultErr = errors.Join(resultErr, s.clearRefresh()); s.rememberCleanup(resultErr); end() }()
	if e = s.connect(work); e != nil {
		return fighterownership.HeadRef{}, errors.Join(ErrRestoredGym, e)
	}
	if _, e = s.beginRefresh(work, work); e != nil {
		return fighterownership.HeadRef{}, e
	}
	for {
		p, e := s.stepRefresh(work, 128)
		if e != nil {
			return fighterownership.HeadRef{}, e
		}
		if p.Ready {
			return *p.Head, nil
		}
	}
}
