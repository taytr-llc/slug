// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"encoding/json"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/poolshare"
)

// Sync consumes one fixed accepted tail. It closes rounds only at accepted
// winning shares and never moves members when delivery or chain status changes.
// It makes no new node observation and grants no confirmation by itself.
func (s *Store) Sync(ctx context.Context) (result Summary, err error) {
	if s != nil && s.auditOnly {
		return Summary{}, ErrInvalid
	}
	end, err := s.begin(ctx)
	if err != nil {
		return Summary{}, err
	}
	defer end()
	defer func() { s.revokeLiveOnErrorLocked(err) }()
	delta := s.newLiveDeltaLocked()
	if !s.validMetadata(s.meta) {
		return Summary{}, ErrCorrupt
	}
	if s.meta.Through == 0 && s.meta.Prefix != initialPrefix(s.sourceConfig) {
		return Summary{}, ErrSourceChanged
	}
	builder := newRoundDerivation(s.sourceConfig)
	batch := new(leveldb.Batch)
	next := s.meta
	var oldRounds, oldBytes uint64
	capacity := false
	through, prefix, err := s.streamAccepted(ctx, nil, func(entry poolshare.AccountingEntry, prefix string) error {
		if entry.Share.AcceptedSequence == s.meta.Through && prefix != s.meta.Prefix {
			return ErrSourceChanged
		}
		round, err := builder.add(entry)
		if builder.overWindow {
			capacity = true
		}
		if err != nil || round == nil {
			return err
		}
		if round.LastAccepted <= s.meta.Through {
			budget, err := s.verifyStreamRound(*round)
			if err != nil {
				return err
			}
			if oldBytes > s.meta.Bytes || budget > s.meta.Bytes-oldBytes {
				return ErrCorrupt
			}
			oldRounds++
			oldBytes += budget
			return nil
		}
		if capacity || round.Sequence > s.limits.MaxRounds {
			capacity = true
			return nil
		}
		budget, err := roundBudget(*round)
		if err != nil {
			if err == ErrCapacity {
				capacity = true
				return nil
			}
			return err
		}
		if budget > s.limits.MaxBytes-next.Bytes {
			capacity = true
			return nil
		}
		raw, err := recordBytes(Record{Round: *round})
		if err != nil {
			return err
		}
		for _, key := range [][]byte{roundKey(round.BlockHash), indexKey(round.Sequence)} {
			exists, err := s.db.Has(key, nil)
			if err != nil || exists {
				return ErrCorrupt
			}
		}
		delta.add(round.BlockHash, round.Sequence, raw)
		batch.Put(roundKey(round.BlockHash), raw)
		batch.Put(indexKey(round.Sequence), []byte(round.BlockHash))
		next.Bytes += budget
		return nil
	})
	if err != nil {
		return Summary{}, err
	}
	if through < s.meta.Through || oldRounds != s.meta.Rounds {
		return Summary{}, ErrSourceChanged
	}
	if oldBytes != s.meta.Bytes {
		return Summary{}, ErrCorrupt
	}
	if err := s.verifyStreamKeys(ctx); err != nil {
		return Summary{}, err
	}
	// Capacity never masks corruption or an incomplete source read. All staged
	// additions are discarded together when the complete new tail cannot fit.
	if capacity {
		return Summary{}, ErrCapacity
	}
	result = Summary{Through: through, Rounds: builder.rounds, PendingShares: through - (builder.first - 1)}
	if through == s.meta.Through {
		return result, ctx.Err()
	}
	next.Through, next.Prefix, next.Rounds = through, prefix, builder.rounds
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}
	if err := s.commitWithLiveLocked(batch, next, delta); err != nil {
		return Summary{}, err
	}
	return result, nil
}

func find(records []Record, hash string) (Record, error) {
	if !canonicalHash(hash, false) {
		return Record{}, ErrInvalid
	}
	for _, record := range records {
		if record.Round.BlockHash == hash {
			return record, nil
		}
	}
	return Record{}, ErrNotFound
}

