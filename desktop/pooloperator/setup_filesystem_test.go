//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func opSetupTestDir(t *testing.T) string {
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
func opSetupTestOpenFiles(t *testing.T, path string, s Setup, create bool) *operatorFiles {
	t.Helper()
	f, e := openOperatorFiles(path, s, create)
	if e != nil {
		if f != nil {
			_ = f.close()
		}
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := f.close(); e != nil {
			t.Error(e)
		}
	})
	return f
}
func opSetupTestRefused(t *testing.T, path string, s Setup, create bool) {
	t.Helper()
	f, e := openOperatorFiles(path, s, create)
	if f != nil {
		ce := f.close()
		if ce != nil {
			t.Fatal("refused selection cleanup", ce)
		}
	}
	if e == nil {
		t.Fatal("invalid original selection admitted")
	}
}
func opSetupTestAdopt(t *testing.T, f *operatorFiles, name string) {
	t.Helper()
	path, create, e := f.child(name)
	if e != nil || !create {
		t.Fatal("missing exact creation permission", name, e)
	}
	if e = os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	if e = f.adopt(name); e != nil {
		t.Fatal(e)
	}
}
func opSetupTestCompleteFilesystem(t *testing.T, f *operatorFiles) {
	t.Helper()
	for _, name := range componentNames {
		path, create, e := f.child(name)
		if e != nil {
			t.Fatal(e)
		}
		if create {
			if e = os.Mkdir(path, 0700); e != nil {
				t.Fatal(e)
			}
		}
		if e = f.adopt(name); e != nil {
			t.Fatal(e)
		}
	}
	if e := f.markReady(); e != nil {
		t.Fatal(e)
	}
}

