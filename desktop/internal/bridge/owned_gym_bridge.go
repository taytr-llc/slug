// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

// OwnedContext is minted only for a validated view of this exact helper. It
// remains private native state; public DTO copies cannot reconstruct it.
type OwnedContext struct {
	helper *OwnedBridge
	view   pilot.OwnedGymView
	scope  pilot.OwnedGymScope
}

func (v *OwnedContext) View() pilot.OwnedGymView {
	if v == nil {
		return pilot.OwnedGymView{}
	}
	return v.view
}
func (v *OwnedContext) Scope() pilot.OwnedGymScope {
	if v == nil {
		return pilot.OwnedGymScope{}
	}
	var out pilot.OwnedGymScope
	b, _ := json.Marshal(v.scope)
	_ = json.Unmarshal(b, &out)
	return out
}

type OwnedReply struct {
	Envelope pilot.OwnedGymResponse
	Value    any
}
type OwnedFailure struct{ Code string }

func (e *OwnedFailure) Error() string { return "The gym request could not complete (" + e.Code + ")." }

// OwnedBridge has one gate and one response reader. It never queues mutations,
// automatically resubmits, or replaces a failed helper inside an original view.
type OwnedBridge struct {
	cmd                    *exec.Cmd
	input                  io.WriteCloser
	output                 io.ReadCloser
	lines                  chan []byte
	failed, done, readDone chan struct{}
	gate                   chan struct{}
	once                   sync.Once
	mu                     sync.Mutex
	failure, waitErr       error
	nextID                 uint64
	activeFixture          string
	bootstrapSource        *pilot.OwnedSourceConfig
	pendingResetID         string
	scopes                 map[string]pilot.OwnedGymScope
	view                   *OwnedContext
	stable                 *pilot.OwnedStableCapture
	history                *gymstable.OwnedCareerHistoryCapture
	transfer               *pilot.OwnedTransferReview
	delivery               *pilot.OwnedTransferDeliveryReview
	slot                   *gymstable.OwnedIntent
	entry                  *gymentry.OwnedIntent
}

func LaunchOwnedGym(ctx context.Context, helper, dataDir, node string) (*OwnedBridge, error) {
	return LaunchOwnedGymWithOptions(ctx, helper, dataDir, node, pilot.PoolOptions{})
}
func LaunchOwnedGymWithOptions(ctx context.Context, helper, dataDir, node string, options pilot.PoolOptions) (*OwnedBridge, error) {
	options, e := pilot.ResolvePoolOptions(options)
	if e != nil || ctx == nil || !filepath.IsAbs(dataDir) || filepath.Clean(dataDir) != dataDir || strings.ContainsAny(dataDir, "\x00\r\n\t") {
		return nil, ErrUnavailable
	}
	defaults, e := pilot.ResolvePoolOptions(pilot.PoolOptions{})
	if e != nil {
		return nil, ErrUnavailable
	}
	args := []string{"--candidate-owned-gym", "--data-dir", dataDir, "--node-binary", node}
	if options.CandidateDescriptorID != defaults.CandidateDescriptorID {
		args = append(args, "--candidate-name-version=2")
	}
	cmd := exec.Command(helper, args...)
	input, e := cmd.StdinPipe()
	if e != nil {
		return nil, ErrUnavailable
	}
	output, e := cmd.StdoutPipe()
	if e != nil {
		input.Close()
		return nil, ErrUnavailable
	}
	cmd.Stderr = io.Discard
	if e = cmd.Start(); e != nil {
		input.Close()
		output.Close()
		return nil, ErrUnavailable
	}
	b := &OwnedBridge{cmd: cmd, input: input, output: output, lines: make(chan []byte, 1), failed: make(chan struct{}), done: make(chan struct{}), readDone: make(chan struct{}), gate: make(chan struct{}, 1), activeFixture: "zero256", scopes: map[string]pilot.OwnedGymScope{}}
	go b.readOwned()
	go func() {
		e := cmd.Wait()
		b.mu.Lock()
		b.waitErr = e
		b.mu.Unlock()
		b.failOwned(ErrUnavailable)
		close(b.done)
	}()

	// The initial childless snapshot comes only from this launched root. During
	// an interrupted reset no wallet is opened to satisfy a scope handshake.
	reply, e := b.exchangeOwned(ctx, pilot.OwnedGymRequest{Command: "startup"}, nil, nil)
	if e != nil {
		b.failOwned(e)
		return b, e
	}
	initial, ok := reply.Value.(pilot.OwnedGymSnapshot)
	if !ok {
		b.failOwned(ErrUnavailable)
		return b, ErrUnavailable
	}
	source := initial.Source
	if source.Ownership.Source.DescriptorID != options.CandidateDescriptorID {
		b.failOwned(ErrUnavailable)
		return b, ErrUnavailable
	}
	b.bootstrapSource = &source
	b.pendingResetID = initial.PendingResetID
	if initial.State == "signed_out" && initial.PendingResetID == "" {
		if e = b.loadOwnedScopes(ctx); e != nil {
			b.failOwned(e)
			return b, e
		}
	}
	return b, nil
}

