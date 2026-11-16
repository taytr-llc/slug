// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectservice

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/netutil"
	"golang.org/x/time/rate"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

const operationTimeout = 30 * time.Second

// Node is the existing trusted-node context boundary. It neither chooses the
// operator's prospect policy nor owns private gym keys.
type Node interface {
	Resolve(context.Context, prospectnode.Policy) (prospect.Context, error)
	RecheckContext(context.Context, prospect.Context) error
	Recheck(context.Context, prospect.Context, prospect.Proof) error
}

// AdmissionNode adds D69 observations without changing the v1 node contract.
// verified must come from the registry's own production proof verification.
type AdmissionNode interface {
	RecheckAdmission(context.Context, prospect.Context, prospect.Proof, prospect.Verified) (string, error)
}

func apiPath(version uint32, operation string) string {
	return "/v" + strconv.FormatUint(uint64(version), 10) + "/" + operation
}

type clientCredential struct {
	publicKey string
	digest    [32]byte
	issue     *rate.Limiter
	admit     *rate.Limiter
}

// Server exclusively owns access to the supplied Store until Run returns.
// It never closes the Store or Node; the caller closes those after Run drains.
// Operations are serialized with nonblocking admission, not an internal queue.
type Server struct {
	config    ServerConfig
	policy    prospectnode.Policy
	node      Node
	store     *registry.Store
	clients   []clientCredential
	active    chan struct{}
	poisoned  atomic.Bool
	lifecycle sync.Mutex
	closing   bool
	started   bool
	handlers  sync.WaitGroup
}

// NewServer validates configuration and an existing matching store. Protocol
// v1 requires schema 3; v2 explicitly opts into D69 with schema 4.
// It performs no network request, key generation, or database initialization.
func NewServer(config ServerConfig, policy prospectnode.Policy, node Node, store *registry.Store) (*Server, error) {
	if wire.TestPoW {
		return nil, errors.New("local prospect authority requires production PoW; slugtestpow is unsupported")
	}
	canonical, err := ValidateServerConfig(config)
	if err != nil {
		return nil, err
	}
	if node == nil || store == nil {
		return nil, errors.New("node and existing birth registry are required")
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return nil, errors.New("invalid operator policy")
	}
	policy, err = prospectnode.ParsePolicy(encoded)
	if err != nil {
		return nil, errors.New("invalid operator policy")
	}
	info, err := store.Info()
	if err != nil {
		return nil, errors.New("authority registry is unavailable")
	}
	expectedSchema := registry.Version3
	if canonical.Version == Version2 {
		expectedSchema = registry.Version4
		if _, ok := node.(AdmissionNode); !ok {
			return nil, errors.New("authority v2 requires D69 node observations")
		}
	}
	if info.Version != expectedSchema || info.Network != policy.Network || info.Genesis != policy.Genesis {
		return nil, errors.New("authority requires a matching registry schema and policy")
	}
	if info.ReadOnly {
		return nil, errors.New("authority requires a writable birth registry")
	}
	server := &Server{config: canonical, policy: policy, node: node, store: store, active: make(chan struct{}, 1)}
	for _, credential := range canonical.Clients {
		var digest [32]byte
		decoded, err := hex.DecodeString(credential.TokenSHA256)
		if err != nil || len(decoded) != len(digest) {
			return nil, errors.New("invalid API credential digest")
		}
		copy(digest[:], decoded)
		server.clients = append(server.clients, clientCredential{publicKey: credential.PublicKey, digest: digest,
			issue: rate.NewLimiter(1, 2), admit: rate.NewLimiter(4, 4)})
	}
	return server, nil
}

