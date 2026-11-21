// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
)

const poolIssueApplication = "slug-pool-worker-issue-recovery"
const poolIssueMetadataLimit = 2 * maxMetadataSize
const poolIssueRowLimit = poolservice.MaxWorkResponseSize + 256

type poolIssueRecovery struct {
	config   RecoverablePoolConfig
	readOnly bool
	attempt  *poolRecoverableAttempt
}
type poolIssueMarker struct {
	Application string                `json:"application"`
	Version     uint32                `json:"version"`
	Config      RecoverablePoolConfig `json:"config"`
}
type poolIssueMetadata struct {
	Application     string                `json:"application"`
	Version         uint32                `json:"version"`
	Config          RecoverablePoolConfig `json:"config"`
	LastSequence    uint64                `json:"last_sequence"`
	Records         uint64                `json:"records"`
	Bytes           uint64                `json:"bytes"`
	Pending         uint64                `json:"pending"`
	Reserved        uint64                `json:"reserved"`
	Requests        uint64                `json:"requests"`
	ActiveRequestID string                `json:"active_request_id"`
}
type poolIssueRequest struct {
	RequestID string `json:"request_id"`
}
type poolIssueRow struct {
	Version uint32                     `json:"version"`
	Request poolIssueRequest           `json:"request"`
	Result  *poolservice.IssueResponse `json:"result"`
}
type poolIssueScan struct {
	Version   uint32 `json:"version"`
	RequestID string `json:"request_id"`
}

func issueSourceID(c PoolConfig) string { id, _ := poolshare.DailyConfigID(c.Source); return id }
func poolRequestID(id string) bool {
	return manifestHashShape(id) && id != "0000000000000000000000000000000000000000000000000000000000000000"
}
func poolRequestKey(id string) []byte {
	raw, e := hex.DecodeString(id)
	if e != nil || !poolRequestID(id) {
		return nil
	}
	return append([]byte{'q'}, raw...)
}

func (b *PoolOutbox) encodeMetadata(m poolOutboxMetadata) ([]byte, error) {
	if b.recovery == nil {
		return encodePoolMetadata(m)
	}
	return json.Marshal(poolIssueMetadata{poolIssueApplication, 1, b.recovery.config, m.LastSequence, m.Records, m.Bytes, m.Pending, m.Reserved, m.Requests, m.ActiveRequestID})
}
func (b *PoolOutbox) decodeMetadata(raw []byte) error {
	if b.recovery == nil {
		return decodePoolMetadata(raw, b.config.Version, &b.meta)
	}
	var m poolIssueMetadata
	if poolDecodeBounded(raw, poolIssueMetadataLimit, &m, 14) != nil || m.Application != poolIssueApplication || m.Version != 1 || m.Config != b.recovery.config || m.ActiveRequestID != "" && !poolRequestID(m.ActiveRequestID) {
		return ErrCorrupt
	}
	b.meta = poolOutboxMetadata{Application: poolApplication, Version: 2, Config: b.config, LastSequence: m.LastSequence, Records: m.Records, Bytes: m.Bytes, Pending: m.Pending, Reserved: m.Reserved, Requests: m.Requests, ActiveRequestID: m.ActiveRequestID}
	return nil
}