// Get returns stored observations and immutable allocations. It does not ask
// the node whether an old observation still describes the current branch.
func (s *Store) Get(ctx context.Context, blockHash string) (Record, error) {
	end, err := s.begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer end()
	records, err := s.verifyStored(ctx)
	if err != nil {
		return Record{}, err
	}
	record, err := find(records, blockHash)
	if err != nil {
		return Record{}, err
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	return record, nil
}

// List pages historical records by immutable round sequence. Return values own
// their slices and observations; mutation cannot change subsequent reads.
func (s *Store) List(ctx context.Context, afterRound uint64, limit int) ([]Record, error) {
	end, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	if limit < 1 || limit > MaxPage || afterRound > s.meta.Rounds {
		return nil, ErrInvalid
	}
	records, err := s.verifyStored(ctx)
	if err != nil {
		return nil, err
	}
	last := afterRound + uint64(limit)
	if last > s.meta.Rounds {
		last = s.meta.Rounds
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]Record{}, records[afterRound:last]...), nil
}

func sameEntry(a, b poolshare.AccountingEntry) bool {
	x, e := json.Marshal(a)
	if e != nil {
		return false
	}
	y, e := json.Marshal(b)
	return e == nil && string(x) == string(y)
}

// checkWinner is a final source liveness/integrity guard after the node call.
// The full consumed prefix was already checked; the concrete source promises
// immutable acceptance and remains exclusively owned throughout its lifetime.
func (s *Store) checkWinner(ctx context.Context, r Round) error {
	page, err := s.source.AccountingPage(ctx, r.LastAccepted-1, r.LastAccepted, 1)
	if err != nil {
		return sourceError(err)
	}
	if page.Config != s.sourceConfig || page.Through != r.LastAccepted || len(page.Entries) != 1 || !sameEntry(page.Entries[0], r.Winner) {
		return ErrSourceChanged
	}
	return ctx.Err()
}

// Refresh records one fresh canonical observation. A pre-confirmation orphan
// has no earnings; returning that exact block resumes this same fixed round.
// The first sixty-descendant lock is never removed by later branch conflicts.
// A transport error leaves all prior observations and confirmation untouched.
func (s *Store) Refresh(ctx context.Context, blockHash string) (result Record, err error) {
	if s != nil && s.auditOnly {
		return Record{}, ErrInvalid
	}
	end, err := s.begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer end()
	defer func() { s.revokeLiveOnErrorLocked(err) }()
	records, err := s.verifyStored(ctx)
	if err != nil {
		return Record{}, err
	}
	record, err := find(records, blockHash)
	if err != nil {
		return Record{}, err
	}
	return s.refreshRecordLocked(ctx, record)
}

func (s *Store) refreshRecordLocked(ctx context.Context, record Record) (Record, error) {
	if record.Round.Winner.Proof == nil {
		return Record{}, ErrCorrupt
	}
	winner := cloneEntry(record.Round.Winner)
	observation, err := s.node.ObservePoolBlock(ctx, winner.Job.Context, *winner.Proof, record.Round.BlockHash)
	if err != nil {
		if ctx.Err() != nil {
			return Record{}, ctx.Err()
		}
		return Record{}, ErrNode
	}
	if !validObservation(observation, record.Round) {
		return Record{}, ErrNode
	}
	if err := s.checkWinner(ctx, record.Round); err != nil {
		return Record{}, err
	}
	next := record
	next.Observation = &observation
	if next.Confirmation == nil && observation.Canonical && observation.Descendants >= 60 {
		confirmation := observation
		next.Confirmation = &confirmation
	}
	raw, err := recordBytes(next)
	if err != nil {
		return Record{}, err
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	delta := s.newLiveDeltaLocked()
	if delta != nil {
		delta.add(record.Round.BlockHash, record.Round.Sequence, raw)
	}
	batch := new(leveldb.Batch)
	batch.Put(roundKey(record.Round.BlockHash), raw)
	if err := s.commitWithLiveLocked(batch, s.meta, delta); err != nil {
		return Record{}, err
	}
	return next, nil
}
