// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"

	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/internal/poolcapacity"
	"github.com/taytr-llc/slug/prospectwork"
)

// RecoveryBudget is trusted composition configuration, not a miner-supplied
// allowance. The operator binds these to its original concrete round and
// settlement stores before exposing services. No limit or source is changed.
type RecoveryBudget struct {
	Rounds       uint64
	RoundBytes   uint64
	CreditRounds uint64
	Batches      uint64
	Gyms         uint64
}

func (a *OperatorAdmission) recoveryInputs(b RecoveryBudget) poolcapacity.Inputs {
	return poolcapacity.Inputs{SourceMax: a.pool.config.MaxSubmissions, Rounds: b.Rounds, RoundBytes: b.RoundBytes, CreditRounds: b.CreditRounds, Batches: b.Batches, Gyms: b.Gyms}
}

// EnableCanonicalRecovery retains the ordinary intake ceiling. After drain,
// only an exact canonical winner from original saved work may use the separate
// all-history capacity proof. It never broadcasts an unseen block or admits a
// fighter. Reopen must attach again after auditing the same original stores.
// Physical receipt/byte limits still apply; there is no unbounded continuation.
func (a *OperatorAdmission) EnableCanonicalRecovery(ctx context.Context, budget RecoveryBudget) error {
	return a.with(ctx, func() error {
		if a.recovery != nil {
			return ErrConfig
		}
		bound, err := poolcapacity.Envelope(a.recoveryInputs(budget))
		if err != nil || a.maximum > bound.Submissions {
			return ErrConfig
		}
		if err := a.pool.auditOperatorCounters(ctx); err != nil {
			return err
		}
		// An oversized old source remains held. Attaching cannot promise that
		// its existing obligations fit; each new append proves the whole set.
		a.recovery = &budget
		return nil
	})
}

// recoveryRows runs under the original pool gate. Create/Open already verified
// every proof and index; this bounded scan recounts the immutable block flags.
// Pending, rejected, orphaned and alias block rows all consume the bound. Only
// block rows can gain acceptance later; an unaccepted ordinary row is corrupt.
func (p *Pool) recoveryRows(ctx context.Context) (uint64, map[string]bool, error) {
	gyms := make(map[string]bool)
	var rows, blocks uint64
	it := p.db.NewIterator(util.BytesPrefix([]byte("s")), nil)
	defer it.Release()
	for it.Next() {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}
		var r Submission
		if len(it.Key()) != 65 || decode(it.Value(), maxSubmissionSize, &r) != nil || string(it.Key()[1:]) != r.DuplicateKey || r.Sequence == 0 || r.Sequence > p.meta.Submissions || (!r.Block && r.AcceptedSequence == 0) {
			return 0, nil, ErrCorrupt
		}
		rows++
		gyms[r.GymID] = true
		if !canonicalHash(r.GymID, false) || len(gyms) > 256 {
			return 0, nil, ErrCorrupt
		}
		if r.Block {
			blocks++
		}
	}
	if it.Error() != nil || rows != p.meta.Submissions {
		return 0, nil, ErrCorrupt
	}
	return blocks, gyms, nil
}

func (a *OperatorAdmission) recoveryBound(ctx context.Context, additionalGym string) (poolcapacity.Reservation, uint64, error) {
	if a.recovery == nil {
		return poolcapacity.Reservation{}, 0, ErrCapacity
	}
	blocks, gyms, err := a.pool.recoveryRows(ctx)
	if err != nil {
		return poolcapacity.Reservation{}, 0, err
	}
	n, r := a.pool.meta.Submissions, blocks
	if additionalGym != "" {
		n++
		r++
		gyms[additionalGym] = true
	}
	// V1 balances cap the union of configured destinations and claim gyms at
	// 256. Counting overlaps twice is conservative; unseen work is not a claim.
	if uint64(len(gyms))+a.recovery.Gyms > 256 {
		return poolcapacity.Reservation{}, blocks, ErrCapacity
	}
	bound, err := poolcapacity.Recovery(a.recoveryInputs(*a.recovery), n, r)
	if err != nil {
		return poolcapacity.Reservation{}, blocks, ErrCapacity
	}
	return bound, blocks, nil
}

func (p *Pool) recoverCanonical(ctx context.Context, w savedWork, check prospectwork.ShareCheck, received int64, session *poolSession) (Submission, error) {
	a := p.operatorAdmission
	if a == nil || a.recovery == nil || !check.Block || check.Candidate == nil {
		return Submission{}, ErrCapacity
	}
	if _, _, err := a.recoveryBound(ctx, w.Job.GymID); err != nil {
		return Submission{}, err
	}
	if err := session.check(); err != nil {
		return Submission{}, err
	}
	proof := check.Candidate.Proof
	if proof.MerkleBranch != nil {
		proof.MerkleBranch = make([]string, len(check.Candidate.Proof.MerkleBranch))
		copy(proof.MerkleBranch, check.Candidate.Proof.MerkleBranch)
	}
	if err := p.node.CanonicalBlock(ctx, w.Job.Context, proof, check.WorkHash); err != nil {
		if ctx.Err() != nil {
			return Submission{}, ctx.Err()
		}
		return Submission{}, ErrNode
	}
	// Acceptance and the nonce/sequence indexes synchronize together. A lost
	// acknowledgement is recovered by the existing original-identity branch;
	// no second row, acceptance, broadcast or financial reservation is created.
	r := Submission{Version: Version1, Sequence: p.meta.Submissions + 1, AcceptedSequence: p.meta.Accepted + 1, JobID: w.Job.ID, GymID: w.Job.GymID, Nonce: check.Nonce, PoolTarget: p.config.PoolTarget, Header: check.Header, WorkHash: check.WorkHash, DuplicateKey: check.DuplicateKey, Pool: check.Pool, Prospect: check.Prospect, Block: true, BlockHeight: w.Job.Context.ParentHeight + 1, ReceivedAtMS: received, Acceptance: "canonical-winning-block", BlockState: "canonical-observed"}
	return p.sessionSubmission(session, w.Job, r, nil)
}
