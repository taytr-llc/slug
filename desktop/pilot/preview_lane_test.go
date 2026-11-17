//go:build !slugtestpow

package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"strings"
	"testing"
)

func TestPreviewLaneQueueAndClassRefusalWithoutEngine(t *testing.T) {
	s, records := stableLaneFixture(t)
	ctx := context.Background()
	if e := s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	a, b := records[0].Verified.ProspectID, records[1].Verified.ProspectID
	old := s.StableSnapshot().LastJobID
	ack, e := s.BeginFightPreview(ctx, s.view.ViewID, a, b)
	if e != nil {
		t.Fatal(e)
	}
	if s.StableSnapshot().QueuedJobID != "" || s.StableSnapshot().LastJobID != old {
		t.Fatal("preview leaked old family")
	}
	if _, e = s.BeginFightPreview(ctx, s.view.ViewID, a, b); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal("second preview", e)
	}
	if _, e = s.BeginStableSync(ctx, s.view.ViewID); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal("aggregate queue", e)
	}
	if _, e = s.StableJob(ctx, s.view.ViewID, ack.JobID); !errors.Is(e, gymstable.ErrNotFound) {
		t.Fatal("wrong family lookup", e)
	}
	if e = s.cycle(ctx); e != nil {
		t.Fatal(e)
	}
	job, e := s.FightPreviewJob(ctx, s.view.ViewID, ack.JobID)
	if e != nil || job.State != "failed" || job.Error != "preview_class_mismatch" || job.Result != nil {
		t.Fatalf("class-only path %+v %v", job, e)
	}
	if _, e = MarshalPreviewResponse(PreviewResponse{ID: 1, Version: 1, OK: true, Job: &job}); e != nil {
		t.Fatal("job codec", e)
	}
	snap := s.PreviewSnapshot()
	if _, e = MarshalPreviewResponse(PreviewResponse{ID: 1, Version: 1, OK: true, Snapshot: &snap}); e != nil {
		t.Fatal("snapshot codec", e)
	}
	next, e := s.BeginFightPreview(ctx, s.view.ViewID, a, b)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.FightPreviewJob(ctx, s.view.ViewID, ack.JobID); !errors.Is(e, gymstable.ErrNotFound) {
		t.Fatal("old terminal not evicted", e)
	}
	cancel, e := s.CancelFightPreview(ctx, s.view.ViewID, next.JobID)
	if e != nil || cancel.State != "cancelled" {
		t.Fatal(cancel, e)
	}
	if s.queued != nil || s.previewBytes >= previewAllowance {
		t.Fatal("cancel reservation")
	}
}
func TestPreviewFailedPublicationPreservesTerminalAndCannotRun(t *testing.T) {
	s, r := stableLaneFixture(t)
	s.needSync = false
	v := s.view.ViewID
	old := PreviewJob{Version: 1, JobID: strings.Repeat("9", 64), State: "cancelled", View: s.viewPin(), StoreID: s.stableStatus.StoreID, EngineVersion: 1, EngineID: previewEngineID, FighterAID: strings.Repeat("1", 64), FighterBID: strings.Repeat("2", 64), Error: "preview_cancelled"}
	s.retainPreviewLocked(old)
	beforeBytes := s.previewBytes
	original := s.stableConfig.Lease
	entered, release := make(chan struct{}), make(chan struct{})
	s.stableConfig.Lease = func(ctx context.Context, c gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if e := original(ctx, c, fn); e != nil {
			return e
		}
		close(entered)
		<-release
		return gymaccount.ErrUnauthorized
	}
	done := make(chan error, 1)
	go func() {
		_, e := s.BeginFightPreview(context.Background(), v, r[0].Verified.ProspectID, r[1].Verified.ProspectID)
		done <- e
	}()
	<-entered
	if e := s.cycle(context.Background()); e != nil {
		t.Fatal(e)
	}
	if s.active != nil || s.previewLast.JobID != old.JobID {
		t.Fatal("unpublished task ran")
	}
	close(release)
	if e := <-done; e == nil {
		t.Fatal("failed lease accepted")
	}
	s.stableConfig.Lease = original
	if s.queued != nil || s.previewLast.JobID != old.JobID || s.previewBytes != beforeBytes {
		t.Fatal("failed publication lost old result/reservation")
	}
}
func TestPreviewReservationProtectedCache(t *testing.T) {
	s, r := stableLaneFixture(t)
	s.needSync = false
	s.cache[1] = &manifestCached{manifestBytes: MaxManifestCacheBytes - 140000}
	s.stableProtected = true
	_, e := s.BeginFightPreview(context.Background(), s.view.ViewID, r[0].Verified.ProspectID, r[1].Verified.ProspectID)
	if !errors.Is(e, gymstable.ErrCapacity) || s.queued != nil || len(s.cache) != 1 || !s.stableProtected {
		t.Fatal("capacity mutated protected page", e)
	}
}
