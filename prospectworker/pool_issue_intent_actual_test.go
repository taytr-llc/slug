//go:build slugissuerecoveryactual && !slugtestpow && (linux || darwin)

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolservice"
)

// L is an opt-in process durability control, not the separate real issuer gap
// or scheduled-payout acceptance. It deliberately SIGKILLs one exact child after
// the actual sync checkpoint. The synthetic root is retained on every outcome.
type issueIntentAck struct {
	Version   uint32 `json:"version"`
	PID       int    `json:"pid"`
	RequestID string `json:"request_id"`
	State     string `json:"state"`
}
type issueIntentSelection struct {
	Client poolservice.RecoverableDailyClientConfig `json:"client"`
	Worker RecoverablePoolConfig                    `json:"worker"`
}
type issueIntentCapture struct {
	mu       sync.Mutex
	b        []byte
	overflow bool
}

func (b *issueIntentCapture) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := 8192 - len(b.b)
	if len(p) > remaining {
		b.b = append(b.b, p[:remaining]...)
		b.overflow = true
	} else {
		b.b = append(b.b, p...)
	}
	return len(p), nil
}

func TestRecoverablePoolIntentCrashChild(t *testing.T) {
	if os.Getenv("SLUG_ISSUE_INTENT_CHILD") != "1" {
		t.Skip("parent-owned child only")
	}
	root := os.Getenv("SLUG_ISSUE_INTENT_ROOT")
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		t.Fatal("selected fixture root")
	}
	raw, e := os.ReadFile(filepath.Join(root, "public-selection.json"))
	if e != nil || len(raw) > 512<<10 {
		t.Fatal("public selection", e)
	}
	var selected issueIntentSelection
	if json.Unmarshal(raw, &selected) != nil {
		t.Fatal("public selection grammar")
	}
	canonical, _ := json.Marshal(selected)
	if !bytes.Equal(raw, canonical) {
		t.Fatal("public selection canonicality")
	}
	client, e := poolservice.NewRecoverableDailyClient(selected.Client)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	box, e := CreateRecoverableDailyPoolOutbox(filepath.Join(root, "outbox"), selected.Worker)
	if e != nil {
		t.Fatal(e)
	}
	defer issueClose(t, box)
	ack := os.NewFile(3, "original-intent-checkpoint")
	if ack == nil {
		t.Fatal("missing inherited acknowledgement descriptor")
	}
	defer func() {
		if e := ack.Close(); e != nil {
			t.Error(e)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var once sync.Once
	box.checkpoint = func(phase string) {
		if phase != "commit" || box.meta.ActiveRequestID == "" {
			return
		}
		once.Do(func() {
			id := box.meta.ActiveRequestID
			raw, e := box.db.Get(poolRequestKey(id), nil)
			q, decode := box.decodeIssueRow(raw, id)
			if e != nil || decode != nil || q.Result != nil || box.meta.Requests != 1 || box.meta.Records != 0 {
				t.Error("checkpoint is not original synced null request")
				cancel()
				return
			}
			message, _ := json.Marshal(issueIntentAck{1, os.Getpid(), id, "synced-null-before-dispatch"})
			message = append(message, '\n')
			if len(message) > 512 {
				t.Error("acknowledgement bound")
				cancel()
				return
			}
			n, e := ack.Write(message)
			if e != nil || n != len(message) {
				t.Error("acknowledgement write", e)
				cancel()
				return
			}
			<-ctx.Done() // Root-owned parent must kill/reap this exact process here.
		})
	}
	worker, e := NewRecoverableDailyPool(selected.Worker, client, client, &pwAdmission{c: selected.Worker.Base}, &pwNode{}, issueNoSign, box)
	if e != nil {
		t.Fatal(e)
	}
	if e = worker.Run(ctx); e != nil {
		t.Fatal(e)
	}
	t.Fatal("child returned instead of the selected intentional kill")
}

func TestRecoverablePoolActualIntentCrashBeforeDispatch(t *testing.T) {
	if os.Getenv("SLUG_ISSUE_INTENT_ACTUAL") != "1" {
		t.Skip("explicit process durability acceptance only")
	}
	root := os.Getenv("SLUG_ISSUE_INTENT_ROOT")
	parent := filepath.Dir(root)
	canonical, e := filepath.EvalSymlinks(parent)
	if e != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root || canonical != parent {
		t.Fatal("fresh canonical selected root required", e)
	}
	if e = os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	var samples atomic.Int32
	var mu sync.Mutex
	var requestIDs []string
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, in *http.Request) {
		samples.Add(1)
		var request poolIssueRequest
		if json.NewDecoder(in.Body).Decode(&request) != nil {
			t.Error("request")
		}
		mu.Lock()
		requestIDs = append(requestIDs, request.RequestID)
		mu.Unlock()
		out.Header().Set("Content-Type", "application/json")
		out.WriteHeader(503)
		_, _ = out.Write([]byte("{\"version\":3,\"error\":{\"code\":\"outcome_unknown\",\"outcome_unknown\":true}}\n"))
	}))
	defer server.Close()
	oracle := issueOracleRead(t)
	tc := oracle.Fixtures.Transport[2]
	tc.Client.Base.URL = server.URL
	tc.Client.Base.CACertificate = ""
	selected, client := issueTestConfig(t, tc.Client, tc.Worker.Base)
	configRaw, _ := json.Marshal(issueIntentSelection{tc.Client, selected})
	if e = os.WriteFile(filepath.Join(root, "public-selection.json"), configRaw, 0600); e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	reader, writer, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := reader.Close(); e != nil {
			t.Error(e)
		}
	}()
	cmd := exec.Command(exe, "-test.run=^TestRecoverablePoolIntentCrashChild$", "-test.timeout=25s")
	cmd.Env = append(os.Environ(), "SLUG_ISSUE_INTENT_CHILD=1", "SLUG_ISSUE_INTENT_ROOT="+root)
	cmd.ExtraFiles = []*os.File{writer}
	var output, stderr issueIntentCapture
	cmd.Stdout = &output
	cmd.Stderr = &stderr
	if e = cmd.Start(); e != nil {
		_ = writer.Close()
		t.Fatal(e)
	}
	joined := false
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	// No new process group: the root qualifier still owns every descendant.
	pgid, pe := syscall.Getpgid(cmd.Process.Pid)
	parentPGID := syscall.Getpgrp()
	defer func() {
		if !joined {
			_ = cmd.Process.Kill()
			select {
			case <-wait:
				joined = true
			case <-time.After(5 * time.Second):
				t.Error("original child did not join; retained root", root)
			}
		}
	}()
	if e = writer.Close(); e != nil {
		t.Fatal(e)
	}
	if pe != nil || pgid != parentPGID {
		t.Fatal("child process group changed", pe)
	}
	type ackRead struct {
		raw []byte
		err error
	}
	received := make(chan ackRead, 1)
	go func() {
		raw, e := bufio.NewReader(io.LimitReader(reader, 1025)).ReadBytes('\n')
		received <- ackRead{raw, e}
	}()
	var ack issueIntentAck
	var ackRaw []byte
	select {
	case result := <-received:
		ackRaw = result.raw
		if result.err != nil || len(ackRaw) > 512 || json.Unmarshal(bytes.TrimSuffix(ackRaw, []byte{'\n'}), &ack) != nil {
			t.Fatal("checkpoint acknowledgement", result.err)
		}
	case <-time.After(10 * time.Second):
		if e := reader.Close(); e != nil {
			t.Error(e)
		}
		<-received
		t.Fatal("original checkpoint timed out")
	}
	normalized, _ := json.Marshal(ack)
	if !bytes.Equal(ackRaw, append(normalized, '\n')) || ack.Version != 1 || ack.PID != cmd.Process.Pid || !poolRequestID(ack.RequestID) || ack.State != "synced-null-before-dispatch" || samples.Load() != 0 {
		t.Fatal("not the exact pre-dispatch checkpoint")
	}
	if e = cmd.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	var exit error
	select {
	case exit = <-wait:
		joined = true
	case <-time.After(5 * time.Second):
		t.Fatal("intentional kill did not reap")
	}
	var killed *exec.ExitError
	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !errors.As(exit, &killed) || !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL || samples.Load() != 0 || output.overflow || stderr.overflow {
		t.Fatal("incorrect crash/join evidence", exit)
	}
	path := filepath.Join(root, "outbox")
	box, e := OpenRecoverableDailyPoolOutbox(path, selected)
	if e != nil {
		t.Fatal(e)
	}
	defer issueClose(t, box)
	if box.meta.Requests != 1 || box.meta.ActiveRequestID != ack.RequestID || box.meta.Records != 0 || box.cachedScan != nil {
		t.Fatal("killed original intent not recovered")
	}
	before, e := box.db.Get(poolRequestKey(ack.RequestID), nil)
	if e != nil {
		t.Fatal(e)
	}
	q, e := box.decodeIssueRow(before, ack.RequestID)
	if e != nil || q.Result != nil {
		t.Fatal("crash request changed", e)
	}
	if samples.Load() != 0 {
		t.Fatal("Open dispatched")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker, e := NewRecoverableDailyPool(selected, client, client, &pwAdmission{c: selected.Base}, &pwNode{}, issueNoSign, box)
	if e != nil {
		t.Fatal(e)
	}
	worker.wait = func(context.Context, time.Duration) error { cancel(); return ctx.Err() }
	if e = worker.Run(ctx); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	exact := len(requestIDs) == 1 && requestIDs[0] == ack.RequestID
	mu.Unlock()
	after, e := box.db.Get(poolRequestKey(ack.RequestID), nil)
	if e != nil || !exact || samples.Load() != 1 || !bytes.Equal(before, after) || worker.Status().Attempts != 0 || box.meta.ActiveRequestID != ack.RequestID {
		t.Fatal("explicit Run did not resume exact retained ID", e)
	}
	if e = box.Close(); e != nil {
		t.Fatal(e)
	}
	report := struct {
		Version         uint32 `json:"version"`
		ParentPID       int    `json:"parent_pid"`
		ChildPID        int    `json:"child_pid"`
		PGID            int    `json:"pgid"`
		IntentionalKill bool   `json:"intentional_kill"`
		CleanClose      bool   `json:"clean_close"`
		Joined          bool   `json:"joined"`
		Signal          string `json:"signal"`
		RequestID       string `json:"request_id"`
		Before          uint32 `json:"requests_before_crash"`
		After           uint32 `json:"requests_after_explicit_run"`
		Exact           bool   `json:"exact_reopen_resume"`
	}{1, os.Getpid(), cmd.Process.Pid, pgid, true, false, joined, status.Signal().String(), ack.RequestID, 0, 1, true}
	raw, _ := json.Marshal(report)
	if e = os.WriteFile(filepath.Join(root, "intent-crash-report.json"), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(output.b), "PASS") {
		t.Fatal("killed child was relabeled a successful Close")
	}
}