// Run listens only on the configured literal loopback address. onReady receives
// the actual HTTP origin, including the assigned port when Listen used port 0.
// Cancellation first stops new handlers, then closes HTTP connections, but Run
// always waits for actual handlers before returning. Argon and synchronous
// writes already in progress cannot be interrupted or assumed rolled back.
func (server *Server) Run(ctx context.Context, onReady func(string)) error {
	server.lifecycle.Lock()
	if server.started || server.closing {
		server.lifecycle.Unlock()
		return errors.New("authority server cannot be restarted")
	}
	server.started = true
	server.lifecycle.Unlock()
	if ctx.Err() != nil {
		server.stopAdmission()
		server.handlers.Wait()
		return nil
	}
	listener, err := net.Listen("tcp", server.config.Listen)
	if err != nil {
		server.stopAdmission()
		server.handlers.Wait()
		return errors.New("could not listen on local authority address")
	}
	origin := "http://" + listener.Addr().String()
	base, cancel := context.WithCancel(ctx)
	defer cancel()
	httpServer := &http.Server{
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 35 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8 * 1024,
		BaseContext: func(net.Listener) context.Context { return base },
		ErrorLog:    log.New(io.Discard, "", 0),
	}
	completed := make(chan error, 1)
	go func() { completed <- httpServer.Serve(netutil.LimitListener(listener, 16)) }()
	if onReady != nil {
		onReady(origin)
	}
	var serveErr error
	stopped := false
	select {
	case serveErr = <-completed:
		stopped = true
	case <-ctx.Done():
	}
	server.stopAdmission()
	cancel()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	if err := httpServer.Shutdown(shutdown); err != nil {
		_ = httpServer.Close()
	}
	stop()
	if !stopped {
		serveErr = <-completed
	}
	// Add and the closing transition share a mutex. After stopAdmission there
	// can be no new tracked handler touching the Store, even after forced Close.
	server.handlers.Wait()
	if ctx.Err() != nil || errors.Is(serveErr, http.ErrServerClosed) {
		return nil
	}
	return errors.New("local authority listener stopped unexpectedly")
}

func (server *Server) stopAdmission() {
	server.lifecycle.Lock()
	server.closing = true
	server.lifecycle.Unlock()
}

func (server *Server) beginHandler() bool {
	server.lifecycle.Lock()
	defer server.lifecycle.Unlock()
	if server.closing {
		return false
	}
	server.handlers.Add(1)
	return true
}

func validHost(request *http.Request) bool {
	host, port, err := net.SplitHostPort(request.Host)
	if err != nil || strings.Contains(host, "%") {
		return false
	}
	ip := net.ParseIP(host)
	number, err := strconv.ParseUint(port, 10, 16)
	if ip == nil || !ip.IsLoopback() || err != nil || number == 0 || strconv.FormatUint(number, 10) != port {
		return false
	}
	if local, ok := request.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		localHost, localPort, err := net.SplitHostPort(local.String())
		if err != nil || localPort != port || !ip.Equal(net.ParseIP(localHost)) {
			return false
		}
	}
	return true
}

func (server *Server) requestShape(request *http.Request) bool {
	if request.Method != http.MethodPost || (request.URL.Path != apiPath(server.config.Version, "jobs") && request.URL.Path != apiPath(server.config.Version, "admissions")) ||
		request.URL.RawQuery != "" || request.URL.ForceQuery || request.URL.IsAbs() || request.URL.Host != "" ||
		request.URL.RawPath != "" || len(request.Header.Values("Content-Encoding")) != 0 {
		return false
	}
	values := request.Header.Values("Content-Type")
	if len(values) != 1 {
		return false
	}
	kind, params, err := mime.ParseMediaType(values[0])
	if err != nil || kind != "application/json" {
		return false
	}
	for key, value := range params {
		if key != "charset" || !strings.EqualFold(value, "utf-8") {
			return false
		}
	}
	return true
}

func (server *Server) authenticate(request *http.Request) *clientCredential {
	values := request.Header.Values("Authorization")
	if len(values) != 1 || len(values[0]) != len("Bearer ")+64 || !strings.HasPrefix(values[0], "Bearer ") {
		return nil
	}
	encoded := values[0][len("Bearer "):]
	var token [32]byte
	if _, err := hex.Decode(token[:], []byte(encoded)); err != nil {
		return nil
	}
	digest := sha256.Sum256(token[:])
	matched := -1
	for i := range server.clients {
		if subtle.ConstantTimeCompare(digest[:], server.clients[i].digest[:]) == 1 {
			matched = i
		}
	}
	if matched < 0 {
		return nil
	}
	return &server.clients[matched]
}

type responseError struct {
	code    string
	status  int
	unknown bool
}

func apiError(code string, status int) *responseError {
	return &responseError{code: code, status: status}
}

