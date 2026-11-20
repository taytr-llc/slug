// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/registry"
)

const poolLeagueDirectory = "league-v1"
const poolLeagueIntentName = "SLUG-POOL-LEAGUE-INTENT.json"
const poolLeagueReadyName = "SLUG-POOL-LEAGUE-READY.json"

// These are private, immutable dependencies. Only the explicitly tagged test
// constructor installs another clock; neither Config nor the wire can do so.
type poolLeagueFactories struct {
	poolPipelineFactories
	createStable func(string, gymstable.Pin, *registry.ManifestStore) (*gymstable.Store, error)
	openStable   func(string, gymstable.Pin, *registry.ManifestStore) (*gymstable.Store, error)
	createEntry  func(context.Context, string, gymentry.Pin, *gymstable.Store) (*gymentry.Store, error)
	openEntry    func(context.Context, string, gymentry.Pin, *gymstable.Store) (*gymentry.Store, error)
	createEvent  func(context.Context, string, gymevent.Pin) (*gymevent.Store, error)
	openEvent    func(context.Context, string, gymevent.Pin) (*gymevent.Store, error)
	resumeEvent  func(context.Context, string, gymevent.Pin) (*gymevent.Store, error)
}

func defaultPoolLeagueFactories() *poolLeagueFactories {
	return &poolLeagueFactories{poolPipelineFactories: defaultPoolPipelineFactories(), createStable: gymstable.CreateShared, openStable: gymstable.OpenShared, createEntry: gymentry.Create, openEntry: gymentry.Open, createEvent: gymevent.Create, openEvent: gymevent.Open, resumeEvent: gymevent.ResumePrepared}
}

type poolLeagueChildBinding struct {
	SourceBindingID string
	EntryPin        gymentry.Pin
	EntryStoreID    string
	EventPin        gymevent.Pin
	EventStoreID    string
	EventCreatedMS  int64
}

type poolLeagueParticipant struct {
	AccountID      string       `json:"account_id"`
	StableExisting bool         `json:"stable_existing"`
	EntryPin       gymentry.Pin `json:"entry_pin"`
	EntryStoreID   string       `json:"entry_store_id"`
}

type poolLeagueIntent struct {
	Application     string                  `json:"application"`
	Version         uint32                  `json:"version"`
	SourceBindingID string                  `json:"source_binding_id"`
	Participants    []poolLeagueParticipant `json:"participants"`
	EventPin        gymevent.Pin            `json:"event_pin"`
	EventStoreID    string                  `json:"event_store_id"`
}

type poolLeagueReady struct {
	Version        uint32           `json:"version"`
	Intent         poolLeagueIntent `json:"intent"`
	EventCreatedMS int64            `json:"event_created_ms"`
}

type poolLeagueParticipantReady struct {
	Version         uint32                `json:"version"`
	SourceBindingID string                `json:"source_binding_id"`
	Participant     poolLeagueParticipant `json:"participant"`
}

func poolStablePin(b PoolRecipientBinding) gymstable.Pin {
	return gymstable.Pin{Version: gymstable.SharedPinVersion2, AccountAuthorityID: b.Config.AccountAuthorityID, AccountID: b.Config.AccountID, Identity: b.Config.Identity, Source: b.Config.Source.AuthorityPin, ConditionVersion: fightercondition.Version1, ProjectorID: fightercondition.ProjectorID}
}

