// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type remoteWireTestController struct {
	mu                                         sync.Mutex
	initial, logged, running, stopped, current RemotePoolWireSnapshot
	starts, stops, logins, logouts, closes     int
	startError, postError, errorNextRead       error
	closeError                                 error
	closeEntered, closeRelease                 chan struct{}
	startEntered, startEnded                   chan struct{}
	closeOnce                                  sync.Once
}

func remoteWireTestCopy(s RemotePoolWireSnapshot) RemotePoolWireSnapshot {
	if s.View != nil {
		v := *s.View
		s.View = &v
	}
	if s.Remote != nil {
		r := *s.Remote
		s.Remote = &r
	}
	return s
}
func (c *remoteWireTestController) WireSnapshot(ctx context.Context) (RemotePoolWireSnapshot, error) {
	if e := ctx.Err(); e != nil {
		return RemotePoolWireSnapshot{}, e
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.errorNextRead != nil {
		e := c.errorNextRead
		c.errorNextRead = nil
		return RemotePoolWireSnapshot{}, e
	}
	return remoteWireTestCopy(c.current), nil
}
func (c *remoteWireTestController) LoginFixture(ctx context.Context, f string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || f != c.logged.Account.Fixture {
		return ErrRemotePoolProtocol
	}
	c.logins++
	c.current = c.logged
	return nil
}
func (c *remoteWireTestController) Start(ctx context.Context, v string) error {
	c.mu.Lock()
	if ctx.Err() != nil || c.current.View == nil || c.current.View.ViewID != v {
		c.mu.Unlock()
		return ErrRemotePoolProtocol
	}
	c.starts++
	c.current = c.running
	c.errorNextRead = c.postError
	entered, ended, result := c.startEntered, c.startEnded, c.startError
	c.mu.Unlock()
	if entered != nil {
		close(entered)
		<-ctx.Done()
		close(ended)
		return ctx.Err()
	}
	return result
}
func (c *remoteWireTestController) Stop(ctx context.Context, v string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || c.current.View == nil || c.current.View.ViewID != v {
		return ErrRemotePoolProtocol
	}
	c.stops++
	c.current = c.stopped
	return nil
}
func (c *remoteWireTestController) Logout(ctx context.Context, v string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || c.current.View == nil || c.current.View.ViewID != v {
		return ErrRemotePoolProtocol
	}
	c.logouts++
	c.current = c.initial
	return nil
}
func (c *remoteWireTestController) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closes++
		c.mu.Unlock()
		if c.closeEntered != nil {
			close(c.closeEntered)
		}
		if c.closeRelease != nil {
			<-c.closeRelease
		}
	})
	return c.closeError
}

type remoteWireTestOutput struct {
	bytes.Buffer
	closed bool
}

func (o *remoteWireTestOutput) Close() error { o.closed = true; return nil }

type remoteWireTestWriter struct {
	io.WriteCloser
	closed bool
}

func (w *remoteWireTestWriter) Close() error { w.closed = true; return w.WriteCloser.Close() }

// Keep stdin open until each exact response arrives. EOF is a shutdown request,
// not permission for the helper to finish a pre-buffered command batch.
func remoteWireTestConversation(t *testing.T, c *remoteWireTestController, cfg RemotePoolConfig, frames [][]byte) (*remoteWireTestOutput, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	in, send := io.Pipe()
	receive, out := io.Pipe()
	writer := &remoteWireTestWriter{WriteCloser: out}
	done := make(chan error, 1)
	go func() { done <- ServeRemotePool(ctx, in, writer, c, cfg) }()
	got := &remoteWireTestOutput{}
	reader := bufio.NewReader(receive)
	for _, frame := range frames {
		if _, e := send.Write(frame); e != nil {
			break
		}
		line, e := reader.ReadBytes('\n')
		if e != nil {
			break
		}
		got.Write(line)
	}
	send.Close()
	e := <-done
	receive.Close()
	got.closed = writer.closed
	return got, e
}
func remoteWireTestQueries(t *testing.T, c *remoteWireTestController, cfg RemotePoolConfig, qs []RemotePoolRequest) (*remoteWireTestOutput, error) {
	t.Helper()
	frames := make([][]byte, 0, len(qs))
	for _, q := range qs {
		frames = append(frames, remoteWireTestStream(t, []RemotePoolRequest{q}, cfg))
	}
	return remoteWireTestConversation(t, c, cfg, frames)
}

