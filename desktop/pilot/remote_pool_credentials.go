// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/wire/v2"
)

const remotePoolCredentialLimit = 2048

var (
	errRemotePoolCredential            = errors.New("remote pool credential unavailable")
	errRemotePoolCredentialChanged     = errors.New("remote pool credential identity changed")
	errRemotePoolCredentialCleanup     = errors.New("remote pool credential cleanup incomplete")
	errRemotePoolCredentialUnsupported = errors.New("remote pool credential platform unsupported")
	errRemotePoolCredentialBusy        = errors.New("remote pool credential operation busy")
	errRemotePoolCredentialProfile     = errors.New("remote pool credentials require production PoW")
)

type remotePoolCredentialSelection struct{ PoolPath, AdmissionPath string }
type remotePoolCredentialRecord struct {
	Version     uint32 `json:"version"`
	ConfigID    string `json:"config_id"`
	GymID       string `json:"gym_id"`
	Audience    string `json:"audience"`
	Generation  uint32 `json:"generation"`
	ExpiresAtMS int64  `json:"expires_at_ms"`
	Token       string `json:"token"`
}
type remotePoolCredentialSlot struct {
	file    *remotePoolPrivateInput
	token   []byte
	expires int64
	lease   *remotePoolLease
}

// Private to the future concrete attachment owner: no token/client getter.
type remotePoolCredentials struct {
	recovery        *RecoverableRemotePoolConfig
	config          RemotePoolConfig
	configID        string
	worker          prospectworker.PoolConfig
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
	closing, failed bool
	clockMS         int64
	slots           [2]remotePoolCredentialSlot
	closeOnce       sync.Once
	closed          chan struct{}
	closeErr        error
}

func remotePoolParseCredential(raw []byte, id, gym, audience string) (remotePoolCredentialRecord, error) {
	return remotePoolParseSelectedCredential(raw, id, gym, audience, 1)
}
func remotePoolParseSelectedCredential(raw []byte, id, gym, audience string, version uint32) (remotePoolCredentialRecord, error) {
	var r remotePoolCredentialRecord
	if len(raw) < 2 || len(raw) > remotePoolCredentialLimit || raw[len(raw)-1] != '\n' ||
		ownedcodec.Decode(raw[:len(raw)-1], remotePoolCredentialLimit-1, &r) != nil ||
		(version != 1 && version != 2) || r.Version != version || r.ConfigID != id || r.GymID != gym || r.Audience != audience ||
		(audience != poolminer.PoolAudience && audience != poolminer.AdmissionAudience) ||
		r.Generation == 0 || r.ExpiresAtMS <= 0 || r.ExpiresAtMS > poolminer.MaxTimeMS || !remotePoolID(r.Token) {
		return remotePoolCredentialRecord{}, errRemotePoolCredential
	}
	return r, nil
}

func openRemotePoolCredentials(life context.Context, expected RemotePoolConfig, selection remotePoolCredentialSelection) (*remotePoolCredentials, error) {
	return openSelectedRemotePoolCredentials(life, expected, selection, false)
}
func openSelectedRemotePoolCredentials(life context.Context, expected RemotePoolConfig, selection remotePoolCredentialSelection, daily bool) (*remotePoolCredentials, error) {
	return openRemotePoolCredentialsSelection(life, expected, selection, daily, nil)
}
func openRemotePoolCredentialsSelection(life context.Context, expected RemotePoolConfig, selection remotePoolCredentialSelection, daily bool, recovery *RecoverableRemotePoolConfig) (out *remotePoolCredentials, resultErr error) {
	if wire.TestPoW {
		return nil, errRemotePoolCredentialProfile
	}
	if life == nil || life.Err() != nil {
		return nil, errRemotePoolCredential
	}
	raw, e := marshalSelectedRemotePoolConfig(expected, daily)
	if e != nil {
		return nil, e
	}
	config, e := parseSelectedRemotePoolConfig(raw, daily)
	if e != nil {
		return nil, e
	}
	id, e := selectedRemotePoolConfigID(config)
	if e != nil {
		return nil, e
	}
	worker, e := remotePoolWorkerConfig(config)
	if e != nil {
		return nil, e
	}
	if recovery != nil && (!daily || ValidateRecoverableRemotePoolConfig(*recovery) != nil || !sameRecoverableRemoteBase(*recovery, config)) {
		return nil, ErrRemotePoolConfig
	}
	ctx, cancel := context.WithCancel(life)
	o := &remotePoolCredentials{recovery: recovery, config: config, configID: id, worker: worker, ctx: ctx, cancel: cancel, closed: make(chan struct{})}
	defer func() {
		if resultErr == nil {
			return
		}
		ce := o.Close()
		resultErr = errors.Join(resultErr, ce)
		if ce != nil {
			out = o
		} else {
			out = nil
		}
	}()
	limit := remotePoolCredentialLimit
	if recovery != nil {
		limit += 1024
	}
	for n, path := range []string{selection.PoolPath, selection.AdmissionPath} {
		f, e := selectRemotePoolPrivateInput(ctx, path, limit)
		o.slots[n].file = f // Retain even failed partial descriptor construction.
		if e != nil {
			return nil, e
		}
		b, e := f.read(ctx)
		if e != nil {
			return nil, e
		}
		audience := poolminer.PoolAudience
		if n == 1 {
			audience = poolminer.AdmissionAudience
		}
		r, e := o.parseCredential(b, audience)
		clear(b)
		if e != nil {
			return nil, e
		}
		o.slots[n].token, o.slots[n].expires = []byte(r.Token), r.ExpiresAtMS
		r.Token = ""
	}
	if bytes.Equal(o.slots[0].token, o.slots[1].token) {
		return nil, errRemotePoolCredential
	}
	for i := range o.slots {
		if e := o.check(ctx, i); e != nil {
			return nil, e
		}
	}
	return o, nil
}