// gate is held or this helper has not yet been published to native callers.
func (b *OwnedBridge) loadOwnedScopes(ctx context.Context) error {
	scopes := map[string]pilot.OwnedGymScope{}
	for _, fixture := range []string{"zero256", "incrementing256"} {
		reply, e := b.exchangeOwned(ctx, pilot.OwnedGymRequest{Command: "scope", Fixture: fixture}, nil, nil)
		if e != nil {
			return e
		}
		s, ok := reply.Value.(pilot.OwnedGymScope)
		if !ok || b.bootstrapSource == nil || s.Source != *b.bootstrapSource {
			return ErrUnavailable
		}
		scopes[fixture] = s
	}
	a, z := scopes["zero256"], scopes["incrementing256"]
	if a.Source != z.Source || a.PoolSource != z.PoolSource || !ownedcodec.Equal(a.EventPin, z.EventPin) || a.StablePin.Stable.AccountID == z.StablePin.Stable.AccountID || a.Carrier.JournalID == z.Carrier.JournalID {
		return ErrUnavailable
	}
	b.mu.Lock()
	b.scopes = scopes
	b.pendingResetID = ""
	b.mu.Unlock()
	return nil
}

func (b *OwnedBridge) failOwned(e error) {
	b.once.Do(func() { b.mu.Lock(); b.failure = e; b.mu.Unlock(); close(b.failed); _ = b.input.Close() })
}
func (b *OwnedBridge) readOwned() {
	defer close(b.readDone)
	s := bufio.NewScanner(b.output)
	s.Buffer(make([]byte, 4096), pilot.MaxOwnedGymResponseSize+1)
	for s.Scan() {
		raw := append([]byte(nil), s.Bytes()...)
		if len(raw) > pilot.MaxOwnedGymResponseSize {
			b.failOwned(ErrUnavailable)
			return
		}
		select {
		case b.lines <- raw:
		case <-b.failed:
			return
		default:
			b.failOwned(ErrUnavailable)
			return
		}
	}
	b.failOwned(ErrUnavailable)
}