func remoteWireTestOwner(t *testing.T) (*remoteWireTestController, RemotePoolConfig, []RemotePoolRequest) {
	t.Helper()
	r, configs := remoteWireTestReference(t)
	c := &remoteWireTestController{}
	for _, x := range r.Positives {
		var s *RemotePoolWireSnapshot
		switch x.Name {
		case "zero256-default-snapshot":
			s = &c.initial
		case "zero256-default-login_fixture":
			s = &c.logged
		case "zero256-default-start":
			s = &c.running
		case "zero256-default-stop":
			s = &c.stopped
		}
		if s != nil {
			var response RemotePoolResponse
			if e := json.Unmarshal(x.Response, &response); e != nil || response.Result == nil {
				t.Fatal(e)
			}
			*s = *response.Result
		}
	}
	c.current = c.initial
	var qs []RemotePoolRequest
	for _, x := range r.Requests {
		if x.Config == "zero256-default" {
			q := remoteWireTestRequest(t, x.Request)
			if q.ID <= 5 {
				qs = append(qs, q)
			}
		}
	}
	if len(qs) != 5 || c.logged.View == nil {
		t.Fatal("finite stream fixtures")
	}
	return c, configs["zero256-default"], qs
}
func remoteWireTestStream(t *testing.T, qs []RemotePoolRequest, c RemotePoolConfig) []byte {
	t.Helper()
	var b bytes.Buffer
	for _, q := range qs {
		raw, e := MarshalRemotePoolRequest(q, c)
		if e != nil {
			t.Fatal(e)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.Bytes()
}
func TestServeRemotePoolOriginalFlowAndNoAutostart(t *testing.T) {
	c, cfg, qs := remoteWireTestOwner(t)
	o, e := remoteWireTestQueries(t, c, cfg, qs)
	if e != nil {
		t.Fatal(e)
	}
	if c.logins != 1 || c.starts != 1 || c.stops != 1 || c.logouts != 1 || c.closes != 1 || !o.closed {
		t.Fatal("effects or join", c.logins, c.starts, c.stops, c.logouts, c.closes)
	}
	scan := bufio.NewScanner(bytes.NewReader(o.Bytes()))
	var prior *RemotePoolView
	count := 0
	for scan.Scan() {
		if count >= len(qs) {
			t.Fatal("extra response")
		}
		r, e := ParseRemotePoolResponse(scan.Bytes(), qs[count], cfg, prior)
		if e != nil || !r.OK {
			t.Fatal("stream response", count, e)
		}
		if count == 0 && r.Result.Remote != nil {
			t.Fatal("bootstrap opened child")
		}
		prior = r.Result.View
		count++
	}
	if scan.Err() != nil || count != 5 {
		t.Fatal("joined response count", count, scan.Err())
	}
}
func TestServeRemotePoolStrictFramingAndSequence(t *testing.T) {
	r, _ := remoteWireTestReference(t)
	for _, f := range r.Framing {
		t.Run(f.Name, func(t *testing.T) {
			c, cfg, _ := remoteWireTestOwner(t)
			o := &remoteWireTestOutput{}
			var e error
			if f.Accept {
				o, e = remoteWireTestConversation(t, c, cfg, [][]byte{remoteWireTestHex(t, f.RawHex)})
			} else {
				e = ServeRemotePool(context.Background(), io.NopCloser(bytes.NewReader(remoteWireTestHex(t, f.RawHex))), o, c, cfg)
			}
			if (e == nil) != f.Accept {
				t.Fatal("frame acceptance", e)
			}
			if c.starts != 0 || c.closes != 1 {
				t.Fatal("unexpected work or unjoined close")
			}
		})
	}
	c, cfg, qs := remoteWireTestOwner(t)
	o, e := remoteWireTestQueries(t, c, cfg, []RemotePoolRequest{qs[0], qs[0]})
	if e == nil || c.starts != 0 || bytes.Count(o.Bytes(), []byte{'\n'}) != 1 {
		t.Fatal("replayed request was not fatal", e)
	}
}
func TestServeRemotePoolMutationUnknownRetiresOwner(t *testing.T) {
	for _, post := range []bool{false, true} {
		t.Run(map[bool]string{false: "dispatch-error", true: "post-disclosure"}[post], func(t *testing.T) {
			c, cfg, qs := remoteWireTestOwner(t)
			if post {
				c.postError = gymstable.ErrBusy
			} else {
				c.startError = gymstable.ErrBusy
			}
			o, e := remoteWireTestQueries(t, c, cfg, qs)
			if e == nil || c.starts != 1 || c.stops != 0 || c.logouts != 0 || c.closes != 1 {
				t.Fatal("unknown action not retired", e)
			}
			lines := bytes.Split(bytes.TrimSuffix(o.Bytes(), []byte{'\n'}), []byte{'\n'})
			if len(lines) != 3 {
				t.Fatal("unknown response count")
			}
			r, e := ParseRemotePoolResponse(lines[2], qs[2], cfg, c.logged.View)
			if e != nil || r.OK || r.Error != "outcome_unknown" {
				t.Fatal("mutation Busy misclassified", r.Error, e)
			}
		})
	}
}
func TestRemotePoolWireReadErrorDoesNotMaskUncertainty(t *testing.T) {
	if remoteWireReadError(gymstable.ErrBusy) != "busy" {
		t.Fatal("exact read busy")
	}
	if remoteWireReadError(errors.Join(gymstable.ErrBusy, errors.New("storage"))) == "busy" {
		t.Fatal("mixed failure hidden by busy")
	}
	if remoteWireReadError(errors.Join(gymstable.ErrBusy, gymstable.ErrOutcomeUnknown)) != "outcome_unknown" {
		t.Fatal("unknown hidden")
	}
	if remoteWireReadError(errors.Join(ErrRemotePoolCleanup, gymstable.ErrBusy)) != "cleanup_incomplete" {
		t.Fatal("cleanup hidden")
	}
}
func TestServeRemotePoolStaleViewCannotMutate(t *testing.T) {
	c, cfg, qs := remoteWireTestOwner(t)
	bad := qs[2]
	bad.ViewID = strings.Repeat("f", 64)
	input := []RemotePoolRequest{qs[0], qs[1], bad, qs[3], qs[4]}
	o, e := remoteWireTestQueries(t, c, cfg, input)
	if e != nil {
		t.Fatal(e)
	}
	if c.starts != 0 || c.stops != 1 || c.logouts != 1 {
		t.Fatal("stale view retargeted")
	}
	lines := bytes.Split(o.Bytes(), []byte{'\n'})
	var reply RemotePoolResponse
	if e := json.Unmarshal(lines[2], &reply); e != nil || reply.Error != "stale_view" {
		t.Fatal("stale refusal", e)
	}
}
func TestServeRemotePoolCancellationJoinsReadAndClose(t *testing.T) {
	c, cfg, _ := remoteWireTestOwner(t)
	c.closeEntered = make(chan struct{})
	c.closeRelease = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	input, writer := io.Pipe()
	o := &remoteWireTestOutput{}
	done := make(chan error, 1)
	go func() { done <- ServeRemotePool(ctx, input, o, c, cfg) }()
	cancel()
	select {
	case <-c.closeEntered:
	case <-time.After(2 * time.Second):
		close(c.closeRelease)
		writer.Close()
		<-done
		t.Fatal("Close not reached")
	}
	select {
	case <-done:
		close(c.closeRelease)
		t.Fatal("returned before original Close joined")
	default:
	}
	close(c.closeRelease)
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancellation lost")
		}
	case <-time.After(2 * time.Second):
		writer.Close()
		<-done
		t.Fatal("read/Close not joined")
	}
	writer.Close()
	if c.closes != 1 || !o.closed {
		t.Fatal("owner cleanup missing")
	}
}
func TestServeRemotePoolCancellationJoinsBlockedWrite(t *testing.T) {
	c, cfg, qs := remoteWireTestOwner(t)
	ctx, cancel := context.WithCancel(context.Background())
	output, writer := io.Pipe()
	input, send := io.Pipe()
	defer send.Close()
	done := make(chan error, 1)
	stream := remoteWireTestStream(t, qs[:1], cfg)
	go func() {
		done <- ServeRemotePool(ctx, input, writer, c, cfg)
	}()
	if _, e := send.Write(stream); e != nil {
		cancel()
		output.Close()
		<-done
		t.Fatal(e)
	}
	// Read one byte to prove that response delivery entered the original pipe,
	// then leave the remaining Write blocked and cancel its owner.
	got := make(chan error, 1)
	go func() { var one [1]byte; _, e := io.ReadFull(output, one[:]); got <- e }()
	select {
	case e := <-got:
		if e != nil {
			cancel()
			output.Close()
			<-done
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		cancel()
		output.Close()
		<-done
		<-got
		t.Fatal("write not entered")
	}
	cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("blocked write cancellation lost")
		}
	case <-time.After(2 * time.Second):
		output.Close()
		<-done
		t.Fatal("write not joined")
	}
	output.Close()
	if c.closes != 1 {
		t.Fatal("controller not joined")
	}
}