func (server *Server) writeFailure(writer http.ResponseWriter, failure *responseError) {
	data, err := encodeJSON(ErrorResponse{Version: server.config.Version, Error: APIError{Code: failure.code, OutcomeUnknown: failure.unknown}})
	if err != nil {
		// This is a fixed public envelope, never input or a backend error.
		data = []byte("{\"version\":" + strconv.FormatUint(uint64(server.config.Version), 10) + ",\"error\":{\"code\":\"unavailable\",\"outcome_unknown\":false}}\n")
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(failure.status)
	_, _ = writer.Write(data)
}

// ServeHTTP accepts only two strict public-input POST operations. The bearer
// credential grants local API access for one key, never signing authority.
func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if !server.beginHandler() {
		server.writeFailure(writer, apiError("unavailable", 503))
		return
	}
	defer server.handlers.Done()
	if !validHost(request) || len(request.Header.Values("Origin")) != 0 {
		server.writeFailure(writer, apiError("forbidden", 403))
		return
	}
	if !server.requestShape(request) {
		server.writeFailure(writer, apiError("invalid_request", 400))
		return
	}
	credential := server.authenticate(request)
	if credential == nil {
		server.writeFailure(writer, apiError("unauthorized", 401))
		return
	}
	limiter := credential.admit
	if request.URL.Path == apiPath(server.config.Version, "jobs") {
		limiter = credential.issue
	}
	if !limiter.Allow() {
		server.writeFailure(writer, apiError("rate_limited", 429))
		return
	}
	select {
	case server.active <- struct{}{}:
	default:
		server.writeFailure(writer, apiError("busy", 503))
		return
	}
	// Release before socket writes: a slow reader must not hold the operation
	// slot after synchronized work and bounded encoding have completed.
	data, failure := func() ([]byte, *responseError) {
		defer func() { <-server.active }()
		ctx, cancel := context.WithTimeout(request.Context(), operationTimeout)
		defer cancel()
		if server.poisoned.Load() {
			return nil, &responseError{"outcome_unknown", 503, true}
		}
		if ctx.Err() != nil {
			return nil, apiError("unavailable", 503)
		}
		if request.ContentLength > MaxRequestSize {
			return nil, apiError("too_large", 413)
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, MaxRequestSize+1))
		if len(body) > MaxRequestSize {
			return nil, apiError("too_large", 413)
		}
		if err != nil {
			return nil, apiError("invalid_request", 400)
		}
		if ctx.Err() != nil {
			return nil, apiError("unavailable", 503)
		}
		var result any
		var operationErr *responseError
		if request.URL.Path == apiPath(server.config.Version, "jobs") {
			result, operationErr = server.issue(ctx, credential, body)
		} else {
			result, operationErr = server.admit(ctx, credential, body)
		}
		if operationErr != nil {
			return nil, operationErr
		}
		encoded, err := encodeJSON(result)
		if err != nil || ctx.Err() != nil {
			// A synchronized mutation or historical lookup already succeeded.
			// Never disguise that outcome as a definite non-commit.
			return nil, &responseError{"outcome_unknown", 503, true}
		}
		return encoded, nil
	}()
	if failure != nil {
		server.writeFailure(writer, failure)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}

var (
	errPolicy   = errors.New("policy mismatch")
	errCapacity = errors.New("authority capacity reached")
	errNode     = errors.New("trusted node check failed")
)

func (server *Server) samePolicy(context prospect.Context) bool {
	return context.Version == server.policy.Version && context.Network == server.policy.Network &&
		strings.EqualFold(context.Genesis, server.policy.Genesis) && context.DNAVersion == server.policy.DNAVersion &&
		strings.EqualFold(context.ProspectTarget, server.policy.ProspectTarget)
}

func (server *Server) storeFailure(err error) *responseError {
	switch {
	case errors.Is(err, registry.ErrPoisoned):
		server.poisoned.Store(true)
		return &responseError{"outcome_unknown", 503, true}
	case errors.Is(err, errCapacity):
		return apiError("capacity", 503)
	case errors.Is(err, registry.ErrJobExpired):
		return apiError("expired", 409)
	case errors.Is(err, registry.ErrJobNotFound):
		return apiError("unknown_job", 404)
	case errors.Is(err, registry.ErrClock), errors.Is(err, registry.ErrClosed), errors.Is(err, registry.ErrReadOnly), errors.Is(err, registry.ErrCorrupt), errors.Is(err, errNode), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return apiError("unavailable", 503)
	default:
		return apiError("rejected", 422)
	}
}

