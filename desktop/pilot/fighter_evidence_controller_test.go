//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/fighterevidence"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func evidenceLaneFixture(t *testing.T) (*StableController, CareerHistoryJob) {
	t.Helper()
	s, pid := historyLaneFixture(t)
	j := historyDone(t, s, pid)
	s.stableMu.Lock()
	s.mining = false
	s.paused = true
	s.needSync = false
	s.stableMu.Unlock()
	s.config.NodeBinary = filepath.Join(s.config.DataDir, "slugd")
	return s, j
}
func evidenceSelection(t *testing.T) (*recoveryfile.Selection, string) {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "public.json")
	selection, e := recoveryfile.Capture(path, nil, recoveryfile.CreateMode)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = selection.Close() })
	return selection, path
}
func evidenceWait(t *testing.T, s *StableController, id string) FighterEvidenceJob {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		j, e := s.FighterEvidenceJob(t.Context(), s.view.ViewID, id)
		if e != nil {
			t.Fatal(e)
		}
		if j.State != "running" {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("evidence task did not join")
	return FighterEvidenceJob{}
}
func TestFighterEvidenceLiveExportOriginalCaptureAndChecksum(t *testing.T) {
	s, history := evidenceLaneFixture(t)
	selection, path := evidenceSelection(t)
	var reads atomic.Int32
	var inside atomic.Bool
	original := s.stableConfig.Lease
	s.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return original(ctx, gymaccount.View, func(a gymaccount.Account, check func() error) error {
			inside.Store(true)
			defer inside.Store(false)
			return fn(a, check)
		})
	}
	s.evidenceCheckpoint = func(stage string) {
		if inside.Load() {
			t.Error("source/staging escaped into account lease", stage)
		}
		if stage == "before-source" {
			reads.Add(1)
		}
	}
	id := strings.Repeat("a", 64)
	j, e := s.BeginFighterEvidence(t.Context(), s.view.ViewID, history.JobID, history.Capture.HistoryID, id, selection.Pin())
	if e != nil || j.JobID != id {
		t.Fatal(j, e)
	}
	j = evidenceWait(t, s, id)
	if j.State != "done" || j.Summary == nil || reads.Load() != 1 {
		t.Fatal(j.State, j.Error, reads.Load())
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if j.Summary.FileSHA256 != hex.EncodeToString(sum[:]) || j.Summary.FileBytes != uint32(len(raw)) {
		t.Fatal("checksum mismatch")
	}
	archive, e := fighterevidence.Parse(raw)
	if e != nil || archive.Capture != *history.Capture || len(archive.Rows) != int(history.Capture.EventCount) {
		t.Fatal("changed selected capture", e)
	}
	if s.historyLast.JobID != history.JobID || s.historyBytes != careerHistoryAllowance || MaxManifestCacheBytes != 1<<20 {
		t.Fatal("export changed legacy history/cache contract")
	}
	j.Summary.FileBytes = 1
	again, e := s.FighterEvidenceJob(t.Context(), s.view.ViewID, id)
	if e != nil || again.Summary.FileBytes != uint32(len(raw)) {
		t.Fatal("caller changed retained metadata", e)
	}
	if _, e = s.BeginFighterEvidence(t.Context(), s.view.ViewID, history.JobID, history.Capture.HistoryID, id, selection.Pin()); !errors.Is(e, ErrEvidenceOperationConflict) {
		t.Fatal("operation reused", e)
	}
}
func TestFighterEvidenceStoppedAdmissionAndCrossChildReservation(t *testing.T) {
	s, h := evidenceLaneFixture(t)
	selection, path := evidenceSelection(t)
	id := strings.Repeat("b", 64)
	s.mining = true
	if _, e := s.BeginFighterEvidence(t.Context(), s.view.ViewID, h.JobID, h.Capture.HistoryID, id, selection.Pin()); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal("active run admitted", e)
	}
	s.mining = false
	var parent EvidencePermit
	s.stableConfig.EvidencePermit = &parent
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	s.evidenceCheckpoint = func(stage string) {
		if stage == "before-source" {
			close(entered)
			<-release
		}
	}
	j, e := s.BeginFighterEvidence(t.Context(), s.view.ViewID, h.JobID, h.Capture.HistoryID, id, selection.Pin())
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("source not entered")
	}
	if parent.reserve(new(int)) {
		t.Fatal("successor child bypassed parent reservation")
	}
	if e = s.Start(); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal("Start admitted during export", e)
	}
	if _, e = s.BeginStableSync(t.Context(), s.view.ViewID); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal("lane queued during export", e)
	}
	cancelled, e := s.CancelFighterEvidence(t.Context(), s.view.ViewID, j.JobID)
	if e != nil || cancelled.State != "running" {
		t.Fatal("cancel falsely joined", cancelled, e)
	}
	if parent.reserve(new(int)) {
		t.Fatal("cancel released entered work")
	}
	releaseOnce.Do(func() { close(release) })
	j = evidenceWait(t, s, id)
	if j.State != "cancelled" {
		t.Fatal(j.State, j.Error)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("cancel wrote final file", e)
	}
	owner := new(int)
	if !parent.reserve(owner) {
		t.Fatal("joined task retained reservation")
	}
	parent.release(owner, nil)
}
func TestFighterEvidenceRevokedViewAfterSourceCannotPublish(t *testing.T) {
	s, h := evidenceLaneFixture(t)
	selection, path := evidenceSelection(t)
	original := s.stableConfig.Lease
	var revoked atomic.Bool
	s.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		if revoked.Load() {
			return gymaccount.ErrUnauthorized
		}
		return original(ctx, gymaccount.View, fn)
	}
	s.evidenceCheckpoint = func(stage string) {
		if stage == "after-source" {
			revoked.Store(true)
		}
	}
	id := strings.Repeat("c", 64)
	if _, e := s.BeginFighterEvidence(t.Context(), s.view.ViewID, h.JobID, h.Capture.HistoryID, id, selection.Pin()); e != nil {
		t.Fatal(e)
	}
	s.stableMu.Lock()
	op := s.evidenceOp
	s.stableMu.Unlock()
	if op != nil {
		select {
		case <-op.done:
		case <-time.After(20 * time.Second):
			t.Fatal("revoked export did not join")
		}
	}
	if j, e := s.FighterEvidenceJob(t.Context(), s.view.ViewID, id); e == nil || j.Version != 0 {
		t.Fatal("revoked payload disclosed", e)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("revoked file published", e)
	}
}
func TestFighterEvidenceLostAdmissionAckRecoversKnownID(t *testing.T) {
	s, h := evidenceLaneFixture(t)
	selection, _ := evidenceSelection(t)
	original := s.stableConfig.Lease
	var once atomic.Bool
	s.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		e := original(ctx, gymaccount.View, fn)
		s.stableMu.Lock()
		admitted := s.evidenceOp != nil && s.evidenceOp.admitted
		s.stableMu.Unlock()
		if e == nil && admitted && once.CompareAndSwap(false, true) {
			return context.Canceled
		}
		return e
	}
	id := strings.Repeat("d", 64)
	if _, e := s.BeginFighterEvidence(t.Context(), s.view.ViewID, h.JobID, h.Capture.HistoryID, id, selection.Pin()); !errors.Is(e, ErrEvidenceReplyUnknown) {
		t.Fatal("lost acknowledgement certainty", e)
	}
	j := evidenceWait(t, s, id)
	if j.State != "done" {
		t.Fatal(j.State, j.Error)
	}
}

