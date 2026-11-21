// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/poolingress"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolschedule"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlenode"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/wallet"
	// The dedicated wallet selects bdb. Register it in the concrete host,
	// independently of test imports or the graphical desktop pilot.
	_ "github.com/taytr-llc/slugwallet/walletdb/bdb"
)

// BorrowedOwners must outlive successful Runtime.Close. They are never replaced
// or closed by Runtime. Their custody contexts must remain alive while Run is
// cancelled and Runtime.Close joins/locks; cancel custody only afterwards. The selected wallet's backend attachment is one-use:
// after any constructor attempt that attached it, close/reopen the wallet before
// constructing a replacement runtime, even if cleanup of that attempt succeeded.
type BorrowedOwners struct {
	Authority *operatorauthority.Owner
	Wallet    *wallet.PoolWalletOwner
}

// PrivateInputs are local selections, never included in Setup, Status or errors.
// NodeRPC is copied into concrete clients once. Selected TLS/passphrase files
// remain retained and are never reselected after path replacement.
type PrivateInputs struct {
	NodeRPC              prospectnode.RPCConfig
	TLSCertificateFile   string
	TLSPrivateKeyFile    string
	UnlockPassphraseFile string
}

func (PrivateInputs) String() string   { return "pool operator private input selections" }
func (PrivateInputs) GoString() string { return "pool operator private input selections" }

// Create creates or resumes the same incomplete intent. Open requires READY and
// all established children. ctx is the original runtime lifetime, not a deadline
// that may be substituted after construction. No constructor starts a service,
// miner, scheduler or custody-renewal goroutine.
func Create(ctx context.Context, root string, expected Setup, borrowed BorrowedOwners, private PrivateInputs) (*Runtime, error) {
	return construct(ctx, root, expected, borrowed, private, true)
}
func Open(ctx context.Context, root string, expected Setup, borrowed BorrowedOwners, private PrivateInputs) (*Runtime, error) {
	return construct(ctx, root, expected, borrowed, private, false)
}