// The caller must hold an admitted lease, or be the unpublished constructor.
// Close cannot close descriptors until every registered lease has joined.
func (o *remotePoolCredentials) check(ctx context.Context, audience int) error {
	if o == nil || ctx == nil || audience < 0 || audience > 1 || ctx.Err() != nil {
		return errRemotePoolCredential
	}
	o.mu.Lock()
	now := time.Now().UnixMilli()
	bad := o.closing || o.failed || o.ctx.Err() != nil || now < o.clockMS || now >= o.slots[audience].expires
	if now < o.clockMS {
		o.failed = true
	}
	if now >= o.clockMS {
		o.clockMS = now
	}
	o.mu.Unlock()
	if bad {
		return errRemotePoolCredential
	}
	for i := range o.slots {
		if e := o.slots[i].file.check(ctx); e != nil {
			o.mu.Lock()
			o.failed = true
			o.mu.Unlock()
			return errRemotePoolCredentialChanged
		}
	}
	return ctx.Err()
}

type remotePoolLease struct {
	owner      *remotePoolCredentials
	audience   int
	ctx        context.Context
	cancel     context.CancelFunc
	caller     context.Context
	stopCaller func() bool
	ready      chan struct{}
	pool       *poolservice.Client
	admission  *prospectservice.ManifestClient
	mu         sync.Mutex
	closing    bool
	gate       chan struct{}
	active     sync.WaitGroup
	closeOnce  sync.Once
	closed     chan struct{}
}
type remotePoolPoolLease struct{ lease *remotePoolLease }
type remotePoolAdmissionLease struct{ lease *remotePoolLease }

