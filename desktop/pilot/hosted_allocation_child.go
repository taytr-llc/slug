// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

func (c *PoolChild) attachHostedAllocator(ctx context.Context) error {
	h := c.resources.Hosted
	if h == nil || h.allocation == nil {
		return nil
	}
	if c.account.Hosted == nil || c.wallet == nil {
		return ErrPoolRuntime
	}
	inv, e := h.allocation.Inventory(c.account.Manifest.Account.Fixture)
	if e != nil {
		return e
	}
	if !c.wallet.mu.TryLock() {
		return ErrPoolRuntimeBusy
	}
	w := c.wallet.wallet
	available := !c.wallet.closed && w != nil
	c.wallet.mu.Unlock()
	if !available {
		return ErrPoolRuntime
	}
	// Prepare owns the unexposed child and retained wallet until Close joins it.
	c.wallet.allocation, e = inv.Attach(ctx, w, c.account.Hosted.login.Token, wallet.HostedAllocationEpochFence(c.account.Hosted.epoch))
	if e != nil {
		return e
	}
	h.allocation.mu.Lock()
	intent, ok := h.allocation.intents[c.account.Manifest.Account.Fixture]
	h.allocation.mu.Unlock()
	if !ok {
		return ErrManifestInitialization
	}
	c.wallet.allocationRecipientID = intent.PoolAllocationID
	c.allocationScope, e = hostedScopeForBinding(inv.Binding())
	if e != nil {
		return e
	}
	c.allocationStatus, e = inv.Status(ctx, c.account.Hosted.login.Token, wallet.HostedAllocationEpochFence(c.account.Hosted.epoch))
	return e
}
func (c *PoolChild) HostedAllocationSnapshot() (HostedAllocationScope, string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.prepared || c.wallet == nil || c.wallet.allocation == nil {
		return HostedAllocationScope{}, "", "", ErrPoolRuntime
	}
	return c.allocationScope, c.allocationStatus.PendingReceiveID, c.allocationStatus.IssuedReceiveID, nil
}
func (c *PoolChild) refreshHostedAllocation(ctx context.Context) error {
	if c.resources.Hosted == nil || c.resources.Hosted.allocation == nil {
		return nil
	}
	inv, e := c.resources.Hosted.allocation.Inventory(c.account.Manifest.Account.Fixture)
	if e != nil {
		return e
	}
	status, e := inv.Status(ctx, c.account.Hosted.login.Token, wallet.HostedAllocationEpochFence(c.account.Hosted.epoch))
	if e != nil {
		return e
	}
	return c.publish(ctx, func() error { c.allocationStatus = status; return nil })
}
func (c *PoolChild) exportHostedInventoryRecovery(op *poolBackupOperation, pin recoveryfile.Pin) (code string, resultErr error) {
	code = "backup_unavailable"
	h := c.resources.Hosted
	if h == nil || h.allocation == nil || c.account.Hosted == nil || recoveryfile.ValidatePin(pin, recoveryfile.CreateMode) != nil {
		return code, ErrPoolRuntime
	}
	release, e := h.acquireRecovery()
	if e != nil {
		return code, e
	}
	defer release()
	protected, e := c.poolBackupProtected()
	if e != nil {
		return code, e
	}
	protected = append(protected, filepath.Dir(filepath.Dir(filepath.Dir(c.resources.Config.DataDir))))
	destination, e := recoveryvault.OpenCurrentDestination(op.ctx, recoveryvault.CurrentDestinationPin{Version: 1, Path: pin.Path, ParentDevice: pin.Parent.Device, ParentInode: pin.Parent.Inode}, protected)
	if e != nil {
		return code, e
	}
	cleanup := func(e error) {
		if e == nil {
			return
		}
		if code != "backup_export_uncertain" {
			code = "backup_storage_failed"
		}
		c.mu.Lock()
		c.backupCloseErr = errors.Join(c.backupCloseErr, e)
		c.mu.Unlock()
		resultErr = errors.Join(resultErr, e)
	}
	defer func() { cleanup(destination.Close()) }()
	inv, e := h.allocation.Inventory(c.account.Manifest.Account.Fixture)
	if e != nil {
		return code, e
	}
	head, e := inv.CaptureRecoveryHead(op.ctx, c.account.Hosted.login.Token, wallet.HostedAllocationEpochFence(c.account.Hosted.epoch))
	if e != nil {
		return code, e
	}
	prepared, e := h.vault.PrepareInventoryExport(op.ctx, c.account.Hosted.login.Token, head, destination, c.account.Hosted.epoch)
	if e != nil {
		return code, e
	}
	defer func() { cleanup(prepared.Close()) }()
	e = prepared.Publish(op.ctx)
	switch {
	case errors.Is(e, recoveryvault.ErrExportUncertain), errors.Is(e, recoveryvault.ErrOutcomeUnknown):
		code = "backup_export_uncertain"
	case errors.Is(e, recoveryvault.ErrCleanup):
		code = "backup_storage_failed"
	case e == nil:
		code = ""
	}
	return code, e
}

