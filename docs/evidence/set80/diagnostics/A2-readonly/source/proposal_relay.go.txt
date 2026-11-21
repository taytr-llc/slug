package main

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

// Original-node forwarding with one acknowledged post-issuer proposal barrier.
// Every response is original node evidence; no fabricated/cached authority.
type observedSend struct {
	Raw  string `json:"raw"`
	TxID string `json:"txid"`
	AtMS int64  `json:"at_ms"`
}
type proposalBarrier struct {
	Block  string `json:"block"`
	SHA256 string `json:"sha256"`
	AtMS   int64  `json:"at_ms"`
}
type rpcRelay struct {
	url             string
	server          *http.Server
	listener        net.Listener
	done            chan error
	client          *http.Client
	mu              sync.Mutex
	active          sync.WaitGroup
	closing         bool
	calls, attempts []observedSend
	failure         error
	proposal        chan proposalBarrier
	proposalDone    chan struct{}
	armed, captured bool
}

func startRPCRelay(ctx context.Context, upstream string) *rpcRelay {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	check(e, "relay listen")
	r := &rpcRelay{proposal: make(chan proposalBarrier, 1), proposalDone: make(chan struct{}), url: "http://" + l.Addr().String(), listener: l, done: make(chan error, 1), client: &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 65536}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	r.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return ctx }}
	r.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		r.mu.Lock()
		if r.closing {
			r.mu.Unlock()
			http.Error(w, "closed", 503)
			return
		}
		r.active.Add(1)
		r.mu.Unlock()
		defer r.active.Done()
		raw, e := io.ReadAll(io.LimitReader(q.Body, (1<<20)+1))
		if e != nil || len(raw) > 1<<20 {
			http.Error(w, "bound", 400)
			return
		}
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.Unmarshal(raw, &req) != nil {
			http.Error(w, "request", 400)
			return
		}
		var send string
		if req.Method == "sendrawtransaction" {
			if len(req.Params) < 1 || json.Unmarshal(req.Params[0], &send) != nil || len(send) > 800000 || send == "" {
				http.Error(w, "send", 400)
				return
			}
			r.mu.Lock()
			if len(r.attempts) >= 16 {
				r.failure = errors.New("send attempt bound")
				r.mu.Unlock()
				http.Error(w, "bound", 503)
				return
			}
			r.attempts = append(r.attempts, observedSend{Raw: send, AtMS: time.Now().UnixMilli()})
			r.mu.Unlock()
		}
		up, e := http.NewRequestWithContext(q.Context(), http.MethodPost, upstream, bytes.NewReader(raw))
		if e != nil {
			http.Error(w, "request", 502)
			return
		}
		up.SetBasicAuth("fixture", "public-pool-actual")
		up.Header.Set("Content-Type", "application/json")
		res, e := r.client.Do(up)
		if e != nil {
			http.Error(w, "upstream", 502)
			return
		}
		body, e := io.ReadAll(io.LimitReader(res.Body, (9<<20)+1))
		ce := res.Body.Close()
		if e != nil || ce != nil || len(body) > 9<<20 {
			http.Error(w, "reply", 502)
			return
		}
		if send != "" && res.StatusCode == 200 {
			var v struct {
				Result string          `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(body, &v) == nil && bytes.Equal(bytes.TrimSpace(v.Error), []byte("null")) {
				r.mu.Lock()
				r.calls = append(r.calls, observedSend{send, v.Result, time.Now().UnixMilli()})
				r.mu.Unlock()
			}
		}
		if req.Method == "getblocktemplate" && len(req.Params) == 1 {
			var v struct {
				Mode string `json:"mode"`
				Data string `json:"data"`
			}
			if json.Unmarshal(req.Params[0], &v) == nil && v.Mode == "proposal" {
				r.mu.Lock()
				armed := r.armed && !r.captured
				if armed {
					r.captured = true
				}
				r.mu.Unlock()
				if armed {
					defer close(r.proposalDone)
					var reply struct {
						ID     json.RawMessage `json:"id"`
						Result json.RawMessage `json:"result"`
						Error  json.RawMessage `json:"error"`
					}
					if len(req.ID) == 0 || len(req.ID) > 256 || bytes.Equal(bytes.TrimSpace(req.ID), []byte("null")) || res.StatusCode != 200 || json.Unmarshal(body, &reply) != nil || !bytes.Equal(bytes.TrimSpace(req.ID), bytes.TrimSpace(reply.ID)) || !bytes.Equal(bytes.TrimSpace(reply.Error), []byte("null")) || !bytes.Equal(bytes.TrimSpace(reply.Result), []byte("null")) || len(v.Data) == 0 || len(v.Data) > 800000 {
						r.mu.Lock()
						r.failure = errors.Join(r.failure, errors.New("original proposal refusal"))
						r.mu.Unlock()
						http.Error(w, "proposal", 502)
						return
					}
					ack := proposalBarrier{Block: v.Data, SHA256: hx(digest(dec(v.Data))), AtMS: time.Now().UnixMilli()}
					r.proposal <- ack
					// Parent kills/reaps original operator promptly. Never deliver this held
					// reply after the barrier; cancellation joins this exact handler.
					<-q.Context().Done()
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.StatusCode)
		if _, e = w.Write(body); e != nil && send != "" {
			r.mu.Lock()
			r.failure = errors.Join(r.failure, errors.New("send response write"))
			r.mu.Unlock()
		}
	})
	go func() { r.done <- r.server.Serve(l) }()
	return r
}
func (r *rpcRelay) sends() ([]observedSend, []observedSend, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]observedSend{}, r.calls...), append([]observedSend{}, r.attempts...), r.failure
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
	r.active.Wait()
	r.client.CloseIdleConnections()
	if errors.Is(se, http.ErrServerClosed) {
		se = nil
	}
	r.mu.Lock()
	f := r.failure
	r.mu.Unlock()
	return errors.Join(e, se, f)
}

func (r *rpcRelay) armProposal() {
	r.mu.Lock()
	defer r.mu.Unlock()
	require(!r.armed && !r.captured, "one original proposal barrier")
	r.armed = true
}
func (r *rpcRelay) proposalJoined(ctx context.Context) {
	select {
	case <-r.proposalDone:
	case <-ctx.Done():
		panic("held proposal handler not joined")
	}
}
