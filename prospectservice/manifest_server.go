// Copyright (c) 2026 The SLUG developers. ISC license.

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

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"golang.org/x/net/netutil"
	"golang.org/x/time/rate"
)

// ManifestServer owns exclusive access to the borrowed manifest store until
// Run drains. It has no account authority of its own; session mode delegates
// the complete account-before-registry lease without static fallback.
type ManifestServer struct {
	config    ManifestServerConfig
	policy    prospectnode.Policy
	node      ManifestNode
	store     *registry.ManifestStore
	clients   []clientCredential
	sessions  SessionAuthorizer
	clientMu  sync.Mutex
	dynamic   map[string]*clientCredential
	active    chan struct{}
	poisoned  atomic.Bool
	lifecycle sync.Mutex
	closing   bool
	started   bool
	handlers  sync.WaitGroup
}

// NewManifestServer selects static per-gym credentials for protocol 3.
func NewManifestServer(config ManifestServerConfig, policy prospectnode.Policy, node ManifestNode, store *registry.ManifestStore) (*ManifestServer, error) {
	return newManifestServer(config, policy, node, store, nil)
}

// NewManifestSessionServer selects revocable enrolled mining sessions. The
// explicit authorizer is required and static Clients must be empty.
func NewManifestSessionServer(config ManifestServerConfig, policy prospectnode.Policy, node ManifestNode, store *registry.ManifestStore, sessions SessionAuthorizer) (*ManifestServer, error) {
	if sessions == nil {
		return nil, errors.New("account session authorizer is required")
	}
	return newManifestServer(config, policy, node, store, sessions)
}

func newManifestServer(config ManifestServerConfig, policy prospectnode.Policy, node ManifestNode, store *registry.ManifestStore, sessions SessionAuthorizer) (*ManifestServer, error) {
	if wire.TestPoW {
		return nil, errors.New("manifest authority requires production PoW")
	}
	canonical, err := ValidateManifestServerConfig(config)
	if err != nil {
		return nil, err
	}
	if (sessions == nil && len(canonical.Clients) == 0) || (sessions != nil && len(canonical.Clients) != 0) {
		return nil, errManifestConfig
	}
	if node == nil || store == nil {
		return nil, errors.New("node and existing manifest authority are required")
	}
	policy, err = manifestPolicy(policy)
	if err != nil {
		return nil, err
	}
	descriptor, err := manifestPin(canonical.AuthorityPin)
	if err != nil || policy.Network != canonical.AuthorityPin.Network || policy.Genesis != canonical.AuthorityPin.Genesis || policy.DNAVersion != descriptor.DNAVersion {
		return nil, errManifestConfig
	}
	info, err := store.Info()
	if err != nil {
		return nil, errors.New("manifest authority is unavailable")
	}
	if info.ReadOnly || info.Pin != canonical.AuthorityPin || info.Descriptor != descriptor {
		return nil, errors.New("manifest authority requires an exact writable pin")
	}
	server := &ManifestServer{config: canonical, policy: policy, node: node, store: store, sessions: sessions, active: make(chan struct{}, 1), dynamic: make(map[string]*clientCredential)}
	for _, credential := range canonical.Clients {
		var digest [32]byte
		raw, _ := hex.DecodeString(credential.TokenSHA256)
		copy(digest[:], raw)
		server.clients = append(server.clients, clientCredential{publicKey: credential.PublicKey, digest: digest, issue: rate.NewLimiter(1, 2), admit: rate.NewLimiter(4, 4)})
	}
	return server, nil
}

// Bound the typed configuration before marshaling caller-owned strings.
func manifestPolicy(policy prospectnode.Policy) (prospectnode.Policy, error) {
	if len(policy.Genesis) != 64 || len(policy.ProspectTarget) != 64 {
		return prospectnode.Policy{}, errManifestConfig
	}
	raw, _ := json.Marshal(policy)
	result, err := prospectnode.ParsePolicy(raw)
	if err != nil {
		return prospectnode.Policy{}, errManifestConfig
	}
	return result, nil
}

