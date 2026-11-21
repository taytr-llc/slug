//go:build slugdailyactual && !slugtestpow && (linux || (darwin && cgo))

package poolsettle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type lostBroadcast struct {
	Raw  string `json:"raw"`
	TxID string `json:"txid"`
	AtMS int64  `json:"at_ms"`
}
type rpcRelay struct {
	hold bool
	held chan lostBroadcast

	url             string
	server          *http.Server
	listener        net.Listener
	done            chan error
	client          *http.Client
	mu              sync.Mutex
	calls           []lostBroadcast
	attempts        []lostBroadcast
	attemptOverflow bool
	lost            chan lostBroadcast
	drop            bool
	handlers        sync.WaitGroup
	closing         bool
	closeError      error
}

func startRPCRelay(ctx context.Context, upstream string) *rpcRelay {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	check(e, "RPC relay listen")
	r := &rpcRelay{url: "http://" + l.Addr().String(), listener: l, done: make(chan error, 1), lost: make(chan lostBroadcast, 1), drop: false, hold: false, held: make(chan lostBroadcast, 1), client: &http.Client{Transport: &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 65536}, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	r.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return ctx }}
	r.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		r.mu.Lock()
		if r.closing {
			r.mu.Unlock()
			http.Error(w, "closed", 503)
			return
		}
		r.handlers.Add(1)
		r.mu.Unlock()
		defer r.handlers.Done()
		body, e := io.ReadAll(io.LimitReader(q.Body, 1<<20))
		if e != nil || len(body) >= 1<<20 {
			http.Error(w, "bounded request", 400)
			return
		}
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.Unmarshal(body, &req) != nil {
			http.Error(w, "request", 400)
			return
		}
		if req.Method == "sendrawtransaction" {
			var rawHex string
			if len(req.Params) == 0 || json.Unmarshal(req.Params[0], &rawHex) != nil || len(rawHex) == 0 || len(rawHex) > 2*400000 {
				r.mu.Lock()
				r.attemptOverflow = true
				r.mu.Unlock()
				http.Error(w, "invalid bounded send attempt", 400)
				return
			}
			r.mu.Lock()
			if len(r.attempts) >= 16 {
				r.attemptOverflow = true
				r.mu.Unlock()
				http.Error(w, "send attempt cap", 503)
				return
			}
			attempt := lostBroadcast{Raw: rawHex, AtMS: time.Now().UnixMilli()}
			r.attempts = append(r.attempts, attempt)
			held := r.hold
			r.mu.Unlock()
			if held {
				select {
				case r.held <- attempt:
				default:
				}
				http.Error(w, "synthetic pre-forward payout hold", 503)
				return
			}
		}
		up, e := http.NewRequestWithContext(q.Context(), http.MethodPost, upstream, bytes.NewReader(body))
		if e != nil {
			http.Error(w, "upstream", 502)
			return
		}
		up.SetBasicAuth("fixture", "public-pool-actual")
		up.Header.Set("Content-Type", "application/json")
		res, e := r.client.Do(up)
		if e != nil {
			http.Error(w, "upstream", 502)
			return
		}
		reply, e := io.ReadAll(io.LimitReader(res.Body, (9<<20)+1))
		ce := res.Body.Close()
		if e != nil || ce != nil || len(reply) > 9<<20 {
			http.Error(w, "reply", 502)
			return
		}
		if req.Method == "sendrawtransaction" && res.StatusCode == 200 {
			var envelope struct {
				Result string          `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			var rawHex string
			if json.Unmarshal(reply, &envelope) == nil && bytes.Equal(bytes.TrimSpace(envelope.Error), []byte("null")) && len(req.Params) > 0 && json.Unmarshal(req.Params[0], &rawHex) == nil {
				observed := lostBroadcast{Raw: rawHex, TxID: envelope.Result, AtMS: time.Now().UnixMilli()}
				r.mu.Lock()
				r.calls = append(r.calls, observed)
				drop := r.drop
				r.drop = false
				r.mu.Unlock()
				if drop {
					r.lost <- observed
					if h, ok := w.(http.Hijacker); ok {
						conn, _, he := h.Hijack()
						if he == nil {
							if closeErr := conn.Close(); closeErr != nil {
								r.mu.Lock()
								r.closeError = errors.Join(r.closeError, closeErr)
								r.mu.Unlock()
							}
							return
						}
					}
					http.Error(w, "accepted response intentionally lost", 502)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(reply)
	})
	go func() { r.done <- r.server.Serve(l) }()
	return r
}
func (r *rpcRelay) sends() []lostBroadcast {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]lostBroadcast(nil), r.calls...)
}
func (r *rpcRelay) attempted() ([]lostBroadcast, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]lostBroadcast(nil), r.attempts...), r.attemptOverflow
}
func assertAttempts(r *rpcRelay, original string) {
	attempts, overflow := r.attempted()
	require(!overflow && len(attempts) > 0 && len(attempts) <= 16, "bounded attempted send inventory")
	for _, attempt := range attempts {
		require(attempt.Raw == original, "every attempted broadcast retains exact original signed bytes")
	}
}
func (r *rpcRelay) close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.closing = true
	r.mu.Unlock()
	e := r.server.Close()
	se := <-r.done
	r.handlers.Wait()
	r.client.CloseIdleConnections()
	if errors.Is(se, http.ErrServerClosed) {
		se = nil
	}
	r.mu.Lock()
	ce := r.closeError
	r.mu.Unlock()
	return errors.Join(e, se, ce)
}