// leagueIntent derives authority from the already verified runtime recipients.
// Disk supplies only the recorded initial existence bit, never a replacement pin.
func (r *PoolRuntime) leagueIntent(recorded *poolLeagueIntent) (poolLeagueIntent, error) {
	ready, e := r.readyRecord()
	if e != nil {
		return poolLeagueIntent{}, e
	}
	v := poolLeagueIntent{Application: "slug-shared-pool-league", Version: 1, SourceBindingID: ready.Binding, Participants: []poolLeagueParticipant{}, EventPin: gymevent.Pin{Version: 1, CalendarID: gymentry.CalendarID(), RulesID: gymevent.RulesID(), Participants: []gymentry.Pin{}}}
	for _, b := range ready.Recipients {
		if b.Config.Source != r.source || b.Config.AccountAuthorityID != r.marker.AccountAuthorityID {
			return poolLeagueIntent{}, ErrPoolRuntime
		}
		sp, e := gymstable.ValidatePin(poolStablePin(b))
		if e != nil {
			return poolLeagueIntent{}, e
		}
		sid, e := gymstable.ID(sp)
		if e != nil {
			return poolLeagueIntent{}, e
		}
		namespace, e := gymentry.NamespaceID(sp.Source.Network, sp.Source.Genesis, sp.AccountAuthorityID)
		if e != nil {
			return poolLeagueIntent{}, e
		}
		ep, e := gymentry.ValidatePin(gymentry.Pin{Version: 1, NamespaceID: namespace, CalendarID: gymentry.CalendarID(), RulesID: gymentry.RulesID(), StableStoreID: sid, StablePin: sp})
		if e != nil {
			return poolLeagueIntent{}, e
		}
		eid, _ := gymentry.ID(ep)
		x := poolLeagueParticipant{AccountID: sp.AccountID, EntryPin: ep, EntryStoreID: eid}
		if recorded != nil {
			if len(recorded.Participants) != 2 {
				return poolLeagueIntent{}, ErrPoolRuntime
			}
			found := false
			for _, p := range recorded.Participants {
				if p.AccountID == x.AccountID {
					if found {
						return poolLeagueIntent{}, ErrPoolRuntime
					}
					x.StableExisting, found = p.StableExisting, true
				}
			}
			if !found {
				return poolLeagueIntent{}, ErrPoolRuntime
			}
		} else {
			root := filepath.Join(r.config.DataDir, "gyms", x.AccountID)
			x.StableExisting, e = poolLeagueDirectoryExists(filepath.Join(root, "stable"))
			if e != nil {
				return poolLeagueIntent{}, e
			}
			// A pre-existing completed child has already established this store.
			childReady, e := poolRuntimeExists(filepath.Join(root, "SLUG-POOL-CHILD-READY.json"))
			if e != nil || childReady && !x.StableExisting {
				return poolLeagueIntent{}, ErrPoolRuntime
			}
			if exists, e := poolRuntimeExists(filepath.Join(root, "entries")); e != nil || exists {
				return poolLeagueIntent{}, ErrPoolRuntime
			}
		}
		v.Participants = append(v.Participants, x)
		v.EventPin.Participants = append(v.EventPin.Participants, ep)
	}
	sort.Slice(v.Participants, func(i, j int) bool { return v.Participants[i].AccountID < v.Participants[j].AccountID })
	sort.Slice(v.EventPin.Participants, func(i, j int) bool {
		return v.EventPin.Participants[i].StablePin.Identity.GymID < v.EventPin.Participants[j].StablePin.Identity.GymID
	})
	v.EventPin, e = gymevent.ValidatePin(v.EventPin)
	if e != nil {
		return poolLeagueIntent{}, e
	}
	v.EventStoreID, e = gymevent.ID(v.EventPin)
	if e != nil || recorded != nil && !reflect.DeepEqual(v, *recorded) {
		return poolLeagueIntent{}, ErrPoolRuntime
	}
	return v, nil
}

func poolLeagueDirectoryExists(path string) (bool, error) {
	i, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return false, ErrPoolRuntime
	}
	return true, nil
}

func poolLeagueEmpty(path string) bool {
	f, e := os.Open(path)
	if e != nil {
		return false
	}
	names, readErr := f.Readdirnames(1)
	closeErr := f.Close()
	return len(names) == 0 && (readErr == nil || errors.Is(readErr, io.EOF)) && closeErr == nil
}

func poolLeagueMkdir(path string) error {
	exists, e := poolLeagueDirectoryExists(path)
	if e != nil {
		return e
	}
	if !exists {
		if e = os.Mkdir(path, 0700); e != nil {
			return e
		}
	}
	return syncDirectory(filepath.Dir(path))
}