func authorityBinding(p registry.OperatorProfile) (operatorauthority.Binding, error) {
	pin := p.Pin()
	genesis, e := chainhash.NewHashFromStr(pin.Source.Genesis)
	if e != nil {
		return operatorauthority.Binding{}, ErrInvalid
	}
	return operatorauthority.Binding{Version: 2, PinID: p.ID(), Network: wire.BitcoinNet(pin.Source.Network), Genesis: *genesis, Keys: operatorauthority.Keys{Admission: pin.AdmissionKey, Publication: pin.PublicationKey, Cutoff: pin.CutoffKey, Checkpoint: pin.CheckpointKey}}, nil
}
func construct(ctx context.Context, root string, expected Setup, borrowed BorrowedOwners, private PrivateInputs, create bool) (*Runtime, error) {
	return constructVersion(ctx, root, expected, borrowed, private, create, false)
}
func constructVersion(ctx context.Context, root string, expected Setup, borrowed BorrowedOwners, private PrivateInputs, create, daily bool) (r *Runtime, err error) {
	if ctx == nil || borrowed.Authority == nil || borrowed.Wallet == nil || wire.TestPoW {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	version := uint32(1)
	if daily {
		version = 2
	}
	raw, e := marshalSetupVersion(expected, version)
	if e != nil {
		return nil, e
	}
	owned, e := parseSetupVersion(raw, version)
	if e != nil {
		return nil, e
	}
	var reservation CapacityReservation
	var dailyReservation *DailyCapacityReservation
	if daily {
		v, x := DailyCapacityEnvelope(owned)
		if x != nil {
			return nil, x
		}
		dailyReservation = &v
	} else {
		reservation, e = CapacityEnvelope(owned)
		if e != nil {
			return nil, e
		}
	}
	profile, e := registry.NewOperatorProfile(owned.Authority)
	if e != nil {
		return nil, e
	}
	binding, e := authorityBinding(profile)
	if e != nil {
		return nil, e
	}
	if borrowed.Authority.Capabilities() != (operatorauthority.Capabilities{Admission: true, Publication: true, Cutoff: true}) || borrowed.Authority.Check(ctx, binding) != nil || !reflect.DeepEqual(borrowed.Wallet.Binding(), owned.walletBinding()) {
		return nil, ErrInvalid
	}
	rpc := private.NodeRPC
	if len(rpc.URL)+len(rpc.Username)+len(rpc.Password)+len(rpc.CACertificate) > prospectnode.MaxRPCConfigSize {
		return nil, ErrInvalid
	}
	rpcRaw, e := json.Marshal(rpc)
	if e != nil {
		return nil, ErrInvalid
	}
	if _, e = prospectnode.ParseRPCConfig(rpcRaw); e != nil {
		return nil, ErrInvalid
	}
	life, cancel := context.WithCancel(ctx)
	id, _ := setupIDVersion(owned, version)
	r = &Runtime{daily: daily, dailyReservation: dailyReservation, setup: owned, setupID: id, profile: profile, borrowed: borrowed, binding: binding, life: life, cancel: cancel, runDone: make(chan struct{}), localGate: make(chan struct{}, 1)}
	r.reservation = reservation
	r.status = Status{Version: version, RuntimeID: owned.RuntimeID, SetupID: id, Custody: "locked", Capacity: CapacityStatus{Reservation: reservation, DailyReservation: dailyReservation, Mode: "paused"}, PublicCode: "not_started", RoundCode: "not_started"}
	defer func() {
		if err != nil {
			// Concrete child cleanup errors carry their original owner. Keep that cause
			// and the outer root reachable; closing another owner cannot release it.
			var credentialChild *poolminer.CleanupError
			var schedulerChild *poolschedule.CleanupError
			if errors.As(err, &credentialChild) || errors.As(err, &schedulerChild) {
				r.unreleasedChild = err
			}
			original := err
			if ce := r.Close(); ce != nil {
				err = errors.Join(original, ce)
				return
			}
			r = nil
		}
	}()
	r.files, err = openOperatorFilesVersion(root, owned, create, version)
	if err != nil {
		return r, err
	}
	r.tlsCertificate, err = selectPrivateInput(ctx, private.TLSCertificateFile, 65536)
	if err != nil {
		return r, err
	}
	r.tlsKey, err = selectPrivateInput(ctx, private.TLSPrivateKeyFile, 16384)
	if err != nil {
		return r, err
	}
	r.unlockInput, err = selectPrivateInput(ctx, private.UnlockPassphraseFile, 1024)
	if err != nil {
		return r, err
	}
	cert, e := r.tlsCertificate.read(ctx)
	if e != nil {
		return r, e
	}
	defer clear(cert)
	key, e := r.tlsKey.read(ctx)
	if e != nil {
		return r, e
	}
	defer clear(key)
	newIngress := poolingress.New
	if daily {
		newIngress = poolingress.NewDaily
	}
	r.ingress, err = newIngress(owned.Ingress, cert, key)
	if err != nil {
		return r, err
	}
	r.node, err = prospectnode.NewClient(rpc)
	if err != nil {
		return r, ErrUnavailable
	}
	r.chain, err = fighterownership.NewOperatorOwnedChainSource(ctx, profile, rpc, life)
	if err != nil {
		return r, err
	}
	newFunding := poolsettlenode.New
	newBackend := wallet.NewPoolSettlementBackend
	if daily {
		newFunding = poolsettlenode.NewDaily
		newBackend = wallet.NewDailyPoolSettlementBackend
	}
	r.fundingNode, err = newFunding(life, owned.Node, rpc)
	if err != nil {
		return r, err
	}
	r.backend, err = newBackend(life, borrowed.Wallet, r.fundingNode, owned.walletBinding())
	if err != nil {
		return r, err
	}
	if err = r.constructStores(ctx); err != nil {
		return r, err
	}
	bindBackend := r.backend.BindSettlement
	if daily {
		bindBackend = r.backend.BindDailySettlement
	}
	if err = bindBackend(ctx, r.settlement); err != nil {
		return r, err
	}
	if daily {
		gyms := make([]string, len(owned.Settlement.Destinations))
		for i, d := range owned.Settlement.Destinations {
			gyms[i] = d.GymID
		}
		r.intake, err = r.pool.AttachDailyOperatorAdmission(ctx, owned.Pool, poolshare.DailyRecoveryBudget{Rounds: owned.RoundLimits.MaxRounds, RoundBytes: owned.RoundLimits.MaxBytes, CreditRounds: uint64(owned.Settlement.MaxRounds), Batches: uint64(owned.Settlement.MaxBatches), GymIDs: gyms})
		if err != nil {
			return r, err
		}
	}
	if err = r.constructServices(); err != nil {
		return r, err
	}
	r.public = publicMaintenance{profile: profile, owner: borrowed.Authority, publisher: r.publisher, core: r.ownership, node: r.chain}
	r.round = roundMaintenance{store: r.rounds}
	if err = r.checkInputs(ctx); err != nil {
		return r, err
	}
	if err = r.files.markReady(); err != nil {
		return r, err
	}
	return r, nil
}

func (r *Runtime) constructStores(ctx context.Context) error {
	path, fresh, e := r.files.child("miners")
	if e != nil {
		return e
	}
	createMiners, openMiners := poolminer.Create, poolminer.Open
	if r.daily {
		createMiners, openMiners = poolminer.CreateDaily, poolminer.OpenDaily
	}
	if fresh {
		r.miners, e = createMiners(ctx, path, r.setup.Miners, r.setup.minerBinding(r.profile))
	} else {
		r.miners, e = openMiners(ctx, path, r.setup.Miners, r.setup.minerBinding(r.profile))
	}
	if e != nil {
		return e
	}
	if e = r.files.adopt("miners"); e != nil {
		return e
	}
	path, fresh, e = r.files.child("issuer")
	if e != nil {
		return e
	}
	if fresh {
		descriptor, x := fighterdescriptor.ResolveCandidate(r.setup.Authority.Source.DescriptorID)
		if x != nil {
			return x
		}
		r.issuer, e = registry.CreateOperatorManifestAuthority(path, descriptor, r.profile, r.borrowed.Authority)
	} else {
		r.issuer, e = registry.OpenOperatorManifestAuthority(path, r.profile, r.borrowed.Authority)
	}
	if e != nil {
		return e
	}
	if e = r.files.adopt("issuer"); e != nil {
		return e
	}
	r.publisher, e = r.issuer.OperatorPublication(r.profile)
	if e != nil {
		return e
	}
	path, fresh, e = r.files.child("pool")
	if e != nil {
		return e
	}
	if r.daily {
		if fresh {
			r.pool, e = poolshare.CreateDaily(ctx, path, r.setup.Pool, r.issuer, r.node, r.miners.PoolSessions())
		} else {
			r.pool, e = poolshare.OpenDaily(ctx, path, r.setup.Pool, r.issuer, r.node, r.miners.PoolSessions())
		}
	} else if fresh {
		r.pool, e = poolshare.Create(path, r.setup.Pool, r.issuer, r.node, r.miners.PoolSessions())
	} else {
		r.pool, e = poolshare.Open(path, r.setup.Pool, r.issuer, r.node, r.miners.PoolSessions())
	}
	if e != nil {
		return e
	}
	if e = r.files.adopt("pool"); e != nil {
		return e
	}
	if !r.daily {
		r.intake, e = r.pool.AttachOperatorAdmission(ctx, r.setup.Pool, r.reservation.Submissions)
		if e != nil {
			return e
		}
	}
	path, fresh, e = r.files.child("rounds")
	if e != nil {
		return e
	}
	if r.daily {
		if fresh {
			r.rounds, e = poolround.CreateDaily(ctx, path, r.setup.RoundLimits, r.pool, r.node)
		} else {
			r.rounds, e = poolround.OpenDaily(ctx, path, r.setup.RoundLimits, r.pool, r.node)
		}
	} else if fresh {
		r.rounds, e = poolround.Create(path, r.setup.RoundLimits, r.pool, r.node)
	} else {
		r.rounds, e = poolround.Open(path, r.setup.RoundLimits, r.pool, r.node)
	}
	if e != nil {
		return e
	}
	if e = r.files.adopt("rounds"); e != nil {
		return e
	}
	path, fresh, e = r.files.child("settlement")
	if e != nil {
		return e
	}
	if r.daily {
		if fresh {
			r.settlement, e = poolsettle.CreateDaily(ctx, path, r.setup.Settlement, r.rounds, r.backend)
		} else {
			r.settlement, e = poolsettle.OpenDaily(ctx, path, r.setup.Settlement, r.rounds, r.backend)
		}
	} else if fresh {
		r.settlement, e = poolsettle.Create(path, r.setup.Settlement, r.rounds, r.backend)
	} else {
		r.settlement, e = poolsettle.Open(path, r.setup.Settlement, r.rounds, r.backend)
	}
	if e != nil {
		return e
	}
	if e = r.files.adopt("settlement"); e != nil {
		return e
	}
	path, fresh, e = r.files.child("scheduler")
	if e != nil {
		return e
	}
	createSchedule, openSchedule := poolschedule.Create, poolschedule.Open
	if r.daily {
		createSchedule, openSchedule = poolschedule.CreateDaily, poolschedule.OpenDaily
	}
	if fresh {
		r.schedule, e = createSchedule(ctx, path, r.setup.Schedule, r.settlement)
	} else {
		r.schedule, e = openSchedule(ctx, path, r.setup.Schedule, r.settlement)
	}
	if e != nil {
		return e
	}
	if e = r.files.adopt("scheduler"); e != nil {
		return e
	}
	path, fresh, e = r.files.child("ownership")
	if e != nil {
		return e
	}
	identity := fighterownership.StoreIdentity{StoreID: r.setup.OwnershipStoreID}
	if fresh {
		r.ownership, e = fighterownership.CreateOperator(path, r.profile, identity, r.borrowed.Authority)
	} else {
		r.ownership, e = fighterownership.OpenOperator(path, r.profile, identity, r.borrowed.Authority)
	}
	if e != nil {
		return e
	}
	if e = r.files.adopt("ownership"); e != nil {
		return e
	}
	// All original concrete journals, scheduler and custody bindings have been
	// reopened and audited before granting the canonical-only recovery lane.
	// The ordinary reservation and every persisted setup limit stay unchanged.
	if r.daily {
		return nil
	}
	return r.intake.EnableCanonicalRecovery(ctx, poolshare.RecoveryBudget{
		Rounds: r.setup.RoundLimits.MaxRounds, RoundBytes: r.setup.RoundLimits.MaxBytes,
		CreditRounds: uint64(r.setup.Settlement.MaxRounds), Batches: uint64(r.setup.Settlement.MaxBatches),
		Gyms: uint64(len(r.setup.Settlement.Destinations)),
	})
}
func privateListen(origin string) string { u, _ := url.Parse(origin); return u.Host }
func (r *Runtime) constructServices() error {
	var e error
	if r.daily {
		r.poolServer, e = poolservice.NewDailyServer(r.life, poolservice.ServerConfig{Version: 2, Listen: privateListen(r.setup.Ingress.PoolURL)}, r.pool, r.settlement, r.miners.PoolSessions())
	} else {
		r.poolServer, e = poolservice.NewServer(poolservice.ServerConfig{Version: 1, Listen: privateListen(r.setup.Ingress.PoolURL)}, r.pool, r.settlement, r.miners.PoolSessions())
	}
	if e != nil {
		return e
	}
	r.admissionServer, e = prospectservice.NewPausedFencedManifestAdmissionServer(prospectservice.ManifestServerConfig{Version: 3, Listen: privateListen(r.setup.Ingress.AdmissionURL), AuthorityPin: r.setup.Pool.AuthorityPin, Clients: []prospectservice.Credential{}, MaxJobs: r.setup.Pool.MaxWorks, MaxRecords: uint64(r.setup.Authority.Limits.Admissions)}, r.setup.Pool.Policy, r.node, r.issuer, r.miners.AdmissionSessions())
	if e != nil {
		return e
	}
	r.publicServer, e = fighterownership.NewOperatorPublicReadServer(r.profile, r.publisher, r.ownership, r.life)
	return e
}
