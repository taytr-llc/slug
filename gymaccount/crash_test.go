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

// TestStoreCrashAfterReset proves recovery from an actual killed process after
// the synchronized acknowledgment, without relying on orderly LevelDB Close.
func TestStoreCrashAfterReset(t *testing.T) {
	const environment = "SLUG_ACCOUNT_PUBLIC_FIXTURE_CRASH"
	if path := os.Getenv(environment); path != "" {
		s, err := Create(path, accountTestConfig())
		if err != nil {
			t.Fatal(err)
		}
		clock := new(atomic.Int64)
		clock.Store(1700000000000)
		s.now = func() time.Time { return time.UnixMilli(clock.Load()) }
		login := accountTestLogin(t, s, "alice", clock)
		accountTestEnroll(t, s, login, "alice", "enroll")
		principal := accountTestPrincipal("alice", clock)
		principal.RecoveryVerified = true
		principal.RecoveryEventID = "crash-ack-fixture"
		if _, _, err = s.ApplyReset(context.Background(), principal); err != nil {
			t.Fatal(err)
		}
		fmt.Println("ACCOUNT_RESET_SYNCED")
		// Parent terminates this process; no deferred Close or test cleanup runs.
		for {
			time.Sleep(time.Second)
		}
	}
	path := filepath.Join(t.TempDir(), "accounts")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStoreCrashAfterReset$")
	cmd.Env = append(os.Environ(), environment+"="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	acknowledged := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if scanner.Text() == "ACCOUNT_RESET_SYNCED" {
				acknowledged <- true
				return
			}
		}
		acknowledged <- false
	}()
	select {
	case ok := <-acknowledged:
		if !ok {
			t.Fatal("child exited before synchronized reset acknowledgment")
		}
	case <-ctx.Done():
		t.Fatal("child reset timed out")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err == nil {
		t.Fatal("child unexpectedly exited cleanly")
	}
	s, err := Open(path, accountTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	clock := new(atomic.Int64)
	clock.Store(1700000000000)
	s.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	principal := accountTestPrincipal("alice", clock)
	principal.RecoveryVerified = true
	principal.RecoveryEventID = "crash-ack-fixture"
	reset, inserted, err := s.ApplyReset(context.Background(), principal)
	if err != nil || inserted || reset.Generation != 2 || reset.HoldUntilMS != clock.Load()+RecoveryHoldMS {
		t.Fatalf("crash reset replay: inserted=%v err=%v", inserted, err)
	}
	session := accountTestLogin(t, s, "alice", clock)
	account := accountTestRead(t, s, session.Token)
	expected, _ := FixtureIdentity("zero256")
	if account.Enrollment == nil || *account.Enrollment != expected || !account.RecoveryRequired || account.Generation != 2 {
		t.Fatal("crash recovery lost original binding or security state")
	}
}
