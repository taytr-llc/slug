// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
)

// This context-aware account-gate model serializes reset exactly through each
// callback. The shared service composition separately qualifies real accounts.
type fenceSessions struct {
	gate chan struct{}
	keys map[string]string
	held atomic.Bool
}

func newFenceSessions(key string) *fenceSessions {
	return &fenceSessions{gate: make(chan struct{}, 1), keys: map[string]string{
		poolTestToken: key,
		"other-gym":   "02c6047f9441ed7d6d3045406e95c07cd85aef011134fd65ddfa65673a7ee568f",
	}}
}
func (s *fenceSessions) lock(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:
		if e := ctx.Err(); e != nil {
			<-s.gate
			return e
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *fenceSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	if e := s.lock(ctx); e != nil {
		return e
	}
	s.held.Store(true)
	defer func() { s.held.Store(false); <-s.gate }()
	key := s.keys[token]
	if key == "" {
		return errors.New("test credential revoked")
	}
	return fn(key, func() error {
		if s.keys[token] != key {
			return errors.New("test credential changed")
		}
		return ctx.Err()
	})
}
func (s *fenceSessions) revoke(ctx context.Context, token string) error {
	if e := s.lock(ctx); e != nil {
		return e
	}
	defer func() { <-s.gate }()
	delete(s.keys, token)
	return nil
}

type fenceNode struct {
	Node
	t                *testing.T
	sessions         *fenceSessions
	stage            string
	entered, release chan struct{}
	once             sync.Once
}

func (n *fenceNode) pause(ctx context.Context, stage string) error {
	if n.sessions.held.Load() {
		n.t.Error("node call ran under account lease: " + stage)
		return errors.New("node under account lease")
	}
	if stage != n.stage {
		return nil
	}
	var e error
	n.once.Do(func() {
		close(n.entered)
		select {
		case <-n.release:
		case <-ctx.Done():
			e = ctx.Err()
		}
	})
	return e
}
func (n *fenceNode) ResolveTemplate(ctx context.Context, p prospectnode.Policy) (prospectwork.Template, error) {
	if e := n.pause(ctx, "resolve"); e != nil {
		return prospectwork.Template{}, e
	}
	return n.Node.ResolveTemplate(ctx, p)
}
func (n *fenceNode) Proposal(ctx context.Context, c prospect.Context, b string) error {
	if e := n.pause(ctx, "proposal"); e != nil {
		return e
	}
	return n.Node.Proposal(ctx, c, b)
}
func (n *fenceNode) RecheckContext(ctx context.Context, c prospect.Context) error {
	if e := n.pause(ctx, "current"); e != nil {
		return e
	}
	return n.Node.RecheckContext(ctx, c)
}
func (n *fenceNode) SubmitBlock(ctx context.Context, c prospect.Context, b string) error {
	if e := n.pause(ctx, "submit"); e != nil {
		return e
	}
	return n.Node.SubmitBlock(ctx, c, b)
}
func (n *fenceNode) CanonicalBlock(ctx context.Context, c prospect.Context, p prospect.Proof, h string) error {
	if e := n.pause(ctx, "canonical"); e != nil {
		return e
	}
	return n.Node.CanonicalBlock(ctx, c, p, h)
}

func TestPoolSessionFenceResetAndOtherGymDuringNodeWork(t *testing.T) {
	for _, tc := range []struct {
		operation, stage string
		unknown          bool
	}{{"issue", "resolve", false}, {"issue", "current", false}, {"issue", "proposal", true}, {"share", "current", false}, {"winner", "submit", true}, {"winner", "canonical", true}} {
		t.Run(tc.operation+"-"+tc.stage, func(t *testing.T) {
			h := setupPool(t, "", 0)
			sessions := newFenceSessions(h.source.Job.PublicKey)
			h.pool.sessions = sessions
			node := &fenceNode{Node: h.node, t: t, sessions: sessions, stage: tc.stage, entered: make(chan struct{}), release: make(chan struct{})}
			h.pool.node = node
			if tc.operation == "issue" {
				h.pool.now = time.Now
			}
			done := make(chan error, 1)
			go func() {
				var e error
				if tc.operation == "issue" {
					w, err := h.pool.Issue(context.Background(), poolTestToken)
					e = err
					if err != nil && w.Job.ID != "" {
						t.Error("failed issuance disclosed work")
					}
				} else {
					nonce := h.ordinary.Nonce
					if tc.operation == "winner" {
						nonce = h.winner.Nonce
					}
					r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, nonce)
					e = err
					if err != nil && r.Version != 0 {
						t.Error("failed submission disclosed receipt")
					}
				}
				done <- e
			}()
			select {
			case <-node.entered:
			case e := <-done:
				t.Fatal("operation missed node boundary", e)
			case <-time.After(5 * time.Second):
				t.Fatal("node boundary timed out")
			}
			// Reset and the other gym both finish before the slow operation is
			// released. A replacement token is never supplied to that operation.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			probe := sessions.WithSession(ctx, "other-gym", func(_ string, check func() error) error { return check() })
			reset := sessions.revoke(ctx, poolTestToken)
			cancel()
			close(node.release)
			e := <-done
			if probe != nil || reset != nil {
				t.Fatal("slow node blocked account gate", probe, reset)
			}
			want := ErrUnauthorized
			if tc.unknown {
				want = ErrOutcomeUnknown
			}
			if !errors.Is(e, want) {
				t.Fatal("original session outcome", e, "want", want)
			}
			if tc.operation == "share" && h.pool.meta.Submissions != 0 {
				t.Fatal("revoked ordinary share committed")
			}
			if tc.operation == "winner" {
				r, err := h.pool.loadSubmission(h.source.Job.ID, h.winner.Nonce)
				if err != nil || r.Sequence != 1 || r.BlockState == "canonical-observed" {
					t.Fatal("reset lost exact intent or granted post-reset canonical update", r, err)
				}
			}
		})
	}
}

