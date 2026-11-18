// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
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
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/wire/v2"
)

type careerInstallCrashInput struct {
	Root, Node, Stage, Signal string
	Occurrence                int
}
type careerInstallCrashEvidence struct {
	Marker     careerAccountMarker
	Snapshot   pilot.ManifestSnapshot
	Occurrence int
}

// Real child SIGKILL after each durable parent v9 checkpoint. The two enrolled
// published fixtures have no admissions or saved results; this proves install
// recovery and exact retention of their existing rows, not nonempty bout history.
// Start is never called. An explicit evidence directory preserves disposable
// roots and child logs after failure; it must be fresh for each invocation.
func TestCareerInstallProcessCrash(t *testing.T) {
	if input := os.Getenv("SLUG_CAREER_INSTALL_CRASH_INPUT"); input != "" {
		careerInstallCrashChild(t, input)
		return
	}
	if wire.TestPoW {
		t.Skip("normal profile required")
	}
	node := os.Getenv("SLUG_STABLE_NODE")
	if node == "" {
		t.Skip("explicit frozen normal node required")
	}
	if !filepath.IsAbs(node) {
		t.Fatal("absolute node required")
	}
	stages := []struct {
		name, stage string
		occurrence  int
		guards      int
		card        bool
	}{
		{"prepared", "career-prepared", 1, 0, false},
		{"activation", "career-activation", 1, 0, false},
		{"first-stable", "career-stable-bound", 1, 1, false},
		{"second-stable", "career-stable-bound", 2, 2, false},
		{"card", "career-card-bound", 1, 2, true},
		{"ready", "career-ready", 1, 2, true},
	}
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			base := t.TempDir()
			if evidence := os.Getenv("SLUG_CAREER_INSTALL_EVIDENCE_DIR"); evidence != "" {
				if !filepath.IsAbs(evidence) {
					t.Fatal("absolute owned evidence directory required")
				}
				base = filepath.Join(evidence, stage.name)
				if e := os.Mkdir(base, 0700); e != nil {
					t.Fatal("fresh stage directory", e)
				}
			}
			base, e := filepath.EvalSymlinks(base)
			if e != nil {
				t.Fatal(e)
			}
			cfg := pilot.Config{DataDir: filepath.Join(base, "accounts"), NodeBinary: node}
			first, e := NewCareers(t.Context(), cfg)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = first.Close() })
			if e = seedInstallLogin(t, first.ManifestController, "zero256"); e != nil {
				t.Fatal(e)
			}
			stableInstallWaitLogin(t, first.ManifestController, true)
			careerInstallNoWork(t, first.Snapshot())
			if e = first.Close(); e != nil {
				t.Fatal(e)
			}
			markerPath := filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
			initial, e := readCareerMarker(markerPath)
			if e != nil || initial.CareerPhase != "waiting_source" || initial.CareerActivation != nil || len(initial.EntryInstalls) != 1 {
				t.Fatal("first fixture invented activation", e)
			}
			firstInstall := initial.EntryInstalls[0]
			firstDir := filepath.Join(cfg.DataDir, "gyms", firstInstall.AccountID)
			firstRetained := stableInstallCapture(t, firstDir)
			firstStable := stableInstallRows(t, filepath.Join(firstDir, "stable", "db"))
			firstEntry := stableInstallRows(t, filepath.Join(firstDir, "entries", "db"))
			in := careerInstallCrashInput{cfg.DataDir, node, stage.stage, filepath.Join(base, "checkpoint.json"), stage.occurrence}
			input := filepath.Join(base, "input.json")
			raw, e := json.Marshal(in)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(input, raw, 0600); e != nil {
				t.Fatal(e)
			}
			careerInstallKill(t, input, in.Signal, filepath.Join(base, "child.log"))
			var killed careerInstallCrashEvidence
			raw, e = os.ReadFile(in.Signal)
			if e != nil || json.Unmarshal(raw, &killed) != nil || killed.Occurrence != stage.occurrence {
				t.Fatal("checkpoint evidence", e)
			}
			m := killed.Marker
			wantPhase := "prepared"
			if stage.stage == "career-ready" {
				wantPhase = "ready"
			}
			if m.CareerPhase != wantPhase || m.ResultPhase != "ready" || len(m.StableInstalls) != 2 || len(m.EntryInstalls) != 2 || (m.CareerActivation == nil) != (stage.stage == "career-prepared") {
				t.Fatal("wrong durable checkpoint phase")
			}
			careerInstallNoWork(t, killed.Snapshot)
			before := careerInstallRows(t, cfg.DataDir, m)
			bound := 0
			for _, x := range m.StableInstalls {
				if before[filepath.Join("gyms", x.AccountID, "stable")][careerconsumer.GuardKey] != nil {
					bound++
				}
			}
			if bound != stage.guards || (before["cards"][careerconsumer.GuardKey] != nil) != stage.card {
				t.Fatalf("wrong partial guard state: stable=%d card=%v", bound, before["cards"][careerconsumer.GuardKey] != nil)
			}
			stableInstallAssertRetained(t, firstDir, firstRetained, false)
			if !reflect.DeepEqual(firstStable, careerInstallWithoutGuard(before[filepath.Join("gyms", firstInstall.AccountID, "stable")])) || !reflect.DeepEqual(firstEntry, before[filepath.Join("gyms", firstInstall.AccountID, "entries")]) {
				t.Fatal("second fixture preparation rewrote first history")
			}
			careerInstallOldRefusals(t, cfg, markerPath)
			if !reflect.DeepEqual(before, careerInstallRows(t, cfg.DataDir, m)) {
				t.Fatal("old constructor refusal changed rows")
			}
			resumed, e := NewCareers(t.Context(), cfg)
			if e != nil {
				t.Fatal("resume", e)
			}
			t.Cleanup(func() {
				if resumed != nil {
					_ = resumed.Close()
				}
			})
			activation, e := resumed.resultOwner.store.CareerInfo(t.Context())
			if e != nil || careerstate.ValidateActivation(activation) != nil || activation.ResultStoreID != m.ResultStoreID {
				t.Fatal("resumed activation", e)
			}
			if m.CareerActivation != nil && activation != *m.CareerActivation {
				t.Fatal("recovery replaced activation")
			}
			if e = resumed.Close(); e != nil {
				t.Fatal(e)
			}
			ready, e := readCareerMarker(markerPath)
			if e != nil || ready.CareerPhase != "ready" || ready.CareerActivation == nil || *ready.CareerActivation != activation || !reflect.DeepEqual(ready.resultAccountMarker, m.resultAccountMarker) {
				t.Fatal("ready changed source identity or activation", e)
			}
			after := careerInstallRows(t, cfg.DataDir, ready)
			careerInstallCompareRows(t, before, after, activation, stage.stage == "career-prepared")
			stableInstallAssertRetained(t, firstDir, firstRetained, false)
			// Authenticated attach is a separate observation: only the logged-in
			// Stable/Entry clock fields may advance, never historical payloads.
			resumed, e = NewCareers(t.Context(), cfg)
			if e != nil {
				t.Fatal(e)
			}
			if e = seedInstallLogin(t, resumed.ManifestController, "incrementing256"); e != nil {
				t.Fatal(e)
			}
			stableInstallWaitLogin(t, resumed.ManifestController, true)
			snap := resumed.Snapshot()
			careerInstallNoWork(t, snap)
			if snap.View == nil || snap.GymID != killed.Snapshot.GymID || snap.GymPublicKey != killed.Snapshot.GymPublicKey || snap.PayoutAddress != killed.Snapshot.PayoutAddress {
				t.Fatal("recovered fixture identity changed")
			}
			jobID := careerInstallPageJob(t, resumed, snap.View.ViewID)
			page, e := resumed.CareerInventory(t.Context(), snap.View.ViewID, jobID, 0, 8)
			if e != nil || page.Activation != activation || len(page.Items) != 0 || len(page.Source.Items) != 0 {
				t.Fatal("career attachment", e)
			}
			if e = resumed.Close(); e != nil {
				t.Fatal(e)
			}
			postMarker, e := readCareerMarker(markerPath)
			if e != nil || !reflect.DeepEqual(postMarker, ready) {
				t.Fatal("login changed retained ready root", e)
			}
			postLogin := careerInstallRows(t, cfg.DataDir, ready)
			for name, rows := range after {
				switch name {
				case filepath.Join("gyms", snap.View.AccountID, "stable"):
					boutInstallAssertLoginClockOnly(t, name, rows, postLogin[name], "clock_ms", "last_as_of_ms")
				case filepath.Join("gyms", snap.View.AccountID, "entries"):
					boutInstallAssertLoginClockOnly(t, name, rows, postLogin[name], "clock_ms")
				default:
					if !reflect.DeepEqual(rows, postLogin[name]) {
						t.Fatalf("login rewrote %s", name)
					}
				}
			}
			if stage.stage == "career-ready" {
				careerInstallMissingReady(t, cfg, ready)
			}
			t.Logf("SIGKILL recovered %s occurrence=%d exact activation=%s; no admissions/mining", stage.stage, stage.occurrence, activation.ActivationID)
		})
	}
}

