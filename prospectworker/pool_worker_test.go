//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

type pwOracleCase struct {
	Source   string `json:"source_candidate"`
	Nonce    uint32 `json:"nonce"`
	Header   string `json:"header"`
	Hash     string `json:"work_hash"`
	Key      string `json:"submission_key"`
	Prospect bool   `json:"qualifies_prospect"`
	Block    bool   `json:"qualifies_block"`
	Targets  []struct {
		Relation string `json:"relation"`
		Target   string `json:"target"`
	} `json:"share_targets"`
}

func pwFixture(t *testing.T, name string, nonce uint32, below bool) (PoolConfig, poolshare.MiningWork, pwOracleCase) {
	t.Helper()
	raw, e := os.ReadFile("../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("independent Set52 pool corpus changed")
	}
	var corpus struct {
		Sources []struct {
			Name     string                  `json:"name"`
			Job      prospectjob.ManifestJob `json:"job"`
			Template prospectwork.Template   `json:"template"`
			Payout   string                  `json:"payout_address"`
		} `json:"sources"`
		Cases []pwOracleCase `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("decode independent pool corpus")
	}
	var work poolshare.MiningWork
	var payout string
	var tc pwOracleCase
	for _, s := range corpus.Sources {
		if s.Name == name {
			work = poolshare.MiningWork{Job: s.Job, Template: s.Template}
			payout = s.Payout
		}
	}
	for _, c := range corpus.Cases {
		if c.Source == name && c.Nonce == nonce {
			tc = c
		}
	}
	if work.Job.ID == "" || tc.Hash == "" {
		t.Fatal("missing independent pool case")
	}
	j := work.Job
	p := prospectnode.Policy{Version: j.Context.Version, Network: j.Context.Network, Genesis: j.Context.Genesis, DNAVersion: j.Context.DNAVersion, ProspectTarget: j.Context.ProspectTarget}
	target := strings.Repeat("f", 64)
	if below {
		target = ""
		for _, row := range tc.Targets {
			if row.Relation == "digest_minus_one" {
				target = row.Target
			}
		}
		if target == "" {
			t.Fatal("no independent below-boundary target")
		}
	}
	source := poolshare.Config{Version: 1, AuthorityPin: registry.ManifestAuthorityPin{Version: registry.Version5, Network: p.Network, Genesis: p.Genesis, AuthorityID: j.AuthorityID, DescriptorID: j.DescriptorID}, Policy: p, PayoutAddress: payout, PoolTarget: target, MaxWorks: 256, MaxSubmissions: 4096, MaxBytes: 64 << 20}
	c := PoolConfig{Version: PoolVersion1, Source: source, PublicKey: j.PublicKey, AuthorityOrigin: "http://127.0.0.1:39001", AdmissionOrigin: "http://127.0.0.1:39002", MaxRecords: 1, MaxBytes: 64 << 20}
	c.TransportTrustID, e = poolservice.TransportTrustID(poolservice.ClientConfig{Version: 1, URL: c.AuthorityOrigin, Token: strings.Repeat("0", 64), PublicKey: c.PublicKey, Source: c.Source})
	if e != nil {
		t.Fatal(e)
	}
	c.AdmissionTrustID, e = prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: c.AdmissionOrigin, Token: strings.Repeat("0", 64), AuthorityPin: c.Source.AuthorityPin})
	if e != nil {
		t.Fatal(e)
	}
	return c, work, tc
}
func pwAdmissionFixture(t *testing.T) mwCase {
	t.Helper()
	raw, e := os.ReadFile("../registry/testdata/manifest/reference/manifest_admission_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "d32a8653c76c643addb3df091ca8d7dfdba66c2486f2ca305e718113f71d5794" {
		t.Fatal("independent admission corpus changed")
	}
	var corpus struct {
		Cases []mwCase `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Cases) != 5 {
		t.Fatal("decode independent admission corpus")
	}
	return corpus.Cases[0]
}
func pwReceipt(c PoolConfig, w poolshare.MiningWork, f pwOracleCase) poolshare.Submission {
	r := poolshare.Submission{Version: 1, Sequence: 1, AcceptedSequence: 1, JobID: w.Job.ID, GymID: w.Job.GymID, Nonce: f.Nonce, PoolTarget: c.Source.PoolTarget, Header: f.Header, WorkHash: f.Hash, DuplicateKey: f.Key, Pool: true, Prospect: f.Prospect, Block: f.Block, ReceivedAtMS: w.Job.IssuedAtMS + 100, AcceptedAtMS: w.Job.IssuedAtMS + 100, Acceptance: "current-parent", BlockState: "none"}
	if f.Block {
		r.BlockHeight = w.Job.Context.ParentHeight + 1
		r.BlockState = "canonical-observed"
		r.BlockObservedAtMS = w.Job.IssuedAtMS + 100
	}
	return r
}

