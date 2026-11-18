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

type resultInstallCrashInput struct{ Root, Node, Stage, Signal string }
type resultInstallCrashEvidence struct {
	Marker      resultAccountMarker
	Snapshot    pilot.ManifestSnapshot
	Retained    stableInstallEvidence
	CreatedAtMS int64
}

// Opt-in real SIGKILL with two published fixture wallets, a frozen normal node,
// no mining, and only owned disposable data. Old evidence and root files survive.
func TestResultInstallProcessCrash(t *testing.T) {
	if input := os.Getenv("SLUG_RESULT_INSTALL_CRASH_INPUT"); input != "" {
		resultInstallCrashChild(t, input)
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
	for _, stage := range []string{"result-prepared", "result-created", "result-ready"} {
		t.Run(stage, func(t *testing.T) {
			base, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			root := filepath.Join(base, "accounts")
			cfg := pilot.Config{DataDir: root, NodeBinary: node}
			first, e := NewResults(context.Background(), cfg)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = first.Close() })
			if e = seedInstallLogin(t, first.ManifestController, "zero256"); e != nil {
				first.Close()
				t.Fatal(e)
			}
			stableInstallWaitLogin(t, first.ManifestController, true)
			if first.ResultSnapshot().State != "setup_incomplete" {
				first.Close()
				t.Fatal("first fixture invented source")
			}
			if e = first.Close(); e != nil {
				t.Fatal(e)
			}
			markerPath := filepath.Join(root, "SLUG-MANIFEST-ACCOUNTS.json")
			m, e := readResultMarker(markerPath)
			if e != nil || len(m.EntryInstalls) != 1 {
				t.Fatal(e)
			}
			firstInstall := m.EntryInstalls[0]
			firstDir := filepath.Join(root, "gyms", firstInstall.AccountID)
			retained := stableInstallCapture(t, firstDir)
			stableRows := stableInstallRows(t, filepath.Join(firstDir, "stable", "db"))
			entryRows := stableInstallRows(t, filepath.Join(firstDir, "entries", "db"))
			in := resultInstallCrashInput{root, node, stage, filepath.Join(base, "checkpoint.json")}
			input := filepath.Join(base, "input.json")
			b, _ := json.Marshal(in)
			if e = os.WriteFile(input, b, 0600); e != nil {
				t.Fatal(e)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestResultInstallProcessCrash$", "-test.timeout=180s", "-test.v")
			cmd.Env = append(os.Environ(), "SLUG_RESULT_INSTALL_CRASH_INPUT="+input)
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
			var killed resultInstallCrashEvidence
			b, e = os.ReadFile(in.Signal)
			if e != nil || json.Unmarshal(b, &killed) != nil {
				t.Fatal(e)
			}
			want := "prepared"
			if stage == "result-ready" {
				want = "ready"
			}
			if killed.Marker.ResultPhase != want || killed.Marker.BoutPhase != "ready" || killed.Marker.CardPhase != "ready" || killed.Marker.SeedPhase != "ready" || killed.Marker.EventPhase != "ready" || len(killed.Marker.EntryInstalls) != 2 {
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
				m, e := readResultMarker(markerPath)
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
			boutRows := stableInstallRows(t, filepath.Join(root, "bouts", "db"))
			for _, open := range []func() error{func() error {
				x, e := NewBouts(context.Background(), cfg)
				if x != nil {
					x.Close()
				}
				return e
			}, func() error {
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
					t.Fatal("old route accepted v8")
				}
				now, _ := os.ReadFile(markerPath)
				if !bytes.Equal(now, prior) {
					t.Fatal("old refusal mutated root")
				}
			}
			resultPath := filepath.Join(root, "results")
			if stage != "result-prepared" {
				rows := stableInstallRows(t, filepath.Join(resultPath, "db"))
				var meta struct {
					CreatedAtMS int64 `json:"created_at_ms"`
				}
				if json.Unmarshal(rows["meta"], &meta) != nil || meta.CreatedAtMS == 0 {
					t.Fatal("missing killed initialized result metadata")
				}
				killed.CreatedAtMS = meta.CreatedAtMS
			}
			if stage == "result-prepared" {
				if _, e = os.Lstat(resultPath); !os.IsNotExist(e) {
					t.Fatal("early result create")
				}
				if e = os.Mkdir(resultPath, 0700); e != nil {
					t.Fatal(e)
				}
				partial := filepath.Join(resultPath, "unknown")
				kept := []byte("retained partial evidence")
				_ = os.WriteFile(partial, kept, 0600)
				bad, e := NewResults(context.Background(), cfg)
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
			resumed, e := NewResults(context.Background(), cfg)
			if e != nil {
				t.Fatal("resume", e)
			}
			t.Cleanup(func() { _ = resumed.Close() })
			info, e := resumed.resultOwner.store.Info(t.Context())
			if e != nil || info.ResultCount != 0 || info.StoreID != killed.Marker.ResultStoreID {
				resumed.Close()
				t.Fatal("resume pin", e)
			}
			cardInfo, se := resumed.boutOwner.store.Info(t.Context())
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
			for name, rows := range map[string]map[string][]byte{"events": eventRows, "seeds": seedRows, "cards": cardRows, "bouts": boutRows} {
				if !reflect.DeepEqual(rows, stableInstallRows(t, filepath.Join(root, name, "db"))) {
					t.Fatalf("resume before login changed %s rows", name)
				}
			}
			resumed, e = NewResults(context.Background(), cfg)
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
			if snap.Attempts != 0 || snap.AcceptedCount != 0 || snap.Pending != 0 || snap.Height != 0 || snap.State != "stopped" {
				resumed.Close()
				t.Fatal("installation mined")
			}
			if resumed.ResultSnapshot().State != "ready" {
				resumed.Close()
				t.Fatal("result attach")
			}
			if e = resumed.Close(); e != nil {
				t.Fatal(e)
			}
			assertFirst()
			afterMarker, e := readResultMarker(markerPath)
			if e != nil || afterMarker.ResultPhase != "ready" || afterMarker.ResultStoreID != killed.Marker.ResultStoreID || !reflect.DeepEqual(afterMarker.boutAccountMarker, killed.Marker.boutAccountMarker) {
				t.Fatal("resume changed an existing root field or source identity", e)
			}
			boutInstallAssertLoginClockOnly(t, "stable", secondStableRows, stableInstallRows(t, filepath.Join(secondDir, "stable", "db")), "clock_ms", "last_as_of_ms")
			boutInstallAssertLoginClockOnly(t, "entries", secondEntryRows, stableInstallRows(t, filepath.Join(secondDir, "entries", "db")), "clock_ms")
			if !reflect.DeepEqual(eventRows, stableInstallRows(t, filepath.Join(root, "events", "db"))) {
				t.Fatal("result installation changed existing event rows")
			}
			if !reflect.DeepEqual(seedRows, stableInstallRows(t, filepath.Join(root, "seeds", "db"))) {
				t.Fatal("result install changed seed rows")
			}
			if !reflect.DeepEqual(cardRows, stableInstallRows(t, filepath.Join(root, "cards", "db"))) {
				t.Fatal("result install changed card rows")
			}
			if !reflect.DeepEqual(boutRows, stableInstallRows(t, filepath.Join(root, "bouts", "db"))) {
				t.Fatal("result install changed bout rows")
			}
			if e = os.Rename(resultPath, resultPath+"-retained"); e != nil {
				t.Fatal(e)
			}
			bad, e := NewResults(context.Background(), cfg)
			if bad != nil {
				bad.Close()
			}
			if e == nil || bad != nil {
				t.Fatal("missing ready recreated")
			}
			if _, e = os.Lstat(resultPath); !os.IsNotExist(e) {
				t.Fatal("missing refusal wrote path")
			}
			if e = os.Rename(resultPath+"-retained", resultPath); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func resultInstallCrashChild(t *testing.T, input string) {
	b, e := os.ReadFile(input)
	if e != nil {
		t.Fatal(e)
	}
	var in resultInstallCrashInput
	if json.Unmarshal(b, &in) != nil {
		t.Fatal("input")
	}
	c, e := NewResults(context.Background(), pilot.Config{DataDir: in.Root, NodeBinary: in.Node})
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
		m, e := readResultMarker(filepath.Join(in.Root, "SLUG-MANIFEST-ACCOUNTS.json"))
		if e != nil {
			t.Error(e)
			return
		}
		snap := child.Snapshot()
		if snap.Attempts != 0 || snap.AcceptedCount != 0 || snap.Pending != 0 || snap.Height != 0 {
			t.Error("unexpected work")
			return
		}
		proof := resultInstallCrashEvidence{Marker: m, Snapshot: snap, Retained: stableInstallCapture(t, filepath.Join(in.Root, "gyms", child.AccountID()))}
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
