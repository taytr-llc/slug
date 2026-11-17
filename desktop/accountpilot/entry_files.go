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
	"sort"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
)

type entryInstall struct {
	AccountID string       `json:"account_id"`
	Pin       gymentry.Pin `json:"pin"`
	Phase     string       `json:"phase"`
}
type entryAccountMarker struct {
	Version            uint32          `json:"version"`
	Mode               string          `json:"mode"`
	AccountAuthorityID string          `json:"account_authority_id"`
	DescriptorID       string          `json:"descriptor_id"`
	StableSchema       uint32          `json:"stable_schema"`
	StableInstalls     []stableInstall `json:"stable_installs"`
	EntrySchema        uint32          `json:"entry_schema"`
	CalendarID         string          `json:"entry_calendar_id"`
	RulesID            string          `json:"entry_rules_id"`
	EntryInstalls      []entryInstall  `json:"entry_installs"`
}

func entryMarkerFromStable(m stableAccountMarker) entryAccountMarker {
	return entryAccountMarker{3, m.Mode, m.AccountAuthorityID, m.DescriptorID, m.StableSchema, m.StableInstalls, 1, gymentry.CalendarID(), gymentry.RulesID(), []entryInstall{}}
}
func readEntryMarker(path string) (entryAccountMarker, error) {
	var m entryAccountMarker
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
	if !bytes.Equal(raw, append(canonical, '\n')) || m.Version != 3 || m.Mode != pilot.ManifestMode || !stableID(m.AccountAuthorityID) || m.DescriptorID != descriptor || m.StableSchema != 1 || m.StableInstalls == nil || len(m.StableInstalls) > 2 || m.EntrySchema != 1 || m.CalendarID != gymentry.CalendarID() || m.RulesID != gymentry.RulesID() || m.EntryInstalls == nil || len(m.EntryInstalls) > 2 {
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
	return m, nil
}
func writeEntryMarker(path string, m entryAccountMarker) error {
	raw, err := json.Marshal(m)
	if err != nil || len(raw)+1 > 16384 {
		return pilot.ErrManifestInitialization
	}
	return writeMarker(path, append(raw, '\n'))
}
func openEntryAccountStore(config pilot.Config) (store *gymaccount.Store, lock *leveldb.DB, marker entryAccountMarker, err error) {
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
		if _, e := readEntryMarker(path); e != nil {
			if _, e2 := readStableMarker(path); e2 == nil {
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
		store, err = gymaccount.Create(filepath.Join(root, "authority"), storeConfig())
		if err != nil {
			return
		}
		if err = os.Mkdir(filepath.Join(root, "gyms"), 0700); err != nil {
			return
		}
		marker = entryMarkerFromStable(stableAccountMarker{2, pilot.ManifestMode, store.AuthorityID(), descriptor, 1, []stableInstall{}})
		err = writeEntryMarker(path, marker)
		return
	}
	upgrade := false
	marker, err = readEntryMarker(path)
	if err != nil {
		old, e := readStableMarker(path)
		if e == nil {
			marker = entryMarkerFromStable(old)
			err = nil
			upgrade = true
		} else {
			legacy, e := readManifestAccountMarker(path)
			if e != nil || legacy.Version != 1 || legacy.Mode != pilot.ManifestMode || legacy.DescriptorID != descriptor || !stableID(legacy.AccountAuthorityID) {
				err = pilot.ErrManifestInitialization
				return
			}
			marker = entryMarkerFromStable(stableAccountMarker{2, pilot.ManifestMode, legacy.AccountAuthorityID, descriptor, 1, []stableInstall{}})
			err = nil
			upgrade = true
		}
	}

	if !realDirectory(filepath.Join(root, "authority")) || !realDirectory(filepath.Join(root, "gyms")) {
		err = pilot.ErrManifestInitialization
		return
	}
	store, err = gymaccount.Open(filepath.Join(root, "authority"), storeConfig())
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
			stable, err = gymstable.Open(stablePath, installed.Pin, source)
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
		stable, e := gymstable.Open(filepath.Join(root, "gyms", id, "stable"), item.Pin.StablePin, source)
		if e != nil {
			err = errors.Join(e, source.Close())
			return
		}
		entry, e := gymentry.Open(context.Background(), ep, item.Pin, stable)
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
		err = writeEntryMarker(path, marker)
	}
	return
}
func (c *StableController) installEntries(ctx context.Context, child manifestChild) error {
	stable, ok := child.(*pilot.StableController)
	if !ok {
		return gymstable.ErrInvalid
	}
	c.mu.Lock()
	session, view := c.session, c.viewID
	c.mu.Unlock()
	check := func() error {
		return c.withView(ctx, session, view, child, func(_ gymaccount.Account, recheck func() error) error { return recheck() })
	}
	if err := check(); err != nil {
		return err
	}
	pin, err := stable.StablePin()
	if err != nil {
		return err
	}
	path := filepath.Join(c.config.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	marker, err := readEntryMarker(path)
	if err != nil {
		return err
	}
	index := -1
	for i, entry := range marker.StableInstalls {
		if entry.AccountID == pin.AccountID {
			if entry.Pin != pin {
				return pilot.ErrManifestInitialization
			}
			index = i
		}
	}
	if index < 0 {
		if len(marker.StableInstalls) >= 2 {
			return pilot.ErrManifestInitialization
		}
		if _, err := os.Lstat(filepath.Join(c.config.DataDir, "gyms", pin.AccountID, "stable")); !os.IsNotExist(err) {
			return pilot.ErrManifestInitialization
		}
		marker.StableInstalls = append(marker.StableInstalls, stableInstall{pin.AccountID, pin, "prepared"})
		sort.Slice(marker.StableInstalls, func(i, j int) bool { return marker.StableInstalls[i].AccountID < marker.StableInstalls[j].AccountID })
		if err = writeEntryMarker(path, marker); err != nil {
			return err
		}
		if c.checkpoint != nil {
			c.checkpoint("stable-prepared")
		}
		for i, entry := range marker.StableInstalls {
			if entry.AccountID == pin.AccountID {
				index = i
			}
		}
	}
	if marker.StableInstalls[index].Phase == "prepared" {
		if err = stable.CreateStableComponent(ctx); err != nil {
			return err
		}
		if c.checkpoint != nil {
			c.checkpoint("stable-created")
		}
		marker.StableInstalls[index].Phase = "ready"
		if err = writeEntryMarker(path, marker); err != nil {
			return err
		}
	}
	if c.checkpoint != nil {
		c.checkpoint("stable-ready")
	}
	if err = check(); err != nil {
		return err
	}
	ep, err := stable.EntryPin()
	if err != nil {
		return err
	}
	ei := -1
	for i, x := range marker.EntryInstalls {
		if x.AccountID == pin.AccountID {
			if x.Pin != ep {
				return pilot.ErrManifestInitialization
			}
			ei = i
		}
	}
	if ei < 0 {
		if len(marker.EntryInstalls) >= 2 {
			return pilot.ErrManifestInitialization
		}
		if _, e := os.Lstat(filepath.Join(c.config.DataDir, "gyms", pin.AccountID, "entries")); !os.IsNotExist(e) {
			return pilot.ErrManifestInitialization
		}
		marker.EntryInstalls = append(marker.EntryInstalls, entryInstall{pin.AccountID, ep, "prepared"})
		sort.Slice(marker.EntryInstalls, func(i, j int) bool { return marker.EntryInstalls[i].AccountID < marker.EntryInstalls[j].AccountID })
		if err = writeEntryMarker(path, marker); err != nil {
			return err
		}
		if c.checkpoint != nil {
			c.checkpoint("entry-prepared")
		}
		for i, x := range marker.EntryInstalls {
			if x.AccountID == pin.AccountID {
				ei = i
			}
		}
	}
	if marker.EntryInstalls[ei].Phase == "prepared" {
		if err = stable.CreateEntryComponent(ctx); err != nil {
			return err
		}
		if c.checkpoint != nil {
			c.checkpoint("entry-created")
		}
		marker.EntryInstalls[ei].Phase = "ready"
		if err = writeEntryMarker(path, marker); err != nil {
			return err
		}
	}
	if c.checkpoint != nil {
		c.checkpoint("entry-ready")
	}
	if err = check(); err != nil {
		return err
	}
	if err = stable.AttachEntries(ctx); err != nil {
		return err
	}

	return check()
}
