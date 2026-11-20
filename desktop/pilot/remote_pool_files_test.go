//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/wire/v2"
)

func remoteAttachmentTestDir(t *testing.T) string {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-profile attachment")
	}
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(p, 0700); e != nil {
		t.Fatal(e)
	}
	return p
}
func remoteAttachmentTestOpenFiles(t *testing.T, path string, s RemotePoolConfig, create bool) *remotePoolFiles {
	t.Helper()
	f, e := openRemotePoolFiles(path, s, create)
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
func remoteAttachmentTestRefused(t *testing.T, path string, s RemotePoolConfig, create bool) {
	t.Helper()
	f, e := openRemotePoolFiles(path, s, create)
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
func remoteAttachmentTestAdopt(t *testing.T, f *remotePoolFiles, name string) {
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
func remoteAttachmentTestCompleteFilesystem(t *testing.T, f *remotePoolFiles) {
	t.Helper()
	for _, name := range remotePoolComponentNames {
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
func TestRemotePoolFilesCreateResumeReady(t *testing.T) {
	v := remotePoolTestReference(t).Positives[0]
	path := filepath.Join(remoteAttachmentTestDir(t), "operator")
	first := remoteAttachmentTestOpenFiles(t, path, v.Config, true)
	intent, e := os.ReadFile(filepath.Join(path, remotePoolIntentName))
	if e != nil || !bytes.Equal(intent, remotePoolTestHex(t, v.IntentHex)) {
		t.Fatal("independent intent", e)
	}
	if _, e = os.Lstat(filepath.Join(path, remotePoolReadyName)); !os.IsNotExist(e) {
		t.Fatal("READY before children")
	}
	if e = first.markReady(); !errors.Is(e, ErrRemotePoolChanged) {
		t.Fatal("incomplete child set marked ready", e)
	}
	remoteAttachmentTestAdopt(t, first, "outbox")
	if e = first.close(); e != nil {
		t.Fatal(e)
	}
	// Read/open cannot mistake a retained incomplete intent for READY.
	remoteAttachmentTestRefused(t, path, v.Config, false)
	resumed := remoteAttachmentTestOpenFiles(t, path, v.Config, true)
	child, create, e := resumed.child("outbox")
	if e != nil || create || child != filepath.Join(path, "outbox") {
		t.Fatal("retained child recreated", e)
	}
	if e = resumed.adopt("outbox"); e != nil {
		t.Fatal(e)
	}
	remoteAttachmentTestCompleteFilesystem(t, resumed)
	ready, e := os.ReadFile(filepath.Join(path, remotePoolReadyName))
	if e != nil || !bytes.Equal(ready, remotePoolTestHex(t, v.ReadyHex)) {
		t.Fatal("independent READY", e)
	}
	before := append([]byte(nil), ready...)
	if e = resumed.markReady(); e != nil {
		t.Fatal("exact repeated READY check", e)
	}
	ready, e = os.ReadFile(filepath.Join(path, remotePoolReadyName))
	if e != nil || !bytes.Equal(before, ready) {
		t.Fatal("READY rewritten", e)
	}
	if e = resumed.close(); e != nil {
		t.Fatal(e)
	}
	// The attachment factory supports exact resume/open only; an established
	// root can never grant creation permission for a missing child.
	opened := remoteAttachmentTestOpenFiles(t, path, v.Config, true)
	if !opened.established {
		t.Fatal("established state lost")
	}
	for _, name := range remotePoolComponentNames {
		_, create, e := opened.child(name)
		if e != nil || create {
			t.Fatal("established child create permission", name, e)
		}
	}
	if e = opened.close(); e != nil {
		t.Fatal(e)
	}
	if e = opened.check(); !errors.Is(e, ErrRemotePoolChanged) {
		t.Fatal("closed filesystem owner revived", e)
	}
}
func TestRemotePoolFilesOriginalSetupAndMissingChild(t *testing.T) {
	refs := remotePoolTestReference(t)
	v := refs.Positives[0]
	path := filepath.Join(remoteAttachmentTestDir(t), "operator")
	f := remoteAttachmentTestOpenFiles(t, path, v.Config, true)
	remoteAttachmentTestCompleteFilesystem(t, f)
	if e := f.close(); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(filepath.Join(path, remotePoolIntentName))
	if e != nil {
		t.Fatal(e)
	}
	remoteAttachmentTestRefused(t, path, refs.Positives[1].Config, false)
	after, e := os.ReadFile(filepath.Join(path, remotePoolIntentName))
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("changed expected setup adopted/rewritten", e)
	}
	if e = os.Remove(filepath.Join(path, "outbox")); e != nil {
		t.Fatal(e)
	}
	remoteAttachmentTestRefused(t, path, v.Config, false)
	if _, e = os.Lstat(filepath.Join(path, "outbox")); !os.IsNotExist(e) {
		t.Fatal("missing outbox child recreated")
	}
}
func TestRemotePoolFilesLockAndIdentity(t *testing.T) {
	v := remotePoolTestReference(t).Positives[0]
	base := remoteAttachmentTestDir(t)
	path := filepath.Join(base, "operator")
	f := remoteAttachmentTestOpenFiles(t, path, v.Config, true)
	remoteAttachmentTestRefused(t, path, v.Config, true)
	if e := f.check(); e != nil {
		t.Fatal("competing attempt changed original owner", e)
	}
	if _, _, e := f.child("../other"); !errors.Is(e, ErrRemotePoolConfig) {
		t.Fatal("foreign child path", e)
	}
	remoteAttachmentTestAdopt(t, f, "outbox")
	if e := os.Rename(filepath.Join(path, "outbox"), filepath.Join(base, "prior-issuer")); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(path, "outbox"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := f.check(); !errors.Is(e, ErrRemotePoolChanged) {
		t.Fatal("replacement child adopted", e)
	}
	if e := f.close(); e != nil {
		t.Fatal(e)
	}
	// A new exact partial resume may select the remaining on-disk identities;
	// the closed prior handle itself cannot be revived or retargeted.
	next := remoteAttachmentTestOpenFiles(t, path, v.Config, true)
	if e := next.check(); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(path, filepath.Join(base, "prior-root")); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	if e := next.check(); !errors.Is(e, ErrRemotePoolChanged) {
		t.Fatal("renamed root retargeted", e)
	}
}
func TestRemotePoolFilesNamespaceAndSymlinkRefusals(t *testing.T) {
	v := remotePoolTestReference(t).Positives[0]
	for _, name := range []string{"foreign-entry", "intent-symlink", "component-symlink", "broad-root", "root-symlink", "ready-byte-change"} {
		t.Run(name, func(t *testing.T) {
			base := remoteAttachmentTestDir(t)
			path := filepath.Join(base, "operator")
			if name == "root-symlink" {
				target := filepath.Join(base, "target")
				if e := os.Mkdir(target, 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(target, path); e != nil {
					t.Fatal(e)
				}
				remoteAttachmentTestRefused(t, path, v.Config, true)
				return
			}
			f := remoteAttachmentTestOpenFiles(t, path, v.Config, true)
			if name == "ready-byte-change" {
				remoteAttachmentTestCompleteFilesystem(t, f)
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
				if e := os.Rename(filepath.Join(path, remotePoolIntentName), original); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(original, filepath.Join(path, remotePoolIntentName)); e != nil {
					t.Fatal(e)
				}
			case "component-symlink":
				target := filepath.Join(base, "outside")
				if e := os.Mkdir(target, 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(target, filepath.Join(path, "outbox")); e != nil {
					t.Fatal(e)
				}
			case "broad-root":
				if e := os.Chmod(path, 0755); e != nil {
					t.Fatal(e)
				}
			case "ready-byte-change":
				raw := remotePoolTestHex(t, v.ReadyHex)
				raw[len(raw)-1] = ' '
				if e := os.WriteFile(filepath.Join(path, remotePoolReadyName), raw, 0600); e != nil {
					t.Fatal(e)
				}
				create = false
			}
			remoteAttachmentTestRefused(t, path, v.Config, create)
		})
	}
}

func remoteAttachmentTestForcePrivateClose(s *remotePoolPrivateInput) {
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
func remoteAttachmentTestForceFilesClose(f *remotePoolFiles) {
	if f == nil {
		return
	}
	remoteAttachmentTestForcePrivateClose(f.ready)
	remoteAttachmentTestForcePrivateClose(f.intent)
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
func TestRemotePoolFilesFailedCloseRetainsRootLock(t *testing.T) {
	v := remotePoolTestReference(t).Positives[0]
	for _, stage := range []string{"temporary", "intent", "child", "ancestor"} {
		t.Run(stage, func(t *testing.T) {
			path := filepath.Join(remoteAttachmentTestDir(t), "operator")
			f, e := openRemotePoolFiles(path, v.Config, true)
			if e != nil {
				remoteAttachmentTestForceFilesClose(f)
				t.Fatal(e)
			}
			defer remoteAttachmentTestForceFilesClose(f)
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
				if e = f.closeTemporary(failed); !errors.Is(e, ErrRemotePoolCleanup) {
					t.Fatal(e)
				}
			case "intent":
				failed = f.intent.file
				if e = failed.Close(); e != nil {
					t.Fatal(e)
				}
			case "child":
				remoteAttachmentTestAdopt(t, f, "outbox")
				failed = f.children["outbox"].file
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
			if !errors.Is(first, ErrRemotePoolCleanup) || f.close() != first {
				t.Fatal("cleanup uncertainty erased", first)
			}
			if _, e = remotePoolStatFD(f.root()); e != nil {
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
			remoteAttachmentTestRefused(t, path, v.Config, true)
			if e = f.intent.check(context.Background()); stage == "intent" && e == nil {
				t.Fatal("failed input revived")
			}
		})
	}
}

// This is a real empty durable outbox; no worker is constructed or run.
func TestRemotePoolFilesActualOutboxReopen(t *testing.T) {
	v := remotePoolTestReference(t).Positives[0]
	path := filepath.Join(remoteAttachmentTestDir(t), "attachment")
	files := remoteAttachmentTestOpenFiles(t, path, v.Config, true)
	child, create, e := files.child("outbox")
	if e != nil || !create {
		t.Fatal(e)
	}
	box, e := prospectworker.CreatePoolOutbox(child, v.Worker)
	if e != nil {
		t.Fatal(e)
	}
	if e = files.adopt("outbox"); e != nil {
		t.Fatal(e)
	}
	if e = files.markReady(); e != nil {
		t.Fatal(e)
	}
	if e = box.Close(); e != nil {
		t.Fatal(e)
	}
	if e = files.close(); e != nil {
		t.Fatal(e)
	}
	reopened := remoteAttachmentTestOpenFiles(t, path, v.Config, true)
	child, create, e = reopened.child("outbox")
	if e != nil || create {
		t.Fatal("reopen granted create", e)
	}
	box, e = prospectworker.OpenPoolOutbox(child, v.Worker)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := box.Close(); e != nil {
			t.Error(e)
		}
	}()
	rows, e := box.List(0, 100)
	if e != nil || len(rows) != 0 {
		t.Fatal("read-only reopen changed outbox", e)
	}
}

// Synthetic admitted work exercises the attachment join, using the actual
// filesystem owner. It grants no successful wallet/node/worker startup claim.
func TestRemotePoolAttachmentCloseJoinsAndRetainsFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "failed-close"}[fail], func(t *testing.T) {
			c := remotePoolTestReference(t).Positives[0].Config
			f, e := openRemotePoolFiles(filepath.Join(remoteAttachmentTestDir(t), "attachment"), c, true)
			if e != nil {
				t.Fatal(e)
			}
			defer remoteAttachmentTestForceFilesClose(f)
			life, cancel := context.WithCancel(t.Context())
			a := &RemotePoolAttachment{files: f, life: life, cancel: cancel, closeDone: make(chan struct{})}
			if fail {
				if e = f.intent.file.Close(); e != nil {
					t.Fatal(e)
				}
			}
			a.active.Add(1)
			entered := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			go func() { defer a.active.Done(); <-life.Done(); close(entered); <-release }()
			result := make(chan error, 1)
			go func() { result <- a.Close() }()
			defer func() {
				cancel()
				once.Do(func() { close(release) })
				select {
				case <-a.closeDone:
				case <-time.After(2 * time.Second):
					t.Error("cleanup did not join the original close")
				}
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("close did not cancel")
			}
			select {
			case <-result:
				t.Fatal("close did not join admitted work")
			default:
			}
			if _, e = remotePoolStatFD(f.root()); e != nil {
				t.Fatal("root released before join", e)
			}
			once.Do(func() { close(release) })
			var first error
			select {
			case first = <-result:
			case <-time.After(2 * time.Second):
				t.Fatal("close failed to join")
			}
			if fail {
				if !errors.Is(first, ErrRemotePoolCleanup) || a.Close() != first {
					t.Fatal("lost sticky failure", first)
				}
				if _, e = remotePoolStatFD(f.root()); e != nil {
					t.Fatal("failed-close released root", e)
				}
			} else if first != nil || a.Close() != nil {
				t.Fatal(first)
			}
		})
	}
}
