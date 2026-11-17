// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymprovider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
)

// Only public fixture routing/reconciliation metadata crosses the process
// boundary. No signed JWT, session token, password or wallet key is written.
type providerCrashState struct {
	Root              string
	Stage             string
	Config            Config
	IdentityConfig    gymaccount.IdentityConfig
	Flow              LoginFlow
	Request           ResetRequest
	Origin            string
	Operation         string
	AuthenticatedAtMS int64
	VerifiedAtMS      int64
}

func writeProviderCrashState(t *testing.T, path string, state providerCrashState) {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, we := f.Write(raw)
	if err := errors.Join(we, f.Sync(), f.Close()); err != nil {
		t.Fatal(err)
	}
}

func TestProviderCrashChild(t *testing.T) {
	path := os.Getenv("SLUG_TEST_PROVIDER_CRASH_STATE")
	if path == "" {
		t.Skip("subprocess only")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state providerCrashState
	if err = json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	v, err := gymidentity.NewVerifier(FixtureVerifierConfig())
	if err != nil {
		t.Fatal(err)
	}
	a, err := gymaccount.OpenIdentity(filepath.Join(state.Root, "authority"), state.IdentityConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	c, err := Open(context.Background(), filepath.Join(state.Root, "coordinator"), state.Config, a, v)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.checkpoint = func(stage string) {
		if stage != state.Stage {
			return
		}
		if stage == "reserve-login" || stage == "authority-issue-login" {
			it := c.db.NewIterator(util.BytesPrefix([]byte("login:")), nil)
			for it.Next() {
				var record loginRecord
				if json.Unmarshal(it.Value(), &record) != nil {
					it.Release()
					t.Fatal("invalid child journal")
				}
				if record.Phase == "reserved" {
					state.Flow = record.Flow
					break
				}
			}
			err := it.Error()
			it.Release()
			if err != nil {
				t.Fatal(err)
			}
			if state.Flow.ID == "" {
				t.Fatal("checkpoint missing reserved flow")
			}
		}
		writeProviderCrashState(t, path, state)
		if err := os.WriteFile(path+".ready", []byte(stage), 0600); err != nil {
			t.Fatal(err)
		}
		// Parent kills this process without executing either Close or any
		// deferred cleanup. A test timeout bounds unexpected parent failure.
		<-time.After(time.Hour)
	}
	ctx := context.Background()
	switch state.Stage {
	case "reserve-login", "authority-issue-login":
		_, err = c.IssueLogin(ctx)
	case "submitting-login", "authority-login":
		_, err = c.FinishLogin(ctx, state.Flow.ID, signLogin(t, state.Flow, "alice", state.Origin, state.Flow.ID, "fresh", 1, state.AuthenticatedAtMS))
	case "begin-prepared", "authority-begin":
		_, err = c.BeginReset(ctx, state.Request.EventID, signReset(t, state.Request, "alice", state.Operation, "begin", 1, 2, state.VerifiedAtMS))
	case "complete-prepared", "authority-complete":
		_, err = c.CompleteReset(ctx, state.Request.EventID, signReset(t, state.Request, "alice", state.Operation, "complete", 1, 2, state.VerifiedAtMS))
	default:
		t.Fatal("unknown checkpoint")
	}
	t.Fatalf("checkpoint was not reached: %v", err)
}

func killProviderAtCheckpoint(t *testing.T, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProviderCrashChild$", "-test.count=1", "-test.timeout=24s")
	command.Env = append(os.Environ(), "SLUG_TEST_PROVIDER_CRASH_STATE="+path)
	log, err := os.OpenFile(path+".child-log", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	command.Stdout = log
	command.Stderr = log
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	joined := false
	defer func() {
		if !joined {
			_ = command.Process.Kill()
			<-done
		}
	}()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			joined = true
			data, _ := os.ReadFile(path + ".child-log")
			t.Fatalf("child exited before checkpoint (%v): %s", err, data)
		case <-deadline.C:
			t.Fatal("child did not reach durable checkpoint")
		case <-tick.C:
			if _, err := os.Stat(path + ".ready"); err != nil {
				continue
			}
			if err := command.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			err := <-done
			joined = true
			if err == nil || command.ProcessState.Success() {
				t.Fatal("child was not abruptly terminated")
			}
			data, readErr := os.ReadFile(path + ".child-log")
			if readErr != nil {
				t.Fatal(readErr)
			}
			if strings.Contains(string(data), "WARNING: DATA RACE") || strings.Contains(string(data), "race detected") {
				t.Fatal("child reported a race before abrupt termination")
			}
			return
		}
	}
}

func inspectCrashedAuthority(t *testing.T, root string) (uint32, []gymaccount.ResetBarrier) {
	t.Helper()
	db, err := leveldb.OpenFile(filepath.Join(root, "authority", "db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw, err := db.Get([]byte("meta"), nil)
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		LoginAttempts uint32 `json:"login_attempts"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	var barriers []gymaccount.ResetBarrier
	it := db.NewIterator(util.BytesPrefix([]byte("barrier:")), nil)
	defer it.Release()
	for it.Next() {
		var b gymaccount.ResetBarrier
		if err := json.Unmarshal(it.Value(), &b); err != nil {
			t.Fatal(err)
		}
		barriers = append(barriers, b)
	}
	if err := it.Error(); err != nil {
		t.Fatal(err)
	}
	return meta.LoginAttempts, barriers
}

func TestProviderDurableBoundaryProcessKillRecovery(t *testing.T) {
	for _, stage := range []string{"reserve-login", "authority-issue-login", "submitting-login", "authority-login", "begin-prepared", "authority-begin", "complete-prepared", "authority-complete"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			f := newProviderFixture(t)
			state := providerCrashState{Root: f.Root, Stage: stage, Config: f.Config, IdentityConfig: f.IdentityConfig,
				Origin: strings.Repeat("a8", 32), Operation: strings.Repeat("b9", 32)}
			var err error
			if stage == "submitting-login" || stage == "authority-login" {
				state.Flow, err = f.Coordinator.IssueLogin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				state.AuthenticatedAtMS = state.Flow.IssuedAtMS / 1000 * 1000
			}
			if strings.Contains(stage, "begin") || strings.Contains(stage, "complete") {
				adversarialLogin(t, f)
				state.Request, err = f.Coordinator.IssueReset(ctx)
				if err != nil {
					t.Fatal(err)
				}
				state.VerifiedAtMS = time.Now().Unix() * 1000
				if strings.Contains(stage, "complete") {
					if _, err = f.Coordinator.BeginReset(ctx, state.Request.EventID, signReset(t, state.Request, "alice", state.Operation, "begin", 1, 2, state.VerifiedAtMS)); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err = f.Coordinator.Close(); err != nil {
				t.Fatal(err)
			}
			if err = f.Authority.Close(); err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(f.Root, "public-crash-state.json")
			writeProviderCrashState(t, statePath, state)
			killProviderAtCheckpoint(t, statePath)
			raw, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(raw, &state); err != nil {
				t.Fatal(err)
			}
			attemptCount, barriers := inspectCrashedAuthority(t, f.Root)
			f.Authority, err = gymaccount.OpenIdentity(filepath.Join(f.Root, "authority"), f.IdentityConfig)
			if err != nil {
				t.Fatal(err)
			}
			f.Coordinator, err = Open(ctx, filepath.Join(f.Root, "coordinator"), f.Config, f.Authority, f.Verifier)
			if err != nil {
				t.Fatal(err)
			}
			switch stage {
			case "reserve-login", "authority-issue-login":
				want := uint32(0)
				if stage == "authority-issue-login" {
					want = 1
				}
				if attemptCount != want {
					t.Fatalf("wrong orphan attempt count: got%d want%d", attemptCount, want)
				}
				status, err := f.Coordinator.Status(ctx, "login", state.Flow.ID)
				if err != nil || status.Phase != "unavailable" || !status.OutcomeUnknown {
					t.Fatalf("unacknowledged issue became usable: %v", err)
				}
				if session, err := f.Coordinator.FinishLogin(ctx, state.Flow.ID, ""); err == nil || session.Token != "" {
					t.Fatal("reserved orphan yielded session")
				}
				if f.Coordinator.meta.LoginFlows != 1 {
					t.Fatal("orphan reservation not counted")
				}
				next, err := f.Coordinator.IssueLogin(ctx)
				if err != nil || next.ID == state.Flow.ID || f.Coordinator.meta.LoginFlows != 2 {
					t.Fatalf("new issue reused abandoned reservation: %v", err)
				}
			case "submitting-login", "authority-login":
				token := signLogin(t, state.Flow, "alice", state.Origin, state.Flow.ID, "fresh", 1, state.AuthenticatedAtMS)
				session, err := f.Coordinator.FinishLogin(ctx, state.Flow.ID, token)
				if stage == "submitting-login" {
					if err != nil || session.Token == "" {
						t.Fatalf("unconsumed exact attempt did not resume: %v", err)
					}
				} else {
					if !errors.Is(err, ErrOutcomeUnknown) || session.Token != "" {
						t.Fatalf("consumed session was reconstructed: %v", err)
					}
					flow, err := f.Coordinator.IssueLogin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if session, err := f.Coordinator.FinishLogin(ctx, flow.ID, signLogin(t, flow, "alice", state.Origin, state.Flow.ID, "reuse", 1, state.AuthenticatedAtMS)); err == nil || session.Token != "" {
						t.Fatal("uncertain origin was promoted to accepted reuse")
					}
				}
			default:
				if stage == "begin-prepared" && len(barriers) != 0 {
					t.Fatal("barrier existed before authority call")
				}
				if stage != "begin-prepared" && len(barriers) != 1 {
					t.Fatal("acknowledged authority barrier lost")
				}
				begin := signReset(t, state.Request, "alice", state.Operation, "begin", 1, 2, state.VerifiedAtMS)
				if strings.Contains(stage, "begin") {
					pending, err := f.Coordinator.BeginReset(ctx, state.Request.EventID, begin)
					if err != nil || !pending.CanCommit || pending.Phase != "pending" {
						t.Fatalf("pending begin could not reconcile: %v", err)
					}
					if len(barriers) == 1 && pending.Barrier != barriers[0] {
						t.Fatal("begin recovery changed original barrier")
					}
				}
				complete := signReset(t, state.Request, "alice", state.Operation, "complete", 1, 2, state.VerifiedAtMS)
				done, err := f.Coordinator.CompleteReset(ctx, state.Request.EventID, complete)
				if err != nil || done.CanCommit || done.Phase != "completed" {
					t.Fatalf("completion could not reconcile: %v", err)
				}
				if stage == "authority-complete" && done.Barrier != barriers[0] {
					t.Fatal("completion recovery extended hold or changed receipt")
				}
				if len(barriers) == 1 && (done.Barrier.BeganAtMS != barriers[0].BeganAtMS || done.Barrier.BeginSequence != barriers[0].BeginSequence || done.Barrier.Generation != barriers[0].Generation) {
					t.Fatal("completion changed original barrier identity")
				}
				historical, err := f.Coordinator.BeginReset(ctx, state.Request.EventID, begin)
				if err != nil || historical.CanCommit || historical.Barrier != done.Barrier {
					t.Fatalf("completed begin replay granted another permit: %v", err)
				}
			}
		})
	}
}
