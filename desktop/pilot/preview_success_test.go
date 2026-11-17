//go:build !slugtestpow

package pilot

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// These extra real normal-PoW proofs are behavioral public-fixture input only.
// No output generated here becomes an expected transcript or crypto golden.
func previewLanePairFixture(t *testing.T, counts ...int) (*StableController, [2]string) {
	t.Helper()
	s, records := stableLaneFixture(t)
	pin := s.stableStatus.Pin
	raw, e := os.ReadFile("../../gymaccount/testdata/enrollment_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var keys struct {
		Cases []struct {
			Private       string `json:"test_only_private_key"`
			Authorization gymauth.Authorization
		}
	}
	if e = json.Unmarshal(raw, &keys); e != nil {
		t.Fatal(e)
	}
	var scalar []byte
	for _, k := range keys.Cases {
		if k.Authorization.PublicKey == pin.Identity.GymPublicKey {
			scalar, _ = hex.DecodeString(k.Private)
		}
	}
	if len(scalar) != 32 {
		t.Fatal("published fixture key missing")
	}
	key, _ := btcec.PrivKeyFromBytes(scalar)
	defer key.Zero()
	base := records[0]
	count := 6
	if len(counts) > 0 {
		count = counts[0]
	}
	for len(records) < count {
		r := base
		r.Sequence = uint64(len(records) + 1)
		header, _ := hex.DecodeString(r.Proof.Header)
		binary.LittleEndian.PutUint32(header[76:], uint32(50000+r.Sequence))
		r.Proof.Header = hex.EncodeToString(header)
		r.Verified, e = prospect.Verify(r.Context, r.Proof)
		if e != nil {
			t.Fatal(e)
		}
		r.Authorization, e = gymauth.SignAdmission(r.Context, r.Proof, key)
		if e != nil {
			t.Fatal(e)
		}
		m, e := fightermanifest.GenerateText(1, r.Verified.WorkHash)
		if e != nil {
			t.Fatal(e)
		}
		r.StaticBinding.ManifestDigest, e = fightermanifest.Digest(m)
		if e != nil {
			t.Fatal(e)
		}
		r.StaticBinding.PortraitDigest, e = fighterportrait.Digest(1, m)
		if e != nil {
			t.Fatal(e)
		}
		r.Eligibility = registry.Eligibility{RuleVersion: registry.EligibilityRuleVersion2, Source: "current-parent", Hash: r.Context.ParentHash, Height: r.Context.ParentHeight}
		records = append(records, r)
	}
	if e = s.closePair(); e != nil {
		t.Fatal(e)
	}
	sourceRows(t, filepath.Join(s.config.DataDir, "manifest-authority"), pin, records)
	if e = s.openPair(nil); e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(context.Background()); e != nil {
		t.Fatal(e)
	}
	for i, a := range s.last.Sync.Items {
		for _, b := range s.last.Sync.Items[i+1:] {
			if a.Manifest.Physical.Class == b.Manifest.Physical.Class {
				return s, [2]string{a.Assignment.ProspectID, b.Assignment.ProspectID}
			}
		}
	}
	t.Fatal("six genuine proofs must have same-class pair")
	return nil, [2]string{}
}
func TestPreviewLaneSuccessReplayAndRevocation(t *testing.T) {
	s, ids := previewLanePairFixture(t)
	ctx := context.Background()
	before := *s.stableStatus
	cache := s.cache
	s.stableProtected = true
	counts := map[string]int{}
	s.previewCheckpoint = func(stage string) { counts[stage]++ }
	ack, e := s.BeginFightPreview(ctx, s.view.ViewID, ids[0], ids[1])
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	done, e := s.FightPreviewJob(ctx, s.view.ViewID, ack.JobID)
	if e != nil || done.State != "done" || done.Result == nil {
		t.Fatalf("preview result %+v %v", done, e)
	}
	raw, e := MarshalPreviewResponse(PreviewResponse{ID: 1, Version: 1, OK: true, Job: &done})
	if e != nil {
		t.Fatal("completed codec", e)
	}
	if _, e = ParsePreviewResponse(raw); e != nil {
		t.Fatal(e)
	}
	if counts["pair-read"] != 1 || counts["before-engine"] != 1 || counts["before-validation"] != 1 || counts["before-publication"] != 1 {
		t.Fatal("unexpected execution", counts)
	}
	if done.Result.Pair.Fighters[0].Receipt().Version != 0 || done.Result.Pair.Fighters[1].Receipt().Version != 0 {
		t.Fatal("private evidence retained")
	}
	after := *s.stableStatus
	after.LastAsOfMS = before.LastAsOfMS
	if after != before || !s.stableProtected || len(s.cache) != len(cache) {
		t.Fatal("preview changed stable/page")
	}
	done.Result.EngineResult.Rounds[0].Ticks[0].Strike = "mutated"
	for i := 0; i < 3; i++ {
		replay, e := s.FightPreviewJob(ctx, s.view.ViewID, ack.JobID)
		if e != nil || replay.Result.EngineResult.Rounds[0].Ticks[0].Strike == "mutated" {
			t.Fatal("replay ownership", e)
		}
	}
	if counts["pair-read"] != 1 || counts["before-engine"] != 1 {
		t.Fatal("poll caused work")
	}
	original := s.stableConfig.Lease
	s.stableConfig.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		return gymaccount.ErrUnauthorized
	}
	if got, e := s.FightPreviewJob(ctx, s.view.ViewID, ack.JobID); e == nil || got.Version != 0 {
		t.Fatal("revoked cached result disclosed")
	}
	if s.PreviewSnapshot().View != nil {
		t.Fatal("revoked snapshot disclosed")
	}
	s.stableConfig.Lease = original
}
func TestPreviewLaneCancelAfterPairAndAggregateQueue(t *testing.T) {
	s, ids := previewLanePairFixture(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	var engine atomic.Int32
	s.previewCheckpoint = func(stage string) {
		if stage == "pair-read" {
			close(entered)
			<-release
		}
		if stage == "before-engine" {
			engine.Add(1)
		}
	}
	ack, e := s.BeginFightPreview(ctx, s.view.ViewID, ids[0], ids[1])
	if e != nil {
		t.Fatal(e)
	}
	finished := make(chan error, 1)
	go func() { finished <- s.cycle(ctx) }()
	<-entered
	queued, e := s.BeginStableInventory(ctx, s.view.ViewID, 0, 1)
	if e != nil {
		t.Fatal("active preview+queued stable", e)
	}
	if _, e = s.BeginFightPreview(ctx, s.view.ViewID, ids[0], ids[1]); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal("second preview", e)
	}
	if _, e = s.CancelFightPreview(ctx, s.view.ViewID, ack.JobID); e != nil {
		t.Fatal(e)
	}
	close(release)
	if e = <-finished; e != nil {
		t.Fatal(e)
	}
	done, e := s.FightPreviewJob(ctx, s.view.ViewID, ack.JobID)
	if e != nil || done.State != "cancelled" || engine.Load() != 0 {
		t.Fatal("cancel before engine", done.State, e, engine.Load())
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	if j, e := s.StableJob(ctx, s.view.ViewID, queued.JobID); e != nil || j.State != "done" {
		t.Fatal("queued stable lost", e)
	}
}

func TestPreviewStableActiveThenQueuedPreviewPreservesStableResult(t *testing.T) {
	s, ids := previewLanePairFixture(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	s.laneCheckpoint = func(kind string) {
		if kind == "inventory" {
			close(entered)
			<-release
		}
	}
	stable, e := s.BeginStableInventory(ctx, s.view.ViewID, 0, 1)
	if e != nil {
		t.Fatal(e)
	}
	finished := make(chan error, 1)
	go func() { finished <- s.cycle(ctx) }()
	<-entered
	preview, e := s.BeginFightPreview(ctx, s.view.ViewID, ids[0], ids[1])
	if e != nil {
		t.Fatal("active stable + preview queue", e)
	}
	snap := s.StableSnapshot()
	ps := s.PreviewSnapshot()
	if snap.ActiveJobID != stable.JobID || snap.QueuedJobID != "" || ps.QueuedJobID != preview.JobID || ps.ActiveJobID != "" {
		t.Fatal("family IDs mixed")
	}
	close(release)
	if e = <-finished; e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	if got, e := s.StableJob(ctx, s.view.ViewID, stable.JobID); e != nil || got.State != "done" || got.Page == nil {
		t.Fatal("stable result overwritten", e)
	}
	if got, e := s.FightPreviewJob(ctx, s.view.ViewID, preview.JobID); e != nil || got.State != "done" {
		t.Fatal("queued preview lost", e)
	}
}
func TestPreviewFinalLeaseLossWithholdsCompletedComputation(t *testing.T) {
	s, ids := previewLanePairFixture(t)
	ctx := context.Background()
	original := s.stableConfig.Lease
	var revoked atomic.Bool
	s.stableConfig.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if revoked.Load() {
			return gymaccount.ErrUnauthorized
		}
		return original(ctx, cap, fn)
	}
	s.previewCheckpoint = func(stage string) {
		if stage == "before-publication" {
			revoked.Store(true)
		}
	}
	ack, e := s.BeginFightPreview(ctx, s.view.ViewID, ids[0], ids[1])
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	if s.previewLast == nil || s.previewLast.State != "failed" || s.previewLast.Result != nil {
		t.Fatal("late computation published")
	}
	if got, e := s.FightPreviewJob(ctx, s.view.ViewID, ack.JobID); e == nil || got.Version != 0 {
		t.Fatal("revoked cache disclosure")
	}
	if s.PreviewSnapshot().View != nil {
		t.Fatal("revoked snapshot disclosure")
	}
}

func TestPreviewFatalClosedDependencyStopsLane(t *testing.T) {
	s, ids := previewLanePairFixture(t)
	ctx := context.Background()
	ack, e := s.BeginFightPreview(ctx, s.view.ViewID, ids[0], ids[1])
	if e != nil {
		t.Fatal(e)
	}
	if e = s.stableStore.Close(); e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); !errors.Is(e, gymstable.ErrClosed) {
		t.Fatal("closed dependency swallowed", e)
	}
	if j, e := s.FightPreviewJob(ctx, s.view.ViewID, ack.JobID); e != nil || j.State != "failed" || j.Error != "preview_unavailable" {
		t.Fatal("fatal projection", j, e)
	}
	if s.stableState != "failed" || s.needSync {
		t.Fatal("fatal lane continued")
	}
	if previewProtocolError(gymstable.ErrOutcomeUnknown) != "preview_observation_unknown" {
		t.Fatal("observation incorrectly treated as roster mutation")
	}
}

