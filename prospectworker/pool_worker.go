// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sync"
	"time"

	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectwork"
)

// PoolWorker mines only the pool's exact issued candidate. Run is single-use;
// cancellation joins a hash already started and synchronizes its result. Close
// the outbox, transports and account resources only after Run returns.
type PoolWorker struct {
	config         PoolConfig
	authority      poolservice.Authority
	admission      ManifestAuthority
	node           Node
	signer         Signer
	outbox         *PoolOutbox
	observer       ManifestReceiptObserver
	cycleHook      ManifestCycleHook
	mu             sync.Mutex
	status         Status
	started        bool
	now            func() time.Time
	wait           func(context.Context, time.Duration) error
	check          func(context.Context, *prospectwork.ManifestWork, uint32) (prospectwork.ShareCheck, error)
	current        *PoolScan
	lastTime       int64
	capacityPaused bool
	recovery       *poolRecoveryWorker
}

func NewPool(c PoolConfig, a poolservice.Authority, admission ManifestAuthority, node Node, signer Signer, outbox *PoolOutbox) (*PoolWorker, error) {
	return NewPoolWithHooks(c, a, admission, node, signer, outbox, nil, nil)
}
func NewPoolWithHooks(c PoolConfig, a poolservice.Authority, admission ManifestAuthority, node Node, signer Signer, outbox *PoolOutbox, observer ManifestReceiptObserver, hook ManifestCycleHook) (*PoolWorker, error) {
	return newPool(c, a, admission, node, signer, outbox, observer, hook, false)
}
func newPool(c PoolConfig, a poolservice.Authority, admission ManifestAuthority, node Node, signer Signer, outbox *PoolOutbox, observer ManifestReceiptObserver, hook ManifestCycleHook, daily bool) (*PoolWorker, error) {
	if err := manifestProduction(); err != nil {
		return nil, err
	}
	c, err := validateSelectedPoolConfig(c, daily)
	if err != nil {
		return nil, err
	}
	if a == nil || admission == nil || node == nil || signer == nil || outbox == nil {
		return nil, errors.New("pool worker requires both pinned authorities, node, signer and outbox")
	}
	w := &PoolWorker{config: c, authority: a, admission: admission, node: node, signer: signer, outbox: outbox, observer: observer, cycleHook: hook, now: time.Now, wait: waitFor, status: Status{State: "ready"}, check: func(ctx context.Context, work *prospectwork.ManifestWork, nonce uint32) (prospectwork.ShareCheck, error) {
		return work.CheckNonce(ctx, nonce)
	}}
	if !w.authorityMatches() {
		return nil, ErrCorrupt
	}
	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if outbox.state() != nil || outbox.recovery != nil || !reflect.DeepEqual(outbox.config, c) {
		return nil, ErrCorrupt
	}
	return w, nil
}
func (w *PoolWorker) authorityMatches() bool {
	if w.recovery != nil {
		return w.recoverableAuthorityMatches()
	}
	c, a, m := w.config, w.authority, w.admission
	return a.ProtocolVersion() == c.Version && a.AuthorityOrigin() == c.AuthorityOrigin && a.SourceConfig() == c.Source && a.TransportTrustID() == c.TransportTrustID && a.PublicKey() == c.PublicKey && m.ProtocolVersion() == 3 && m.AuthorityOrigin() == c.AdmissionOrigin && m.AuthorityPin() == c.Source.AuthorityPin && m.TransportTrustID() == c.AdmissionTrustID
}
func (w *PoolWorker) Status() Status { w.mu.Lock(); defer w.mu.Unlock(); return w.status }
func (w *PoolWorker) set(state, code string) {
	pending := w.outbox.pendingCount()
	w.outbox.mu.Lock()
	records, size := w.outbox.meta.Records, w.outbox.meta.Bytes
	w.outbox.mu.Unlock()
	w.mu.Lock()
	w.status.State, w.status.LastError = state, code
	w.status.Records, w.status.Bytes, w.status.Pending = records, size, pending
	w.mu.Unlock()
}
func (w *PoolWorker) clock() (int64, error) {
	now := w.now().UnixMilli()
	if now < 0 || now < w.lastTime {
		return 0, errors.New("pool worker clock moved backwards")
	}
	w.lastTime = now
	return now, nil
}
func (w *PoolWorker) clearScan() error {
	if err := w.outbox.saveScan(nil); err != nil {
		return err
	}
	w.current = nil
	return nil
}
func (w *PoolWorker) runCycleHook(ctx context.Context) (err error) {
	if w.cycleHook == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			err = ErrManifestCycleHook
		}
		if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			err = nil
		}
		if err != nil {
			w.set("failed", "cycle_unavailable")
			err = ErrManifestCycleHook
		}
	}()
	return w.cycleHook(ctx)
}

