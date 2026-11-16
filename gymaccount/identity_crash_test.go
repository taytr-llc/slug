// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdentityCrashAfterResetStages(t *testing.T) {
	const pathEnv = "SLUG_IDENTITY_PUBLIC_FIXTURE_CRASH_PATH"
	const stageEnv = "SLUG_IDENTITY_PUBLIC_FIXTURE_CRASH_STAGE"
	if path := os.Getenv(pathEnv); path != "" {
		s, err := CreateIdentity(path, identityTestConfig())
		if err != nil {
			t.Fatal(err)
		}
		clock := new(atomic.Int64)
		clock.Store(1700000000000)
		s.now = func() time.Time { return time.UnixMilli(clock.Load()) }
		session := identityTestLogin(t, s, clock, "alice", 1)
		accountTestEnroll(t, s, session, "alice", "enroll")
		reset := identityTestReset(clock, "alice", "c", "begin", 1)
		if _, _, err = s.BeginReset(context.Background(), reset); err != nil {
			t.Fatal(err)
		}
		if os.Getenv(stageEnv) == "complete" {
			if _, _, err = s.CompleteReset(context.Background(), identityRefreshReset(reset, clock, "complete")); err != nil {
				t.Fatal(err)
			}
		}
		fmt.Println("IDENTITY_STAGE_SYNCED")
		for {
			time.Sleep(time.Second)
		} // Parent SIGKILL; no orderly Close runs.
	}
	for _, stage := range []string{"begin", "complete"} {
		t.Run(stage, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "authority")
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestIdentityCrashAfterResetStages$")
			cmd.Env = append(os.Environ(), pathEnv+"="+path, stageEnv+"="+stage)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { cmd.Process.Kill(); cmd.Wait() }()
			ack := make(chan bool, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					if scanner.Text() == "IDENTITY_STAGE_SYNCED" {
						ack <- true
						return
					}
				}
				ack <- false
			}()
			select {
			case ok := <-ack:
				if !ok {
					t.Fatal("child exited before durable stage")
				}
			case <-ctx.Done():
				t.Fatal("child stage timeout")
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err = cmd.Wait(); err == nil {
				t.Fatal("child did not crash")
			}
			s, err := OpenIdentity(path, identityTestConfig())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			clock := new(atomic.Int64)
			clock.Store(1700000000000)
			s.now = func() time.Time { return time.UnixMilli(clock.Load()) }
			reset := identityTestReset(clock, "alice", "c", "begin", 1)
			retained, newBegin, err := s.BeginReset(context.Background(), reset)
			if err != nil || newBegin || retained.Generation != 2 {
				t.Fatal("begin acknowledgment lost", err)
			}
			if stage == "begin" {
				if retained.Status != "pending" {
					t.Fatal("crash released pending")
				}
				a, p := identityTestAttempt(t, s, clock, "alice", 2)
				if _, err = s.LoginWithAssertion(context.Background(), a.ID, p); err == nil {
					t.Fatal("pending crash allowed login")
				}
			}
			result, newComplete, err := s.CompleteReset(context.Background(), identityRefreshReset(reset, clock, "complete"))
			if err != nil || newComplete != (stage == "begin") || result.Generation != 2 || result.HoldUntilMS != clock.Load()+RecoveryHoldMS {
				t.Fatal("completion not reconciled", err)
			}
			login := identityTestLogin(t, s, clock, "alice", 2)
			a := accountTestRead(t, s, login.Token)
			expected, _ := FixtureIdentity("zero256")
			if a.Enrollment == nil || *a.Enrollment != expected || !a.RecoveryRequired {
				t.Fatal("crash lost original wallet binding")
			}
		})
	}
}
