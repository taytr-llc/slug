// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func candidateTestRoot(t *testing.T) string {
	t.Helper()
	// macOS's temporary root may use the /var alias; tests deliberately pass
	// real absolute paths so rejected symlinks are the ones each test creates.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func candidateTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		value := fmt.Sprint(info.Mode())
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += ":" + string(body)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += ":" + target
		}
		tree[rel] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func candidateMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateCustomNestedOldRootDoesNotMutate(t *testing.T) {
	for _, marker := range []string{"SLUG-PILOT.json", "SLUG-ACCOUNTS.json"} {
		t.Run(marker, func(t *testing.T) {
			root := candidateTestRoot(t)
			old := filepath.Join(root, "custom-retained-old-root")
			candidateMkdir(t, old)
			if err := os.WriteFile(filepath.Join(old, marker), []byte("retained identity; must not change\n"), 0600); err != nil {
				t.Fatal(err)
			}
			before := candidateTree(t, root)
			// This is the original native failure: MkdirAll used to create the
			// missing nested parent before the helper refused the old marker.
			for _, target := range []string{
				filepath.Join(old, "missing-parent", "candidate"),
				filepath.Join(old, "candidate"),
			} {
				if err := prepareCandidateDataParent(target, root, false); err == nil {
					t.Fatal("accepted nested old managed root")
				}
				if after := candidateTree(t, root); !reflect.DeepEqual(before, after) {
					t.Fatalf("refused startup changed retained tree: before=%v after=%v", before, after)
				}
			}
		})
	}
}

func TestCandidateCustomParentIsReadOnly(t *testing.T) {
	root := candidateTestRoot(t)
	before := candidateTree(t, root)
	if err := prepareCandidateDataParent(filepath.Join(root, "candidate"), root, false); err != nil {
		t.Fatal(err)
	}
	if err := prepareCandidateDataParent(filepath.Join(root, "missing", "candidate"), root, false); err == nil {
		t.Fatal("created a custom missing parent")
	}
	if after := candidateTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("custom startup mutated tree: %v", after)
	}
}

func TestCandidateDefaultCreatesOnlySharedParent(t *testing.T) {
	root := candidateTestRoot(t)
	target := filepath.Join(root, "SLUG", "LocalManifestPilot")
	before := candidateTree(t, root)
	if err := prepareCandidateDataParent(target, root, false); err == nil {
		t.Fatal("explicit default-spelled path created a missing parent")
	}
	if after := candidateTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("explicit custom refusal mutated tree")
	}
	if err := prepareCandidateDataParent(target, root, true); err != nil {
		t.Fatal(err)
	}
	shared, err := os.Lstat(filepath.Join(root, "SLUG"))
	if err != nil || !shared.IsDir() || shared.Mode().Perm() != 0700 {
		t.Fatalf("shared directory shape: %v %v", shared, err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("native startup created candidate root: %v", err)
	}
	// Old siblings are allowed, and neither their marker nor data is touched.
	for _, name := range []string{"LocalPilot", "LocalWalletPilot", "LocalAccountPilot"} {
		old := filepath.Join(root, "SLUG", name)
		candidateMkdir(t, old)
		if err := os.WriteFile(filepath.Join(old, "SLUG-PILOT.json"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before = candidateTree(t, root)
	if err := prepareCandidateDataParent(target, root, true); err != nil {
		t.Fatal(err)
	}
	if after := candidateTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("default reopen changed old sibling data")
	}
}

func TestCandidateUnsafeParentsRefuseWithoutMutation(t *testing.T) {
	for _, kind := range []string{"old-name", "old-marker", "candidate-ancestor", "symlink-config", "symlink-shared", "symlink-selected", "file-parent", "missing-config"} {
		t.Run(kind, func(t *testing.T) {
			root := candidateTestRoot(t)
			config := filepath.Join(root, "config")
			candidateMkdir(t, config)
			target := filepath.Join(config, "SLUG", "LocalManifestPilot")
			switch kind {
			case "old-name":
				config = filepath.Join(root, "LocalAccountPilot")
				candidateMkdir(t, config)
			case "old-marker", "candidate-ancestor":
				marker := "SLUG-ACCOUNTS.json"
				if kind == "candidate-ancestor" {
					marker = "SLUG-MANIFEST-ACCOUNTS.json"
				}
				if err := os.WriteFile(filepath.Join(config, marker), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink-config":
				config = filepath.Join(root, "alias")
				if err := os.Symlink(filepath.Join(root, "config"), config); err != nil {
					t.Fatal(err)
				}
			case "symlink-shared", "symlink-selected":
				link := filepath.Join(config, "SLUG")
				if kind == "symlink-selected" {
					candidateMkdir(t, link)
					link = target
				}
				if err := os.Symlink(root, link); err != nil {
					t.Fatal(err)
				}
			case "file-parent":
				if err := os.WriteFile(filepath.Join(config, "SLUG"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-config":
				config = filepath.Join(root, "missing")
			}
			target = filepath.Join(config, "SLUG", "LocalManifestPilot")
			before := candidateTree(t, root)
			if err := prepareCandidateDataParent(target, config, true); err == nil {
				t.Fatal("accepted unsafe default parent")
			}
			if after := candidateTree(t, root); !reflect.DeepEqual(before, after) {
				t.Fatalf("refusal mutated tree: %v", after)
			}
		})
	}
}

func TestCandidateExistingSelectedMarkerRemainsBackendOwned(t *testing.T) {
	root := candidateTestRoot(t)
	target := filepath.Join(root, "LocalManifestPilot")
	candidateMkdir(t, target)
	if err := os.WriteFile(filepath.Join(target, "SLUG-MANIFEST-ACCOUNTS.json"), []byte("backend must validate this"), 0600); err != nil {
		t.Fatal(err)
	}
	before := candidateTree(t, root)
	if err := prepareCandidateDataParent(target, root, false); err != nil {
		t.Fatal(err)
	}
	if after := candidateTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("native preflight changed candidate marker")
	}
}
