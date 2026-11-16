// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// saveReceipt accepts only a native-dialog-selected path, never web input.
// A temporary file plus fsync and rename prevents a partial JSON export.
func saveReceipt(path string, raw []byte, protectedRoots ...string) error {
	if !filepath.IsAbs(path) || len(raw) == 0 || len(raw) > 256<<10 {
		return errors.New("invalid export")
	}
	path, err := selectedExportPath(path, protectedRoots)
	if err != nil {
		return err
	}
	return writeReceipt(path, raw)
}

// selectedExportPath is shared by public receipt and wallet recovery dialogs.
// Resolve parent aliases before refusing every known application data root.
func selectedExportPath(path string, protectedRoots []string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("invalid export")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", errors.New("export directory unavailable")
	}
	path = filepath.Join(parent, filepath.Base(path))
	for _, root := range protectedRoots {
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			return "", errors.New("export overlaps managed data")
		}
	}
	return path, nil
}

func writeReceipt(path string, raw []byte) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("invalid export target")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".slug-receipt-")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(temp, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
