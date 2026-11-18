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
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"reflect"
)

type resultAccountMarker struct {
	boutAccountMarker
	ResultSchema  uint32 `json:"result_schema"`
	ResultPhase   string `json:"result_phase"`
	ResultStoreID string `json:"result_store_id"`
}

func resultMarkerFromBout(m boutAccountMarker) resultAccountMarker {
	m.Version = 8
	return resultAccountMarker{m, 1, "waiting_source", ""}
}
func resultBase(m resultAccountMarker) boutAccountMarker {
	b := m.boutAccountMarker
	b.Version = 7
	return b
}
func validateResultPhase(m resultAccountMarker) error {
	if m.Version != 8 || m.ResultSchema != 1 || validateBoutPhase(resultBase(m)) != nil {
		return pilot.ErrManifestInitialization
	}
	switch m.ResultPhase {
	case "waiting_source":
		if m.ResultStoreID != "" {
			return pilot.ErrManifestInitialization
		}
	case "prepared", "ready":
		if m.BoutPhase != "ready" {
			return pilot.ErrManifestInitialization
		}
		ev, e := seedEventAccountPin(cardBase(boutBase(resultBase(m))))
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
		bp, e := gymbout.CandidatePin(cp)
		if e != nil {
			return e
		}
		pin, e := gymresult.CandidatePin(bp)
		if e != nil {
			return e
		}
		id, e := gymresult.ID(pin)
		if e != nil || id != m.ResultStoreID {
			return pilot.ErrManifestInitialization
		}
	default:
		return pilot.ErrManifestInitialization
	}
	return nil
}
func readResultMarker(path string) (resultAccountMarker, error) {
	var m resultAccountMarker
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
	if !bytes.Equal(raw, append(canonical, '\n')) {
		return m, pilot.ErrManifestInitialization
	}
	return validateResultAccountMarker(m)
}
func validateResultAccountMarker(m resultAccountMarker) (resultAccountMarker, error) {
	descriptor, _ := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if m.Version != 8 || m.Mode != pilot.ManifestMode || !stableID(m.AccountAuthorityID) || m.DescriptorID != descriptor || m.StableSchema != 1 || m.StableInstalls == nil || len(m.StableInstalls) > 2 || m.EntrySchema != 1 || m.CalendarID != gymentry.CalendarID() || m.RulesID != gymentry.RulesID() || m.EntryInstalls == nil || len(m.EntryInstalls) > 2 {
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
	if e := validateResultPhase(m); e != nil {
		return m, e
	}
	return m, nil
}
func writeResultMarker(path string, m resultAccountMarker) error {
	if m.EventPhase == "waiting_participants" {
		if p, e := seedEventAccountPin(cardBase(boutBase(resultBase(m)))); e == nil {
			m.EventPhase = "prepared"
			m.EventStoreID, _ = gymevent.ID(p)
		}
	}
	if e := validateResultPhase(m); e != nil {
		return e
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw)+1 > 16384 {
		return pilot.ErrManifestInitialization
	}
	return writeMarker(path, append(raw, '\n'))
}
func openResultAccountStore(ctx context.Context, config pilot.Config, deps resultAccountFactories) (store *gymaccount.Store, lock *leveldb.DB, marker resultAccountMarker, err error) {
	return openResultAccountStoreMode(ctx, config, deps, nil)
}
func openResultAccountStoreMode(ctx context.Context, config pilot.Config, deps resultAccountFactories, career *careerAccountOwner) (store *gymaccount.Store, lock *leveldb.DB, marker resultAccountMarker, err error) {
	readRoot := readResultMarker
	if career != nil {
		readRoot = readCareerResultMarker
	}
	if ctx == nil {
		err = gymresult.ErrInvalid
		return
	}
	if err = ctx.Err(); err != nil {
		return
	}
	eventDeps := deps.bout.card.seed.event
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
		if _, e := readRoot(path); e != nil {
			if _, e7 := readBoutMarker(path); e7 == nil {
			} else if _, e6 := readCardMarker(path); e6 == nil {
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
		marker = resultMarkerFromBout(boutMarkerFromCard(cardMarkerFromSeed(seedMarkerFromStable(stableAccountMarker{2, pilot.ManifestMode, store.AuthorityID(), descriptor, 1, []stableInstall{}}))))
		if err = ctx.Err(); err != nil {
			return
		}
		err = writeResultMarker(path, marker)
		return
	}
	upgrade := false
	marker, err = readRoot(path)
	if err != nil {
		if prior, e7 := readBoutMarker(path); e7 == nil {
			marker = resultMarkerFromBout(prior)
			err = nil
			upgrade = true
		} else if prior, e6 := readCardMarker(path); e6 == nil {
			marker = resultMarkerFromBout(boutMarkerFromCard(prior))
			err = nil
			upgrade = true
		} else if prior, e5 := readSeedMarker(path); e5 == nil {
			marker = resultMarkerFromBout(boutMarkerFromCard(cardMarkerFromSeed(prior)))
			err = nil
			upgrade = true
		} else if prior, e4 := readEventMarker(path); e4 == nil {
			marker = resultMarkerFromBout(boutMarkerFromCard(cardMarkerFromSeed(seedMarkerFromEvent(prior))))
			err = nil
			upgrade = true
		} else if prior, e3 := readEntryMarker(path); e3 == nil {
			marker = resultMarkerFromBout(boutMarkerFromCard(cardMarkerFromSeed(seedMarkerFromEntry(prior))))
			err = nil
			upgrade = true
		} else {
			old, e := readStableMarker(path)
			if e == nil {
				marker = resultMarkerFromBout(boutMarkerFromCard(cardMarkerFromSeed(seedMarkerFromStable(old))))
				err = nil
				upgrade = true
			} else {
				legacy, e := readManifestAccountMarker(path)
				if e != nil || legacy.Version != 1 || legacy.Mode != pilot.ManifestMode || legacy.DescriptorID != descriptor || !stableID(legacy.AccountAuthorityID) {
					err = pilot.ErrManifestInitialization
					return
				}
				marker = resultMarkerFromBout(boutMarkerFromCard(cardMarkerFromSeed(seedMarkerFromStable(stableAccountMarker{2, pilot.ManifestMode, legacy.AccountAuthorityID, descriptor, 1, []stableInstall{}}))))
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
			p, e := seedEventAccountPin(cardBase(boutBase(resultBase(marker))))
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
					seed, e = deps.bout.card.seed.openSeed(ctx, filepath.Join(root, "seeds"), sp, event)
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
							cards, e = deps.bout.card.openCard(ctx, filepath.Join(root, "cards"), cp, seed)
							if e == nil {
								ci, ce := cards.Info(ctx)
								e = ce
								if e == nil && (ci.StoreID != marker.CardStoreID || !reflect.DeepEqual(ci.Pin, cp)) {
									e = pilot.ErrManifestInitialization
								}
							}
							if e == nil && marker.BoutPhase == "ready" {
								bp, be := gymbout.CandidatePin(cp)
								e = be
								if e == nil {
									var bouts *gymbout.Store
									bouts, e = deps.bout.openBout(ctx, filepath.Join(root, "bouts"), bp, cards)
									if e == nil {
										bi, be := bouts.Info(ctx)
										e = be
										ci, ce := cards.Info(ctx)
										if e == nil {
											e = ce
										}
										if e == nil && (bi.StoreID != marker.BoutStoreID || !reflect.DeepEqual(bi.Pin, bp) || bi.SourceCreatedAtMS != ci.CreatedAtMS) {
											e = pilot.ErrManifestInitialization
										}
									}
									if bouts != nil {
										e = errors.Join(e, bouts.Close())
									}
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
		if _, e := os.Lstat(filepath.Join(root, "results")); !os.IsNotExist(e) {
			err = pilot.ErrManifestInitialization
			return
		}
		if err = ctx.Err(); err != nil {
			return
		}
		err = writeResultMarker(path, marker)
	}
	return
}

// Result-route-only adapters retain the complete v8 suffix on every older update.
func (c *StableController) readBoutRoot(path string) (boutAccountMarker, error) {
	if c.resultOwner == nil {
		return readBoutMarker(path)
	}
	m, e := c.readResultRoot(path)
	return resultBase(m), e
}
func (c *StableController) writeBoutRoot(path string, b boutAccountMarker) error {
	if c.resultOwner == nil {
		return writeBoutMarker(path, b)
	}
	m, e := c.readResultRoot(path)
	if e != nil {
		return e
	}
	b.Version = 8
	m.boutAccountMarker = b
	return c.writeResultRoot(path, m)
}
func (c *StableController) installResultComponents(ctx context.Context, child manifestChild) error {
	if e := c.installBoutComponents(ctx, child); e != nil {
		return e
	}
	if e := c.ensureResults(ctx); e != nil {
		return e
	}
	s, ok := child.(*pilot.StableController)
	if !ok {
		return gymresult.ErrInvalid
	}
	if c.careerOwner != nil {
		if e := c.ensureCareers(ctx, s); e != nil {
			return e
		}
	}
	return s.AttachResults(ctx, c.resultOwner.store)
}
func (c *StableController) ensureResults(ctx context.Context) error {
	path := filepath.Join(c.config.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	m, e := c.readResultRoot(path)
	if e != nil {
		return e
	}
	dir := filepath.Join(c.config.DataDir, "results")
	if m.ResultPhase == "waiting_source" {
		if _, e := os.Lstat(dir); !os.IsNotExist(e) {
			return pilot.ErrManifestInitialization
		}
		if m.BoutPhase != "ready" || c.boutOwner.store == nil {
			return nil
		}
		source, e := c.boutOwner.store.Info(ctx)
		if e != nil {
			return e
		}
		p, e := gymresult.CandidatePin(source.Pin)
		if e != nil {
			return e
		}
		m.ResultStoreID, e = gymresult.ID(p)
		if e != nil {
			return e
		}
		m.ResultPhase = "prepared"
		if e = c.writeResultRoot(path, m); e != nil {
			return e
		}
	}
	if c.boutOwner.store == nil {
		return pilot.ErrManifestInitialization
	}
	source, e := c.boutOwner.store.Info(ctx)
	if e != nil {
		return e
	}
	pin, e := gymresult.CandidatePin(source.Pin)
	if e != nil {
		return e
	}
	if source.StoreID != m.BoutStoreID {
		return pilot.ErrManifestInitialization
	}
	if c.resultOwner.store != nil {
		info, e := c.resultOwner.store.Info(ctx)
		if e != nil {
			return e
		}
		if m.ResultPhase != "ready" || info.StoreID != m.ResultStoreID || !reflect.DeepEqual(info.Pin, pin) || info.SourceCreatedAtMS != source.CreatedAtMS {
			return pilot.ErrManifestInitialization
		}
		return nil
	}
	var store *gymresult.Store
	if m.ResultPhase == "ready" {
		store, e = c.resultOwner.deps.openResult(ctx, dir, pin, c.boutOwner.store)
	} else {
		if c.checkpoint != nil {
			c.checkpoint("result-prepared")
		}
		store, e = c.resultOwner.deps.resumeResult(ctx, dir, pin, c.boutOwner.store)
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
	if info.StoreID != m.ResultStoreID || !reflect.DeepEqual(info.Pin, pin) || info.SourceCreatedAtMS != source.CreatedAtMS || info.CreatedAtMS < source.CreatedAtMS {
		return pilot.ErrManifestInitialization
	}
	if m.ResultPhase == "prepared" {
		if info.ResultCount != 0 {
			return pilot.ErrManifestInitialization
		}
		if c.checkpoint != nil {
			c.checkpoint("result-created")
		}
		m.ResultPhase = "ready"
		if e = c.writeResultRoot(path, m); e != nil {
			return e
		}
		if c.checkpoint != nil {
			c.checkpoint("result-ready")
		}
	}
	c.resultOwner.store = store
	keep = true
	return nil
}
