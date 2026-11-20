// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/registry"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// PublicReadServer borrows two original public-evidence owners. It does not
// create a listener or expose any mutation or locally verified state map.
type PublicReadServer struct {
	pin       Pin
	pinID     string
	publisher *registry.ManifestPublication
	store     *Store
	life      context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    atomic.Bool
	active    chan struct{}
	calls     sync.WaitGroup
	closeOnce sync.Once
}

func NewPublicReadServer(expected Pin, publisher *registry.ManifestPublication, store *Store, life context.Context) (*PublicReadServer, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if life == nil || publisher == nil || store == nil {
		return nil, ErrInvalid
	}
	if e := life.Err(); e != nil {
		return nil, e
	}
	id, e := ID(expected)
	if e != nil || publisher.Pin() != expected || store.Pin() != expected {
		return nil, ErrSource
	}
	owned, cancel := context.WithCancel(life)
	s := &PublicReadServer{pin: expected, pinID: id, publisher: publisher, store: store, life: owned, cancel: cancel, active: make(chan struct{}, 8)}
	if _, e = s.status(owned); e != nil {
		cancel()
		return nil, e
	}
	return s, nil
}
func (s *PublicReadServer) alive() error {
	if s == nil || s.life == nil || s.closed.Load() || s.life.Err() != nil {
		return ErrClosed
	}
	return s.publisher.Alive()
}
func (s *PublicReadServer) Close() error {
	if s == nil || s.life == nil {
		return nil
	}
	s.closeOnce.Do(func() { s.mu.Lock(); s.closed.Store(true); s.cancel(); s.mu.Unlock(); s.calls.Wait() })
	return nil
}
func (s *PublicReadServer) begin(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		return nil, nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	if e := s.alive(); e != nil {
		return nil, nil, e
	}
	s.mu.Lock()
	if e := s.alive(); e != nil {
		s.mu.Unlock()
		return nil, nil, e
	}
	select {
	case s.active <- struct{}{}:
	default:
		s.mu.Unlock()
		return nil, nil, ErrBusy
	}
	s.calls.Add(1)
	s.mu.Unlock()
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	stop := context.AfterFunc(s.life, cancel)
	return op, func() { stop(); cancel(); <-s.active; s.calls.Done() }, nil
}
func (s *PublicReadServer) status(ctx context.Context) (PublicReadStatus, error) {
	if e := ctx.Err(); e != nil {
		return PublicReadStatus{}, e
	}
	if e := s.alive(); e != nil {
		return PublicReadStatus{}, e
	}
	o := s.store
	if !o.mu.TryLock() {
		return PublicReadStatus{}, ErrBusy
	}
	defer o.mu.Unlock()
	if e := o.state(); e != nil {
		return PublicReadStatus{}, e
	}
	p, e := s.publisher.Status(ctx)
	if e != nil {
		return PublicReadStatus{}, e
	}
	head := zeroID
	if o.meta.FinalitySequence > 0 {
		body, ok := o.frameBodies[o.meta.FinalitySequence]
		if !ok {
			return PublicReadStatus{}, ErrUnavailable
		}
		head, e = objectDigest("SLUG fighter ownership finality body", body)
		if e != nil {
			return PublicReadStatus{}, e
		}
	}
	v := PublicReadStatus{Version: 1, PinID: s.pinID, Publication: p, FinalitySequence: o.meta.FinalitySequence, PublishedFinalitySequence: o.meta.PublishedFinalitySequence, FinalityBodyHead: head, FinalityHead: o.meta.FinalityHead}
	if e = validatePublicReadStatus(s.pin, v); e != nil {
		return PublicReadStatus{}, e
	}
	return v, nil
}
func (s *PublicReadServer) read(ctx context.Context, r publicReadRequest) (any, error) {
	if e := s.coreAlive(); e != nil {
		return nil, e
	}
	switch r.Method {
	case "status":
		return s.status(ctx)
	case "admission":
		return s.publisher.Admission(ctx, r.Sequence)
	case "publication":
		return s.publisher.Publication(ctx, r.Sequence)
	case "cutoff_intent":
		return s.publisher.CutoffIntent(ctx, r.EventID)
	case "cutoff":
		return s.publisher.Cutoff(ctx, r.EventID)
	case "finality_frame":
		o := s.store
		if !o.mu.TryLock() {
			return nil, ErrBusy
		}
		defer o.mu.Unlock()
		if e := o.state(); e != nil {
			return nil, e
		}
		frame, ok := o.frames[r.Sequence]
		if !ok || r.Sequence > o.meta.PublishedFinalitySequence {
			return nil, registry.ErrNotFound
		}
		if e := ValidateFinalityFrame(s.pin, frame); e != nil {
			return nil, e
		}
		return frame, nil
	default:
		return nil, ErrInvalid
	}
}

func (s *PublicReadServer) coreAlive() error {
	if !s.store.mu.TryLock() {
		return ErrBusy
	}
	defer s.store.mu.Unlock()
	return s.store.state()
}
func (s *PublicReadServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path != publicReadPath || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Content-Encoding") != "" || r.ContentLength > 1024 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	ctx, done, e := s.begin(r.Context())
	if e != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	defer done()
	// Cancel slow clients as well as store work. Read/Write deadlines belong to
	// the response controller so Close can join even a stalled request body.
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(5 * time.Second)
	if controller.SetReadDeadline(deadline) != nil || controller.SetWriteDeadline(deadline) != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	bodyClosed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { defer close(bodyClosed); _ = r.Body.Close() })
	defer func() {
		if !stop() {
			<-bodyClosed
		}
	}()
	raw, e := io.ReadAll(io.LimitReader(r.Body, 1025))
	if e != nil || len(raw) > 1024 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var request publicReadRequest
	if decode(raw, 1024, &request) != nil || request.validate(s.pinID) != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	value, e := s.read(ctx, request)
	var result json.RawMessage = []byte("null")
	if e == nil {
		result, e = json.Marshal(value)
		if e == nil && len(result) > publicReadLimit(request.Method) {
			e = ErrUnavailable
		}
	}
	if e == nil {
		e = ctx.Err()
	}
	if e == nil {
		e = s.alive()
	}
	if e == nil {
		e = s.coreAlive()
	}
	if e != nil {
		result = []byte("null")
	}
	response := publicReadResponse{Version: 1, PinID: s.pinID, ID: request.ID, Method: request.Method, Result: result, Error: publicReadError(e)}
	body, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