func TestFighterEvidencePoolStopJoinsDirectLaneAndPermit(t *testing.T) {
	s, h := evidenceLaneFixture(t)
	selection, path := evidenceSelection(t)
	var permit EvidencePermit
	s.stableConfig.EvidencePermit = &permit
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	s.evidenceCheckpoint = func(stage string) {
		if stage == "before-source" {
			close(entered)
			<-release
		}
	}
	id := strings.Repeat("e", 64)
	if _, err := s.BeginFighterEvidence(t.Context(), s.view.ViewID, h.JobID, h.Capture.HistoryID, id, selection.Pin()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("source not entered")
	}
	child := &PoolChild{stable: s}
	stopped := make(chan struct{})
	go func() { child.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop returned before direct lane joined")
	case <-time.After(50 * time.Millisecond):
	}
	if permit.reserve(new(int)) {
		t.Fatal("Stop released entered export reservation")
	}
	once.Do(func() { close(release) })
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop failed to join")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stopped export published", err)
	}
	owner := new(int)
	if !permit.reserve(owner) {
		t.Fatal("joined Stop retained reservation")
	}
	permit.release(owner, nil)
}

func TestFighterEvidencePoolBackupDrainsDirectLane(t *testing.T) {
	s, h := evidenceLaneFixture(t)
	selection, path := evidenceSelection(t)
	var permit EvidencePermit
	s.stableConfig.EvidencePermit = &permit
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	s.evidenceCheckpoint = func(stage string) {
		if stage == "before-source" {
			close(entered)
			<-release
		}
	}
	if _, e := s.BeginFighterEvidence(t.Context(), s.view.ViewID, h.JobID, h.Capture.HistoryID, strings.Repeat("f", 64), selection.Pin()); e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("source not entered")
	}
	child := &PoolChild{stable: s, account: s.stableConfig, ctx: t.Context(), prepared: true, snapshot: BlankPoolSnapshot()}
	type result struct {
		op  *poolBackupOperation
		err error
	}
	ready := make(chan result, 1)
	go func() { op, e := child.beginPoolBackup(t.Context(), true); ready <- result{op, e} }()
	select {
	case v := <-ready:
		t.Fatal("backup passed export drain early", v.err)
	case <-time.After(50 * time.Millisecond):
	}
	if permit.reserve(new(int)) {
		t.Fatal("backup released live export reservation")
	}
	once.Do(func() { close(release) })
	var v result
	select {
	case v = <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("backup drain deadlock")
	}
	if v.err != nil || v.op == nil {
		t.Fatal(v.err)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("backup-cancelled export published", e)
	}
	v.op.cancel()
	child.finishPoolBackup(v.op, "")
	select {
	case <-v.op.done:
	default:
		t.Fatal("backup cleanup did not finish")
	}
}
