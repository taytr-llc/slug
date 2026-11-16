package accountpilot

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
)

type marker struct {
	Version     uint32 `json:"version"`
	Mode        string `json:"mode"`
	AuthorityID string `json:"authority_id"`
}

func realDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func openStore(root string) (*gymaccount.Store, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, errors.New("absolute account pilot directory required")
	}
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return nil, errors.New("account directory unavailable")
	}
	if !realDirectory(root) {
		return nil, errors.New("account directory unavailable")
	}
	markerPath := filepath.Join(root, "SLUG-ACCOUNTS.json")
	if _, err := os.Lstat(markerPath); os.IsNotExist(err) {
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			return nil, errors.New("unrecognized account data retained")
		}
		store, err := gymaccount.Create(filepath.Join(root, "authority"), storeConfig())
		if err != nil {
			return nil, errors.New("account store initialization failed")
		}
		ok := false
		defer func() {
			if !ok {
				_ = store.Close()
			}
		}()
		if os.Mkdir(filepath.Join(root, "gyms"), 0700) != nil {
			return nil, errors.New("account storage initialization failed")
		}
		data, _ := json.Marshal(marker{Version: 1, Mode: "local-account-pilot", AuthorityID: store.AuthorityID()})
		if writeMarker(markerPath, append(data, '\n')) != nil {
			return nil, errors.New("account identity initialization failed")
		}
		ok = true
		return store, nil
	} else if err != nil {
		return nil, errors.New("account identity unavailable")
	}
	info, err := os.Lstat(markerPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1024 {
		return nil, errors.New("account identity unavailable")
	}
	f, err := os.Open(markerPath)
	if err != nil {
		return nil, errors.New("account identity unavailable")
	}
	data, readErr := io.ReadAll(io.LimitReader(f, 1025))
	closeErr := f.Close()
	var m marker
	if readErr != nil || closeErr != nil || len(data) > 1024 || json.Unmarshal(data, &m) != nil {
		return nil, errors.New("account identity unavailable")
	}
	canonical, _ := json.Marshal(m)
	if !bytes.Equal(bytes.TrimSpace(data), canonical) || m.Version != 1 || m.Mode != "local-account-pilot" || !realDirectory(filepath.Join(root, "gyms")) || !realDirectory(filepath.Join(root, "authority")) {
		return nil, errors.New("account identity mismatch")
	}
	store, err := gymaccount.Open(filepath.Join(root, "authority"), storeConfig())
	if err != nil {
		return nil, errors.New("account store unavailable")
	}
	if store.AuthorityID() != m.AuthorityID {
		_ = store.Close()
		return nil, errors.New("account authority mismatch")
	}
	return store, nil
}

func writeMarker(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".account-marker-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

// Protect the entire custom account root, including authority and sibling gyms.
// The child additionally protects default application roots and bundled nodes.
func (c *Controller) backupPath(path string) error {
	if !pilot.ValidRecoveryPath(path) {
		return errors.New("backup path unavailable")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(filepath.Clean(path)))
	if err != nil {
		return errors.New("backup directory unavailable")
	}
	root, err := filepath.EvalSymlinks(c.config.DataDir)
	if err != nil {
		return errors.New("account directory unavailable")
	}
	for _, protected := range []string{root, filepath.Join(filepath.Dir(root), "LocalPilot"), filepath.Join(filepath.Dir(root), "LocalWalletPilot")} {
		if resolved, err := filepath.EvalSymlinks(protected); err == nil {
			protected = resolved
		}
		rel, err := filepath.Rel(protected, filepath.Join(parent, filepath.Base(path)))
		if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return errors.New("backup path overlaps account data")
		}
	}
	return nil
}