func TestPoolSessionFenceHoldsResetThroughActualBatch(t *testing.T) {
	h := setupPool(t, "", 0)
	sessions := newFenceSessions(h.source.Job.PublicKey)
	h.pool.sessions = sessions
	entered, release := make(chan struct{}), make(chan struct{})
	write := h.pool.writeBatch
	h.pool.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
		d := new(submissionBatchDetector)
		if e := batch.Replay(d); e != nil {
			return e
		}
		if !sessions.held.Load() {
			t.Error("authenticated clock/receipt write outside lease")
		}
		if d.found {
			close(entered)
			<-release
		}
		return write(batch, options)
	}
	done := make(chan error, 1)
	go func() {
		_, e := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
		done <- e
	}()
	select {
	case <-entered:
	case e := <-done:
		t.Fatal("submission missed durable batch", e)
	case <-time.After(5 * time.Second):
		t.Fatal("durable batch timed out")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	e := sessions.revoke(ctx, poolTestToken)
	cancel()
	close(release)
	if got := <-done; got != nil {
		t.Fatal(got)
	}
	if !errors.Is(e, context.DeadlineExceeded) || h.pool.meta.Accepted != 1 {
		t.Fatal("reset crossed synchronous batch", e, h.pool.meta.Accepted)
	}
	if e := sessions.revoke(context.Background(), poolTestToken); e != nil {
		t.Fatal(e)
	}
	if _, e := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("old token disclosed accepted receipt after reset", e)
	}
}

type fenceAfterCommitSessions struct {
	inner *fenceSessions
	pool  *Pool
	fired bool
}

func (s *fenceAfterCommitSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	e := s.inner.WithSession(ctx, token, fn)
	if e == nil && s.pool.meta.Submissions != 0 && !s.fired {
		s.fired = true
		return errors.New("test: authorizer failed after protected commit")
	}
	return e
}

func TestPoolSessionFencePostCommitFailureRecoversExactlyOnce(t *testing.T) {
	h := setupPool(t, "", 0)
	access := &fenceAfterCommitSessions{inner: newFenceSessions(h.source.Job.PublicKey), pool: h.pool}
	h.pool.sessions = access
	got, e := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if !errors.Is(e, ErrOutcomeUnknown) || !access.fired || got.Version != 0 || h.pool.meta.Accepted != 1 {
		t.Fatal("post-commit authorization failure became a definitive rejection", got, e)
	}
	h.reopen(t)
	saved, e := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if e != nil || saved.Sequence != 1 || saved.AcceptedSequence != 1 || saved.Acceptance != "current-parent" {
		t.Fatal("committed share not recoverable", saved, e)
	}
	again, e := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if e != nil || again != saved || h.pool.meta.Submissions != 1 || h.pool.meta.Accepted != 1 {
		t.Fatal("recovery duplicated original acceptance", again, e)
	}
}
