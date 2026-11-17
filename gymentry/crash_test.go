// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type crashState struct {
	Pin                         Pin
	Path, OperationID, Saturday string
	Now                         int64
}

func TestEntryCrashHelper(t *testing.T) {
	root := os.Getenv("SLUG_ENTRY_CRASH_ROOT")
	if root == "" {
		t.Skip("process-kill helper")
	}
	stage := os.Getenv("SLUG_ENTRY_CRASH_STAGE")
	x := newEntryFixture(t, 0)
	ctx := context.Background()
	r := x.request(t, "withdraw", 0)
	state := crashState{Pin: x.pin, Path: x.path, Saturday: r.Saturday, Now: x.now}
	barrier := func(mark string) {
		if mark != stage {
			return
		}
		raw, _ := json.Marshal(state)
		if e := os.WriteFile(filepath.Join(root, "barrier.json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		select {}
	}
	if stage == "before_initialize" || stage == "initialized" {
		x.s.Close()
		x.s = nil
		if e := os.RemoveAll(x.path); e != nil {
			t.Fatal(e)
		}
		var e error
		x.s, e = create(ctx, x.path, x.pin, x.stable, time.Now, barrier)
		if e != nil {
			t.Fatal(e)
		}
		t.Fatal("creation barrier not reached")
	}
	x.s.checkpoint = barrier
	i, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	state.OperationID = i.OperationID
	if stage == "before_abandon" || stage == "after_abandon" {
		_, e = x.s.Abandon(ctx, x.session, i.OperationID)
	} else {
		_, e = x.s.Apply(ctx, x.session, i.OperationID)
	}
	if e != nil {
		t.Fatal(e)
	}
	t.Fatal("mutation barrier not reached")
}
func TestEntryProcessKillDurableBoundaries(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal fixture profile")
	}
	for _, stage := range []string{"before_initialize", "initialized", "before_prepare", "after_prepare", "before_apply", "after_apply", "before_abandon", "after_abandon"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestEntryCrashHelper$", "-test.timeout=20s")
			cmd.Env = append(os.Environ(), "SLUG_ENTRY_CRASH_ROOT="+dir, "SLUG_ENTRY_CRASH_STAGE="+stage)
			log, e := os.Create(filepath.Join(dir, "helper.log"))
			if e != nil {
				t.Fatal(e)
			}
			defer log.Close()
			cmd.Stdout = log
			cmd.Stderr = log
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			killed := false
			defer func() {
				if !killed {
					cmd.Process.Kill()
					<-done
				}
			}()
			var state crashState
			deadline := time.After(10 * time.Second)
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
		wait:
			for {
				select {
				case e := <-done:
					killed = true
					b, _ := os.ReadFile(filepath.Join(dir, "helper.log"))
					t.Fatalf("helper exited: %v %s", e, b)
				case <-deadline:
					t.Fatal("barrier timeout")
				case <-ticker.C:
					b, e := os.ReadFile(filepath.Join(dir, "barrier.json"))
					if e == nil && json.Unmarshal(b, &state) == nil {
						break wait
					}
				}
			}
			if e = cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			<-done
			killed = true
			source, e := registry.OpenManifestAuthority(filepath.Join(dir, "source"), state.Pin.StablePin.Source, false)
			if e != nil {
				t.Fatal(e)
			}
			defer source.Close()
			stable, e := gymstable.Open(filepath.Join(dir, "stable"), state.Pin.StablePin, source)
			if e != nil {
				t.Fatal(e)
			}
			defer stable.Close()
			s, e := Open(context.Background(), state.Path, state.Pin, stable)
			if stage == "before_initialize" {
				if e == nil {
					s.Close()
					t.Fatal("partial create adopted")
				}
				return
			}
			if e != nil {
				t.Fatal("reopen", e)
			}
			defer s.Close()
			count := uint32(0)
			pending := false
			revision := uint64(0)
			switch stage {
			case "after_prepare", "before_apply", "before_abandon":
				count = 1
				pending = true
			case "after_apply":
				count = 1
				revision = 1
			case "after_abandon":
				count = 1
			}
			if s.meta.OperationCount != count || (s.meta.PendingOperationID != "") != pending {
				t.Fatal("durability mismatch", s.meta)
			}
			session := entrySession(state.Pin.StablePin, 1, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			status, e := s.Status(context.Background(), session, state.Saturday)
			if e != nil || status.Revision != revision {
				t.Fatal(status, e)
			}
		})
	}
}
