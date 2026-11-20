// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"os"
	"path/filepath"
	"testing"
)

func remoteControllerTestParent(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(p, 0700); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestRemotePoolExistingRootNeverCreates(t *testing.T) {
	parent := remoteControllerTestParent(t)
	missing := filepath.Join(parent, "missing")
	if owner, e := openOwnedRootMode(missing, true); e == nil || owner != nil {
		if owner != nil {
			if ce := owner.Close(); ce != nil {
				t.Error(ce)
			}
		}
		t.Fatal("missing existing root admitted", e)
	}
	if _, e := os.Lstat(missing); !os.IsNotExist(e) {
		t.Fatal("missing root was created", e)
	}
	empty := filepath.Join(parent, "empty")
	if e := os.Mkdir(empty, 0700); e != nil {
		t.Fatal(e)
	}
	if owner, e := openOwnedRootMode(empty, true); e == nil || owner != nil {
		if owner != nil {
			if ce := owner.Close(); ce != nil {
				t.Error(ce)
			}
		}
		t.Fatal("missing existing lock admitted", e)
	}
	rows, e := os.ReadDir(empty)
	if e != nil || len(rows) != 0 {
		t.Fatal("missing lock was recreated", e)
	}
}
func TestRemotePoolExistingRootUsesOriginalExclusiveLock(t *testing.T) {
	root := filepath.Join(remoteControllerTestParent(t), "existing")
	original, e := openOwnedRoot(root)
	if e != nil {
		t.Fatal(e)
	}
	if e = original.Close(); e != nil {
		t.Fatal(e)
	}
	original, e = openOwnedRootMode(root, true)
	if e != nil {
		t.Fatal(e)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			if e := original.Close(); e != nil {
				t.Error(e)
			}
		}
	})
	if replacement, e := openOwnedRootMode(root, true); e == nil || replacement != nil {
		if replacement != nil {
			if ce := replacement.Close(); ce != nil {
				t.Error(ce)
			}
		}
		t.Fatal("second owner bypassed original lock", e)
	}
	e = original.Close()
	closed = true
	if e != nil {
		t.Fatal(e)
	}
	next, e := openOwnedRootMode(root, true)
	if e != nil {
		t.Fatal(e)
	}
	if e = next.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestRemotePoolOriginalRootReplacementRefused(t *testing.T) {
	parent := remoteControllerTestParent(t)
	root := filepath.Join(parent, "owned")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	lock := filepath.Join(root, "instance-lock")
	if e := os.Mkdir(lock, 0700); e != nil {
		t.Fatal(e)
	}
	rootInfo, e := os.Lstat(root)
	if e != nil {
		t.Fatal(e)
	}
	lockInfo, e := os.Lstat(lock)
	if e != nil {
		t.Fatal(e)
	}
	c := &RemotePoolController{root: root, rootInfo: rootInfo, lockInfo: lockInfo}
	if e = c.auditRootIdentity(); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(root, root+".original"); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(lock, 0700); e != nil {
		t.Fatal(e)
	}
	if e = c.auditRootIdentity(); e == nil {
		t.Fatal("different original root was adopted")
	}
}
func TestRemotePoolRootsStaySeparate(t *testing.T) {
	parent := remoteControllerTestParent(t)
	root := filepath.Join(parent, "owned")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	if e := remotePoolRoots(root, filepath.Join(parent, "remote")); e != nil {
		t.Fatal(e)
	}
	for _, candidate := range []string{root, parent, filepath.Join(root, "remote"), "relative"} {
		if e := remotePoolRoots(root, candidate); e == nil {
			t.Fatal("overlapping/relative root admitted", candidate)
		}
	}
	alias := filepath.Join(parent, "alias")
	if e := os.Symlink(root, alias); e != nil {
		t.Fatal(e)
	}
	if e := remotePoolRoots(alias, filepath.Join(parent, "remote")); e == nil {
		t.Fatal("symlink-selected original root admitted")
	}
}

func TestRemotePoolExistingVaultMarkerRequired(t *testing.T) {
	root := remoteControllerTestParent(t)
	vault := filepath.Join(root, "hosted-vault")
	if e := os.Mkdir(vault, 0700); e != nil {
		t.Fatal(e)
	}
	marker := filepath.Join(vault, "SLUG-RECOVERY-VAULT.json")
	if e := requireRemoteHostedVaultMarker(root); e == nil {
		t.Fatal("missing established vault marker accepted")
	}
	if _, e := os.Lstat(marker); !os.IsNotExist(e) {
		t.Fatal("presence audit repaired vault marker", e)
	}
	// This helper proves presence/type only; OpenHosted still validates full bytes.
	if e := os.WriteFile(marker, []byte("{}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := requireRemoteHostedVaultMarker(root); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(marker, marker+".original"); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(marker+".original", marker); e != nil {
		t.Fatal(e)
	}
	if e := requireRemoteHostedVaultMarker(root); e == nil {
		t.Fatal("symlink marker accepted")
	}
}
