// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// This controller is a conditional dispatch/pipe fixture. It does not certify
// an account, credential, HTTP page, wallet or ledger.
type dailyWireTestController struct {
	*remoteWireTestController
	readMu           sync.Mutex
	last             RemoteDailyPayoutJob
	begun, cancelled int
	closeFailure     error
}

func (c *dailyWireTestController) DailyWalletSnapshot(ctx context.Context) (RemotePoolWireSnapshot, error) {
	return c.WireSnapshot(ctx)
}
func (c *dailyWireTestController) BeginDailyPayoutRead(ctx context.Context, v string, seq, cap, before uint64) (RemoteDailyPayoutJob, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if ctx.Err() != nil {
		return RemoteDailyPayoutJob{}, ctx.Err()
	}
	if v != c.logged.View.ViewID || seq == 0 {
		return RemoteDailyPayoutJob{}, ErrDailyPayoutReadInvalid
	}
	if seq == c.last.Sequence {
		if cap != c.last.CaptureSequence || before != c.last.BeforeBatch {
			return RemoteDailyPayoutJob{}, ErrDailyPayoutReadInvalid
		}
		return c.last, nil
	}
	if seq != c.last.Sequence+1 {
		return RemoteDailyPayoutJob{}, ErrDailyPayoutReadInvalid
	}
	c.begun++
	c.last = RemoteDailyPayoutJob{Sequence: seq, CaptureSequence: cap, BeforeBatch: before, State: "working"}
	return c.last, nil
}
func (c *dailyWireTestController) DailyPayoutRead(ctx context.Context, v string, seq uint64) (RemoteDailyPayoutRead, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if ctx.Err() != nil {
		return RemoteDailyPayoutRead{}, ctx.Err()
	}
	if v != c.logged.View.ViewID || seq == 0 || seq != c.last.Sequence {
		return RemoteDailyPayoutRead{}, ErrDailyPayoutReadNotFound
	}
	return RemoteDailyPayoutRead{Job: c.last}, nil
}
func (c *dailyWireTestController) CancelDailyPayoutRead(ctx context.Context, v string, seq uint64) error {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if v != c.logged.View.ViewID || seq == 0 || seq != c.last.Sequence {
		return ErrDailyPayoutReadNotFound
	}
	if c.last.State == "working" {
		c.cancelled++
		c.last.State = "cancelled"
	}
	return nil
}
func (c *dailyWireTestController) Close() error {
	return errors.Join(c.remoteWireTestController.Close(), c.closeFailure)
}

type dailyWireConversation struct {
	input  *io.PipeWriter
	output *io.PipeReader
	reader *bufio.Reader
	done   chan error
	cancel context.CancelFunc
	config RecoverableRemotePoolConfig
	prior  *RemotePoolView
	id     uint64
	joined bool
}

