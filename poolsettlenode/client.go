// Copyright (c) 2026 The SLUG developers. ISC license.
// Package poolsettlenode owns exact node evidence and fee quotation, never keys.
package poolsettlenode

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

var (
	ErrInvalid     = errors.New("invalid pool settlement node configuration or request")
	ErrUnavailable = errors.New("pool settlement node evidence unavailable")
	ErrClosed      = errors.New("pool settlement node owner closed")
	ErrBusy        = errors.New("pool settlement node operation busy")
)

type Client struct {
	config                   Config
	endpoint, user, password string
	http                     *http.Client
	id                       atomic.Uint64
	life                     context.Context
	cancel                   context.CancelFunc
	mu                       sync.Mutex
	active, closing          bool
	users                    sync.WaitGroup
	done                     chan struct{}
}

func (*Client) String() string   { return "pool settlement node owner" }
func (*Client) GoString() string { return "pool settlement node owner" }

// New retains the original runtime lifetime. No request context or public input
// can choose another endpoint after creation. Constructor failure joins cleanup.
func New(life context.Context, config Config, rpc prospectnode.RPCConfig) (*Client, error) {
	if life == nil || ValidateConfig(config) != nil {
		return nil, ErrInvalid
	}
	if wire.TestPoW {
		return nil, poolsettle.ErrProfile
	}
	if err := life.Err(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(rpc)
	if err != nil {
		return nil, ErrInvalid
	}
	if _, err = prospectnode.ParseRPCConfig(raw); err != nil {
		return nil, ErrInvalid
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS12}
	if rpc.CACertificate != "" {
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(rpc.CACertificate)) {
			return nil, ErrInvalid
		}
		tc.RootCAs = roots
	}
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: 65536, TLSClientConfig: tc, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext}
	owned, cancel := context.WithCancel(life)
	b := &Client{config: config, endpoint: rpc.URL, user: rpc.Username, password: rpc.Password, life: owned, cancel: cancel, done: make(chan struct{}), http: &http.Client{Transport: tr, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	ctx, end, err := b.begin(life)
	if err == nil {
		_, err = b.snapshot(ctx)
		end()
	}
	if err != nil {
		_ = b.Close()
		return nil, err
	}
	return b, nil
}
func (b *Client) Config() Config {
	if b == nil {
		return Config{}
	}
	return b.config
}
func (b *Client) begin(ctx context.Context) (context.Context, func(), error) {
	if b == nil || ctx == nil || b.life == nil || b.done == nil {
		return nil, nil, ErrInvalid
	}
	if wire.TestPoW {
		return nil, nil, poolsettle.ErrProfile
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closing || b.life.Err() != nil {
		return nil, nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if b.active {
		return nil, nil, ErrBusy
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(b.life, cancel)
	b.active = true
	b.users.Add(1)
	return work, func() {
		stop()
		cancel()
		b.mu.Lock()
		b.active = false
		b.users.Done()
		b.mu.Unlock()
	}, nil
}
func (b *Client) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.life.Err() != nil {
		return ErrClosed
	}
	return nil
}
func (b *Client) Close() error {
	if b == nil {
		return nil
	}
	if b.life == nil || b.done == nil || b.cancel == nil {
		return ErrInvalid
	}
	b.mu.Lock()
	first := !b.closing
	if first {
		b.closing = true
		b.cancel()
	}
	b.mu.Unlock()
	if !first {
		<-b.done
		return nil
	}
	b.users.Wait()
	b.http.CloseIdleConnections()
	close(b.done)
	return nil
}
func (b *Client) Inspect(ctx context.Context, coins []poolsettle.Coin) ([]poolsettle.Funding, error) {
	work, end, err := b.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	out, err := b.inspect(work, coins)
	if err != nil {
		return nil, err
	}
	if err = b.check(work); err != nil {
		return nil, err
	}
	return out, nil
}
func (b *Client) ObserveTransaction(ctx context.Context, id string) (poolsettle.TransactionObservation, error) {
	work, end, err := b.begin(ctx)
	if err != nil {
		return poolsettle.TransactionObservation{}, err
	}
	defer end()
	out, err := b.observeTransaction(work, id)
	if err != nil {
		return poolsettle.TransactionObservation{}, err
	}
	if err = b.check(work); err != nil {
		return poolsettle.TransactionObservation{}, err
	}
	return out, nil
}

// Broadcast has no automatic retry. An uncertain response is reconciled by the
// original settlement journal, which retains the exact signed bytes.
func (b *Client) Broadcast(ctx context.Context, raw []byte) (string, error) {
	work, end, err := b.begin(ctx)
	if err != nil {
		return "", err
	}
	defer end()
	out, err := b.broadcast(work, raw)
	if err != nil {
		return "", err
	}
	if err = b.check(work); err != nil {
		return "", err
	}
	return out, nil
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*rpcError) Error() string { return "pool node RPC refused" }

func (b *Client) call(ctx context.Context, method string, params []any, out any) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrUnavailable
	}
	id := b.id.Add(1)
	if id == 0 {
		return ErrUnavailable
	}
	raw, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
	}{"1.0", id, method, params})
	if err != nil {
		return ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(raw))
	if err != nil {
		return ErrUnavailable
	}
	req.SetBasicAuth(b.user, b.password)
	req.Header.Set("Content-Type", "application/json")
	res, err := b.http.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Encoding") != "" {
		return ErrUnavailable
	}
	raw, err = io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 || uniqueJSON(raw) != nil {
		return ErrUnavailable
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 4 {
		return ErrUnavailable
	}
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      uint64          `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *rpcError       `json:"error"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&envelope) != nil || envelope.JSONRPC != "1.0" || envelope.ID != id || len(envelope.Result) == 0 || fields["error"] == nil {
		return ErrUnavailable
	}
	if envelope.Error != nil {
		if !bytes.Equal(bytes.TrimSpace(envelope.Result), []byte("null")) {
			return ErrUnavailable
		}
		return envelope.Error
	}
	if out == nil {
		return nil
	}
	if json.Unmarshal(envelope.Result, out) != nil {
		return ErrUnavailable
	}
	return nil
}

