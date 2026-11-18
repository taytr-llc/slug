// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"reflect"
)

type cardAccountMarker struct {
	seedAccountMarker
	CardSchema  uint32 `json:"card_schema"`
	CardPhase   string `json:"card_phase"`
	CardStoreID string `json:"card_store_id"`
}

func cardMarkerFromSeed(m seedAccountMarker) cardAccountMarker {
	m.Version = 6
	return cardAccountMarker{m, 1, "waiting_source", ""}
}
func cardBase(m cardAccountMarker) seedAccountMarker {
	b := m.seedAccountMarker
	b.Version = 5
	return b
}
func validateCardPhase(m cardAccountMarker) error {
	if m.Version != 6 || m.CardSchema != 1 || validateSeedEventPhase(cardBase(m)) != nil {
		return pilot.ErrManifestInitialization
	}
	switch m.CardPhase {
	case "waiting_source":
		if m.CardStoreID != "" {
			return pilot.ErrManifestInitialization
		}
	case "prepared", "ready":
		if m.SeedPhase != "ready" {
			return pilot.ErrManifestInitialization
		}
		ev, e := seedEventAccountPin(cardBase(m))
		if e != nil {
			return e
		}
		sp, e := gymseed.CandidatePin(ev)
		if e != nil {
			return e
		}
		pin, e := gymcard.CandidatePin(sp)
		if e != nil {
			return e
		}
		id, e := gymcard.ID(pin)
		if e != nil || id != m.CardStoreID {
			return pilot.ErrManifestInitialization
		}
	default:
		return pilot.ErrManifestInitialization
	}
	return nil
}
func readCardMarker(path string) (cardAccountMarker, error) {
	var m cardAccountMarker
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > 16384 {
		return m, pilot.ErrManifestInitialization
	}
	f, err := os.Open(path)
	if err != nil {
		return m, err
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return m, pilot.ErrManifestInitialization
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(raw) > 16384 || json.Unmarshal(raw, &m) != nil {
		return m, pilot.ErrManifestInitialization
	}
	canonical, _ := json.Marshal(m)
	descriptor, _ := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if !bytes.Equal(raw, append(canonical, '\n')) || m.Version != 6 || m.Mode != pilot.ManifestMode || !stableID(m.AccountAuthorityID) || m.DescriptorID != descriptor || m.StableSchema != 1 || m.StableInstalls == nil || len(m.StableInstalls) > 2 || m.EntrySchema != 1 || m.CalendarID != gymentry.CalendarID() || m.RulesID != gymentry.RulesID() || m.EntryInstalls == nil || len(m.EntryInstalls) > 2 {
		return m, pilot.ErrManifestInitialization
	}
	last := ""
	for _, entry := range m.StableInstalls {
		pin, err := gymstable.ValidatePin(entry.Pin)
		if err != nil || entry.AccountID <= last || entry.AccountID != pin.AccountID || pin.AccountAuthorityID != m.AccountAuthorityID || pin.Source.DescriptorID != descriptor || (entry.Phase != "prepared" && entry.Phase != "ready") {
			return m, pilot.ErrManifestInitialization
		}
		last = entry.AccountID
	}
	last = ""
	for _, x := range m.EntryInstalls {
		p, e := gymentry.ValidatePin(x.Pin)
		if e != nil || x.AccountID <= last || x.AccountID != p.StablePin.AccountID || (x.Phase != "ready" && x.Phase != "prepared") {
			return m, pilot.ErrManifestInitialization
		}
		found := false
		for _, y := range m.StableInstalls {
			if y.AccountID == x.AccountID && y.Phase == "ready" && y.Pin == p.StablePin {
				found = true
			}
		}
		if !found {
			return m, pilot.ErrManifestInitialization
		}
		last = x.AccountID
	}
	if e := validateCardPhase(m); e != nil {
		return m, e
	}
	return m, nil
}
func writeCardMarker(path string, m cardAccountMarker) error {
	if m.EventPhase == "waiting_participants" {
		if p, e := seedEventAccountPin(cardBase(m)); e == nil {
			m.EventPhase = "prepared"
			m.EventStoreID, _ = gymevent.ID(p)
		}
	}
	if e := validateCardPhase(m); e != nil {
		return e
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw)+1 > 16384 {
		return pilot.ErrManifestInitialization
	}
	return writeMarker(path, append(raw, '\n'))
}
func openCardAccountStore(config pilot.Config, deps cardAccountFactories) (store *gymaccount.Store, lock *leveldb.DB, marker cardAccountMarker, err error) {
	eventDeps := deps.seed.event
	root := config.DataDir
	if err = manifestPath(root); err != nil {
		return
	}
	if err = os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return
	}
	err = nil
	if !realDirectory(root) {
		err = pilot.ErrManifestInitialization
		return
	}
	path := filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json")
	_, statErr := os.Lstat(path)
	fresh := os.IsNotExist(statErr)
	if statErr != nil && !fresh {
		err = statErr
		return
	}
	if fresh {
		entries, e := os.ReadDir(root)
		if e != nil {
			err = e
			return
		}
		for _, entry := range entries {
			if entry.Name() != "instance-lock" {
				err = pilot.ErrManifestInitialization
				return
			}
		}
	}
	if !fresh {
		if _, e := readCardMarker(path); e != nil {
			if _, e5 := readSeedMarker(path); e5 == nil {
			} else if _, e4 := readEventMarker(path); e4 == nil {
			} else if _, e3 := readEntryMarker(path); e3 == nil {
			} else if _, e2 := readStableMarker(path); e2 == nil {
			} else {
				old, e := readManifestAccountMarker(path)
				descriptor, _ := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
				if e != nil || old.Version != 1 || old.Mode != pilot.ManifestMode || old.DescriptorID != descriptor || !stableID(old.AccountAuthorityID) {
					err = pilot.ErrManifestInitialization
					return
				}
			}
		}
	}
	lockPath := filepath.Join(root, "instance-lock")
	if !fresh && !realDirectory(lockPath) {
		err = pilot.ErrManifestInitialization
		return
	}
	if _, e := os.Lstat(lockPath); e == nil && !realDirectory(lockPath) {
		err = pilot.ErrManifestInitialization
		return
	}
	lock, err = leveldb.OpenFile(lockPath, nil)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			if store != nil {
				err = errors.Join(err, store.Close())
				store = nil
			}
			err = errors.Join(err, lock.Close())
			lock = nil
		}
	}()
	descriptor, _ := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if fresh {
		entries, e := os.ReadDir(root)
		if e != nil || len(entries) != 1 || entries[0].Name() != "instance-lock" {
			err = pilot.ErrManifestInitialization
			return
		}
		store, err = eventDeps.createAccount(filepath.Join(root, "authority"), storeConfig())
		if err != nil {
			return
		}
		if err = os.Mkdir(filepath.Join(root, "gyms"), 0700); err != nil {
			return
		}
		marker = cardMarkerFromSeed(seedMarkerFromStable(stableAccountMarker{2, pilot.ManifestMode, store.AuthorityID(), descriptor, 1, []stableInstall{}}))
		err = writeCardMarker(path, marker)
		return
	}
	upgrade := false
	marker, err = readCardMarker(path)
	if err != nil {
		if prior, e5 := readSeedMarker(path); e5 == nil {
			marker = cardMarkerFromSeed(prior)
			err = nil
			upgrade = true
		} else if prior, e4 := readEventMarker(path); e4 == nil {
			marker = cardMarkerFromSeed(seedMarkerFromEvent(prior))
			err = nil
			upgrade = true
		} else if prior, e3 := readEntryMarker(path); e3 == nil {
			marker = cardMarkerFromSeed(seedMarkerFromEntry(prior))
			err = nil
			upgrade = true
		} else {
			old, e := readStableMarker(path)
			if e == nil {
				marker = cardMarkerFromSeed(seedMarkerFromStable(old))
				err = nil
				upgrade = true
			} else {
				legacy, e := readManifestAccountMarker(path)
				if e != nil || legacy.Version != 1 || legacy.Mode != pilot.ManifestMode || legacy.DescriptorID != descriptor || !stableID(legacy.AccountAuthorityID) {
					err = pilot.ErrManifestInitialization
					return
				}
				marker = cardMarkerFromSeed(seedMarkerFromStable(stableAccountMarker{2, pilot.ManifestMode, legacy.AccountAuthorityID, descriptor, 1, []stableInstall{}}))
				err = nil
				upgrade = true
			}
		}

	}

	if !realDirectory(filepath.Join(root, "authority")) || !realDirectory(filepath.Join(root, "gyms")) {
		err = pilot.ErrManifestInitialization
		return
	}
	store, err = eventDeps.openAccount(filepath.Join(root, "authority"), storeConfig())
	if err != nil {
		return
	}
	if store.AuthorityID() != marker.AccountAuthorityID {
		err = pilot.ErrManifestInitialization
		return
	}
	entries, e := os.ReadDir(filepath.Join(root, "gyms"))
	if e != nil || len(entries) > 2 {
		err = pilot.ErrManifestInitialization
		return
	}
	known := map[string]bool{}
	for _, entry := range entries {
		id := entry.Name()
		dir := filepath.Join(root, "gyms", id)
		if !stableID(id) || !realDirectory(dir) {
			err = pilot.ErrManifestInitialization
			return
		}
		known[id] = true
		// Validate the established child before invoking any constructor that could
		// otherwise initialize a missing marker or wallet.
		if err = pilot.ValidateStableUpgradeChild(pilot.Config{DataDir: dir, NodeBinary: config.NodeBinary}, marker.AccountAuthorityID, id); err != nil {
			return
		}
		var installed *stableInstall
		for i := range marker.StableInstalls {
			if marker.StableInstalls[i].AccountID == id {
				installed = &marker.StableInstalls[i]
			}
		}
		stablePath := filepath.Join(dir, "stable")
		if installed == nil {
			if _, e := os.Lstat(stablePath); !os.IsNotExist(e) {
				err = pilot.ErrManifestInitialization
				return
			}
			continue
		}
		if installed.Phase == "ready" {
			var source *registry.ManifestStore
			source, err = registry.OpenManifestAuthority(filepath.Join(dir, "manifest-authority"), installed.Pin.Source, true)
			if err != nil {
				return
			}
			var stable *gymstable.Store
			stable, err = eventDeps.openStable(stablePath, installed.Pin, source)
			if stable != nil {
				err = errors.Join(err, stable.Close())
			}
			err = errors.Join(err, source.Close())
			if err != nil {
				return
			}
		}
	}
	for _, entry := range marker.StableInstalls {
		if !known[entry.AccountID] {
			err = pilot.ErrManifestInitialization
			return
		}
	}
	for id := range known {
		var item *entryInstall
		for i := range marker.EntryInstalls {
			if marker.EntryInstalls[i].AccountID == id {
				item = &marker.EntryInstalls[i]
			}
		}
		ep := filepath.Join(root, "gyms", id, "entries")
		if item == nil {
			if _, e := os.Lstat(ep); !os.IsNotExist(e) {
				err = pilot.ErrManifestInitialization
				return
			}
			continue
		}
		if item.Phase == "prepared" {
			if _, e := os.Lstat(ep); os.IsNotExist(e) {
				continue
			}
			if realDirectory(ep) {
				f, e := os.Open(ep)
				if e != nil {
					err = e
					return
				}
				names, e := f.Readdirnames(1)
				_ = f.Close()
				if len(names) == 0 && errors.Is(e, io.EOF) {
					continue
				}
			}
		}
		source, e := registry.OpenManifestAuthority(filepath.Join(root, "gyms", id, "manifest-authority"), item.Pin.StablePin.Source, true)
		if e != nil {
			err = e
			return
		}
		stable, e := eventDeps.openStable(filepath.Join(root, "gyms", id, "stable"), item.Pin.StablePin, source)
		if e != nil {
			err = errors.Join(e, source.Close())
			return
		}
		entry, e := eventDeps.openEntry(context.Background(), ep, item.Pin, stable)
		if entry != nil {
			e = errors.Join(e, entry.Close())
		}
		err = errors.Join(e, stable.Close(), source.Close())
		if err != nil {
			return
		}
	}
	for _, x := range marker.EntryInstalls {
		if !known[x.AccountID] {
			err = pilot.ErrManifestInitialization
			return
		}
	}
	if upgrade {
		if marker.EventPhase == "ready" {
			p, e := seedEventAccountPin(cardBase(marker))
			if e != nil {
				err = e
				return
			}
			event, e := eventDeps.openEvent(context.Background(), filepath.Join(root, "events"), p)
			if e != nil {
				err = e
				return
			}
			info, e := event.Info(context.Background())
			if e == nil && (info.StoreID != marker.EventStoreID || !reflect.DeepEqual(info.Pin, p)) {
				e = pilot.ErrManifestInitialization
			}
			if e == nil && marker.SeedPhase == "ready" {
				sp, pe := gymseed.CandidatePin(p)
				e = pe
				if e == nil {
					var seed *gymseed.Store
					seed, e = deps.seed.openSeed(context.Background(), filepath.Join(root, "seeds"), sp, event)
					if e == nil {
						si, se := seed.Info(context.Background())
						e = se
						if e == nil && (si.StoreID != marker.SeedStoreID || !reflect.DeepEqual(si.Pin, sp)) {
							e = pilot.ErrManifestInitialization
						}
					}
					if seed != nil {
						e = errors.Join(e, seed.Close())
					}
				}
			}
			err = errors.Join(e, event.Close())
			if err != nil {
				return
			}

		}
		if _, e := os.Lstat(filepath.Join(root, "cards")); !os.IsNotExist(e) {
			err = pilot.ErrManifestInitialization
			return
		}
		err = writeCardMarker(path, marker)
	}
	return
}