func dailyWireStart(t *testing.T, c *dailyWireTestController, cfg RecoverableRemotePoolConfig) *dailyWireConversation {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	in, send := io.Pipe()
	receive, out := io.Pipe()
	x := &dailyWireConversation{input: send, output: receive, reader: bufio.NewReader(receive), done: make(chan error, 1), cancel: cancel, config: cfg}
	go func() { x.done <- ServeDailyWallet(ctx, in, out, c, cfg) }()
	t.Cleanup(func() {
		cancel()
		_ = send.Close()
		_ = receive.Close()
		if !x.joined {
			select {
			case <-x.done:
				x.joined = true
			case <-time.After(5 * time.Second):
				t.Error("fixture helper cleanup did not join")
			}
		}
	})
	return x
}
func (x *dailyWireConversation) call(t *testing.T, command string, seq, cap, before uint64) DailyWalletResponse {
	t.Helper()
	x.id++
	cid, e := RecoverableRemotePoolConfigID(x.config)
	if e != nil {
		t.Fatal(e)
	}
	q := DailyWalletRequest{Version: 1, Application: DailyWalletApplication, ID: x.id, Command: command, ConfigID: cid, Sequence: seq, CaptureSequence: cap, BeforeBatch: before}
	if x.prior != nil {
		q.ViewID = x.prior.ViewID
	}
	if command == "login_fixture" {
		q.Fixture = x.config.Base.ExpectedLocal.Recipient.Config.Fixture
	}
	raw, e := MarshalDailyWalletRequest(q, x.config)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.input.Write(append(raw, '\n')); e != nil {
		t.Fatal(e)
	}
	line, e := x.reader.ReadBytes('\n')
	if e != nil {
		t.Fatal(e)
	}
	out, e := ParseDailyWalletResponse(line[:len(line)-1], q, x.config, x.prior)
	if e != nil {
		t.Fatal(e)
	}
	if out.OK && out.Snapshot != nil {
		x.prior = out.Snapshot.View
	}
	return out
}
func (x *dailyWireConversation) join(t *testing.T) error {
	t.Helper()
	if e := x.input.Close(); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-x.done:
		x.joined = true
		x.cancel()
		_ = x.output.Close()
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("original helper did not join")
		return nil
	}
}
func TestDailyWalletServeOriginalSequenceAndExplicitStart(t *testing.T) {
	r := dailyWalletTestReference(t)
	initial, logged := dailyWalletTestSnapshots(t, r)
	running := remoteWireTestCopy(logged)
	running.Remote.State = "running"
	c := &dailyWireTestController{remoteWireTestController: &remoteWireTestController{initial: initial, logged: logged, running: running, stopped: logged, current: initial}}
	x := dailyWireStart(t, c, r.Selection.Remote)
	if out := x.call(t, "snapshot", 0, 0, 0); out.Snapshot.State != "signed_out" {
		t.Fatal("did not start signed out")
	}
	x.call(t, "login_fixture", 0, 0, 0)
	if c.starts != 0 {
		t.Fatal("login implicitly started mining")
	}
	for i := 0; i < 2; i++ {
		if out := x.call(t, "daily_payout_begin", 1, 0, 0); !out.OK || out.Job.State != "working" {
			t.Fatal("original retry", out)
		}
	}
	if c.begun != 1 || c.starts != 0 {
		t.Fatal("repeated Begin replaced read or mined")
	}
	if out := x.call(t, "daily_payout_read", 1, 0, 0); !out.OK || out.Read.Job.Sequence != 1 {
		t.Fatal("lost correlation")
	}
	if out := x.call(t, "daily_payout_cancel", 1, 0, 0); !out.OK || out.Read.Job.State != "cancelled" || c.cancelled != 1 {
		t.Fatal("cancel did not return joined original read")
	}
	if out := x.call(t, "daily_payout_read", 2, 0, 0); out.OK || out.Error != "not_found" {
		t.Fatal("adopted latest job")
	}
	x.call(t, "start", 0, 0, 0)
	x.call(t, "stop", 0, 0, 0)
	x.call(t, "logout", 0, 0, 0)
	if e := x.join(t); e != nil {
		t.Fatal(e)
	}
	if c.starts != 1 || c.stops != 1 || c.closes != 1 {
		t.Fatal("original lifecycle counts")
	}
}
func TestDailyWalletServeDuplicateFrameRetainsCleanupFailure(t *testing.T) {
	r := dailyWalletTestReference(t)
	initial, logged := dailyWalletTestSnapshots(t, r)
	marker := errors.New("conditional-original-close-failure")
	c := &dailyWireTestController{remoteWireTestController: &remoteWireTestController{initial: initial, logged: logged, current: initial}, closeFailure: marker}
	x := dailyWireStart(t, c, r.Selection.Remote)
	x.call(t, "snapshot", 0, 0, 0)
	q := DailyWalletRequest{Version: 1, Application: DailyWalletApplication, ID: 1, Command: "snapshot", ConfigID: r.Selection.ConfigID}
	raw, e := json.Marshal(q)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.input.Write(append(raw, '\n')); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-x.done:
		x.joined = true
		if !errors.Is(e, ErrDailyWalletProtocol) || !errors.Is(e, marker) {
			t.Fatal("protocol or original cleanup cause lost", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("duplicate did not join")
	}
	if c.closes != 1 || c.logins != 0 || c.starts != 0 || c.begun != 0 {
		t.Fatal("invalid frame had effects")
	}
}

type dailyWireEOFWriteFailure struct {
	input        *io.PipeWriter
	ownerClosing <-chan struct{}
	cause        error
}

func (w *dailyWireEOFWriteFailure) Write([]byte) (int, error) {
	// Consume the original stdin EOF before exposing this independent write
	// failure. No scheduler timing can decide whether the error is retained.
	if e := w.input.Close(); e != nil {
		return 0, e
	}
	<-w.ownerClosing
	return 0, w.cause
}
func (*dailyWireEOFWriteFailure) Close() error { return nil }
func TestDailyWalletServeEOFDoesNotEraseWriteFailure(t *testing.T) {
	r := dailyWalletTestReference(t)
	initial, logged := dailyWalletTestSnapshots(t, r)
	marker := errors.New("conditional-original-write-failure")
	c := &dailyWireTestController{remoteWireTestController: &remoteWireTestController{initial: initial, logged: logged, current: initial, closeEntered: make(chan struct{})}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	in, send := io.Pipe()
	out := &dailyWireEOFWriteFailure{input: send, ownerClosing: c.closeEntered, cause: marker}
	done := make(chan error, 1)
	go func() { done <- ServeDailyWallet(ctx, in, out, c, r.Selection.Remote) }()
	q := DailyWalletRequest{Version: 1, Application: DailyWalletApplication, ID: 1, Command: "snapshot", ConfigID: r.Selection.ConfigID}
	raw, e := MarshalDailyWalletRequest(q, r.Selection.Remote)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = send.Write(append(raw, '\n')); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if !errors.Is(e, marker) || !errors.Is(e, io.ErrShortWrite) {
			t.Fatal("EOF erased observed original write failure", e)
		}
	case <-ctx.Done():
		t.Fatal("original controller/write did not join")
	}
	if c.closes != 1 || c.starts != 0 {
		t.Fatal("unexpected owner lifecycle")
	}
}