func careerInstallNoWork(t *testing.T, s pilot.ManifestSnapshot) {
	t.Helper()
	if s.Attempts != 0 || s.AcceptedCount != 0 || s.Pending != 0 || s.Height != 0 || s.State != "stopped" {
		t.Fatal("installation started work", s.State)
	}
}

func careerInstallKill(t *testing.T, input, signal, logPath string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCareerInstallProcessCrash$", "-test.timeout=180s", "-test.v")
	cmd.Env = append(os.Environ(), "SLUG_CAREER_INSTALL_CRASH_INPUT="+input)
	log, e := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
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
		if _, e = os.Stat(signal); e == nil {
			break
		} else if !os.IsNotExist(e) {
			t.Fatal(e)
		}
		select {
		case e = <-done:
			joined = true
			raw, _ := os.ReadFile(logPath)
			t.Fatalf("child exited before durable checkpoint: %v\n%s", e, raw)
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
		t.Fatal("child did not fail by signal", e)
	}
	ws, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
		t.Fatal("child did not terminate by SIGKILL")
	}
}

func careerInstallCrashChild(t *testing.T, path string) {
	raw, e := os.ReadFile(path)
	var in careerInstallCrashInput
	if e != nil || json.Unmarshal(raw, &in) != nil {
		t.Fatal("child input", e)
	}
	c, e := NewCareers(t.Context(), pilot.Config{DataDir: in.Root, NodeBinary: in.Node})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	occurrence := 0
	c.checkpoint = func(stage string) {
		if stage != in.Stage {
			return
		}
		occurrence++
		if occurrence != in.Occurrence {
			return
		}
		m, e := readCareerMarker(filepath.Join(in.Root, "SLUG-MANIFEST-ACCOUNTS.json"))
		if e != nil {
			t.Error(e)
			return
		}
		c.mu.Lock()
		child := c.child
		c.mu.Unlock()
		if child == nil {
			t.Error("missing installing child")
			return
		}
		snapshot := child.Snapshot()
		careerInstallNoWork(t, snapshot)
		raw, e := json.Marshal(careerInstallCrashEvidence{m, snapshot, occurrence})
		if e != nil {
			t.Error(e)
			return
		}
		stableInstallSignal(t, in.Signal, raw)
		select {}
	}
	if e = seedInstallLogin(t, c.ManifestController, "incrementing256"); e != nil {
		t.Fatal(e)
	}
	stableInstallWaitLogin(t, c.ManifestController, true)
	t.Fatal("missed crash checkpoint")
}