// Run listens only on the configured literal loopback address. onReady receives
// the actual HTTP origin, including the assigned port when Listen used port 0.
// Cancellation first stops new handlers, then closes HTTP connections, but Run
// always waits for actual handlers before returning. Argon and synchronous
// writes already in progress cannot be interrupted or assumed rolled back.
func (server *ManifestServer) Run(ctx context.Context, onReady func(string)) error {
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

func (server *ManifestServer) stopAdmission() {
	server.lifecycle.Lock()
	server.closing = true
	server.lifecycle.Unlock()
}

func (server *ManifestServer) beginHandler() bool {
	server.lifecycle.Lock()
	defer server.lifecycle.Unlock()
	if server.closing {
		return false
	}
	server.handlers.Add(1)
	return true
}

func (server *ManifestServer) requestShape(request *http.Request) bool {
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

func (server *ManifestServer) authenticate(request *http.Request) *clientCredential {
	encoded := bearerToken(request)
	if encoded == "" {
		return nil
	}
	if server.sessions != nil {
		var credential *clientCredential
		err := server.sessions.WithSession(request.Context(), encoded, func(key string, recheck func() error) error {
			canonical, err := normalizePublicKey(key)
			if err != nil || recheck == nil || recheck() != nil {
				return errSession
			}
			server.clientMu.Lock()
			defer server.clientMu.Unlock()
			credential = server.dynamic[canonical]
			if credential == nil {
				// A private fixture authority has a bounded identity set. Token
				// rotation must not reset a gym's admission rate allowance.
				if len(server.dynamic) >= 64 {
					return errSession
				}
				credential = &clientCredential{publicKey: canonical, issue: rate.NewLimiter(1, 2), admit: rate.NewLimiter(4, 4)}
				server.dynamic[canonical] = credential
			}
			return nil
		})
		if err != nil {
			return nil
		}
		return credential
	}
	var token [32]byte
	_, _ = hex.Decode(token[:], []byte(encoded))
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

func (server *ManifestServer) writeFailure(writer http.ResponseWriter, failure *responseError) {
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
func (server *ManifestServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
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
	// Session authentication takes the account authority's operation lease.
	// Admit no waiter before the fail-fast permit: otherwise a second request
	// could queue behind Argon/node work before reaching this bound.
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
		request = request.WithContext(ctx)
		credential := server.authenticate(request)
		if credential == nil {
			return nil, apiError("unauthorized", 401)
		}
		limiter := credential.admit
		if request.URL.Path == apiPath(server.config.Version, "jobs") {
			limiter = credential.issue
		}
		if !limiter.Allow() {
			return nil, apiError("rate_limited", 429)
		}
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
		if manifestJSONShape(body, MaxRequestSize, 5, map[string]int{"proof.merkle_branch": 20}) != nil {
			return nil, apiError("invalid_request", 400)
		}
		var result any
		var operationErr *responseError
		operationStarted := false
		operate := func(activeCredential *clientCredential) {
			operationStarted = true
			if request.URL.Path == apiPath(server.config.Version, "jobs") {
				result, operationErr = server.issue(ctx, activeCredential, body)
			} else {
				result, operationErr = server.admit(ctx, activeCredential, body)
			}
		}
		if server.sessions == nil {
			operate(credential)
		} else {
			err := server.sessions.WithSession(ctx, bearerToken(request), func(key string, recheck func() error) error {
				if key != credential.publicKey || recheck == nil || recheck() != nil {
					return errSession
				}
				copy := *credential
				copy.recheck = recheck
				operate(&copy)
				return nil
			})
			if err != nil {
				if operationStarted {
					return nil, &responseError{"outcome_unknown", 503, true}
				}
				return nil, apiError("unauthorized", 401)
			}
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

func (server *ManifestServer) samePolicy(context prospect.Context) bool {
	return context.Version == server.policy.Version && context.Network == server.policy.Network &&
		strings.EqualFold(context.Genesis, server.policy.Genesis) && context.DNAVersion == server.policy.DNAVersion &&
		strings.EqualFold(context.ProspectTarget, server.policy.ProspectTarget)
}

func (server *ManifestServer) storeFailure(err error) *responseError {
	switch {
	case errors.Is(err, errSession):
		return apiError("unauthorized", 401)
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

func (server *ManifestServer) issue(ctx context.Context, credential *clientCredential, body []byte) (any, *responseError) {
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
	job, err := server.store.IssueManifestJob(resolved, credential.publicKey, func() error {
		if ctx.Err() != nil {
			return errNode
		}
		if err := server.node.RecheckContext(ctx, resolved); err != nil {
			return errNode
		}
		if ctx.Err() != nil {
			return errNode
		}
		return credential.validSession()
	})
	if err != nil {
		return nil, server.storeFailure(err)
	}
	return job, nil
}

func (server *ManifestServer) admit(ctx context.Context, credential *clientCredential, body []byte) (any, *responseError) {
	proof, authorization, err := ParseSubmission(body)
	if err != nil || proof.Version != prospect.Version2 || authorization.Version != gymauth.Version2 {
		return nil, apiError("invalid_request", 400)
	}
	if !strings.EqualFold(authorization.PublicKey, credential.publicKey) {
		return nil, apiError("forbidden", 403)
	}
	info, err := server.store.Info()
	if err != nil {
		return nil, server.storeFailure(err)
	}
	record, inserted, err := server.store.RecordManifestJobChecked(proof, authorization, func(initial prospect.Context, guardProof prospect.Proof, verified prospect.Verified) (registry.Eligibility, error) {
		if info.Records >= server.config.MaxRecords {
			return registry.Eligibility{}, errCapacity
		}
		if !server.samePolicy(initial) {
			return registry.Eligibility{}, errPolicy
		}
		if ctx.Err() != nil {
			return registry.Eligibility{}, errNode
		}
		source, err := server.node.RecheckAdmission(ctx, initial, guardProof, verified)
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
		return observation, credential.validSession()
	})
	if err != nil {
		return nil, server.storeFailure(err)
	}
	// Duplicate recovery bypasses the new-record guard, not authorization.
	if !inserted && credential.validSession() != nil {
		return nil, apiError("unauthorized", 401)
	}
	source := "historical-receipt"
	if inserted {
		source = "trusted-node-snapshot"
	}
	return ManifestAdmissionResult{Version: Version3, ContextSource: source, CurrentNodeChecked: inserted, Authentication: "gym-key-control", BirthRule: "job-issuance", Inserted: inserted, Record: record}, nil
}
