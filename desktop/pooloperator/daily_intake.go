// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectservice"
)

// All state here belongs to the original runtime's intakeMu. The first pool
// cause remains available, but cannot hide a later independent admission hold.
// These are lifetime observations, not persisted capacity or birth authority.
func (r *Runtime) noteDailyIntakeCause(cause string) {
	if cause == "" {
		return
	}
	if r.intakeReason == "" {
		r.intakeReason = cause
	}
	if cause != "pool_capacity" && r.intakeAdmissionReason == "" {
		r.intakeAdmissionReason = cause
	}
}

func (r *Runtime) noteDailyPoolStatus(pool poolshare.OperatorAdmissionStatus) {
	if pool.OverBound {
		r.noteDailyIntakeCause("legacy_over_bound")
	} else if pool.Maximum == 0 {
		r.noteDailyIntakeCause("no_reservation")
	} else if pool.Mode == "draining" {
		r.noteDailyIntakeCause("pool_capacity")
	}
}

// Only the exact error from the selected original Enable operation supplies
// its capacity transition. An admission refusal never becomes a pool-only hold;
// wrappers/mixed failures retain their original error and stop this operation.
func (r *Runtime) dailyEnableFailure(err error, admission bool) error {
	if admission && err == prospectservice.ErrIntakeDraining {
		r.noteDailyIntakeCause("admission_capacity")
		return nil
	}
	if !admission && err == poolshare.ErrCapacity {
		r.noteDailyIntakeCause("pool_capacity")
		return nil
	}
	return err
}

func (r *Runtime) holdDailyAdmission(ctx context.Context) error {
	if err := r.admissionServer.DrainOperatorIntake(ctx); err != nil {
		return err
	}
	return r.intake.Drain(ctx)
}

// The caller retains the existing single 30-second transition budget and joined
// original controllers. Neither readiness nor "enabled" promises free birth
// capacity: every request still checks original proof, session, clock and limits.
func (r *Runtime) updateDailyIntakeLocked(ctx context.Context, publicReady, roundReady bool, cause string) error {
	r.intakePublicReady = r.intakePublicReady || publicReady
	r.intakeRoundReady = r.intakeRoundReady || roundReady
	// Retain an explicit hard cause before a later source audit can return Busy.
	r.noteDailyIntakeCause(cause)
	mode, err := r.admissionServer.OperatorIntakeMode(ctx)
	if err != nil {
		return err
	}
	if mode == "draining" {
		r.noteDailyIntakeCause("admission_capacity")
	}
	pool, err := r.intake.Status(ctx)
	if err != nil {
		return err
	}
	r.noteDailyPoolStatus(pool)
	if r.intakeAdmissionReason != "" {
		if err = r.holdDailyAdmission(ctx); err != nil {
			return err
		}
		pool.Mode = "draining"
	} else {
		if r.intakeReason != "" {
			if err = r.intake.Drain(ctx); err != nil {
				return err
			}
			pool.Mode = "draining"
		}
		if r.intakePublicReady && r.intakeRoundReady {
			// This also enables a freshly reopened, initially paused admission
			// owner when the original source is already full. No mining is restarted.
			if err = r.admissionServer.EnableOperatorIntake(ctx); err != nil {
				if err = r.dailyEnableFailure(err, true); err != nil {
					return err
				}
			} else if r.intakeReason == "" {
				if err = r.intake.Enable(ctx); err != nil {
					if err = r.dailyEnableFailure(err, false); err != nil {
						return err
					}
					// A concurrent source admission may have filled the envelope.
					// Recheck over-bound/no-reservation before allowing delivery.
					pool, err = r.intake.Status(ctx)
					if err != nil {
						return err
					}
					r.noteDailyPoolStatus(pool)
				} else {
					pool.Mode = "enabled"
				}
			}
			if r.intakeAdmissionReason != "" {
				if err = r.holdDailyAdmission(ctx); err != nil {
					return err
				}
				pool.Mode = "draining"
			} else if r.intakeReason != "" {
				if err = r.intake.Drain(ctx); err != nil {
					return err
				}
				pool.Mode = "draining"
			}
		}
	}
	reason := r.intakeReason
	if r.intakeAdmissionReason != "" {
		reason = r.intakeAdmissionReason
	}
	r.mu.Lock()
	r.status.Capacity = CapacityStatus{Reservation: r.reservation, DailyReservation: r.dailyReservation, Mode: pool.Mode, Reason: reason, Submissions: pool.Submissions, OverBound: pool.OverBound, CanonicalRecovery: pool.CanonicalRecovery, RecoveryBlocks: pool.RecoveryBlocks, AtMS: time.Now().UTC().UnixMilli()}
	r.mu.Unlock()
	return ctx.Err()
}
