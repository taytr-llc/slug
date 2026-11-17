//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type manifestTestAuthorizer struct{}

func (manifestTestAuthorizer) WithSession(context.Context, string, func(string, func() error) error) error {
	return gymaccount.ErrUnauthorized
}
func manifestTestAccount() ManifestAccountConfig {
	a := ManifestAccountConfig{Account: AccountConfig{Fixture: "zero256", AccountID: strings.Repeat("a", 64), AuthorityID: strings.Repeat("b", 64), Authorizer: manifestTestAuthorizer{}, MiningToken: func(context.Context) (string, error) { return fixtureToken, nil }}, ViewID: strings.Repeat("c", 64), Generation: 1}
	a.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		return fn(gymaccount.Account{ID: a.Account.AccountID, Generation: 1}, func() error { return ctx.Err() })
	}
	return a
}
func TestManifestEngineNewIdentityAndIncompleteRefusal(t *testing.T) {
	cfg := testConfig(t)
	a := manifestTestAccount()
	c, e := NewManifestAccount(cfg, a)
	if e != nil {
		t.Fatal(e)
	}
	m := c.manifest
	if c.Snapshot().Attempts != 0 || c.Snapshot().View != nil {
		t.Fatal("constructor started work")
	}
	if _, e = NewManifestAccount(cfg, a); e == nil {
		t.Fatal("second owner")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c, e = NewManifestAccount(cfg, a)
	if e != nil || c.manifest != m {
		t.Fatal("new root not reopenable", e)
	}
	c.Close()
	path := filepath.Join(cfg.DataDir, manifestMarkerName)
	raw, _ := os.ReadFile(path)
	bad := bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1)
	os.WriteFile(path, bad, 0600)
	if c, e = NewManifestAccount(cfg, a); e == nil || c != nil {
		t.Fatal("duplicate marker accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, bad) {
		t.Fatal("rewrote rejected evidence")
	}
	os.WriteFile(path, raw, 0600)
	m.Phase = "initializing"
	atomicJSON(path, m)
	if c, e = NewManifestAccount(cfg, a); e == nil || c != nil {
		t.Fatal("incomplete data adopted")
	}
	a.Account.AdmissionSigner = func(context.Context, string, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		return gymauth.Authorization{}, nil
	}
	if c, e = NewManifestAccount(cfg, a); e == nil || c != nil {
		t.Fatal("delegated signer accepted")
	}
}

type manifestEngineCorpus struct {
	Pin    registry.ManifestAuthorityPin `json:"pin"`
	Marker string                        `json:"marker_canonical"`
	Cases  []struct {
		Record registry.ManifestRecord `json:"expected_record"`
		SVG    string                  `json:"svg"`
	} `json:"cases"`
	Rows struct {
		Meta          string `json:"meta"`
		Jobs, Records []struct{ Key, Value string }
		Sequences     []struct {
			KeyHex string `json:"key_hex"`
			Value  string
		}
	} `json:"database_rows"`
}

func manifestEngineFixture(t *testing.T) (*ManifestController, manifestEngineCorpus) {
	t.Helper()
	raw, e := os.ReadFile("../../registry/testdata/manifest/reference/manifest_admission_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "d32a8653c76c643addb3df091ca8d7dfdba66c2486f2ca305e718113f71d5794" {
		t.Fatal("reference changed")
	}
	var f manifestEngineCorpus
	if json.Unmarshal(raw, &f) != nil {
		t.Fatal("corpus")
	}
	a := manifestTestAccount()
	cfg := Config{DataDir: filepath.Join(t.TempDir(), "child"), NodeBinary: "/private/manifest-binary/slugd"}
	os.Mkdir(cfg.DataDir, 0700)
	c := &ManifestController{config: cfg, account: &a.Account, view: a, manifest: manifestIdentity{Phase: "ready", AuthorityPort: 37619, AuthorityPin: f.Pin, Identity: walletIdentity{GymPublicKey: f.Cases[0].Record.Authorization.PublicKey}}, cache: map[uint64]*manifestCached{}, snapshot: BlankManifestSnapshot()}
	c.manifest.TransportTrustID, e = prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: c.manifest.origin(), Token: fixtureToken, AuthorityPin: f.Pin})
	if e != nil {
		t.Fatal(e)
	}
	p := c.viewPin()
	c.snapshot.View = &p
	return c, f
}
func manifestSeedAuthority(t *testing.T, c *ManifestController, f manifestEngineCorpus) {
	t.Helper()
	p := filepath.Join(c.config.DataDir, "manifest-authority")
	os.Mkdir(p, 0700)
	if e := os.WriteFile(filepath.Join(p, "SLUG-REGISTRY.json"), []byte(f.Marker), 0600); e != nil {
		t.Fatal(e)
	}
	db, e := leveldb.OpenFile(filepath.Join(p, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	b := new(leveldb.Batch)
	b.Put([]byte("meta"), []byte(strings.Replace(f.Rows.Meta, `"last_sequence":5`, `"last_sequence":3`, 1)))
	for _, r := range f.Rows.Jobs {
		b.Put([]byte(r.Key), []byte(r.Value))
	}
	for _, r := range f.Rows.Records[:3] {
		b.Put([]byte(r.Key), []byte(r.Value))
	}
	for _, r := range f.Rows.Sequences[:3] {
		key, _ := hex.DecodeString(r.KeyHex)
		b.Put(key, []byte(r.Value))
	}
	if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	probe, e := registry.OpenManifestAuthority(p, f.Pin, true)
	if e != nil {
		t.Fatal("raw independent fixture", e)
	}
	if e = probe.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestManifestEngineLiveProjectionReferenceAndOwnership(t *testing.T) {
	c, f := manifestEngineFixture(t)
	ctx := context.Background()
	for _, i := range []int{1, 0, 2} {
		if e := c.observeManifest(ctx, f.Cases[i].Record); e != nil {
			t.Fatal(e)
		}
	}
	s := c.Snapshot()
	if s.AcceptedCount != 3 || len(s.Projection.Items) != 3 || !s.Projection.CompleteRecent || s.Projection.Revision != 3 {
		t.Fatal(s.Projection)
	}
	s.Projection.Items[0].ID = "mutated"
	if c.Snapshot().Projection.Items[0].ID == "mutated" {
		t.Fatal("snapshot alias")
	}
	portrait, e := c.ManifestPortrait(ctx, 3)
	if e != nil || portrait.SVG != f.Cases[2].SVG {
		t.Fatal("independent SVG differs", e)
	}
	receipt, e := c.ManifestReceipt(ctx, 3)
	if e != nil {
		t.Fatal(e)
	}
	receipt.Record.Proof.MerkleBranch[0] = "mutated"
	again, e := c.ManifestReceipt(ctx, 3)
	if e != nil || !reflect.DeepEqual(again.Record, f.Cases[2].Record) {
		t.Fatal("receipt alias", e)
	}
	if e = c.observeManifest(ctx, f.Cases[2].Record); e != nil || c.Snapshot().Projection.Revision != 3 {
		t.Fatal("non-idempotent repeat", e)
	}
	c.mu.Lock()
	c.snapshot.AcceptedCount = 6
	c.recomputeProjectionLocked()
	c.mu.Unlock()
	if c.Snapshot().Projection.CompleteRecent {
		t.Fatal("stale observed count advertised complete")
	}
	bad := f.Cases[2].Record
	bad.Sequence = 4
	if e = c.observeManifest(ctx, bad); e == nil || len(c.Snapshot().Projection.Items) != 0 {
		t.Fatal("conflicting identity replacement", e)
	}
}
func TestManifestEngineColdVerifiedPageAndLeaseRevocation(t *testing.T) {
	c, f := manifestEngineFixture(t)
	manifestSeedAuthority(t, c, f)
	if e := c.LoadManifestProspects(context.Background()); e != nil {
		t.Fatal(e)
	}
	s := c.Snapshot()
	if s.AcceptedCount != 3 || len(s.Projection.Items) != 3 || !s.Projection.CompleteRecent {
		t.Fatal(s.Projection)
	}
	var calls atomic.Int32
	a := c.view
	c.view.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		if calls.Add(1) == 2 {
			return gymaccount.ErrUnauthorized
		}
		return a.ViewLease(ctx, fn)
	}
	if e := c.LoadManifestProspects(context.Background()); e == nil || len(c.Snapshot().Projection.Items) != 0 {
		t.Fatal("revoked view published partial page", e)
	}
}
func TestManifestEnginePortraitFinalLeaseAndExportSlot(t *testing.T) {
	c, f := manifestEngineFixture(t)
	ctx := context.Background()
	if e := c.observeManifest(ctx, f.Cases[2].Record); e != nil {
		t.Fatal(e)
	}
	var calls int
	lease := c.view.ViewLease
	c.view.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		calls++
		if calls == 2 {
			return gymaccount.ErrExpired
		}
		return lease(ctx, fn)
	}
	if p, e := c.ManifestPortrait(ctx, 3); e == nil || p != (ManifestPortrait{}) {
		t.Fatal("late expired render disclosed")
	}
	c.view.ViewLease = lease
	receipt, e := c.ManifestReceipt(ctx, 3)
	if e != nil {
		t.Fatal(e)
	}
	c.mu.Lock()
	delete(c.cache, 3)
	c.recomputeProjectionLocked()
	c.mu.Unlock()
	path := filepath.Join(t.TempDir(), "receipt.json")
	if e = c.ExportManifestReceipt(ctx, 3, receipt.View.ViewID, receipt.Record.Verified.ProspectID, path); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(path)
	want, _ := json.Marshal(f.Cases[2].Record)
	if e != nil || !bytes.Equal(raw, append(want, '\n')) {
		t.Fatal("selected export changed", e)
	}
	if e = c.ExportManifestReceipt(ctx, 3, receipt.View.ViewID, receipt.Record.Verified.ProspectID, path); e == nil {
		t.Fatal("reused/overwrote export")
	}
}
func TestManifestExclusivePublicationRaceAndCancellation(t *testing.T) {
	for _, kind := range []string{"appeared", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "receipt.json")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			guard := func() error {
				if kind == "appeared" {
					return os.WriteFile(path, []byte("keep"), 0600)
				}
				cancel()
				return nil
			}
			if e := manifestExclusiveFile(ctx, path, []byte("candidate\n"), guard); e == nil {
				t.Fatal("late failure accepted")
			}
			raw, e := os.ReadFile(path)
			if kind == "appeared" && (e != nil || string(raw) != "keep") {
				t.Fatal("destination overwritten", e)
			}
			if kind == "cancelled" && !os.IsNotExist(e) {
				t.Fatal("cancelled write published")
			}
		})
	}
}
func TestManifestStopDrainsWithoutHoldingCacheLock(t *testing.T) {
	c, _ := manifestEngineFixture(t)
	ctx, done, e := c.beginOperation(context.Background(), "projection", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	entered := make(chan struct{})
	released := make(chan struct{})
	go func() { <-ctx.Done(); close(entered); <-released; c.finishOperation(done) }()
	stopped := make(chan struct{})
	go func() { c.Stop(); close(stopped) }()
	<-entered
	c.mu.Lock()
	c.mu.Unlock()
	select {
	case <-stopped:
		t.Fatal("did not drain")
	case <-time.After(20 * time.Millisecond):
	}
	close(released)
	<-stopped
}

func TestManifestProjectionRecentEightCoverageAndBudgets(t *testing.T) {
	c, f := manifestEngineFixture(t)
	v, e := c.projectManifest(f.Cases[0].Record)
	if e != nil {
		t.Fatal(e)
	}
	c.snapshot.Projection.State = "ready"
	c.snapshot.AcceptedCount = 12
	for seq := uint64(5); seq <= 12; seq++ {
		owned := *v
		owned.summary.Sequence = seq
		owned.summary.ID = strings.Repeat(string(rune('a'+seq-5)), 64)
		c.cache[seq] = &owned
	}
	c.recomputeProjectionLocked()
	p := c.Snapshot().Projection
	if !p.CompleteRecent || !p.HasOlder || p.ThroughSequence != 12 || len(p.Items) != 8 {
		t.Fatal(p)
	}
	// Equal display names remain independent sequence/ID members.
	if p.Items[0].Name != p.Items[1].Name || p.Items[0].ID == p.Items[1].ID {
		t.Fatal("fixture grouping")
	}
	delete(c.cache, 9)
	c.recomputeProjectionLocked()
	if c.Snapshot().Projection.CompleteRecent {
		t.Fatal("gap claimed complete")
	}
	if !manifestCacheFits(c.cache, nil) {
		t.Fatal("valid bounded cache refused")
	}
	c.cache[5].svg = strings.Repeat("x", 262145)
	if manifestCacheFits(c.cache, nil) {
		t.Fatal("SVG subtotal exceeded")
	}
	c.cache[5].svg = ""
	c.cache[5].receiptBytes = 524289
	if manifestCacheFits(c.cache, nil) {
		t.Fatal("receipt subtotal exceeded")
	}
	c.cache[5].receiptBytes = 0
	c.cache[5].manifestBytes = 65537
	if manifestCacheFits(c.cache, nil) {
		t.Fatal("manifest subtotal exceeded")
	}
}
func TestManifestMarkersStrictCanonicalLFAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marker")
	v := savedStatus{Height: 1}
	if atomicJSON(path, v) != nil {
		t.Fatal("write")
	}
	var got savedStatus
	if readManifestJSON(path, 2048, &got) != nil || got != v {
		t.Fatal("roundtrip")
	}
	raw, _ := os.ReadFile(path)
	for _, bad := range [][]byte{bytes.TrimSuffix(raw, []byte("\n")), append(append([]byte{}, raw...), '\n'), append([]byte(" "), raw...), bytes.Replace(raw, []byte(`"height":1`), []byte(`"height":1,"height":1`), 1), bytes.Replace(raw, []byte(`"height"`), []byte(`"Height"`), 1)} {
		os.WriteFile(path, bad, 0600)
		if readManifestJSON(path, 2048, &got) == nil {
			t.Fatal("noncanonical accepted")
		}
	}
	os.WriteFile(path, raw, 0600)
	link := filepath.Join(dir, "link")
	if os.Symlink(path, link) != nil {
		t.Fatal("symlink")
	}
	if readManifestJSON(link, 2048, &got) == nil {
		t.Fatal("symlink read")
	}
}