func (o *remotePoolCredentials) begin(ctx context.Context, audience int) (*remotePoolLease, error) {
	if o == nil || ctx == nil || ctx.Err() != nil || audience < 0 || audience > 1 {
		return nil, errRemotePoolCredential
	}
	o.mu.Lock()
	if o.closing || o.failed || o.ctx.Err() != nil {
		o.mu.Unlock()
		return nil, errRemotePoolCredential
	}
	if o.slots[audience].lease != nil {
		o.mu.Unlock()
		return nil, errRemotePoolCredentialBusy
	}
	life, cancel := context.WithCancel(o.ctx)
	l := &remotePoolLease{owner: o, audience: audience, ctx: life, cancel: cancel, caller: ctx, ready: make(chan struct{}), gate: make(chan struct{}, 1), closed: make(chan struct{})}
	l.stopCaller = context.AfterFunc(ctx, cancel)
	o.slots[audience].lease = l // Register before any descriptor/client work.
	o.mu.Unlock()
	var e error
	func() {
		defer close(l.ready)
		if e = o.check(life, audience); e != nil {
			return
		}
		if audience == 0 {
			if o.config.Version == 2 {
				var dc poolservice.DailyClientConfig
				dc, e = remotePoolDailyClientConfig(o.config, string(o.slots[0].token))
				if e == nil {
					if o.recovery != nil {
						l.pool, e = poolservice.NewRecoverableDailyClient(poolservice.RecoverableDailyClientConfig{Version: 1, Base: dc, IssueProfile: o.recovery.IssueProfile})
					} else {
						l.pool, e = poolservice.NewDailyClient(dc)
					}
				}
			} else {
				l.pool, e = poolservice.NewClient(remotePoolClientConfig(o.config, string(o.slots[0].token)))
			}
		} else {
			l.admission, e = prospectservice.NewManifestClient(remotePoolAdmissionConfig(o.config, string(o.slots[1].token)), o.config.Source.Policy)
		}
		if e == nil {
			e = o.check(life, audience)
		}
		if e == nil {
			e = ctx.Err()
		}
	}()
	if e != nil {
		_ = l.Close()
		return nil, errRemotePoolCredential
	}
	return l, nil
}
func (o *remotePoolCredentials) beginPool(ctx context.Context) (*remotePoolPoolLease, error) {
	l, e := o.begin(ctx, 0)
	if e != nil {
		return nil, e
	}
	return &remotePoolPoolLease{l}, nil
}
func (o *remotePoolCredentials) beginAdmission(ctx context.Context) (*remotePoolAdmissionLease, error) {
	l, e := o.begin(ctx, 1)
	if e != nil {
		return nil, e
	}
	return &remotePoolAdmissionLease{l}, nil
}

func remotePoolLeaseCall[T any](ctx context.Context, l *remotePoolLease, mutation bool, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if l == nil || ctx == nil || ctx.Err() != nil {
		return zero, errRemotePoolCredential
	}
	l.mu.Lock()
	if l.closing || l.ctx.Err() != nil || l.caller.Err() != nil {
		l.mu.Unlock()
		return zero, errRemotePoolCredential
	}
	select {
	case l.gate <- struct{}{}:
	default:
		l.mu.Unlock()
		return zero, errRemotePoolCredentialBusy
	}
	l.active.Add(1)
	l.mu.Unlock()
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(l.ctx, cancel)
	defer func() { stop(); cancel(); <-l.gate; l.active.Done() }()
	if l.ctx.Err() != nil || l.caller.Err() != nil {
		return zero, errRemotePoolCredential
	}
	if e := l.owner.check(work, l.audience); e != nil {
		return zero, e
	}
	v, e := fn(work)
	if l.owner.check(work, l.audience) != nil || l.ctx.Err() != nil || l.caller.Err() != nil {
		if mutation {
			if l.audience == 1 {
				return zero, &prospectservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
			}
			return zero, &poolservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
		}
		return zero, errRemotePoolCredential
	}
	return v, e
}

func (l *remotePoolLease) Close() error {
	if l == nil {
		return nil
	}
	l.closeOnce.Do(func() {
		l.mu.Lock()
		l.closing = true
		l.mu.Unlock()
		l.cancel()
		<-l.ready
		l.active.Wait()
		l.stopCaller()
		if l.pool != nil {
			l.pool.Close()
			l.pool = nil
		}
		if l.admission != nil {
			l.admission.Close()
			l.admission = nil
		}
		l.owner.mu.Lock()
		if l.owner.slots[l.audience].lease == l {
			l.owner.slots[l.audience].lease = nil
		}
		l.owner.mu.Unlock()
		close(l.closed)
	})
	<-l.closed
	return nil
}
func (l *remotePoolPoolLease) Close() error {
	if l == nil {
		return nil
	}
	return l.lease.Close()
}
func (l *remotePoolAdmissionLease) Close() error {
	if l == nil {
		return nil
	}
	return l.lease.Close()
}

func (o *remotePoolCredentials) Close() error {
	if o == nil {
		return nil
	}
	o.closeOnce.Do(func() {
		o.mu.Lock()
		o.closing = true
		leases := [2]*remotePoolLease{o.slots[0].lease, o.slots[1].lease}
		o.mu.Unlock()
		o.cancel()
		for _, l := range leases {
			_ = l.Close()
		}
		for i := range o.slots {
			clear(o.slots[i].token)
			o.slots[i].token = nil
			if e := o.slots[i].file.close(); e != nil {
				o.closeErr = errRemotePoolCredentialCleanup
			}
		}
		close(o.closed)
	})
	<-o.closed
	return o.closeErr
}
