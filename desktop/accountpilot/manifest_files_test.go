// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestManifestAccountMarkerIsolationAndIncompleteRetention(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal storage")
	}
	base := t.TempDir()
	for _, name := range []string{"LocalPilot", "LocalWalletPilot", "LocalAccountPilot"} {
		root := filepath.Join(base, name)
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		before := []byte("retained old fixture data\n")
		if err := os.WriteFile(filepath.Join(root, "SLUG-PILOT.json"), before, 0600); err != nil {
			t.Fatal(err)
		}
		for _, target := range []string{root, filepath.Join(root, "nested")} {
			if c, err := NewManifest(pilot.Config{DataDir: target, NodeBinary: "/fixture/slugd"}); err == nil {
				_ = c.Close()
				t.Fatal("old root accepted")
			}
		}
		after, _ := os.ReadFile(filepath.Join(root, "SLUG-PILOT.json"))
		if !bytes.Equal(before, after) {
			t.Fatal("old root changed")
		}
		if _, err := os.Lstat(filepath.Join(root, "nested")); !os.IsNotExist(err) {
			t.Fatal("nested old mode touched")
		}
	}
	root := filepath.Join(base, "fresh")
	c, err := NewManifest(pilot.Config{DataDir: root, NodeBinary: "/fixture/slugd"})
	if err != nil {
		t.Fatal(err)
	}
	id := c.store.AuthorityID()
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	reopen, err := NewManifest(pilot.Config{DataDir: root, NodeBinary: "/fixture/slugd"})
	if err != nil {
		t.Fatal(err)
	}
	if reopen.store.AuthorityID() != id {
		t.Fatal("outer account identity changed")
	}
	if err = reopen.Close(); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json")
	if err = os.Rename(marker, marker+".retained"); err != nil {
		t.Fatal(err)
	}
	if c, err := NewManifest(pilot.Config{DataDir: root, NodeBinary: "/fixture/slugd"}); !errors.Is(err, pilot.ErrManifestInitialization) || c != nil {
		t.Fatal("lost marker silently adopted", err)
	}
	if _, err = os.Stat(filepath.Join(root, "authority")); err != nil {
		t.Fatal("incomplete authority erased")
	}
	if _, err = os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatal("incomplete marker replaced")
	}
}

func TestManifestAccountMarkerPreflightBeforeLock(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal storage")
	}
	c, err := NewManifest(pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/fixture/slugd"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	path := filepath.Join(c.config.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var marker manifestAccountMarker
	if json.Unmarshal(original, &marker) != nil {
		t.Fatal("marker")
	}
	for _, mutate := range []func(*manifestAccountMarker){
		func(m *manifestAccountMarker) { m.Version = 2 }, func(m *manifestAccountMarker) { m.Mode = pilot.AccountMode },
		func(m *manifestAccountMarker) { m.DescriptorID = strings.Repeat("00", 32) }, func(m *manifestAccountMarker) { m.AccountAuthorityID = strings.Repeat("AA", 32) },
	} {
		m := marker
		mutate(&m)
		raw, _ := json.Marshal(m)
		if err = os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		// The real instance lock is held by c. Invalid marker must refuse
		// before attempting that lock; a lock error would mask this boundary.
		if next, err := NewManifest(c.config); !errors.Is(err, pilot.ErrManifestInitialization) || next != nil {
			t.Fatal("marker preflight not isolated", err)
		}
	}
	for _, raw := range [][]byte{append([]byte(" "), original...), bytes.TrimSuffix(original, []byte("\n")), bytes.Replace(original, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1)} {
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readManifestAccountMarker(path); err == nil {
			t.Fatal("noncanonical marker accepted")
		}
	}
	if err = os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestManifestAccountBackupProtectsSiblingRootsAndAliases(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal storage")
	}
	c, _ := manifestTestController(t)
	base := filepath.Dir(c.config.DataDir)
	for _, name := range []string{"LocalPilot", "LocalWalletPilot", "LocalAccountPilot", "LocalManifestPilot"} {
		root := filepath.Join(base, name)
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		if c.backupPath(filepath.Join(root, "receipt.json")) == nil {
			t.Fatal("sibling export allowed", name)
		}
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(c.config.DataDir, alias); err != nil {
		t.Fatal(err)
	}
	if c.backupPath(filepath.Join(alias, "receipt.json")) == nil {
		t.Fatal("symlink alias exported inside authority")
	}
	if err := c.backupPath(filepath.Join(base, "outside.json")); err != nil {
		t.Fatal(err)
	}
}
