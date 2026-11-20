// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/poolingress"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolschedule"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlenode"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/wallet"
	"golang.org/x/net/netutil"
)

// Status consists of dated local observations, never current funding, signing,
// ownership or credential authority. Public/round failures clear success flags.
type Status struct {
	Version        uint32                    `json:"version"`
	RuntimeID      string                    `json:"runtime_id"`
	SetupID        string                    `json:"setup_id"`
	Running        bool                      `json:"running"`
	Closing        bool                      `json:"closing"`
	Custody        string                    `json:"custody"`
	CustodyAtMS    int64                     `json:"custody_at_ms"`
	CustodyUntilMS int64                     `json:"custody_until_ms"`
	Public         fighterownership.Progress `json:"public"`
	PublicCode     string                    `json:"public_code"`
	PublicAtMS     int64                     `json:"public_at_ms"`
	Rounds         poolround.Summary         `json:"rounds"`
	RoundCode      string                    `json:"round_code"`
	RoundAtMS      int64                     `json:"round_at_ms"`
	Schedule       *poolschedule.Status      `json:"schedule"`
}

// Runtime owns created children and transports, and borrows exactly the two
// supplied custody owners. It exposes no child/store/wallet/authority getter.
type Runtime struct {
	mu                                  sync.Mutex
	closeMu                             sync.Mutex
	inputMu                             sync.Mutex
	calls                               sync.WaitGroup
	localGate                           chan struct{}
	setup                               Setup
	setupID                             string
	profile                             registry.OperatorProfile
	binding                             operatorauthority.Binding
	borrowed                            BorrowedOwners
	life                                context.Context
	cancel                              context.CancelFunc
	started, closing, closed            bool
	runDone                             chan struct{}
	closeErr                            error
	unreleasedChild                     error
	status                              Status
	files                               *operatorFiles
	tlsCertificate, tlsKey, unlockInput *privateInput
	node                                *prospectnode.Client
	chain                               *fighterownership.OwnedChainSource
	fundingNode                         *poolsettlenode.Client
	backend                             *wallet.PoolSettlementBackend
	miners                              *poolminer.Store
	issuer                              *registry.ManifestStore
	publisher                           *registry.ManifestPublication
	pool                                *poolshare.Pool
	rounds                              *poolround.Store
	settlement                          *poolsettle.Store
	schedule                            *poolschedule.Scheduler
	ownership                           *fighterownership.Store
	poolServer                          *poolservice.Server
	admissionServer                     *prospectservice.ManifestServer
	publicServer                        *fighterownership.PublicReadServer
	ingress                             *poolingress.Server
	public                              publicMaintenance
	round                               roundMaintenance
}

