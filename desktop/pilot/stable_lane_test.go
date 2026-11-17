//go:build !slugtestpow

package pilot

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sourceRows(t *testing.T, path string, pin gymstable.Pin, records []registry.ManifestRecord) {
	t.Helper()
	if e := os.MkdirAll(filepath.Join(path, "db"), 0700); e != nil {
		t.Fatal(e)
	}
	descriptor, e := fighterdescriptor.ResolveCandidate(pin.Source.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	marker := struct {
		Application  string                       `json:"application"`
		Version      uint32                       `json:"version"`
		Network      uint32                       `json:"network"`
		Genesis      string                       `json:"genesis"`
		AuthorityID  string                       `json:"authority_id"`
		DescriptorID string                       `json:"descriptor_id"`
		Descriptor   fighterdescriptor.Descriptor `json:"descriptor"`
	}{"slug-prospect-registry", 5, pin.Source.Network, pin.Source.Genesis, pin.Source.AuthorityID, pin.Source.DescriptorID, descriptor}
	raw, _ := json.Marshal(marker)
	if e = os.WriteFile(filepath.Join(path, "SLUG-REGISTRY.json"), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	b := new(leveldb.Batch)
	jobs := map[string]bool{}
	var clock int64
	for _, r := range records {
		raw, _ = json.Marshal(r)
		b.Put([]byte("record:"+r.Verified.ProspectID), raw)
		k := append([]byte("sequence:"), make([]byte, 8)...)
		binary.BigEndian.PutUint64(k[len("sequence:"):], r.Sequence)
		b.Put(k, []byte(r.Verified.ProspectID))
		raw, _ = json.Marshal(r.Job)
		b.Put([]byte("job:"+r.Job.ID), raw)
		jobs[r.Job.ID] = true
		clock = max(clock, r.Birth.AcceptedAtMS)
	}
	// Structural fixture assembly uses the immutable independent records as input.
	// No new proof/receipt expectation is produced by the implementation under test.
	meta := struct {
		Version      uint32                       `json:"version"`
		Network      uint32                       `json:"network"`
		Genesis      string                       `json:"genesis"`
		AuthorityID  string                       `json:"authority_id"`
		DescriptorID string                       `json:"descriptor_id"`
		Descriptor   fighterdescriptor.Descriptor `json:"descriptor"`
		LastSequence uint64                       `json:"last_sequence"`
		JobCount     uint64                       `json:"job_count"`
		LastClockMS  int64                        `json:"last_clock_ms"`
	}{5, pin.Source.Network, pin.Source.Genesis, pin.Source.AuthorityID, pin.Source.DescriptorID, descriptor, uint64(len(records)), uint64(len(jobs)), clock}
	raw, _ = json.Marshal(meta)
	b.Put([]byte("meta"), raw)
	if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
}
func stableLaneFixture(t *testing.T) (*StableController, []registry.ManifestRecord) {
	t.Helper()
	var corpus struct {
		Pins []struct {
			Name string
			Pin  gymstable.Pin
		}
		Cases []struct {
			Fixture string
			Record  registry.ManifestRecord
		}
	}
	raw, err := os.ReadFile("../../gymstable/testdata/reference/stable_v1.json")
	if err != nil || json.Unmarshal(raw, &corpus) != nil {
		t.Fatal(err)
	}
	var pin gymstable.Pin
	for _, p := range corpus.Pins {
		if p.Name == "zero256" {
			pin = p.Pin
		}
	}
	records := []registry.ManifestRecord{}
	for _, v := range corpus.Cases {
		if v.Fixture == "zero256" {
			records = append(records, v.Record)
		}
	}
	root := t.TempDir()
	sourcePath := filepath.Join(root, "manifest-authority")
	sourceRows(t, sourcePath, pin, records)
	source, err := registry.OpenManifestAuthority(sourcePath, pin.Source, true)
	if err != nil {
		t.Fatal(err)
	}
	st, err := gymstable.Create(filepath.Join(root, "stable"), pin, source)
	if err != nil {
		t.Fatal(err)
	}
	a := manifestTestAccount()
	a.Account.AccountID = pin.AccountID
	a.Account.AuthorityID = pin.AccountAuthorityID
	lease := func(ctx context.Context, _ gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		id := pin.Identity
		return fn(gymaccount.Account{ID: pin.AccountID, Generation: 1, Enrollment: &id}, func() error { return ctx.Err() })
	}
	a.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return lease(ctx, gymaccount.View, fn)
	}
	base := &ManifestController{config: Config{DataDir: root}, account: &a.Account, view: a, cache: map[uint64]*manifestCached{}, snapshot: BlankManifestSnapshot(), manifest: manifestIdentity{Version: 1, Mode: ManifestMode, Phase: "ready", AccountAuthorityID: pin.AccountAuthorityID, AccountID: pin.AccountID, Fixture: "zero256", Network: pin.Source.Network, Genesis: pin.Source.Genesis, ActivationHeight: 62, RecoveryProfile: pin.Identity.Profile, DescriptorID: pin.Source.DescriptorID, Payout: walletPayout{Address: "aiEhKC3LRX8PPG5haqRFUcFvj3pXpBts2P", Purpose: 44}, AuthorityPort: 37619, AuthorityPin: pin.Source, Identity: walletIdentity{RootPublicKey: pin.Identity.RootPublicKey, GymPublicKey: pin.Identity.GymPublicKey, GymID: pin.Identity.GymID}}}
	base.manifest.TransportTrustID, err = prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: base.manifest.origin(), Token: strings.Repeat("a", 64), AuthorityPin: pin.Source})
	if err != nil {
		t.Fatal(err)
	}
	s := &StableController{ManifestController: base, stableConfig: StableAccountConfig{Manifest: a, Lease: lease}, laneGate: make(chan struct{}, 1), wake: make(chan struct{}, 1), stableStore: st, source: source, ownedSource: true, installed: true, mining: true, stableState: "syncing", needSync: true}
	status, err := st.Status(context.Background(), s.session())
	if err != nil {
		t.Fatal(err)
	}
	s.stableStatus = &status
	base.stable = s
	t.Cleanup(func() {
		s.stopStoppedLoop()
		s.laneGate <- struct{}{}
		if err := s.closePair(); err != nil {
			t.Error(err)
		}
		<-s.laneGate
	})
	return s, records
}
func assertStableResponse(t *testing.T, r StableResponse) {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseStableResponse(raw); err != nil {
		t.Fatalf("invalid produced response %s: %v", raw, err)
	}
}
func TestStableLaneAutomaticDiscoveryPrepareApplyAndPrivateCache(t *testing.T) {
	s, records := stableLaneFixture(t)
	ctx := context.Background()
	if err := s.cycle(ctx); err != nil {
		t.Fatal(err)
	}
	snap := s.StableSnapshot()
	if snap.LastJobID == "" || snap.Status == nil || snap.Status.Checkpoint != uint64(len(records)) {
		t.Fatalf("undiscoverable auto result %+v", snap)
	}
	assertStableResponse(t, StableResponse{ID: 1, OK: true, Snapshot: &snap})
	job, err := s.StableJob(ctx, s.view.ViewID, snap.LastJobID)
	if err != nil || job.Sync == nil {
		t.Fatal(err)
	}
	assertStableResponse(t, StableResponse{ID: 1, OK: true, Job: &job})
	if job.Sync.Items[0].Receipt().Version != 0 {
		t.Fatal("public completed job retained raw proof")
	}
	request := gymstable.SlotRequest{Slot: 1, ProspectID: records[0].Verified.ProspectID}
	ack, err := s.BeginStablePrepare(ctx, s.view.ViewID, request)
	if err != nil {
		t.Fatal(err)
	}
	assertStableResponse(t, StableResponse{ID: 1, OK: true, Job: &ack})
	if err = s.cycle(ctx); err != nil {
		t.Fatal(err)
	}
	done, err := s.StableJob(ctx, s.view.ViewID, ack.JobID)
	if err != nil || done.Intent == nil {
		t.Fatalf("prepare %+v %v", done, err)
	}
	snap = s.StableSnapshot()
	assertStableResponse(t, StableResponse{ID: 1, OK: true, Snapshot: &snap})
	apply, err := s.BeginStableApply(ctx, s.view.ViewID, done.Intent.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.cycle(ctx); err != nil {
		t.Fatal(err)
	}
	done, err = s.StableJob(ctx, s.view.ViewID, apply.JobID)
	if err != nil || done.Mutation == nil || done.Mutation.Receipt.Revision != 1 {
		t.Fatalf("apply %+v %v", done, err)
	}
	assertStableResponse(t, StableResponse{ID: 1, OK: true, Job: &done})
	inv, err := s.BeginStableInventory(ctx, s.view.ViewID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.cycle(ctx)
	done, err = s.StableJob(ctx, s.view.ViewID, inv.JobID)
	if err != nil || done.Page == nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	protected := s.stableProtected
	cached := len(s.cache)
	s.mu.Unlock()
	if !protected || cached != 1 {
		t.Fatal("selected evidence not pinned")
	}
	s.stableMu.Lock()
	s.needSync = true
	s.stableMu.Unlock()
	s.cycle(ctx)
	s.mu.Lock()
	cached = len(s.cache)
	s.mu.Unlock()
	if cached != 1 {
		t.Fatal("auto arrival replaced selected evidence")
	}
}
func TestStableLaneQueueCancellationAndMetadataBudget(t *testing.T) {
	s, _ := stableLaneFixture(t)
	ctx := context.Background()
	s.stableMu.Lock()
	s.needSync = false
	s.stableMu.Unlock()
	ack, err := s.BeginStableSync(ctx, s.view.ViewID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginStableSync(ctx, s.view.ViewID); !errors.Is(err, gymstable.ErrBusy) {
		t.Fatal("queue unbounded", err)
	}
	canceled, err := s.CancelStableJob(ctx, s.view.ViewID, ack.JobID)
	if err != nil || canceled.State != "cancelled" {
		t.Fatal(err)
	}
	s.cycle(ctx)
	if s.stableStatus.Checkpoint != 0 {
		t.Fatal("cancelled work executed")
	}
	s.laneGate <- struct{}{}
	start := time.Now()
	_, err = s.StablePending(ctx, s.view.ViewID)
	if !errors.Is(err, gymstable.ErrBusy) || time.Since(start) > 100*time.Millisecond {
		t.Fatal("metadata waited on hash lane", err)
	}
	<-s.laneGate
	var calls atomic.Int32
	s.stableConfig.Lease = func(ctx context.Context, _ gymaccount.Capability, _ func(gymaccount.Account, func() error) error) error {
		calls.Add(1)
		<-ctx.Done()
		return ctx.Err()
	}
	start = time.Now()
	snap := s.StableSnapshot()
	if snap.View != nil || time.Since(start) > time.Second || calls.Load() != 1 {
		t.Fatal("metadata account acquisition not bounded")
	}
}

func TestStableLaneFailedQueuePublicationNeverExecutes(t *testing.T) {
	s, _ := stableLaneFixture(t)
	s.stableMu.Lock()
	s.needSync = false
	s.stableMu.Unlock()
	original := s.stableConfig.Lease
	entered, release := make(chan struct{}), make(chan struct{})
	s.stableConfig.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if err := original(ctx, cap, fn); err != nil {
			return err
		}
		close(entered)
		<-release
		return gymaccount.ErrUnauthorized
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.BeginStablePrepare(context.Background(), s.view.ViewID, gymstable.SlotRequest{Slot: 1})
		done <- err
	}()
	<-entered
	if err := s.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.stableStatus.OperationCount != 0 {
		t.Fatal("unacknowledged queued mutation ran")
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("failed final lease accepted")
	}
	s.stableConfig.Lease = original
	s.cycle(context.Background())
	if s.stableStatus.OperationCount != 0 || s.queued != nil {
		t.Fatal("failed publication left runnable task")
	}
}
func TestStableLaneHistoricalAbandonKeepsNewPending(t *testing.T) {
	s, _ := stableLaneFixture(t)
	ctx := context.Background()
	s.cycle(ctx)
	a, err := s.BeginStablePrepare(ctx, s.view.ViewID, gymstable.SlotRequest{Slot: 1})
	if err != nil {
		t.Fatal(err)
	}
	s.cycle(ctx)
	j, _ := s.StableJob(ctx, s.view.ViewID, a.JobID)
	old := j.Intent.OperationID
	_, err = s.AbandonStableOperation(ctx, s.view.ViewID, old)
	if err != nil {
		t.Fatal(err)
	}
	a, err = s.BeginStablePrepare(ctx, s.view.ViewID, gymstable.SlotRequest{Slot: 2})
	if err != nil {
		t.Fatal(err)
	}
	s.cycle(ctx)
	j, _ = s.StableJob(ctx, s.view.ViewID, a.JobID)
	newID := j.Intent.OperationID
	if _, err = s.AbandonStableOperation(ctx, s.view.ViewID, old); err != nil {
		t.Fatal(err)
	}
	snap := s.StableSnapshot()
	if snap.PendingOperationID != newID || snap.Status.LastOperationID != newID {
		t.Fatal("historical abandon erased current pending")
	}
	assertStableResponse(t, StableResponse{ID: 1, OK: true, Snapshot: &snap})
	s.Stop()
	p, err := s.StablePending(ctx, s.view.ViewID)
	if err != nil || p.Operation == nil || p.Operation.Intent.OperationID != newID {
		t.Fatal("stopped pending unavailable", err)
	}
}
func TestStableLaneExplicitRecentUnprotectsEvidence(t *testing.T) {
	s, _ := stableLaneFixture(t)
	ctx := context.Background()
	s.cycle(ctx)
	job, err := s.BeginStableInventory(ctx, s.view.ViewID, 0, 8)
	if err != nil {
		t.Fatal(err)
	}
	s.cycle(ctx)
	done, _ := s.StableJob(ctx, s.view.ViewID, job.JobID)
	if done.Page == nil || !s.stableProtected {
		t.Fatal("ordinary last page was treated as recent")
	}
	job, err = s.BeginStableRecent(ctx, s.view.ViewID)
	if err != nil {
		t.Fatal(err)
	}
	s.cycle(ctx)
	done, _ = s.StableJob(ctx, s.view.ViewID, job.JobID)
	if done.Page == nil || s.stableProtected {
		t.Fatal("explicit recent kept fixed-page protection")
	}
	assertStableResponse(t, StableResponse{ID: 1, OK: true, Job: &done})
	if err = s.LoadManifestProspects(ctx); !errors.Is(err, gymstable.ErrForbidden) {
		t.Fatal("legacy cold route escaped lane", err)
	}
}
