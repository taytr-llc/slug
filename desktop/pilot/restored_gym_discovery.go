// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slugwallet/wallet"
)

// RestoredRecoveryResult is dated display data. CurrentlyAbsent also includes
// an escrow spent by a mempool transaction; it is not permanent spend evidence.
// Matched only describes original-file path coverage, never signing permission.
type RestoredRecoveryResult struct {
	Candidate    fighterownership.RecoveryCommitCandidate `json:"candidate"`
	Status       string                                   `json:"status"`
	ObservedAtMS int64                                    `json:"observed_at_ms"`
}
type RestoredRecoveryDiscovery struct {
	Head            fighterownership.HeadRef `json:"head"`
	ExaminedCommits uint32                   `json:"examined_commits"`
	ExaminedKeys    uint32                   `json:"examined_keys"`
	TotalKeys       uint32                   `json:"total_keys"`
	Complete        bool                     `json:"complete"`
	Result          *RestoredRecoveryResult  `json:"result,omitempty"`
}
type restoredDiscovery struct {
	head         *fighterownership.HeadLease
	capture      *fighterownership.RecoveryCommitCapture
	cursor       uint32
	pageComplete bool
	complete     bool
	candidate    *fighterownership.RecoveryCommitCandidate
	match        *wallet.CarrierRecoveryMatchScan
	progress     wallet.CarrierRecoveryProgress
	last         *RestoredRecoveryResult
}

func (d *restoredDiscovery) Close() error {
	if d == nil {
		return nil
	}
	var e error
	if d.match != nil {
		e = errors.Join(e, d.match.Close())
	}
	if d.capture != nil {
		e = errors.Join(e, d.capture.Close())
	}
	if d.head != nil {
		e = errors.Join(e, d.head.Close())
	}
	return e
}
func (s *RestoredGymSession) clearDiscovery() error {
	if s.discovery == nil {
		return nil
	}
	e := s.discovery.Close()
	if e == nil {
		s.discovery = nil
	}
	return e
}
func (s *RestoredGymSession) disclose(ctx context.Context) error {
	e := errors.Join(ctx.Err(), s.life.Err(), s.root.check())
	s.rememberCleanup(e)
	return e
}

// Fighters returns one bounded page from the exact already-replayed head. A
// caller continuing pagination supplies that original head; a changed head
// refuses instead of silently combining snapshots.
func (s *RestoredGymSession) Fighters(ctx context.Context, expected fighterownership.HeadRef, after uint32) (fighterownership.OwnedPage, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return fighterownership.OwnedPage{}, e
	}
	defer end()
	if s.ready == nil || s.store == nil {
		return fighterownership.OwnedPage{}, ErrRestoredGym
	}
	c, e := s.store.CaptureOwned(work, s.initial.OriginalManifest.GymPublicKey)
	if e != nil {
		return fighterownership.OwnedPage{}, e
	}
	if c.Head() != expected {
		return fighterownership.OwnedPage{}, fighterownership.ErrStale
	}
	v, e := s.store.OwnedPage(work, c, after, 8)
	if e != nil {
		return fighterownership.OwnedPage{}, e
	}
	if e = s.disclose(work); e != nil {
		return fighterownership.OwnedPage{}, e
	}
	return v, nil
}

// BeginRecoveryDiscovery holds one concrete public capture, at most 4096
// compact indices. It creates no carrier intent, wallet reservation or signer.
// Refresh is explicit and must complete first; restarting this scan only
// replaces this process's read-only cursor.
func (s *RestoredGymSession) BeginRecoveryDiscovery(ctx context.Context) (RestoredRecoveryDiscovery, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	defer end()
	if s.ready == nil || s.carrier == nil || s.store == nil {
		return RestoredRecoveryDiscovery{}, ErrRestoredGym
	}
	if e = s.clearDiscovery(); e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	head, e := s.store.Head(work)
	if e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	lease, e := s.store.AcquireHead(work, head)
	if e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	capture, e := s.store.CaptureRecoveryCommits(work, lease, s.initial.OriginalManifest.GymPublicKey)
	if e != nil {
		return RestoredRecoveryDiscovery{}, errors.Join(e, lease.Close())
	}
	s.discovery = &restoredDiscovery{head: lease, capture: capture}
	if e = s.disclose(work); e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	return RestoredRecoveryDiscovery{Head: head}, nil
}

