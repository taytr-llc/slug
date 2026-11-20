// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

type marker struct {
	Application string `json:"application"`
	Version     uint32 `json:"version"`
	Config      Config `json:"config"`
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func directory(path string) bool {
	i, err := os.Lstat(path)
	return err == nil && i.IsDir() && i.Mode()&os.ModeSymlink == 0
}

func decode(raw []byte, limit int, value any) error {
	if len(raw) == 0 || len(raw) > limit {
		return ErrCorrupt
	}
	depth, quoted, escape := 0, false, false
	for _, b := range raw {
		if quoted {
			if escape {
				escape = false
			} else if b == '\\' {
				escape = true
			} else if b == '"' {
				quoted = false
			}
			continue
		}
		switch b {
		case '"':
			quoted = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
		if depth < 0 || depth > 8 {
			return ErrCorrupt
		}
	}
	if depth != 0 || quoted {
		return ErrCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil {
		return ErrCorrupt
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return ErrCorrupt
	}
	return nil
}

func newPool(c Config, authority *registry.ManifestStore, node Node, sessions prospectservice.SessionAuthorizer) *Pool {
	return &Pool{config: c, authority: authority, node: node, sessions: sessions, gate: make(chan struct{}, 1), closeDone: make(chan struct{})}
}

// Create initializes a new journal only; an existing path is never adopted.
func Create(path string, c Config, authority *registry.ManifestStore, node Node, sessions prospectservice.SessionAuthorizer) (*Pool, error) {
	if err := validateConfig(c, authority, node, sessions); err != nil {
		return nil, err
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, ErrInvalid
	}
	raw, _ := json.Marshal(marker{application, Version1, c})
	f, err := os.OpenFile(filepath.Join(path, markerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, ErrCorrupt
	}
	_, we := f.Write(raw)
	if errors.Join(we, f.Sync(), f.Close()) != nil {
		return nil, ErrPoisoned
	}
	dbPath := filepath.Join(path, "db")
	if os.Mkdir(dbPath, 0700) != nil {
		return nil, ErrCorrupt
	}
	db, err := leveldb.OpenFile(dbPath, &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
	if err != nil {
		return nil, ErrCorrupt
	}
	p := newPool(c, authority, node, sessions)
	p.db, p.writeBatch = db, db.Write
	p.meta = metadata{Application: application, Version: Version1, Config: c}
	if err := p.commit(new(leveldb.Batch), p.meta); err != nil {
		_ = db.Close()
		return nil, err
	}
	if errors.Join(syncDir(dbPath), syncDir(path), syncDir(filepath.Dir(filepath.Clean(path)))) != nil {
		_ = db.Close()
		return nil, ErrPoisoned
	}
	return p, nil
}

// Open requires the original exact configuration and source authority. It
// verifies every retained proof/index before exposing evidence. No migration,
// repair, live node call or acceptance replay occurs during this audit.
func Open(path string, c Config, authority *registry.ManifestStore, node Node, sessions prospectservice.SessionAuthorizer) (*Pool, error) {
	return openMode(path, c, authority, node, sessions, false)
}

func openMode(path string, c Config, authority *registry.ManifestStore, node Node, sessions prospectservice.SessionAuthorizer, auditOnly bool) (*Pool, error) {
	if err := validateConfigMode(c, authority, node, sessions, auditOnly); err != nil {
		return nil, err
	}
	if !directory(path) || !directory(filepath.Join(path, "db")) {
		return nil, ErrCorrupt
	}
	name := filepath.Join(path, markerName)
	i, err := os.Lstat(name)
	if err != nil || !i.Mode().IsRegular() || i.Size() > maxMetadataSize {
		return nil, ErrCorrupt
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, ErrCorrupt
	}
	raw, re := io.ReadAll(io.LimitReader(f, maxMetadataSize+1))
	if errors.Join(re, f.Close()) != nil {
		return nil, ErrCorrupt
	}
	var m marker
	if decode(raw, maxMetadataSize, &m) != nil || m.Application != application || m.Version != Version1 || m.Config != c {
		return nil, ErrCorrupt
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, ReadOnly: auditOnly, Strict: opt.StrictAll})
	if err != nil {
		return nil, ErrCorrupt
	}
	p := newPool(c, authority, node, sessions)
	p.db, p.writeBatch, p.auditOnly = db, db.Write, auditOnly
	if err := p.audit(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return p, nil
}

func workKey(id string) []byte   { return []byte("w" + id) }
func recordKey(id string) []byte { return []byte("s" + id) }
func nonceKey(id string, nonce uint32) []byte {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], nonce)
	return []byte("n" + id + hex.EncodeToString(n[:]))
}
func seqKey(prefix byte, n uint64) []byte {
	k := make([]byte, 9)
	k[0] = prefix
	binary.BigEndian.PutUint64(k[1:], n)
	return k
}
func (p *Pool) mark(stage string) {
	if p.checkpoint != nil {
		p.checkpoint(stage)
	}
}

func (p *Pool) commit(batch *leveldb.Batch, next metadata) error {
	if p.auditOnly {
		return ErrInvalid
	}
	if p.poisoned {
		return ErrPoisoned
	}
	if next.Application != application || next.Version != Version1 || next.Config != p.config || next.Works > p.config.MaxWorks || next.Submissions > p.config.MaxSubmissions || next.Accepted > next.Submissions || next.Bytes > p.config.MaxBytes || next.LastClockMS < p.meta.LastClockMS {
		return ErrCorrupt
	}
	raw, err := json.Marshal(next)
	if err != nil || len(raw) > maxMetadataSize {
		return ErrCorrupt
	}
	batch.Put([]byte("meta"), raw)
	if p.writeBatch(batch, &opt.WriteOptions{Sync: true}) != nil {
		p.poisoned = true
		return ErrPoisoned
	}
	p.meta = next
	p.mark("commit")
	return nil
}

// clock persists high-water observations even when a later expiry/eligibility
// check rejects work, so restart cannot revive an already observed deadline.
func (p *Pool) observeClock(recheck func() error) (int64, error) {
	if recheck == nil {
		return 0, ErrUnauthorized
	}
	if err := recheck(); err != nil {
		return 0, err
	}
	now := time.Now
	if p.now != nil {
		now = p.now
	}
	ms := now().UnixMilli()
	if ms <= 0 || ms < p.clockSeen || ms < p.meta.LastClockMS {
		return 0, ErrClock
	}
	p.clockSeen = ms
	return ms, nil
}

func (p *Pool) persistClock(ms int64) error {
	if ms > p.meta.LastClockMS {
		next := p.meta
		next.LastClockMS = ms
		if err := p.commit(new(leveldb.Batch), next); err != nil {
			return err
		}
		p.mark("clock")
	}
	return nil
}

func (p *Pool) clock(recheck func() error) (int64, error) {
	ms, err := p.observeClock(recheck)
	if err != nil {
		return 0, err
	}
	if err := p.persistClock(ms); err != nil {
		return 0, err
	}
	return ms, nil
}

func (p *Pool) prepare(w savedWork) (*prospectwork.ManifestWork, error) {
	j := w.Job
	if !p.samePolicy(j.Context) || j.AuthorityID != p.config.AuthorityPin.AuthorityID || j.DescriptorID != p.config.AuthorityPin.DescriptorID || !templateBounds(w.Template) {
		return nil, ErrCorrupt
	}
	actual, err := p.authority.GetManifestJob(j.ID)
	if err != nil || actual != j {
		return nil, ErrCorrupt
	}
	prepared, err := prospectwork.PrepareManifestWork(j, w.Template, p.config.PayoutAddress, 0, p.config.PoolTarget)
	if err != nil {
		return nil, ErrCorrupt
	}
	return prepared, nil
}

func (p *Pool) loadWork(id string) (savedWork, *prospectwork.ManifestWork, error) {
	if !canonicalHash(id, false) {
		return savedWork{}, nil, ErrInvalid
	}
	raw, err := p.db.Get(workKey(id), nil)
	if errors.Is(err, leveldb.ErrNotFound) {
		return savedWork{}, nil, ErrNotFound
	}
	var w savedWork
	if err != nil || decode(raw, maxWorkSize, &w) != nil || w.Job.ID != id || w.Job.IssuedAtMS > p.meta.LastClockMS {
		return savedWork{}, nil, ErrCorrupt
	}
	prepared, err := p.prepare(w)
	return w, prepared, err
}

// saveWork runs inside the original account lease. Its caller already verified
// this exact immutable job/template with prepare outside that lease.
func (p *Pool) saveWork(w savedWork, recheck func() error) error {
	raw, err := json.Marshal(w)
	if err != nil || len(raw) > maxWorkSize {
		return ErrCapacity
	}
	if existing, err := p.db.Get(workKey(w.Job.ID), nil); err == nil {
		if !bytes.Equal(existing, raw) {
			return ErrCorrupt
		}
		return recheck()
	} else if !errors.Is(err, leveldb.ErrNotFound) {
		return ErrCorrupt
	}
	if p.meta.Works >= p.config.MaxWorks || p.meta.Submissions >= p.config.MaxSubmissions || uint64(len(raw))+maxSubmissionSize > p.config.MaxBytes-p.meta.Bytes {
		return ErrCapacity
	}
	ms, err := p.observeClock(recheck)
	if err != nil {
		return err
	}
	if ms < w.Job.IssuedAtMS {
		return ErrClock
	}
	if ms >= w.Job.ExpiresAtMS {
		if err := p.persistClock(ms); err != nil {
			return err
		}
		return ErrExpired
	}
	next := p.meta
	next.Works++
	next.Bytes += uint64(len(raw))
	next.LastClockMS = ms
	batch := new(leveldb.Batch)
	batch.Put(workKey(w.Job.ID), raw)
	if err := p.commit(batch, next); err != nil {
		return err
	}
	p.mark("work")
	return nil
}

func (p *Pool) loadSubmission(jobID string, nonce uint32) (Submission, error) {
	key, err := p.db.Get(nonceKey(jobID, nonce), nil)
	if errors.Is(err, leveldb.ErrNotFound) {
		return Submission{}, ErrNotFound
	}
	if err != nil || !canonicalHash(string(key), false) {
		return Submission{}, ErrCorrupt
	}
	raw, err := p.db.Get(recordKey(string(key)), nil)
	var r Submission
	if err != nil || decode(raw, maxSubmissionSize, &r) != nil || r.JobID != jobID || r.Nonce != nonce || r.DuplicateKey != string(key) {
		return Submission{}, ErrCorrupt
	}
	return r, nil
}

// The exact issued job was loaded and checked before entering the account lease.
// No issuer lock, normal proof, or network call belongs in this final batch.
func (p *Pool) saveSubmission(job prospectjob.ManifestJob, r Submission, previous *Submission, recheck func() error) (Submission, error) {
	if job.ID != r.JobID || job.GymID != r.GymID {
		return Submission{}, ErrCorrupt
	}
	if r.ReceivedAtMS < job.IssuedAtMS {
		return Submission{}, ErrClock
	}
	next := p.meta
	batch := new(leveldb.Batch)
	if previous == nil {
		if p.meta.Submissions >= p.config.MaxSubmissions || maxSubmissionSize > p.config.MaxBytes-p.meta.Bytes {
			return Submission{}, ErrCapacity
		}
		if r.Sequence != next.Submissions+1 {
			return Submission{}, ErrCorrupt
		}
		for _, k := range [][]byte{recordKey(r.DuplicateKey), nonceKey(r.JobID, r.Nonce), seqKey('q', r.Sequence)} {
			exists, e := p.db.Has(k, nil)
			if e != nil || exists {
				return Submission{}, ErrCorrupt
			}
		}
		next.Submissions++
		next.Bytes += maxSubmissionSize
		batch.Put(nonceKey(r.JobID, r.Nonce), []byte(r.DuplicateKey))
		batch.Put(seqKey('q', r.Sequence), []byte(r.DuplicateKey))
	} else {
		fixedOld, fixedNew := *previous, r
		fixedOld.BlockState, fixedNew.BlockState = "", ""
		fixedOld.BlockObservedAtMS, fixedNew.BlockObservedAtMS = 0, 0
		if previous.AcceptedSequence == 0 {
			fixedNew.AcceptedSequence, fixedNew.AcceptedAtMS, fixedNew.Acceptance = 0, 0, ""
		}
		if fixedOld != fixedNew || previous.BlockState == "canonical-observed" {
			return Submission{}, ErrCorrupt
		}
	}
	if r.AcceptedSequence != 0 && (previous == nil || previous.AcceptedSequence == 0) {
		if r.AcceptedSequence != next.Accepted+1 {
			return Submission{}, ErrCorrupt
		}
		exists, e := p.db.Has(seqKey('a', r.AcceptedSequence), nil)
		if e != nil || exists {
			return Submission{}, ErrCorrupt
		}
	}
	// All source/index reads and eligibility checks precede this final sample.
	// Its high water and the receipt synchronize in one batch; a separate clock
	// sync must not stall across expiry and then authorize an old timestamp.
	ms, err := p.observeClock(recheck)
	if err != nil {
		return Submission{}, err
	}
	if ms < r.ReceivedAtMS {
		return Submission{}, ErrClock
	}
	if r.AcceptedSequence != 0 && (previous == nil || previous.AcceptedSequence == 0) {
		r.AcceptedAtMS = ms
		if r.Acceptance == "current-parent" && ms >= job.ExpiresAtMS {
			if !r.Block {
				if err := p.persistClock(ms); err != nil {
					return Submission{}, err
				}
				return Submission{}, ErrExpired
			}
			r.AcceptedSequence, r.AcceptedAtMS, r.Acceptance = 0, 0, ""
		} else if r.Acceptance != "current-parent" {
			r.Acceptance = "canonical-winning-block"
			if ms >= job.ExpiresAtMS {
				r.Acceptance = "canonical-late-winner"
			}
		}
	}
	if previous != nil && r.BlockState != "pending" {
		r.BlockObservedAtMS = ms
	}
	if r.AcceptedSequence != 0 && (previous == nil || previous.AcceptedSequence == 0) {
		next.Accepted++
		batch.Put(seqKey('a', r.AcceptedSequence), []byte(r.DuplicateKey))
	}
	next.LastClockMS = ms
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > maxSubmissionSize {
		return Submission{}, ErrCorrupt
	}
	batch.Put(recordKey(r.DuplicateKey), raw)
	if err := p.commit(batch, next); err != nil {
		return Submission{}, err
	}
	if previous == nil {
		p.mark("submission")
	} else {
		p.mark("delivery")
	}
	return r, nil
}

func (p *Pool) validateSubmission(ctx context.Context, r Submission, w savedWork, prepared *prospectwork.ManifestWork) (prospectwork.ShareCheck, error) {
	check, err := prepared.CheckNonce(ctx, r.Nonce)
	if err != nil {
		return prospectwork.ShareCheck{}, err
	}
	if r.Version != Version1 || r.Sequence == 0 || r.Sequence > p.meta.Submissions || r.JobID != w.Job.ID || r.GymID != w.Job.GymID || r.PoolTarget != p.config.PoolTarget || r.Header != check.Header || r.WorkHash != check.WorkHash || r.DuplicateKey != check.DuplicateKey || r.Pool != check.Pool || r.Prospect != check.Prospect || r.Block != check.Block || (!r.Pool && !r.Block) || r.ReceivedAtMS < w.Job.IssuedAtMS || r.ReceivedAtMS > p.meta.LastClockMS {
		return prospectwork.ShareCheck{}, ErrCorrupt
	}
	if r.AcceptedSequence == 0 {
		if r.AcceptedAtMS != 0 || r.Acceptance != "" || !r.Block || r.BlockState == "canonical-observed" {
			return prospectwork.ShareCheck{}, ErrCorrupt
		}
	} else {
		if r.AcceptedSequence > p.meta.Accepted || r.AcceptedAtMS < r.ReceivedAtMS || r.AcceptedAtMS > p.meta.LastClockMS {
			return prospectwork.ShareCheck{}, ErrCorrupt
		}
		switch r.Acceptance {
		case "current-parent":
			if !r.Pool || r.AcceptedAtMS >= w.Job.ExpiresAtMS {
				return prospectwork.ShareCheck{}, ErrCorrupt
			}
		case "canonical-winning-block":
			if !r.Block || r.BlockState != "canonical-observed" || r.AcceptedAtMS >= w.Job.ExpiresAtMS {
				return prospectwork.ShareCheck{}, ErrCorrupt
			}
		case "canonical-late-winner":
			if !r.Block || r.BlockState != "canonical-observed" || r.AcceptedAtMS < w.Job.ExpiresAtMS {
				return prospectwork.ShareCheck{}, ErrCorrupt
			}
		default:
			return prospectwork.ShareCheck{}, ErrCorrupt
		}
	}
	if !r.Block {
		if r.BlockHeight != 0 || r.BlockState != "none" || r.BlockObservedAtMS != 0 {
			return prospectwork.ShareCheck{}, ErrCorrupt
		}
	} else {
		if r.BlockHeight != w.Job.Context.ParentHeight+1 {
			return prospectwork.ShareCheck{}, ErrCorrupt
		}
		switch r.BlockState {
		case "pending":
			if r.BlockObservedAtMS != 0 {
				return prospectwork.ShareCheck{}, ErrCorrupt
			}
		case "processed", "unknown", "rejected", "canonical-observed":
			if r.BlockObservedAtMS < r.ReceivedAtMS || r.BlockObservedAtMS > p.meta.LastClockMS {
				return prospectwork.ShareCheck{}, ErrCorrupt
			}
		default:
			return prospectwork.ShareCheck{}, ErrCorrupt
		}
	}
	for _, pair := range []struct{ k, v []byte }{{nonceKey(r.JobID, r.Nonce), []byte(r.DuplicateKey)}, {seqKey('q', r.Sequence), []byte(r.DuplicateKey)}} {
		v, e := p.db.Get(pair.k, nil)
		if e != nil || !bytes.Equal(v, pair.v) {
			return prospectwork.ShareCheck{}, ErrCorrupt
		}
	}
	if r.AcceptedSequence != 0 {
		v, e := p.db.Get(seqKey('a', r.AcceptedSequence), nil)
		if e != nil || string(v) != r.DuplicateKey {
			return prospectwork.ShareCheck{}, ErrCorrupt
		}
	}
	return check, nil
}

func (p *Pool) audit() error {
	raw, err := p.db.Get([]byte("meta"), nil)
	if err != nil || decode(raw, maxMetadataSize, &p.meta) != nil || p.meta.Application != application || p.meta.Version != Version1 || p.meta.Config != p.config || p.meta.Works > p.config.MaxWorks || p.meta.Submissions > p.config.MaxSubmissions || p.meta.Accepted > p.meta.Submissions || p.meta.Bytes > p.config.MaxBytes || p.meta.LastClockMS < 0 {
		return ErrCorrupt
	}
	var works, submissions, nonces, sequences, accepted, size uint64
	it := p.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		k, v := it.Key(), it.Value()
		if string(k) == "meta" {
			continue
		}
		if len(k) == 0 {
			return ErrCorrupt
		}
		switch k[0] {
		case 'w':
			if len(k) != 65 || !canonicalHash(string(k[1:]), false) {
				return ErrCorrupt
			}
			if _, _, err := p.loadWork(string(k[1:])); err != nil {
				return err
			}
			works++
			size += uint64(len(v))
		case 's':
			if len(k) != 65 {
				return ErrCorrupt
			}
			var r Submission
			if decode(v, maxSubmissionSize, &r) != nil || string(k[1:]) != r.DuplicateKey {
				return ErrCorrupt
			}
			w, prepared, err := p.loadWork(r.JobID)
			if err != nil {
				return err
			}
			if _, err = p.validateSubmission(context.Background(), r, w, prepared); err != nil {
				return err
			}
			submissions++
			size += maxSubmissionSize
		case 'n':
			if len(k) != 73 || !canonicalHash(string(v), false) {
				return ErrCorrupt
			}
			raw, e := p.db.Get(recordKey(string(v)), nil)
			var r Submission
			if e != nil || decode(raw, maxSubmissionSize, &r) != nil || !bytes.Equal(k, nonceKey(r.JobID, r.Nonce)) {
				return ErrCorrupt
			}
			nonces++
		case 'q', 'a':
			if len(k) != 9 || !canonicalHash(string(v), false) {
				return ErrCorrupt
			}
			raw, e := p.db.Get(recordKey(string(v)), nil)
			var r Submission
			if e != nil || decode(raw, maxSubmissionSize, &r) != nil {
				return ErrCorrupt
			}
			n := binary.BigEndian.Uint64(k[1:])
			if n == 0 {
				return ErrCorrupt
			}
			if k[0] == 'q' {
				if n != r.Sequence {
					return ErrCorrupt
				}
				sequences++
			} else {
				if n != r.AcceptedSequence {
					return ErrCorrupt
				}
				accepted++
			}
		default:
			return ErrCorrupt
		}
		if works > p.meta.Works || submissions > p.meta.Submissions || nonces > p.meta.Submissions || sequences > p.meta.Submissions || accepted > p.meta.Accepted || size > p.meta.Bytes {
			return ErrCorrupt
		}
	}
	if it.Error() != nil || works != p.meta.Works || submissions != p.meta.Submissions || nonces != submissions || sequences != submissions || accepted != p.meta.Accepted || size != p.meta.Bytes {
		return ErrCorrupt
	}
	return nil
}

func (p *Pool) Close() error {
	if p == nil || p.gate == nil || p.closeDone == nil || p.db == nil {
		return ErrClosed
	}
	p.lifecycle.Lock()
	if p.closing {
		done := p.closeDone
		p.lifecycle.Unlock()
		<-done
		return p.closeErr
	}
	p.closing = true
	p.lifecycle.Unlock()
	p.gate <- struct{}{}
	if p.db.Close() != nil {
		p.closeErr = ErrPoisoned
	}
	<-p.gate
	close(p.closeDone)
	return p.closeErr
}