func (server *Server) issue(ctx context.Context, credential *clientCredential, body []byte) (any, *responseError) {
	key, err := ParseIssueRequest(body)
	if err != nil {
		return nil, apiError("invalid_request", 400)
	}
	if !strings.EqualFold(key, credential.publicKey) {
		return nil, apiError("forbidden", 403)
	}
	info, err := server.store.Info()
	if err != nil {
		return nil, server.storeFailure(err)
	}
	if info.Jobs >= server.config.MaxJobs {
		return nil, apiError("capacity", 503)
	}
	resolved, err := server.node.Resolve(ctx, server.policy)
	if err != nil || ctx.Err() != nil {
		return nil, apiError("unavailable", 503)
	}
	if !server.samePolicy(resolved) {
		return nil, apiError("unavailable", 503)
	}
	job, err := server.store.IssueJob(resolved, credential.publicKey, func() error {
		if ctx.Err() != nil {
			return errNode
		}
		if err := server.node.RecheckContext(ctx, resolved); err != nil {
			return errNode
		}
		if ctx.Err() != nil {
			return errNode
		}
		return nil
	})
	if err != nil {
		return nil, server.storeFailure(err)
	}
	return job, nil
}

func (server *Server) admit(ctx context.Context, credential *clientCredential, body []byte) (any, *responseError) {
	proof, authorization, err := ParseSubmission(body)
	if err != nil || proof.Version != prospect.Version2 || authorization.Version != prospect.Version2 {
		return nil, apiError("invalid_request", 400)
	}
	if !strings.EqualFold(authorization.PublicKey, credential.publicKey) {
		return nil, apiError("forbidden", 403)
	}
	info, err := server.store.Info()
	if err != nil {
		return nil, server.storeFailure(err)
	}
	checkPolicy := func(initial prospect.Context) error {
		// Only new receipts invoke this guard. Historical recovery retains
		// the original policy, birth and eligibility even after node outage.
		if info.Records >= server.config.MaxRecords {
			return errCapacity
		}
		if !server.samePolicy(initial) {
			return errPolicy
		}
		if ctx.Err() != nil {
			return errNode
		}
		return nil
	}
	var record registry.Record
	var inserted bool
	if server.config.Version == Version1 {
		record, inserted, err = server.store.RecordJob(proof, authorization, func(initial prospect.Context, guardProof prospect.Proof) error {
			if err := checkPolicy(initial); err != nil {
				return err
			}
			if server.node.Recheck(ctx, initial, guardProof) != nil || ctx.Err() != nil {
				return errNode
			}
			return nil
		})
	} else {
		record, inserted, err = server.store.RecordJobChecked(proof, authorization, func(initial prospect.Context, guardProof prospect.Proof, verified prospect.Verified) (registry.Eligibility, error) {
			if err := checkPolicy(initial); err != nil {
				return registry.Eligibility{}, err
			}
			source, err := server.node.(AdmissionNode).RecheckAdmission(ctx, initial, guardProof, verified)
			if err != nil || ctx.Err() != nil {
				return registry.Eligibility{}, errNode
			}
			observation := registry.Eligibility{RuleVersion: registry.EligibilityRuleVersion2, Source: source}
			switch source {
			case "current-parent":
				observation.Hash, observation.Height = initial.ParentHash, initial.ParentHeight
			case "canonical-winning-block":
				observation.Hash, observation.Height = verified.WorkHash, initial.ParentHeight+1
			default:
				return registry.Eligibility{}, errNode
			}
			return observation, nil
		})
	}
	if err != nil {
		return nil, server.storeFailure(err)
	}
	source := "historical-receipt"
	if inserted {
		source = "trusted-node-snapshot"
	}
	return AdmissionResult{Version: server.config.Version, ContextSource: source, CurrentNodeChecked: inserted,
		Authentication: "gym-key-control", BirthRule: "job-issuance", Inserted: inserted, Record: record}, nil
}
