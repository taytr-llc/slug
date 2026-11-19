//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
)

func poolLeagueEntryPin(t *testing.T, p gymstable.Pin) gymentry.Pin {
	t.Helper()
	id, err := gymstable.ID(p)
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := gymentry.NamespaceID(p.Identity.Network, p.Identity.Genesis, p.AccountAuthorityID)
	if err != nil {
		t.Fatal(err)
	}
	v, err := gymentry.ValidatePin(gymentry.Pin{Version: 1, NamespaceID: namespace, CalendarID: gymentry.CalendarID(), RulesID: gymentry.RulesID(), StableStoreID: id, StablePin: p})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func poolLeagueChildFixture(t *testing.T) (*PoolChild, *poolDesktopHarness, *gymevent.Store) {
	t.Helper()
	c, issuer, h, _ := poolChildLifecycleFixture(t)
	s := c.stable
	// These tests exercise asynchronous lane publication. Model the real
	// account store's serial leases, not the lightweight nested-call detector's
	// rejection of two unrelated concurrent callers.
	originalLease := h.account.ViewLease
	leaseGate := make(chan struct{}, 1)
	h.account.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		select {
		case leaseGate <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-leaseGate }()
		return originalLease(ctx, fn)
	}
	s.view.ViewLease = h.account.ViewLease
	c.account.Manifest.ViewLease = h.account.ViewLease
	endpoint, err := url.Parse(h.config.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.ParseUint(endpoint.Port(), 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: h.config.URL, Token: h.token, AuthorityPin: s.sharedPin.Source})
	if err != nil {
		t.Fatal(err)
	}
	s.manifest.AuthorityPort, s.manifest.TransportTrustID = uint16(port), trust
	if !stableViewValid(s.viewPin()) {
		t.Fatal("fixture lacks a complete transport-bound view")
	}
	entryPin := poolLeagueEntryPin(t, *s.sharedPin)
	entry, err := gymentry.Create(t.Context(), filepath.Join(s.config.DataDir, "entries"), entryPin, s.stableStore)
	if err != nil {
		t.Fatal(err)
	}
	if err = entry.Close(); err != nil {
		t.Fatal(err)
	}
	other := *s.sharedPin
	other.Identity, _ = gymaccount.FixtureIdentity("incrementing256")
	other.AccountID = strings.Repeat("e", 64)
	eventPin := gymevent.Pin{Version: 1, CalendarID: gymentry.CalendarID(), RulesID: gymevent.RulesID(), Participants: []gymentry.Pin{entryPin, poolLeagueEntryPin(t, other)}}
	sort.Slice(eventPin.Participants, func(i, j int) bool {
		return eventPin.Participants[i].StablePin.Identity.GymID < eventPin.Participants[j].StablePin.Identity.GymID
	})
	eventRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("canonical event fixture root", err)
	}
	events, err := gymevent.Create(t.Context(), filepath.Join(eventRoot, "events"), eventPin)
	if err != nil {
		t.Fatal("create event fixture", err)
	}
	t.Cleanup(func() {
		if err := events.Close(); err != nil {
			t.Error(err)
		}
	})
	info, err := events.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	r := c.resources
	r.Config, r.Issuer, r.StablePin = s.config, issuer, *s.sharedPin
	r.Source = h.config.Source
	r.Source.AuthorityPin = s.sharedPin.Source
	sourceID, err := poolservice.BindingID(r.Source)
	if err != nil {
		t.Fatal(err)
	}
	r.Recipient.SourceBindingID = sourceID
	entryID, _ := gymentry.ID(entryPin)
	r.League = &poolLeagueChildBinding{SourceBindingID: sourceID, EntryPin: entryPin, EntryStoreID: entryID, EventPin: info.Pin, EventStoreID: info.StoreID, EventCreatedMS: info.CreatedAtMS}
	r.Events, r.Factories = events, defaultPoolLeagueFactories()
	c.resources = r
	if err := s.closePair(); err != nil {
		t.Fatal(err)
	}
	s.installed = false
	if err := s.preparePoolLeague(r); err != nil {
		t.Fatal(err)
	}
	if err := s.openPair(nil); err != nil {
		t.Fatal(err)
	}
	status, err := s.stableStore.Status(t.Context(), s.session())
	if err != nil {
		t.Fatal(err)
	}
	s.stableStatus, s.installed = &status, true
	if err := s.attachPoolLeague(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	return c, h, events
}

func poolLeagueEntryDone(t *testing.T, c *PoolChild, job EntryJob) EntryJob {
	t.Helper()
	var out EntryJob
	poolBackupWait(t, func() bool {
		j, err := c.EntryJob(t.Context(), job.View.ViewID, job.JobID)
		if err == nil {
			out = j
		}
		return err == nil && j.State != "queued" && j.State != "running"
	})
	if out.State != "complete" {
		t.Fatalf("entry failed: %s/%s", out.State, out.Error)
	}
	return out
}

