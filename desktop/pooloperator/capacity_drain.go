// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
)

// CapacityStatus reports intake state only. Draining leaves the original
// settlement scheduler, exact pending transactions and retained round refresh
// alive. It never promises payment of below-minimum, fee- or funding-held carry.
type CapacityStatus struct {
	Reservation       CapacityReservation `json:"reservation"`
	Mode              string              `json:"mode"`
	Reason            string              `json:"reason"`
	Submissions       uint64              `json:"submissions"`
	OverBound         bool                `json:"over_bound"`
	CanonicalRecovery bool                `json:"canonical_recovery"`
	RecoveryBlocks    uint64              `json:"recovery_blocks"`
	AtMS              int64               `json:"at_ms"`
}

// Only a nonempty error tree made entirely of known capacity leaves can enter
// the financial-drain branch. Joined integrity/cleanup/custody errors stay fatal.
func capacityOnly(err error) bool {
	switch err {
	case poolshare.ErrCapacity, poolround.ErrCapacity, registry.ErrPublicCapacity, fighterownership.ErrCapacity, prospectservice.ErrIntakeDraining:
		return true
	}
	if many, ok := err.(interface{ Unwrap() []error }); ok {
		children := many.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !capacityOnly(child) {
				return false
			}
		}
		return true
	}
	if one, ok := err.(interface{ Unwrap() error }); ok {
		return capacityOnly(one.Unwrap())
	}
	return false
}

// updateIntake is called only by the two joined maintenance loops, or Run before
// its listeners. The mutex coordinates their transitions, not financial work.
// Each concrete controller independently joins its original active writer.
func (r *Runtime) updateIntake(ctx context.Context, publicReady, roundReady bool, cause string) error {
	r.intakeMu.Lock()
	defer r.intakeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	r.intakePublicReady = r.intakePublicReady || publicReady
	r.intakeRoundReady = r.intakeRoundReady || roundReady
	pool, err := r.intake.Status(ctx)
	if err != nil {
		return err
	}
	mode, err := r.admissionServer.OperatorIntakeMode(ctx)
	if err != nil {
		return err
	}
	if cause == "" && pool.Mode == "draining" {
		cause = "pool_capacity"
		if pool.OverBound {
			cause = "legacy_over_bound"
		} else if pool.Maximum == 0 {
			cause = "no_reservation"
		}
	}
	if cause == "" && mode == "draining" {
		cause = "admission_capacity"
	}
	if cause != "" && r.intakeReason == "" {
		r.intakeReason = cause
	}
	if r.intakeReason != "" {
		// Keep custody and the original pool alive: saved submissions retain
		// exact recovery, and unseen canonical winners have their own audited
		// whole-history reservation. Ordinary intake remains closed.
		if err = r.admissionServer.DrainOperatorIntake(ctx); err != nil {
			return err
		}
		if err = r.intake.Drain(ctx); err != nil {
			return err
		}
		pool.Mode = "draining"
	} else if r.intakePublicReady && r.intakeRoundReady {
		err = r.admissionServer.EnableOperatorIntake(ctx)
		if err == nil {
			err = r.intake.Enable(ctx)
		}
		if err != nil {
			if !capacityOnly(err) {
				return err
			}
			r.intakeReason = "pool_capacity"
			if e := r.admissionServer.DrainOperatorIntake(ctx); e != nil {
				return errors.Join(err, e)
			}
			if e := r.intake.Drain(ctx); e != nil {
				return errors.Join(err, e)
			}
			pool.Mode = "draining"
		} else {
			pool.Mode = "enabled"
		}
	}
	r.mu.Lock()
	r.status.Capacity = CapacityStatus{Reservation: r.reservation, Mode: pool.Mode, Reason: r.intakeReason, Submissions: pool.Submissions, OverBound: pool.OverBound, CanonicalRecovery: pool.CanonicalRecovery, RecoveryBlocks: pool.RecoveryBlocks, AtMS: time.Now().UTC().UnixMilli()}
	r.mu.Unlock()
	return ctx.Err()
}

// The retained-refresh branch may wait for exact known observation contention;
// it must not hide a second error behind an errors.Is match.
func retainedRoundRetry(err error) bool {
	switch err {
	case poolround.ErrBusy, poolround.ErrNode, poolround.ErrSourceUnavailable, context.DeadlineExceeded:
		return true
	}
	if many, ok := err.(interface{ Unwrap() []error }); ok {
		children := many.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !retainedRoundRetry(child) {
				return false
			}
		}
		return true
	}
	if one, ok := err.(interface{ Unwrap() error }); ok {
		return retainedRoundRetry(one.Unwrap())
	}
	return false
}
