// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"reflect"
	"strings"
	"sync"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

// PoolAuthorityConfig is private desktop wiring. It pins a shared pool and
// transport trust; no token, signing key, payee change or worker setting is
// serialized into it. Existing solo payout markers remain unchanged.
type PoolAuthorityConfig struct {
	Version                                         uint32
	URL, CACertificate, PublicKey, TransportTrustID string
	Source                                          poolshare.Config
}

// PoolSessionAuthority refreshes credentials between calls while retaining the
// original account, generation and view. External I/O never holds an account
// lease. Closing cancels and drains the active call before an account switch.
type PoolSessionAuthority struct {
	config  PoolAuthorityConfig
	account ManifestAccountConfig
	ctx     context.Context
	cancel  context.CancelFunc
	gate    chan struct{}
	mu      sync.Mutex
	closing bool
	done    chan struct{}
}

func poolAccessError(mutation bool) error {
	if mutation {
		return &poolservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	return &poolservice.ClientError{Code: "unauthorized"}
}
func NewPoolSessionAuthority(c PoolAuthorityConfig, a ManifestAccountConfig) (*PoolSessionAuthority, error) {
	if wire.TestPoW {
		return nil, poolshare.ErrProfile
	}
	fixture, e := gymaccount.FixtureIdentity(a.Account.Fixture)
	var nilAuthorizer = a.Account.Authorizer == nil
	if !nilAuthorizer {
		v := reflect.ValueOf(a.Account.Authorizer)
		switch v.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Func, reflect.Interface, reflect.Slice, reflect.Chan:
			nilAuthorizer = v.IsNil()
		}
	}
	if e != nil || c.Version != poolservice.Version1 || c.PublicKey != fixture.GymPublicKey || !validAccountID(a.Account.AccountID) || !validAccountID(a.Account.AuthorityID) || !validAccountID(a.ViewID) || a.Generation == 0 || a.Generation > manifestMaxSafe || a.ViewLease == nil || nilAuthorizer || a.Account.MiningToken == nil {
		return nil, poolAccessError(false)
	}
	probe := poolservice.ClientConfig{Version: c.Version, URL: c.URL, CACertificate: c.CACertificate, PublicKey: c.PublicKey, Source: c.Source, Token: strings.Repeat("0", 64)}
	canonical, e := poolservice.ValidateClientConfig(probe)
	if e != nil {
		return nil, poolAccessError(false)
	}
	trust, e := poolservice.TransportTrustID(canonical)
	if e != nil || trust != c.TransportTrustID {
		return nil, poolAccessError(false)
	}
	c.URL, c.CACertificate = canonical.URL, canonical.CACertificate
	ctx, cancel := context.WithCancel(context.Background())
	return &PoolSessionAuthority{config: c, account: a, ctx: ctx, cancel: cancel, gate: make(chan struct{}, 1), done: make(chan struct{})}, nil
}
func (a *PoolSessionAuthority) ProtocolVersion() uint32        { return poolservice.Version1 }
func (a *PoolSessionAuthority) AuthorityOrigin() string        { return a.config.URL }
func (a *PoolSessionAuthority) SourceConfig() poolshare.Config { return a.config.Source }
func (a *PoolSessionAuthority) TransportTrustID() string       { return a.config.TransportTrustID }
func (a *PoolSessionAuthority) PublicKey() string              { return a.config.PublicKey }

