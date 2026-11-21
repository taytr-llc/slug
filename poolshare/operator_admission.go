// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/wire/v2"
)

// OperatorAdmission belongs to one original Pool. Its ordinary submission-row
// ceiling is immutable. Canonical recovery separately proves whole-history
// headroom within the original physical and financial limits. All state is
// protected by that Pool's gate.
type OperatorAdmission struct {
	pool              *Pool
	maximum           uint64
	enabled, draining bool
	recovery          *RecoveryBudget
}

// OperatorAdmissionStatus is a local counter observation, never funding or
// settlement authority. OverBound identifies a journal outside its attached
// reservation; with canonical recovery attached this uses the separate audited
// row/block bound, never an assumption that draining removed old obligations.
type OperatorAdmissionStatus struct {
	Maximum           uint64 `json:"maximum"`
	Submissions       uint64 `json:"submissions"`
	Accepted          uint64 `json:"accepted"`
	Works             uint64 `json:"works"`
	Bytes             uint64 `json:"bytes"`
	Mode              string `json:"mode"`
	OverBound         bool   `json:"over_bound"`
	CanonicalRecovery bool   `json:"canonical_recovery"`
	RecoveryBlocks    uint64 `json:"recovery_blocks"`
}

// AttachOperatorAdmission is additive: unattached legacy pools retain their
// original limits. The original owner must attach before serving callers.
// Existing content was verified by Create/Open; this repeats the physical row
// and logical-byte counter audit under the same gate without repeating Argon.
// An existing journal above maximum attaches only for drain and exact recovery.
func (p *Pool) AttachOperatorAdmission(ctx context.Context, expected Config, maximum uint64) (*OperatorAdmission, error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if p == nil || ctx == nil || p.auditOnly || maximum > 256 || maximum > expected.MaxSubmissions {
		return nil, ErrInvalid
	}
	var result *OperatorAdmission
	err := p.operatorGate(ctx, func(work context.Context) error {
		if p.operatorAdmission != nil || p.config != expected {
			return ErrConfig
		}
		if err := p.auditOperatorCounters(work); err != nil {
			return err
		}
		result = &OperatorAdmission{pool: p, maximum: maximum, draining: maximum == 0 || p.meta.Submissions >= maximum}
		p.operatorAdmission = result
		return nil
	})
	return result, err
}

// operatorGate waits only for the one bounded original pool action. Closing
// still wins before a new controller operation may mutate state.
func (p *Pool) operatorGate(ctx context.Context, fn func(context.Context) error) error {
	if p == nil || ctx == nil || p.gate == nil || p.db == nil || p.auditOnly {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	p.lifecycle.Lock()
	closed := p.closing
	p.lifecycle.Unlock()
	if closed {
		return ErrClosed
	}
	select {
	case p.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-p.gate }()
	p.lifecycle.Lock()
	closed = p.closing
	p.lifecycle.Unlock()
	if closed {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.poisoned {
		return ErrPoisoned
	}
	if err := fn(ctx); err != nil {
		return err
	}
	return ctx.Err()
}

// operatorIntake runs inside the original action gate. Saved-row Submit,
// Lookup, Work and accounting reads never invoke it.
func (p *Pool) operatorIntake() error {
	a := p.operatorAdmission
	if a == nil {
		return nil
	}
	if a.draining || p.meta.Submissions >= a.maximum {
		a.draining = true
		return ErrCapacity
	}
	if !a.enabled {
		return ErrBusy
	}
	return nil
}

func (a *OperatorAdmission) with(ctx context.Context, fn func() error) error {
	if a == nil || a.pool == nil {
		return ErrInvalid
	}
	return a.pool.operatorGate(ctx, func(context.Context) error {
		if a.pool.operatorAdmission != a {
			return ErrInvalid
		}
		return fn()
	})
}
func (a *OperatorAdmission) Enable(ctx context.Context) error {
	return a.with(ctx, func() error {
		if a.draining || a.pool.meta.Submissions >= a.maximum {
			a.draining = true
			return ErrCapacity
		}
		a.enabled = true
		return nil
	})
}

// Drain joins the active writer. Its irreversible in-memory transition never
// deletes work and never disables exact recovery of an already saved nonce.
func (a *OperatorAdmission) Drain(ctx context.Context) error {
	return a.with(ctx, func() error { a.draining = true; return nil })
}
func (a *OperatorAdmission) Close() error { return a.Drain(context.Background()) }
func (a *OperatorAdmission) Status(ctx context.Context) (OperatorAdmissionStatus, error) {
	var out OperatorAdmissionStatus
	err := a.with(ctx, func() error {
		m := a.pool.meta
		mode := "paused"
		if a.enabled {
			mode = "enabled"
		}
		if a.draining || m.Submissions >= a.maximum {
			mode = "draining"
			a.draining = true
		}
		over := m.Submissions > a.maximum
		var blocks uint64
		if a.recovery != nil {
			_, count, err := a.recoveryBound(ctx, "")
			if err != nil && err != ErrCapacity {
				return err
			}
			blocks = count
			over = err != nil
		}
		out = OperatorAdmissionStatus{Maximum: a.maximum, Submissions: m.Submissions, Accepted: m.Accepted, Works: m.Works, Bytes: m.Bytes, Mode: mode, OverBound: over, CanonicalRecovery: a.recovery != nil, RecoveryBlocks: blocks}
		return nil
	})
	return out, err
}

func (p *Pool) auditOperatorCounters(ctx context.Context) error {
	raw, err := p.db.Get([]byte("meta"), nil)
	var meta metadata
	if err != nil || decode(raw, maxMetadataSize, &meta) != nil || meta != p.meta || meta.Config != p.config {
		return ErrCorrupt
	}
	var works, rows, nonces, sequences, accepted, size uint64
	it := p.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		k, v := it.Key(), it.Value()
		if string(k) == "meta" {
			continue
		}
		if len(k) == 0 {
			return ErrCorrupt
		}
		switch k[0] {
		case 'w':
			if len(k) != 65 || len(v) > maxWorkSize {
				return ErrCorrupt
			}
			works++
			size += uint64(len(v))
		case 's':
			if len(k) != 65 || len(v) > maxSubmissionSize {
				return ErrCorrupt
			}
			rows++
			size += maxSubmissionSize
		case 'n':
			if len(k) != 73 || len(v) != 64 {
				return ErrCorrupt
			}
			nonces++
		case 'q':
			if len(k) != 9 || len(v) != 64 {
				return ErrCorrupt
			}
			sequences++
		case 'a':
			if len(k) != 9 || len(v) != 64 {
				return ErrCorrupt
			}
			accepted++
		default:
			return ErrCorrupt
		}
		if works > meta.Works || rows > meta.Submissions || nonces > meta.Submissions || sequences > meta.Submissions || accepted > meta.Accepted || size > meta.Bytes {
			return ErrCorrupt
		}
	}
	if it.Error() != nil || works != meta.Works || rows != meta.Submissions || nonces != rows || sequences != rows || accepted != meta.Accepted || size != meta.Bytes {
		return ErrCorrupt
	}
	return ctx.Err()
}
