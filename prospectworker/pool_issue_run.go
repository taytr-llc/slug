// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/poolservice"
)

type poolRecoverableAttempt struct {
	owner   *PoolWorker
	parent  context.Context
	request string
}

func recoverableIssueRetry(e error) bool {
	var failure *poolservice.ClientError
	if !errors.As(e, &failure) || failure == nil {
		return false
	}
	return failure.Code == "busy" || failure.Code == "rate_limited" || failure.Code == "unavailable" || failure.Code == "outcome_unknown"
}

func (w *PoolWorker) issueRecoverable(ctx context.Context) (*PoolScan, error) {
	b, r := w.outbox, w.recovery
	if ctx == nil {
		return nil, ErrCorrupt
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if r == nil || !w.authorityMatches() {
		return nil, ErrCorrupt
	}
	b.mu.Lock()
	if e := b.state(); e != nil {
		b.mu.Unlock()
		return nil, e
	}
	if b.recovery == nil || b.recovery.readOnly || b.recovery.config != r.config || !b.running || b.cachedScan != nil || b.recovery.attempt != nil {
		b.mu.Unlock()
		return nil, ErrCorrupt
	}
	id := b.meta.ActiveRequestID
	if id == "" {
		if w.recoveryIssueFull() {
			b.mu.Unlock()
			return nil, ErrCapacity
		}
		var random [32]byte
		if _, e := rand.Read(random[:]); e != nil {
			b.mu.Unlock()
			return nil, e
		}
		id = hex.EncodeToString(random[:])
		if !poolRequestID(id) {
			b.mu.Unlock()
			return nil, ErrCorrupt
		}
		exists, e := b.db.Has(poolRequestKey(id), nil)
		if e != nil || exists {
			b.mu.Unlock()
			return nil, ErrCorrupt
		}
		q := poolIssueRow{Version: 1, Request: poolIssueRequest{id}}
		raw, e := json.Marshal(q)
		if e != nil {
			b.mu.Unlock()
			return nil, e
		}
		reserve, e := poolActiveReserve(q)
		if e != nil {
			b.mu.Unlock()
			return nil, e
		}
		m := b.meta
		m.Requests++
		m.ActiveRequestID = id
		m.Bytes += uint64(len(raw))
		m.Reserved += reserve
		if !b.fits(m) {
			b.mu.Unlock()
			return nil, ErrCapacity
		}
		batch := new(leveldb.Batch)
		batch.Put(poolRequestKey(id), raw)
		if e = b.commit(batch, m); e != nil {
			b.mu.Unlock()
			return nil, e
		}
	}
	before, e := b.db.Get(poolRequestKey(id), nil)
	if e != nil {
		b.mu.Unlock()
		return nil, ErrCorrupt
	}
	q, e := b.decodeIssueRow(before, id)
	if e != nil || q.Result != nil {
		b.mu.Unlock()
		return nil, ErrCorrupt
	}
	attempt := &poolRecoverableAttempt{w, ctx, id}
	b.recovery.attempt = attempt
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		if b.recovery.attempt == attempt {
			b.recovery.attempt = nil
		}
		b.mu.Unlock()
	}()
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	response, issueErr := r.authority.ResumeIssue(call, id)
	callErr := call.Err()
	cancel()
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if callErr != nil {
		return nil, &poolservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	if issueErr != nil {
		return nil, issueErr
	} // No error clears the durable request.
	if !w.authorityMatches() || r.client.CheckIssueResponse(id, response) != nil {
		return nil, ErrCorrupt
	}
	// Own the entire validated response outside the outbox lock. The receipt is
	// intentionally absent from canonical/durable JSON and cannot be reminted.
	q.Result = &response
	raw, e := json.Marshal(q)
	if e != nil {
		return nil, ErrCorrupt
	}
	owned, e := b.decodeIssueRow(raw, id)
	if e != nil || owned.Result == nil {
		return nil, ErrCorrupt
	}
	var prepared *poolPreparedScan
	var scanRaw []byte
	if owned.Result.Result.State == "work" {
		work := owned.Result.Result.Work
		if work == nil {
			return nil, ErrCorrupt
		}
		prepared, e = preparePoolScan(&PoolScan{Version: 2, Work: *work}, w.config)
		if e != nil {
			return nil, e
		}
		if prepared.reservation > poolMaxEntrySize {
			return nil, ErrCapacity
		}
		scanRaw, e = json.Marshal(poolIssueScan{1, id})
		if e != nil {
			return nil, e
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if e = b.state(); e != nil {
		return nil, e
	}
	if ctx.Err() != nil || attempt.parent.Err() != nil || attempt.owner != w || attempt.request != id || b.recovery.attempt != attempt || !b.running || b.recovery.config != r.config || b.meta.ActiveRequestID != id || b.cachedScan != nil {
		return nil, ErrCorrupt
	}
	current, e := b.db.Get(poolRequestKey(id), nil)
	if e != nil || string(current) != string(before) {
		return nil, ErrCorrupt
	}
	oldReserve, e := poolActiveReserve(poolIssueRow{Version: 1, Request: poolIssueRequest{id}})
	if e != nil {
		return nil, e
	}
	m := b.meta
	if m.Bytes < uint64(len(before)) || m.Reserved < oldReserve {
		return nil, ErrCorrupt
	}
	m.Bytes = m.Bytes - uint64(len(before)) + uint64(len(raw))
	m.Reserved -= oldReserve
	m.ActiveRequestID = ""
	batch := new(leveldb.Batch)
	batch.Put(poolRequestKey(id), raw)
	if prepared != nil {
		m.Bytes += uint64(len(scanRaw)) + 8
		m.FirstReserved = prepared.reservation
		m.Reserved += prepared.reservation
		batch.Put([]byte("scan"), scanRaw)
		batch.Put([]byte("cursor"), cursor(0))
	}
	if !b.fits(m) {
		return nil, ErrCapacity
	}
	if e = b.commit(batch, m); e != nil {
		return nil, e
	}
	b.recovery.attempt = nil
	if prepared == nil {
		return nil, errPoolIssueTerminal
	}
	b.cachedScan = &prepared.scan
	b.cachedEntryBytes = prepared.reservation
	scan, e := clonePoolScan(prepared.scan)
	return &scan, e
}
