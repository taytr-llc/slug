package main

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/fighterownership"
	"testing"
	"time"
)

type refreshDriverFixture struct {
	original               context.Context
	begins, steps, cancels int
	fail                   error
	cancel                 context.CancelFunc
}

func (s *refreshDriverFixture) BeginRefresh(ctx context.Context) (fighterownership.Progress, error) {
	s.begins++
	s.original = ctx
	return fighterownership.Progress{WorkID: "exact-retained-work"}, nil
}
func (s *refreshDriverFixture) RefreshStep(ctx context.Context, n uint32) (fighterownership.Progress, error) {
	if ctx != s.original || n != 8 {
		panic("replaced original context or slice bound")
	}
	s.steps++
	if s.cancel != nil {
		s.cancel()
		return fighterownership.Progress{}, context.Canceled
	}
	if s.fail != nil {
		return fighterownership.Progress{}, s.fail
	}
	p := fighterownership.Progress{WorkID: "exact-retained-work"}
	if s.steps == 3 {
		p.Ready = true
		p.Head = &fighterownership.HeadRef{ChainHeight: 163}
	}
	return p, nil
}
func (s *refreshDriverFixture) CancelRefresh(ctx context.Context) error {
	s.cancels++
	if ctx != s.original {
		panic("replacement cleanup context")
	}
	return nil
}
func TestRestoredCLIRefreshDriverRetainsBoundedOriginalOperation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	deadline, _ := ctx.Deadline()
	s := new(refreshDriverFixture)
	h, e := refreshHead(ctx, s)
	if e != nil || h.ChainHeight != 163 || s.begins != 1 || s.steps != 3 || s.cancels != 1 {
		t.Fatal("bounded driver", h, s, e)
	}
	after, _ := ctx.Deadline()
	if deadline != after || ctx.Err() != nil {
		t.Fatal("driver changed original lifetime")
	}
}
func TestRestoredCLIRefreshDriverDoesNotRetryFailure(t *testing.T) {
	for _, failure := range []error{context.DeadlineExceeded, fighterownership.ErrStale, fighterownership.ErrUnavailable} {
		s := &refreshDriverFixture{fail: failure}
		h, e := refreshHead(context.Background(), s)
		if !errors.Is(e, failure) || h != (fighterownership.HeadRef{}) || s.begins != 1 || s.steps != 1 || s.cancels != 1 {
			t.Fatal("failed slice retried or disclosed", s, e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &refreshDriverFixture{cancel: cancel}
	h, e := refreshHead(ctx, s)
	if !errors.Is(e, context.Canceled) || h != (fighterownership.HeadRef{}) || s.steps != 1 || s.cancels != 1 {
		t.Fatal("cancel rescued", e)
	}
}