func TestManifestMalformedIdentityDoesNotCreateLock(t *testing.T) {
	cfg := testConfig(t)
	if os.Mkdir(cfg.DataDir, 0700) != nil {
		t.Fatal("mkdir")
	}
	path := filepath.Join(cfg.DataDir, manifestMarkerName)
	raw := []byte("{}\n")
	os.WriteFile(path, raw, 0600)
	if c, e := NewManifestAccount(cfg, manifestTestAccount()); e == nil || c != nil {
		t.Fatal("unrecognized marker accepted")
	}
	if _, e := os.Lstat(filepath.Join(cfg.DataDir, "instance-lock")); !os.IsNotExist(e) {
		t.Fatal("created lock before validating marker")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("rewrote unknown identity")
	}
}
func TestManifestOldAncestorRejectedWithoutMutation(t *testing.T) {
	cfg := testConfig(t)
	parent := filepath.Dir(cfg.DataDir)
	path := filepath.Join(parent, "SLUG-PILOT.json")
	os.WriteFile(path, []byte("old bytes"), 0600)
	if c, e := NewManifestAccount(cfg, manifestTestAccount()); e == nil || c != nil {
		t.Fatal("adopted old ancestor")
	}
	if _, e := os.Lstat(cfg.DataDir); !os.IsNotExist(e) {
		t.Fatal("created inside old root")
	}
	after, _ := os.ReadFile(path)
	if string(after) != "old bytes" {
		t.Fatal("old marker changed")
	}
}