func poolLeagueFiles(root string, intent poolLeagueIntent) error {
	allowed := map[string]bool{poolLeagueIntentName: true, poolLeagueReadyName: true, "events": true}
	for _, p := range intent.Participants {
		allowed[p.AccountID+"-READY.json"] = true
	}
	f, e := os.Open(root)
	if e != nil {
		return e
	}
	entries, readErr := f.ReadDir(len(allowed) + 1)
	closeErr := f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil {
		return ErrPoolRuntime
	}
	if len(entries) > len(allowed) {
		return ErrPoolRuntime
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] || entry.Type()&os.ModeSymlink != 0 {
			return ErrPoolRuntime
		}
	}
	return nil
}

// prepareLeague is called with the runtime gate and no account lease. No child
// may borrow any stable until all participant stores and the event owner are
// established. Neither initialization nor reopening performs mining.
func (r *PoolRuntime) prepareLeague(ctx context.Context) (resultErr error) {
	r.mu.Lock()
	if r.closed || len(r.borrows) != 0 || !r.verified["zero256"] || !r.verified["incrementing256"] {
		r.mu.Unlock()
		return ErrPoolRuntime
	}
	if r.league != nil && r.events != nil {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()
	if ctx.Err() != nil || r.issuer == nil {
		return ErrPoolRuntime
	}
	factories := r.leagueFactories
	if factories == nil {
		factories = defaultPoolLeagueFactories()
		r.leagueFactories = factories
	}
	root := filepath.Join(r.config.DataDir, poolLeagueDirectory)
	if e := poolLeagueMkdir(root); e != nil {
		return e
	}
	intentPath := filepath.Join(root, poolLeagueIntentName)
	exists, e := poolRuntimeExists(intentPath)
	if e != nil {
		return e
	}
	var intent poolLeagueIntent
	if exists {
		if e = readManifestJSON(intentPath, poolRuntimeMarkerLimit, &intent); e != nil {
			return e
		}
		intent, e = r.leagueIntent(&intent)
	} else {
		if !poolLeagueEmpty(root) {
			return ErrPoolRuntime
		}
		intent, e = r.leagueIntent(nil)
	}
	if e != nil {
		return e
	}
	if e = poolLeagueFiles(root, intent); e != nil {
		return e
	}
	if e = writePoolRuntimeRecord(intentPath, intent); e != nil {
		return e
	}
	readyPath := filepath.Join(root, poolLeagueReadyName)
	established, e := poolRuntimeExists(readyPath)
	if e != nil {
		return e
	}
	var ready poolLeagueReady
	if established {
		if e = readManifestJSON(readyPath, poolRuntimeMarkerLimit, &ready); e != nil || ready.Version != 1 || !reflect.DeepEqual(ready.Intent, intent) || ready.EventCreatedMS <= 0 {
			return ErrPoolRuntime
		}
	}
	if e = r.loadPipelineBootstrap(ready, established); e != nil {
		return e
	}
	for _, p := range intent.Participants {
		if e = r.prepareLeagueParticipant(ctx, root, intent.SourceBindingID, p, established, factories); e != nil {
			return e
		}
	}
	path := filepath.Join(root, "events")
	hasEvent, e := poolLeagueDirectoryExists(path)
	if e != nil || established && !hasEvent {
		return ErrPoolRuntime
	}
	var events *gymevent.Store
	if !hasEvent || !established && poolLeagueEmpty(path) {
		events, e = factories.createEvent(ctx, path, intent.EventPin)
	} else if established {
		events, e = factories.openEvent(ctx, path, intent.EventPin)
	} else {
		// ResumePrepared verifies the complete pin and distinguishes an
		// unfinished initializer from an already complete original store.
		events, e = factories.resumeEvent(ctx, path, intent.EventPin)
	}
	if e != nil {
		return e
	}
	keep := false
	defer func() {
		if !keep {
			resultErr = errors.Join(resultErr, events.Close())
		}
	}()
	info, e := events.Info(ctx)
	if e != nil || info.StoreID != intent.EventStoreID || !reflect.DeepEqual(info.Pin, intent.EventPin) {
		return ErrPoolRuntime
	}
	want := poolLeagueReady{Version: 1, Intent: intent, EventCreatedMS: info.CreatedAtMS}
	if established && !reflect.DeepEqual(ready, want) {
		return ErrPoolRuntime
	}
	if e = writePoolRuntimeRecord(readyPath, want); e != nil {
		return e
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || ctx.Err() != nil || r.life.Err() != nil {
		return ErrPoolRuntime
	}
	r.league, r.events = &want, events
	keep = true
	return nil
}

func (r *PoolRuntime) prepareLeagueParticipant(ctx context.Context, leagueRoot, binding string, p poolLeagueParticipant, established bool, f *poolLeagueFactories) (resultErr error) {
	root := filepath.Join(r.config.DataDir, "gyms", p.AccountID)
	if ok, e := poolLeagueDirectoryExists(root); e != nil || !ok {
		return ErrPoolRuntime
	}
	readyPath := filepath.Join(leagueRoot, p.AccountID+"-READY.json")
	ready, e := poolRuntimeExists(readyPath)
	if e != nil || established && !ready {
		return ErrPoolRuntime
	}
	want := poolLeagueParticipantReady{1, binding, p}
	if ready && readPoolRuntimeRecord(readyPath, want) != nil {
		return ErrPoolRuntime
	}
	stablePath := filepath.Join(root, "stable")
	hasStable, e := poolLeagueDirectoryExists(stablePath)
	if e != nil {
		return e
	}
	childReady, e := poolRuntimeExists(filepath.Join(root, "SLUG-POOL-CHILD-READY.json"))
	if e != nil || !hasStable && (p.StableExisting || ready || established || childReady) {
		return ErrPoolRuntime
	}
	var stable *gymstable.Store
	if !hasStable || !p.StableExisting && !ready && !childReady && poolLeagueEmpty(stablePath) {
		stable, e = f.createStable(stablePath, p.EntryPin.StablePin, r.issuer)
	} else {
		stable, e = r.openPipelineStable(stablePath, p.EntryPin.StablePin, f)
	}
	if e != nil {
		return e
	}
	defer func() {
		if stable != nil {
			resultErr = errors.Join(resultErr, stable.Close())
		}
	}()
	entryPath := filepath.Join(root, "entries")
	hasEntry, e := poolLeagueDirectoryExists(entryPath)
	if e != nil || !hasEntry && (ready || established) {
		return ErrPoolRuntime
	}
	var entry *gymentry.Store
	if !hasEntry || !ready && poolLeagueEmpty(entryPath) {
		entry, e = f.createEntry(ctx, entryPath, p.EntryPin, stable)
	} else {
		entry, e = f.openEntry(ctx, entryPath, p.EntryPin, stable)
	}
	if e != nil {
		return e
	}
	// Successful open verifies the complete immutable source and journal. No
	// account session is invented for provisioning or retained by the parent.
	e = errors.Join(entry.Close(), stable.Close())
	stable = nil
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return writePoolRuntimeRecord(readyPath, want)
}

func (r *PoolRuntime) leagueChild(account string, pin gymstable.Pin) (*poolLeagueChildBinding, error) {
	if r.league == nil || r.events == nil {
		return nil, ErrPoolRuntime
	}
	id, e := poolservice.BindingID(r.source)
	if e != nil || id != r.league.Intent.SourceBindingID {
		return nil, ErrPoolRuntime
	}
	for _, p := range r.league.Intent.Participants {
		if p.AccountID == account {
			if p.EntryPin.StablePin != pin {
				return nil, ErrPoolRuntime
			}
			ep := r.league.Intent.EventPin
			ep.Participants = append([]gymentry.Pin{}, ep.Participants...)
			return &poolLeagueChildBinding{id, p.EntryPin, p.EntryStoreID, ep, r.league.Intent.EventStoreID, r.league.EventCreatedMS}, nil
		}
	}
	return nil, ErrPoolRuntime
}