func (b *OwnedBridge) Context() (*OwnedContext, error) {
	if b == nil {
		return nil, ErrUnavailable
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.view == nil {
		return nil, ErrUnavailable
	}
	return b.view, nil
}
func (b *OwnedBridge) ContextMatches(v *OwnedContext) bool {
	if b == nil || v == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failure == nil && b.view == v && v.helper == b
}

// Close retains the owner until both process and pipe reader join. A caller
// timeout reports incomplete cleanup; it never authorizes a replacement owner.
func (b *OwnedBridge) Close(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if ctx == nil {
		return ErrUnavailable
	}
	b.failOwned(ErrClosed)
	select {
	case <-b.done:
	case <-ctx.Done():
		return errors.Join(ErrUnavailable, ctx.Err())
	}
	_ = b.output.Close()
	select {
	case <-b.readDone:
	case <-ctx.Done():
		return errors.Join(ErrUnavailable, ctx.Err())
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.waitErr
}

// Request is a native-only transport API. The Wails service exposes narrower
// controls, retained captures, and native confirmation dialogs instead of this
// request object. The original context is required for every child action.
func (b *OwnedBridge) Request(ctx context.Context, original *OwnedContext, r pilot.OwnedGymRequest) (OwnedReply, error) {
	if b == nil || ctx == nil {
		return OwnedReply{}, ErrUnavailable
	}
	select {
	case b.gate <- struct{}{}:
	default:
		return OwnedReply{}, ErrBusy
	}
	defer func() { <-b.gate }()
	b.mu.Lock()
	if b.failure != nil {
		e := b.failure
		b.mu.Unlock()
		return OwnedReply{}, e
	}
	bootstrap := len(b.scopes) != 2
	if bootstrap {
		if original != nil || r.ViewID != "" || r.Command != "snapshot" && (r.Command != "resume_reset" || b.pendingResetID == "" || r.OperationID != b.pendingResetID) {
			b.mu.Unlock()
			return OwnedReply{}, ErrUnavailable
		}
		b.mu.Unlock()
		reply, e := b.exchangeOwned(ctx, r, nil, nil)
		if e != nil {
			return reply, e
		}
		snapshot, ok := reply.Value.(pilot.OwnedGymSnapshot)
		if !ok {
			b.failOwned(ErrUnavailable)
			return OwnedReply{}, ErrUnavailable
		}
		if snapshot.PendingResetID != "" && snapshot.PendingResetID != b.pendingResetID {
			b.failOwned(ErrUnavailable)
			return OwnedReply{}, ErrUnavailable
		}
		if snapshot.State == "signed_out" && snapshot.PendingResetID == "" {
			if e = b.loadOwnedScopes(ctx); e != nil {
				b.failOwned(e)
				return OwnedReply{}, e
			}
		}
		return reply, nil
	}
	fixture := b.activeFixture
	if r.Command == "login_fixture" {
		fixture = r.Fixture
	}
	scope, ok := b.scopes[fixture]
	var view *pilot.OwnedGymView
	if original != nil {
		if b.view != original || original.helper != b {
			b.mu.Unlock()
			return OwnedReply{}, &OwnedFailure{Code: "stale_view"}
		}
		v := original.view
		view = &v
		if r.ViewID != "" && r.ViewID != v.ViewID {
			b.mu.Unlock()
			return OwnedReply{}, ErrUnavailable
		}
		r.ViewID = v.ViewID
		scope = original.scope
	}
	if e := b.reviewBeforeOwnedDispatch(r); e != nil {
		b.mu.Unlock()
		return OwnedReply{}, e
	}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		switch r.Command {
		case "transfer_confirm":
			b.transfer = nil
		case "transfer_deliver":
			b.delivery = nil
		case "apply_slot":
			b.slot = nil
		case "apply_entry":
			b.entry = nil
		}
	}()
	if !ok || r.Command == "scope" || r.Command == "startup" {
		return OwnedReply{}, ErrUnavailable
	}
	r.Version = 1
	// The only unbound commands are explicit parent controls. ValidateRequest
	// rejects every child command without its original context's view ID.
	if original == nil && r.ViewID != "" {
		return OwnedReply{}, ErrUnavailable
	}
	reply, e := b.exchangeOwned(ctx, r, &scope, view)
	if e != nil {
		return reply, e
	}
	if e = b.retainOwned(r, reply.Value, original); e != nil {
		b.failOwned(e)
		return OwnedReply{}, e
	}
	if r.Command == "login_fixture" {
		b.mu.Lock()
		b.activeFixture = fixture
		b.mu.Unlock()
	}
	return reply, nil
}

func (b *OwnedBridge) exchangeOwned(ctx context.Context, r pilot.OwnedGymRequest, scope *pilot.OwnedGymScope, view *pilot.OwnedGymView) (OwnedReply, error) {
	// gate is held by Request, or startup is the sole caller before publication.
	work, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if e := work.Err(); e != nil {
		return OwnedReply{}, e
	}
	b.nextID++
	r.ID = b.nextID
	r.Version = 1
	raw, e := pilot.MarshalOwnedGymRequest(r)
	if e != nil {
		return OwnedReply{}, e
	}
	// Cancel closes the concrete exec pipe and joins its blocked Write. Once
	// any byte may have left, this helper is uncertain and cannot be reused.
	raw = append(raw, '\n')
	if e := work.Err(); e != nil {
		return OwnedReply{}, e
	}
	writeCancelled := make(chan struct{})
	stopWrite := context.AfterFunc(work, func() { defer close(writeCancelled); b.failOwned(ErrUnavailable) })
	n, writeErr := b.input.Write(raw)
	if !stopWrite() {
		<-writeCancelled
	}
	if writeErr != nil || n != len(raw) || work.Err() != nil {
		b.failOwned(ErrUnavailable)
		return OwnedReply{}, errors.Join(ErrUnavailable, work.Err())
	}
	select {
	case raw := <-b.lines:
		if e := work.Err(); e != nil {
			b.failOwned(ErrUnavailable)
			return OwnedReply{}, errors.Join(ErrUnavailable, e)
		}
		var envelope pilot.OwnedGymResponse
		var value any
		var e error
		if scope == nil && r.Command != "scope" {
			envelope, value, e = pilot.ParseOwnedGymBootstrapResponse(raw, r, b.bootstrapSource)
		} else {
			envelope, value, e = pilot.ParseOwnedGymResponse(raw, r, scope, view)
		}
		if e != nil {
			b.failOwned(ErrUnavailable)
			return OwnedReply{}, ErrUnavailable
		}
		if e := work.Err(); e != nil {
			b.failOwned(ErrUnavailable)
			return OwnedReply{}, errors.Join(ErrUnavailable, e)
		}
		reply := OwnedReply{Envelope: envelope, Value: value}
		if !envelope.OK {
			return reply, &OwnedFailure{Code: envelope.Error}
		}
		return reply, nil
	case <-work.Done():
		b.failOwned(ErrUnavailable)
		return OwnedReply{}, errors.Join(ErrUnavailable, work.Err())
	case <-b.failed:
		return OwnedReply{}, ErrUnavailable
	}
}

func (b *OwnedBridge) retainOwned(r pilot.OwnedGymRequest, value any, original *OwnedContext) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil {
		return b.failure
	}
	if original != nil && b.view != original {
		return ErrUnavailable
	}
	switch v := value.(type) {
	case pilot.OwnedGymSnapshot:
		if v.View == nil {
			b.slot, b.entry = nil, nil
			b.view = nil
			b.stable = nil
			b.history = nil
			b.transfer = nil
			b.delivery = nil
		} else if b.view == nil || b.view.view != *v.View {
			b.slot, b.entry = nil, nil
			scope, ok := b.scopes[v.View.Fixture]
			if !ok || !scope.ViewMatches(*v.View) {
				return ErrUnavailable
			}
			b.view = &OwnedContext{helper: b, view: *v.View, scope: scope}
			b.stable = nil
			b.history = nil
			b.transfer = nil
			b.delivery = nil
		}
	case pilot.OwnedStableCapture:
		copy := ownedBridgeCopy(v)
		b.stable = &copy
	case gymstable.OwnedPage:
		if b.stable == nil || r.CaptureID != b.stable.CaptureID || v.Head != b.stable.Page.Head || v.AsOfMS != b.stable.Page.AsOfMS || !ownedcodec.Equal(v.Status, b.stable.Page.Status) {
			return ErrUnavailable
		}
	case gymstable.OwnedCareerHistoryCapture:
		copy := ownedBridgeCopy(v)
		b.history = &copy
	case gymstable.OwnedCareerHistoryPage:
		if b.history == nil || !ownedcodec.Equal(v.Capture, *b.history) {
			return ErrUnavailable
		}
	case gymstable.OwnedIntent:
		copy := ownedBridgeCopy(v)
		b.slot = &copy
	case gymstable.OwnedOperation:
		if v.State == "prepared" {
			i := ownedBridgeCopy(v.Intent)
			b.slot = &i
		} else {
			b.slot = nil
		}
	case gymstable.OwnedMutationResult:
		if original == nil || b.slot == nil || r.OperationID != b.slot.OperationID || v.Receipt.OperationID != b.slot.OperationID {
			return ErrUnavailable
		}
		if _, e := gymstable.ValidateOwnedOperation(original.scope.StablePin, gymstable.OwnedOperation{Version: 1, Intent: *b.slot, State: "committed", Receipt: &v.Receipt}); e != nil {
			return ErrUnavailable
		}
	case gymentry.OwnedIntent:
		copy := ownedBridgeCopy(v)
		b.entry = &copy
	case gymentry.OwnedOperation:
		if v.State == "prepared" {
			i := ownedBridgeCopy(v.Intent)
			b.entry = &i
		} else {
			b.entry = nil
		}
	case gymentry.OwnedMutationResult:
		if original == nil || b.entry == nil || r.OperationID != b.entry.OperationID || v.Receipt.OperationID != b.entry.OperationID {
			return ErrUnavailable
		}
		if _, e := gymentry.ValidateOwnedOperation(original.scope.EntryPin, gymentry.OwnedOperation{Version: 1, Intent: *b.entry, State: "committed", Receipt: &v.Receipt}); e != nil {
			return ErrUnavailable
		}
	case pilot.OwnedTransferReview:
		b.transfer = &v
		b.delivery = nil
	case pilot.OwnedTransferDeliveryReview:
		b.transfer = nil
		b.delivery = &v
	case fightercarrier.Operation:
		if r.Command == "transfer_confirm" {
			if b.transfer == nil || r.ReviewID != b.transfer.ReviewID || v.RequestID != b.transfer.RequestID || v.Preparation == nil || v.Preparation.Review != b.transfer.Details || v.SignedPackage == nil {
				return ErrUnavailable
			}
			b.transfer = nil
		}
		if r.Command == "transfer_deliver" {
			if b.delivery == nil || r.ReviewID != b.delivery.ReviewID || v.RequestID != b.delivery.RequestID || v.SignedPackage == nil || v.Preparation == nil || v.Preparation.Review != b.delivery.Details {
				return ErrUnavailable
			}
			leg := v.SignedPackage.Commit
			if b.delivery.Leg == "spend" {
				leg = v.SignedPackage.Spend
			}
			if leg.TxID != b.delivery.TxID || leg.RawSHA256 != b.delivery.RawSHA256 || v.SignedPackage.TransferID != b.delivery.TransferID {
				return ErrUnavailable
			}
			b.delivery = nil
		}
	}
	return nil
}

