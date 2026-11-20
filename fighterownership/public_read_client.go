// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// PublicReadConfig selects a separately retained public endpoint. TLS uses
// normal WebPKI verification AND the expected leaf SPKI hash. Plain HTTP is
// limited to literal loopback addresses for the local candidate.
type PublicReadConfig struct {
	URL           string
	TLSSPKISHA256 string
}

type PublicReader struct {
	codec           ownershipCodec
	pin             Pin
	pinID, endpoint string
	http            *http.Client
	life            context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
	closed          atomic.Bool
	calls           sync.WaitGroup
	active          chan struct{}
	nextID          atomic.Uint64
	closeOnce       sync.Once
}

func NewPublicReader(ctx context.Context, expected Pin, config PublicReadConfig, life context.Context) (*PublicReader, error) {
	return newSelectedPublicReader(ctx, expected, config, life, ownershipCodec{})
}
func newSelectedPublicReader(ctx context.Context, expected Pin, config PublicReadConfig, life context.Context, codec ownershipCodec) (*PublicReader, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if ctx == nil || life == nil {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if e := life.Err(); e != nil {
		return nil, e
	}
	id, e := codec.ID(expected)
	if e != nil {
		return nil, e
	}
	u, e := url.Parse(config.URL)
	if e != nil || len(config.URL) > 2048 || u.User != nil || u.Opaque != "" || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery || u.RawPath != "" || u.Path != codec.readPath() || u.Hostname() == "" || strings.ContainsAny(config.URL, "\r\n\t") {
		return nil, ErrInvalid
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 || strconv.Itoa(n) != p {
			return nil, ErrInvalid
		}
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	switch u.Scheme {
	case "http":
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() || config.TLSSPKISHA256 != "" {
			return nil, ErrSource
		}
	case "https":
		if !hexValue(config.TLSSPKISHA256, 32, true) {
			return nil, ErrSource
		}
		want := config.TLSSPKISHA256
		tlsConfig.VerifyConnection = func(c tls.ConnectionState) error {
			if len(c.VerifiedChains) == 0 || len(c.PeerCertificates) == 0 {
				return ErrSource
			}
			h := sha256.Sum256(c.PeerCertificates[0].RawSubjectPublicKeyInfo)
			if hex.EncodeToString(h[:]) != want {
				return ErrSource
			}
			return nil
		}
	default:
		return nil, ErrSource
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSClientConfig: tlsConfig, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 8192, DisableCompression: true, DisableKeepAlives: true}
	owned, cancel := context.WithCancel(life)
	r := &PublicReader{codec: codec, pin: expected, pinID: id, endpoint: u.String(), life: owned, cancel: cancel, active: make(chan struct{}, 8), http: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if _, e = r.Status(ctx); e != nil {
		_ = r.Close()
		return nil, e
	}
	return r, nil
}
func (r *PublicReader) Pin() Pin {
	if r == nil {
		return Pin{}
	}
	return r.pin
}
func (r *PublicReader) Alive() error {
	if r == nil || r.life == nil || r.http == nil || r.closed.Load() || r.life.Err() != nil {
		return ErrClosed
	}
	return nil
}
func (r *PublicReader) begin(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		return nil, nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	if e := r.Alive(); e != nil {
		return nil, nil, e
	}
	r.mu.Lock()
	if e := r.Alive(); e != nil {
		r.mu.Unlock()
		return nil, nil, e
	}
	select {
	case r.active <- struct{}{}:
	default:
		r.mu.Unlock()
		return nil, nil, ErrBusy
	}
	r.calls.Add(1)
	r.mu.Unlock()
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	stop := context.AfterFunc(r.life, cancel)
	return op, func() { stop(); cancel(); <-r.active; r.calls.Done() }, nil
}
func (r *PublicReader) Close() error {
	if r == nil || r.life == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed.Store(true)
		r.cancel()
		r.mu.Unlock()
		r.calls.Wait()
		r.http.CloseIdleConnections()
	})
	return nil
}
func (r *PublicReader) read(ctx context.Context, method string, sequence uint32, event string) (json.RawMessage, error) {
	var id uint64
	for {
		old := r.nextID.Load()
		if old == math.MaxUint64 {
			r.cancel()
			return nil, ErrCapacity
		}
		if r.nextID.CompareAndSwap(old, old+1) {
			id = old + 1
			break
		}
	}
	v := publicReadRequest{Version: r.codec.wireVersion(), PinID: r.pinID, ID: id, Method: method, Sequence: sequence, EventID: event}
	if e := v.validateSelected(r.pinID, r.codec.wireVersion()); e != nil {
		return nil, e
	}
	raw, e := json.Marshal(v)
	if e != nil {
		return nil, ErrInvalid
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(raw))
	if e != nil {
		return nil, ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, e := r.http.Do(req)
	if e != nil {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	limit := publicReadLimit(method)
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/json" || response.Header.Get("Content-Encoding") != "" || response.ContentLength > int64(limit+2048) {
		return nil, ErrUnavailable
	}
	raw, e = io.ReadAll(io.LimitReader(response.Body, int64(limit+2049)))
	if e != nil || len(raw) > limit+2048 {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		return nil, ErrUnavailable
	}
	var out publicReadResponse
	if decode(raw, limit+2048, &out) != nil || out.Version != r.codec.wireVersion() || out.PinID != r.pinID || out.ID != id || out.Method != method {
		return nil, ErrUnavailable
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if e = r.Alive(); e != nil {
		return nil, e
	}
	if out.Error != "" {
		if !bytes.Equal(out.Result, []byte("null")) {
			return nil, ErrUnavailable
		}
		return nil, parsePublicReadError(out.Error)
	}
	if len(out.Result) == 0 || len(out.Result) > limit || bytes.Equal(out.Result, []byte("null")) {
		return nil, ErrUnavailable
	}
	return out.Result, nil
}
func (r *PublicReader) Status(ctx context.Context) (PublicReadStatus, error) {
	ctx, done, e := r.begin(ctx)
	if e != nil {
		return PublicReadStatus{}, e
	}
	defer done()
	raw, e := r.read(ctx, "status", 0, "")
	var v PublicReadStatus
	if e == nil {
		e = decode(raw, 2048, &v)
	}
	if e == nil {
		e = r.codec.validatePublicReadStatus(r.pin, v)
	}
	if e == nil {
		e = ctx.Err()
	}
	if e == nil {
		e = r.Alive()
	}
	return v, e
}
func (r *PublicReader) Admission(ctx context.Context, n uint32) (AdmissionStatement, error) {
	ctx, done, e := r.begin(ctx)
	if e != nil {
		return AdmissionStatement{}, e
	}
	defer done()
	raw, e := r.read(ctx, "admission", n, "")
	if e != nil {
		return AdmissionStatement{}, e
	}
	v, e := r.codec.ParsePublicAdmission(r.pin, raw)
	if e == nil && v.AdmissionSequence != n {
		e = ErrInvalid
	}
	if e == nil {
		e = ctx.Err()
	}
	if e == nil {
		e = r.Alive()
	}
	return v, e
}
func (r *PublicReader) Publication(ctx context.Context, n uint32) (PublicationEntry, error) {
	ctx, done, e := r.begin(ctx)
	if e != nil {
		return PublicationEntry{}, e
	}
	defer done()
	raw, e := r.read(ctx, "publication", n, "")
	if e != nil {
		return PublicationEntry{}, e
	}
	v, e := r.codec.ParsePublicPublication(r.pin, raw)
	if e == nil && v.Sequence != n {
		e = ErrInvalid
	}
	if e == nil {
		e = ctx.Err()
	}
	if e == nil {
		e = r.Alive()
	}
	return v, e
}
func (r *PublicReader) FinalityFrame(ctx context.Context, n uint32) (FinalityFrame, error) {
	ctx, done, e := r.begin(ctx)
	if e != nil {
		return FinalityFrame{}, e
	}
	defer done()
	raw, e := r.read(ctx, "finality_frame", n, "")
	var v FinalityFrame
	if e == nil {
		e = decode(raw, 8192, &v)
	}
	if e == nil {
		e = r.codec.ValidateFinalityFrame(r.pin, v)
	}
	if e == nil && v.Sequence != n {
		e = ErrInvalid
	}
	if e == nil {
		e = ctx.Err()
	}
	if e == nil {
		e = r.Alive()
	}
	return v, e
}
func (r *PublicReader) CutoffIntent(ctx context.Context, event string) (CutoffIntent, error) {
	ctx, done, e := r.begin(ctx)
	if e != nil {
		return CutoffIntent{}, e
	}
	defer done()
	raw, e := r.read(ctx, "cutoff_intent", 0, event)
	var v CutoffIntent
	if e == nil {
		e = decode(raw, 4096, &v)
	}
	if e == nil {
		e = r.codec.ValidatePublicCutoffIntent(r.pin, v)
	}
	if e == nil && v.EventID != event {
		e = ErrInvalid
	}
	if e == nil {
		e = ctx.Err()
	}
	if e == nil {
		e = r.Alive()
	}
	return v, e
}
func (r *PublicReader) Cutoff(ctx context.Context, event string) (CutoffRecord, error) {
	ctx, done, e := r.begin(ctx)
	if e != nil {
		return CutoffRecord{}, e
	}
	defer done()
	raw, e := r.read(ctx, "cutoff", 0, event)
	if e != nil {
		return CutoffRecord{}, e
	}
	v, e := r.codec.ParsePublicCutoff(r.pin, raw)
	if e == nil && v.EventID != event {
		e = ErrInvalid
	}
	if e == nil {
		e = ctx.Err()
	}
	if e == nil {
		e = r.Alive()
	}
	return v, e
}
