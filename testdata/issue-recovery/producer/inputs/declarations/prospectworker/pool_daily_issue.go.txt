// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"errors"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/poolservice"
)

var errPoolIssueHeld = errors.New("daily pool issuance outcome unknown")

// This token is only the original live attempt's completion fence. It is never
// serialized and is not an issuance identifier or remote authority.
type poolDailyIssueAttempt struct {
	owner  *PoolWorker
	parent context.Context
}

func dailyIssueNoEffect(err error) bool {
	f, ok := err.(*poolservice.ClientError)
	return ok && f != nil && !f.OutcomeUnknown && (f.Code == "busy" || f.Code == "rate_limited")
}

func (w *PoolWorker) issueDaily(ctx context.Context) (*PoolScan, error) {
	b := w.outbox
	if ctx == nil || ctx.Err() != nil || !w.authorityMatches() {
		return nil, errPoolIssueHeld
	}
	b.mu.Lock()
	if e := b.state(); e != nil {
		b.mu.Unlock()
		return nil, e
	}
	if w.config.Version != PoolVersion2 || b.config != w.config || !b.running || b.meta.IssuePending || b.cachedScan != nil || b.issueAttempt != nil {
		b.mu.Unlock()
		return nil, errPoolIssueHeld
	}
	m := b.meta
	m.IssuePending = true
	if e := b.commit(new(leveldb.Batch), m); e != nil {
		b.mu.Unlock()
		return nil, e
	}
	attempt := &poolDailyIssueAttempt{w, ctx}
	b.issueAttempt = attempt
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		if b.issueAttempt == attempt {
			b.issueAttempt = nil
		}
		b.mu.Unlock()
	}()

	if ctx.Err() != nil || !w.authorityMatches() {
		return nil, errPoolIssueHeld
	}
	// Neither transport nor complete response validation holds the outbox mutex.
	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	work, issueErr := w.authority.Issue(call)
	callErr := call.Err()
	cancel()
	if ctx.Err() != nil || callErr != nil || !w.authorityMatches() {
		return nil, errPoolIssueHeld
	}
	var prepared *poolPreparedScan
	if issueErr == nil {
		var e error
		prepared, e = preparePoolScan(&PoolScan{Version: PoolVersion2, Work: work}, w.config)
		if e != nil {
			return nil, errPoolIssueHeld
		}
	} else if !dailyIssueNoEffect(issueErr) {
		return nil, errPoolIssueHeld
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return nil, e
	}
	if ctx.Err() != nil || attempt.parent.Err() != nil || attempt.owner != w || b.issueAttempt != attempt || !b.running || b.config != w.config || !b.meta.IssuePending || b.cachedScan != nil {
		return nil, errPoolIssueHeld
	}
	m = b.meta
	m.IssuePending = false
	if issueErr != nil {
		if e := b.commit(new(leveldb.Batch), m); e != nil {
			return nil, e
		}
		b.issueAttempt = nil
		return nil, issueErr
	}
	if e := b.savePreparedScan(prepared, m); e != nil {
		if errors.Is(e, ErrCapacity) || errors.Is(e, ErrCorrupt) {
			return nil, errPoolIssueHeld
		}
		return nil, e
	}
	b.issueAttempt = nil
	// The worker and retained cache need separate owned mutable cursor objects.
	scan := prepared.scan
	return &scan, nil
}