type pwAuthority struct {
	c                        PoolConfig
	w                        poolshare.MiningWork
	issues, lookups, submits int
	issue                    func()
	lookup                   func() (poolshare.Submission, error)
	submit                   func() (poolshare.Submission, error)
}

func (a *pwAuthority) ProtocolVersion() uint32        { return poolservice.Version1 }
func (a *pwAuthority) AuthorityOrigin() string        { return a.c.AuthorityOrigin }
func (a *pwAuthority) SourceConfig() poolshare.Config { return a.c.Source }
func (a *pwAuthority) TransportTrustID() string       { return a.c.TransportTrustID }
func (a *pwAuthority) PublicKey() string              { return a.c.PublicKey }
func (a *pwAuthority) Issue(context.Context) (poolshare.MiningWork, error) {
	a.issues++
	if a.issue != nil {
		a.issue()
	}
	return a.w, nil
}
func (a *pwAuthority) Work(context.Context, string) (poolshare.MiningWork, error) { return a.w, nil }
func (a *pwAuthority) Submit(_ context.Context, w poolshare.MiningWork, n uint32) (poolshare.Submission, error) {
	a.submits++
	if a.submit == nil {
		return poolshare.Submission{}, errors.New("unexpected pool submit")
	}
	return a.submit()
}
func (a *pwAuthority) Lookup(_ context.Context, w poolshare.MiningWork, n uint32) (poolshare.Submission, error) {
	a.lookups++
	if a.lookup == nil {
		return poolshare.Submission{}, &poolservice.ClientError{Code: "not_found"}
	}
	return a.lookup()
}
func (a *pwAuthority) Balance(context.Context) (poolservice.BalanceResponse, error) {
	return poolservice.BalanceResponse{}, errors.New("worker must not poll settlement during hashing")
}

type pwAdmission struct {
	c      PoolConfig
	f      mwCase
	calls  int
	submit func()
}

func (a *pwAdmission) ProtocolVersion() uint32                     { return 3 }
func (a *pwAdmission) AuthorityOrigin() string                     { return a.c.AdmissionOrigin }
func (a *pwAdmission) AuthorityPin() registry.ManifestAuthorityPin { return a.c.Source.AuthorityPin }
func (a *pwAdmission) TransportTrustID() string                    { return a.c.AdmissionTrustID }
func (a *pwAdmission) IssueManifestJob(context.Context, string) (prospectjob.ManifestJob, error) {
	return prospectjob.ManifestJob{}, errors.New("pool admission must never issue a second birth job")
}
func (a *pwAdmission) SubmitManifest(_ context.Context, j prospectjob.ManifestJob, p prospect.Proof, auth gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error) {
	a.calls++
	if !reflect.DeepEqual(j, a.f.Job) || !reflect.DeepEqual(p, a.f.Proof) || auth != a.f.Auth {
		return prospectservice.ManifestAdmissionResult{}, errors.New("unexpected admission evidence")
	}
	if a.submit != nil {
		a.submit()
	}
	return mwResult(a.f), nil
}

type pwNode struct {
	calls []string
	call  func(string)
}

func (n *pwNode) mark(s string) {
	n.calls = append(n.calls, s)
	if n.call != nil {
		n.call(s)
	}
}
func (n *pwNode) ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error) {
	n.mark("resolve")
	return prospectwork.Template{}, errors.New("pool worker must not rebuild from its own template")
}
func (n *pwNode) Proposal(context.Context, prospect.Context, string) error {
	n.mark("proposal")
	return nil
}
func (n *pwNode) RecheckContext(context.Context, prospect.Context) error {
	n.mark("recheck")
	return nil
}
func (n *pwNode) SubmitBlock(context.Context, prospect.Context, string) error {
	n.mark("broadcast")
	return nil
}
func (n *pwNode) CanonicalBlock(context.Context, prospect.Context, prospect.Proof, string) error {
	n.mark("canonical")
	return nil
}
func pwBox(t *testing.T, c PoolConfig) (*PoolOutbox, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pool-outbox")
	box, e := CreatePoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = box.Close() })
	return box, path
}
func pwClock(w *PoolWorker, work poolshare.MiningWork) {
	w.now = func() time.Time { return time.UnixMilli(work.Job.IssuedAtMS + 100) }
}
func pwStopAtCapacity(w *PoolWorker, cancel context.CancelFunc) {
	w.wait = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
}