func TestPoolLeagueChildBindingAndBorrowedLifetime(t *testing.T) {
	c, _, events := poolLeagueChildFixture(t)
	base := c.resources
	for name, change := range map[string]func(*poolChildResources){
		"missing": func(r *poolChildResources) { r.League = nil },
		"source":  func(r *poolChildResources) { r.League.SourceBindingID = strings.Repeat("a", 64) },
		"current_entry": func(r *poolChildResources) {
			r.League.EntryPin = r.League.EventPin.Participants[0]
			if r.League.EntryPin == base.League.EntryPin {
				r.League.EntryPin = r.League.EventPin.Participants[1]
			}
			r.League.EntryStoreID, _ = gymentry.ID(r.League.EntryPin)
		},
		"entry_id": func(r *poolChildResources) { r.League.EntryStoreID = strings.Repeat("a", 64) },
		"event_id": func(r *poolChildResources) { r.League.EventStoreID = strings.Repeat("a", 64) },
		"foreign_participant_source": func(r *poolChildResources) {
			for i := range r.League.EventPin.Participants {
				p := &r.League.EventPin.Participants[i]
				if *p == r.League.EntryPin {
					continue
				}
				p.StablePin.Source.AuthorityID = strings.Repeat("a", 64)
				p.StableStoreID, _ = gymstable.ID(p.StablePin)
			}
			r.League.EventStoreID, _ = gymevent.ID(r.League.EventPin)
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := base
			x := *base.League
			x.EventPin = cloneEventPin(x.EventPin)
			r.League = &x
			change(&r)
			if _, err := validatePoolLeagueChild(r); err == nil {
				t.Fatal("substituted league accepted")
			}
		})
	}
	view := c.account.Manifest.ViewID
	entry, event := c.EntrySnapshot(), c.EventSnapshot()
	if entry.StoreID != base.League.EntryStoreID || event.StoreID != base.League.EventStoreID || event.Pin == nil || event.View == nil || event.View.ViewID != view || event.ParticipantID != base.StablePin.Identity.GymID {
		t.Fatal("wrong attached authority")
	}
	event.Pin.Participants[0].StablePin.AccountID = strings.Repeat("0", 64)
	if c.EventSnapshot().Pin.Participants[0].StablePin.AccountID == strings.Repeat("0", 64) {
		t.Fatal("event snapshot aliases authority")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Info(t.Context()); err != nil {
		t.Fatal("child closed borrowed event owner", err)
	}
	if _, err := base.Issuer.Info(); err != nil {
		t.Fatal("child closed borrowed issuer", err)
	}
	if c.EntrySnapshot().View != nil || c.EventSnapshot().View != nil {
		t.Fatal("closed child disclosed league")
	}
}

