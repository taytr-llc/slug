// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

const MaxAccountingPage = 8

// AccountingEntry is an immutable projection of an accepted share. Share's
// mutable BlockState and BlockObservedAtMS fields are intentionally cleared.
// Proof and RewardAtoms are present only for a winning block, rebuilt from the
// pinned original job/template/payout. They grant no maturity or spendability.
type AccountingEntry struct {
	Share       Submission              `json:"share"`
	Job         prospectjob.ManifestJob `json:"job"`
	Proof       *prospect.Proof         `json:"proof"`
	RewardAtoms int64                   `json:"reward_atoms"`
}

type AccountingPage struct {
	Config  Config            `json:"config"`
	Through uint64            `json:"through"`
	Entries []AccountingEntry `json:"entries"`
}

// AccountingPage is an operator-only in-process read for the round accountant,
// not an authenticated gym endpoint. Do not expose it on the miner transport.
// A zero through selects the current acceptance tail; reuse that returned tail
// on subsequent pages for a fixed snapshot while newer shares may append.
// Pending, unaccepted block intents never appear. Every entry is reverified.
func (p *Pool) AccountingPage(ctx context.Context, after, through uint64, limit int) (AccountingPage, error) {
	if wire.TestPoW {
		return AccountingPage{}, ErrProfile
	}
	if ctx == nil || limit < 1 || limit > MaxAccountingPage {
		return AccountingPage{}, ErrInvalid
	}
	if p == nil || p.gate == nil || p.db == nil {
		return AccountingPage{}, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return AccountingPage{}, err
	}
	p.lifecycle.Lock()
	if p.closing {
		p.lifecycle.Unlock()
		return AccountingPage{}, ErrClosed
	}
	select {
	case p.gate <- struct{}{}:
	default:
		p.lifecycle.Unlock()
		return AccountingPage{}, ErrBusy
	}
	p.lifecycle.Unlock()
	defer func() { <-p.gate }()
	if p.poisoned {
		return AccountingPage{}, ErrPoisoned
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if through == 0 {
		through = p.meta.Accepted
	}
	if after > through || through > p.meta.Accepted {
		return AccountingPage{}, ErrInvalid
	}
	page := AccountingPage{Config: p.config, Through: through, Entries: make([]AccountingEntry, 0, limit)}
	end := after + uint64(limit)
	if end > through {
		end = through
	}
	for sequence := after + 1; sequence <= end; sequence++ {
		if err := ctx.Err(); err != nil {
			return AccountingPage{}, err
		}
		key, err := p.db.Get(seqKey('a', sequence), nil)
		if err != nil || !canonicalHash(string(key), false) {
			return AccountingPage{}, ErrCorrupt
		}
		raw, err := p.db.Get(recordKey(string(key)), nil)
		var share Submission
		if err != nil || decode(raw, maxSubmissionSize, &share) != nil || share.DuplicateKey != string(key) || share.AcceptedSequence != sequence {
			return AccountingPage{}, ErrCorrupt
		}
		work, prepared, err := p.loadWorkContext(ctx, share.JobID)
		if err != nil {
			return AccountingPage{}, err
		}
		checked, err := p.validateSubmission(ctx, share, work, prepared)
		if err != nil {
			return AccountingPage{}, err
		}
		share.BlockState, share.BlockObservedAtMS = "", 0
		entry := AccountingEntry{Share: share, Job: work.Job}
		if share.Block {
			if checked.Candidate == nil {
				return AccountingPage{}, ErrCorrupt
			}
			proof := checked.Candidate.Proof
			entry.Proof = &proof
			entry.RewardAtoms = checked.Candidate.CoinbaseValue
		}
		page.Entries = append(page.Entries, entry)
	}
	if err := ctx.Err(); err != nil {
		return AccountingPage{}, err
	}
	return page, nil
}
