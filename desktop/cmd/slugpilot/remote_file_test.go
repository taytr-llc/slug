//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
)

func remoteLaunchTestDir(t *testing.T) string {
	t.Helper()
	d, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(d, 0700); e != nil {
		t.Fatal(e)
	}
	return d
}
func remoteLaunchTestFile(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(remoteLaunchTestDir(t), "selected")
	if e := os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func remoteLaunchTestSelect(t *testing.T, p string) *remoteLaunchInput {
	t.Helper()
	f, e := selectRemoteLaunchInput(context.Background(), p, 262144)
	if e != nil {
		if f != nil {
			if ce := f.close(); ce != nil {
				t.Error(ce)
			}
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
func TestRemoteHelperSelectedFilesOriginalIdentity(t *testing.T) {
	public := []byte("PUBLIC SYNTHETIC RPC INPUT")
	for _, kind := range []string{"file-replace", "parent-replace", "content", "mode"} {
		t.Run(kind, func(t *testing.T) {
			p := remoteLaunchTestFile(t, public)
			f := remoteLaunchTestSelect(t, p)
			b, e := f.read(context.Background())
			if e != nil || !bytes.Equal(b, public) {
				t.Fatal(e)
			}
			clear(b)
			if e = os.WriteFile(filepath.Join(filepath.Dir(p), "sibling"), []byte("public"), 0600); e != nil {
				t.Fatal(e)
			}
			if e = f.check(context.Background()); e != nil {
				t.Fatal("parent contents are not identity", e)
			}
			switch kind {
			case "file-replace":
				if e = os.Rename(p, p+".retained"); e == nil {
					e = os.WriteFile(p, public, 0600)
				}
			case "parent-replace":
				d := filepath.Dir(p)
				if e = os.Rename(d, d+".retained"); e == nil {
					e = os.Mkdir(d, 0700)
				}
				if e == nil {
					e = os.WriteFile(p, public, 0600)
				}
				t.Cleanup(func() {
					if e := os.RemoveAll(d + ".retained"); e != nil {
						t.Error(e)
					}
				})
			case "content":
				e = os.WriteFile(p, []byte("DIFFERENT SYNTHETIC INPUT"), 0600)
			case "mode":
				e = os.Chmod(p, 0644)
			}
			if e != nil {
				t.Fatal(e)
			}
			if b, e = f.read(context.Background()); e == nil || b != nil {
				t.Fatal("changed original disclosed", e)
			}
		})
	}
}
func TestRemoteHelperSelectedFilesBoundsAndLinks(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "mode", "parent-mode", "empty", "oversize", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			p := remoteLaunchTestFile(t, []byte("PUBLIC"))
			ctx := context.Background()
			switch kind {
			case "symlink":
				if e := os.Symlink(p, p+".link"); e != nil {
					t.Fatal(e)
				}
				p += ".link"
			case "hardlink":
				if e := os.Link(p, p+".link"); e != nil {
					t.Fatal(e)
				}
			case "mode":
				if e := os.Chmod(p, 0644); e != nil {
					t.Fatal(e)
				}
			case "parent-mode":
				if e := os.Chmod(filepath.Dir(p), 0755); e != nil {
					t.Fatal(e)
				}
			case "empty":
				if e := os.WriteFile(p, nil, 0600); e != nil {
					t.Fatal(e)
				}
			case "oversize":
				if e := os.WriteFile(p, make([]byte, 65), 0600); e != nil {
					t.Fatal(e)
				}
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			f, e := selectRemoteLaunchInput(ctx, p, 64)
			if f != nil {
				if ce := f.close(); ce != nil {
					t.Fatal(ce)
				}
			}
			if e == nil || strings.Contains(e.Error(), p) {
				t.Fatal("unsafe input accepted or path leaked", e)
			}
		})
	}
}
func TestRemoteHelperCloseJoinsAndRetainsFailedFile(t *testing.T) {
	p := remoteLaunchTestFile(t, []byte("PUBLIC"))
	f, e := selectRemoteLaunchInput(context.Background(), p, 64)
	if e != nil {
		if f != nil {
			_ = f.close()
		}
		t.Fatal(e)
	}
	life, cancel := context.WithCancel(context.Background())
	o := &remoteLaunchOwner{life: life, cancel: cancel, files: [2]*remoteLaunchInput{f, nil}}
	o.active.Add(1)
	var release sync.Once
	releaseCall := func() { release.Do(o.active.Done) }
	joined := make(chan error, 1)
	go func() { joined <- o.Close() }()
	defer func() {
		releaseCall()
		if e := o.Close(); e != nil {
			t.Error(e)
		}
	}()
	select {
	case <-life.Done():
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel")
	}
	if f.closeDone {
		t.Fatal("selected file closed before call joined")
	}
	select {
	case <-joined:
		t.Fatal("Close did not wait original call")
	default:
	}
	releaseCall()
	select {
	case e := <-joined:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not join")
	}
	if !f.closeDone || o.Close() != nil {
		t.Fatal("completed close not sticky")
	}

	// Real failed-close leaf: never retry the descriptor after an ambiguous close.
	q := remoteLaunchTestFile(t, []byte("PUBLIC SECOND"))
	bad, e := selectRemoteLaunchInput(context.Background(), q, 64)
	if e != nil {
		if bad != nil {
			_ = bad.close()
		}
		t.Fatal(e)
	}
	if e = bad.file.Close(); e != nil {
		_ = bad.close()
		t.Fatal(e)
	}
	failed := &remoteLaunchOwner{files: [2]*remoteLaunchInput{bad, nil}}
	first := failed.Close()
	if !errors.Is(first, errRemoteLaunchCleanup) || failed.Close() != first || failed.files[0] != bad {
		t.Fatal("failed original owner lost", first)
	}
	held := &remoteLaunchError{cause: first, retained: failed}
	if held.retained != failed || strings.Contains(held.Error(), q) || !errors.Is(held, errRemoteLaunchCleanup) {
		t.Fatal("cleanup error did not retain private owner")
	}
}
func TestRemoteHelperCanceledOwnerAndChangedSelectionRefuseDispatch(t *testing.T) {
	for _, changed := range []bool{false, true} {
		p := remoteLaunchTestFile(t, []byte("PUBLIC CONFIG"))
		q := remoteLaunchTestFile(t, []byte("PUBLIC RPC"))
		a := remoteLaunchTestSelect(t, p)
		b := remoteLaunchTestSelect(t, q)
		life, cancel := context.WithCancel(context.Background())
		o := &remoteLaunchOwner{life: life, cancel: cancel, files: [2]*remoteLaunchInput{a, b}}
		t.Cleanup(func() {
			if e := o.Close(); e != nil {
				t.Error(e)
			}
		})
		if changed {
			if e := os.Chmod(q, 0644); e != nil {
				t.Fatal(e)
			}
		} else {
			cancel()
		}
		snap, e := o.WireSnapshot(context.Background())
		want := error(context.Canceled)
		if changed {
			want = errRemoteLaunchIntegrity
		}
		if !errors.Is(e, want) {
			t.Fatal("original failure cause lost before dispatch", e)
		}
		if e == nil || !o.closing || !a.closeDone || !b.closeDone {
			t.Fatal("invalid original selection not retired", snap, e)
		}
		if e = o.Start(context.Background(), strings.Repeat("1", 64)); e == nil {
			t.Fatal("closed owner admitted mutation")
		}
	}
}
func remoteLaunchReference(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile("../../pilot/testdata/remote-pool/reference-v1.json")
	if e != nil || len(b) != 563867 {
		t.Fatal("reference input", e)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != "5f67fcaaba0c78b97b0f116a4c598eb0ec62ee1436bbcf280edcc747856e5177" {
		t.Fatal("reference input changed")
	}
	var r struct {
		Positives []struct {
			CanonicalHex string `json:"canonical_hex"`
		} `json:"positives"`
	}
	if e = json.Unmarshal(b, &r); e != nil || len(r.Positives) != 4 {
		t.Fatal("reference scope", e)
	}
	raw, e := hex.DecodeString(r.Positives[0].CanonicalHex)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestRemoteHelperOriginalConfigBeforeControllerEffects(t *testing.T) {
	raw := remoteLaunchReference(t)
	root := remoteLaunchTestDir(t)
	config := filepath.Join(root, "config")
	rpc := filepath.Join(root, "rpc")
	if e := os.WriteFile(config, raw, 0600); e != nil {
		t.Fatal(e)
	}
	// An otherwise valid remote config must reach the local RPC profile refusal,
	// before opening any original wallet/account root or reading credential paths.
	if e := os.WriteFile(rpc, []byte(`{"version":1,"url":"https://127.0.0.1:29332","username":"public-test-user","password":"public-test-password","ca_certificate":""}`), 0600); e != nil {
		t.Fatal(e)
	}
	paths := remoteLaunchPaths{filepath.Join(root, "original"), filepath.Join(root, "attachment"), config, rpc, filepath.Join(root, "pool-missing"), filepath.Join(root, "admission-missing")}
	o, got, e := openRemoteLaunch(context.Background(), paths, pilot.PoolOptions{})
	if o != nil || !errors.Is(e, errRemoteLaunchInvalid) || got.ConnectionID == "" {
		t.Fatal("RPC profile did not refuse before concrete constructor", e)
	}
	for _, p := range []string{paths.data, paths.attachment, paths.poolCredential, paths.admissionCredential} {
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			t.Fatal("refusal touched later input")
		}
	}
	if strings.Contains(e.Error(), "public-test-password") || strings.Contains(e.Error(), root) {
		t.Fatal("private configuration leaked")
	}
}