func TestPreviewClosedDependencyErrorPrecedence(t *testing.T) {
	s, _ := stableLaneFixture(t)
	if e := s.stableStore.Close(); e != nil {
		t.Fatal(e)
	}
	// A completed Close is a failed dependency, not cancellation of this live
	// request. Exercise repeated entry to expose either branch of begin's select.
	for i := 0; i < 2048; i++ {
		if _, e := s.stableStore.Status(context.Background(), s.session()); !errors.Is(e, gymstable.ErrClosed) {
			t.Fatalf("completed Close classified as %v on request %d", e, i)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.stableStore.Status(ctx, s.session()); !errors.Is(e, gymstable.ErrClosed) {
		t.Fatal("completed Close lost precedence to caller cancellation", e)
	}
}

func TestPreviewLiveDependencyCallerCancellation(t *testing.T) {
	s, _ := stableLaneFixture(t)
	holder := s.session()
	original := holder.Lease
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	holder.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		once.Do(func() { close(entered); <-release })
		return original(ctx, cap, fn)
	}
	done := make(chan error, 1)
	go func() { _, e := s.stableStore.Status(context.Background(), holder); done <- e }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.stableStore.Status(ctx, s.session()); !errors.Is(e, gymstable.ErrCancelled) {
		close(release)
		<-done
		t.Fatal("live dependency changed caller cancellation", e)
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal("unrelated live gate holder failed", e)
	}
	if _, e := s.stableStore.Status(context.Background(), s.session()); e != nil {
		t.Fatal("cancelled request retained the resource gate or poisoned the store", e)
	}
}

func TestPreviewClosingDependencyDoesNotEnterLease(t *testing.T) {
	s, _ := stableLaneFixture(t)
	holder := s.session()
	original := holder.Lease
	entered, closing, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	holder.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		once.Do(func() {
			close(entered)
			<-ctx.Done()
			close(closing)
			<-release
		})
		return original(ctx, cap, fn)
	}
	done := make(chan error, 1)
	go func() { _, e := s.stableStore.Status(context.Background(), holder); done <- e }()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- s.stableStore.Close() }()
	<-closing // Close cancelled the lifetime but cannot finish before the holder.
	probe := s.session()
	probe.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
		t.Error("new request entered a closing dependency lease")
		return nil
	}
	_, e := s.stableStore.Status(context.Background(), probe)
	close(release)
	if holderErr := <-done; !errors.Is(holderErr, gymstable.ErrCancelled) {
		t.Fatal("in-progress request cancellation changed", holderErr)
	}
	if closeErr := <-closed; closeErr != nil {
		t.Fatal(closeErr)
	}
	if !errors.Is(e, gymstable.ErrClosed) {
		t.Fatal("closing dependency did not fail before acquiring the gate", e)
	}
}
func TestPreviewStopDrainsBeforeSourceClose(t *testing.T) {
	s, ids := previewLanePairFixture(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	var engine atomic.Int32
	s.previewCheckpoint = func(stage string) {
		if stage == "pair-read" {
			close(entered)
			<-release
		}
		if stage == "before-engine" {
			engine.Add(1)
		}
	}
	ack, e := s.BeginFightPreview(ctx, s.view.ViewID, ids[0], ids[1])
	if e != nil {
		t.Fatal(e)
	}
	cycleDone := make(chan error, 1)
	go func() { cycleDone <- s.cycle(ctx) }()
	<-entered
	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop escaped active lane")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if e = <-cycleDone; e != nil {
		t.Fatal(e)
	}
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop failed to join")
	}
	if engine.Load() != 0 {
		t.Fatal("engine after Stop")
	}
	if got, e := s.FightPreviewJob(ctx, s.view.ViewID, ack.JobID); e != nil || got.State != "cancelled" {
		t.Fatal("cancel completion", e)
	}
}

func TestPreviewDoesNotScheduleImplicitCatchup(t *testing.T) {
	s, ids := previewLanePairFixture(t, 9)
	ctx := context.Background()
	if s.stableStatus.Checkpoint != 8 || s.stableStatus.ObservedSourceCount != 9 {
		t.Fatal("fixture backlog")
	}
	s.needSync = false
	s.stableState = "paused"
	ack, e := s.BeginFightPreview(ctx, s.view.ViewID, ids[0], ids[1])
	if e != nil {
		t.Fatal(e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	if got, e := s.FightPreviewJob(ctx, s.view.ViewID, ack.JobID); e != nil || got.State != "done" {
		t.Fatal("preview backlog", e)
	}
	if s.needSync {
		t.Fatal("preview scheduled implicit assignment catch-up")
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	if s.stableStatus.Checkpoint != 8 || s.stableStatus.ObservedSourceCount != 9 || s.stableStatus.CaughtUp {
		t.Fatal("preview consumed source backlog")
	}
	snap := s.StableSnapshot()
	if !stableSnapshotValid(snap) {
		t.Fatal("backlog cached status dishonest", snap)
	}
}