// These tests exercise the filesystem owner only. Empty synthetic child
// directories are not child journal audits or a claim that Runtime is READY.
func TestOperatorFilesCreateResumeReady(t *testing.T) {
	v := opSetupTestReference(t).Positives[0]
	path := filepath.Join(opSetupTestDir(t), "operator")
	first := opSetupTestOpenFiles(t, path, v.Setup, true)
	intent, e := os.ReadFile(filepath.Join(path, intentName))
	if e != nil || !bytes.Equal(intent, opSetupTestHex(t, v.Intent)) {
		t.Fatal("independent intent", e)
	}
	if _, e = os.Lstat(filepath.Join(path, readyName)); !os.IsNotExist(e) {
		t.Fatal("READY before children")
	}
	if e = first.markReady(); !errors.Is(e, ErrIntegrity) {
		t.Fatal("incomplete child set marked ready", e)
	}
	opSetupTestAdopt(t, first, "issuer")
	if e = first.close(); e != nil {
		t.Fatal(e)
	}
	// Read/open cannot mistake a retained incomplete intent for READY.
	opSetupTestRefused(t, path, v.Setup, false)
	resumed := opSetupTestOpenFiles(t, path, v.Setup, true)
	child, create, e := resumed.child("issuer")
	if e != nil || create || child != filepath.Join(path, "issuer") {
		t.Fatal("retained child recreated", e)
	}
	if e = resumed.adopt("issuer"); e != nil {
		t.Fatal(e)
	}
	opSetupTestCompleteFilesystem(t, resumed)
	ready, e := os.ReadFile(filepath.Join(path, readyName))
	if e != nil || !bytes.Equal(ready, opSetupTestHex(t, v.Ready)) {
		t.Fatal("independent READY", e)
	}
	before := append([]byte(nil), ready...)
	if e = resumed.markReady(); e != nil {
		t.Fatal("exact repeated READY check", e)
	}
	ready, e = os.ReadFile(filepath.Join(path, readyName))
	if e != nil || !bytes.Equal(before, ready) {
		t.Fatal("READY rewritten", e)
	}
	if e = resumed.close(); e != nil {
		t.Fatal(e)
	}
	// Established roots admit only Open, never a fresh Create path.
	opSetupTestRefused(t, path, v.Setup, true)
	opened := opSetupTestOpenFiles(t, path, v.Setup, false)
	if !opened.established {
		t.Fatal("established state lost")
	}
	for _, name := range componentNames {
		_, create, e := opened.child(name)
		if e != nil || create {
			t.Fatal("established child create permission", name, e)
		}
	}
	if e = opened.close(); e != nil {
		t.Fatal(e)
	}
	if e = opened.check(); !errors.Is(e, ErrIntegrity) {
		t.Fatal("closed filesystem owner revived", e)
	}
}
func TestOperatorFilesOriginalSetupAndMissingChild(t *testing.T) {
	refs := opSetupTestReference(t)
	v := refs.Positives[0]
	path := filepath.Join(opSetupTestDir(t), "operator")
	f := opSetupTestOpenFiles(t, path, v.Setup, true)
	opSetupTestCompleteFilesystem(t, f)
	if e := f.close(); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(filepath.Join(path, intentName))
	if e != nil {
		t.Fatal(e)
	}
	opSetupTestRefused(t, path, refs.Positives[1].Setup, false)
	after, e := os.ReadFile(filepath.Join(path, intentName))
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("changed expected setup adopted/rewritten", e)
	}
	if e = os.Remove(filepath.Join(path, "settlement")); e != nil {
		t.Fatal(e)
	}
	opSetupTestRefused(t, path, v.Setup, false)
	if _, e = os.Lstat(filepath.Join(path, "settlement")); !os.IsNotExist(e) {
		t.Fatal("missing financial child recreated")
	}
}
func TestOperatorFilesLockAndIdentity(t *testing.T) {
	v := opSetupTestReference(t).Positives[0]
	base := opSetupTestDir(t)
	path := filepath.Join(base, "operator")
	f := opSetupTestOpenFiles(t, path, v.Setup, true)
	opSetupTestRefused(t, path, v.Setup, true)
	if e := f.check(); e != nil {
		t.Fatal("competing attempt changed original owner", e)
	}
	if _, _, e := f.child("../other"); !errors.Is(e, ErrInvalid) {
		t.Fatal("foreign child path", e)
	}
	opSetupTestAdopt(t, f, "issuer")
	if e := os.Rename(filepath.Join(path, "issuer"), filepath.Join(base, "prior-issuer")); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(path, "issuer"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := f.check(); !errors.Is(e, ErrIntegrity) {
		t.Fatal("replacement child adopted", e)
	}
	if e := f.close(); e != nil {
		t.Fatal(e)
	}
	// A new exact partial resume may select the remaining on-disk identities;
	// the closed prior handle itself cannot be revived or retargeted.
	next := opSetupTestOpenFiles(t, path, v.Setup, true)
	if e := next.check(); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(path, filepath.Join(base, "prior-root")); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	if e := next.check(); !errors.Is(e, ErrIntegrity) {
		t.Fatal("renamed root retargeted", e)
	}
}
func TestOperatorFilesNamespaceAndSymlinkRefusals(t *testing.T) {
	v := opSetupTestReference(t).Positives[0]
	for _, name := range []string{"foreign-entry", "intent-symlink", "component-symlink", "broad-root", "root-symlink", "ready-byte-change"} {
		t.Run(name, func(t *testing.T) {
			base := opSetupTestDir(t)
			path := filepath.Join(base, "operator")
			if name == "root-symlink" {
				target := filepath.Join(base, "target")
				if e := os.Mkdir(target, 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(target, path); e != nil {
					t.Fatal(e)
				}
				opSetupTestRefused(t, path, v.Setup, true)
				return
			}
			f := opSetupTestOpenFiles(t, path, v.Setup, true)
			if name == "ready-byte-change" {
				opSetupTestCompleteFilesystem(t, f)
			}
			if e := f.close(); e != nil {
				t.Fatal(e)
			}
			create := true
			switch name {
			case "foreign-entry":
				if e := os.WriteFile(filepath.Join(path, "unknown"), []byte("public fixture"), 0600); e != nil {
					t.Fatal(e)
				}
			case "intent-symlink":
				original := filepath.Join(base, "intent-original")
				if e := os.Rename(filepath.Join(path, intentName), original); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(original, filepath.Join(path, intentName)); e != nil {
					t.Fatal(e)
				}
			case "component-symlink":
				target := filepath.Join(base, "outside")
				if e := os.Mkdir(target, 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(target, filepath.Join(path, "settlement")); e != nil {
					t.Fatal(e)
				}
			case "broad-root":
				if e := os.Chmod(path, 0755); e != nil {
					t.Fatal(e)
				}
			case "ready-byte-change":
				raw := opSetupTestHex(t, v.Ready)
				raw[len(raw)-1] = ' '
				if e := os.WriteFile(filepath.Join(path, readyName), raw, 0600); e != nil {
					t.Fatal(e)
				}
				create = false
			}
			opSetupTestRefused(t, path, v.Setup, create)
		})
	}
}

func opSetupTestForcePrivateClose(s *privateInput) {
	if s == nil {
		return
	}
	if s.file != nil {
		_ = s.file.Close()
	}
	for _, d := range s.dirs {
		if d.file != nil {
			_ = d.file.Close()
		}
	}
}
func opSetupTestForceFilesClose(f *operatorFiles) {
	if f == nil {
		return
	}
	opSetupTestForcePrivateClose(f.ready)
	opSetupTestForcePrivateClose(f.intent)
	for _, d := range f.children {
		if d.file != nil {
			_ = d.file.Close()
		}
	}
	for _, file := range f.failedClosers {
		_ = file.Close()
	}
	for _, d := range f.dirs {
		if d.file != nil {
			_ = d.file.Close()
		}
	}
}
func TestOperatorFilesFailedCloseRetainsRootLock(t *testing.T) {
	v := opSetupTestReference(t).Positives[0]
	for _, stage := range []string{"temporary", "intent", "child", "ancestor"} {
		t.Run(stage, func(t *testing.T) {
			path := filepath.Join(opSetupTestDir(t), "operator")
			f, e := openOperatorFiles(path, v.Setup, true)
			if e != nil {
				opSetupTestForceFilesClose(f)
				t.Fatal(e)
			}
			defer opSetupTestForceFilesClose(f)
			var failed *os.File
			switch stage {
			case "temporary":
				failed, e = os.Open(path)
				if e != nil {
					t.Fatal(e)
				}
				if e = failed.Close(); e != nil {
					t.Fatal(e)
				}
				if e = f.closeTemporary(failed); !errors.Is(e, ErrCleanup) {
					t.Fatal(e)
				}
			case "intent":
				failed = f.intent.file
				if e = failed.Close(); e != nil {
					t.Fatal(e)
				}
			case "child":
				opSetupTestAdopt(t, f, "issuer")
				failed = f.children["issuer"].file
				if e = failed.Close(); e != nil {
					t.Fatal(e)
				}
			case "ancestor":
				failed = f.dirs[0].file
				if e = failed.Close(); e != nil {
					t.Fatal(e)
				}
			}
			first := f.close()
			if !errors.Is(first, ErrCleanup) || f.close() != first {
				t.Fatal("cleanup uncertainty erased", first)
			}
			if _, e = statFD(f.root()); e != nil {
				t.Fatal("root lock owner released after earlier close failure", e)
			}
			if stage == "temporary" || stage == "child" || stage == "ancestor" {
				found := false
				for _, held := range f.failedClosers {
					if held == failed {
						found = true
					}
				}
				if !found {
					t.Fatal("exact failed owner dropped")
				}
			}
			if stage == "intent" && f.intent.file != failed {
				t.Fatal("failed selected input owner dropped")
			}
			opSetupTestRefused(t, path, v.Setup, true)
			if e = f.intent.check(context.Background()); stage == "intent" && e == nil {
				t.Fatal("failed input revived")
			}
		})
	}
}