// New-route-only adapters preserve the v6 suffix on every older component update.
func (c *StableController) readSeedRoot(path string) (seedAccountMarker, error) {
	if c.cardOwner == nil {
		return readSeedMarker(path)
	}
	m, e := c.readCardRoot(path)
	return cardBase(m), e
}
func (c *StableController) writeSeedRoot(path string, b seedAccountMarker) error {
	if c.cardOwner == nil {
		return writeSeedMarker(path, b)
	}
	m, e := c.readCardRoot(path)
	if e != nil {
		return e
	}
	b.Version = 6
	m.seedAccountMarker = b
	return c.writeCardRoot(path, m)
}
func (c *StableController) installCardComponents(ctx context.Context, child manifestChild) error {
	if e := c.installSeedComponents(ctx, child); e != nil {
		return e
	}
	if e := c.ensureCards(ctx); e != nil {
		return e
	}
	s, ok := child.(*pilot.StableController)
	if !ok {
		return gymcard.ErrInvalid
	}
	return s.AttachCards(ctx, c.cardOwner.store)
}
func (c *StableController) ensureCards(ctx context.Context) error {
	path := filepath.Join(c.config.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	m, e := c.readCardRoot(path)
	if e != nil {
		return e
	}
	dir := filepath.Join(c.config.DataDir, "cards")
	if m.CardPhase == "waiting_source" {
		if _, e := os.Lstat(dir); !os.IsNotExist(e) {
			return pilot.ErrManifestInitialization
		}
		if m.SeedPhase != "ready" || c.seedOwner.store == nil {
			return nil
		}
		source, e := c.seedOwner.store.Info(ctx)
		if e != nil {
			return e
		}
		p, e := gymcard.CandidatePin(source.Pin)
		if e != nil {
			return e
		}
		m.CardStoreID, e = gymcard.ID(p)
		if e != nil {
			return e
		}
		m.CardPhase = "prepared"
		if e = c.writeCardRoot(path, m); e != nil {
			return e
		}
	}
	if c.seedOwner.store == nil {
		return pilot.ErrManifestInitialization
	}
	source, e := c.seedOwner.store.Info(ctx)
	if e != nil {
		return e
	}
	pin, e := gymcard.CandidatePin(source.Pin)
	if e != nil {
		return e
	}
	if source.StoreID != m.SeedStoreID {
		return pilot.ErrManifestInitialization
	}
	if c.cardOwner.store != nil {
		info, e := c.cardOwner.store.Info(ctx)
		if e != nil {
			return e
		}
		if m.CardPhase != "ready" || info.StoreID != m.CardStoreID || !reflect.DeepEqual(info.Pin, pin) || info.SourceCreatedAtMS != source.CreatedAtMS {
			return pilot.ErrManifestInitialization
		}
		return nil
	}
	var store *gymcard.Store
	if m.CardPhase == "ready" {
		store, e = c.cardOwner.deps.openCard(ctx, dir, pin, c.seedOwner.store)
	} else {
		if c.checkpoint != nil {
			c.checkpoint("card-prepared")
		}
		store, e = c.cardOwner.deps.resumeCard(ctx, dir, pin, c.seedOwner.store)
	}
	if e != nil {
		return e
	}
	keep := false
	defer func() {
		if !keep {
			_ = store.Close()
		}
	}()
	info, e := store.Info(ctx)
	if e != nil {
		return e
	}
	if info.StoreID != m.CardStoreID || !reflect.DeepEqual(info.Pin, pin) || info.SourceCreatedAtMS != source.CreatedAtMS || info.CreatedAtMS < source.CreatedAtMS {
		return pilot.ErrManifestInitialization
	}
	if m.CardPhase == "prepared" {
		if info.EventCount != 0 {
			return pilot.ErrManifestInitialization
		}
		if c.checkpoint != nil {
			c.checkpoint("card-created")
		}
		m.CardPhase = "ready"
		if e = c.writeCardRoot(path, m); e != nil {
			return e
		}
		if c.checkpoint != nil {
			c.checkpoint("card-ready")
		}
	}
	c.cardOwner.store = store
	keep = true
	return nil
}
