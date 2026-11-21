// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"encoding/binary"

	"github.com/taytr-llc/slug/poolshare"
)

// verifyStream authenticates the complete consumed prefix without retaining
// historical jobs/proofs or completed rounds. All V1 duplicate guards, bounds,
// prefix bytes and per-round arithmetic remain in force. The caller holds the
// original store gate, so rows and observations cannot change during the audit.
func (s *Store) verifyStream(ctx context.Context) (err error) {
	defer func() { s.revokeLiveOnErrorLocked(err) }()
	if !s.validMetadata(s.meta) {
		return ErrCorrupt
	}
	builder := newRoundDerivation(s.sourceConfig)
	var size uint64
	_, prefix, err := s.streamAccepted(ctx, &s.meta.Through, func(entry poolshare.AccountingEntry, _ string) error {
		round, err := builder.add(entry)
		if err != nil {
			return err
		}
		if round != nil {
			budget, err := s.verifyStreamRound(*round)
			if err != nil {
				return err
			}
			if size > s.meta.Bytes || budget > s.meta.Bytes-size {
				return ErrCorrupt
			}
			size += budget
		}
		return nil
	})
	if err != nil {
		return err
	}
	if prefix != s.meta.Prefix || builder.rounds != s.meta.Rounds {
		return ErrSourceChanged
	}
	if builder.overWindow {
		return ErrCorrupt
	}
	if size != s.meta.Bytes {
		return ErrCorrupt
	}
	return s.verifyStreamKeys(ctx)
}

// streamAccepted consumes one fixed, concrete source tail with bounded pages.
// A nil limit captures today's tail; a nonnil zero is the empty consumed prefix.
// The callback receives the exact prefix after this entry. It must not persist
// anything until this whole read and the caller's journal audit have succeeded.
func (s *Store) streamAccepted(ctx context.Context, limit *uint64, visit func(poolshare.AccountingEntry, string) error) (uint64, string, error) {
	requested := uint64(0)
	if limit != nil {
		requested = *limit
	}
	page, err := s.source.AccountingPage(ctx, 0, requested, poolshare.MaxAccountingPage)
	if err != nil {
		return 0, "", sourceError(err)
	}
	if page.Config != s.sourceConfig || page.Through > s.sourceConfig.MaxSubmissions || requested != 0 && page.Through != requested {
		return 0, "", ErrSourceChanged
	}
	through := page.Through
	if limit != nil {
		through = *limit
	}
	prefix := initialPrefix(s.sourceConfig)
	var consumed uint64
	for consumed < through {
		if err := ctx.Err(); err != nil {
			return 0, "", err
		}
		if page.Config != s.sourceConfig || page.Through != through || len(page.Entries) == 0 || len(page.Entries) > poolshare.MaxAccountingPage {
			return 0, "", ErrSourceChanged
		}
		for _, entry := range page.Entries {
			if consumed >= through || entry.Share.AcceptedSequence != consumed+1 {
				return 0, "", ErrSourceChanged
			}
			prefix, err = extendPrefix(prefix, entry)
			if err != nil {
				return 0, "", err
			}
			if err := visit(entry, prefix); err != nil {
				return 0, "", err
			}
			consumed++
		}
		if consumed < through {
			page, err = s.source.AccountingPage(ctx, consumed, through, poolshare.MaxAccountingPage)
			if err != nil {
				return 0, "", sourceError(err)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, "", err
	}
	return through, prefix, nil
}

func (s *Store) verifyStreamRound(expected Round) (uint64, error) {
	index, err := s.db.Get(indexKey(expected.Sequence), nil)
	if err != nil || string(index) != expected.BlockHash {
		return 0, ErrCorrupt
	}
	raw, err := s.db.Get(roundKey(expected.BlockHash), nil)
	var record Record
	if err != nil || decode(raw, maxRecordSize, &record) != nil || !sameRound(record.Round, expected) || !validRecord(record) {
		return 0, ErrCorrupt
	}
	if err := s.checkLiveRecordBytesLocked(expected.BlockHash, expected.Sequence, raw); err != nil {
		return 0, err
	}
	budget, err := roundBudget(expected)
	if err != nil || uint64(len(raw)) > budget {
		return 0, ErrCorrupt
	}
	return budget, nil
}

// Every expected key/value was verified above. Unique database keys plus exact
// namespace counts prove there are no extra rows or indexes. This final pass
// must finish before a settlement visitor sees even the first page.
func (s *Store) verifyStreamKeys(ctx context.Context) error {
	var rows, indexes uint64
	seenMetadata := false
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		k, v := it.Key(), it.Value()
		switch {
		case string(k) == "meta":
			var m metadata
			if decode(v, maxMetadataSize, &m) != nil || m != s.meta {
				return ErrCorrupt
			}
			seenMetadata = true
		case len(k) == 65 && k[0] == 'r':
			rows++
		case len(k) == 9 && k[0] == 'i':
			n := binary.BigEndian.Uint64(k[1:])
			if n == 0 || n > s.meta.Rounds {
				return ErrCorrupt
			}
			indexes++
		default:
			return ErrCorrupt
		}
		if rows > s.meta.Rounds || indexes > s.meta.Rounds {
			return ErrCorrupt
		}
	}
	if it.Error() != nil || !seenMetadata || rows != s.meta.Rounds || indexes != s.meta.Rounds {
		return ErrCorrupt
	}
	return ctx.Err()
}

// walkRecords reads only already-verified, gate-protected rows. Each page owns
// its nested data and is discarded here after the callback. No database handle
// or snapshot escapes the operation or needs an independent Close lifecycle.
func (s *Store) walkRecords(ctx context.Context, visit func([]Record) error) (err error) {
	defer func() { s.revokeLiveOnErrorLocked(err) }()
	if visit == nil {
		return ctx.Err()
	}
	page := make([]Record, 0, MaxPage)
	for sequence := uint64(1); sequence <= s.meta.Rounds; sequence++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		index, err := s.db.Get(indexKey(sequence), nil)
		if err != nil || !canonicalHash(string(index), false) {
			return ErrCorrupt
		}
		raw, err := s.db.Get(roundKey(string(index)), nil)
		var record Record
		if err != nil || decode(raw, maxRecordSize, &record) != nil || record.Round.Sequence != sequence || record.Round.BlockHash != string(index) || !validRecord(record) {
			return ErrCorrupt
		}
		page = append(page, record)
		if len(page) == MaxPage || sequence == s.meta.Rounds {
			if err := visit(page); err != nil {
				return err
			}
			page = make([]Record, 0, MaxPage)
		}
	}
	return ctx.Err()
}
