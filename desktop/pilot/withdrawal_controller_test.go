// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slugwallet/wallet"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type withdrawalFake struct {
	journal    *walletwithdraw.Journal
	draft      walletwithdraw.Draft
	raw        []byte
	prepared   bool
	calls      atomic.Int32
	signCalls  atomic.Int32
	deliveries atomic.Int32
	hook       func(context.Context, string) error
	leaseGate  chan struct{}
	t          *testing.T
}

func (f *withdrawalFake) check(ctx context.Context, stage string) error {
	// Concurrent short polling is legitimate. A bounded acquire proves the
	// caller did not carry its account lease into this wallet/node callback.
	probe, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	select {
	case f.leaseGate <- struct{}{}:
		<-f.leaseGate
	case <-probe.Done():
		f.t.Error("wallet/node work blocked account lease", stage)
		return probe.Err()
	}
	if f.hook != nil {
		return f.hook(ctx, stage)
	}
	return ctx.Err()
}
func (f *withdrawalFake) Prepare(ctx context.Context, r walletwithdraw.Request) (walletwithdraw.Draft, error) {
	f.calls.Add(1)
	if e := f.check(ctx, "prepare"); e != nil {
		return walletwithdraw.Draft{}, e
	}
	f.prepared = true
	return f.draft, nil
}
func (f *withdrawalFake) WalletRecord(ctx context.Context, id string) (wallet.CandidateWithdrawalRecord, error) {
	if e := f.check(ctx, "lookup"); e != nil {
		return wallet.CandidateWithdrawalRecord{}, e
	}
	if !f.prepared {
		return wallet.CandidateWithdrawalRecord{}, wallet.ErrWithdrawalNotFound
	}
	raw, _ := json.Marshal(f.draft)
	var d wallet.CandidateWithdrawalDraft
	_ = json.Unmarshal(raw, &d)
	digest, _ := walletwithdraw.DraftDigest(f.draft)
	return wallet.CandidateWithdrawalRecord{Version: 1, State: "prepared", DraftDigest: digest, Draft: d}, nil
}
func (f *withdrawalFake) Sign(ctx context.Context, d walletwithdraw.Draft) ([]byte, error) {
	f.signCalls.Add(1)
	if e := f.check(ctx, "sign"); e != nil {
		return nil, e
	}
	return append([]byte(nil), f.raw...), nil
}
func (f *withdrawalFake) Reconcile(ctx context.Context, r *walletwithdraw.Reconciliation) error {
	if e := f.check(ctx, "reconcile"); e != nil {
		return e
	}
	return r.Check()
}
func (f *withdrawalFake) Inspect(ctx context.Context, d walletwithdraw.Draft) error {
	return f.check(ctx, "inspect")
}
func (f *withdrawalFake) Deliver(ctx context.Context, d *walletwithdraw.Delivery) (walletwithdraw.DeliveryResult, string, error) {
	if e := f.check(ctx, "deliver"); e != nil {
		return "", "", e
	}
	raw, _, _, e := d.Take()
	if e != nil {
		return "", "", e
	}
	if hex.EncodeToString(raw) != hex.EncodeToString(f.raw) {
		f.t.Error("delivery replaced independently signed bytes")
	}
	f.deliveries.Add(1)
	return walletwithdraw.DeliveryAcknowledged, "", nil
}
func (f *withdrawalFake) Observe(ctx context.Context, o *walletwithdraw.Owner, id string) (walletwithdraw.ObservationInput, error) {
	if e := f.check(ctx, "observe"); e != nil {
		return walletwithdraw.ObservationInput{}, e
	}
	return walletwithdraw.ObservationInput{State: walletwithdraw.ObservationMempool, NodeTip: f.draft.TipHash, NodeHeight: f.draft.TipHeight}, nil
}
func (f *withdrawalFake) Close() error { return f.journal.Close() }
func withdrawalFixture(t *testing.T) (*PoolChild, *poolDesktopHarness, *withdrawalFake, WithdrawalRequest) {
	t.Helper()
	c, h, _ := payoutFixture(t, 0)
	// The inherited CAS-only fixture assumes synchronous calls. Withdrawal
	// workers and exact-action polling are concurrent, like the real account
	// store: serialize those callbacks rather than misclassifying them nested.
	leaseGate := make(chan struct{}, 1)
	originalLease := c.account.Lease
	c.account.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		select {
		case leaseGate <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-leaseGate }()
		return originalLease(ctx, cap, fn)
	}
	c.account.AuthenticatedAtMS = time.Now().UnixMilli()
	c.account.SessionExpiresAtMS = c.account.AuthenticatedAtMS + 3600000
	f, _ := gymaccount.FixtureIdentity("zero256")
	p := walletwithdraw.Pin{Version: 1, Network: f.Network, Genesis: f.Genesis, AccountAuthorityID: c.AccountAuthorityIDForTest(), AccountID: c.AccountID(), Fixture: "zero256", WalletProfile: f.Profile, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID, PoolSourceBindingID: c.resources.Recipient.SourceBindingID}
	identity := walletwithdraw.Identity{JournalID: strings.Repeat("e", 64), Pin: p}
	binding, e := walletwithdraw.BindingID(identity)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "withdrawals")
	j, e := walletwithdraw.Create(t.Context(), path, identity)
	if e != nil {
		t.Fatal(e)
	}
	c.withdrawal = &poolWithdrawalResources{Journal: j, Scope: poolWithdrawalScope{Identity: identity, BindingID: binding}}
	raw, e := os.ReadFile("../../walletwithdraw/testdata/reference/journal-reference/wallet-reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Cases []struct {
			Draft  walletwithdraw.Draft `json:"draft"`
			Signed struct {
				Raw string `json:"raw_hex"`
			} `json:"expected_signed"`
		} `json:"cases"`
	}
	if e = json.Unmarshal(raw, &corpus); e != nil || len(corpus.Cases) == 0 {
		t.Fatal(e)
	}
	// Only relational identity fields change; transaction/prevout/signature inputs
	// remain the independent public oracle bytes, never current code outputs.
	d := corpus.Cases[0].Draft
	d.BindingID = binding
	d.OperationID = strings.Repeat("c", 64)
	signedHex := corpus.Cases[0].Signed.Raw
	if signedHex == "" {
		t.Fatal("independent signed bytes missing")
	}
	signed, e := hex.DecodeString(signedHex)
	if e != nil {
		t.Fatal(e)
	}
	backend := &withdrawalFake{journal: j, draft: d, raw: signed, leaseGate: leaseGate, t: t}
	c.withdrawalIO = backend
	req := WithdrawalRequest{ID: 1, Version: 1, Command: "withdrawal_prepare", ViewID: c.account.Manifest.ViewID, OperationID: d.OperationID, ActionID: strings.Repeat("d", 64), Intent: &WithdrawalIntent{d.Recipient, d.RecipientScript, d.AmountAtoms, 1000, 100000}}
	return c, h, backend, req
}
func (c *PoolChild) AccountAuthorityIDForTest() string { return c.account.Manifest.Account.AuthorityID }
func withdrawalWait(t *testing.T, c *PoolChild, r WithdrawalRequest) WithdrawalAction {
	t.Helper()
	var a WithdrawalAction
	poolBackupWait(t, func() bool {
		q := WithdrawalRequest{ID: 1, Version: 1, Command: "withdrawal_action", ViewID: r.ViewID, OperationID: r.OperationID, ActionID: r.ActionID}
		out := c.HandleWithdrawal(t.Context(), q)
		if out.OK && out.Action != nil {
			a = *out.Action
			c.mu.Lock()
			joined := c.withdrawalTask == nil
			c.mu.Unlock()
			return a.State != "running" && joined
		}
		return false
	})
	return a
}
func TestWithdrawalPrepareReviewSignAndExactReplay(t *testing.T) {
	c, _, f, r := withdrawalFixture(t)
	out := c.HandleWithdrawal(t.Context(), r)
	if !out.OK {
		t.Fatal(out.Error)
	}
	done := withdrawalWait(t, c, r)
	if done.State != "done" || done.Operation == nil || done.Operation.Draft == nil {
		withdrawalPrepareFailure(t, c, r, done)
	}
	if done.State != "done" || done.Operation.Stage != "prepared" {
		t.Fatal(done)
	}
	again := r
	again.ID = 7
	out = c.HandleWithdrawal(t.Context(), again)
	if !out.OK || out.Action.State != "done" || f.calls.Load() != 1 {
		t.Fatal("same action reran allocation", out.Error)
	}
	review := WithdrawalRequest{ID: 2, Version: 1, Command: "withdrawal_review", ViewID: r.ViewID, OperationID: r.OperationID, ActionID: strings.Repeat("1", 64), ReviewID: strings.Repeat("2", 64), ReviewKind: "send", DraftDigest: done.Operation.Draft.DraftDigest}
	out = c.HandleWithdrawal(t.Context(), review)
	if !out.OK || out.Review == nil {
		t.Fatal(out.Error)
	}
	expires := out.Review.ExpiresAtMS
	replay := c.HandleWithdrawal(t.Context(), review)
	if !replay.OK || replay.Review.ExpiresAtMS != expires {
		t.Fatal("review replay extended freshness")
	}
	approve := review
	approve.Command = "withdrawal_approve"
	approve.ReviewKind = ""
	out = c.HandleWithdrawal(t.Context(), approve)
	if !out.OK {
		t.Fatal(out.Error)
	}
	sent := withdrawalWait(t, c, approve)
	if sent.State != "done" || sent.Operation.Stage != "submitted" || f.signCalls.Load() != 1 || f.deliveries.Load() != 1 {
		t.Fatal("reviewed exact payment did not submit", sent)
	}
	out = c.HandleWithdrawal(t.Context(), approve)
	if !out.OK || f.signCalls.Load() != 1 || f.deliveries.Load() != 1 {
		t.Fatal("approval replay signed or dispatched again", out.Error)
	}
}
func TestWithdrawalStopJoinsBlockedWalletAndRetainsIntent(t *testing.T) {
	c, _, f, r := withdrawalFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	f.hook = func(ctx context.Context, stage string) error {
		if stage == "prepare" {
			close(entered)
			<-ctx.Done()
			<-release
			return ctx.Err()
		}
		return nil
	}
	if out := c.HandleWithdrawal(t.Context(), r); !out.OK {
		t.Fatal(out.Error)
	}
	<-entered
	done := make(chan struct{})
	go func() { c.Stop(); close(done) }()
	select {
	case <-done:
		t.Fatal("Stop returned before wallet joined")
	case <-time.After(30 * time.Millisecond):
	}
	c.mu.Lock()
	reserved := c.withdrawalScratch
	c.mu.Unlock()
	if reserved != WithdrawalWorkBytes {
		t.Fatal("work reservation released early")
	}
	once.Do(func() { close(release) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not join")
	}
	owner, e := f.journal.TryAcquire(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	o, e := owner.Lookup(r.OperationID)
	if e != nil || o.Stage != walletwithdraw.StageIntent {
		t.Fatal("Stop erased durable intent", e)
	}
}
func TestWithdrawalResetDuringScratchSignCannotCommitOrSend(t *testing.T) {
	c, h, f, r := withdrawalFixture(t)
	if o := c.HandleWithdrawal(t.Context(), r); !o.OK {
		t.Fatal(o.Error)
	}
	done := withdrawalWait(t, c, r)
	if done.State != "done" || done.Operation == nil || done.Operation.Draft == nil {
		withdrawalPrepareFailure(t, c, r, done)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	f.hook = func(ctx context.Context, stage string) error {
		if stage == "sign" {
			close(entered)
			<-release
		}
		return ctx.Err()
	}
	q := WithdrawalRequest{ID: 2, Version: 1, Command: "withdrawal_review", ViewID: r.ViewID, OperationID: r.OperationID, ActionID: strings.Repeat("1", 64), ReviewID: strings.Repeat("2", 64), ReviewKind: "send", DraftDigest: done.Operation.Draft.DraftDigest}
	if out := c.HandleWithdrawal(t.Context(), q); !out.OK {
		t.Fatal(out.Error)
	}
	q.Command, q.ReviewKind = "withdrawal_approve", ""
	if out := c.HandleWithdrawal(t.Context(), q); !out.OK {
		t.Fatal(out.Error)
	}
	<-entered
	h.mu.Lock()
	h.view = false
	h.mu.Unlock()
	c.mu.Lock()
	task := c.withdrawalTask
	c.mu.Unlock()
	once.Do(func() { close(release) })
	select {
	case <-task.done:
	case <-time.After(time.Second):
		t.Fatal("revoked signer failed to join")
	}
	owner, e := f.journal.TryAcquire(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	o, e := owner.Lookup(r.OperationID)
	if e != nil || o.Stage != walletwithdraw.StagePrepared || o.Signed != nil || f.deliveries.Load() != 0 {
		t.Fatal("revoked scratch signature became authority", e)
	}
}
func TestWithdrawalFenceRejectsMissingDoubleAndSwallowedCallbacks(t *testing.T) {
	for _, mode := range []string{"missing", "double", "swallow"} {
		t.Run(mode, func(t *testing.T) {
			c, _, _, r := withdrawalFixture(t)
			original := c.account.Lease
			c.account.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
				if mode == "missing" {
					return nil
				}
				return original(ctx, cap, func(a gymaccount.Account, check func() error) error {
					e := fn(a, check)
					if mode == "double" {
						_ = fn(a, check)
					}
					if mode == "swallow" {
						return nil
					}
					return e
				})
			}
			sentinel := errors.New("inner refusal")
			e := c.withdrawalFence(t.Context(), r.ViewID, gymaccount.View, nil)(func(func() error) error { return sentinel })
			if e == nil {
				t.Fatal("malicious capability callback accepted")
			}
		})
	}
}
func TestWithdrawalHeldViewRecoveryAndFreshReviewBoundary(t *testing.T) {
	c, _, _, r := withdrawalFixture(t)
	if out := c.HandleWithdrawal(t.Context(), r); !out.OK {
		t.Fatal(out.Error)
	}
	done := withdrawalWait(t, c, r)
	if done.State != "done" || done.Operation == nil || done.Operation.Draft == nil {
		withdrawalPrepareFailure(t, c, r, done)
	}
	var held atomic.Bool
	original := c.account.Lease
	c.account.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if held.Load() && cap == gymaccount.Withdrawal {
			return gymaccount.ErrHeld
		}
		return original(ctx, cap, fn)
	}
	held.Store(true)
	lookup := WithdrawalRequest{ID: 1, Version: 1, Command: "withdrawal_operation", ViewID: r.ViewID, OperationID: r.OperationID}
	if out := c.HandleWithdrawal(t.Context(), lookup); !out.OK || out.Operation.Stage != "prepared" {
		t.Fatal("held View cannot recover saved operation", out.Error)
	}
	review := WithdrawalRequest{ID: 1, Version: 1, Command: "withdrawal_review", ViewID: r.ViewID, OperationID: r.OperationID, ActionID: strings.Repeat("1", 64), ReviewID: strings.Repeat("2", 64), ReviewKind: "send", DraftDigest: done.Operation.Draft.DraftDigest}
	if out := c.HandleWithdrawal(t.Context(), review); out.OK || out.Error != "held" {
		t.Fatal("hold allowed review", out.Error)
	}
	held.Store(false)
	// The inherited CAS-only fixture assumes synchronous calls. Withdrawal
	// workers and exact-action polling are concurrent, like the real account
	// store: serialize those callbacks rather than misclassifying them nested.
	leaseGate := make(chan struct{}, 1)
	originalLease := c.account.Lease
	c.account.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		select {
		case leaseGate <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-leaseGate }()
		return originalLease(ctx, cap, fn)
	}
	c.account.AuthenticatedAtMS = time.Now().UnixMilli() - 300000
	if out := c.HandleWithdrawal(t.Context(), review); out.OK || out.Error != "reauthentication_required" {
		t.Fatal("five-minute boundary extended", out.Error)
	}
	if out := c.HandleWithdrawal(t.Context(), lookup); !out.OK {
		t.Fatal("fresh-auth expiry blocked View", out.Error)
	}
}
func TestWithdrawalBackupJoinsAuthoringBeforeWalletReuse(t *testing.T) {
	c, _, f, r := withdrawalFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	f.hook = func(ctx context.Context, stage string) error {
		if stage == "prepare" {
			close(entered)
			<-ctx.Done()
			<-release
			return ctx.Err()
		}
		return nil
	}
	if out := c.HandleWithdrawal(t.Context(), r); !out.OK {
		t.Fatal(out.Error)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("wallet not entered")
	}
	done := make(chan *poolBackupOperation, 1)
	errc := make(chan error, 1)
	go func() { op, e := c.beginPoolBackup(t.Context(), true); done <- op; errc <- e }()
	select {
	case <-done:
		t.Fatal("backup returned before authoring joined")
	case <-time.After(30 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	select {
	case op := <-done:
		if e := <-errc; e != nil || op == nil {
			t.Fatal(e)
		}
		c.finishPoolBackup(op, "")
	case <-time.After(time.Second):
		t.Fatal("backup drain deadlocked")
	}
}

func TestWithdrawalFailedOuterAdmissionNeverRunsBackend(t *testing.T) {
	for _, command := range []string{"prepare", "approve"} {
		t.Run(command, func(t *testing.T) {
			c, _, f, r := withdrawalFixture(t)
			if command == "approve" {
				if out := c.HandleWithdrawal(t.Context(), r); !out.OK {
					t.Fatal(out.Error)
				}
				done := withdrawalWait(t, c, r)
				if done.State != "done" || done.Operation == nil || done.Operation.Draft == nil {
					withdrawalPrepareFailure(t, c, r, done)
				}
				r = WithdrawalRequest{ID: 2, Version: 1, Command: "withdrawal_review", ViewID: r.ViewID, OperationID: r.OperationID, ActionID: strings.Repeat("1", 64), ReviewID: strings.Repeat("2", 64), ReviewKind: "send", DraftDigest: done.Operation.Draft.DraftDigest}
				if out := c.HandleWithdrawal(t.Context(), r); !out.OK {
					t.Fatal(out.Error)
				}
				r.Command, r.ReviewKind = "withdrawal_approve", ""
			}
			before := f.calls.Load()
			original := c.account.Lease
			var once atomic.Bool
			c.account.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
				e := original(ctx, cap, fn)
				if once.CompareAndSwap(false, true) {
					return errors.Join(e, errors.New("outer refused after callback"))
				}
				return e
			}
			out := c.HandleWithdrawal(t.Context(), r)
			if out.OK || out.Error != "outcome_unknown" {
				t.Fatal("outer failure accepted", out)
			}
			c.mu.Lock()
			active, reserved := c.withdrawalTask, c.withdrawalScratch
			c.mu.Unlock()
			if active != nil || reserved != 0 {
				t.Fatal("failed admission retained a worker/reservation")
			}
			if f.calls.Load() != before || f.signCalls.Load() != 0 || f.deliveries.Load() != 0 {
				t.Fatal("failed outer capability reached backend")
			}
			a := c.HandleWithdrawal(t.Context(), WithdrawalRequest{ID: 3, Version: 1, Command: "withdrawal_action", ViewID: r.ViewID, OperationID: r.OperationID, ActionID: r.ActionID})
			if !a.OK || a.Action.State != "outcome_unknown" {
				t.Fatal("exact uncertain metadata not recoverable", a.Error)
			}
		})
	}
}

func TestWithdrawalSendReviewInspectsBeforePublication(t *testing.T) {
	c, h, f, r := withdrawalFixture(t)
	if out := c.HandleWithdrawal(t.Context(), r); !out.OK {
		t.Fatal(out.Error)
	}
	done := withdrawalWait(t, c, r)
	if done.State != "done" || done.Operation == nil || done.Operation.Draft == nil {
		withdrawalPrepareFailure(t, c, r, done)
	}
	r = WithdrawalRequest{ID: 2, Version: 1, Command: "withdrawal_review", ViewID: r.ViewID, OperationID: r.OperationID, ActionID: strings.Repeat("1", 64), ReviewID: strings.Repeat("2", 64), ReviewKind: "send", DraftDigest: done.Operation.Draft.DraftDigest}
	var inspected atomic.Int32
	f.hook = func(ctx context.Context, stage string) error {
		if stage == "inspect" {
			inspected.Add(1)
			return errors.New("input not available")
		}
		return nil
	}
	out := c.HandleWithdrawal(t.Context(), r)
	if out.OK || inspected.Load() != 1 {
		t.Fatal("uninspected review published", out)
	}
	c.mu.Lock()
	review := c.withdrawalReviewValue
	c.mu.Unlock()
	if review != nil {
		t.Fatal("failed inspection retained an actionable review")
	}
	f.hook = func(ctx context.Context, stage string) error {
		if stage == "inspect" {
			inspected.Add(1)
			h.mu.Lock()
			h.view = false
			h.mu.Unlock()
		}
		return nil
	}
	out = c.HandleWithdrawal(t.Context(), r)
	if out.OK || inspected.Load() != 2 {
		t.Fatal("revoked view published inspected review", out)
	}
	if f.signCalls.Load() != 0 || f.deliveries.Load() != 0 {
		t.Fatal("review signed or transmitted")
	}
}

// Preserve the exact journal error when a preparation unexpectedly fails. This
// is diagnostic only; it neither retries nor changes the operation.
func withdrawalPrepareFailure(t *testing.T, c *PoolChild, r WithdrawalRequest, a WithdrawalAction) {
	t.Helper()
	owner, e := c.withdrawal.Journal.TryAcquire(t.Context())
	if e != nil {
		t.Fatalf("prepare failed: action=%+v journal=%v", a, e)
	}
	row, lookup := owner.Lookup(r.OperationID)
	closeErr := owner.Close()
	t.Fatalf("prepare failed: action=%+v stage=%s lookup=%v close=%v", a, row.Stage, lookup, closeErr)
}
