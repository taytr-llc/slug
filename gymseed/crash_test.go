//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymevent"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSeedCrashChild(t *testing.T) {
	stage := os.Getenv("SLUG_SEED_KILL_STAGE")
	if stage == "" {
		t.Skip("owned subprocess only")
	}
	x := newEventFixture(t, true)
	p, e := CandidatePin(x.p)
	if e != nil {
		t.Fatal(e)
	}
	hook := func(got string) {
		if got == stage {
			if e := os.WriteFile(filepath.Join(filepath.Dir(x.path), "barrier"), []byte(got), 0600); e != nil {
				t.Fatal(e)
			}
			for {
				time.Sleep(time.Second)
			}
		}
	}
	s, e := create(context.Background(), filepath.Join(filepath.Dir(x.path), "seeds"), p, x.s, func() time.Time { return time.UnixMilli(x.now) }, hook)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	x.now = x.cutoff(t) + 1000
	for i := range x.entry {
		if _, e = x.s.Capture(context.Background(), x.entry[i], x.session[i], "2026-11-21"); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = s.Prepare(context.Background(), x.s, x.entry[0], x.session[0], "2026-11-21"); e != nil {
		t.Fatal(e)
	}
	t.Fatal("requested barrier was not reached")
}
func TestSeedActualProcessKillInitializationAndPreparedRecord(t *testing.T) {
	requireNormal(t)
	for _, stage := range []string{"marker-synced", "init-synced", "initialized", "before-prepare", "prepared"} {
		t.Run(stage, func(t *testing.T) {
			root := realTemp(t)
			cmd := exec.Command(os.Args[0], "-test.run=^TestSeedCrashChild$", "-test.count=1", "-test.timeout=50s")
			cmd.Env = append(os.Environ(), "SLUG_SEED_TEST_ROOT="+root, "SLUG_SEED_KILL_STAGE="+stage)
			log, e := os.Create(filepath.Join(root, "child.log"))
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
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			})
			deadline := time.Now().Add(40 * time.Second)
			for {
				raw, e := os.ReadFile(filepath.Join(root, "barrier"))
				if e == nil && string(raw) == stage {
					break
				}
				if time.Now().After(deadline) {
					raw, _ := os.ReadFile(filepath.Join(root, "child.log"))
					t.Fatalf("barrier timeout: %s", raw)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if e = cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			e = cmd.Wait()
			joined = true
			var exit *exec.ExitError
			if !errors.As(e, &exit) {
				t.Fatal("expected actual kill", e)
			}
			wait, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !wait.Signaled() || wait.Signal() != syscall.SIGKILL {
				t.Fatal("not SIGKILL", e)
			}
			ep := eventPin(t)
			event, _ := gymevent.EventForDate(ep, "2026-11-21")
			now := int64(1795000000100)
			if stage == "before-prepare" || stage == "prepared" {
				now = event.CutoffMS + 2000
			}
			events, e := gymevent.OpenForTestClock(context.Background(), filepath.Join(root, "events"), ep, func() time.Time { return time.UnixMilli(now) })
			if e != nil {
				t.Fatal(e)
			}
			defer events.Close()
			p, _ := CandidatePin(ep)
			path := filepath.Join(root, "seeds")
			var s *Store
			if stage == "marker-synced" || stage == "init-synced" {
				if s, e = OpenForTestClock(context.Background(), path, p, events, func() time.Time { return time.UnixMilli(now) }); e == nil {
					s.Close()
					t.Fatal("Open completed partial")
				}
				s, e = ResumePreparedForTestClock(context.Background(), path, p, events, func() time.Time { return time.UnixMilli(now) })
			} else {
				s, e = OpenForTestClock(context.Background(), path, p, events, func() time.Time { return time.UnixMilli(now) })
			}
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			info, e := s.Info(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			want := uint32(0)
			if stage == "prepared" {
				want = 1
			}
			if info.EventCount != want {
				t.Fatal("partial record count", info)
			}
			if stage == "prepared" {
				r, exists, e := s.record(eventKey(p, event))
				if e != nil || !exists || r.PreparedAtMS != event.CutoffMS+1000 {
					t.Fatal("retained prepare", e)
				}
				s.Close()
				if reopened, e := ResumePreparedForTestClock(context.Background(), path, p, events, func() time.Time { return time.UnixMilli(now) }); e == nil {
					reopened.Close()
					t.Fatal("event-bearing prepared adoption")
				}
			}
			t.Log("actual SIGKILL and joined recovery:", stage)
		})
	}
}
