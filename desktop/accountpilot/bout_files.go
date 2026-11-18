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
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"reflect"
)

type boutAccountMarker struct {
	cardAccountMarker
	BoutSchema  uint32 `json:"bout_schema"`
	BoutPhase   string `json:"bout_phase"`
	BoutStoreID string `json:"bout_store_id"`
}

func boutMarkerFromCard(m cardAccountMarker) boutAccountMarker {
	m.Version = 7
	return boutAccountMarker{m, 1, "waiting_source", ""}
}
func boutBase(m boutAccountMarker) cardAccountMarker {
	b := m.cardAccountMarker
	b.Version = 6
	return b
}
func validateBoutPhase(m boutAccountMarker) error {
	if m.Version != 7 || m.BoutSchema != 1 || validateCardPhase(boutBase(m)) != nil {
		return pilot.ErrManifestInitialization
	}
	switch m.BoutPhase {
	case "waiting_source":
		if m.BoutStoreID != "" {
			return pilot.ErrManifestInitialization
		}
	case "prepared", "ready":
		if m.CardPhase != "ready" {
			return pilot.ErrManifestInitialization
		}
		ev, e := seedEventAccountPin(cardBase(boutBase(m)))
		if e != nil {
			return e
		}
		sp, e := gymseed.CandidatePin(ev)
		if e != nil {
			return e
		}
		cp, e := gymcard.CandidatePin(sp)
		if e != nil {
			return e
		}
		pin, e := gymbout.CandidatePin(cp)
		if e != nil {
			return e
		}
		id, e := gymbout.ID(pin)
		if e != nil || id != m.BoutStoreID {
			return pilot.ErrManifestInitialization
		}
	default:
		return pilot.ErrManifestInitialization
	}
	return nil
}
func readBoutMarker(path string) (boutAccountMarker, error) {
	var m boutAccountMarker
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
	if !bytes.Equal(raw, append(canonical, '\n')) || m.Version != 7 || m.Mode != pilot.ManifestMode || !stableID(m.AccountAuthorityID) || m.DescriptorID != descriptor || m.StableSchema != 1 || m.StableInstalls == nil || len(m.StableInstalls) > 2 || m.EntrySchema != 1 || m.CalendarID != gymentry.CalendarID() || m.RulesID != gymentry.RulesID() || m.EntryInstalls == nil || len(m.EntryInstalls) > 2 {
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
	if e := validateBoutPhase(m); e != nil {
		return m, e
	}
	return m, nil
}
func writeBoutMarker(path string, m boutAccountMarker) error {
	if m.EventPhase == "waiting_participants" {
		if p, e := seedEventAccountPin(cardBase(boutBase(m))); e == nil {
			m.EventPhase = "prepared"
			m.EventStoreID, _ = gymevent.ID(p)
		}
	}
	if e := validateBoutPhase(m); e != nil {
		return e
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw)+1 > 16384 {
		return pilot.ErrManifestInitialization
	}
	return writeMarker(path, append(raw, '\n'))
}
func openBoutAccountStore(ctx context.Context, config pilot.Config, deps boutAccountFactories) (store *gymaccount.Store, lock *leveldb.DB, marker boutAccountMarker, err error) {
	if ctx == nil {
		err = gymbout.ErrInvalid
		return
	}
	if err = ctx.Err(); err != nil {
		return
	}
	eventDeps := deps.card.seed.event
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
			if err = ctx.Err(); err != nil {
				return
			}
			if entry.Name() != "instance-lock" {
				err = pilot.ErrManifestInitialization
				return
			}
		}
	}
	if !fresh {
		if _, e := readBoutMarker(path); e != nil {
			if _, e6 := readCardMarker(path); e6 == nil {
			} else if _, e5 := readSeedMarker(path); e5 == nil {
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
		marker = boutMarkerFromCard(cardMarkerFromSeed(seedMarkerFromStable(stableAccountMarker{2, pilot.ManifestMode, store.AuthorityID(), descriptor, 1, []stableInstall{}})))
		if err = ctx.Err(); err != nil {
			return
		}
		err = writeBoutMarker(path, marker)
		return
	}
	upgrade := false
	marker, err = readBoutMarker(path)
	if err != nil {
		if prior, e6 := readCardMarker(path); e6 == nil {
			marker = boutMarkerFromCard(prior)
			err = nil
			upgrade = true
		} else if prior, e5 := readSeedMarker(path); e5 == nil {
			marker = boutMarkerFromCard(cardMarkerFromSeed(prior))
			err = nil
			upgrade = true
		} else if prior, e4 := readEventMarker(path); e4 == nil {
			marker = boutMarkerFromCard(cardMarkerFromSeed(seedMarkerFromEvent(prior)))
			err = nil
			upgrade = true
		} else if prior, e3 := readEntryMarker(path); e3 == nil {
			marker = boutMarkerFromCard(cardMarkerFromSeed(seedMarkerFromEntry(prior)))
			err = nil
			upgrade = true
		} else {
			old, e := readStableMarker(path)
			if e == nil {
				marker = boutMarkerFromCard(cardMarkerFromSeed(seedMarkerFromStable(old)))
				err = nil
				upgrade = true
			} else {
				legacy, e := readManifestAccountMarker(path)
				if e != nil || legacy.Version != 1 || legacy.Mode != pilot.ManifestMode || legacy.DescriptorID != descriptor || !stableID(legacy.AccountAuthorityID) {
					err = pilot.ErrManifestInitialization
					return
				}
				marker = boutMarkerFromCard(cardMarkerFromSeed(seedMarkerFromStable(stableAccountMarker{2, pilot.ManifestMode, legacy.AccountAuthorityID, descriptor, 1, []stableInstall{}})))
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
		if err = ctx.Err(); err != nil {
			return
		}
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
		if err = ctx.Err(); err != nil {
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
			source, err = openBoutStartupSource(ctx, filepath.Join(dir, "manifest-authority"), installed.Pin.Source)
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
		if err = ctx.Err(); err != nil {
			return
		}
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
		source, e := openBoutStartupSource(ctx, filepath.Join(root, "gyms", id, "manifest-authority"), item.Pin.StablePin.Source)
		if e != nil {
			err = e
			return
		}
		stable, e := eventDeps.openStable(filepath.Join(root, "gyms", id, "stable"), item.Pin.StablePin, source)
		if e != nil {
			err = errors.Join(e, source.Close())
			return
		}
		entry, e := eventDeps.openEntry(ctx, ep, item.Pin, stable)
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
	if err = ctx.Err(); err != nil {
		return
	}
	if upgrade {
		if marker.EventPhase == "ready" {
			p, e := seedEventAccountPin(cardBase(boutBase(marker)))
			if e != nil {
				err = e
				return
			}
			event, e := eventDeps.openEvent(ctx, filepath.Join(root, "events"), p)
			if e != nil {
				err = e
				return
			}
			info, e := event.Info(ctx)
			if e == nil && (info.StoreID != marker.EventStoreID || !reflect.DeepEqual(info.Pin, p)) {
				e = pilot.ErrManifestInitialization
			}
			if e == nil && marker.SeedPhase == "ready" {
				sp, pe := gymseed.CandidatePin(p)
				e = pe
				if e == nil {
					var seed *gymseed.Store
					seed, e = deps.card.seed.openSeed(ctx, filepath.Join(root, "seeds"), sp, event)
					if e == nil {
						si, se := seed.Info(ctx)
						e = se
						if e == nil && (si.StoreID != marker.SeedStoreID || !reflect.DeepEqual(si.Pin, sp)) {
							e = pilot.ErrManifestInitialization
						}
					}
					if e == nil && marker.CardPhase == "ready" {
						cp, ce := gymcard.CandidatePin(sp)
						e = ce
						if e == nil {
							var cards *gymcard.Store
							cards, e = deps.card.openCard(ctx, filepath.Join(root, "cards"), cp, seed)
							if e == nil {
								ci, ce := cards.Info(ctx)
								e = ce
								if e == nil && (ci.StoreID != marker.CardStoreID || !reflect.DeepEqual(ci.Pin, cp)) {
									e = pilot.ErrManifestInitialization
								}
							}
							if cards != nil {
								e = errors.Join(e, cards.Close())
							}
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
		if _, e := os.Lstat(filepath.Join(root, "bouts")); !os.IsNotExist(e) {
			err = pilot.ErrManifestInitialization
			return
		}
		if err = ctx.Err(); err != nil {
			return
		}
		err = writeBoutMarker(path, marker)
	}
	return
}

// New-route-only adapters retain the complete v7 suffix on every older update.
func (c *StableController) readCardRoot(path string) (cardAccountMarker, error) {
	if c.boutOwner == nil {
		return readCardMarker(path)
	}
	m, e := readBoutMarker(path)
	return boutBase(m), e
}
func (c *StableController) writeCardRoot(path string, b cardAccountMarker) error {
	if c.boutOwner == nil {
		return writeCardMarker(path, b)
	}
	m, e := readBoutMarker(path)
	if e != nil {
		return e
	}
	b.Version = 7
	m.cardAccountMarker = b
	return writeBoutMarker(path, m)
}
func (c *StableController) installBoutComponents(ctx context.Context, child manifestChild) error {
	if e := c.installCardComponents(ctx, child); e != nil {
		return e
	}
	if e := c.ensureBouts(ctx); e != nil {
		return e
	}
	s, ok := child.(*pilot.StableController)
	if !ok {
		return gymbout.ErrInvalid
	}
	return s.AttachBouts(ctx, c.boutOwner.store)
}
func (c *StableController) ensureBouts(ctx context.Context) error {
	path := filepath.Join(c.config.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	m, e := readBoutMarker(path)
	if e != nil {
		return e
	}
	dir := filepath.Join(c.config.DataDir, "bouts")
	if m.BoutPhase == "waiting_source" {
		if _, e := os.Lstat(dir); !os.IsNotExist(e) {
			return pilot.ErrManifestInitialization
		}
		if m.CardPhase != "ready" || c.cardOwner.store == nil {
			return nil
		}
		source, e := c.cardOwner.store.Info(ctx)
		if e != nil {
			return e
		}
		p, e := gymbout.CandidatePin(source.Pin)
		if e != nil {
			return e
		}
		m.BoutStoreID, e = gymbout.ID(p)
		if e != nil {
			return e
		}
		m.BoutPhase = "prepared"
		if e = writeBoutMarker(path, m); e != nil {
			return e
		}
	}
	if c.cardOwner.store == nil {
		return pilot.ErrManifestInitialization
	}
	source, e := c.cardOwner.store.Info(ctx)
	if e != nil {
		return e
	}
	pin, e := gymbout.CandidatePin(source.Pin)
	if e != nil {
		return e
	}
	if source.StoreID != m.CardStoreID {
		return pilot.ErrManifestInitialization
	}
	if c.boutOwner.store != nil {
		info, e := c.boutOwner.store.Info(ctx)
		if e != nil {
			return e
		}
		if m.BoutPhase != "ready" || info.StoreID != m.BoutStoreID || !reflect.DeepEqual(info.Pin, pin) || info.SourceCreatedAtMS != source.CreatedAtMS {
			return pilot.ErrManifestInitialization
		}
		return nil
	}
	var store *gymbout.Store
	if m.BoutPhase == "ready" {
		store, e = c.boutOwner.deps.openBout(ctx, dir, pin, c.cardOwner.store)
	} else {
		if c.checkpoint != nil {
			c.checkpoint("bout-prepared")
		}
		store, e = c.boutOwner.deps.resumeBout(ctx, dir, pin, c.cardOwner.store)
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
	if info.StoreID != m.BoutStoreID || !reflect.DeepEqual(info.Pin, pin) || info.SourceCreatedAtMS != source.CreatedAtMS || info.CreatedAtMS < source.CreatedAtMS {
		return pilot.ErrManifestInitialization
	}
	if m.BoutPhase == "prepared" {
		if info.BoutCount != 0 {
			return pilot.ErrManifestInitialization
		}
		if c.checkpoint != nil {
			c.checkpoint("bout-created")
		}
		m.BoutPhase = "ready"
		if e = writeBoutMarker(path, m); e != nil {
			return e
		}
		if c.checkpoint != nil {
			c.checkpoint("bout-ready")
		}
	}
	c.boutOwner.store = store
	keep = true
	return nil
}

// Context-free legacy opens are joined, never abandoned in a timeout goroutine.
// The startup context bounds admission and is checked again before publication.
func openBoutStartupSource(ctx context.Context, path string, pin registry.ManifestAuthorityPin) (*registry.ManifestStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store, err := registry.OpenManifestAuthority(path, pin, true)
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
		if store != nil {
			_ = store.Close()
			store = nil
		}
	}
	return store, err
}
