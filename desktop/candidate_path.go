// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var errCandidateDataPath = errors.New("candidate application storage is unavailable")

// prepareCandidateDataParent never creates a custom parent. The only native
// creation permitted is the shared SLUG child of an existing config directory
// for the actual default selection. The helper owns candidate-root creation and
// exact marker validation; this preflight must not modify an older mode first.
func prepareCandidateDataParent(dataDir, configDir string, defaultSelection bool) error {
	if !filepath.IsAbs(dataDir) || filepath.Clean(dataDir) != dataDir || strings.ContainsAny(dataDir, "\x00\r\n\t") {
		return errCandidateDataPath
	}
	parent := filepath.Dir(dataDir)
	if defaultSelection && (!filepath.IsAbs(configDir) || filepath.Clean(configDir) != configDir ||
		dataDir != filepath.Join(configDir, "SLUG", "LocalManifestPilot")) {
		return errCandidateDataPath
	}
	if _, err := os.Lstat(parent); err == nil {
		if _, err = candidateSafeAncestors(parent, dataDir); err != nil {
			return errCandidateDataPath
		}
	} else if !os.IsNotExist(err) || !defaultSelection {
		return errCandidateDataPath
	} else {
		before, err := candidateSafeAncestors(configDir, dataDir)
		if err != nil {
			return errCandidateDataPath
		}
		root, err := os.OpenRoot(configDir)
		if err != nil {
			return errCandidateDataPath
		}
		defer root.Close()
		dir, err := root.Open(".")
		if err != nil {
			return errCandidateDataPath
		}
		defer dir.Close()
		opened, err := dir.Stat()
		if err != nil || !os.SameFile(before, opened) {
			return errCandidateDataPath
		}
		// Pin the validated directory before the only filesystem mutation. Do
		// not recursively create config roots or follow a replacement SLUG link.
		if err = root.Mkdir("SLUG", 0700); err != nil && !os.IsExist(err) {
			return errCandidateDataPath
		}
		child, err := root.Lstat("SLUG")
		if err != nil || !child.IsDir() || child.Mode()&os.ModeSymlink != 0 {
			return errCandidateDataPath
		}
		if err = dir.Sync(); err != nil {
			return errCandidateDataPath
		}
		if _, err = candidateSafeAncestors(parent, dataDir); err != nil {
			return errCandidateDataPath
		}
	}
	if _, err := os.Lstat(dataDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return errCandidateDataPath
	}
	_, err := candidateSafeAncestors(dataDir, dataDir)
	return err
}

// Every inspected ancestor must already be a real directory. Marker contents
// are not trusted here: their mere presence excludes a foreign managed root.
// Only the selected candidate account root may contain its own outer marker.
func candidateSafeAncestors(path, selected string) (os.FileInfo, error) {
	var initial os.FileInfo
	for p := path; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errCandidateDataPath
		}
		if initial == nil {
			initial = info
		}
		switch filepath.Base(p) {
		case "LocalPilot", "LocalWalletPilot", "LocalAccountPilot":
			return nil, errCandidateDataPath
		case "LocalManifestPilot":
			if p != selected {
				return nil, errCandidateDataPath
			}
		}
		for _, marker := range []string{
			"SLUG-PILOT.json", "SLUG-PILOT-READY.json", "SLUG-PILOT-CHAINS.json", "SLUG-ACCOUNTS.json",
			"SLUG-MANIFEST-PILOT.json", "SLUG-MANIFEST-PILOT-READY.json", "SLUG-MANIFEST-PILOT-CHAINS.json",
			"SLUG-MANIFEST-ACCOUNTS.json",
		} {
			if marker == "SLUG-MANIFEST-ACCOUNTS.json" && p == selected {
				continue
			}
			if _, err = os.Lstat(filepath.Join(p, marker)); !os.IsNotExist(err) {
				return nil, errCandidateDataPath
			}
		}
		if filepath.Dir(p) == p {
			return initial, nil
		}
	}
}