func (a *PoolSessionAuthority) checkView(ctx context.Context) error {
	called := false
	var operationErr error
	err := a.account.ViewLease(ctx, func(account gymaccount.Account, recheck func() error) (e error) {
		defer func() { operationErr = e }()
		if called {
			return poolAccessError(false)
		}
		called = true
		if account.ID != a.account.Account.AccountID || account.Generation != a.account.Generation || recheck == nil {
			return poolAccessError(false)
		}
		if e = recheck(); e != nil {
			return e
		}
		return ctx.Err()
	})
	if err != nil || operationErr != nil || !called || ctx.Err() != nil {
		return poolAccessError(false)
	}
	return nil
}
func (a *PoolSessionAuthority) checkToken(ctx context.Context, token string) error {
	called := false
	var operationErr error
	err := a.account.Account.Authorizer.WithSession(ctx, token, func(key string, recheck func() error) (e error) {
		defer func() { operationErr = e }()
		if called {
			return poolAccessError(false)
		}
		called = true
		if key != a.config.PublicKey || recheck == nil {
			return poolAccessError(false)
		}
		if e = recheck(); e != nil {
			return e
		}
		return ctx.Err()
	})
	if err != nil || operationErr != nil || !called || ctx.Err() != nil {
		return poolAccessError(false)
	}
	return nil
}
func (a *PoolSessionAuthority) begin(ctx context.Context) (context.Context, func(), error) {
	if a == nil || ctx == nil {
		return nil, nil, poolAccessError(false)
	}
	a.mu.Lock()
	if a.closing {
		a.mu.Unlock()
		return nil, nil, poolAccessError(false)
	}
	select {
	case a.gate <- struct{}{}:
	default:
		a.mu.Unlock()
		return nil, nil, &poolservice.ClientError{Code: "busy"}
	}
	a.mu.Unlock()
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(a.ctx, cancel)
	return c, func() { stop(); cancel(); <-a.gate }, nil
}
func (a *PoolSessionAuthority) withClient(ctx context.Context, mutation bool, fn func(context.Context, *poolservice.Client) error) error {
	c, end, e := a.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	if e = a.checkView(c); e != nil {
		return e
	}
	token, e := a.account.Account.MiningToken(c)
	if e != nil {
		return poolAccessError(false)
	}
	if e = a.checkToken(c, token); e != nil {
		return e
	}
	client, e := poolservice.NewClient(poolservice.ClientConfig{Version: a.config.Version, URL: a.config.URL, CACertificate: a.config.CACertificate, PublicKey: a.config.PublicKey, Source: a.config.Source, Token: token})
	if e != nil {
		return e
	}
	defer client.Close()
	if client.AuthorityOrigin() != a.config.URL || client.SourceConfig() != a.config.Source || client.PublicKey() != a.config.PublicKey || client.TransportTrustID() != a.config.TransportTrustID || client.ProtocolVersion() != poolservice.Version1 {
		return poolAccessError(false)
	}
	resultErr := fn(c, client)
	// Recheck the token actually used, never a replacement minted after reset.
	if a.checkToken(c, token) != nil || a.checkView(c) != nil {
		return poolAccessError(mutation)
	}
	return resultErr
}
func (a *PoolSessionAuthority) Issue(ctx context.Context) (poolshare.MiningWork, error) {
	var result poolshare.MiningWork
	e := a.withClient(ctx, true, func(c context.Context, client *poolservice.Client) (e error) { result, e = client.Issue(c); return e })
	if e != nil {
		return poolshare.MiningWork{}, e
	}
	return result, nil
}
func (a *PoolSessionAuthority) Work(ctx context.Context, id string) (poolshare.MiningWork, error) {
	var result poolshare.MiningWork
	e := a.withClient(ctx, false, func(c context.Context, client *poolservice.Client) (e error) {
		result, e = client.Work(c, id)
		return e
	})
	if e != nil {
		return poolshare.MiningWork{}, e
	}
	return result, nil
}
func (a *PoolSessionAuthority) Submit(ctx context.Context, w poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	var result poolshare.Submission
	e := a.withClient(ctx, true, func(c context.Context, client *poolservice.Client) (e error) {
		result, e = client.Submit(c, w, nonce)
		return e
	})
	if e != nil {
		return poolshare.Submission{}, e
	}
	return result, nil
}
func (a *PoolSessionAuthority) Lookup(ctx context.Context, w poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	var result poolshare.Submission
	e := a.withClient(ctx, false, func(c context.Context, client *poolservice.Client) (e error) {
		result, e = client.Lookup(c, w, nonce)
		return e
	})
	if e != nil {
		return poolshare.Submission{}, e
	}
	return result, nil
}
func (a *PoolSessionAuthority) Balance(ctx context.Context) (poolservice.BalanceResponse, error) {
	var result poolservice.BalanceResponse
	e := a.withClient(ctx, false, func(c context.Context, client *poolservice.Client) (e error) { result, e = client.Balance(c); return e })
	if e != nil {
		return poolservice.BalanceResponse{}, e
	}
	return result, nil
}
func (a *PoolSessionAuthority) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	if a.closing {
		done := a.done
		a.mu.Unlock()
		<-done
		return nil
	}
	a.closing = true
	a.cancel()
	a.mu.Unlock()
	a.gate <- struct{}{}
	<-a.gate
	close(a.done)
	return nil
}

var _ poolservice.Authority = (*PoolSessionAuthority)(nil)