func TestPoolWorkerProspectOnlyRemainsIndependent(t *testing.T) {
	c, work, f := pwFixture(t, "coinbase-only", 0, true)
	admissionFixture := pwAdmissionFixture(t)
	box, _ := pwBox(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	a := &pwAuthority{c: c, w: work}
	ad := &pwAdmission{c: c, f: admissionFixture}
	node := &pwNode{}
	signs := 0
	observerCalls := 0
	w, e := NewPoolWithHooks(c, a, ad, node, func(_ context.Context, _ prospect.Context, p prospect.Proof) (gymauth.Authorization, error) {
		signs++
		if !reflect.DeepEqual(p, admissionFixture.Proof) {
			t.Fatal("prospect proof changed")
		}
		return admissionFixture.Auth, nil
	}, box, func(_ context.Context, r registry.ManifestRecord) error {
		observerCalls++
		if !reflect.DeepEqual(r, admissionFixture.Record) {
			t.Fatal("observer received different fighter")
		}
		cancel()
		return nil
	}, nil)
	if e != nil {
		t.Fatal(e)
	}
	pwClock(w, work)
	if e = w.Run(ctx); e != nil {
		t.Fatal(e)
	}
	rows, e := box.List(0, 8)
	if e != nil || len(rows) != 1 || rows[0].WorkHash != f.Hash || rows[0].PoolWin || rows[0].PoolReceipt != nil || rows[0].Fighter == nil || rows[0].Fighter.AdmissionState != "accepted" {
		t.Fatalf("prospect-only result %+v %v", rows, e)
	}
	if a.lookups != 0 || a.submits != 0 || signs != 1 || ad.calls != 1 || observerCalls != 1 || w.Status().Attempts != 1 {
		t.Fatalf("prospect-only credit/admission mismatch: %+v signs%d admission%d observer%d status%+v", a, signs, ad.calls, observerCalls, w.Status())
	}
	for _, s := range node.calls {
		if s == "resolve" || s == "broadcast" || s == "canonical" {
			t.Fatalf("unexpected prospect-only node action %s", s)
		}
	}
}

func TestPoolWorkerPoolOnlyCreatesNoFighter(t *testing.T) {
	c, work, f := pwFixture(t, "modeled-all-three-miss", 0, false)
	box, _ := pwBox(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	a := &pwAuthority{c: c, w: work, submit: func() (poolshare.Submission, error) {
		rows, e := box.List(0, 1)
		if e != nil || len(rows) != 1 {
			t.Fatal("submit preceded durable find", e)
		}
		return pwReceipt(c, work, f), nil
	}}
	ad := &pwAdmission{c: c}
	node := &pwNode{}
	w, e := NewPool(c, a, ad, node, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		t.Fatal("pool-only work reached fighter signer")
		return gymauth.Authorization{}, nil
	}, box)
	if e != nil {
		t.Fatal(e)
	}
	pwClock(w, work)
	pwStopAtCapacity(w, cancel)
	if e = w.Run(ctx); e != nil {
		t.Fatal(e)
	}
	rows, e := box.List(0, 8)
	if e != nil || len(rows) != 1 || rows[0].WorkHash != f.Hash || !rows[0].PoolWin || rows[0].PoolReceipt == nil || rows[0].PoolState != "accepted" || rows[0].Fighter != nil {
		t.Fatalf("pool-only result %+v %v", rows, e)
	}
	if a.submits != 1 || ad.calls != 0 || w.Status().Attempts != 1 {
		t.Fatalf("pool-only delivered more than once: %+v %+v", a, w.Status())
	}
	for _, s := range node.calls {
		if s == "resolve" || s == "broadcast" || s == "canonical" {
			t.Fatalf("unexpected pool-only node action %s", s)
		}
	}
}

func TestPoolWorkerCancellationDrainsHashAndSavesFoundNonce(t *testing.T) {
	c, work, f := pwFixture(t, "modeled-all-three-miss", 0, false)
	box, _ := pwBox(t, c)
	a := &pwAuthority{c: c, w: work}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w, e := NewPool(c, a, &pwAdmission{c: c}, &pwNode{}, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		t.Fatal("sign after cancel")
		return gymauth.Authorization{}, nil
	}, box)
	if e != nil {
		t.Fatal(e)
	}
	pwClock(w, work)
	entered, release := make(chan struct{}), make(chan struct{})
	w.check = func(c context.Context, p *prospectwork.ManifestWork, n uint32) (prospectwork.ShareCheck, error) {
		close(entered)
		<-release
		if c.Err() != nil {
			t.Error("in-flight normal hash was canceled instead of drained")
		}
		return p.CheckNonce(c, n)
	}
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		close(release)
		t.Fatal("hash did not begin")
	}
	cancel()
	select {
	case e := <-done:
		close(release)
		t.Fatalf("Run returned before hash drained: %v", e)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hash did not drain")
	}
	rows, e := box.List(0, 8)
	if e != nil || len(rows) != 1 || rows[0].Nonce != 0 || rows[0].WorkHash != f.Hash || rows[0].PoolReceipt != nil {
		t.Fatalf("found work not retained on stop: %+v %v", rows, e)
	}
	scan, ok, e := box.scan()
	if e != nil || !ok || scan.NextNonce != 1 {
		t.Fatalf("cursor did not synchronize with found work: %+v %v %v", scan, ok, e)
	}
	if a.lookups != 0 || a.submits != 0 || w.Status().Attempts != 1 {
		t.Fatal("cancellation dispatched or continued hashing")
	}
}

