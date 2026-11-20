//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
)

func remoteBridgeCloseTest(t *testing.T, b *RemotePoolBridge) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := b.Close(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case <-b.readDone:
	default:
		t.Fatal("reader not joined")
	}
	select {
	case <-b.waitDone:
	default:
		t.Fatal("process not joined")
	}
	if b.cmd != nil && (b.cmd.ProcessState == nil || !b.cmd.ProcessState.Exited()) {
		t.Fatal("process not reaped")
	}
}

func TestRemotePoolBridgeOriginalProcessViewAndFiveCommands(t *testing.T) {
	for _, fixture := range []string{"zero256-default", "incrementing256-default"} {
		t.Run(fixture, func(t *testing.T) {
			cmd, config := remoteBridgeFixtureCommand(t, fixture, "normal")
			open, cancelOpen := context.WithCancel(context.Background())
			b, e := startRemotePoolCommand(open, cmd, config)
			cancelOpen()
			if b != nil {
				t.Cleanup(func() { remoteBridgeCloseTest(t, b) })
			}
			if e != nil {
				t.Fatal(e)
			}
			config.Miners.Enrollments[0].GymID = strings.Repeat("f", 64)
			config.CACertificate = "caller mutation"
			out, e := b.Snapshot(context.Background(), nil)
			if e != nil || out.State != "signed_out" {
				t.Fatal("launch context/config aliased", e)
			}
			login, cancelLogin := context.WithCancel(context.Background())
			out, e = b.LoginFixture(login, b.config.ExpectedLocal.Recipient.Config.Fixture)
			cancelLogin()
			if e != nil || out.Remote == nil || out.Remote.State != "stopped" {
				t.Fatal(e)
			}
			v, e := b.Context()
			if e != nil {
				t.Fatal(e)
			}
			want := v.View()
			out.View.GymID = strings.Repeat("e", 64)
			out.Remote.View.ViewID = strings.Repeat("d", 64)
			if v.View() != want || !b.ContextMatches(v) {
				t.Fatal("response alias changed original context")
			}
			copy := *v
			other := &RemotePoolContext{helper: &RemotePoolBridge{}, view: want}
			before := b.nextID
			for _, bad := range []*RemotePoolContext{nil, &copy, other} {
				if _, e = b.Start(context.Background(), bad); e == nil || b.nextID != before {
					t.Fatal("manufactured view dispatched", e)
				}
			}
			if out, e = b.Start(context.Background(), v); e != nil || out.View == nil || *out.View != want {
				t.Fatal("original Start", e)
			}
			if out, e = b.Stop(context.Background(), v); e != nil || out.Remote.State != "stopped" || out.Remote.HashesPerSecondMilli != 0 {
				t.Fatal("joined Stop", e)
			}
			if out, e = b.Logout(context.Background(), v); e != nil || out.State != "signed_out" || out.View != nil || out.Remote != nil {
				t.Fatal("Logout", e)
			}
			if b.ContextMatches(v) {
				t.Fatal("retired view survived logout")
			}
			before = b.nextID
			if _, e = b.Stop(context.Background(), v); e == nil || b.nextID != before {
				t.Fatal("retired view sent again")
			}
			if _, e = b.Snapshot(context.Background(), nil); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestRemotePoolBridgeUncertainMutationRetiresOriginalHelper(t *testing.T) {
	for _, mode := range []string{"wrong-id", "wrong-config", "unknown", "cleanup", "missing-lf", "crlf", "oversize", "double", "lost-reply"} {
		t.Run(mode, func(t *testing.T) {
			cmd, config := remoteBridgeFixtureCommand(t, "zero256-default", mode)
			b, e := startRemotePoolCommand(context.Background(), cmd, config)
			if b != nil {
				t.Cleanup(func() { remoteBridgeCloseTest(t, b) })
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = b.LoginFixture(context.Background(), "zero256"); e != nil {
				t.Fatal(e)
			}
			v, e := b.Context()
			if e != nil {
				t.Fatal(e)
			}
			limit := 2 * time.Second
			if mode == "lost-reply" {
				limit = 200 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), limit)
			_, e = b.Start(ctx, v)
			cancel()
			if mode == "double" && e == nil {
				// A first reply may linearize before the extra frame arrives, but
				// the reader must independently retire that exact original owner.
				select {
				case <-b.failed:
				case <-time.After(time.Second):
					t.Fatal("extra frame accepted")
				}
			} else if e == nil {
				t.Fatal("uncertain mutation reported success")
			}
			if e != nil && mode != "cleanup" && !errors.Is(e, ErrRemotePoolOutcomeUnknown) {
				t.Fatal("lost mutation classified definite", e)
			}
			if mode != "lost-reply" && errors.Is(e, context.DeadlineExceeded) {
				t.Fatal("framing/refusal control reached timeout instead", e)
			}
			remoteBridgeCloseTest(t, b)
			before := b.nextID
			if b.ContextMatches(v) {
				t.Fatal("failed process context survived")
			}
			if _, e = b.Start(context.Background(), v); e == nil || b.nextID != before {
				t.Fatal("failed helper dispatched again")
			}
		})
	}
}

func TestRemotePoolBridgeDefiniteRefusalKeepsOriginalView(t *testing.T) {
	cmd, config := remoteBridgeFixtureCommand(t, "zero256-default", "busy")
	b, e := startRemotePoolCommand(context.Background(), cmd, config)
	if b != nil {
		t.Cleanup(func() { remoteBridgeCloseTest(t, b) })
	}
	if e != nil {
		t.Fatal(e)
	}
	if _, e = b.LoginFixture(context.Background(), "zero256"); e != nil {
		t.Fatal(e)
	}
	v, _ := b.Context()
	_, e = b.Start(context.Background(), v)
	var failure *RemotePoolFailure
	if !errors.As(e, &failure) || failure.Code != "busy" || !b.ContextMatches(v) {
		t.Fatal(e)
	}
	if _, e = b.Snapshot(context.Background(), v); e != nil {
		t.Fatal(e)
	}
}

func TestRemotePoolBridgeUnsolicitedFrameRetires(t *testing.T) {
	cmd, config := remoteBridgeFixtureCommand(t, "zero256-default", "unsolicited")
	b, _ := startRemotePoolCommand(context.Background(), cmd, config)
	if b == nil {
		t.Fatal("launched owner lost")
	}
	select {
	case <-b.failed:
	case <-time.After(time.Second):
		t.Fatal("unsolicited frame not refused")
	}
	remoteBridgeCloseTest(t, b)
}

type remoteBridgeBlockingWrite struct {
	entered, released chan struct{}
	once              sync.Once
	writes, closes    atomic.Int32
	closeError        error
}

func (w *remoteBridgeBlockingWrite) Write(raw []byte) (int, error) {
	w.writes.Add(1)
	close(w.entered)
	<-w.released
	return 0, io.ErrClosedPipe
}
func (w *remoteBridgeBlockingWrite) Close() error {
	w.closes.Add(1)
	w.once.Do(func() { close(w.released) })
	return w.closeError
}
func remoteBridgeTransportFixture(t *testing.T, w *remoteBridgeBlockingWrite) *RemotePoolBridge {
	config, _ := remoteBridgeReferenceFor(t, "zero256-default")
	id, e := pilot.RemotePoolConfigID(config)
	if e != nil {
		t.Fatal(e)
	}
	read, wait := make(chan struct{}), make(chan struct{})
	close(read)
	close(wait)
	return &RemotePoolBridge{input: w, pipes: []remotePoolPipe{{owner: w}}, config: config, configID: id,
		gate: make(chan struct{}, 1), lines: make(chan []byte, 1), failed: make(chan struct{}), readDone: read, waitDone: wait}
}

func TestRemotePoolBridgeCancellationAndCloseJoinBlockedWrite(t *testing.T) {
	for _, closeAPI := range []bool{false, true} {
		w := &remoteBridgeBlockingWrite{entered: make(chan struct{}), released: make(chan struct{})}
		b := remoteBridgeTransportFixture(t, w)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, e := b.Snapshot(ctx, nil); !errors.Is(e, context.Canceled) || w.writes.Load() != 0 || b.retired {
			t.Fatal("canceled preflight dispatched", e)
		}
		ctx, cancel = context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { _, e := b.Snapshot(ctx, nil); done <- e }()
		defer func() { cancel(); _ = b.Close(context.Background()); <-done }()
		select {
		case <-w.entered:
		case <-time.After(time.Second):
			t.Fatal("write did not enter")
		}
		if closeAPI {
			remoteBridgeCloseTest(t, b)
		} else {
			cancel()
		}
		select {
		case e := <-done:
			if e == nil || !closeAPI && !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
			done <- e // deferred join owns a value on every later assertion path.
		case <-time.After(time.Second):
			t.Fatal("write/callback not joined")
		}
		if !b.retired || w.closes.Load() != 1 {
			t.Fatal("original pipe remained active")
		}
	}
}

func TestRemotePoolBridgeFailedCloserRetainedWithoutRetry(t *testing.T) {
	w := &remoteBridgeBlockingWrite{entered: make(chan struct{}), released: make(chan struct{}), closeError: errors.New("private-path-secret")}
	b := remoteBridgeTransportFixture(t, w)
	for i := 0; i < 2; i++ {
		e := b.Close(context.Background())
		if !errors.Is(e, ErrRemotePoolBridgeCleanup) || strings.Contains(e.Error(), "private-path-secret") {
			t.Fatal("cleanup cause leaked/lost", e)
		}
	}
	if w.closes.Load() != 1 || b.pipes[0].owner != w {
		t.Fatal("failed concrete owner retried/lost")
	}
}

func TestRemotePoolBridgeIncompleteJoinRetainsOriginalOwner(t *testing.T) {
	w := &remoteBridgeBlockingWrite{entered: make(chan struct{}), released: make(chan struct{})}
	b := remoteBridgeTransportFixture(t, w)
	b.waitDone = make(chan struct{})
	defer close(b.waitDone)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	e := b.Close(ctx)
	if !errors.Is(e, context.DeadlineExceeded) || !errors.Is(e, ErrRemotePoolBridgeCleanup) || !b.retired || b.pipes[0].owner != w {
		t.Fatal("incomplete cleanup lost original owner", e)
	}
	if w.closes.Load() != 1 {
		t.Fatal("incomplete cleanup retried descriptor")
	}
}

func TestRemotePoolBridgeLaunchUsesOnlySelectedPaths(t *testing.T) {
	s := RemotePoolLaunchSelection{ExistingRoot: "/fixture/existing", AttachmentRoot: "/fixture/attachment", ConfigPath: "/fixture/public.json", NodeRPCPath: "/fixture/rpc.json", PoolCredentialPath: "/fixture/pool.token", AdmissionCredentialPath: "/fixture/admission.token"}
	cmd, e := remotePoolLaunchCommand("/fixture/helper", s)
	want := []string{"/fixture/helper", "--candidate-remote-pool", "--data-dir", s.ExistingRoot, "--remote-attachment-root", s.AttachmentRoot, "--remote-config", s.ConfigPath, "--remote-node-rpc", s.NodeRPCPath, "--remote-pool-credential", s.PoolCredentialPath, "--remote-admission-credential", s.AdmissionCredentialPath}
	if e != nil || !reflect.DeepEqual(cmd.Args, want) {
		t.Fatal("launch changed selected mode/paths", e)
	}
	s.NodeRPCPath = "relative-secret"
	if _, e = remotePoolLaunchCommand("/fixture/helper", s); e == nil {
		t.Fatal("relative input accepted")
	}
	s.NodeRPCPath = "/fixture/rpc.json"
	s.Options.CandidateDescriptorID = strings.Repeat("f", 64)
	if _, e = remotePoolLaunchCommand("/fixture/helper", s); e == nil {
		t.Fatal("unselected descriptor accepted")
	}
}
