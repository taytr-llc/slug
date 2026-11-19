// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"encoding/json"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/poolshare"
)

func summary(through uint64, rounds []Round) Summary {
	last := uint64(0)
	if len(rounds) > 0 {
		last = rounds[len(rounds)-1].LastAccepted
	}
	return Summary{Through: through, Rounds: uint64(len(rounds)), PendingShares: through - last}
}

// Sync consumes one fixed accepted tail. It closes rounds only at accepted
// winning shares and never moves members when delivery or chain status changes.
// It makes no new node observation and grants no confirmation by itself.
func (s *Store) Sync(ctx context.Context) (Summary, error) {
	end, err := s.begin(ctx)
	if err != nil {
		return Summary{}, err
	}
	defer end()
	entries, prefix, err := s.snapshot(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	if uint64(len(entries)) < s.meta.Through {
		return Summary{}, ErrSourceChanged
	}
	oldPrefix := initialPrefix(s.sourceConfig)
	for _, entry := range entries[:s.meta.Through] {
		oldPrefix, err = extendPrefix(oldPrefix, entry)
		if err != nil {
			return Summary{}, err
		}
	}
	if oldPrefix != s.meta.Prefix {
		return Summary{}, ErrSourceChanged
	}
	rounds, err := derive(s.sourceConfig, entries)
	if err != nil {
		return Summary{}, err
	}
	oldRounds := 0
	for oldRounds < len(rounds) && rounds[oldRounds].LastAccepted <= s.meta.Through {
		oldRounds++
	}
	if _, err := s.stored(rounds[:oldRounds]); err != nil {
		return Summary{}, err
	}
	result := summary(uint64(len(entries)), rounds)
	if result.Through == s.meta.Through {
		return result, ctx.Err()
	}
	if result.Rounds > s.limits.MaxRounds {
		return Summary{}, ErrCapacity
	}
	batch := new(leveldb.Batch)
	next := s.meta
	for _, round := range rounds[oldRounds:] {
		budget, err := roundBudget(round)
		if err != nil {
			return Summary{}, err
		}
		if budget > s.limits.MaxBytes-next.Bytes {
			return Summary{}, ErrCapacity
		}
		record := Record{Round: round}
		raw, err := recordBytes(record)
		if err != nil {
			return Summary{}, err
		}
		for _, key := range [][]byte{roundKey(round.BlockHash), indexKey(round.Sequence)} {
			exists, e := s.db.Has(key, nil)
			if e != nil || exists {
				return Summary{}, ErrCorrupt
			}
		}
		batch.Put(roundKey(round.BlockHash), raw)
		batch.Put(indexKey(round.Sequence), []byte(round.BlockHash))
		next.Bytes += budget
	}
	next.Through, next.Prefix, next.Rounds = result.Through, prefix, result.Rounds
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}
	if err := s.commit(batch, next); err != nil {
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
func (s *Store) Refresh(ctx context.Context, blockHash string) (Record, error) {
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
	batch := new(leveldb.Batch)
	batch.Put(roundKey(blockHash), raw)
	if err := s.commit(batch, s.meta); err != nil {
		return Record{}, err
	}
	return next, nil
}