func careerInstallRows(t *testing.T, root string, m careerAccountMarker) map[string]map[string][]byte {
	t.Helper()
	out := make(map[string]map[string][]byte)
	for _, name := range []string{"events", "seeds", "cards", "bouts", "results"} {
		out[name] = stableInstallRows(t, filepath.Join(root, name, "db"))
	}
	for _, x := range m.StableInstalls {
		for _, component := range []string{"stable", "entries", "manifest-authority", "manifest-outbox"} {
			name := filepath.Join("gyms", x.AccountID, component)
			out[name] = stableInstallRows(t, filepath.Join(root, name, "db"))
		}
	}
	return out
}

func careerInstallWithoutGuard(rows map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(rows))
	for key, raw := range rows {
		if key != careerconsumer.GuardKey {
			out[key] = raw
		}
	}
	return out
}

func careerInstallCompareRows(t *testing.T, before, after map[string]map[string][]byte, activation careerstate.Activation, activated bool) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatal("component set changed")
	}
	var guard []byte
	for name, prior := range before {
		current := after[name]
		if name == "cards" || filepath.Base(name) == "stable" {
			raw := current[careerconsumer.GuardKey]
			var g struct {
				Version     uint32                 `json:"version"`
				Activation  careerstate.Activation `json:"activation"`
				FenceCount  uint32                 `json:"fence_count"`
				FenceDigest string                 `json:"fence_digest"`
			}
			if json.Unmarshal(raw, &g) != nil || g.Version != 1 || g.Activation != activation || g.FenceCount != 0 || !careerstate.ID(g.FenceDigest) {
				t.Fatalf("bad installed guard %s", name)
			}
			if guard == nil {
				guard = raw
			} else if !bytes.Equal(guard, raw) {
				t.Fatal("mixed guard identity")
			}
			if old := prior[careerconsumer.GuardKey]; old != nil && !bytes.Equal(old, raw) {
				t.Fatal("rewrote existing guard")
			}
			if !reflect.DeepEqual(careerInstallWithoutGuard(prior), careerInstallWithoutGuard(current)) {
				t.Fatalf("binding changed %s history", name)
			}
		} else if name == "results" && activated {
			careerInstallActivationRows(t, prior, current, activation)
		} else if !reflect.DeepEqual(prior, current) {
			t.Fatalf("resume changed %s rows", name)
		}
	}
}

