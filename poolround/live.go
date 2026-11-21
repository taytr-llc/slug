// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/taytr-llc/slug/wire/v2"
)

// Live selects bounded touched-record reads on one fully audited Store. It
// caches no chain observation or financial authority. Existing full-audit APIs
// retain their complete source/namespace checks. Keep the original Store open;
// this handle owns no files or goroutines and cannot be transferred to another
// Store or used after its verification epoch is revoked.
type Live struct {
	store     *Store
	directory *liveDirectory
}

type liveRecord struct {
	sequence uint64
	digest   [sha256.Size]byte
}
type liveDirectory struct {
	handle     *Live
	meta       metadata
	metaDigest [sha256.Size]byte
	bySequence map[uint64]string
	byHash     map[string]liveRecord
}
type liveUpdate struct {
	hash   string
	record liveRecord
}
type liveDelta struct {
	directory *liveDirectory
	before    metadata
	updates   []liveUpdate
}

// BeginLive explicitly selects the original owner's touched-key contract.
// First selection (or selection after revocation) fully audits before creating
// a new epoch. Canceling this operation's context does not close the owner.
func (s *Store) BeginLive(ctx context.Context) (*Live, error) {
	end, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	if s.auditOnly {
		return nil, ErrInvalid
	}
	if s.live != nil {
		if err := s.checkLiveMetadataLocked(ctx, s.live); err != nil {
			return nil, err
		}
		if err := s.checkLiveSourceLocked(ctx); err != nil {
			return nil, err
		}
		return s.live.handle, nil
	}
	d, err := s.buildLiveLocked(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h := &Live{store: s, directory: d}
	d.handle = h
	s.live = d
	return h, nil
}

func (s *Store) buildLiveLocked(ctx context.Context) (*liveDirectory, error) {
	if err := s.verifyStream(ctx); err != nil {
		return nil, err
	}
	d := &liveDirectory{meta: s.meta, bySequence: make(map[uint64]string), byHash: make(map[string]liveRecord)}
	raw, err := s.db.Get([]byte("meta"), nil)
	var m metadata
	if err != nil || decode(raw, maxMetadataSize, &m) != nil || m != s.meta {
		return nil, ErrCorrupt
	}
	d.metaDigest = sha256.Sum256(raw)
	for n := uint64(1); n <= s.meta.Rounds; n++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		index, err := s.db.Get(indexKey(n), nil)
		if err != nil || !canonicalHash(string(index), false) {
			return nil, ErrCorrupt
		}
		raw, err := s.db.Get(roundKey(string(index)), nil)
		var record Record
		if err != nil || decode(raw, maxRecordSize, &record) != nil || !validRecord(record) || record.Round.Sequence != n || record.Round.BlockHash != string(index) {
			return nil, ErrCorrupt
		}
		if _, exists := d.byHash[string(index)]; exists {
			return nil, ErrCorrupt
		}
		d.bySequence[n] = string(index)
		d.byHash[string(index)] = liveRecord{sequence: n, digest: sha256.Sum256(raw)}
	}
	return d, ctx.Err()
}

