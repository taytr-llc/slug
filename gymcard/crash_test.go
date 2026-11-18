//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func cardCrashHook(t *testing.T) func(string) {
	stage := os.Getenv("SLUG_CARD_KILL_STAGE")
	if stage == "" {
		return nil
	}
	return func(got string) {
		if got != stage {
			return
		}
		if e := os.WriteFile(filepath.Join(os.Getenv("SLUG_CARD_TEST_ROOT"), "barrier"), []byte(got), 0600); e != nil {
			t.Fatal(e)
		}
		for {
			time.Sleep(time.Second)
		}
	}
}
func TestCardCrashChild(t *testing.T) {
	if os.Getenv("SLUG_CARD_KILL_STAGE") == "" {
		t.Skip("owned subprocess only")
	}
	f := newCardFixture(t)
	f.s.checkpoint = cardCrashHook(t)
	if _, e := f.build(t); e != nil {
		t.Fatal(e)
	}
	t.Fatal("barrier not reached")
}
func TestCardActualKillAndPreparedRecovery(t *testing.T) {
	requireNormal(t)
	for _, stage := range []string{"marker-synced", "init-synced", "initialized", "before-card-sync", "card-synced"} {
		t.Run(stage, func(t *testing.T) {
			dir := realTemp(t)
			cmd := exec.Command(os.Args[0], "-test.run=^TestCardCrashChild$", "-test.count=1", "-test.timeout=45s")
			cmd.Env = append(os.Environ(), "SLUG_CARD_TEST_ROOT="+dir, "SLUG_CARD_KILL_STAGE="+stage)
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
			joined := false
			t.Cleanup(func() {
				if !joined {
					cmd.Process.Kill()
					cmd.Wait()
				}
			})
			deadline := time.Now().Add(40 * time.Second)
			for {
				b, e := os.ReadFile(filepath.Join(dir, "barrier"))
				if e == nil && string(b) == stage {
					break
				}
				if time.Now().After(deadline) {
					b, _ := os.ReadFile(filepath.Join(dir, "child.log"))
					t.Fatalf("barrier timeout %s", b)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if e = cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			e = cmd.Wait()
			joined = true
			var ee *exec.ExitError
			if !errors.As(e, &ee) {
				t.Fatal("kill result", e)
			}
			status, ok := ee.Sys().(syscall.WaitStatus)
			if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
				t.Fatal("not SIGKILL", e)
			}
			ep := eventPin(t)
			ev, _ := gymevent.EventForDate(ep, "2026-11-21")
			clock := func() time.Time { return time.UnixMilli(ev.CutoffMS + 1001) }
			events, e := gymevent.OpenForTestClock(context.Background(), filepath.Join(dir, "events"), ep, clock)
			if e != nil {
				t.Fatal(e)
			}
			defer events.Close()
			sp, e := gymseed.CandidatePin(ep)
			if e != nil {
				t.Fatal(e)
			}
			seeds, e := gymseed.OpenForTestClock(context.Background(), filepath.Join(dir, "seeds"), sp, events, clock)
			if e != nil {
				t.Fatal(e)
			}
			defer seeds.Close()
			p, _ := CandidatePin(sp)
			path := filepath.Join(dir, "cards")
			store, e := open(context.Background(), path, p, seeds, clock, false)
			partial := stage == "marker-synced" || stage == "init-synced"
			if partial {
				if e == nil {
					store.Close()
					t.Fatal("ready Open adopted partial")
				}
				store, e = open(context.Background(), path, p, seeds, clock, true)
			}
			if e != nil {
				t.Fatal("recovery", e)
			}
			info, e := store.Info(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			expected := uint32(0)
			if stage == "card-synced" {
				expected = 1
			}
			if info.EventCount != expected {
				t.Fatal("atomic row count", info.EventCount, expected)
			}
			created := info.CreatedAtMS
			store.Close()
			if expected == 1 {
				if s, e := open(context.Background(), path, p, seeds, clock, true); e == nil {
					s.Close()
					t.Fatal("prepared adopted populated store")
				}
			} else {
				s, e := open(context.Background(), path, p, seeds, clock, true)
				if e != nil {
					t.Fatal(e)
				}
				again, _ := s.Info(context.Background())
				s.Close()
				if again.CreatedAtMS != created {
					t.Fatal("creation reset")
				}
			}
		})
	}
}