func careerInstallActivationRows(t *testing.T, before, after map[string][]byte, activation careerstate.Activation) {
	t.Helper()
	if len(after) != len(before)+1 || before["career_init"] != nil || after["career_init"] == nil {
		t.Fatal("unexpected activation row set")
	}
	for key, raw := range before {
		if key != "meta" && !bytes.Equal(raw, after[key]) {
			t.Fatal("activation changed historical result row")
		}
	}
	var a, b map[string]json.RawMessage
	if json.Unmarshal(before["meta"], &a) != nil || json.Unmarshal(after["meta"], &b) != nil || string(a["version"]) != "1" || string(b["version"]) != "2" || a["career"] != nil {
		t.Fatal("activation metadata versions")
	}
	var got careerstate.Activation
	var oldClock, newClock int64
	if json.Unmarshal(b["career"], &got) != nil || got != activation || json.Unmarshal(a["clock_ms"], &oldClock) != nil || json.Unmarshal(b["clock_ms"], &newClock) != nil || newClock != activation.ActivatedAtMS || newClock < oldClock || newClock > time.Now().UnixMilli() {
		t.Fatal("activation time/identity")
	}
	for _, key := range []string{"version", "clock_ms", "career"} {
		delete(a, key)
		delete(b, key)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("activation changed preexisting metadata")
	}
}