func TestPoolWorkerLostPoolAckRecoversAtCapacityAfterRestart(t *testing.T) {
	c, work, f := pwFixture(t, "coinbase-only", 0, false)
	admissionFixture := pwAdmissionFixture(t)
	box, path := pwBox(t, c)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	committed := false
	a := &pwAuthority{c: c, w: work}
	a.lookup = func() (poolshare.Submission, error) {
		if committed {
			return pwReceipt(c, work, f), nil
		}
		return poolshare.Submission{}, &poolservice.ClientError{Code: "not_found"}
	}
	a.submit = func() (poolshare.Submission, error) {
		committed = true
		cancel()
		return poolshare.Submission{}, &poolservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	ad := &pwAdmission{c: c, f: admissionFixture}
	signer := func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		return admissionFixture.Auth, nil
	}
	w, e := NewPool(c, a, ad, &pwNode{}, signer, box)
	if e != nil {
		t.Fatal(e)
	}
	pwClock(w, work)
	if e = w.Run(ctx); e != nil {
		t.Fatal(e)
	}
	rows, e := box.List(0, 1)
	if e != nil || len(rows) != 1 || !committed || a.submits != 1 || rows[0].PoolReceipt != nil {
		t.Fatalf("lost acknowledgment not retained: %+v %v", rows, e)
	}
	if e = box.Close(); e != nil {
		t.Fatal(e)
	}
	box, e = OpenPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel2()
	issuesBefore := a.issues
	w, e = NewPoolWithHooks(c, a, ad, &pwNode{}, signer, box, func(context.Context, registry.ManifestRecord) error { cancel2(); return nil }, nil)
	if e != nil {
		t.Fatal(e)
	}
	w.now = func() time.Time { return time.UnixMilli(work.Job.IssuedAtMS + 10000) }
	if e = w.Run(ctx2); e != nil {
		t.Fatal(e)
	}
	rows, e = box.List(0, 1)
	if e != nil || len(rows) != 1 || rows[0].WorkHash != f.Hash || rows[0].PoolState != "accepted" || rows[0].PoolReceipt == nil || rows[0].Fighter == nil || rows[0].Fighter.AdmissionState != "accepted" {
		t.Fatalf("recovery lost independent outcomes: %+v %v", rows, e)
	}
	if a.submits != 1 || a.issues != issuesBefore || w.Status().Attempts != 0 {
		t.Fatalf("capacity recovery reissued work or started new mining: %+v status%+v", a, w.Status())
	}
}

