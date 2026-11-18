// Copyright (c) 2026 The SLUG developers. ISC license.
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

type boutInstallCrashInput struct{ Root, Node, Stage, Signal string }
type boutInstallCrashEvidence struct {
	Marker      boutAccountMarker
	Snapshot    pilot.ManifestSnapshot
	Retained    stableInstallEvidence
	CreatedAtMS int64
}

// Opt-in real SIGKILL with two published fixture wallets, a frozen normal node,
// no mining, and only owned disposable data. Old evidence and root files survive.
func TestBoutInstallProcessCrash(t *testing.T) {
	if input := os.Getenv("SLUG_BOUT_INSTALL_CRASH_INPUT"); input != "" {
		boutInstallCrashChild(t, input)
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
	for _, stage := range []string{"bout-prepared", "bout-created", "bout-ready"} {
		t.Run(stage, func(t *testing.T) {
			base, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			root := filepath.Join(base, "accounts")
			cfg := pilot.Config{DataDir: root, NodeBinary: node}
			first, e := NewBouts(context.Background(), cfg)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = first.Close() })
			if e = seedInstallLogin(t, first.ManifestController, "zero256"); e != nil {
				first.Close()
				t.Fatal(e)
			}
			stableInstallWaitLogin(t, first.ManifestController, true)
			if first.BoutSnapshot().State != "setup_incomplete" {
				first.Close()
				t.Fatal("first fixture invented source")
			}
			if e = first.Close(); e != nil {
				t.Fatal(e)
			}
			markerPath := filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json")
			m, e := readBoutMarker(markerPath)
			if e != nil || len(m.EntryInstalls) != 1 {
				t.Fatal(e)
			}
			firstInstall := m.EntryInstalls[0]
			firstDir := filepath.Join(root, "gyms", firstInstall.AccountID)
			retained := stableInstallCapture(t, firstDir)
			stableRows := stableInstallRows(t, filepath.Join(firstDir, "stable", "db"))
			entryRows := stableInstallRows(t, filepath.Join(firstDir, "entries", "db"))
			in := boutInstallCrashInput{root, node, stage, filepath.Join(base, "checkpoint.json")}
			input := filepath.Join(base, "input.json")
			b, _ := json.Marshal(in)
			if e = os.WriteFile(input, b, 0600); e != nil {
				t.Fatal(e)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestBoutInstallProcessCrash$", "-test.timeout=180s", "-test.v")
			cmd.Env = append(os.Environ(), "SLUG_BOUT_INSTALL_CRASH_INPUT="+input)
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
			var killed boutInstallCrashEvidence
			b, e = os.ReadFile(in.Signal)
			if e != nil || json.Unmarshal(b, &killed) != nil {
				t.Fatal(e)
			}
			want := "prepared"
			if stage == "bout-ready" {
				want = "ready"
			}
			if killed.Marker.BoutPhase != want || killed.Marker.CardPhase != "ready" || killed.Marker.SeedPhase != "ready" || killed.Marker.EventPhase != "ready" || len(killed.Marker.EntryInstalls) != 2 {
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
			secondStableRows := stableInstallRows(t, filepath.Join(secondDir, "stable", "db"))
			secondEntryRows := stableInstallRows(t, filepath.Join(secondDir, "entries", "db"))
			stableInstallAssertRetained(t, secondDir, killed.Retained, true)
			assertFirst := func() {
				t.Helper()
				stableInstallAssertRetained(t, firstDir, retained, false)
				if !reflect.DeepEqual(stableRows, stableInstallRows(t, filepath.Join(firstDir, "stable", "db"))) || !reflect.DeepEqual(entryRows, stableInstallRows(t, filepath.Join(firstDir, "entries", "db"))) {
					t.Fatal("first histories changed")
				}
				m, e := readBoutMarker(markerPath)
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
			seedRows := stableInstallRows(t, filepath.Join(root, "seeds", "db"))
			cardRows := stableInstallRows(t, filepath.Join(root, "cards", "db"))
			for _, open := range []func() error{func() error {
				x, e := NewCards(cfg)
				if x != nil {
					x.Close()
				}
				return e
			}, func() error {
				x, e := NewSeeds(cfg)
				if x != nil {
					x.Close()
				}
				return e
			}, func() error {
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
					t.Fatal("old route accepted v7")
				}
				now, _ := os.ReadFile(markerPath)
				if !bytes.Equal(now, prior) {
					t.Fatal("old refusal mutated root")
				}
			}
			boutPath := filepath.Join(root, "bouts")
			if stage != "bout-prepared" {
				rows := stableInstallRows(t, filepath.Join(boutPath, "db"))
				var meta struct {
					CreatedAtMS int64 `json:"created_at_ms"`
				}
				if json.Unmarshal(rows["meta"], &meta) != nil || meta.CreatedAtMS == 0 {
					t.Fatal("missing killed initialized bout metadata")
				}
				killed.CreatedAtMS = meta.CreatedAtMS
			}
			if stage == "bout-prepared" {
				if _, e = os.Lstat(boutPath); !os.IsNotExist(e) {
					t.Fatal("early bout create")
				}
				if e = os.Mkdir(boutPath, 0700); e != nil {
					t.Fatal(e)
				}
				partial := filepath.Join(boutPath, "unknown")
				kept := []byte("retained partial evidence")
				_ = os.WriteFile(partial, kept, 0600)
				bad, e := NewBouts(context.Background(), cfg)
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
			resumed, e := NewBouts(context.Background(), cfg)
			if e != nil {
				t.Fatal("resume", e)
			}
			t.Cleanup(func() { _ = resumed.Close() })
			info, e := resumed.boutOwner.store.Info(t.Context())
			if e != nil || info.BoutCount != 0 || info.StoreID != killed.Marker.BoutStoreID {
				resumed.Close()
				t.Fatal("resume pin", e)
			}
			cardInfo, se := resumed.cardOwner.store.Info(t.Context())
			if se != nil || info.SourceCreatedAtMS != cardInfo.CreatedAtMS {
				t.Fatal("source creation binding", se)
			}
			if killed.CreatedAtMS > 0 && info.CreatedAtMS != killed.CreatedAtMS {
				resumed.Close()
				t.Fatal("creation time replaced")
			}
			// Resume installation alone must not mutate either participant's histories.
			// Close before inspecting LevelDB; then separately exercise authenticated attach.
			if e = resumed.Close(); e != nil {
				t.Fatal(e)
			}
			assertFirst()
			for name, rows := range map[string]map[string][]byte{"stable": secondStableRows, "entries": secondEntryRows} {
				if !reflect.DeepEqual(rows, stableInstallRows(t, filepath.Join(secondDir, name, "db"))) {
					t.Fatalf("resume before login changed second %s rows", name)
				}
			}
			for name, rows := range map[string]map[string][]byte{"events": eventRows, "seeds": seedRows, "cards": cardRows} {
				if !reflect.DeepEqual(rows, stableInstallRows(t, filepath.Join(root, name, "db"))) {
					t.Fatalf("resume before login changed %s rows", name)
				}
			}
			resumed, e = NewBouts(context.Background(), cfg)
			if e != nil {
				t.Fatal(e)
			}
			if e = seedInstallLogin(t, resumed.ManifestController, "incrementing256"); e != nil {
				resumed.Close()
				t.Fatal(e)
			}
			stableInstallWaitLogin(t, resumed.ManifestController, true)
			snap := resumed.Snapshot()
			if snap.GymID != killed.Snapshot.GymID || snap.GymPublicKey != killed.Snapshot.GymPublicKey || snap.PayoutAddress != killed.Snapshot.PayoutAddress {
				t.Fatal("resumed second fixture identity changed")
			}
			if snap.Attempts != 0 || snap.AcceptedCount != 0 || snap.Height != 0 || snap.State != "stopped" {
				resumed.Close()
				t.Fatal("installation mined")
			}
			if resumed.BoutSnapshot().State != "ready" {
				resumed.Close()
				t.Fatal("bout attach")
			}
			if e = resumed.Close(); e != nil {
				t.Fatal(e)
			}
			assertFirst()
			afterMarker, e := readBoutMarker(markerPath)
			if e != nil || afterMarker.BoutPhase != "ready" || afterMarker.BoutStoreID != killed.Marker.BoutStoreID || !reflect.DeepEqual(afterMarker.cardAccountMarker, killed.Marker.cardAccountMarker) {
				t.Fatal("resume changed an existing root field or source identity", e)
			}
			boutInstallAssertLoginClockOnly(t, "stable", secondStableRows, stableInstallRows(t, filepath.Join(secondDir, "stable", "db")), "clock_ms", "last_as_of_ms")
			boutInstallAssertLoginClockOnly(t, "entries", secondEntryRows, stableInstallRows(t, filepath.Join(secondDir, "entries", "db")), "clock_ms")
			if !reflect.DeepEqual(eventRows, stableInstallRows(t, filepath.Join(root, "events", "db"))) {
				t.Fatal("bout installation changed existing event rows")
			}
			if !reflect.DeepEqual(seedRows, stableInstallRows(t, filepath.Join(root, "seeds", "db"))) {
				t.Fatal("bout install changed seed rows")
			}
			if !reflect.DeepEqual(cardRows, stableInstallRows(t, filepath.Join(root, "cards", "db"))) {
				t.Fatal("bout install changed card rows")
			}
			if e = os.Rename(boutPath, boutPath+"-retained"); e != nil {
				t.Fatal(e)
			}
			bad, e := NewBouts(context.Background(), cfg)
			if bad != nil {
				bad.Close()
			}
			if e == nil || bad != nil {
				t.Fatal("missing ready recreated")
			}
			if _, e = os.Lstat(boutPath); !os.IsNotExist(e) {
				t.Fatal("missing refusal wrote path")
			}
			if e = os.Rename(boutPath+"-retained", boutPath); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func boutInstallCrashChild(t *testing.T, input string) {
	b, e := os.ReadFile(input)
	if e != nil {
		t.Fatal(e)
	}
	var in boutInstallCrashInput
	if json.Unmarshal(b, &in) != nil {
		t.Fatal("input")
	}
	c, e := NewBouts(context.Background(), pilot.Config{DataDir: in.Root, NodeBinary: in.Node})
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
		m, e := readBoutMarker(filepath.Join(in.Root, "SLUG-MANIFEST-ACCOUNTS.json"))
		if e != nil {
			t.Error(e)
			return
		}
		snap := child.Snapshot()
		if snap.Attempts != 0 || snap.AcceptedCount != 0 || snap.Height != 0 {
			t.Error("unexpected work")
			return
		}
		proof := boutInstallCrashEvidence{Marker: m, Snapshot: snap, Retained: stableInstallCapture(t, filepath.Join(in.Root, "gyms", child.AccountID()))}
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

// Installation was already checked byte-exact before login. Authenticated attach
// performs existing stable Sync/entry observations, which can advance only these
// explicitly named timestamps. Every other metadata field and row stays exact.
func boutInstallAssertLoginClockOnly(t *testing.T, name string, before, after map[string][]byte, fields ...string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("%s row set changed", name)
	}
	for key, old := range before {
		current, ok := after[key]
		if !ok {
			t.Fatalf("%s row missing", name)
		}
		if key != "meta" {
			if !bytes.Equal(old, current) {
				t.Fatalf("%s non-metadata row changed", name)
			}
			continue
		}
		var a, b map[string]json.RawMessage
		if json.Unmarshal(old, &a) != nil || json.Unmarshal(current, &b) != nil {
			t.Fatal("metadata decode")
		}
		for _, field := range fields {
			var x, y int64
			if json.Unmarshal(a[field], &x) != nil || json.Unmarshal(b[field], &y) != nil || x <= 0 || y < x || y > time.Now().UnixMilli() {
				t.Fatalf("%s invalid monotone %s", name, field)
			}
			delete(a, field)
			delete(b, field)
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%s non-clock metadata changed", name)
		}
	}
}
