// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
)

type entryInstallCrashInput struct{ Root, Node, Stage, Signal string }
type entryInstallCrashEvidence struct {
	Snapshot  pilot.ManifestSnapshot
	Marker    entryAccountMarker
	AccountID string
	Retained  stableInstallEvidence
	Stable    map[string][]byte
}

// This opt-in test uses only an immutable normal node and published fixtures at
// genesis. It never starts mining or adopts user-owned data. Each kill occurs
// after a real synced parent/component boundary, not after a simulated Close.
func TestEntryInstallProcessCrash(t *testing.T) {
	if input := os.Getenv("SLUG_ENTRY_INSTALL_CRASH_INPUT"); input != "" {
		entryInstallCrashChild(t, input)
		return
	}
	if wire.TestPoW {
		t.Skip("normal fixture identity required")
	}
	node := os.Getenv("SLUG_STABLE_NODE")
	if node == "" {
		t.Skip("set SLUG_STABLE_NODE to an immutable normal-PoW slugd")
	}
	if !filepath.IsAbs(node) {
		t.Fatal("absolute immutable node path required")
	}
	for _, stage := range []string{"entry-prepared", "entry-created", "entry-ready"} {
		t.Run(stage, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "accounts")
			cfg := pilot.Config{DataDir: root, NodeBinary: node}
			// Complete the first gym before crashing the second fixture's first login.
			// This exercises the v3-preserving stable writers as well as entry writers.
			first, e := NewEntries(cfg)
			if e != nil {
				t.Fatal(e)
			}
			if e = first.LoginFixture("zero256"); e != nil {
				first.Close()
				t.Fatal(e)
			}
			stableInstallWaitLogin(t, first.ManifestController, true)
			firstSnapshot := first.Snapshot()
			if e = first.Close(); e != nil {
				t.Fatal(e)
			}
			markerPath := filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json")
			before, e := readEntryMarker(markerPath)
			if e != nil || len(before.EntryInstalls) != 1 || before.EntryInstalls[0].Phase != "ready" {
				t.Fatal("first gym not ready", e)
			}
			firstInstall := before.EntryInstalls[0]
			firstDir := filepath.Join(root, "gyms", firstInstall.AccountID)
			firstEvidence := stableInstallCapture(t, firstDir)
			firstStable := stableInstallRows(t, filepath.Join(firstDir, "stable", "db"))
			firstEntries := stableInstallRows(t, filepath.Join(firstDir, "entries", "db"))
			in := entryInstallCrashInput{root, node, stage, filepath.Join(filepath.Dir(root), "checkpoint.json")}
			input := filepath.Join(filepath.Dir(root), "input.json")
			raw, _ := json.Marshal(in)
			if e = os.WriteFile(input, raw, 0600); e != nil {
				t.Fatal(e)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestEntryInstallProcessCrash$", "-test.timeout=180s", "-test.v")
			cmd.Env = append(os.Environ(), "SLUG_ENTRY_INSTALL_CRASH_INPUT="+input)
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
					cmd.Process.Kill()
					<-done
				}
			}()
			deadline := time.NewTimer(150 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			for {
				if _, e = os.Stat(in.Signal); e == nil {
					break
				}
				select {
				case e = <-done:
					joined = true
					b, _ := os.ReadFile(log.Name())
					t.Fatalf("child exited before checkpoint: %v\n%s", e, b)
				case <-deadline.C:
					t.Fatal("checkpoint timeout")
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
				t.Fatal("not a killed helper", e)
			}
			ws, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
				t.Fatal("not actual SIGKILL", e)
			}
			raw, e = os.ReadFile(in.Signal)
			if e != nil {
				t.Fatal(e)
			}
			var killed entryInstallCrashEvidence
			if e = json.Unmarshal(raw, &killed); e != nil {
				t.Fatal(e)
			}
			if killed.AccountID == firstInstall.AccountID || len(killed.Marker.EntryInstalls) != 2 || len(killed.Marker.StableInstalls) != 2 {
				t.Fatal("second fixture isolation")
			}
			second, ok := entryCrashFind(killed.Marker, killed.AccountID)
			if !ok {
				t.Fatal("missing second install")
			}
			want := "prepared"
			if stage == "entry-ready" {
				want = "ready"
			}
			if second.Phase != want {
				t.Fatal("wrong durable phase", second.Phase, want)
			}
			childDir := filepath.Join(root, "gyms", killed.AccountID)
			entryPath := filepath.Join(childDir, "entries")
			if stage == "entry-prepared" {
				if _, e = os.Lstat(entryPath); !os.IsNotExist(e) {
					t.Fatal("prepared component unexpectedly exists")
				}
			} else if _, e = os.Stat(entryPath); e != nil {
				t.Fatal(e)
			}
			stableInstallAssertRetained(t, childDir, killed.Retained, true)
			if !reflect.DeepEqual(stableInstallRows(t, filepath.Join(childDir, "stable", "db")), killed.Stable) {
				t.Fatal("crash rewrote stable rows")
			}
			assertFirst := func() {
				t.Helper()
				stableInstallAssertRetained(t, firstDir, firstEvidence, false)
				if !reflect.DeepEqual(firstStable, stableInstallRows(t, filepath.Join(firstDir, "stable", "db"))) || !reflect.DeepEqual(firstEntries, stableInstallRows(t, filepath.Join(firstDir, "entries", "db"))) {
					t.Fatal("second fixture changed first gym history")
				}
				m, e := readEntryMarker(markerPath)
				if e != nil {
					t.Fatal(e)
				}
				v, ok := entryCrashFind(m, firstInstall.AccountID)
				if !ok || v != firstInstall {
					t.Fatal("first install pin/phase changed")
				}
			}
			assertFirst()
			markerRaw, _ := os.ReadFile(markerPath)
			for _, oldRoute := range []string{"manifest", "stable"} {
				if oldRoute == "manifest" {
					c, e := NewManifest(cfg)
					if c != nil {
						c.Close()
					}
					if e == nil || c != nil {
						t.Fatal("manifest route accepted v3")
					}
				} else {
					c, e := NewStable(cfg)
					if c != nil {
						c.Close()
					}
					if e == nil || c != nil {
						t.Fatal("stable route accepted v3")
					}
				}
				after, _ := os.ReadFile(markerPath)
				if !bytes.Equal(markerRaw, after) {
					t.Fatal("old refusal rewrote v3")
				}
			}
			if stage == "entry-prepared" {
				// Retain and refuse explicitly unknown/partial contents; delete only this
				// test's injected file to separately test the supported empty-directory path.
				if e = os.Mkdir(entryPath, 0700); e != nil {
					t.Fatal(e)
				}
				partial := filepath.Join(entryPath, "SLUG-CANDIDATE-ENTRIES.json")
				kept := []byte("partial fixture evidence\n")
				if e = os.WriteFile(partial, kept, 0600); e != nil {
					t.Fatal(e)
				}
				rejected, e := NewEntries(cfg)
				if e == nil {
					if e = rejected.LoginFixture("incrementing256"); e != nil {
						rejected.Close()
						t.Fatal(e)
					}
					stableInstallWaitLogin(t, rejected.ManifestController, false)
					rejected.Close()
				} else if rejected != nil {
					rejected.Close()
					t.Fatal("error returned live controller")
				}
				got, e := os.ReadFile(partial)
				if e != nil || !bytes.Equal(got, kept) {
					t.Fatal("partial evidence changed", e)
				}
				entries, e := os.ReadDir(entryPath)
				if e != nil || len(entries) != 1 {
					t.Fatal("partial component adopted", e)
				}
				m, e := readEntryMarker(markerPath)
				v, ok := entryCrashFind(m, killed.AccountID)
				if e != nil || !ok || v.Phase != "prepared" {
					t.Fatal("partial install promoted")
				}
				if e = os.Remove(partial); e != nil {
					t.Fatal(e)
				}
			}
			resumed, e := NewEntries(cfg)
			if e != nil {
				t.Fatal("resume constructor", e)
			}
			if e = resumed.LoginFixture("incrementing256"); e != nil {
				resumed.Close()
				t.Fatal(e)
			}
			stableInstallWaitLogin(t, resumed.ManifestController, true)
			snap := resumed.Snapshot()
			if snap.Attempts != 0 || snap.AcceptedCount != 0 || snap.Height != 0 || snap.State != "stopped" {
				resumed.Close()
				t.Fatal("installation mined/issued")
			}
			if snap.GymID != killed.Snapshot.GymID || snap.GymPublicKey != killed.Snapshot.GymPublicKey || snap.PayoutAddress != killed.Snapshot.PayoutAddress || snap.GymID == firstSnapshot.GymID {
				resumed.Close()
				t.Fatal("second wallet identity changed")
			}
			if e = resumed.Close(); e != nil {
				t.Fatal(e)
			}
			after, e := readEntryMarker(markerPath)
			v, ok := entryCrashFind(after, killed.AccountID)
			if e != nil || !ok || v.Phase != "ready" || v.Pin != second.Pin {
				t.Fatal("ready binding changed", e)
			}
			assertFirst()
			stableInstallAssertRetained(t, childDir, killed.Retained, false)
			if e = os.Rename(entryPath, entryPath+"-retained"); e != nil {
				t.Fatal(e)
			}
			bad, e := NewEntries(cfg)
			if bad != nil {
				bad.Close()
			}
			if e == nil || bad != nil {
				t.Fatal("ready missing entry recreated")
			}
			if _, e = os.Lstat(entryPath); !os.IsNotExist(e) {
				t.Fatal("ready refusal created path")
			}
			if e = os.Rename(entryPath+"-retained", entryPath); e != nil {
				t.Fatal(e)
			}
			assertFirst()
		})
	}
}
func entryCrashFind(m entryAccountMarker, id string) (entryInstall, bool) {
	for _, v := range m.EntryInstalls {
		if v.AccountID == id {
			return v, true
		}
	}
	return entryInstall{}, false
}
func entryInstallCrashChild(t *testing.T, input string) {
	raw, e := os.ReadFile(input)
	if e != nil {
		t.Fatal(e)
	}
	var in entryInstallCrashInput
	if json.Unmarshal(raw, &in) != nil {
		t.Fatal("input")
	}
	c, e := NewEntries(pilot.Config{DataDir: in.Root, NodeBinary: in.Node})
	if e != nil {
		t.Fatal(e)
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
		if snap.Attempts != 0 || snap.AcceptedCount != 0 || snap.Height != 0 || snap.State != "stopped" {
			t.Error("non-idle fixture at checkpoint")
			return
		}
		m, e := readEntryMarker(filepath.Join(in.Root, "SLUG-MANIFEST-ACCOUNTS.json"))
		if e != nil {
			t.Error(e)
			return
		}
		dir := filepath.Join(in.Root, "gyms", child.AccountID())
		evidence := entryInstallCrashEvidence{Snapshot: snap, Marker: m, AccountID: child.AccountID(), Retained: stableInstallCapture(t, dir), Stable: stableInstallRows(t, filepath.Join(dir, "stable", "db"))}
		b, e := json.Marshal(evidence)
		if e != nil {
			t.Error(e)
			return
		}
		stableInstallSignal(t, in.Signal, b)
		select {}
	}
	if e = c.LoginFixture("incrementing256"); e != nil {
		t.Fatal(e)
	}
	stableInstallWaitLogin(t, c.ManifestController, true)
	t.Fatal("login finished without checkpoint")
}