func (s *Store) invalidateLiveLocked() { s.live = nil }
func (s *Store) revokeLiveOnErrorLocked(err error) {
	if errors.Is(err, ErrCorrupt) || errors.Is(err, ErrSourceChanged) || errors.Is(err, ErrPoisoned) {
		s.invalidateLiveLocked()
	}
}
func (s *Store) checkLiveMetadataLocked(ctx context.Context, d *liveDirectory) (err error) {
	defer func() { s.revokeLiveOnErrorLocked(err) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if d == nil || s.live != d {
		return ErrLiveStale
	}
	if d.meta != s.meta || !s.validMetadata(s.meta) {
		return ErrCorrupt
	}
	raw, err := s.db.Get([]byte("meta"), nil)
	if err != nil || sha256.Sum256(raw) != d.metaDigest {
		return ErrCorrupt
	}
	return nil
}
func (s *Store) checkLiveSourceLocked(ctx context.Context) (err error) {
	defer func() { s.revokeLiveOnErrorLocked(err) }()
	page, err := s.source.AccountingPage(ctx, s.meta.Through, s.meta.Through, 1)
	if err != nil {
		return sourceError(err)
	}
	if page.Config != s.sourceConfig || (s.meta.Through != 0 && (page.Through != s.meta.Through || len(page.Entries) != 0)) || len(page.Entries) > 1 {
		return ErrSourceChanged
	}
	// Through=0 is an existing source convention for today's tail. Its at most
	// one newer entry and its returned tail are liveness only and are discarded.
	return ctx.Err()
}
func (l *Live) begin(ctx context.Context) (*Store, func(), error) {
	if wire.TestPoW {
		return nil, nil, ErrProfile
	}
	if l == nil || l.store == nil || l.directory == nil {
		return nil, nil, ErrLiveStale
	}
	s := l.store
	end, err := s.begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	if s.auditOnly || s.live != l.directory || l.directory.handle != l {
		end()
		return nil, nil, ErrLiveStale
	}
	if err := s.checkLiveMetadataLocked(ctx, l.directory); err != nil {
		end()
		return nil, nil, err
	}
	return s, end, nil
}

// checkLiveRecordBytesLocked also runs inside full audits. A full audit must
// not silently replace a remembered record with different well-formed bytes.
func (s *Store) checkLiveRecordBytesLocked(hash string, sequence uint64, raw []byte) error {
	if s.live == nil {
		return nil
	}
	known, ok := s.live.byHash[hash]
	if !ok || known.sequence != sequence || s.live.bySequence[sequence] != hash || known.digest != sha256.Sum256(raw) {
		s.invalidateLiveLocked()
		return ErrCorrupt
	}
	return nil
}
func (s *Store) readLiveRecordLocked(ctx context.Context, hash string) (record Record, err error) {
	defer func() { s.revokeLiveOnErrorLocked(err) }()
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if s.live == nil {
		return Record{}, ErrLiveStale
	}
	known, ok := s.live.byHash[hash]
	if !ok {
		return Record{}, ErrNotFound
	}
	index, err := s.db.Get(indexKey(known.sequence), nil)
	if err != nil || string(index) != hash || s.live.bySequence[known.sequence] != hash {
		return Record{}, ErrCorrupt
	}
	raw, err := s.db.Get(roundKey(hash), nil)
	if err != nil || s.checkLiveRecordBytesLocked(hash, known.sequence, raw) != nil || decode(raw, maxRecordSize, &record) != nil || !validRecord(record) || record.Round.Sequence != known.sequence || record.Round.BlockHash != hash {
		return Record{}, ErrCorrupt
	}
	return record, ctx.Err()
}

// List returns at most eight original committed records. It does not consume
// an uncommitted source tail or make a node observation. Unrelated raw database
// mutations are detected by the unchanged full APIs, not promised by this call.
func (l *Live) List(ctx context.Context, after uint64) ([]Record, error) {
	s, end, err := l.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	if after > s.meta.Rounds {
		return nil, ErrInvalid
	}
	if err := s.checkLiveSourceLocked(ctx); err != nil {
		return nil, err
	}
	return s.listLiveRecordsLocked(ctx, after)
}

// The caller holds the original gate and has checked metadata/source liveness.
func (s *Store) listLiveRecordsLocked(ctx context.Context, after uint64) ([]Record, error) {
	last := after + MaxPage
	if last > s.meta.Rounds {
		last = s.meta.Rounds
	}
	rows := make([]Record, 0, last-after)
	for n := after + 1; n <= last; n++ {
		hash, ok := s.live.bySequence[n]
		if !ok {
			s.invalidateLiveLocked()
			return nil, ErrCorrupt
		}
		r, err := s.readLiveRecordLocked(ctx, hash)
		if err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

// Refresh checks the selected original source winner before and after the
// existing fresh node observation. It never caches canonicality or earnings.
func (l *Live) Refresh(ctx context.Context, hash string) (record Record, err error) {
	s, end, err := l.begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer end()
	defer func() { s.revokeLiveOnErrorLocked(err) }()
	if !canonicalHash(hash, false) {
		return Record{}, ErrInvalid
	}
	record, err = s.readLiveRecordLocked(ctx, hash)
	if err != nil {
		return Record{}, err
	}
	if err := s.checkWinner(ctx, record.Round); err != nil {
		return Record{}, err
	}
	return s.refreshRecordLocked(ctx, record)
}

func (s *Store) newLiveDeltaLocked() *liveDelta {
	if s.live == nil {
		return nil
	}
	return &liveDelta{directory: s.live, before: s.meta}
}
func (d *liveDelta) add(hash string, sequence uint64, raw []byte) {
	if d != nil {
		d.updates = append(d.updates, liveUpdate{hash: hash, record: liveRecord{sequence: sequence, digest: sha256.Sum256(raw)}})
	}
}

// checkLiveDeltaLocked is complete before the synchronous write; apply cannot
// fail afterward. Only validated original Sync/Refresh operations create deltas.
func (s *Store) checkLiveDeltaLocked(d *liveDelta, next metadata) error {
	if d == nil {
		return nil
	}
	if s.live != d.directory || d.before != s.meta || d.directory.meta != s.meta {
		return ErrLiveStale
	}
	seen := make(map[uint64]bool, len(d.updates))
	hashes := make(map[string]bool, len(d.updates))
	added := uint64(0)
	for _, u := range d.updates {
		if !canonicalHash(u.hash, false) || u.record.sequence == 0 || u.record.sequence > next.Rounds || seen[u.record.sequence] || hashes[u.hash] {
			return ErrCorrupt
		}
		seen[u.record.sequence] = true
		hashes[u.hash] = true
		old, exists := d.directory.byHash[u.hash]
		indexed, indexExists := d.directory.bySequence[u.record.sequence]
		if u.record.sequence <= s.meta.Rounds {
			if !exists || !indexExists || indexed != u.hash || old.sequence != u.record.sequence {
				return ErrCorrupt
			}
		} else {
			if exists || indexExists {
				return ErrCorrupt
			}
			added++
		}
	}
	if next.Rounds < s.meta.Rounds || added != next.Rounds-s.meta.Rounds {
		return ErrCorrupt
	}
	return nil
}
func (s *Store) applyLiveDeltaLocked(d *liveDelta, m metadata, raw []byte) {
	if d == nil {
		s.invalidateLiveLocked()
		return
	}
	for _, u := range d.updates {
		d.directory.bySequence[u.record.sequence] = u.hash
		d.directory.byHash[u.hash] = u.record
	}
	d.directory.meta, d.directory.metaDigest = m, sha256.Sum256(raw)
}
