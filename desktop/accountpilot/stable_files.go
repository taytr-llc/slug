// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
)

type stableInstall struct {
	AccountID string        `json:"account_id"`
	Pin       gymstable.Pin `json:"pin"`
	Phase     string        `json:"phase"`
}
type stableAccountMarker struct {
	Version            uint32          `json:"version"`
	Mode               string          `json:"mode"`
	AccountAuthorityID string          `json:"account_authority_id"`
	DescriptorID       string          `json:"descriptor_id"`
	StableSchema       uint32          `json:"stable_schema"`
	StableInstalls     []stableInstall `json:"stable_installs"`
}

func stableID(v string) bool {
	if len(v) != 64 || strings.ToLower(v) != v {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func readStableMarker(path string) (stableAccountMarker, error) {
	var m stableAccountMarker
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > 8192 {
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
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(raw) > 8192 || json.Unmarshal(raw, &m) != nil {
		return m, pilot.ErrManifestInitialization
	}
	canonical, _ := json.Marshal(m)
	descriptor, _ := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if !bytes.Equal(raw, append(canonical, '\n')) || m.Version != 2 || m.Mode != pilot.ManifestMode || !stableID(m.AccountAuthorityID) || m.DescriptorID != descriptor || m.StableSchema != 1 || m.StableInstalls == nil || len(m.StableInstalls) > 2 {
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
	return m, nil
}
func writeStableMarker(path string, m stableAccountMarker) error {
	raw, err := json.Marshal(m)
	if err != nil || len(raw)+1 > 8192 {
		return pilot.ErrManifestInitialization
	}
	return writeMarker(path, append(raw, '\n'))
}
func openStableAccountStore(config pilot.Config) (store *gymaccount.Store, lock *leveldb.DB, marker stableAccountMarker, err error) {
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
		if _, e := readStableMarker(path); e != nil {
			old, e := readManifestAccountMarker(path)
			descriptor, _ := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
			if e != nil || old.Version != 1 || old.Mode != pilot.ManifestMode || old.DescriptorID != descriptor || !stableID(old.AccountAuthorityID) {
				err = pilot.ErrManifestInitialization
				return
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
		marker = stableAccountMarker{2, pilot.ManifestMode, store.AuthorityID(), descriptor, 1, []stableInstall{}}
		err = writeStableMarker(path, marker)
		return
	}
	upgrade := false
	marker, err = readStableMarker(path)
	if err != nil {
		var old manifestAccountMarker
		old, err = readManifestAccountMarker(path)
		if err != nil || old.Version != 1 || old.Mode != pilot.ManifestMode || old.DescriptorID != descriptor || !stableID(old.AccountAuthorityID) {
			err = pilot.ErrManifestInitialization
			return
		}
		marker = stableAccountMarker{2, pilot.ManifestMode, old.AccountAuthorityID, descriptor, 1, []stableInstall{}}
		upgrade = true
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
	if upgrade {
		err = writeStableMarker(path, marker)
	}
	return
}
func (c *StableController) install(ctx context.Context, child manifestChild) error {
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
	marker, err := readStableMarker(path)
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
		if err = writeStableMarker(path, marker); err != nil {
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
		if err = writeStableMarker(path, marker); err != nil {
			return err
		}
	}
	if c.checkpoint != nil {
		c.checkpoint("stable-ready")
	}
	if err = check(); err != nil {
		return err
	}
	if err = stable.AttachStable(ctx); err != nil {
		return err
	}
	return check()
}
