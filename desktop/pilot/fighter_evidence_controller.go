// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/fighterevidence"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type fighterEvidenceOperation struct {
	job      FighterEvidenceJob
	capture  gymstable.CareerHistoryCapture
	handle   *gymstable.CareerHistoryRead
	epoch    uint64
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	file     *recoveryfile.PublicEvidenceFile
	permit   *EvidencePermit
	admitted bool
}

func cloneEvidenceJob(j FighterEvidenceJob) FighterEvidenceJob {
	if j.Summary != nil {
		v := *j.Summary
		j.Summary = &v
	}
	return j
}
func (s *StableController) evidenceBusyLocked() bool { return s.evidenceOp != nil }
func (s *StableController) cancelEvidenceLocked() {
	if s.evidenceOp != nil {
		s.evidenceOp.cancel()
	}
}
func (s *StableController) evidenceProtected() ([]string, error) {
	cfg, e := os.UserConfigDir()
	if e != nil {
		return nil, e
	}
	roots := []string{s.config.DataDir, filepath.Dir(s.config.NodeBinary), filepath.Join(cfg, "SLUG")}
	if filepath.Base(filepath.Dir(s.config.DataDir)) == "gyms" {
		roots = append(roots, filepath.Dir(filepath.Dir(s.config.DataDir)))
	}
	executable, e := os.Executable()
	if e != nil {
		return nil, e
	}
	for _, file := range []string{executable, s.config.NodeBinary} {
		for parent := filepath.Dir(file); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
			if strings.HasSuffix(strings.ToLower(parent), ".app") {
				roots = append(roots, parent)
				break
			}
		}
	}
	return roots, nil
}
func (s *StableController) FighterEvidenceSnapshot(ctx context.Context) (FighterEvidenceSnapshot, error) {
	out := BlankFighterEvidenceSnapshot()
	if ctx == nil {
		return out, gymstable.ErrInvalid
	}
	e := s.historyView(ctx, s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed || !s.installed || s.stableStatus == nil || !s.careerEnabled || s.evidenceFailure != nil {
			return nil
		}
		v := s.viewPin()
		out = FighterEvidenceSnapshot{Version: 1, State: "idle", View: &v, StoreID: s.stableStatus.StoreID}
		if op := s.evidenceOp; op != nil && op.admitted {
			out.State = "busy"
			out.ActiveJobID = op.job.JobID
		}
		if s.evidenceLast != nil {
			out.LastJobID = s.evidenceLast.JobID
		}
		return nil
	})
	if e != nil {
		return BlankFighterEvidenceSnapshot(), e
	}
	return out, nil
}
func (s *StableController) evidenceLookup(ctx context.Context, view, id string, cancel bool) (FighterEvidenceJob, error) {
	if ctx == nil || !canonicalManifestID(id) {
		return FighterEvidenceJob{}, gymstable.ErrInvalid
	}
	var out FighterEvidenceJob
	e := s.historyView(ctx, view, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed {
			return gymstable.ErrClosed
		}
		if op := s.evidenceOp; op != nil && op.admitted && op.job.JobID == id {
			if cancel {
				op.cancel()
			}
			out = cloneEvidenceJob(op.job)
			return nil
		}
		if s.evidenceLast != nil && s.evidenceLast.JobID == id {
			out = cloneEvidenceJob(*s.evidenceLast)
			return nil
		}
		return ErrEvidenceNotFound
	})
	if e != nil {
		return FighterEvidenceJob{}, e
	}
	return out, nil
}
func (s *StableController) FighterEvidenceJob(ctx context.Context, v, id string) (FighterEvidenceJob, error) {
	return s.evidenceLookup(ctx, v, id, false)
}
func (s *StableController) CancelFighterEvidence(ctx context.Context, v, id string) (FighterEvidenceJob, error) {
	return s.evidenceLookup(ctx, v, id, true)
}

