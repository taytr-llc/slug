// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"sort"

	"github.com/taytr-llc/slug/internal/poolcapacity"
	"github.com/taytr-llc/slug/wire/v2"
)

// DailyRecoveryBudget is selected by original-owner composition. GymIDs must be
// the exact sorted settlement destination set and contain every enrollment. It
// grants no session, node, settlement or signing authority.
type DailyRecoveryBudget struct {
	Rounds       uint64
	RoundBytes   uint64
	CreditRounds uint64
	Batches      uint64
	GymIDs       []string
}

type dailyAdmission struct {
	inputs poolcapacity.DailyInputs
	gyms   []string // owned, sorted, immutable after attachment
}

func (d *dailyAdmission) member(gym string) bool {
	i := sort.SearchStrings(d.gyms, gym)
	return i < len(d.gyms) && d.gyms[i] == gym
}

// AttachDailyOperatorAdmission selects finite daily recovery on this original
// Source2 owner only. The ordinary ceiling is derived, never caller supplied.
// Reopen requires an explicit new attachment to the same audited source.
func (p *Pool) AttachDailyOperatorAdmission(ctx context.Context, expected Config, budget DailyRecoveryBudget) (*OperatorAdmission, error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if p == nil || ctx == nil || p.auditOnly {
		return nil, ErrInvalid
	}
	var result *OperatorAdmission
	err := p.operatorGate(ctx, func(work context.Context) error {
		if p.operatorAdmission != nil || p.config != expected || expected.Version != Version2 || len(budget.GymIDs) == 0 || len(budget.GymIDs) > 256 {
			return ErrConfig
		}
		for i, gym := range budget.GymIDs {
			if !canonicalHash(gym, true) || (i > 0 && budget.GymIDs[i-1] >= gym) {
				return ErrConfig
			}
		}
		d := &dailyAdmission{inputs: poolcapacity.DailyInputs{SourceMax: p.config.MaxSubmissions, SourceBytes: p.config.MaxBytes, Rounds: budget.Rounds, RoundBytes: budget.RoundBytes, CreditRounds: budget.CreditRounds, Batches: budget.Batches, Gyms: uint64(len(budget.GymIDs))}, gyms: append([]string(nil), budget.GymIDs...)}
		maximum, _, envelopeErr := poolcapacity.DailyEnvelope(d.inputs)
		if envelopeErr == poolcapacity.ErrInvalid {
			return ErrConfig
		}
		// Even insufficient envelope headroom cannot hide bad original evidence.
		a := &OperatorAdmission{pool: p, maximum: maximum, daily: d}
		_, over, err := a.dailyCensus(work)
		if err != nil {
			return err
		}
		if envelopeErr != nil {
			return ErrCapacity
		}
		a.draining = over || maximum == 0 || p.meta.Submissions >= maximum
		p.lifecycle.Lock()
		defer p.lifecycle.Unlock()
		if p.closing {
			return ErrClosed
		}
		if err := work.Err(); err != nil {
			return err
		}
		result, p.operatorAdmission = a, a
		return nil
	})
	// The original nonnil paused handle survives a cancellation that wins after
	// publication, so callers can join it instead of losing cleanup ownership.
	return result, err
}

func (a *OperatorAdmission) dailyWith(ctx context.Context, fn func(context.Context) error) error {
	if a == nil || a.pool == nil || a.daily == nil {
		return ErrInvalid
	}
	return a.pool.operatorGate(ctx, func(work context.Context) error {
		if a.pool.operatorAdmission != a {
			return ErrInvalid
		}
		return fn(work)
	})
}

func (a *OperatorAdmission) enableDaily(ctx context.Context) error {
	return a.dailyWith(ctx, func(work context.Context) error {
		_, over, err := a.dailyCensus(work)
		if err != nil {
			return err
		}
		if over || a.draining || a.pool.meta.Submissions >= a.maximum {
			a.draining = true
			return ErrCapacity
		}
		a.enabled = true
		return nil
	})
}

func (a *OperatorAdmission) dailyStatus(ctx context.Context) (OperatorAdmissionStatus, error) {
	var out OperatorAdmissionStatus
	err := a.dailyWith(ctx, func(work context.Context) error {
		blocks, over, err := a.dailyCensus(work)
		if err != nil {
			return err
		}
		m := a.pool.meta
		mode := "paused"
		if a.enabled {
			mode = "enabled"
		}
		if over || a.draining || m.Submissions >= a.maximum {
			a.draining, mode = true, "draining"
		}
		out = OperatorAdmissionStatus{Maximum: a.maximum, Submissions: m.Submissions, Accepted: m.Accepted, Works: m.Works, Bytes: m.Bytes, Mode: mode, OverBound: over, CanonicalRecovery: true, RecoveryBlocks: blocks}
		return nil
	})
	return out, err
}

func (p *Pool) dailyRequestMember(gym string) error {
	if a := p.operatorAdmission; a != nil && a.daily != nil && !a.daily.member(gym) {
		return ErrUnauthorized
	}
	return nil
}

// Called under the original pool gate before issuer effects, and again with
// actual encoded work bytes under the original account lease before saveWork.
// This scalar check performs no external call or nested owner acquisition.
func (p *Pool) dailyWorkHeadroom(workBytes uint64) error {
	a := p.operatorAdmission
	if a == nil || a.daily == nil {
		return nil
	}
	remaining := uint64(1) // the reserved unseen canonical receipt
	if a.maximum > p.meta.Submissions {
		remaining += a.maximum - p.meta.Submissions
	}
	if p.meta.Bytes > p.config.MaxBytes {
		return ErrCorrupt
	}
	free, receipts := p.config.MaxBytes-p.meta.Bytes, remaining*maxSubmissionSize
	if workBytes > free || receipts > free-workBytes {
		return ErrCapacity
	}
	return nil
}

func (a *OperatorAdmission) dailyRecoveryBound(ctx context.Context) error {
	blocks, over, err := a.dailyCensus(ctx)
	if err != nil {
		return err
	}
	if over {
		return ErrCapacity
	}
	p := a.pool
	_, err = poolcapacity.DailyRecovery(a.daily.inputs, p.meta.Submissions+1, blocks+1)
	if err == poolcapacity.ErrInvalid {
		return ErrCorrupt
	}
	if err != nil || maxSubmissionSize > p.config.MaxBytes-p.meta.Bytes {
		return ErrCapacity
	}
	return ctx.Err()
}
