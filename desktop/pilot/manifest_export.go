// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/recovery"
	"os"
	"path/filepath"
	"strings"
)

func withinManifestRoot(root, path string) bool {
	r, e := filepath.Rel(root, path)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}
func (c *ManifestController) manifestExportPath(path string, existing bool) (string, error) {
	if !ValidRecoveryPath(path) || strings.ContainsRune(path, '\t') {
		return "", ErrManifestReceipt
	}
	path = filepath.Clean(path)
	parent, e := filepath.EvalSymlinks(filepath.Dir(path))
	if e != nil {
		return "", ErrManifestReceipt
	}
	path = filepath.Join(parent, filepath.Base(path))
	cfg, e := os.UserConfigDir()
	if e != nil {
		return "", ErrManifestReceipt
	}
	roots := []string{c.config.DataDir, filepath.Dir(c.config.NodeBinary), filepath.Join(cfg, "SLUG")}
	if filepath.Base(filepath.Dir(c.config.DataDir)) == "gyms" {
		roots = append(roots, filepath.Dir(filepath.Dir(c.config.DataDir)))
	}
	for _, base := range []string{"LocalPilot", "LocalWalletPilot", "LocalAccountPilot", "LocalManifestPilot"} {
		roots = append(roots, filepath.Join(filepath.Dir(c.config.DataDir), base), filepath.Join(cfg, "SLUG", base))
	}
	for _, r := range roots {
		if p, e := filepath.EvalSymlinks(r); e == nil {
			r = p
		}
		if withinManifestRoot(r, path) {
			return "", ErrManifestReceipt
		}
	}
	i, e := os.Lstat(path)
	if existing {
		if e != nil || !i.Mode().IsRegular() || i.Size() > recovery.MaxExportSize {
			return "", ErrManifestReceipt
		}
	} else if e == nil || !os.IsNotExist(e) {
		return "", ErrManifestReceipt
	}
	return path, nil
}

// Link into the pinned parent is exclusive: a destination appearing after the
// native dialog is never overwritten. Sync failure after linking is uncertain.
func manifestExclusiveFile(ctx context.Context, path string, raw []byte, guard func() error) (err error) {
	if len(raw) == 0 || len(raw) > registry.MaxRecordSize+1 {
		return ErrManifestReceipt
	}
	parent := filepath.Dir(path)
	info, e := os.Stat(parent)
	if e != nil || !info.IsDir() {
		return ErrManifestReceipt
	}
	root, e := os.OpenRoot(parent)
	if e != nil {
		return ErrManifestReceipt
	}
	defer root.Close()
	opened, e := root.Open(".")
	if e != nil {
		return ErrManifestReceipt
	}
	pinned, statErr := opened.Stat()
	closeErr := opened.Close()
	if statErr != nil || closeErr != nil || !os.SameFile(info, pinned) {
		return ErrManifestReceipt
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return ErrManifestReceipt
	}
	name := ".manifest-export-" + hex.EncodeToString(nonce[:])
	f, e := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return ErrManifestReceipt
	}
	defer root.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(raw)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return ErrManifestReceipt
	}
	now, e := os.Stat(parent)
	if e != nil || !os.SameFile(info, now) || ctx.Err() != nil || guard == nil || guard() != nil || ctx.Err() != nil {
		return ErrManifestReceipt
	}
	if root.Link(name, filepath.Base(path)) != nil {
		return ErrManifestReceipt
	}
	dir, e := root.Open(".")
	if e != nil {
		return ErrManifestReceipt
	}
	e = dir.Sync()
	ce = dir.Close()
	if e != nil || ce != nil || ctx.Err() != nil {
		return ErrManifestReceipt
	}
	return nil
}
func (c *ManifestController) ExportManifestReceipt(ctx context.Context, sequence uint64, viewID, id, path string) error {
	path, e := c.manifestExportPath(path, false)
	if e != nil {
		return ErrManifestReceipt
	}
	ctx, finish, e := c.beginLazy(ctx)
	if e != nil {
		return ErrManifestReceipt
	}
	defer finish()
	e = c.withView(ctx, func(check func() error) error {
		if e := check(); e != nil {
			return e
		}
		c.mu.Lock()
		s := c.receiptSlot
		if s == nil || s.View != c.viewPin() || s.View.ViewID != viewID || s.Record.Sequence != sequence || s.Record.Verified.ProspectID != id {
			c.mu.Unlock()
			return ErrManifestReceipt
		}
		record := ownedManifestRecord(s.Record)
		c.mu.Unlock()
		raw, e := json.Marshal(record)
		if e != nil || len(raw) > registry.MaxRecordSize {
			return ErrManifestReceipt
		}
		if e = manifestExclusiveFile(ctx, path, append(raw, '\n'), check); e != nil {
			return e
		}
		c.mu.Lock()
		if c.receiptSlot == s {
			c.receiptSlot = nil
		}
		c.mu.Unlock()
		return nil
	})
	if e != nil {
		return ErrManifestReceipt
	}
	return nil
}
