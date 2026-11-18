//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func resultCrashHook() func(string) {
	stage := os.Getenv("SLUG_RESULT_CRASH_STAGE")
	if stage == "" {
		return nil
	}
	return func(s string) {
		if s == stage {
			if e := os.WriteFile(filepath.Join(os.Getenv("SLUG_RESULT_CRASH_ROOT"), "checkpoint"), []byte(s), 0600); e != nil {
				panic(e)
			}
			select {}
		}
	}
}
func TestResultCrashChild(t *testing.T) {
	if os.Getenv("SLUG_RESULT_CRASH_STAGE") == "" {
		t.Skip("owned subprocess only")
	}
	f := newResultFixture(t)
	hook := resultCrashHook()
	f.s.checkpoint = hook
	c, done := fixtureCall(t, f)
	defer done()
	first, e := f.s.prepare(c)
	if e != nil {
		t.Fatal(e)
	}
	m := fundedPublisher(t, f, first.Record)
	f.s.checkpoint = func(stage string) {
		if stage == "record-synced" {
			r, ok, e := f.s.record(c.key)
			if e != nil {
				panic(e)
			}
			if ok && r.Publication != nil && r.Publication.State == "submitted" {
				hook("submitted-synced")
			}
		}
		hook(stage)
	}
	if _, e = f.s.publish(c, m); e != nil {
		t.Fatal(e)
	}
	t.Fatal("checkpoint not reached")
}
func TestResultActualSIGKILLRecovery(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal profile")
	}
	for _, stage := range []string{"marker-synced", "init-synced", "initialized", "before-prepare", "prepared", "transaction-retained", "dispatch-armed", "dispatch-acknowledged", "submitted-synced"} {
		t.Run(stage, func(t *testing.T) {
			dir, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestResultCrashChild$", "-test.timeout=18s")
			cmd.Env = append(os.Environ(), "SLUG_RESULT_CRASH_ROOT="+dir, "SLUG_RESULT_CRASH_STAGE="+stage)
			log, e := os.Create(filepath.Join(dir, "child.log"))
			if e != nil {
				t.Fatal(e)
			}
			defer log.Close()
			cmd.Stdout = log
			cmd.Stderr = log
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			wait := make(chan error, 1)
			go func() { wait <- cmd.Wait() }()
			joined := false
			defer func() {
				if !joined {
					_ = cmd.Process.Kill()
					<-wait
				}
			}()
			for {
				if raw, e := os.ReadFile(filepath.Join(dir, "checkpoint")); e == nil && string(raw) == stage {
					break
				}
				select {
				case e := <-wait:
					joined = true
					raw, _ := os.ReadFile(filepath.Join(dir, "child.log"))
					t.Fatalf("child exited: %v %s", e, raw)
				case <-ctx.Done():
					t.Fatal("checkpoint deadline")
				case <-time.After(10 * time.Millisecond):
				}
			}
			if e = cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			if e = <-wait; e == nil {
				t.Fatal("expected killed child")
			}
			joined = true
			// Identical concrete source authorities/creation facts, freshly opened in
			// an isolated test fixture, authenticate recovery of the child's result DB.
			// This tests result journal durability, not a real network dispatch.
			f := newResultFixture(t)
			f.s.Close()
			f.now.Add(1000)
			path := filepath.Join(dir, "result")
			partial := stage == "marker-synced" || stage == "init-synced"
			if partial {
				if _, e = open(context.Background(), path, f.pin, f.bout, f.clock, false); !errors.Is(e, ErrCorrupt) {
					t.Fatal("ready adopted partial", e)
				}
			}
			f.s, e = open(context.Background(), path, f.pin, f.bout, f.clock, partial)
			if e != nil {
				t.Fatal("reopen", e)
			}
			info, e := f.s.Info(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			want := uint32(1)
			if partial || stage == "initialized" || stage == "before-prepare" {
				want = 0
			}
			if info.ResultCount != want {
				t.Fatal("atomic result count", info.ResultCount, want)
			}
			if partial && info.CreatedAtMS != f.now.Load() {
				t.Fatal("resume backdated")
			}
			if !partial && info.CreatedAtMS >= f.now.Load() {
				t.Fatal("reopen replaced creation")
			}
			if want == 1 {
				c, done := fixtureCall(t, f)
				r, _, e := f.s.read(c, "view", true)
				done()
				if e != nil {
					t.Fatal(e)
				}
				state := ""
				count := uint32(0)
				switch stage {
				case "transaction-retained":
					state = "built"
				case "dispatch-armed", "dispatch-acknowledged":
					state = "dispatch_unknown"
					count = 1
				case "submitted-synced":
					state = "submitted"
					count = 1
				}
				if state == "" {
					if r.Publication != nil {
						t.Fatal("uncommitted publication retained")
					}
				} else if r.Publication == nil || r.Publication.State != state || r.Publication.DispatchCount != count {
					t.Fatal("dispatch state guessed", state, r.Publication)
				}
			}
			auditCurrent(t, f)
			t.Logf("SIGKILL joined at %s; recovered results=%d", stage, want)
		})
	}
}
