package accountpilot

import (
	"bytes"
	"context"
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

type seedInstallCrashInput struct{ Root, Node, Stage, Signal string }
type seedInstallCrashEvidence struct {
	Marker      seedAccountMarker
	Snapshot    pilot.ManifestSnapshot
	Retained    stableInstallEvidence
	CreatedAtMS int64
}

// Opt-in real SIGKILL with two published fixture wallets, a frozen normal node,
// no mining, and only owned disposable data. Old evidence and root files survive.
func TestSeedInstallProcessCrash(t *testing.T) {
	if input := os.Getenv("SLUG_SEED_INSTALL_CRASH_INPUT"); input != "" {
		seedInstallCrashChild(t, input)
		return
	}
	if wire.TestPoW {
		t.Skip("normal profile")
	}
	node := os.Getenv("SLUG_STABLE_NODE")
	if node == "" {
		t.Skip("explicit frozen normal node required")
	}
	if !filepath.IsAbs(node) {
		t.Fatal("absolute node")
	}
	for _, stage := range []string{"seed-prepared", "seed-created", "seed-ready"} {
		t.Run(stage, func(t *testing.T) {
			base, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			root := filepath.Join(base, "accounts")
			cfg := pilot.Config{DataDir: root, NodeBinary: node}
			first, e := NewSeeds(cfg)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = first.Close() })
			if e = seedInstallLogin(t, first.ManifestController, "zero256"); e != nil {
				first.Close()
				t.Fatal(e)
			}
			stableInstallWaitLogin(t, first.ManifestController, true)
			if first.SeedSnapshot().State != "setup_incomplete" {
				first.Close()
				t.Fatal("first fixture invented source")
			}
			if e = first.Close(); e != nil {
				t.Fatal(e)
			}
			markerPath := filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json")
			m, e := readSeedMarker(markerPath)
			if e != nil || len(m.EntryInstalls) != 1 {
				t.Fatal(e)
			}
			firstInstall := m.EntryInstalls[0]
			firstDir := filepath.Join(root, "gyms", firstInstall.AccountID)
			retained := stableInstallCapture(t, firstDir)
			stableRows := stableInstallRows(t, filepath.Join(firstDir, "stable", "db"))
			entryRows := stableInstallRows(t, filepath.Join(firstDir, "entries", "db"))
			in := seedInstallCrashInput{root, node, stage, filepath.Join(base, "checkpoint.json")}
			input := filepath.Join(base, "input.json")
			b, _ := json.Marshal(in)
			if e = os.WriteFile(input, b, 0600); e != nil {
				t.Fatal(e)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestSeedInstallProcessCrash$", "-test.timeout=180s", "-test.v")
			cmd.Env = append(os.Environ(), "SLUG_SEED_INSTALL_CRASH_INPUT="+input)
			log, e := os.Create(filepath.Join(base, "child.log"))
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
			deadline := time.Now().Add(150 * time.Second)
			for {
				if _, e = os.Stat(in.Signal); e == nil {
					break
				}
				select {
				case e = <-done:
					joined = true
					raw, _ := os.ReadFile(log.Name())
					t.Fatalf("child exited before checkpoint %v\n%s", e, raw)
				case <-time.After(10 * time.Millisecond):
				}
				if time.Now().After(deadline) {
					t.Fatal("checkpoint timeout")
				}
			}
			if e = cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			e = <-done
			joined = true
			var exit *exec.ExitError
			if !errors.As(e, &exit) {
				t.Fatal("not killed", e)
			}
			ws, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
				t.Fatal("not SIGKILL")
			}
			var killed seedInstallCrashEvidence
			b, e = os.ReadFile(in.Signal)
			if e != nil || json.Unmarshal(b, &killed) != nil {
				t.Fatal(e)
			}
			want := "prepared"
			if stage == "seed-ready" {
				want = "ready"
			}
			if killed.Marker.SeedPhase != want || killed.Marker.EventPhase != "ready" || len(killed.Marker.EntryInstalls) != 2 {
				t.Fatal("wrong durable phase")
			}
			for _, x := range killed.Marker.EntryInstalls {
				if x.Phase != "ready" {
					t.Fatal("non-atomic second participant ready")
				}
			}
			var second entryInstall
			for _, x := range killed.Marker.EntryInstalls {
				if x.AccountID != firstInstall.AccountID {
					second = x
				}
			}
			secondDir := filepath.Join(root, "gyms", second.AccountID)
			stableInstallAssertRetained(t, secondDir, killed.Retained, true)
			assertFirst := func() {
				t.Helper()
				stableInstallAssertRetained(t, firstDir, retained, false)
				if !reflect.DeepEqual(stableRows, stableInstallRows(t, filepath.Join(firstDir, "stable", "db"))) || !reflect.DeepEqual(entryRows, stableInstallRows(t, filepath.Join(firstDir, "entries", "db"))) {
					t.Fatal("first histories changed")
				}
				m, e := readSeedMarker(markerPath)
				if e != nil {
					t.Fatal(e)
				}
				found := false
				for _, x := range m.EntryInstalls {
					if x == firstInstall {
						found = true
					}
				}
				if !found {
					t.Fatal("first pin replaced")
				}
			}
			assertFirst()
			prior, _ := os.ReadFile(markerPath)
			eventRows := stableInstallRows(t, filepath.Join(root, "events", "db"))
			for _, open := range []func() error{func() error {
				x, e := NewEvents(cfg)
				if x != nil {
					x.Close()
				}
				return e
			}, func() error {
				x, e := NewManifest(cfg)
				if x != nil {
					x.Close()
				}
				return e
			}, func() error {
				x, e := NewStable(cfg)
				if x != nil {
					x.Close()
				}
				return e
			}, func() error {
				x, e := NewEntries(cfg)
				if x != nil {
					x.Close()
				}
				return e
			}} {
				if open() == nil {
					t.Fatal("old route accepted v5")
				}
				now, _ := os.ReadFile(markerPath)
				if !bytes.Equal(now, prior) {
					t.Fatal("old refusal mutated root")
				}
			}
			seedPath := filepath.Join(root, "seeds")
			if stage != "seed-prepared" {
				rows := stableInstallRows(t, filepath.Join(seedPath, "db"))
				var meta struct {
					CreatedAtMS int64 `json:"created_at_ms"`
				}
				if json.Unmarshal(rows["meta"], &meta) != nil || meta.CreatedAtMS == 0 {
					t.Fatal("missing killed initialized metadata")
				}
				killed.CreatedAtMS = meta.CreatedAtMS
			}
			if stage == "seed-prepared" {
				if _, e = os.Lstat(seedPath); !os.IsNotExist(e) {
					t.Fatal("early seed create")
				}
				if e = os.Mkdir(seedPath, 0700); e != nil {
					t.Fatal(e)
				}
				partial := filepath.Join(seedPath, "unknown")
				kept := []byte("retained partial evidence")
				_ = os.WriteFile(partial, kept, 0600)
				bad, e := NewSeeds(cfg)
				if bad != nil {
					bad.Close()
				}
				if e == nil || bad != nil {
					t.Fatal("unknown partial adopted")
				}
				got, _ := os.ReadFile(partial)
				if !bytes.Equal(got, kept) {
					t.Fatal("partial changed")
				}
				if e = os.Remove(partial); e != nil {
					t.Fatal(e)
				}
			}
			resumed, e := NewSeeds(cfg)
			if e != nil {
				t.Fatal("resume", e)
			}
			t.Cleanup(func() { _ = resumed.Close() })
			info, e := resumed.seedOwner.store.Info(t.Context())
			if e != nil || info.EventCount != 0 || info.StoreID != killed.Marker.SeedStoreID {
				resumed.Close()
				t.Fatal("resume pin", e)
			}
			if killed.CreatedAtMS > 0 && info.CreatedAtMS != killed.CreatedAtMS {
				resumed.Close()
				t.Fatal("creation time replaced")
			}
			if e = seedInstallLogin(t, resumed.ManifestController, "incrementing256"); e != nil {
				resumed.Close()
				t.Fatal(e)
			}
			stableInstallWaitLogin(t, resumed.ManifestController, true)
			snap := resumed.Snapshot()
			if snap.Attempts != 0 || snap.AcceptedCount != 0 || snap.Height != 0 || snap.State != "stopped" {
				resumed.Close()
				t.Fatal("installation mined")
			}
			if resumed.SeedSnapshot().State != "ready" {
				resumed.Close()
				t.Fatal("seed attach")
			}
			if e = resumed.Close(); e != nil {
				t.Fatal(e)
			}
			assertFirst()
			if !reflect.DeepEqual(eventRows, stableInstallRows(t, filepath.Join(root, "events", "db"))) {
				t.Fatal("seed installation changed existing event rows")
			}
			if e = os.Rename(seedPath, seedPath+"-retained"); e != nil {
				t.Fatal(e)
			}
			bad, e := NewSeeds(cfg)
			if bad != nil {
				bad.Close()
			}
			if e == nil || bad != nil {
				t.Fatal("missing ready recreated")
			}
			if _, e = os.Lstat(seedPath); !os.IsNotExist(e) {
				t.Fatal("missing refusal wrote path")
			}
			if e = os.Rename(seedPath+"-retained", seedPath); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func seedInstallCrashChild(t *testing.T, input string) {
	b, e := os.ReadFile(input)
	if e != nil {
		t.Fatal(e)
	}
	var in seedInstallCrashInput
	if json.Unmarshal(b, &in) != nil {
		t.Fatal("input")
	}
	c, e := NewSeeds(pilot.Config{DataDir: in.Root, NodeBinary: in.Node})
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
		m, e := readSeedMarker(filepath.Join(in.Root, "SLUG-MANIFEST-ACCOUNTS.json"))
		if e != nil {
			t.Error(e)
			return
		}
		snap := child.Snapshot()
		if snap.Attempts != 0 || snap.AcceptedCount != 0 || snap.Height != 0 {
			t.Error("unexpected work")
			return
		}
		proof := seedInstallCrashEvidence{Marker: m, Snapshot: snap, Retained: stableInstallCapture(t, filepath.Join(in.Root, "gyms", child.AccountID()))}
		b, e := json.Marshal(proof)
		if e != nil {
			t.Error(e)
			return
		}
		stableInstallSignal(t, in.Signal, b)
		select {}
	}
	if e = seedInstallLogin(t, c.ManifestController, "incrementing256"); e != nil {
		t.Fatal(e)
	}
	stableInstallWaitLogin(t, c.ManifestController, true)
	t.Fatal("missed crash checkpoint")
}

// Exercise the same begin/login path as LoginFixture while retaining a useful
// sanitized failure cause in this owned fixture test. No alternate authority.
func seedInstallLogin(t *testing.T, c *ManifestController, fixture string) error {
	t.Helper()
	return c.begin("signing_in", func(ctx context.Context) error {
		e := c.login(ctx, fixture)
		if e != nil {
			t.Logf("fixture login failure (%s): %v", fixture, e)
		}
		return e
	})
}