func TestServeRemotePoolRetainsOriginalCloseFailure(t *testing.T) {
	c, cfg, qs := remoteWireTestOwner(t)
	c.closeError = errors.New("synthetic original owner close failure")
	o, e := remoteWireTestQueries(t, c, cfg, qs[:1])
	if !errors.Is(e, c.closeError) || c.closes != 1 || c.starts != 0 || !o.closed {
		t.Fatal("original cleanup uncertainty lost", e)
	}
	if !errors.Is(c.Close(), c.closeError) || c.closes != 1 {
		t.Fatal("failed original owner replaced or retried")
	}
}

func TestServeRemotePoolEOFAndPipelineCancelBlockedAction(t *testing.T) {
	for _, pipeline := range []bool{false, true} {
		t.Run(map[bool]string{false: "stdin-eof", true: "pipelined-command"}[pipeline], func(t *testing.T) {
			c, cfg, qs := remoteWireTestOwner(t)
			c.startEntered, c.startEnded = make(chan struct{}), make(chan struct{})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			input, send := io.Pipe()
			receive, output := io.Pipe()
			done := make(chan error, 1)
			go func() { done <- ServeRemotePool(ctx, input, output, c, cfg) }()
			joined := false
			defer func() {
				cancel()
				send.Close()
				receive.Close()
				if !joined {
					<-done
				}
			}()
			reader := bufio.NewReader(receive)
			for _, q := range qs[:2] {
				if _, e := send.Write(remoteWireTestStream(t, []RemotePoolRequest{q}, cfg)); e != nil {
					t.Fatal(e)
				}
				if _, e := reader.ReadBytes('\n'); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := send.Write(remoteWireTestStream(t, qs[2:3], cfg)); e != nil {
				t.Fatal(e)
			}
			select {
			case <-c.startEntered:
			case <-ctx.Done():
				t.Fatal("blocked Start not entered")
			}
			if pipeline {
				// The same original view is valid; only the unacknowledged
				// in-flight action makes this second command inadmissible.
				_, _ = send.Write(remoteWireTestStream(t, qs[3:4], cfg))
			} else {
				if e := send.Close(); e != nil {
					t.Fatal(e)
				}
			}
			select {
			case e := <-done:
				joined = true
				if (e != nil) != pipeline {
					t.Fatal("EOF/pipeline classification", e)
				}
			case <-ctx.Done():
				t.Fatal("stdin did not cancel/join the original action")
			}
			if ctx.Err() != nil {
				t.Fatal("completion relied on the parent deadline")
			}
			select {
			case <-c.startEnded:
			default:
				t.Fatal("action escaped joined lifetime")
			}
			if c.starts != 1 || c.stops != 0 || c.closes != 1 {
				t.Fatal("pipelined mutation or missing owner join")
			}
		})
	}
}