func (w *PoolWorker) Run(ctx context.Context) (runErr error) {
	if ctx == nil {
		return errors.New("pool worker requires a context")
	}
	if err := manifestProduction(); err != nil {
		return err
	}
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return ErrInUse
	}
	w.started = true
	w.mu.Unlock()
	w.outbox.mu.Lock()
	if err := w.outbox.state(); err != nil {
		w.outbox.mu.Unlock()
		return err
	}
	if w.outbox.running {
		w.outbox.mu.Unlock()
		return ErrInUse
	}
	w.outbox.running = true
	w.outbox.mu.Unlock()
	defer func() {
		w.outbox.mu.Lock()
		w.outbox.running = false
		w.outbox.issueAttempt = nil
		if w.outbox.recovery != nil {
			w.outbox.recovery.attempt = nil
		}
		w.outbox.mu.Unlock()
		s := w.Status()
		if runErr != nil {
			w.set("failed", s.LastError)
		} else {
			w.set("stopped", s.LastError)
		}
	}()
	start := time.Now()
	scan, exists, err := w.outbox.scan()
	if err != nil {
		return err
	}
	if exists {
		w.current = &scan
	}
	var prepared *prospectwork.ManifestWork
	var preparedID string
	var refreshed time.Time
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		if !w.authorityMatches() {
			return ErrCorrupt
		}
		if err := w.runCycleHook(ctx); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		now, err := w.clock()
		if err != nil {
			w.set("failed", "clock")
			return err
		}
		pending, found, err := w.outbox.nextDue(now)
		if err != nil {
			return err
		}
		if found {
			if err := w.reconcile(ctx, &pending); err != nil {
				return err
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		now, err = w.clock()
		if err != nil {
			w.set("failed", "clock")
			return err
		}
		if w.current != nil && (now < w.current.Work.Job.IssuedAtMS || now >= w.current.Work.Job.ExpiresAtMS-renewalLead.Milliseconds() || w.current.NextNonce == 1<<32) {
			if now < w.current.Work.Job.IssuedAtMS {
				w.set("failed", "clock")
				return errors.New("pool job is ahead of worker clock")
			}
			if err := w.clearScan(); err != nil {
				return err
			}
		}
		if w.current == nil {
			w.outbox.mu.Lock()
			full := w.outbox.meta.Records >= w.config.MaxRecords
			held := w.outbox.meta.IssuePending
			if w.recovery != nil {
				full = w.recoveryIssueFull()
			}
			w.outbox.mu.Unlock()
			if held {
				w.set("paused", "issue_unknown")
				if w.wait(ctx, time.Second) != nil {
					return nil
				}
				continue
			}
			if full || w.capacityPaused {
				w.set("paused", "capacity")
				if w.wait(ctx, time.Second) != nil {
					return nil
				}
				continue
			}
			w.set("preparing", "")
			var fresh PoolScan
			if w.recovery != nil {
				issued, issueErr := w.issueRecoverable(ctx)
				if ctx.Err() != nil {
					return nil
				}
				if errors.Is(issueErr, errPoolIssueTerminal) {
					backoff = time.Second
					continue
				}
				if issueErr != nil {
					if errors.Is(issueErr, ErrCapacity) {
						w.capacityPaused = true
						w.set("paused", "capacity")
					} else if recoverableIssueRetry(issueErr) {
						w.set("retrying", poolErrorCode(issueErr))
					} else {
						return issueErr
					}
					if w.wait(ctx, backoff) != nil {
						return nil
					}
					backoff = min(30*time.Second, backoff*2)
					continue
				}
				fresh = *issued
			} else if w.config.Version == PoolVersion2 {
				var issued *PoolScan
				issued, err = w.issueDaily(ctx)
				if ctx.Err() != nil {
					return nil
				}
				if errors.Is(err, errPoolIssueHeld) {
					w.set("paused", "issue_unknown")
					if w.wait(ctx, time.Second) != nil {
						return nil
					}
					continue
				}
				if err != nil {
					if !dailyIssueNoEffect(err) {
						return err
					}
					w.set("retrying", poolErrorCode(err))
					if w.wait(ctx, backoff) != nil {
						return nil
					}
					backoff = min(30*time.Second, backoff*2)
					continue
				}
				fresh = *issued
			} else {
				w.set("preparing", "")
				call, cancel := context.WithTimeout(ctx, 30*time.Second)
				work, issueErr := w.authority.Issue(call)
				cancel()
				if ctx.Err() != nil {
					return nil
				}
				if issueErr != nil {
					w.set("retrying", poolErrorCode(issueErr))
					if w.wait(ctx, backoff) != nil {
						return nil
					}
					backoff = min(30*time.Second, backoff*2)
					continue
				}
				fresh = PoolScan{Version: PoolVersion1, Work: work}
				now, err = w.clock()
				if err != nil {
					return err
				}
				if validatePoolScan(fresh, w.config) != nil {
					return ErrCorrupt
				}
				fresh.Work = poolDeliveryWork(work)
				if now < work.Job.IssuedAtMS || now >= work.Job.ExpiresAtMS-renewalLead.Milliseconds() {
					w.set("retrying", "expired")
					if w.wait(ctx, backoff) != nil {
						return nil
					}
					continue
				}
				if err := w.outbox.saveScan(&fresh); err != nil {
					if errors.Is(err, ErrCapacity) {
						w.capacityPaused = true
						w.set("paused", "capacity")
						if w.wait(ctx, time.Second) != nil {
							return nil
						}
						continue
					}
					return err
				}
			}

			w.current = &fresh
			if w.config.Version == PoolVersion2 && (w.now().UnixMilli() < fresh.Work.Job.IssuedAtMS || w.now().UnixMilli() >= fresh.Work.Job.ExpiresAtMS-renewalLead.Milliseconds()) {
				continue
			}
			prepared = nil
			preparedID = ""
			refreshed = time.Time{}
		}
		s := w.current
		if prepared == nil || preparedID != s.Work.Job.ID {
			prepared, err = preparePoolWork(w.config, s.Work)
			if err != nil {
				return ErrCorrupt
			}
			preparedID = s.Work.Job.ID
			refreshed = time.Time{}
		}
		if err := w.outbox.reserve(*s); err != nil {
			if errors.Is(err, ErrCapacity) {
				w.capacityPaused = true
				w.set("paused", "capacity")
				if w.wait(ctx, time.Second) != nil {
					return nil
				}
				continue
			}
			return err
		}
		w.capacityPaused = false
		if refreshed.IsZero() || w.now().Sub(refreshed) >= refreshInterval {
			call, cancel := context.WithTimeout(ctx, 30*time.Second)
			err = w.node.RecheckContext(call, s.Work.Job.Context)
			cancel()
			if ctx.Err() != nil {
				return nil
			}
			if err != nil {
				if err := w.clearScan(); err != nil {
					return err
				}
				w.set("retrying", "stale")
				if w.wait(ctx, backoff) != nil {
					return nil
				}
				backoff = min(30*time.Second, backoff*2)
				continue
			}
			refreshed = w.now()
		}
		now, err = w.clock()
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		if now >= s.Work.Job.ExpiresAtMS-renewalLead.Milliseconds() {
			continue
		}
		w.mu.Lock()
		w.status.JobID, w.status.ParentHash = s.Work.Job.ID, s.Work.Job.Context.ParentHash
		w.mu.Unlock()
		w.set("hashing", "")
		// CheckNonce ordinarily suppresses a result on cancellation. An already
		// started attempt must instead finish and retain found work before Stop
		// returns; this uncancelled context covers precisely one synchronous hash.
		check, err := w.check(context.WithoutCancel(ctx), prepared, uint32(s.NextNonce))
		if err != nil {
			if errors.Is(err, prospectwork.ErrShareBusy) {
				w.set("retrying", "busy")
				if w.wait(ctx, 100*time.Millisecond) != nil {
					return nil
				}
				continue
			}
			return err
		}
		w.mu.Lock()
		w.status.Attempts++
		w.status.HashesPerSecond = float64(w.status.Attempts) / time.Since(start).Seconds()
		w.mu.Unlock()
		s.NextNonce++
		if check.Candidate != nil {
			entry := poolEntryForCheck(*s, check)
			saved, err := w.outbox.found(entry, *s)
			if err != nil {
				return err
			}
			if check.Block && ctx.Err() == nil {
				if err := w.reconcile(ctx, &saved); err != nil {
					return err
				}
			}
		} else if err := w.outbox.advance(*s); err != nil {
			return err
		}
		backoff = time.Second
	}
}

