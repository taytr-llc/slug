// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
)

type remotePoolNode struct {
	client  *prospectnode.Client
	local   *remotePoolLocal
	policy  prospectnode.Policy
	life    context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	closing bool
	active  sync.WaitGroup
	once    sync.Once
}

func remotePoolValidateNode(c prospectnode.RPCConfig) error {
	if c.Version != 1 || len(c.URL) > 4096 || len(c.Username) > 4096 || len(c.Password) > 4096 || c.CACertificate != "" {
		return ErrRemotePoolConfig
	}
	u, e := url.Parse(c.URL)
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Port() == "" || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || u.String() != c.URL || strings.ContainsAny(c.URL, "\x00\r\n\t") {
		return ErrRemotePoolConfig
	}
	p, e := strconv.Atoi(u.Port())
	if e != nil || p < 1 || p > 65535 || strconv.Itoa(p) != u.Port() {
		return ErrRemotePoolConfig
	}
	return nil
}
func openRemotePoolNode(life context.Context, c prospectnode.RPCConfig, l *remotePoolLocal, p prospectnode.Policy) (*remotePoolNode, error) {
	if life == nil || life.Err() != nil || l == nil || remotePoolValidateNode(c) != nil {
		return nil, ErrRemotePoolConfig
	}
	client, e := prospectnode.NewClient(c)
	if e != nil {
		return nil, ErrRemotePoolUnavailable
	}
	ctx, cancel := context.WithCancel(life)
	return &remotePoolNode{client: client, local: l, policy: p, life: ctx, cancel: cancel}, nil
}
func remotePoolNodeCall[T any](ctx context.Context, n *remotePoolNode, fn func(context.Context) (T, error)) (out T, err error) {
	if n == nil || ctx == nil || ctx.Err() != nil {
		return out, ErrRemotePoolUnavailable
	}
	n.mu.Lock()
	if n.closing || n.life.Err() != nil {
		n.mu.Unlock()
		return out, ErrRemotePoolUnavailable
	}
	n.active.Add(1)
	n.mu.Unlock()
	defer n.active.Done()
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(n.life, cancel)
	defer func() { stop(); cancel() }()
	lease, _, e := n.local.capture(work)
	if e != nil {
		return out, e
	}
	out, err = fn(work)
	if work.Err() != nil || ctx.Err() != nil || n.life.Err() != nil || lease.Check() != nil || n.local.check(work) != nil {
		var zero T
		return zero, ErrRemotePoolUnavailable
	}
	return out, err
}
func (n *remotePoolNode) ResolveTemplate(ctx context.Context, p prospectnode.Policy) (prospectwork.Template, error) {
	if n == nil || p != n.policy {
		return prospectwork.Template{}, ErrRemotePoolChanged
	}
	return remotePoolNodeCall(ctx, n, func(c context.Context) (prospectwork.Template, error) { return n.client.ResolveTemplate(c, p) })
}
func (n *remotePoolNode) Proposal(ctx context.Context, p prospect.Context, b string) error {
	_, e := remotePoolNodeCall(ctx, n, func(c context.Context) (struct{}, error) { return struct{}{}, n.client.Proposal(c, p, b) })
	return e
}
func (n *remotePoolNode) RecheckContext(ctx context.Context, p prospect.Context) error {
	_, e := remotePoolNodeCall(ctx, n, func(c context.Context) (struct{}, error) { return struct{}{}, n.client.RecheckContext(c, p) })
	return e
}
func (n *remotePoolNode) SubmitBlock(ctx context.Context, p prospect.Context, b string) error {
	_, e := remotePoolNodeCall(ctx, n, func(c context.Context) (struct{}, error) { return struct{}{}, n.client.SubmitBlock(c, p, b) })
	return e
}
func (n *remotePoolNode) CanonicalBlock(ctx context.Context, p prospect.Context, proof prospect.Proof, hash string) error {
	_, e := remotePoolNodeCall(ctx, n, func(c context.Context) (struct{}, error) {
		return struct{}{}, n.client.CanonicalBlock(c, p, proof, hash)
	})
	return e
}
func (n *remotePoolNode) close() {
	if n == nil {
		return
	}
	n.once.Do(func() {
		n.mu.Lock()
		n.closing = true
		n.cancel()
		n.mu.Unlock()
		n.active.Wait()
		n.client.Close()
	})
}
