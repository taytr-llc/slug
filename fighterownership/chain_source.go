// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// ChainObservation is the exact owned-node RPC snapshot. Neither this transport
// nor its public values establishes ownership or verifies normal header work.
type ChainObservation struct {
	Version    uint32 `json:"version"`
	InstanceID string `json:"instance_id"`
	Revision   string `json:"revision"`
	Hash       string `json:"hash"`
	Height     uint32 `json:"height"`
}
type ChainHeader struct {
	Height uint32
	Hash   string
	Raw    []byte
}

// OwnedChainSource is attached to one original parent lifetime and a literal
// loopback endpoint. Every operation rechecks network/genesis. Replay must also
// bracket its traversal with Check to detect restart, ABA and advancing tips.
type OwnedChainSource struct {
	pin                          Pin
	endpoint, username, password string
	http                         *http.Client
	ctx                          context.Context
	cancel                       context.CancelFunc
	mu                           sync.Mutex
	closed                       atomic.Bool
	calls                        sync.WaitGroup
	nextID                       atomic.Uint64
	closeOnce                    sync.Once
	closeDone                    chan struct{}
}

func NewOwnedChainSource(ctx context.Context, expected Pin, config prospectnode.RPCConfig, life context.Context) (*OwnedChainSource, error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil || life == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := life.Err(); err != nil {
		return nil, err
	}
	if registry.ValidatePublicManifestPin(expected) != nil {
		return nil, ErrSource
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, ErrInvalid
	}
	if _, err = prospectnode.ParseRPCConfig(raw); err != nil {
		return nil, ErrInvalid
	}
	u, err := url.Parse(config.URL)
	if err != nil || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return nil, ErrSource
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if config.CACertificate != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(config.CACertificate)) {
			return nil, ErrInvalid
		}
		tlsConfig.RootCAs = pool
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSClientConfig: tlsConfig, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 65536, DisableCompression: true, DisableKeepAlives: true}
	owned, cancel := context.WithCancel(life)
	s := &OwnedChainSource{pin: expected, endpoint: u.String(), username: config.Username, password: config.Password, ctx: owned, cancel: cancel, closeDone: make(chan struct{}), http: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	op, done, err := s.begin(ctx)
	if err == nil {
		err = s.identity(op)
		done()
	}
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}
func (s *OwnedChainSource) Alive() error {
	if s == nil || s.ctx == nil || s.http == nil || s.closed.Load() || s.ctx.Err() != nil {
		return ErrClosed
	}
	return nil
}
func (s *OwnedChainSource) begin(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		return nil, nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := s.Alive(); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	if err := s.Alive(); err != nil {
		s.mu.Unlock()
		return nil, nil, err
	}
	s.calls.Add(1)
	s.mu.Unlock()
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	stop := context.AfterFunc(s.ctx, cancel)
	return op, func() { stop(); cancel(); s.calls.Done() }, nil
}
func (s *OwnedChainSource) Close() error {
	if s == nil || s.ctx == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed.Store(true)
		s.cancel()
		s.mu.Unlock()
		s.calls.Wait()
		s.http.CloseIdleConnections()
		close(s.closeDone)
	})
	<-s.closeDone
	return nil
}
func chainHex(value string, n int, nonzero bool) bool {
	if len(value) != n*2 {
		return false
	}
	raw, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(raw) != value {
		return false
	}
	if nonzero {
		for _, v := range raw {
			if v != 0 {
				return true
			}
		}
		return false
	}
	return true
}
func chainObject(raw []byte, max int) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > max {
		return nil, ErrUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, ErrUnavailable
	}
	out := make(map[string]json.RawMessage)
	for d.More() {
		tok, err = d.Token()
		key, ok := tok.(string)
		if err != nil || !ok || len(key) > 64 || len(out) >= 8 {
			return nil, ErrUnavailable
		}
		if _, exists := out[key]; exists {
			return nil, ErrUnavailable
		}
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return nil, ErrUnavailable
		}
		out[key] = v
	}
	if tok, err = d.Token(); err != nil || tok != json.Delim('}') {
		return nil, ErrUnavailable
	}
	if _, err = d.Token(); !errors.Is(err, io.EOF) {
		return nil, ErrUnavailable
	}
	return out, nil
}
func (s *OwnedChainSource) call(ctx context.Context, method string, params []any, maxResult int) (json.RawMessage, error) {
	if err := s.Alive(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var id uint64
	for {
		previous := s.nextID.Load()
		if previous == math.MaxUint64 {
			s.cancel()
			return nil, ErrCapacity
		}
		if s.nextID.CompareAndSwap(previous, previous+1) {
			id = previous + 1
			break
		}
	}
	payload, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
	}{"1.0", id, method, params})
	if err != nil {
		return nil, ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, ErrInvalid
	}
	req.SetBasicAuth(s.username, s.password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Encoding") != "" {
		return nil, ErrUnavailable
	}
	maxEnvelope := maxResult + 1024
	if resp.ContentLength > int64(maxEnvelope) {
		return nil, ErrCapacity
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxEnvelope)+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	if len(raw) > maxEnvelope {
		return nil, ErrCapacity
	}
	fields, err := chainObject(raw, maxEnvelope)
	if err != nil {
		return nil, err
	}
	for key := range fields {
		if key != "id" && key != "result" && key != "error" && key != "jsonrpc" {
			return nil, ErrUnavailable
		}
	}
	var received uint64
	if json.Unmarshal(fields["id"], &received) != nil || received != id {
		return nil, ErrUnavailable
	}
	if version, ok := fields["jsonrpc"]; ok {
		var v string
		if json.Unmarshal(version, &v) != nil || (v != "1.0" && v != "2.0") {
			return nil, ErrUnavailable
		}
	}
	errRaw, ok := fields["error"]
	if !ok {
		return nil, ErrUnavailable
	}
	if !bytes.Equal(errRaw, []byte("null")) {
		e, err := chainObject(errRaw, 512)
		if err != nil || len(e) != 2 {
			return nil, ErrUnavailable
		}
		var code int
		var message string
		if json.Unmarshal(e["code"], &code) != nil || json.Unmarshal(e["message"], &message) != nil {
			return nil, ErrUnavailable
		}
		if method == "getchainobservation" && code == -32603 && message == "chain observation busy" {
			return nil, ErrBusy
		}
		return nil, ErrUnavailable
	}
	value, ok := fields["result"]
	// gettxout alone defines an exact null result as output absence. Its
	// caller still brackets identity/ABA and canonical recovery authority.
	if !ok || (method != "gettxout" && bytes.Equal(value, []byte("null"))) {
		return nil, ErrUnavailable
	}
	if len(value) > maxResult {
		return nil, ErrCapacity
	}
	return value, nil
}
func (s *OwnedChainSource) identity(ctx context.Context) error {
	raw, err := s.call(ctx, "getcurrentnet", []any{}, 32)
	if err != nil {
		return err
	}
	var network uint32
	if json.Unmarshal(raw, &network) != nil || network != s.pin.Source.Network {
		return ErrSource
	}
	genesis, err := s.hashAt(ctx, 0)
	if err != nil {
		return err
	}
	if genesis != s.pin.Source.Genesis {
		return ErrSource
	}
	return s.Alive()
}
func (s *OwnedChainSource) hashAt(ctx context.Context, height uint32) (string, error) {
	raw, err := s.call(ctx, "getblockhash", []any{height}, 66)
	if err != nil {
		return "", err
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || !chainHex(value, 32, false) {
		return "", ErrUnavailable
	}
	return value, nil
}
func validChainObservation(v ChainObservation) bool {
	n, err := strconv.ParseUint(v.Revision, 10, 64)
	return v.Version == 1 && v.Height <= math.MaxInt32 && chainHex(v.InstanceID, 32, true) && chainHex(v.Hash, 32, false) && err == nil && n != 0 && strconv.FormatUint(n, 10) == v.Revision
}
func (s *OwnedChainSource) observe(ctx context.Context) (ChainObservation, error) {
	for {
		raw, err := s.call(ctx, "getchainobservation", []any{}, 512)
		if errors.Is(err, ErrBusy) {
			timer := time.NewTimer(20 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ChainObservation{}, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if err != nil {
			return ChainObservation{}, err
		}
		var value ChainObservation
		if json.Unmarshal(raw, &value) != nil || !validChainObservation(value) {
			return ChainObservation{}, ErrUnavailable
		}
		canonical, err := json.Marshal(value)
		if err != nil || !bytes.Equal(raw, canonical) {
			return ChainObservation{}, ErrUnavailable
		}
		return value, nil
	}
}
func (s *OwnedChainSource) Observe(ctx context.Context) (ChainObservation, error) {
	op, done, err := s.begin(ctx)
	if err != nil {
		return ChainObservation{}, err
	}
	defer done()
	if err = s.identity(op); err != nil {
		return ChainObservation{}, err
	}
	value, err := s.observe(op)
	if err != nil {
		return ChainObservation{}, err
	}
	if err = s.identity(op); err != nil {
		return ChainObservation{}, err
	}
	return value, nil
}
func (s *OwnedChainSource) Check(ctx context.Context, expected ChainObservation) error {
	if !validChainObservation(expected) {
		return ErrInvalid
	}
	got, err := s.Observe(ctx)
	if err != nil {
		return err
	}
	if got != expected {
		return ErrStale
	}
	return nil
}
func (s *OwnedChainSource) rawHex(ctx context.Context, method string, params []any, limit int) ([]byte, error) {
	raw, err := s.call(ctx, method, params, 2*limit+2)
	if err != nil {
		return nil, err
	}
	// Direct framing avoids a second large unescaped JSON-string allocation.
	if len(raw) < 4 || raw[0] != '"' || raw[len(raw)-1] != '"' || (len(raw)-2)%2 != 0 {
		return nil, ErrUnavailable
	}
	raw = raw[1 : len(raw)-1]
	for _, v := range raw {
		if !(v >= '0' && v <= '9' || v >= 'a' && v <= 'f') {
			return nil, ErrUnavailable
		}
	}
	out := make([]byte, len(raw)/2)
	if _, err = hex.Decode(out, raw); err != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}
func (s *OwnedChainSource) HeaderAt(ctx context.Context, height uint32) (ChainHeader, error) {
	if height > math.MaxInt32 {
		return ChainHeader{}, ErrInvalid
	}
	op, done, err := s.begin(ctx)
	if err != nil {
		return ChainHeader{}, err
	}
	defer done()
	if err = s.identity(op); err != nil {
		return ChainHeader{}, err
	}
	hash, err := s.hashAt(op, height)
	if err != nil {
		return ChainHeader{}, err
	}
	raw, err := s.rawHex(op, "getblockheader", []any{hash, false}, 80)
	if err != nil {
		return ChainHeader{}, err
	}
	if len(raw) != 80 {
		return ChainHeader{}, ErrUnavailable
	}
	if err = s.identity(op); err != nil {
		return ChainHeader{}, err
	}
	return ChainHeader{height, hash, raw}, nil
}
func (s *OwnedChainSource) Block(ctx context.Context, hash string) ([]byte, error) {
	return s.namedBytes(ctx, "getblock", hash)
}
func (s *OwnedChainSource) Transaction(ctx context.Context, txid string) ([]byte, error) {
	return s.namedBytes(ctx, "getrawtransaction", txid)
}
func (s *OwnedChainSource) namedBytes(ctx context.Context, method, hash string) ([]byte, error) {
	if !chainHex(hash, 32, false) {
		return nil, ErrInvalid
	}
	op, done, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	if err = s.identity(op); err != nil {
		return nil, err
	}
	raw, err := s.rawHex(op, method, []any{hash, 0}, wire.MaxBlockPayload)
	if err != nil {
		return nil, err
	}
	if err = s.identity(op); err != nil {
		return nil, err
	}
	return raw, nil
}