// Reject duplicate members and excessive nesting/tokens before typed decoding.
func uniqueJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return ErrUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	tokens := 0
	var visit func(int) error
	visit = func(depth int) error {
		tokens++
		if depth > 16 || tokens > 262144 {
			return ErrUnavailable
		}
		t, err := d.Token()
		if err != nil {
			return ErrUnavailable
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, err := d.Token()
					key, ok := k.(string)
					if err != nil || !ok || seen[key] || len(seen) >= 256 {
						return ErrUnavailable
					}
					seen[key] = true
					if err = visit(depth + 1); err != nil {
						return err
					}
				}
				t, err = d.Token()
				if err != nil || t != json.Delim('}') {
					return ErrUnavailable
				}
			case '[':
				for d.More() {
					if err = visit(depth + 1); err != nil {
						return err
					}
				}
				t, err = d.Token()
				if err != nil || t != json.Delim(']') {
					return ErrUnavailable
				}
			default:
				return ErrUnavailable
			}
		}
		return nil
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrUnavailable
	}
	return nil
}

// Required fields are checked before typed decoding so omitted zero values do
// not become evidence. Other genuine node result fields remain allowed.
func decodeFields(raw []byte, out any, required ...string) error {
	for _, name := range required {
		current := raw
		for _, component := range strings.Split(name, ".") {
			var fields map[string]json.RawMessage
			if json.Unmarshal(current, &fields) != nil {
				return ErrUnavailable
			}
			value, ok := fields[component]
			if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return ErrUnavailable
			}
			current = value
		}
	}
	if json.Unmarshal(raw, out) != nil {
		return ErrUnavailable
	}
	return nil
}
func (b *Client) callFields(ctx context.Context, method string, params []any, out any, required ...string) error {
	var raw json.RawMessage
	if err := b.call(ctx, method, params, &raw); err != nil {
		return err
	}
	return decodeFields(raw, out, required...)
}
func (b *Client) callNullableFields(ctx context.Context, method string, params []any, out any, required ...string) error {
	var raw json.RawMessage
	if err := b.call(ctx, method, params, &raw); err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return json.Unmarshal(raw, out)
	}
	return decodeFields(raw, out, required...)
}