func TestManifestPopulatedRefreshLoadingCancellationAndFailure(t *testing.T) {
	c, f := manifestEngineFixture(t)
	manifestSeedAuthority(t, c, f)
	if e := c.LoadManifestProspects(context.Background()); e != nil {
		t.Fatal(e)
	}
	original := c.view.ViewLease
	calls := 0
	entered, release := make(chan struct{}), make(chan struct{})
	c.view.ViewLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		calls++
		if calls == 2 {
			close(entered)
			<-release
		}
		return original(ctx, fn)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if e := c.RefreshManifestProspects(ctx); e != nil {
		t.Fatal(e)
	}
	<-entered
	s := c.Snapshot()
	if s.Projection.State != "loading" || !s.Projection.HasOlder || s.Projection.CompleteRecent || len(s.Projection.Items) != 0 || s.AcceptedCount != 3 {
		t.Fatal("invalid loading DTO", s.Projection)
	}
	cancel()
	close(release)
	c.Stop()
	s = c.Snapshot()
	if s.Projection.State != "cancelled" || !s.Projection.HasOlder || s.Projection.CompleteRecent || len(s.Projection.Items) != 0 {
		t.Fatal("invalid cancellation DTO", s.Projection)
	}
	c.view.ViewLease = original
	path := filepath.Join(c.config.DataDir, "manifest-authority", "SLUG-REGISTRY.json")
	if os.WriteFile(path, []byte("{}\n"), 0600) != nil {
		t.Fatal("corrupt marker")
	}
	if e := c.LoadManifestProspects(context.Background()); e == nil {
		t.Fatal("corrupt cold store accepted")
	}
	s = c.Snapshot()
	if s.Projection.State != "failed" || !s.Projection.HasOlder || s.Projection.CompleteRecent || len(s.Projection.Items) != 0 {
		t.Fatal("invalid failure DTO", s.Projection)
	}
}