func (w *PoolWorker) reconcile(ctx context.Context, e *PoolEntry) error {
	w.set("delivering", e.LastError)
	if _, err := w.deliver(ctx, e); err != nil {
		return err
	}
	if !poolDone(*e) {
		if e.RetryCount < 32 {
			e.RetryCount++
		}
		delay := retryDelay(e.RetryCount).Milliseconds()
		now, err := w.clock()
		if err != nil || now > math.MaxInt64-delay {
			return errors.New("invalid pool recovery clock")
		}
		e.NextAttemptMS = now + delay
	} else {
		e.NextAttemptMS = 0
	}
	if err := w.outbox.update(*e); err != nil {
		return err
	}
	code := e.LastError
	if code == "" && e.Fighter != nil {
		code = e.Fighter.LastError
	}
	w.mu.Lock()
	w.status.RecoveryError, w.status.NextRecoveryMS = code, e.NextAttemptMS
	w.mu.Unlock()
	w.set("delivering", code)
	return nil
}

func poolErrorCode(err error) string {
	var failure *poolservice.ClientError
	if errors.As(err, &failure) {
		switch failure.Code {
		case "unavailable", "outcome_unknown", "busy", "capacity", "rate_limited", "expired", "rejected", "not_found", "unauthorized", "forbidden", "invalid_request", "too_large":
			return failure.Code
		}
	}
	return "unavailable"
}