type hostedReceiveOperation struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	command string
	job     HostedReceiveJob
}

func cloneHostedReceive(j HostedReceiveJob) HostedReceiveJob {
	if j.Address != nil {
		a := *j.Address
		j.Address = &a
	}
	return j
}
func (c *PoolChild) hostedReceiveCheck(ctx context.Context, view string) error {
	if view != c.account.Manifest.ViewID {
		return ErrHostedStaleView
	}
	if c.resources.Hosted == nil || c.resources.Hosted.allocation == nil || c.wallet == nil || c.wallet.allocation == nil {
		return ErrPoolRuntime
	}
	return c.poolBackupFence(ctx, gymaccount.View, nil)(func(check func() error) error { return check() })
}
func (c *PoolChild) BeginHostedReceive(ctx context.Context, view, action, operation string) (HostedReceiveJob, error) {
	return c.beginHostedReceive(ctx, "begin", view, action, operation)
}
func (c *PoolChild) ResumeHostedReceive(ctx context.Context, view, action, operation string) (HostedReceiveJob, error) {
	return c.beginHostedReceive(ctx, "resume", view, action, operation)
}
func (c *PoolChild) RedisplayHostedReceive(ctx context.Context, view, action, operation string) (HostedReceiveJob, error) {
	return c.beginHostedReceive(ctx, "redisplay", view, action, operation)
}
func (c *PoolChild) beginHostedReceive(parent context.Context, command, view, action, operation string) (HostedReceiveJob, error) {
	if parent == nil || !payoutNonzeroID(action) || !payoutNonzeroID(operation) {
		return HostedReceiveJob{}, wallet.ErrAllocationInvalid
	}
	if !c.stopMu.TryLock() {
		return HostedReceiveJob{}, wallet.ErrAllocationBusy
	}
	defer c.stopMu.Unlock()
	if e := c.hostedReceiveCheck(parent, view); e != nil {
		return HostedReceiveJob{}, e
	}
	c.mu.Lock()
	old := c.receiveOp
	last := c.receiveLast
	if old != nil {
		j := cloneHostedReceive(old.job)
		same := old.command == command && j.ActionID == action && j.OperationID == operation
		c.mu.Unlock()
		if same {
			return j, nil
		}
		return HostedReceiveJob{}, wallet.ErrAllocationBusy
	}
	if last != nil && last.ActionID == action {
		j := cloneHostedReceive(*last)
		same := c.receiveLastCommand == command && j.OperationID == operation
		c.mu.Unlock()
		if same {
			return j, nil
		}
		return HostedReceiveJob{}, wallet.ErrAllocationConflict
	}
	busy := c.run != nil || c.backup != nil || c.backupCloseErr != nil || c.withdrawalTask != nil || c.payoutOp != nil || c.activityOp != nil || c.activityProjection != nil || c.snapshot.State != "stopped" && c.snapshot.State != "failed"
	s := c.stable
	scope := c.allocationScope
	c.mu.Unlock()
	if busy || s == nil {
		return HostedReceiveJob{}, wallet.ErrAllocationBusy
	}
	s.stableMu.Lock()
	busy = s.active != nil || s.queued != nil || s.evidenceBusyLocked()
	s.stableMu.Unlock()
	if busy {
		return HostedReceiveJob{}, wallet.ErrAllocationBusy
	}
	select {
	case s.laneGate <- struct{}{}:
	default:
		return HostedReceiveJob{}, wallet.ErrAllocationBusy
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	stop := context.AfterFunc(c.ctx, cancel)
	op := &hostedReceiveOperation{ctx: ctx, cancel: func() { stop(); cancel() }, done: make(chan struct{}), command: command, job: HostedReceiveJob{Version: 1, ActionID: action, OperationID: operation, ViewID: view, Allocation: scope, State: "running"}}
	e := c.publish(ctx, func() error {
		if c.receiveOp != nil || c.backup != nil || c.run != nil {
			return wallet.ErrAllocationBusy
		}
		c.receiveOp = op
		c.receiveLast = nil
		c.receiveLastCommand = ""
		return nil
	})
	if e != nil {
		op.cancel()
		c.mu.Lock()
		if c.receiveOp == op {
			c.receiveOp = nil
		}
		c.mu.Unlock()
		<-s.laneGate
		return HostedReceiveJob{}, e
	}
	initial := cloneHostedReceive(op.job)
	go c.runHostedReceive(op, s)
	return initial, nil
}
func (c *PoolChild) runHostedReceive(op *hostedReceiveOperation, s *StableController) {
	defer c.joinHostedReceive(op, s)
	a := c.wallet.allocation
	id := op.job.OperationID
	var ack *wallet.AllocationAcknowledgement
	var e error
	switch op.command {
	case "begin":
		var p *wallet.AllocationPlan
		p, e = a.PlanReceive(op.ctx, id)
		if e == nil {
			var r *wallet.AllocationReservation
			r, e = a.Reserve(op.ctx, p)
			if e == nil {
				var receipt *wallet.AllocationReceipt
				receipt, e = a.Consume(op.ctx, r)
				if e == nil {
					ack, e = a.Acknowledge(op.ctx, receipt)
				}
			}
		}
	case "resume":
		ack, e = a.Resolve(op.ctx, id)
		if errors.Is(e, wallet.ErrAllocationResumeRequired) {
			var r *wallet.AllocationReservation
			r, e = a.Resume(op.ctx, id)
			if e == nil {
				var receipt *wallet.AllocationReceipt
				receipt, e = a.Consume(op.ctx, r)
				if e == nil {
					ack, e = a.Acknowledge(op.ctx, receipt)
				}
			}
		}
	case "redisplay":
		ack, e = a.Resolve(op.ctx, id)
	}
	j := op.job
	if e == nil {
		var final *wallet.AcknowledgedAllocation
		final, e = a.Finalize(op.ctx, ack)
		if e == nil {
			var addr wallet.AllocationAddress
			addr, e = final.Details()
			if e == nil {
				j.Address = &HostedReceiveAddress{addr.Address, addr.Script, addr.Purpose, addr.Coin, addr.Account, addr.Branch, addr.Index, addr.InventoryRevision}
				j.State = "ready"
				if ValidateHostedReceiveJob(j) != nil {
					j.Address = nil
					e = wallet.ErrAllocationConflict
				}
			}
		}
	}
	if e != nil {
		j.Address = nil
		j.Error = HostedRecoveryError(e)
		j.State = "failed"
		switch {
		case errors.Is(e, wallet.ErrAllocationOutcomeUnknown):
			j.State, j.Error = "unknown", "outcome_unknown"
		case errors.Is(e, wallet.ErrAllocationResumeRequired):
			j.State, j.Error = "needs_resume", "allocation_resume_required"
		case op.ctx.Err() != nil:
			j.State, j.Error = "cancelled", "cancelled"
		}
	}
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
	defer cancel()
	inv, statusErr := c.resources.Hosted.allocation.Inventory(c.account.Manifest.Account.Fixture)
	var status wallet.AllocationInventoryStatus
	if statusErr == nil {
		status, statusErr = inv.Status(ctx, c.account.Hosted.login.Token, wallet.HostedAllocationEpochFence(c.account.Hosted.epoch))
	}
	_ = c.publishHostedReceiveResult(ctx, op, j, status, statusErr)
}

func (c *PoolChild) joinHostedReceive(op *hostedReceiveOperation, s *StableController) {
	<-s.laneGate
	op.cancel()
	c.mu.Lock()
	if c.receiveOp == op {
		c.receiveOp = nil
	}
	close(op.done)
	c.mu.Unlock()
}

// Status and the terminal result are one original-view publication. A failed
// status read leaves no address or cached running job after the operation joins;
// an exact subsequent lookup must reconcile the durable ID without mutation.
func (c *PoolChild) publishHostedReceiveResult(ctx context.Context, op *hostedReceiveOperation, j HostedReceiveJob, status wallet.AllocationInventoryStatus, statusErr error) error {
	if statusErr != nil {
		return statusErr
	}
	return c.publish(ctx, func() error {
		if c.receiveOp != op {
			return gymaccount.ErrUnauthorized
		}
		c.allocationStatus = status
		op.job = cloneHostedReceive(j)
		copy := cloneHostedReceive(j)
		c.receiveLast = &copy
		c.receiveLastCommand = op.command
		return nil
	})
}
func (c *PoolChild) LookupHostedReceive(ctx context.Context, view, action, operation string) (HostedReceiveJob, error) {
	if !payoutNonzeroID(action) || !payoutNonzeroID(operation) {
		return HostedReceiveJob{}, wallet.ErrAllocationInvalid
	}
	if e := c.hostedReceiveCheck(ctx, view); e != nil {
		return HostedReceiveJob{}, e
	}
	c.mu.Lock()
	var j *HostedReceiveJob
	if c.receiveOp != nil {
		copy := cloneHostedReceive(c.receiveOp.job)
		j = &copy
	} else if c.receiveLast != nil {
		copy := cloneHostedReceive(*c.receiveLast)
		j = &copy
	}
	c.mu.Unlock()
	if j != nil && j.ActionID == action && j.OperationID == operation {
		if e := c.hostedReceiveCheck(ctx, view); e != nil {
			return HostedReceiveJob{}, e
		}
		return *j, nil
	}
	e := c.wallet.allocation.LookupReceiveAllocation(ctx, operation)
	if e == nil {
		return HostedReceiveJob{}, wallet.ErrAllocationResumeRequired
	}
	return HostedReceiveJob{}, e
}
func (c *PoolChild) CancelHostedReceive(ctx context.Context, view, action, operation string) (HostedReceiveJob, error) {
	if e := c.hostedReceiveCheck(ctx, view); e != nil {
		return HostedReceiveJob{}, e
	}
	c.mu.Lock()
	op := c.receiveOp
	if op != nil && (op.job.ActionID != action || op.job.OperationID != operation) {
		c.mu.Unlock()
		return HostedReceiveJob{}, wallet.ErrAllocationConflict
	}
	if op != nil {
		op.cancel()
	}
	c.mu.Unlock()
	if op != nil {
		<-op.done
	}
	return c.LookupHostedReceive(ctx, view, action, operation)
}
func (c *PoolChild) cancelHostedReceive() {
	c.mu.Lock()
	op := c.receiveOp
	if op != nil {
		op.cancel()
	}
	c.mu.Unlock()
	if op != nil {
		<-op.done
	}
}