func careerInstallOldRefusals(t *testing.T, cfg pilot.Config, markerPath string) {
	t.Helper()
	before, e := os.ReadFile(markerPath)
	if e != nil {
		t.Fatal(e)
	}
	for _, open := range []func() error{
		func() error {
			c, e := NewResults(context.Background(), cfg)
			if c != nil {
				_ = c.Close()
			}
			return e
		},
		func() error {
			c, e := NewBouts(context.Background(), cfg)
			if c != nil {
				_ = c.Close()
			}
			return e
		},
		func() error {
			c, e := NewCards(cfg)
			if c != nil {
				_ = c.Close()
			}
			return e
		},
		func() error {
			c, e := NewSeeds(cfg)
			if c != nil {
				_ = c.Close()
			}
			return e
		},
		func() error {
			c, e := NewEvents(cfg)
			if c != nil {
				_ = c.Close()
			}
			return e
		},
		func() error {
			c, e := NewEntries(cfg)
			if c != nil {
				_ = c.Close()
			}
			return e
		},
		func() error {
			c, e := NewStable(cfg)
			if c != nil {
				_ = c.Close()
			}
			return e
		},
		func() error {
			c, e := NewManifest(cfg)
			if c != nil {
				_ = c.Close()
			}
			return e
		},
	} {
		if open() == nil {
			t.Fatal("old route accepted parent v9")
		}
		after, e := os.ReadFile(markerPath)
		if e != nil || !bytes.Equal(before, after) {
			t.Fatal("old route refusal mutated root")
		}
	}
}

func careerInstallMissingReady(t *testing.T, cfg pilot.Config, m careerAccountMarker) {
	t.Helper()
	markerPath := filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	marker, e := os.ReadFile(markerPath)
	if e != nil {
		t.Fatal(e)
	}
	paths := []string{filepath.Join(cfg.DataDir, "cards", "db")}
	for _, x := range m.StableInstalls {
		paths = append(paths, filepath.Join(cfg.DataDir, "gyms", x.AccountID, "stable", "db"))
	}
	for _, path := range paths {
		db, e := leveldb.OpenFile(path, &opt.Options{ErrorIfMissing: true})
		if e != nil {
			t.Fatal(e)
		}
		raw, e := db.Get([]byte(careerconsumer.GuardKey), nil)
		if e == nil {
			e = db.Delete([]byte(careerconsumer.GuardKey), &opt.WriteOptions{Sync: true})
		}
		e = errors.Join(e, db.Close())
		if e != nil {
			t.Fatal(e)
		}
		before := careerInstallRows(t, cfg.DataDir, m)
		bad, openErr := NewCareers(context.Background(), cfg)
		if bad != nil {
			_ = bad.Close()
		}
		if openErr == nil || bad != nil {
			t.Fatal("ready missing guard adopted")
		}
		after, e := os.ReadFile(markerPath)
		if e != nil || !bytes.Equal(marker, after) || !reflect.DeepEqual(before, careerInstallRows(t, cfg.DataDir, m)) {
			t.Fatal("missing-ready refusal rewrote evidence")
		}
		db, e = leveldb.OpenFile(path, &opt.Options{ErrorIfMissing: true})
		if e != nil {
			t.Fatal(e)
		}
		e = db.Put([]byte(careerconsumer.GuardKey), raw, &opt.WriteOptions{Sync: true})
		e = errors.Join(e, db.Close())
		if e != nil {
			t.Fatal(e)
		}
	}
}

// Keep this helper local: the crash test also compiles in diagnostic profiles,
// whereas the ordinary parent readiness helpers exclude slugtestpow.
func careerInstallPageJob(t *testing.T, c *StableController, view string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	j, e := c.BeginStableInventory(ctx, view, 0, 8)
	if e != nil {
		t.Fatal("career source page admission", e)
	}
	id := j.JobID
	for ctx.Err() == nil {
		j, e = c.StableJob(ctx, view, id)
		if e != nil {
			t.Fatal("career source page poll", e)
		}
		if j.JobID != id || j.View.ViewID != view || j.Kind != "inventory" {
			t.Fatal("career source page identity changed")
		}
		if j.State == "done" {
			if j.Error != "" || j.Page == nil || j.Roster != nil {
				t.Fatal("career source page result", j.Error)
			}
			return id
		}
		if j.State != "queued" && j.State != "running" {
			t.Fatal("career source page terminal", j.State, j.Error)
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Millisecond):
		}
	}
	t.Fatal("career source page deadline", ctx.Err())
	return ""
}
