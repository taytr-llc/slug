// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
)

type manifestAccountMarker struct {
	Version            uint32 `json:"version"`
	Mode               string `json:"mode"`
	AccountAuthorityID string `json:"account_authority_id"`
	DescriptorID       string `json:"descriptor_id"`
}

func manifestContains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func manifestPath(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, "\x00\r\n\t") {
		return errors.New("absolute manifest account directory required")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil {
		return errors.New("manifest parent unavailable")
	}
	resolved := filepath.Join(parent, filepath.Base(root))
	protected := []string{}
	for _, name := range []string{"LocalPilot", "LocalWalletPilot", "LocalAccountPilot"} {
		protected = append(protected, filepath.Join(filepath.Dir(resolved), name))
		if config, err := os.UserConfigDir(); err == nil {
			protected = append(protected, filepath.Join(config, "SLUG", name))
		}
	}
	for _, path := range protected {
		if real, err := filepath.EvalSymlinks(path); err == nil {
			path = real
		}
		if manifestContains(path, resolved) || manifestContains(resolved, path) {
			return errors.New("manifest directory overlaps an older mode")
		}
	}
	// Detect explicit custom old roots as ancestors, without scanning their data.
	for dir := resolved; ; dir = filepath.Dir(dir) {
		for _, name := range []string{"SLUG-ACCOUNTS.json", "SLUG-PILOT.json"} {
			if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
				return errors.New("manifest directory overlaps an older mode")
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return nil
}

func readManifestAccountMarker(path string) (manifestAccountMarker, error) {
	var marker manifestAccountMarker
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > 4096 {
		return marker, pilot.ErrManifestInitialization
	}
	f, err := os.Open(path)
	if err != nil {
		return marker, pilot.ErrManifestInitialization
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return marker, pilot.ErrManifestInitialization
	}
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(raw) > 4096 || json.Unmarshal(raw, &marker) != nil {
		return manifestAccountMarker{}, pilot.ErrManifestInitialization
	}
	canonical, err := json.Marshal(marker)
	if err != nil || !bytes.Equal(raw, append(canonical, '\n')) {
		return manifestAccountMarker{}, pilot.ErrManifestInitialization
	}
	return marker, nil
}

// The separate instance lock serializes initial marker publication. An orphaned
// authority without its committed pin is retained, never inspected into trust.
func openManifestAccountStore(root string) (_ *gymaccount.Store, _ *leveldb.DB, resultErr error) {
	if err := manifestPath(root); err != nil {
		return nil, nil, err
	}
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return nil, nil, err
	}
	if !realDirectory(root) {
		return nil, nil, pilot.ErrManifestInitialization
	}
	markerPath := filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json")
	_, markerErr := os.Lstat(markerPath)
	if markerErr != nil && !os.IsNotExist(markerErr) {
		return nil, nil, pilot.ErrManifestInitialization
	}
	if os.IsNotExist(markerErr) {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, nil, err
		}
		for _, entry := range entries {
			if entry.Name() != "instance-lock" {
				return nil, nil, pilot.ErrManifestInitialization
			}
		}
	} else {
		before, err := readManifestAccountMarker(markerPath)
		descriptor, descriptorErr := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
		_, idErr := hex.DecodeString(before.AccountAuthorityID)
		if err != nil || descriptorErr != nil || before.Version != 1 || before.Mode != pilot.ManifestMode || before.DescriptorID != descriptor || len(before.AccountAuthorityID) != 64 || strings.ToLower(before.AccountAuthorityID) != before.AccountAuthorityID || idErr != nil {
			return nil, nil, pilot.ErrManifestInitialization
		}
	}
	lockPath := filepath.Join(root, "instance-lock")
	if info, err := os.Lstat(lockPath); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return nil, nil, pilot.ErrManifestInitialization
	} else if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	lock, err := leveldb.OpenFile(lockPath, nil)
	if err != nil {
		return nil, nil, errors.New("manifest account pilot already open or unavailable")
	}
	keep := false
	defer func() {
		if !keep {
			_ = lock.Close()
		}
	}()
	descriptorID, err := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if err != nil {
		return nil, nil, err
	}
	var store *gymaccount.Store
	if _, err = os.Lstat(markerPath); os.IsNotExist(err) {
		entries, e := os.ReadDir(root)
		if e != nil || len(entries) != 1 || entries[0].Name() != "instance-lock" {
			return nil, nil, pilot.ErrManifestInitialization
		}
		store, err = gymaccount.Create(filepath.Join(root, "authority"), storeConfig())
		if err != nil {
			return nil, nil, pilot.ErrManifestInitialization
		}
		if err = os.Mkdir(filepath.Join(root, "gyms"), 0700); err != nil {
			_ = store.Close()
			return nil, nil, pilot.ErrManifestInitialization
		}
		marker := manifestAccountMarker{Version: 1, Mode: pilot.ManifestMode, AccountAuthorityID: store.AuthorityID(), DescriptorID: descriptorID}
		raw, _ := json.Marshal(marker)
		if err = writeMarker(markerPath, append(raw, '\n')); err != nil {
			_ = store.Close()
			return nil, nil, pilot.ErrManifestInitialization
		}
	} else {
		marker, e := readManifestAccountMarker(markerPath)
		if e != nil || marker.Version != 1 || marker.Mode != pilot.ManifestMode || marker.DescriptorID != descriptorID || !realDirectory(filepath.Join(root, "authority")) || !realDirectory(filepath.Join(root, "gyms")) {
			return nil, nil, pilot.ErrManifestInitialization
		}
		store, err = gymaccount.Open(filepath.Join(root, "authority"), storeConfig())
		if err != nil {
			return nil, nil, pilot.ErrManifestInitialization
		}
		if store.AuthorityID() != marker.AccountAuthorityID {
			_ = store.Close()
			return nil, nil, pilot.ErrManifestInitialization
		}
	}
	keep = true
	return store, lock, nil
}

func (c *ManifestController) backupPath(path string) error {
	if !pilot.ValidRecoveryPath(path) {
		return errors.New("backup path unavailable")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(filepath.Clean(path)))
	if err != nil {
		return errors.New("backup directory unavailable")
	}
	root, err := filepath.EvalSymlinks(c.config.DataDir)
	if err != nil {
		return errors.New("manifest directory unavailable")
	}
	protected := []string{root}
	for _, name := range []string{"LocalPilot", "LocalWalletPilot", "LocalAccountPilot", "LocalManifestPilot"} {
		protected = append(protected, filepath.Join(filepath.Dir(root), name))
	}
	if config, err := os.UserConfigDir(); err == nil {
		protected = append(protected, filepath.Join(config, "SLUG"))
	}
	for _, target := range protected {
		if resolved, err := filepath.EvalSymlinks(target); err == nil {
			target = resolved
		}
		if manifestContains(target, filepath.Join(parent, filepath.Base(path))) {
			return errors.New("backup path overlaps managed data")
		}
	}
	return nil
}