func (*Runtime) String() string   { return "pool operator runtime" }
func (*Runtime) GoString() string { return "pool operator runtime" }
func (r *Runtime) Setup() Setup {
	if r == nil {
		return Setup{}
	}
	raw, e := MarshalSetup(r.setup)
	if e != nil {
		return Setup{}
	}
	out, _ := ParseSetup(raw)
	return out
}
func cloneStatus(s Status) (Status, error) {
	raw, e := json.Marshal(s)
	if e != nil || len(raw) > 65536 {
		return Status{}, ErrIntegrity
	}
	var out Status
	if json.Unmarshal(raw, &out) != nil {
		return Status{}, ErrIntegrity
	}
	return out, nil
}
func (r *Runtime) Status(ctx context.Context) (Status, error) {
	if r == nil || ctx == nil || r.life == nil || r.runDone == nil {
		return Status{}, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return Status{}, e
	}
	r.mu.Lock()
	if r.closing || r.closed {
		r.mu.Unlock()
		return Status{}, ErrUnavailable
	}
	r.calls.Add(1)
	out, e := cloneStatus(r.status)
	r.mu.Unlock()
	defer r.calls.Done()
	if e != nil {
		return Status{}, e
	}
	if e = r.checkInputs(ctx); e != nil {
		return Status{}, e
	}
	state, e := r.schedule.Status(ctx)
	if e != nil {
		return Status{}, e
	}
	out.Schedule = &state
	if out.CustodyUntilMS != 0 && time.Now().UTC().UnixMilli() >= out.CustodyUntilMS {
		out.Custody = "expired"
	}
	if e = ctx.Err(); e != nil {
		return Status{}, e
	}
	if e = r.life.Err(); e != nil {
		return Status{}, e
	}
	return out, nil
}
func (r *Runtime) checkInputs(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := r.life.Err(); e != nil {
		return e
	}
	if e := r.borrowed.Authority.Check(ctx, r.binding); e != nil {
		return ErrUnavailable
	}
	r.inputMu.Lock()
	defer r.inputMu.Unlock()
	if e := r.files.check(); e != nil {
		return e
	}
	for _, input := range []*privateInput{r.tlsCertificate, r.tlsKey, r.unlockInput} {
		if e := input.check(ctx); e != nil {
			return e
		}
	}
	return ctx.Err()
}
func pause(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// runtimeCancelledOnly preserves every non-cancellation leaf of a joined task
// result. A canceled sibling is expected shutdown, not evidence that an
// independently failed service or cleanup succeeded.
func runtimeCancelledOnly(e error) bool {
	if e == nil || e == context.Canceled {
		return true
	}
	if many, ok := e.(interface{ Unwrap() []error }); ok {
		for _, item := range many.Unwrap() {
			if !runtimeCancelledOnly(item) {
				return false
			}
		}
		return true
	}
	if one, ok := e.(interface{ Unwrap() error }); ok {
		return runtimeCancelledOnly(one.Unwrap())
	}
	return false
}
func runtimeCompletion(ctx context.Context, e error) error {
	if ctx.Err() != nil && runtimeCancelledOnly(e) {
		return ctx.Err()
	}
	return errors.Join(ErrUnavailable, e)
}

// lockWallet retries only a definite non-admitted wallet gate Busy. It never
// retries signing, reads a passphrase, replaces an owner or changes a payment.
func (r *Runtime) lockWallet(ctx context.Context) error {
	if r.borrowed.Wallet == nil {
		return nil
	}
	for {
		e := r.borrowed.Wallet.Lock(ctx)
		if !errors.Is(e, wallet.ErrPoolWalletBusy) {
			return e
		}
		if e = pause(ctx, 10*time.Millisecond); e != nil {
			return e
		}
	}
}
func (r *Runtime) renewWallet(ctx context.Context) error {
	// Unlock starts its timer before bounded KDF work. Start before file input
	// as well, so this dated projection never extends the actual custody window.
	started := time.Now().UTC().UnixMilli()
	r.inputMu.Lock()
	pass, e := r.unlockInput.read(ctx)
	r.inputMu.Unlock()
	if e == nil {
		e = r.borrowed.Wallet.Unlock(ctx, pass, time.Duration(r.setup.UnlockLifetimeMS)*time.Millisecond)
	}
	clear(pass)
	now := time.Now().UTC().UnixMilli()
	state, until := "unlocked", started+r.setup.UnlockLifetimeMS
	var lockErr error
	if e != nil {
		lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		lockErr = r.lockWallet(lockCtx)
		cancel()
		state, until = "unavailable", 0
	}
	r.mu.Lock()
	r.status.Custody = state
	r.status.CustodyAtMS = now
	r.status.CustodyUntilMS = until
	r.mu.Unlock()
	// An unsuccessful unlock with successful explicit Lock is visible deferral.
	// If Lock cannot be established, stop all runtime work and retain the owner.
	if lockErr != nil {
		return errors.Join(ErrCleanup, lockErr)
	}
	return nil
}
func (r *Runtime) custodyLoop(ctx context.Context) error {
	for {
		if e := pause(ctx, time.Duration(r.setup.UnlockRenewalMS)*time.Millisecond); e != nil {
			return e
		}
		if e := r.renewWallet(ctx); e != nil {
			return e
		}
	}
}
func publicRetry(e error) bool {
	// A shared online owner is deliberately nonwaiting. Admission and public
	// maintenance may contend normally; only the original busy error permits
	// retry. Never hide a joined cleanup or custody failure behind that error.
	switch e {
	case fighterownership.ErrBusy, fighterownership.ErrStale, fighterownership.ErrUnavailable,
		registry.ErrBusy, registry.ErrConflict, operatorauthority.ErrBusy, context.DeadlineExceeded:
		return true
	}
	if many, ok := e.(interface{ Unwrap() []error }); ok {
		children := many.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !publicRetry(child) {
				return false
			}
		}
		return true
	}
	if one, ok := e.(interface{ Unwrap() error }); ok {
		return publicRetry(one.Unwrap())
	}
	return false
}
func roundRetry(e error) bool {
	return errors.Is(e, poolround.ErrBusy) || errors.Is(e, poolround.ErrNode) || errors.Is(e, poolround.ErrSourceUnavailable) || errors.Is(e, context.DeadlineExceeded)
}
func maintenanceCode(e error) string {
	if e == nil {
		return ""
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return "timed_out"
	}
	if errors.Is(e, fighterownership.ErrBusy) || errors.Is(e, registry.ErrBusy) || errors.Is(e, poolround.ErrBusy) || errors.Is(e, operatorauthority.ErrBusy) {
		return "busy"
	}
	if errors.Is(e, fighterownership.ErrCapacity) || errors.Is(e, poolround.ErrCapacity) {
		return "capacity"
	}
	return "unavailable"
}
func (r *Runtime) publicLoop(ctx context.Context) (result error) {
	defer func() { result = errors.Join(result, r.public.close()) }()
	for {
		if e := r.checkInputs(ctx); e != nil {
			return e
		}
		step, cancel := context.WithTimeout(ctx, 20*time.Second)
		p, e := r.public.step(step)
		cancel()
		r.mu.Lock()
		r.status.Public = p
		r.status.PublicCode = maintenanceCode(e)
		r.status.PublicAtMS = time.Now().UTC().UnixMilli()
		r.mu.Unlock()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e != nil && !publicRetry(e) {
			return e
		}
		delay := time.Duration(r.setup.MaintenanceIntervalMS) * time.Millisecond
		if e == nil && !p.Ready || errors.Is(e, context.DeadlineExceeded) {
			delay = 10 * time.Millisecond
		}
		if e = pause(ctx, delay); e != nil {
			return e
		}
	}
}
func (r *Runtime) roundLoop(ctx context.Context) error {
	for {
		if e := r.checkInputs(ctx); e != nil {
			return e
		}
		step, cancel := context.WithTimeout(ctx, 30*time.Second)
		summary, e := r.round.step(step)
		cancel()
		r.mu.Lock()
		r.status.Rounds = summary
		r.status.RoundCode = maintenanceCode(e)
		r.status.RoundAtMS = time.Now().UTC().UnixMilli()
		r.mu.Unlock()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e != nil && !roundRetry(e) {
			return e
		}
		if e = pause(ctx, time.Duration(r.setup.MaintenanceIntervalMS)*time.Millisecond); e != nil {
			return e
		}
	}
}

