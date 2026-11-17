// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/wire/v2"
)

// Explicitly opt-in actual-node fault test. Every directory is test-owned; the
// public fixture is prepared at genesis, and Start is never called. Unlike a
// clean Close test, the parent kills the process only after a durable checkpoint.
type stableInstallCrashInput struct{ Root, Node, Stage, Signal string }
type stableInstallEvidence struct {
	Snapshot       pilot.ManifestSnapshot
	Marker         stableAccountMarker
	ChildMarker    []byte
	WalletHash     [32]byte
	Source, Outbox map[string][]byte
}

func TestStableInstallProcessCrash(t *testing.T) {
	if input := os.Getenv("SLUG_STABLE_INSTALL_CRASH_INPUT"); input != "" {
		stableInstallCrashChild(t, input)
		return
	}
	if wire.TestPoW {
		t.Skip("normal fixture identity and node required")
	}
	node := os.Getenv("SLUG_STABLE_NODE")
	if node == "" {
		t.Skip("set SLUG_STABLE_NODE to an immutable normal-PoW slugd")
	}
	if !filepath.IsAbs(node) {
		t.Fatal("node path must be absolute")
	}
	for _, stage := range []string{"stable-prepared", "stable-created", "stable-ready"} {
		t.Run(stage, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "accounts")
			in := stableInstallCrashInput{root, node, stage, filepath.Join(filepath.Dir(root), "checkpoint.json")}
			input := filepath.Join(filepath.Dir(root), "input.json")
			raw, _ := json.Marshal(in)
			if err := os.WriteFile(input, raw, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestStableInstallProcessCrash$", "-test.timeout=180s", "-test.v")
			cmd.Env = append(os.Environ(), "SLUG_STABLE_INSTALL_CRASH_INPUT="+input)
			log, err := os.Create(filepath.Join(filepath.Dir(root), "child.log"))
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			cmd.Stdout, cmd.Stderr = log, log
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			joined := false
			defer func() {
				if !joined {
					_ = cmd.Process.Kill()
					<-done
				}
			}()
			deadline := time.NewTimer(150 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
		wait:
			for {
				if _, err := os.Stat(in.Signal); err == nil {
					break wait
				}
				select {
				case err := <-done:
					joined = true
					output, _ := os.ReadFile(log.Name())
					t.Fatalf("child exited before %s: %v\n%s", stage, err, output)
				case <-deadline.C:
					t.Fatal("checkpoint timeout")
				case <-tick.C:
				}
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			err = <-done
			joined = true
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("not a killed child: %v", err)
			}
			ws, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
				t.Fatalf("not SIGKILL: %v", err)
			}
			raw, err = os.ReadFile(in.Signal)
			if err != nil {
				t.Fatal(err)
			}
			var before stableInstallEvidence
			if err = json.Unmarshal(raw, &before); err != nil {
				t.Fatal(err)
			}
			if len(before.Marker.StableInstalls) != 1 {
				t.Fatal("missing durable install")
			}
			entry := before.Marker.StableInstalls[0]
			want := "prepared"
			if stage == "stable-ready" {
				want = "ready"
			}
			if entry.Phase != want {
				t.Fatalf("phase=%s want=%s", entry.Phase, want)
			}
			childDir := filepath.Join(root, "gyms", entry.AccountID)
			stablePath := filepath.Join(childDir, "stable")
			if stage == "stable-prepared" {
				if _, err = os.Lstat(stablePath); !os.IsNotExist(err) {
					t.Fatal("prepared checkpoint unexpectedly has component")
				}
			} else if _, err = os.Stat(stablePath); err != nil {
				t.Fatal("acknowledged component missing", err)
			}
			stableInstallAssertRetained(t, childDir, before, true)
			cfg := pilot.Config{DataDir: root, NodeBinary: node}
			markerRaw, _ := os.ReadFile(filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json"))
			if old, err := NewManifest(cfg); err == nil || old != nil {
				if old != nil {
					_ = old.Close()
				}
				t.Fatal("legacy route accepted root v2")
			}
			afterOld, _ := os.ReadFile(filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json"))
			if !bytes.Equal(markerRaw, afterOld) {
				t.Fatal("old-route refusal rewrote root")
			}

			if stage == "stable-prepared" {
				for _, name := range []string{"unknown", "SLUG-STABLE.json"} {
					if err = os.Mkdir(stablePath, 0700); err != nil {
						t.Fatal(err)
					}
					partial := filepath.Join(stablePath, name)
					retained := []byte("deliberately incomplete test-owned component\n")
					if err = os.WriteFile(partial, retained, 0600); err != nil {
						t.Fatal(err)
					}
					rejected, e := NewStable(cfg)
					if e == nil {
						t.Cleanup(func() { _ = rejected.Close() })
						if e = rejected.LoginFixture("zero256"); e != nil {
							_ = rejected.Close()
							t.Fatal(e)
						}
						stableInstallWaitLogin(t, rejected.ManifestController, false)
						if e = rejected.Close(); e != nil {
							t.Fatal(e)
						}
					} else if rejected != nil {
						t.Fatal("constructor error returned live controller")
					}
					got, e := os.ReadFile(partial)
					if e != nil || !bytes.Equal(got, retained) {
						t.Fatal("unknown component changed", e)
					}
					entries, e := os.ReadDir(stablePath)
					if e != nil || len(entries) != 1 {
						t.Fatal("partial component adopted", e)
					}
					m, e := readStableMarker(filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json"))
					if e != nil || m.StableInstalls[0].Phase != "prepared" {
						t.Fatal("failed partial install promoted", e)
					}
					stableInstallAssertRetained(t, childDir, before, false)
					// Delete only this test's injected invalid files before the
					// separately asserted authorized absent-component resume.
					if e = os.Remove(partial); e != nil {
						t.Fatal(e)
					}
					if e = os.Remove(stablePath); e != nil {
						t.Fatal(e)
					}
				}
			}
			c, err := NewStable(cfg)
			if err != nil {
				t.Fatal("durable checkpoint failed reopening", err)
			}
			t.Cleanup(func() { _ = c.Close() })
			if err = c.LoginFixture("zero256"); err != nil {
				_ = c.Close()
				t.Fatal(err)
			}
			stableInstallWaitLogin(t, c.ManifestController, true)
			current := c.Snapshot()
			if current.Attempts != 0 || current.AcceptedCount != 0 || current.Height != 0 || current.State != "stopped" {
				_ = c.Close()
				t.Fatalf("installation mined or issued: %+v", current)
			}
			if current.GymID != before.Snapshot.GymID || current.GymPublicKey != before.Snapshot.GymPublicKey || current.PayoutAddress != before.Snapshot.PayoutAddress {
				_ = c.Close()
				t.Fatal("original wallet identity replaced")
			}
			nowMarker, err := readStableMarker(filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json"))
			if err != nil {
				_ = c.Close()
				t.Fatal(err)
			}
			if len(nowMarker.StableInstalls) != 1 || nowMarker.StableInstalls[0].Phase != "ready" || nowMarker.StableInstalls[0].Pin != entry.Pin {
				_ = c.Close()
				t.Fatal("resume changed durable binding")
			}
			if err = c.Close(); err != nil {
				t.Fatal(err)
			}
			stableInstallAssertRetained(t, childDir, before, false)

			// An established ready component is never silently recreated.
			if err = os.Rename(stablePath, stablePath+"-retained"); err != nil {
				t.Fatal(err)
			}
			if got, err := NewStable(cfg); err == nil || got != nil {
				if got != nil {
					_ = got.Close()
				}
				t.Fatal("ready missing stable silently recreated")
			}
			if _, err = os.Lstat(stablePath); !os.IsNotExist(err) {
				t.Fatal("ready refusal created absent component")
			}
			stableInstallAssertRetained(t, childDir, before, false)
			if err = os.Rename(stablePath+"-retained", stablePath); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func stableInstallCrashChild(t *testing.T, input string) {
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var in stableInstallCrashInput
	if json.Unmarshal(raw, &in) != nil {
		t.Fatal("input")
	}
	var c *ManifestController
	if in.Stage == "manifest-child-created" {
		c, err = NewManifest(pilot.Config{DataDir: in.Root, NodeBinary: in.Node})
	} else {
		var stable *StableController
		stable, err = NewStable(pilot.Config{DataDir: in.Root, NodeBinary: in.Node})
		if stable != nil {
			c = stable.ManifestController
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.checkpoint = func(stage string) {
		if stage != in.Stage {
			return
		}
		c.mu.Lock()
		child := c.child
		c.mu.Unlock()
		snap := child.Snapshot()
		if in.Stage == "manifest-child-created" {
			raw, e := os.ReadFile(filepath.Join(in.Root, "gyms", child.AccountID(), "SLUG-MANIFEST-PILOT.json"))
			if e != nil {
				t.Error(e)
				return
			}
			evidence := stableInstallEvidence{Snapshot: snap, ChildMarker: raw}
			b, e := json.Marshal(evidence)
			if e != nil {
				t.Error(e)
				return
			}
			stableInstallSignal(t, in.Signal, b)
			select {}
		}
		if snap.Attempts != 0 || snap.AcceptedCount != 0 || snap.Height != 0 || snap.State != "stopped" {
			t.Error("non-idle fixture at install")
			return
		}
		marker, e := readStableMarker(filepath.Join(in.Root, "SLUG-MANIFEST-ACCOUNTS.json"))
		if e != nil {
			t.Error(e)
			return
		}
		evidence := stableInstallCapture(t, filepath.Join(in.Root, "gyms", marker.StableInstalls[0].AccountID))
		evidence.Snapshot, evidence.Marker = snap, marker
		b, e := json.Marshal(evidence)
		if e != nil {
			t.Error(e)
			return
		}
		stableInstallSignal(t, in.Signal, b)
		select {}
	}
	if err = c.LoginFixture("zero256"); err != nil {
		t.Fatal(err)
	}
	stableInstallWaitLogin(t, c, true)
	t.Fatal("login completed without crash barrier")
}
func stableInstallWaitLogin(t *testing.T, c *ManifestController, want bool) {
	t.Helper()
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		state, busy := c.state, c.operation
		c.mu.Unlock()
		if !busy {
			if (state == "signed_in") != want {
				t.Fatalf("login state %s; want success %v", state, want)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("login deadline")
}
func stableInstallRows(t *testing.T, path string) map[string][]byte {
	t.Helper()
	db, e := leveldb.OpenFile(path, &opt.Options{ReadOnly: true})
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	rows := map[string][]byte{}
	it := db.NewIterator(nil, nil)
	for it.Next() {
		rows[string(it.Key())] = append([]byte{}, it.Value()...)
	}
	e = it.Error()
	it.Release()
	if e != nil {
		t.Fatal(e)
	}
	return rows
}
func stableInstallCapture(t *testing.T, path string) stableInstallEvidence {
	t.Helper()
	var e stableInstallEvidence
	var err error
	e.ChildMarker, err = os.ReadFile(filepath.Join(path, "SLUG-MANIFEST-PILOT.json"))
	if err != nil {
		t.Fatal(err)
	}
	wallet, err := os.ReadFile(filepath.Join(path, "wallet", "slugwallet.db"))
	if err != nil {
		t.Fatal(err)
	}
	e.WalletHash = sha256.Sum256(wallet)
	e.Source = stableInstallRows(t, filepath.Join(path, "manifest-authority", "db"))
	e.Outbox = stableInstallRows(t, filepath.Join(path, "manifest-outbox", "db"))
	return e
}
func stableInstallAssertRetained(t *testing.T, path string, before stableInstallEvidence, exactWallet bool) {
	t.Helper()
	after := stableInstallCapture(t, path)
	if !bytes.Equal(before.ChildMarker, after.ChildMarker) || !reflect.DeepEqual(before.Source, after.Source) || !reflect.DeepEqual(before.Outbox, after.Outbox) {
		t.Fatal("install/reopen rewrote existing child identity, receipts or outbox")
	}
	// Reopening the wallet may commit ordinary synchronization metadata. The
	// crash boundary itself must preserve exact bytes; resumed identity/payout
	// are checked above, and source/outbox bytes remain exact throughout.
	if exactWallet && before.WalletHash != after.WalletHash {
		t.Fatal("crash changed wallet bytes")
	}
}

func stableInstallSignal(t *testing.T, path string, b []byte) {
	t.Helper()
	f, e := os.OpenFile(path+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e == nil {
		e = os.Rename(path+".tmp", path)
	}
	if e != nil {
		t.Fatal(e)
	}
}

func TestStableInstallLegacyPreparationCrash(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal fixture identity required")
	}
	node := os.Getenv("SLUG_STABLE_NODE")
	if node == "" {
		t.Skip("set SLUG_STABLE_NODE")
	}
	root := filepath.Join(t.TempDir(), "accounts")
	in := stableInstallCrashInput{root, node, "manifest-child-created", filepath.Join(filepath.Dir(root), "checkpoint.json")}
	raw, _ := json.Marshal(in)
	input := filepath.Join(filepath.Dir(root), "input.json")
	if e := os.WriteFile(input, raw, 0600); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStableInstallProcessCrash$", "-test.timeout=180s", "-test.v")
	cmd.Env = append(os.Environ(), "SLUG_STABLE_INSTALL_CRASH_INPUT="+input)
	log, e := os.Create(filepath.Join(filepath.Dir(root), "child.log"))
	if e != nil {
		t.Fatal(e)
	}
	defer log.Close()
	cmd.Stdout, cmd.Stderr = log, log
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	joined := false
	defer func() {
		if !joined {
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	deadline := time.NewTimer(150 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
wait:
	for {
		if _, e = os.Stat(in.Signal); e == nil {
			break wait
		}
		select {
		case e = <-done:
			joined = true
			output, _ := os.ReadFile(log.Name())
			t.Fatalf("legacy child exited: %v %s", e, output)
		case <-deadline.C:
			t.Fatal("legacy checkpoint timeout")
		case <-tick.C:
		}
	}
	if e = cmd.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	e = <-done
	joined = true
	var exit *exec.ExitError
	if !errors.As(e, &exit) {
		t.Fatalf("not killed: %v", e)
	}
	ws, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
		t.Fatal("not SIGKILL")
	}
	old, e := readManifestAccountMarker(filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json"))
	if e != nil || old.Version != 1 {
		t.Fatal("not retained v1", e)
	}
	gyms, e := os.ReadDir(filepath.Join(root, "gyms"))
	if e != nil || len(gyms) != 1 {
		t.Fatal("one pre-Prepare child required", e)
	}
	child := filepath.Join(root, "gyms", gyms[0].Name())
	childBefore, e := os.ReadFile(filepath.Join(child, "SLUG-MANIFEST-PILOT.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m struct {
		Phase              string `json:"phase"`
		AccountID          string `json:"account_id"`
		AccountAuthorityID string `json:"account_authority_id"`
	}
	if json.Unmarshal(childBefore, &m) != nil || m.Phase != "new" || m.AccountID != gyms[0].Name() || m.AccountAuthorityID != old.AccountAuthorityID {
		t.Fatal("unexpected pre-Prepare identity")
	}
	for _, name := range []string{"wallet", "manifest-authority", "manifest-outbox"} {
		if _, e = os.Lstat(filepath.Join(child, name)); !os.IsNotExist(e) {
			t.Fatalf("pre-Prepare created %s", name)
		}
	}

	for _, name := range []string{"primary-chain", "peer-chain"} {
		entries, e := os.ReadDir(filepath.Join(child, name))
		if e != nil || len(entries) != 0 {
			t.Fatalf("pre-Prepare chain is not empty: %s %v", name, e)
		}
	}
	c, e := NewStable(pilot.Config{DataDir: root, NodeBinary: node})
	if e != nil {
		t.Fatal("v1 pre-Prepare upgrade", e)
	}
	defer c.Close()
	after, e := os.ReadFile(filepath.Join(child, "SLUG-MANIFEST-PILOT.json"))
	if e != nil || !bytes.Equal(after, childBefore) {
		t.Fatal("upgrade mutated child intent", e)
	}
	upgraded, e := readStableMarker(filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json"))
	if e != nil || upgraded.AccountAuthorityID != old.AccountAuthorityID || len(upgraded.StableInstalls) != 0 {
		t.Fatal("upgrade changed account authority", e)
	}
	if e = c.LoginFixture("zero256"); e != nil {
		t.Fatal(e)
	}
	stableInstallWaitLogin(t, c.ManifestController, true)
	snap := c.Snapshot()
	if snap.Attempts != 0 || snap.AcceptedCount != 0 || snap.Height != 0 || snap.State != "stopped" || snap.View == nil || snap.View.AccountID != m.AccountID {
		t.Fatalf("resumed login replaced account or mined: %+v", snap)
	}
	ready, e := readStableMarker(filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json"))
	if e != nil || len(ready.StableInstalls) != 1 || ready.StableInstalls[0].Phase != "ready" || ready.StableInstalls[0].AccountID != m.AccountID {
		t.Fatal("upgrade did not complete original account", e)
	}
}