// Called only with the request gate and bridge mutex held. Review identities
// are checked before transport, not merely after a potentially signed action.
func (b *OwnedBridge) reviewBeforeOwnedDispatch(r pilot.OwnedGymRequest) error {
	bad := &OwnedFailure{Code: "stale_view"}
	switch r.Command {
	case "transfer_prepare", "transfer_resume", "transfer_review_delivery":
		b.transfer, b.delivery = nil, nil
	case "transfer_confirm":
		if b.transfer == nil || b.transfer.ReviewID != r.ReviewID {
			return bad
		}
	case "transfer_deliver":
		if b.delivery == nil || b.delivery.ReviewID != r.ReviewID || time.Now().UnixMilli() >= b.delivery.ExpiresAtMS || time.Now().UnixMilli() < b.delivery.ExpiresAtMS-60000 {
			return bad
		}
	case "apply_slot":
		if b.slot == nil || b.slot.OperationID != r.OperationID {
			return bad
		}
	case "apply_entry":
		if b.entry == nil || b.entry.OperationID != r.OperationID {
			return bad
		}
	case "stable_page":
		if b.stable == nil || b.stable.CaptureID != r.CaptureID {
			return bad
		}
	case "history_page":
		if b.history == nil || b.history.HistoryID != r.CaptureID {
			return bad
		}
	}
	return nil
}

func ownedBridgeCopy[T any](in T) T {
	var out T
	raw, _ := json.Marshal(in)
	_ = json.Unmarshal(raw, &out)
	return out
}