// runPublicHTTP owns only the exact third private listener and joins the
// concrete public reader's handlers before it returns. It exposes no new route.
func (r *Runtime) runPublicHTTP(ctx context.Context, ready func(string)) error {
	life, cancel := context.WithCancel(ctx)
	defer cancel()
	var lc net.ListenConfig
	listener, e := lc.Listen(life, "tcp", privateListen(r.setup.Ingress.OwnershipURL))
	if e != nil {
		return ErrUnavailable
	}
	server := &http.Server{Handler: r.publicServer, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return life }, ErrorLog: log.New(io.Discard, "", 0)}
	done := make(chan error, 1)
	go func() { done <- server.Serve(netutil.LimitListener(listener, 16)) }()
	ready("http://" + listener.Addr().String())
	var serveErr error
	joined := false
	select {
	case serveErr = <-done:
		joined = true
	case <-life.Done():
	}
	cancel()
	closeErr := server.Close()
	if !joined {
		serveErr = <-done
	}
	handlerErr := r.publicServer.Close()
	if closeErr != nil || handlerErr != nil || serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return ErrUnavailable
	}
	return nil
}

// Run is one-shot and joins every child goroutine before returning. onReady is
// a bounded, non-reentrant notification of the selected public origin; it does
// not assert replay-ready or wallet spendability. Always call Close afterwards.
func (r *Runtime) Run(ctx context.Context, onReady func(string)) (result error) {
	if r == nil || ctx == nil || r.life == nil || r.runDone == nil {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	r.mu.Lock()
	if r.started || r.closing || r.closed {
		r.mu.Unlock()
		return ErrUnavailable
	}
	r.started = true
	r.status.Running = true
	r.mu.Unlock()
	work, cancel := context.WithCancel(r.life)
	stop := context.AfterFunc(ctx, cancel)
	var wg sync.WaitGroup
	type completion struct {
		name string
		err  error
	}
	results := make(chan completion, 8)
	spawn := func(name string, fn func() error) {
		wg.Add(1)
		go func() { defer wg.Done(); results <- completion{name, fn()} }()
	}
	defer func() {
		cancel()
		stop()
		wg.Wait()
		close(results)
		for completed := range results {
			if completed.err != nil {
				result = errors.Join(result, completed.err)
			}
		}
		lockCtx, end := context.WithTimeout(context.Background(), 30*time.Second)
		lockErr := r.lockWallet(lockCtx)
		end()
		r.mu.Lock()
		r.status.Running = false
		r.status.Custody = "locked"
		r.status.CustodyUntilMS = 0
		if lockErr != nil {
			r.status.Custody = "unavailable"
			result = errors.Join(result, ErrCleanup, lockErr)
		}
		close(r.runDone)
		r.mu.Unlock()
	}()
	if e := work.Err(); e != nil {
		return e
	}
	if e := r.checkInputs(work); e != nil {
		return e
	}
	if e := r.renewWallet(work); e != nil {
		return e
	}
	ready := make(chan string, 3)
	spawn("pool", func() error { return r.poolServer.Run(work, func(url string) { ready <- url }) })
	spawn("admission", func() error { return r.admissionServer.Run(work, func(url string) { ready <- url }) })
	spawn("ownership", func() error { return r.runPublicHTTP(work, func(url string) { ready <- url }) })
	waiting := map[string]bool{r.setup.Ingress.PoolURL: true, r.setup.Ingress.AdmissionURL: true, r.setup.Ingress.OwnershipURL: true}
	timer := time.NewTimer(10 * time.Second)
	for len(waiting) > 0 {
		select {
		case url := <-ready:
			if !waiting[url] {
				timer.Stop()
				return ErrIntegrity
			}
			delete(waiting, url)
		case done := <-results:
			timer.Stop()
			return runtimeCompletion(work, done.err)
		case <-work.Done():
			timer.Stop()
			return work.Err()
		case <-timer.C:
			return ErrUnavailable
		}
	}
	timer.Stop()
	spawn("public maintenance", func() error { return r.publicLoop(work) })
	spawn("round maintenance", func() error { return r.roundLoop(work) })
	spawn("schedule", func() error { return r.schedule.Run(work) })
	spawn("custody", func() error { return r.custodyLoop(work) })
	externalReady := make(chan string, 1)
	spawn("ingress", func() error { return r.ingress.Run(work, func(addr string) { externalReady <- addr }) })
	select {
	case addr := <-externalReady:
		if addr != r.setup.Ingress.Listen {
			return ErrIntegrity
		}
		if onReady != nil {
			onReady(r.setup.Ingress.Origin)
		}
	case done := <-results:
		return runtimeCompletion(work, done.err)
	case <-work.Done():
		return work.Err()
	}
	select {
	case done := <-results:
		result = runtimeCompletion(work, done.err)
	case <-work.Done():
		result = work.Err()
	}
	return result
}

// Close cancels and joins Run/API calls, then closes each dependent before its
// source. Successful closes are never repeated. Cleanup failure is sticky and
// preserves this Runtime, the outer root lock, failed handle and borrowed owners.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.closing = true
	r.status.Closing = true
	if r.cancel != nil {
		r.cancel()
	}
	started, done := r.started, r.runDone
	r.mu.Unlock()
	if started {
		<-done
	}
	r.calls.Wait()
	r.closeMu.Lock()
	defer r.closeMu.Unlock()
	if r.closeErr != nil {
		return r.closeErr
	}
	if r.closed {
		return nil
	}
	fail := func(e error) error { r.closeErr = errors.Join(ErrCleanup, e); return r.closeErr }
	// A constructor may return only a typed failed-cleanup owner. Its borrowed
	// stores must outlive it; retaining the outer root after closing those stores
	// would be too late. Keep the entire dependency chain before any close.
	if r.unreleasedChild != nil {
		return fail(r.unreleasedChild)
	}
	if r.ingress != nil {
		if e := r.ingress.Close(); e != nil {
			return fail(e)
		}
		r.ingress = nil
	}
	if e := r.public.close(); e != nil {
		return fail(e)
	}
	if r.publicServer != nil {
		if e := r.publicServer.Close(); e != nil {
			return fail(e)
		}
		r.publicServer = nil
	}
	if r.schedule != nil {
		if e := r.schedule.Close(context.Background()); e != nil {
			return fail(e)
		}
		r.schedule = nil
	}
	if r.settlement != nil {
		if e := r.settlement.Close(); e != nil {
			return fail(e)
		}
		r.settlement = nil
	}
	if r.backend != nil {
		if e := r.backend.Close(); e != nil {
			return fail(e)
		}
		r.backend = nil
	}
	lockCtx, end := context.WithTimeout(context.Background(), 30*time.Second)
	lockErr := r.lockWallet(lockCtx)
	end()
	if lockErr != nil {
		return fail(lockErr)
	}
	if r.rounds != nil {
		if e := r.rounds.Close(); e != nil {
			return fail(e)
		}
		r.rounds = nil
	}
	if r.pool != nil {
		if e := r.pool.Close(); e != nil {
			return fail(e)
		}
		r.pool = nil
	}
	if r.miners != nil {
		if e := r.miners.Close(context.Background()); e != nil {
			return fail(e)
		}
		r.miners = nil
	}
	if r.ownership != nil {
		if e := r.ownership.Close(); e != nil {
			return fail(e)
		}
		r.ownership = nil
	}
	if r.issuer != nil {
		if e := r.issuer.Close(); e != nil {
			return fail(e)
		}
		r.issuer = nil
		r.publisher = nil
	}
	if r.chain != nil {
		if e := r.chain.Close(); e != nil {
			return fail(e)
		}
		r.chain = nil
	}
	if r.fundingNode != nil {
		if e := r.fundingNode.Close(); e != nil {
			return fail(e)
		}
		r.fundingNode = nil
	}
	if r.node != nil {
		r.node.Close()
		r.node = nil
	}
	for _, input := range []*privateInput{r.unlockInput, r.tlsKey, r.tlsCertificate} {
		if input != nil {
			if e := input.close(); e != nil {
				return fail(e)
			}
		}
	}
	if r.files != nil {
		if e := r.files.close(); e != nil {
			return fail(e)
		}
		r.files = nil
	}
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	return nil
}
