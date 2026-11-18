//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestCareerCrashChild(t *testing.T) {
	target := os.Getenv("SLUG_CAREER_CRASH_STAGE")
	if target == "" {
		t.Skip("owned subprocess only")
	}
	f := newResultFixture(t)
	f.s.Close()
	occurrence, _ := strconv.Atoi(os.Getenv("SLUG_CAREER_CRASH_OCCURRENCE"))
	seen := 0
	_, e := openCareer(context.Background(), filepath.Join(f.dir, "result"), f.pin, f.bout, f.clock, "activate", func(stage string) {
		if stage == target {
			seen++
			if seen == occurrence {
				if e := os.WriteFile(filepath.Join(f.dir, "career-checkpoint"), []byte(stage), 0600); e != nil {
					panic(e)
				}
				select {}
			}
		}
	})
	t.Fatal("activation kill point not reached", e)
}
func TestCareerActivationActualSIGKILL(t *testing.T) {
	cuts := []struct {
		stage      string
		occurrence int
	}{{"career-intent-synced", 1}, {"career-marker-temp-created", 1}, {"career-marker-temp-synced", 1}, {"career-marker-renamed", 1}, {"career-marker-dirsynced", 1}, {"career-metadata-synced", 1}, {"career-marker-temp-created", 2}, {"career-marker-temp-synced", 2}, {"career-marker-renamed", 2}, {"career-ready", 1}}
	for _, cut := range cuts {
		t.Run(cut.stage+"-"+strconv.Itoa(cut.occurrence), func(t *testing.T) {
			dir, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCareerCrashChild$", "-test.timeout=18s")
			cmd.Env = append(os.Environ(), "SLUG_RESULT_CRASH_ROOT="+dir, "SLUG_CAREER_CRASH_STAGE="+cut.stage, "SLUG_CAREER_CRASH_OCCURRENCE="+strconv.Itoa(cut.occurrence))
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
					cmd.Process.Kill()
					<-wait
				}
			}()
			for {
				if b, e := os.ReadFile(filepath.Join(dir, "career-checkpoint")); e == nil && string(b) == cut.stage {
					break
				}
				select {
				case e := <-wait:
					joined = true
					b, _ := os.ReadFile(filepath.Join(dir, "child.log"))
					t.Fatalf("child exited: %v %s", e, b)
				case <-ctx.Done():
					t.Fatal("checkpoint deadline")
				case <-time.After(10 * time.Millisecond):
				}
			}
			if e = cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			e = <-wait
			joined = true
			var exit *exec.ExitError
			if !errors.As(e, &exit) {
				t.Fatal("not killed", e)
			}
			st, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !st.Signaled() || st.Signal() != syscall.SIGKILL {
				t.Fatal("not SIGKILL", e)
			}
			// Exact source incarnation in separately opened concrete stores; no chain or
			// account authentication claim is inferred from this journal durability test.
			f := newResultFixture(t)
			f.s.Close()
			f.now.Add(1000)
			path := filepath.Join(dir, "result")
			if old, e := OpenForTestClock(context.Background(), path, f.pin, f.bout, f.clock); old != nil || e == nil {
				if old != nil {
					old.Close()
				}
				t.Fatal("old writer crossed barrier")
			}
			f.s, e = ResumeCareerActivationForTestClock(context.Background(), path, f.pin, f.bout, f.clock)
			if e != nil {
				t.Fatal("recovery", e)
			}
			a, e := f.s.CareerInfo(context.Background())
			if e != nil || a.ResultCreatedAtMS >= f.now.Load() {
				t.Fatal("original identity", e)
			}
			if cut.stage == "career-metadata-synced" || cut.occurrence == 2 || cut.stage == "career-ready" {
				if a.ActivatedAtMS >= f.now.Load() {
					t.Fatal("committed activation resampled")
				}
			} else if a.ActivatedAtMS != f.now.Load() {
				t.Fatal("uncommitted activation backdated")
			}
			f.s.Close()
			f.s, e = OpenCareerForTestClock(context.Background(), path, f.pin, f.bout, f.clock)
			if e != nil {
				t.Fatal(e)
			}
			again, _ := f.s.CareerInfo(context.Background())
			if a != again {
				t.Fatal("ready changed activation")
			}
			t.Log("SIGKILL joined and exact activation resumed")
		})
	}
}