// Begin acquires the pinned FD synchronously. The returned job acknowledges
// ownership/dispatch only; the independent task has a 30-second work budget.
func (s *StableController) BeginFighterEvidence(ctx context.Context, v, hjob, hid, id string, pin recoveryfile.Pin) (FighterEvidenceJob, error) {
	if wire.TestPoW {
		return FighterEvidenceJob{}, gymstable.ErrProfile
	}
	req := FighterEvidenceRequest{ID: 1, Version: 1, Command: "fighter_evidence_begin", ViewID: v, HistoryJobID: hjob, HistoryID: hid, JobID: id, Selection: &pin}
	if ctx == nil || !evidenceRequestValid(req) {
		return FighterEvidenceJob{}, gymstable.ErrInvalid
	}
	if !s.stopMu.TryLock() {
		return FighterEvidenceJob{}, gymstable.ErrBusy
	}
	defer s.stopMu.Unlock()
	select {
	case s.laneGate <- struct{}{}:
	default:
		return FighterEvidenceJob{}, gymstable.ErrBusy
	}
	laneOwned := true
	defer func() {
		if laneOwned {
			<-s.laneGate
		}
	}()
	taskCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	op := &fighterEvidenceOperation{ctx: taskCtx, cancel: cancel, done: make(chan struct{})}
	cleanup := func(err error) (FighterEvidenceJob, error) {
		op.cancel()
		var ce error
		if op.file != nil {
			ce = op.file.Close()
		}
		s.stableMu.Lock()
		if s.evidenceOp == op {
			s.evidenceOp = nil
		}
		if ce != nil {
			s.evidenceFailure = errors.Join(s.evidenceFailure, ce)
		}
		s.stableMu.Unlock()
		if op.permit != nil {
			op.handle = nil
			op.permit.release(op, ce)
		}
		close(op.done)
		return FighterEvidenceJob{}, errors.Join(err, ce)
	}
	e := s.historyView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if _, e := s.historyActivationLocked(); e != nil {
			return e
		}
		if s.evidenceFailure != nil {
			return ErrEvidenceUnavailable
		}
		if s.stopping || s.mining || s.active != nil || s.queued != nil || s.evidenceOp != nil {
			return gymstable.ErrBusy
		}
		if s.evidenceLast != nil && s.evidenceLast.JobID == id {
			return ErrEvidenceOperationConflict
		}
		old := s.historyLast
		if old == nil || old.State != "done" || old.JobID != hjob || old.Capture == nil || old.Capture.HistoryID != hid || old.View != s.viewPin() || s.historyHandle == nil || s.stableStore == nil {
			return gymstable.ErrNotFound
		}
		op.job = FighterEvidenceJob{Version: 1, JobID: id, State: "running", View: s.viewPin(), StoreID: old.StoreID, HistoryJobID: hjob, HistoryID: hid, ProspectID: old.ProspectID}
		op.capture = *old.Capture
		op.handle = s.historyHandle
		op.epoch = s.historyEpoch
		permit := s.stableConfig.EvidencePermit
		if permit == nil {
			permit = &s.localEvidencePermit
		}
		if !permit.reserve(op) {
			return gymstable.ErrCapacity
		}
		op.permit = permit
		s.evidenceOp = op
		return nil
	})
	if e != nil {
		return cleanup(e)
	}
	protected, e := s.evidenceProtected()
	if e == nil {
		op.file, e = recoveryfile.OpenPublicEvidence(pin, protected)
	}
	if e != nil {
		return cleanup(e)
	}
	published := false
	e = s.evidencePublish(ctx, v, func(check func() error) error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if e := s.evidenceOriginalLocked(op); e != nil {
			return e
		}
		if e := check(); e != nil {
			return e
		}
		op.admitted = true
		published = true
		return nil
	})
	if !published {
		if e == nil {
			e = gymstable.ErrCancelled
		}
		return cleanup(e)
	}
	laneOwned = false
	go s.runFighterEvidence(op)
	if e != nil {
		return FighterEvidenceJob{}, ErrEvidenceReplyUnknown
	}
	return cloneEvidenceJob(op.job), nil
}
func (s *StableController) evidenceOriginalLocked(op *fighterEvidenceOperation) error {
	if s.evidenceOp != op || s.stableClosed || s.stopping || s.mining || s.historyHandle != op.handle || s.historyEpoch != op.epoch || s.historyLast == nil || s.historyLast.JobID != op.job.HistoryJobID || s.historyLast.Capture == nil || *s.historyLast.Capture != op.capture || s.viewPin() != op.job.View {
		return gymstable.ErrStaleView
	}
	return op.ctx.Err()
}
func (s *StableController) runFighterEvidence(op *fighterEvidenceOperation) {
	var summary *FighterEvidenceSummary
	if s.evidenceCheckpoint != nil {
		s.evidenceCheckpoint("before-source")
	}
	evidence, e := s.stableStore.ReadCareerHistoryEvidence(op.ctx, s.session(), op.handle)
	if s.evidenceCheckpoint != nil {
		s.evidenceCheckpoint("after-source")
	}
	if e == nil && evidence.Capture != op.capture {
		e = gymstable.ErrConflict
	}
	var raw []byte
	if e == nil {
		raw, e = fighterevidence.Marshal(op.capture.Status.Pin, fighterevidence.Archive{Format: "slug-public-fighter-evidence", Version: 1, Admission: evidence.Admission, Capture: evidence.Capture, Rows: evidence.Rows})
		if e != nil {
			e = errEvidenceInvalid
		}
	}
	if e == nil {
		e = op.ctx.Err()
	}
	if e == nil {
		sum := sha256.Sum256(raw)
		summary = &FighterEvidenceSummary{Version: 1, FileSHA256: hex.EncodeToString(sum[:]), FileBytes: uint32(len(raw)), ProspectID: op.job.ProspectID, HistoryID: op.job.HistoryID, SourceRecordDigest: op.capture.Assignment.SourceRecordDigest, EventCount: op.capture.EventCount, CapturedAsOfMS: op.capture.AsOfMS}
		e = op.file.Stage(op.ctx, raw)
		if s.evidenceCheckpoint != nil {
			s.evidenceCheckpoint("after-stage")
		}
	}
	if e == nil {
		e = op.file.Publish(op.ctx, func(commit func(func() error) error) error {
			return s.evidencePublish(op.ctx, op.job.View.ViewID, func(check func() error) error {
				s.stableMu.Lock()
				defer s.stableMu.Unlock()
				if e := s.evidenceOriginalLocked(op); e != nil {
					return e
				}
				return commit(func() error {
					if e := check(); e != nil {
						return e
					}
					return s.evidenceOriginalLocked(op)
				})
			})
		})
	}
	clear(raw)
	raw = nil
	evidence = gymstable.CareerHistoryEvidence{}
	ce := op.file.Close()
	e = errors.Join(e, ce)
	j := op.job
	j.State = "done"
	j.Summary = summary
	if e != nil {
		j.Summary = nil
		j.State = "failed"
		j.Error = evidenceTaskError(e)
		if !errors.Is(e, recoveryfile.ErrOutcomeUnknown) && ce == nil && (op.ctx.Err() != nil || errors.Is(e, gymstable.ErrCancelled)) {
			j.State = "cancelled"
			j.Error = "evidence_cancelled"
		}
	}
	op.cancel()
	s.stableMu.Lock()
	if ce != nil {
		s.evidenceFailure = errors.Join(s.evidenceFailure, ce)
	}
	if s.evidenceOp == op {
		s.evidenceOp = nil
		if !s.stableClosed {
			owned := cloneEvidenceJob(j)
			s.evidenceLast = &owned
		}
	}
	s.stableMu.Unlock()
	op.handle = nil
	op.permit.release(op, ce)
	close(op.done)
	<-s.laneGate
	s.signal()
}

// Harden admission/final publication against omitted, duplicate, retained or
// error-swallowing lease callbacks. Actual account authority remains original.
func (s *StableController) evidencePublish(ctx context.Context, v string, fn func(func() error) error) error {
	var mu sync.Mutex
	calls := 0
	sealed := false
	var inner error
	outer := s.historyPublish(ctx, v, func(check func() error) error {
		mu.Lock()
		defer mu.Unlock()
		if sealed {
			return gymstable.ErrUnauthorized
		}
		calls++
		if calls != 1 {
			inner = errors.Join(inner, gymstable.ErrUnauthorized)
			return gymstable.ErrUnauthorized
		}
		if check == nil {
			inner = gymstable.ErrUnauthorized
			return inner
		}
		if e := check(); e != nil {
			inner = e
			return e
		}
		inner = fn(check)
		if inner == nil {
			inner = check()
		}
		return inner
	})
	mu.Lock()
	defer mu.Unlock()
	sealed = true
	if calls != 1 {
		return errors.Join(gymstable.ErrUnauthorized, inner, outer)
	}
	return errors.Join(inner, outer)
}