func TestPoolLeagueChildPendingReopensWithSameFactories(t *testing.T) {
	c, _, _ := poolLeagueChildFixture(t)
	s, view := c.stable, c.account.Manifest.ViewID
	upcoming, err := c.EntryUpcoming(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	request := gymentry.Request{Version: 1, EventID: upcoming.Event.EventID, Saturday: upcoming.Event.Saturday, Action: "withdraw", ProspectIDs: []string{}}
	job, err := c.BeginEntryPrepare(t.Context(), view, request)
	if err != nil {
		t.Fatal(err)
	}
	prepared := poolLeagueEntryDone(t, c, job)
	c.Stop()
	var stableOpens, entryOpens atomic.Int32
	f := *defaultPoolLeagueFactories()
	f.openStable = func(p string, pin gymstable.Pin, src *registry.ManifestStore) (*gymstable.Store, error) {
		stableOpens.Add(1)
		return gymstable.OpenShared(p, pin, src)
	}
	f.openEntry = func(ctx context.Context, p string, pin gymentry.Pin, src *gymstable.Store) (*gymentry.Store, error) {
		entryOpens.Add(1)
		return gymentry.Open(ctx, p, pin, src)
	}
	s.sharedFactories = &f
	if err := s.closePair(); err != nil {
		t.Fatal(err)
	}
	if err := s.openPair(nil); err != nil {
		t.Fatal(err)
	}
	s.eventAttached, s.entryPending = false, ""
	if err := s.attachPoolLeague(t.Context(), c.resources); err != nil {
		t.Fatal(err)
	}
	if stableOpens.Load() != 1 || entryOpens.Load() != 1 || c.EntrySnapshot().PendingOperationID != prepared.OperationID {
		t.Fatal("reopen lost factory or durable pending intent")
	}
	job, err = c.BeginEntryApply(t.Context(), view, prepared.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	applied := poolLeagueEntryDone(t, c, job)
	job, err = c.BeginEntryApply(t.Context(), view, prepared.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	replay := poolLeagueEntryDone(t, c, job)
	if applied.Mutation == nil || replay.Mutation == nil || !replay.Mutation.Replayed || !reflect.DeepEqual(replay.Mutation.Receipt, applied.Mutation.Receipt) {
		t.Fatal("retry changed committed entry")
	}
}

func TestPoolLeagueChildBackupJoinsLaneAndRejectsDispatch(t *testing.T) {
	c, h, events := poolLeagueChildFixture(t)
	view := c.account.Manifest.ViewID
	upcoming, err := c.EntryUpcoming(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	c.stable.entryCheckpoint = func(stage string) {
		if stage == "before-entry" {
			close(entered)
			<-release
		}
	}
	job, err := c.BeginEntryCheck(t.Context(), view, upcoming.Event.Saturday)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("entry did not reach pre-work checkpoint")
	}
	result := make(chan *poolBackupOperation, 1)
	failure := make(chan error, 1)
	go func() { op, err := c.beginPoolBackup(t.Context(), false); failure <- err; result <- op }()
	poolBackupWait(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.backup != nil })
	if h.sessions.active.Load() {
		t.Fatal("lane drain holds account lease")
	}
	select {
	case <-result:
		t.Fatal("backup did not join active lane")
	case <-time.After(20 * time.Millisecond):
	}
	if _, err := c.BeginEventCapture(t.Context(), view, upcoming.Event.Saturday); !errors.Is(err, gymstable.ErrBusy) {
		t.Fatal("event dispatched through backup", err)
	}
	if _, err := c.BeginStableSync(t.Context(), view); !errors.Is(err, gymstable.ErrBusy) {
		t.Fatal("stable dispatched through backup", err)
	}
	close(release)
	if err := <-failure; err != nil {
		t.Fatal(err)
	}
	op := <-result
	last, err := c.EntryJob(t.Context(), view, job.JobID)
	if err != nil || last.State != "cancelled" {
		t.Fatal("cancelled job unavailable during backup", last.State, err)
	}
	if _, err := c.CancelEntryJob(t.Context(), view, job.JobID); err != nil {
		t.Fatal(err)
	}
	c.finishPoolBackup(op, "")
	if c.Snapshot().State != "stopped" || c.Snapshot().Attempts != 0 {
		t.Fatal("backup started mining")
	}
	if _, err := events.Info(t.Context()); err != nil {
		t.Fatal("backup closed shared events", err)
	}
}

func TestPoolLeagueChildRevokedCommitRetainsUnknownAndBlanksView(t *testing.T) {
	c, h, _ := poolLeagueChildFixture(t)
	view := c.account.Manifest.ViewID
	upcoming, err := c.EntryUpcoming(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	c.stable.entryCheckpoint = func(stage string) {
		if stage == "before-entry-publication" {
			close(entered)
			<-release
		}
	}
	request := gymentry.Request{Version: 1, EventID: upcoming.Event.EventID, Saturday: upcoming.Event.Saturday, Action: "withdraw", ProspectIDs: []string{}}
	job, err := c.BeginEntryPrepare(t.Context(), view, request)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("entry did not reach committed-publication checkpoint", c.EntrySnapshot().Error)
	}
	h.mu.Lock()
	h.view, h.token = false, strings.Repeat("2", 64)
	h.mu.Unlock()
	close(release)
	poolBackupWait(t, func() bool {
		c.stable.stableMu.Lock()
		defer c.stable.stableMu.Unlock()
		return c.stable.entryLast != nil
	})
	c.stable.stableMu.Lock()
	last := *c.stable.entryLast
	c.stable.stableMu.Unlock()
	if last.Error != "outcome_unknown" || last.Intent != nil {
		t.Fatal("committed-but-undisclosed intent reported definite failure", last.State, last.Error)
	}
	if _, err := c.EntryJob(t.Context(), view, job.JobID); err == nil {
		t.Fatal("replacement token disclosed old job")
	}
	if c.EntrySnapshot().View != nil || c.EventSnapshot().View != nil {
		t.Fatal("revoked child disclosed authority")
	}
}

func TestPoolLeagueChildEstablishedStableCannotBeRecreated(t *testing.T) {
	c, h, _ := poolLeagueChildFixture(t)
	r := c.resources
	r.Config.DataDir = t.TempDir()
	poolTrust, err := poolservice.TransportTrustID(poolservice.ClientConfig{Version: 1, URL: h.config.URL, Token: h.token, PublicKey: h.config.PublicKey, Source: r.Source})
	if err != nil {
		t.Fatal(err)
	}
	admissionTrust, err := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: h.config.URL, Token: h.token, AuthorityPin: r.Source.AuthorityPin})
	if err != nil {
		t.Fatal(err)
	}
	r.WorkerConfig = prospectworker.PoolConfig{Version: 1, Source: r.Source, PublicKey: h.config.PublicKey, AuthorityOrigin: h.config.URL, TransportTrustID: poolTrust, AdmissionOrigin: h.config.URL, AdmissionTrustID: admissionTrust, MaxRecords: 1000, MaxBytes: 256 << 20}
	box, err := openPoolChildOutbox(r)
	if box != nil {
		_ = box.Close()
	}
	if box != nil || !errors.Is(err, ErrManifestInitialization) {
		t.Fatal("missing league-established stable accepted", err)
	}
	if _, err := os.Lstat(filepath.Join(r.Config.DataDir, "stable")); !os.IsNotExist(err) {
		t.Fatal("league-established stable recreated")
	}
}
