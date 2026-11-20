// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type hostedRecoveryWireController struct {
	HostedRecoveryControllerAPI
	starts atomic.Int32
}

func (c *hostedRecoveryWireController) HostedRecoverySnapshot() HostedRecoverySnapshot {
	return BlankHostedRecoverySnapshot()
}
func (c *hostedRecoveryWireController) Start() error { c.starts.Add(1); return nil }
func (c *hostedRecoveryWireController) Stop()        {}
func (c *hostedRecoveryWireController) Close() error { return nil }
func TestHostedRecoveryRejectsLegacyRootDispatch(t *testing.T) {
	for _, command := range []string{"snapshot", "start", "stop", "login_fixture", "logout", "reset_fixture", "export_recovery", "verify_backup", "cancel_pool_backup"} {
		t.Run(command, func(t *testing.T) {
			c := &hostedRecoveryWireController{}
			var out bytes.Buffer
			raw, _ := json.Marshal(PoolRequest{ID: 1, Command: command})
			if e := ServeHostedRecovery(t.Context(), bytes.NewReader(append(raw, '\n')), &out, c); e != nil {
				t.Fatal(e)
			}
			if c.starts.Load() != 0 {
				t.Fatal("legacy start reached new owner")
			}
			var response PoolResponse
			if json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response) != nil || response.OK || response.Error != "invalid_request" {
				t.Fatalf("legacy root accepted: %s", out.Bytes())
			}
		})
	}
	c := &hostedRecoveryWireController{}
	var out bytes.Buffer
	if e := ServeHostedRecovery(t.Context(), strings.NewReader("{\"id\":1,\"version\":1,\"command\":\"hosted_recovery_snapshot\"}\n"), &out, c); e != nil {
		t.Fatal(e)
	}
	response, e := ParseHostedRecoveryResponse(bytes.TrimSpace(out.Bytes()))
	if e != nil || !response.OK || response.Snapshot == nil || response.Snapshot.Kind != "inactive" {
		t.Fatal("new root refused", e, string(out.Bytes()))
	}
}
func TestHostedRecoveryIndependentBindingAndClosedFrames(t *testing.T) {
	raw, e := os.ReadFile("../../../slugwallet/wallet/testdata/hosted-allocation/reference.json")
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Bindings []struct {
			Binding wallet.AllocationBinding `json:"binding"`
			ID      string                   `json:"binding_id"`
		}
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Bindings) != 2 {
		t.Fatal("independent corpus")
	}
	for _, v := range corpus.Bindings {
		b := v.Binding
		m := b.InitialManifest
		scope := HostedAllocationScope{1, b.InventoryID, v.ID, recoveryvault.CurrentBinding{Version: 1, HostedVaultID: b.HostedVaultID, RetentionID: b.RetentionID, WalletOwnerID: b.WalletOwnerID, SourceBindingID: b.SourceBindingID, VaultBinding: recoveryvault.Binding{Version: 1, AuthorityID: b.AuthorityID, AccountID: b.AccountID, PrincipalID: b.PrincipalID, ProvisionID: b.ProvisionID, Network: m.Network, Genesis: m.Genesis, Profile: m.Profile, RootPublicKey: m.RootPublicKey, GymPublicKey: m.GymPublicKey, GymID: m.GymID}}}
		got, e := HostedAllocationBinding(scope)
		if e != nil || allocationJSONHash(got) != allocationJSONHash(b) {
			t.Fatal("independent binding", e)
		}
		scope.CurrentBinding.WalletOwnerID = strings.Repeat("f", 64)
		if ValidateHostedAllocationScope(scope) == nil {
			t.Fatal("foreign owner adopted its own digest")
		}
	}
	for _, bad := range []string{
		`{"id":1,"version":1,"command":"hosted_recovery_start"}`,
		`{"id":1,"version":1,"command":"hosted_recovery_snapshot","view_id":"` + strings.Repeat("a", 64) + `"}`,
		`{"id":1,"id":1,"version":1,"command":"hosted_recovery_snapshot"}`,
		`{"version":1,"id":1,"command":"hosted_recovery_snapshot"}`,
		`{"id":true,"version":1,"command":"hosted_recovery_snapshot"}`,
	} {
		if _, e := ParseHostedRecoveryRequest([]byte(bad)); e == nil {
			t.Fatal("accepted nonclosed frame", bad)
		}
	}
}
func TestHostedRecoveryUncertaintyDominatesCancellation(t *testing.T) {
	for _, tail := range []error{context.Canceled, gymaccount.ErrUnauthorized, wallet.ErrAllocationCapacity} {
		if got := HostedRecoveryError(errors.Join(wallet.ErrAllocationOutcomeUnknown, tail)); got != "outcome_unknown" {
			t.Fatal(got)
		}
		if got := HostedRecoveryError(errors.Join(recoveryvault.ErrExportUncertain, recoveryvault.ErrCleanup, tail)); got != "backup_export_uncertain" {
			t.Fatal(got)
		}
	}
	if HostedRecoveryError(gymaccount.ErrHeld) != "forbidden" {
		t.Fatal("hold changed")
	}
}
func hostedRecoveryFenceFixture(t *testing.T) *HostedRecoveryOwner {
	t.Helper()
	s, e := gymaccount.Create(filepath.Join(t.TempDir(), "accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 16, MaxChallenges: 8, MaxEvents: 32})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = s.Close() })
	session, e := s.Login(t.Context(), gymaccount.Principal{Issuer: "fixture:local", Subject: "alice", AuthenticatedAtMS: time.Now().UnixMilli()})
	if e != nil {
		t.Fatal(e)
	}
	life, cancel := context.WithCancel(context.Background())
	o := &HostedRecoveryOwner{accounts: s, session: session, life: life, cancel: cancel, prepared: true, epoch: func(ctx context.Context, fn func(func() error) error) error {
		return fn(func() error { return ctx.Err() })
	}}
	t.Cleanup(func() { _ = o.Close() })
	return o
}
func TestHostedRecoveryAdmissionRejectsBrokenOuterEpoch(t *testing.T) {
	for _, kind := range []string{"missing", "double-swallowed", "outer-error"} {
		t.Run(kind, func(t *testing.T) {
			o := hostedRecoveryFenceFixture(t)
			o.epoch = func(ctx context.Context, fn func(func() error) error) error {
				if kind == "missing" {
					return nil
				}
				_ = fn(func() error { return nil })
				if kind == "double-swallowed" {
					_ = fn(func() error { return nil })
					return nil
				}
				return errors.New("outer refusal")
			}
			if op, e := o.begin(t.Context(), true); e == nil || op != nil {
				t.Fatal("broken fence admitted operation")
			}
			if o.operation != nil {
				t.Fatal("failed admission retained live task")
			}
		})
	}
}
func TestHostedRecoveryCloseJoinsAndPreservesCleanupFailure(t *testing.T) {
	o := hostedRecoveryFenceFixture(t)
	op, e := o.begin(t.Context(), true)
	if e != nil {
		t.Fatal(e)
	}
	release := make(chan struct{})
	var released atomic.Bool
	unblock := func() {
		if released.CompareAndSwap(false, true) {
			close(release)
		}
	}
	defer unblock()
	failure := errors.New("joined cleanup failed")
	go func() { <-op.ctx.Done(); <-release; o.cleanup(failure); o.finish(op, "backup_storage_failed") }()
	done := make(chan error, 1)
	go func() { done <- o.Close() }()
	<-op.ctx.Done()
	select {
	case <-done:
		t.Fatal("Close returned before cleanup")
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	select {
	case e := <-done:
		if !errors.Is(e, failure) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not join")
	}
	if !errors.Is(o.Close(), failure) {
		t.Fatal("sticky failure lost")
	}
}
func TestHostedRecoveryReceiveStopJoins(t *testing.T) {
	c, _, _, released := poolChildLifecycleFixture(t)
	ctx, cancel := context.WithCancel(c.ctx)
	op := &hostedReceiveOperation{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	c.receiveOp = op
	release := make(chan struct{})
	var once atomic.Bool
	unblock := func() {
		if once.CompareAndSwap(false, true) {
			close(release)
		}
	}
	defer unblock()
	go func() {
		<-ctx.Done()
		<-release
		c.mu.Lock()
		c.receiveOp = nil
		close(op.done)
		c.mu.Unlock()
	}()
	done := make(chan struct{})
	go func() { c.Stop(); close(done) }()
	<-ctx.Done()
	select {
	case <-done:
		t.Fatal("Stop returned before receive joined")
	case <-time.After(20 * time.Millisecond):
	}
	if released.Load() != 0 {
		t.Fatal("parent borrow released")
	}
	unblock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop not joined")
	}
}

func TestHostedRecoveryReceiveStatusAndResultPublishTogether(t *testing.T) {
	c, _, _, _ := poolChildLifecycleFixture(t)
	ctx, cancel := context.WithCancel(c.ctx)
	op := &hostedReceiveOperation{ctx: ctx, cancel: cancel, done: make(chan struct{}), command: "begin", job: HostedReceiveJob{State: "running"}}
	c.receiveOp = op
	c.stable.laneGate <- struct{}{}
	ready := HostedReceiveJob{State: "ready", Address: &HostedReceiveAddress{Address: "not-disclosed-on-status-failure"}}
	status := wallet.AllocationInventoryStatus{Revision: 7, IssuedReceiveID: strings.Repeat("a", 64)}
	failure := errors.New("status read failed")
	if e := c.publishHostedReceiveResult(t.Context(), op, ready, status, failure); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	if c.receiveLast != nil || c.allocationStatus.Revision != 0 || op.job.Address != nil {
		t.Fatal("failed status disclosed address or metadata")
	}
	c.joinHostedReceive(op, c.stable)
	if c.receiveOp != nil || c.receiveLast != nil {
		t.Fatal("failed publication left cached running action")
	}
	ctx, cancel = context.WithCancel(c.ctx)
	op = &hostedReceiveOperation{ctx: ctx, cancel: cancel, done: make(chan struct{}), command: "redisplay", job: HostedReceiveJob{State: "running"}}
	c.receiveOp = op
	c.stable.laneGate <- struct{}{}
	if e := c.publishHostedReceiveResult(t.Context(), op, ready, status, nil); e != nil {
		t.Fatal(e)
	}
	if c.receiveLast == nil || c.receiveLast.State != "ready" || c.allocationStatus != status {
		t.Fatal("terminal result and status did not publish together")
	}
	c.joinHostedReceive(op, c.stable)
}

func TestHostedRecoveryWithdrawalVersionBeforeResolve(t *testing.T) {
	s := &allocatedWithdrawalWallet{}
	for _, version := range []uint32{0, 2} {
		if _, e := s.Prepare(t.Context(), wallet.CandidateWithdrawalRequest{Version: version}); !errors.Is(e, wallet.ErrAllocationInvalid) {
			t.Fatal(e)
		}
	}
}
