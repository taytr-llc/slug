// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"

	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/internal/poolcapacity"
)

// dailyCensus holds the original pool gate throughout. It authenticates all
// retained jobs/rows/indexes before classifying membership or capacity. Issuer-
// only unused jobs carry no Pool liability and are not silently enumerated.
// No parsed metadata, financial grant or verification flag is adopted on error.
func (a *OperatorAdmission) dailyCensus(ctx context.Context) (blocks uint64, over bool, err error) {
	p := a.pool
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	raw, err := p.db.Get([]byte("meta"), nil)
	var candidate metadata
	if err != nil || p.decodeStoredMetadata(raw, &candidate) != nil || candidate != p.meta || candidate.Application != application || candidate.Version != Version2 || candidate.Config != p.config || candidate.Works > p.config.MaxWorks || candidate.Submissions > p.config.MaxSubmissions || candidate.Accepted > candidate.Submissions || candidate.Bytes > p.config.MaxBytes || candidate.LastClockMS < 0 {
		return 0, false, ErrCorrupt
	}
	info, err := p.originalInfo(ctx)
	if err != nil {
		return 0, false, p.sourceReadError(ctx, err)
	}
	if info.Pin != p.config.AuthorityPin || info.ReadOnly || info.Jobs < candidate.Works {
		return 0, false, ErrCorrupt
	}
	if err := p.auditRowsContext(ctx); err != nil {
		if err == ErrNotFound || err == ErrInvalid {
			err = ErrCorrupt
		}
		return 0, false, err
	}
	// All original source proofs are valid before this membership pass, so an
	// early nonmember cannot mask a malformed later row or an unknown namespace.
	nonmember := false
	it := p.db.NewIterator(util.BytesPrefix([]byte("w")), nil)
	for it.Next() {
		if err := ctx.Err(); err != nil {
			it.Release()
			return 0, false, err
		}
		var w savedWork
		if decode(it.Value(), maxWorkSize, &w) != nil || !canonicalHash(w.Job.GymID, true) {
			it.Release()
			return 0, false, ErrCorrupt
		}
		nonmember = nonmember || !a.daily.member(w.Job.GymID)
	}
	iterErr := it.Error()
	it.Release()
	if iterErr != nil {
		return 0, false, ErrCorrupt
	}
	for sequence := uint64(1); sequence <= candidate.Submissions; sequence++ {
		if err := ctx.Err(); err != nil {
			return 0, false, err
		}
		r, err := p.dailyIndexedRow('q', sequence)
		if err != nil {
			return 0, false, err
		}
		if !canonicalHash(r.GymID, true) {
			return 0, false, ErrCorrupt
		}
		nonmember = nonmember || !a.daily.member(r.GymID)
		if r.Block {
			blocks++
		}
		if sequence > a.maximum && (!r.Block || r.AcceptedSequence == 0 || r.BlockState != "canonical-observed" || (r.Acceptance != "canonical-winning-block" && r.Acceptance != "canonical-late-winner")) {
			over = true
		}
	}
	if p.issueRecovery != nil {
		requests := p.db.NewIterator(util.BytesPrefix([]byte("issue/r/")), nil)
		for requests.Next() {
			if err := ctx.Err(); err != nil {
				requests.Release()
				return 0, false, err
			}
			var r issueRequest
			if decode(requests.Value(), 4096, &r) != nil {
				requests.Release()
				return 0, false, ErrCorrupt
			}
			nonmember = nonmember || !a.daily.member(r.Intent.GymID)
		}
		requestErr := requests.Error()
		requests.Release()
		if requestErr != nil {
			return 0, false, ErrCorrupt
		}
	}
	var window uint64
	for accepted := uint64(1); accepted <= candidate.Accepted; accepted++ {
		if err := ctx.Err(); err != nil {
			return 0, false, err
		}
		r, err := p.dailyIndexedRow('a', accepted)
		if err != nil {
			return 0, false, err
		}
		window++
		if window > 4096 || (accepted > a.maximum && !r.Block) {
			over = true
		}
		if r.Block {
			window = 0
		}
	}
	if nonmember {
		return 0, false, ErrConfig
	}
	_, err = poolcapacity.DailyRecovery(a.daily.inputs, candidate.Submissions, blocks)
	if err == poolcapacity.ErrInvalid {
		return 0, false, ErrCorrupt
	}
	if err != nil && err != poolcapacity.ErrCapacity {
		return 0, false, err
	}
	return blocks, over || err == poolcapacity.ErrCapacity, ctx.Err()
}

// The full audit above proves the key bijections and original row proof. This
// bounded ordered pass checks the accepted window without retaining all rows.
func (p *Pool) dailyIndexedRow(prefix byte, sequence uint64) (Submission, error) {
	key, err := p.db.Get(seqKey(prefix, sequence), nil)
	if err != nil || !canonicalHash(string(key), false) {
		return Submission{}, ErrCorrupt
	}
	raw, err := p.db.Get(recordKey(string(key)), nil)
	var r Submission
	if err != nil || decode(raw, maxSubmissionSize, &r) != nil || r.DuplicateKey != string(key) || (prefix == 'q' && r.Sequence != sequence) || (prefix == 'a' && r.AcceptedSequence != sequence) {
		return Submission{}, ErrCorrupt
	}
	return r, nil
}