// RecoveryDiscoveryStep examines at most one canonical candidate and one
// 128-raw-child wallet chunk. Only its latest result is retained in the reply;
// the session never builds an unbounded list of handles or transaction bytes.
func (s *RestoredGymSession) RecoveryDiscoveryStep(ctx context.Context) (RestoredRecoveryDiscovery, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	defer end()
	d := s.discovery
	if d == nil || d.capture == nil {
		return RestoredRecoveryDiscovery{}, fightercarrier.ErrNotFound
	}
	if e = d.head.Check(); e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	result := RestoredRecoveryDiscovery{Head: d.capture.Head(), ExaminedCommits: d.cursor, Complete: d.complete}
	if d.complete {
		return result, s.disclose(work)
	}
	if d.candidate == nil {
		page, e := s.store.RecoveryCommitPage(work, d.capture, d.cursor, 1)
		if e != nil {
			return RestoredRecoveryDiscovery{}, e
		}
		if len(page.Candidates) == 0 {
			if !page.Complete {
				return RestoredRecoveryDiscovery{}, fighterownership.ErrCorrupt
			}
			d.complete = true
			result.Complete = true
			return result, s.disclose(work)
		}
		if len(page.Candidates) != 1 || page.Next != d.cursor+1 {
			return RestoredRecoveryDiscovery{}, fighterownership.ErrCorrupt
		}
		candidate := page.Candidates[0]
		d.candidate = &candidate
		d.pageComplete = page.Complete
	}
	finish := func(status string) (RestoredRecoveryDiscovery, error) {
		last, e := d.finish(status, time.Now().UnixMilli())
		if e != nil {
			return RestoredRecoveryDiscovery{}, e
		}
		result.Result = last
		result.ExaminedCommits = d.cursor
		result.Complete = d.complete
		if e = s.disclose(work); e != nil {
			return RestoredRecoveryDiscovery{}, e
		}
		return result, nil
	}

	if d.match == nil {
		public, e := s.store.PrepareRecovery(work, d.head, d.candidate.CommitTxID)
		// Exact canonical escrow absence is a typed, dated observation. All other
		// unavailable evidence stops the scan without claiming completeness.
		if errors.Is(e, fighterownership.ErrEscrowSpent) {
			return finish("currently_absent")
		}
		if e != nil {
			return RestoredRecoveryDiscovery{}, e
		}
		d.match, e = s.carrier.BeginRecoveryMatch(work, public)
		if errors.Is(e, fighterownership.ErrEscrowSpent) {
			return finish("currently_absent")
		}
		if e != nil {
			return RestoredRecoveryDiscovery{}, e
		}
	}
	p, e := d.match.Step(work)
	if errors.Is(e, fighterownership.ErrEscrowSpent) {
		return finish("currently_absent")
	}
	if e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	d.progress = p
	result.ExaminedKeys = p.Examined
	result.TotalKeys = p.Total
	if p.Complete {
		status := "outside_original_file"
		if p.Matched {
			status = "matched"
		}
		return finish(status)
	}
	if e = s.disclose(work); e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	return result, nil
}

// RecoveryDiscoveryStatus repeats the last exact result after a lost response.
// It cannot start, advance, sign or grant a new candidate capability.
func (s *RestoredGymSession) RecoveryDiscoveryStatus(ctx context.Context) (RestoredRecoveryDiscovery, error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	defer end()
	d := s.discovery
	if d == nil {
		return RestoredRecoveryDiscovery{}, fightercarrier.ErrNotFound
	}
	if e = d.head.Check(); e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	v := RestoredRecoveryDiscovery{Head: d.capture.Head(), ExaminedCommits: d.cursor, ExaminedKeys: d.progress.Examined, TotalKeys: d.progress.Total, Complete: d.complete}
	if d.last != nil {
		last := *d.last
		v.Result = &last
	}
	if e = s.disclose(work); e != nil {
		return RestoredRecoveryDiscovery{}, e
	}
	return v, nil
}

func (d *restoredDiscovery) finish(status string, at int64) (*RestoredRecoveryResult, error) {
	if d.candidate == nil || d.cursor >= fighterownership.MaxRecoveryCommits {
		return nil, fighterownership.ErrCorrupt
	}
	if d.match != nil {
		if e := d.match.Close(); e != nil {
			return nil, e
		}
		d.match = nil
	}
	retained := RestoredRecoveryResult{Candidate: *d.candidate, Status: status, ObservedAtMS: at}
	d.last = &retained
	d.cursor++
	d.complete = d.pageComplete
	d.candidate = nil
	d.progress = wallet.CarrierRecoveryProgress{}
	disclosed := retained
	return &disclosed, nil
}