func CreateRecoverableDailyPoolOutbox(path string, c RecoverablePoolConfig) (*PoolOutbox, error) {
	if e := manifestProduction(); e != nil {
		return nil, e
	}
	c, e := ValidateRecoverableDailyPoolConfig(c)
	if e != nil {
		return nil, e
	}
	raw, e := json.Marshal(poolIssueMarker{poolIssueApplication, 1, c})
	if e != nil || len(raw) > poolIssueMetadataLimit {
		return nil, ErrCorrupt
	}
	if e = os.Mkdir(path, 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(path, poolMarkerName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, e
	}
	n, we := f.Write(raw)
	if n != len(raw) && we == nil {
		we = io.ErrShortWrite
	}
	se := f.Sync()
	ce := f.Close()
	if ce != nil {
		return failedPoolIssueMarker(f, c, errors.Join(we, se, ce))
	}
	if e = errors.Join(we, se); e != nil {
		return nil, e
	}
	dbpath := filepath.Join(path, "db")
	if e = os.Mkdir(dbpath, 0700); e != nil {
		return nil, e
	}
	db, e := leveldb.OpenFile(dbpath, &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
	if e != nil {
		return nil, e
	}
	b := &PoolOutbox{db: db, config: c.Base, recovery: &poolIssueRecovery{config: c}, meta: poolOutboxMetadata{Application: poolApplication, Version: 2, Config: c.Base}, writeBatch: db.Write}
	if e = b.commit(new(leveldb.Batch), b.meta); e != nil {
		return poolOutboxConstructorFailure(b, e, true)
	}
	if e = errors.Join(syncDir(dbpath), syncDir(path), syncDir(filepath.Dir(filepath.Clean(path)))); e != nil {
		return poolOutboxConstructorFailure(b, e, true)
	}
	return b, nil
}
func OpenRecoverableDailyPoolOutbox(path string, c RecoverablePoolConfig) (*PoolOutbox, error) {
	return openRecoverablePoolOutbox(path, c, false)
}
func OpenRecoverableDailyPoolOutboxAudit(path string, c RecoverablePoolConfig) (*PoolOutbox, error) {
	return openRecoverablePoolOutbox(path, c, true)
}
func openRecoverablePoolOutbox(path string, c RecoverablePoolConfig, readOnly bool) (*PoolOutbox, error) {
	if e := manifestProduction(); e != nil {
		return nil, e
	}
	c, e := ValidateRecoverableDailyPoolConfig(c)
	if e != nil {
		return nil, e
	}
	if regularDirectory(path) != nil || regularDirectory(filepath.Join(path, "db")) != nil {
		return nil, ErrCorrupt
	}
	name := filepath.Join(path, poolMarkerName)
	before, e := os.Lstat(name)
	if e != nil || !before.Mode().IsRegular() || before.Size() > poolIssueMetadataLimit {
		return nil, ErrCorrupt
	}
	f, e := os.Open(name)
	if e != nil {
		return nil, e
	}
	opened, se := f.Stat()
	raw, re := io.ReadAll(io.LimitReader(f, poolIssueMetadataLimit+1))
	ce := f.Close()
	if ce != nil {
		return failedPoolIssueMarker(f, c, errors.Join(se, re, ce))
	}
	if errors.Join(se, re) != nil || opened == nil || !os.SameFile(before, opened) {
		return nil, ErrCorrupt
	}
	var marker poolIssueMarker
	if poolDecodeBounded(raw, poolIssueMetadataLimit, &marker, 14) != nil || marker.Application != poolIssueApplication || marker.Version != 1 || marker.Config != c {
		return nil, ErrCorrupt
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, Strict: opt.StrictAll, ReadOnly: readOnly})
	if e != nil {
		return nil, e
	}
	b := &PoolOutbox{db: db, config: c.Base, recovery: &poolIssueRecovery{config: c, readOnly: readOnly}, writeBatch: db.Write}
	if e = b.audit(); e != nil {
		return poolOutboxConstructorFailure(b, e, true)
	}
	return b, nil
}

func (b *PoolOutbox) decodeIssueRow(raw []byte, id string) (poolIssueRow, error) {
	var q poolIssueRow
	if !poolRequestID(id) || poolDecodeBounded(raw, poolIssueRowLimit, &q, 14) != nil || q.Version != 1 || q.Request.RequestID != id {
		return q, ErrCorrupt
	}
	if q.Result != nil && poolservice.ValidateRecoverableIssueResponse(b.recovery.config.selection(), id, *q.Result, false) != nil {
		return q, ErrCorrupt
	}
	return q, nil
}
func poolActiveReserve(q poolIssueRow) (uint64, error) {
	if q.Result != nil || !poolRequestID(q.Request.RequestID) {
		return 0, ErrCorrupt
	}
	raw, e := json.Marshal(q)
	if e != nil {
		return 0, e
	}
	scan, _ := json.Marshal(poolIssueScan{1, q.Request.RequestID})
	maximumQ := len(raw) - 4 + poolservice.MaxWorkResponseSize
	return uint64(maximumQ - len(raw) + len(scan) + 8 + poolMaxEntrySize), nil
}

type poolIssueAudit struct {
	bytes, reserved uint64
	jobs            map[string][32]byte
	requests        map[string]string
	entries         map[string]bool
}

func workDigest(w poolshare.MiningWork) ([32]byte, error) {
	raw, e := json.Marshal(w)
	if e != nil || len(raw) > poolMaxScanSize {
		return [32]byte{}, ErrCorrupt
	}
	return sha256.Sum256(raw), nil
}
func (a *poolIssueAudit) match(w poolshare.MiningWork) bool {
	expected, ok := a.jobs[w.Job.ID]
	actual, e := workDigest(w)
	return ok && e == nil && expected == actual
}
func (b *PoolOutbox) auditIssueRequests() (*poolIssueAudit, error) {
	a := &poolIssueAudit{jobs: map[string][32]byte{}, requests: map[string]string{}, entries: map[string]bool{}}
	it := b.db.NewIterator(util.BytesPrefix([]byte{'q'}), nil)
	defer it.Release()
	var count, nulls uint64
	for it.Next() {
		if len(it.Key()) != 33 {
			return nil, ErrCorrupt
		}
		id := hex.EncodeToString(it.Key()[1:])
		q, e := b.decodeIssueRow(it.Value(), id)
		if e != nil {
			return nil, e
		}
		count++
		if count > b.meta.Requests || count > b.recovery.config.IssueProfile.MaxRequests {
			return nil, ErrCorrupt
		}
		a.bytes += uint64(len(it.Value()))
		if a.bytes > b.config.MaxBytes {
			return nil, ErrCorrupt
		}
		if q.Result == nil {
			nulls++
			if id != b.meta.ActiveRequestID {
				return nil, ErrCorrupt
			}
			a.reserved, e = poolActiveReserve(q)
			if e != nil {
				return nil, e
			}
		} else if q.Result.Result.State == "work" {
			w := q.Result.Result.Work
			if w == nil {
				return nil, ErrCorrupt
			}
			if _, ok := a.jobs[w.Job.ID]; ok {
				return nil, ErrCorrupt
			}
			digest, e := workDigest(*w)
			if e != nil {
				return nil, e
			}
			a.jobs[w.Job.ID] = digest
			a.requests[id] = w.Job.ID
		}
	}
	wantNull := uint64(0)
	if b.meta.ActiveRequestID != "" {
		wantNull = 1
	}
	if it.Error() != nil || count != b.meta.Requests || nulls != wantNull {
		return nil, ErrCorrupt
	}
	return a, nil
}
func (b *PoolOutbox) decodeIssueScan(raw []byte, s *PoolScan, a *poolIssueAudit) error {
	var ref poolIssueScan
	if poolDecodeCanonical(raw, 256, &ref) != nil || ref.Version != 1 || !poolRequestID(ref.RequestID) {
		return ErrCorrupt
	}
	if _, ok := a.requests[ref.RequestID]; !ok {
		return ErrCorrupt
	}
	row, e := b.db.Get(poolRequestKey(ref.RequestID), nil)
	if e != nil {
		return ErrCorrupt
	}
	q, e := b.decodeIssueRow(row, ref.RequestID)
	if e != nil || q.Result == nil || q.Result.Result.Work == nil {
		return ErrCorrupt
	}
	*s = PoolScan{Version: 2, Work: *q.Result.Result.Work}
	if validatePoolScan(*s, b.config) != nil {
		return ErrCorrupt
	}
	return nil
}
func (b *PoolOutbox) clearIssueScan(s *PoolScan) error {
	if s != nil || b.meta.ActiveRequestID != "" || b.recovery.readOnly {
		return ErrCorrupt
	}
	raw, e := b.db.Get([]byte("scan"), nil)
	if errors.Is(e, leveldb.ErrNotFound) {
		if b.cachedScan != nil {
			return ErrCorrupt
		}
		return nil
	}
	if e != nil {
		return ErrCorrupt
	}
	m := b.meta
	if m.Bytes < uint64(len(raw))+8 || m.Reserved < m.FirstReserved {
		return ErrCorrupt
	}
	m.Bytes -= uint64(len(raw)) + 8
	m.Reserved -= m.FirstReserved
	m.FirstReserved = 0
	batch := new(leveldb.Batch)
	batch.Delete([]byte("scan"))
	batch.Delete([]byte("cursor"))
	if e = b.commit(batch, m); e != nil {
		return e
	}
	b.cachedScan = nil
	b.cachedEntryBytes = 0
	return nil
}

func failedPoolIssueMarker(f *os.File, c RecoverablePoolConfig, e error) (*PoolOutbox, error) {
	return &PoolOutbox{config: c.Base, recovery: &poolIssueRecovery{config: c}, closed: true, closeErr: e, failedMarker: f}, e
}
