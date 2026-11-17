//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Kill the actual process only after the real synchronous LevelDB mutation.
// Expected evidence is always the independent published corpus, not child output.
func TestManifestOutboxProcessCrash(t *testing.T) {
	if stage := os.Getenv("SLUG_MANIFEST_CRASH_STAGE"); stage != "" {
		f, c := mwFixture(t)
		b, err := CreateManifestOutbox(os.Getenv("SLUG_MANIFEST_CRASH_BOX"), c)
		if err != nil {
			t.Fatal(err)
		}
		hook := func(string) {
			if err := os.WriteFile(os.Getenv("SLUG_MANIFEST_CRASH_SIGNAL"), []byte("synced"), 0600); err != nil {
				t.Fatal(err)
			}
			for {
				time.Sleep(time.Hour)
			}
		}
		scan := mwScan(f)
		if stage == "scan" {
			b.checkpoint = hook
		}
		if err = b.saveScan(&scan); err != nil {
			t.Fatal(err)
		}
		if stage == "found" {
			b.checkpoint = hook
		}
		scan.NextNonce = 1
		e, err := b.found(ManifestEntry{Version: 2, WorkHash: f.Verified.WorkHash, Job: f.Job, Candidate: f.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"}, scan)
		if err != nil {
			t.Fatal(err)
		}
		e.BlockState = "submitted"
		if err = b.update(e); err != nil {
			t.Fatal(err)
		}
		e.BlockState = "canonical"
		if err = b.update(e); err != nil {
			t.Fatal(err)
		}
		if stage == "authorization" {
			b.checkpoint = hook
		}
		e.Authorization = &f.Auth
		e.AdmissionState = "pending"
		if err = b.update(e); err != nil {
			t.Fatal(err)
		}
		if stage == "receipt" {
			b.checkpoint = hook
		}
		result := mwResult(f)
		e.Receipt = &result
		e.AdmissionState = "accepted"
		if err = b.update(e); err != nil {
			t.Fatal(err)
		}
		t.Fatal("checkpoint not reached")
	}
	for _, stage := range []string{"scan", "found", "authorization", "receipt"} {
		t.Run(stage, func(t *testing.T) {
			f, c := mwFixture(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "box")
			signal := filepath.Join(dir, "signal")
			cmd := exec.Command(os.Args[0], "-test.run=^TestManifestOutboxProcessCrash$", "-test.timeout=30s")
			cmd.Env = append(os.Environ(), "SLUG_MANIFEST_CRASH_STAGE="+stage, "SLUG_MANIFEST_CRASH_BOX="+path, "SLUG_MANIFEST_CRASH_SIGNAL="+signal)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			t.Cleanup(func() {
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			})
			deadline := time.Now().Add(15 * time.Second)
			for {
				if _, err := os.Stat(signal); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("durable checkpoint not reached")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Wait(); err == nil {
				t.Fatal("child did not crash")
			}
			waited = true
			b, err := OpenManifestOutbox(path, c)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			scan, exists, err := b.scan()
			if err != nil || !exists {
				t.Fatal("scan missing", err)
			}
			entries, err := b.List(0, 100)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "scan" {
				if scan.NextNonce != 0 || len(entries) != 0 {
					t.Fatal("unhashed scan advanced")
				}
				return
			}
			if scan.NextNonce != 1 || len(entries) != 1 || entries[0].WorkHash != f.Verified.WorkHash {
				t.Fatal("found/cursor atomicity lost")
			}
			e := entries[0]
			if stage == "found" {
				if e.Authorization != nil || e.Receipt != nil || e.BlockState != "pending" {
					t.Fatal("found changed")
				}
				return
			}
			if e.Authorization == nil || *e.Authorization != f.Auth {
				t.Fatal("authorization lost")
			}
			if stage == "authorization" {
				if e.Receipt != nil || e.AdmissionState != "pending" {
					t.Fatal("pending changed")
				}
				return
			}
			if e.Receipt == nil || e.AdmissionState != "accepted" || e.Receipt.Record.StaticBinding != f.Record.StaticBinding || b.meta.Pending != 0 || b.meta.Reserved != 0 {
				t.Fatal("receipt not recovered")
			}
		})
	}
}

func TestManifestWorkerCancellationDrainsFound(t *testing.T) {
	f, c := mwFixture(t)
	b, _ := mwBox(t, c)
	scan := mwScan(f)
	if err := b.saveScan(&scan); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &mwAuthority{c: c, f: f, submit: func() (r prospectservice.ManifestAdmissionResult, err error) {
		t.Error("transport after cancel")
		return
	}}
	n := &mwNode{f: f, call: func(string) { t.Error("node transport after canceled hash") }}
	w, err := NewManifest(c, a, n, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		t.Error("sign after cancel")
		return gymauth.Authorization{}, nil
	}, b)
	if err != nil {
		t.Fatal(err)
	}
	w.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS + 100) }
	original := w.hash
	entered := make(chan struct{})
	release := make(chan struct{})
	w.hash = func(h *wire.BlockHeader) chainhash.Hash { close(entered); <-release; return original(h) }
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	<-entered
	if !errors.Is(b.Close(), ErrInUse) {
		t.Fatal("close interrupted running hash")
	}
	cancel()
	select {
	case err := <-done:
		t.Fatal("Run abandoned active hash", err)
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	entries, err := b.List(0, 1)
	if err != nil || len(entries) != 1 || entries[0].Authorization != nil || entries[0].BlockState != "pending" {
		t.Fatal("canceled found not persisted", err)
	}
}