func TestPoolWorkerByteCapacityStaysPausedAcrossExpiryAndRecoversPending(t *testing.T) {
	c, work, fixture := pwFixture(t, "modeled-all-three-miss", 0, false)
	c.MaxRecords = 2 // Leave a record slot; only the byte reservation may fail.
	box, path := pwBox(t, c)
	scan := PoolScan{Version: PoolVersion1, Work: work}
	if err := box.saveScan(&scan); err != nil {
		t.Fatal(err)
	}
	if err := box.reserve(scan); err != nil {
		t.Fatal(err)
	}
	check, err := poolCheck(PoolEntry{Work: work, Nonce: fixture.Nonce, WorkHash: fixture.Hash}, c)
	if err != nil || check.Header != fixture.Header || check.DuplicateKey != fixture.Key {
		t.Fatal("frozen pending share structure changed", err)
	}
	scan.NextNonce++
	entry := poolEntryForCheck(scan, check)
	entry.NextAttemptMS = work.Job.ExpiresAtMS + 100
	if _, err = box.found(entry, scan); err != nil {
		t.Fatal(err)
	}
	a := &pwAuthority{c: c, w: work, lookup: func() (poolshare.Submission, error) {
		return pwReceipt(c, work, fixture), nil
	}}
	node := &pwNode{}
	w, err := NewPool(c, a, &pwAdmission{c: c}, node, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		t.Fatal("pool-only recovery invoked fighter signer")
		return gymauth.Authorization{}, nil
	}, box)
	if err != nil {
		t.Fatal(err)
	}
	// A private byte-budget seam reaches the real reserve failure without
	// writing 64 MiB of filler records. Existing pending updates retain their
	// reservation. Restore the validated public limit before reopening below.
	box.mu.Lock()
	box.config.MaxBytes = box.meta.Bytes + box.meta.Reserved
	box.mu.Unlock()
	defer func() {
		box.mu.Lock()
		box.config.MaxBytes = c.MaxBytes
		box.mu.Unlock()
	}()
	if err = box.reserve(scan); !errors.Is(err, ErrCapacity) {
		t.Fatalf("test did not reach byte reservation capacity: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now, pauses := work.Job.IssuedAtMS+100, 0
	w.now = func() time.Time { return time.UnixMilli(now) }
	w.check = func(context.Context, *prospectwork.ManifestWork, uint32) (prospectwork.ShareCheck, error) {
		t.Fatal("byte-full worker started a new mining attempt")
		return prospectwork.ShareCheck{}, errors.New("unexpected mining")
	}
	w.wait = func(context.Context, time.Duration) error {
		pauses++
		if status := w.Status(); status.State != "paused" || status.LastError != "capacity" {
			t.Fatalf("expected capacity pause, got %+v", status)
		}
		if pauses == 1 {
			now = work.Job.ExpiresAtMS + 100
			return nil
		}
		cancel()
		return context.Canceled
	}
	if err = w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if pauses != 2 || a.issues != 0 || a.lookups != 1 || a.submits != 0 || w.Status().Attempts != 0 || len(node.calls) != 0 {
		t.Fatalf("capacity pause issued/mined work or blocked pending recovery: pauses=%d authority=%+v status=%+v node=%v", pauses, a, w.Status(), node.calls)
	}
	if _, exists, err := box.scan(); err != nil || exists {
		t.Fatalf("expired scan was not cleared: exists=%v err=%v", exists, err)
	}
	rows, err := box.List(0, 2)
	if err != nil || len(rows) != 1 || rows[0].PoolState != "accepted" || rows[0].PoolReceipt == nil || rows[0].WorkHash != fixture.Hash {
		t.Fatalf("byte-capacity pause lost original pending recovery: %+v %v", rows, err)
	}
	box.mu.Lock()
	box.config.MaxBytes = c.MaxBytes
	box.mu.Unlock()
	if err = box.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenPoolOutbox(path, c)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, err := reopened.List(0, 2)
	if err != nil || !reflect.DeepEqual(after, rows) {
		t.Fatal("capacity recovery failed durable reopen", err)
	}
}

func TestPoolWorkerRejectsSourceRebindingAndSecondRun(t *testing.T) {
	c, work, _ := pwFixture(t, "coinbase-only", 0, false)
	box, _ := pwBox(t, c)
	a := &pwAuthority{c: c, w: work}
	ad := &pwAdmission{c: c}
	a.c.Source.PoolTarget = strings.Repeat("e", 64)
	if w, e := NewPool(c, a, ad, &pwNode{}, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		return gymauth.Authorization{}, nil
	}, box); e == nil || w != nil {
		t.Fatal("rebound share target accepted")
	}
	a.c = c
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w, e := NewPool(c, a, ad, &pwNode{}, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		return gymauth.Authorization{}, nil
	}, box)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	if e = w.Run(ctx); e != nil {
		t.Fatal(e)
	}
	if e = w.Run(ctx); e == nil {
		t.Fatal("single-run worker restarted")
	}
}
