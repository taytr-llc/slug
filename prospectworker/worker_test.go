//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectworker

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type wkFixture struct {
	Job           prospectjob.Job        `json:"job"`
	Template      prospectwork.Template  `json:"template"`
	Candidate     prospectwork.Candidate `json:"candidate"`
	Authorization gymauth.Authorization  `json:"authorization"`
	Verified      prospect.Verified      `json:"expected_verified"`
}

func wkLoad(t *testing.T) wkFixture {
	t.Helper()
	raw, err := os.ReadFile("../registry/testdata/mining_winner_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f wkFixture
	if json.Unmarshal(raw, &f) != nil {
		t.Fatal("independent fixture")
	}
	return f
}
func wkConfig(f wkFixture) Config {
	return Config{Version: 1, Policy: prospectnode.Policy{Version: 1, Network: f.Job.Context.Network, Genesis: f.Job.Context.Genesis, DNAVersion: f.Job.Context.DNAVersion, ProspectTarget: f.Job.Context.ProspectTarget}, PublicKey: f.Job.PublicKey, PayoutAddress: f.Candidate.PayoutAddress, AuthorityOrigin: "http://127.0.0.1:29440", MaxRecords: 10, MaxBytes: 64 << 20}
}
func wkScan(f wkFixture) Scan {
	return Scan{Version: 1, Job: f.Job, Template: f.Template, Candidate: f.Candidate}
}
func wkSetup(t *testing.T) (wkFixture, Config, *Outbox, string) {
	t.Helper()
	f := wkLoad(t)
	config := wkConfig(f)
	path := filepath.Join(t.TempDir(), "outbox")
	box, err := CreateOutbox(path, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = box.Close() })
	return f, config, box, path
}
func wkSign(ctx context.Context, context prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
	key, _ := btcec.PrivKeyFromBytes([]byte{1})
	return gymauth.SignAdmission(context, proof, key)
}
func wkReceipt(f wkFixture, auth gymauth.Authorization) prospectservice.AdmissionResult {
	return prospectservice.AdmissionResult{Version: 2, ContextSource: "trusted-node-snapshot", CurrentNodeChecked: true, Authentication: "gym-key-control", BirthRule: "job-issuance", Inserted: true, Record: registry.Record{Version: 4, Sequence: 1, Context: f.Job.Context, Proof: f.Candidate.Proof, Verified: f.Verified, Authorization: &auth, Job: &f.Job, Birth: &registry.Birth{RuleVersion: 1, BornAtMS: f.Job.IssuedAtMS, AcceptedAtMS: f.Job.IssuedAtMS + 1234}, Eligibility: &registry.Eligibility{RuleVersion: 2, Source: "canonical-winning-block", Hash: f.Verified.WorkHash, Height: f.Job.Context.ParentHeight + 1}}}
}

type wkAuthority struct {
	version         uint32
	origin          string
	job             prospectjob.Job
	issues, submits int
	issue           func() prospectjob.Job
	submit          func(prospectjob.Job, prospect.Proof, gymauth.Authorization) (prospectservice.AdmissionResult, error)
}

func (a *wkAuthority) ProtocolVersion() uint32 {
	if a.version == 0 {
		return 2
	}
	return a.version
}
func (a *wkAuthority) AuthorityOrigin() string {
	if a.origin == "" {
		return "http://127.0.0.1:29440"
	}
	return a.origin
}
func (a *wkAuthority) IssueJob(context.Context, string) (prospectjob.Job, error) {
	a.issues++
	if a.issue != nil {
		return a.issue(), nil
	}
	return a.job, nil
}
func (a *wkAuthority) Submit(ctx context.Context, job prospectjob.Job, proof prospect.Proof, auth gymauth.Authorization) (prospectservice.AdmissionResult, error) {
	a.submits++
	if a.submit != nil {
		return a.submit(job, proof, auth)
	}
	return prospectservice.AdmissionResult{}, &prospectservice.ClientError{Code: "expired"}
}

type wkNode struct {
	template                        prospectwork.Template
	broadcasts, canonical, rechecks int
	broadcast                       func(string) error
	confirm                         func() error
	recheck                         func() error
}

func (n *wkNode) ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error) {
	return n.template, nil
}
func (n *wkNode) Proposal(context.Context, prospect.Context, string) error { return nil }
func (n *wkNode) RecheckContext(context.Context, prospect.Context) error {
	n.rechecks++
	if n.recheck != nil {
		return n.recheck()
	}
	return nil
}
func (n *wkNode) SubmitBlock(ctx context.Context, c prospect.Context, block string) error {
	n.broadcasts++
	if n.broadcast != nil {
		return n.broadcast(block)
	}
	return nil
}
func (n *wkNode) CanonicalBlock(context.Context, prospect.Context, prospect.Proof, string) error {
	n.canonical++
	if n.confirm != nil {
		return n.confirm()
	}
	return nil
}
func wkWorker(t *testing.T, config Config, box *Outbox, f wkFixture, a *wkAuthority, n *wkNode, signer Signer) *Worker {
	t.Helper()
	worker, err := New(config, a, n, signer, box)
	if err != nil {
		t.Fatal(err)
	}
	worker.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS + 1000) }
	return worker
}

func TestWorkerProductionFoundBroadcastSignPersistSubmit(t *testing.T) {
	f, config, box, _ := wkSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &wkAuthority{job: f.Job}
	n := &wkNode{template: f.Template}
	var events []string
	n.broadcast = func(block string) error {
		entries, err := box.List(0, 10)
		if err != nil || len(entries) != 1 || entries[0].Authorization != nil || entries[0].Candidate.BlockHex != block {
			t.Fatal("block transport preceded durable found work")
		}
		events = append(events, "block")
		return nil
	}
	signer := func(ctx context.Context, c prospect.Context, p prospect.Proof) (gymauth.Authorization, error) {
		if !reflect.DeepEqual(events, []string{"block"}) {
			t.Fatal("signing preceded broadcast")
		}
		events = append(events, "sign")
		return wkSign(ctx, c, p)
	}
	a.submit = func(job prospectjob.Job, proof prospect.Proof, auth gymauth.Authorization) (prospectservice.AdmissionResult, error) {
		entries, err := box.List(0, 10)
		if err != nil || len(entries) != 1 || entries[0].Authorization == nil || *entries[0].Authorization != auth {
			t.Fatal("admission preceded durable signature")
		}
		events = append(events, "admit")
		cancel()
		return wkReceipt(f, auth), nil
	}
	worker := wkWorker(t, config, box, f, a, n, signer)
	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}
	entries, err := box.List(0, 10)
	if err != nil || len(entries) != 1 || entries[0].BlockState != "canonical" || entries[0].AdmissionState != "accepted" || entries[0].WorkHash != f.Verified.WorkHash {
		t.Fatalf("wrong acknowledged record: %+v %v", entries, err)
	}
	if !reflect.DeepEqual(events, []string{"block", "sign", "admit"}) || worker.Status().Attempts != 1 {
		t.Fatal("wrong worker ordering/attempt count")
	}
	if _, exists, err := box.scan(); err != nil || exists {
		t.Fatal("canonical winner did not invalidate current scan")
	}
}

func TestWorkerLostAcknowledgmentReopensAfterExpiry(t *testing.T) {
	f, config, box, path := wkSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	a := &wkAuthority{job: f.Job}
	n := &wkNode{template: f.Template}
	var original gymauth.Authorization
	a.submit = func(job prospectjob.Job, p prospect.Proof, auth gymauth.Authorization) (prospectservice.AdmissionResult, error) {
		original = auth
		cancel()
		return prospectservice.AdmissionResult{}, &prospectservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	worker := wkWorker(t, config, box, f, a, n, wkSign)
	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := box.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenOutbox(path, config)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	ctx, cancel = context.WithCancel(context.Background())
	a = &wkAuthority{job: f.Job}
	n = &wkNode{template: f.Template, broadcast: func(string) error { t.Fatal("rebroadcast acknowledged canonical block"); return nil }}
	a.submit = func(job prospectjob.Job, p prospect.Proof, auth gymauth.Authorization) (prospectservice.AdmissionResult, error) {
		if auth != original || job != f.Job || !reflect.DeepEqual(p, f.Candidate.Proof) {
			t.Fatal("retry replaced original evidence")
		}
		result := wkReceipt(f, auth)
		result.Inserted = false
		result.CurrentNodeChecked = false
		result.ContextSource = "historical-receipt"
		cancel()
		return result, nil
	}
	worker = wkWorker(t, config, reopened, f, a, n, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		t.Fatal("retry called signer")
		return gymauth.Authorization{}, nil
	})
	worker.now = func() time.Time { return time.UnixMilli(f.Job.ExpiresAtMS + 1000) }
	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}
	entries, err := reopened.List(0, 10)
	if err != nil || len(entries) != 1 || entries[0].AdmissionState != "accepted" || entries[0].Receipt.Record.Birth.BornAtMS != f.Job.IssuedAtMS || worker.Status().Attempts != 0 {
		t.Fatal("historical recovery failed")
	}
}

func TestWorkerCancellationDrainsHashAndPersistsFound(t *testing.T) {
	f, config, box, _ := wkSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	a := &wkAuthority{job: f.Job}
	n := &wkNode{template: f.Template}
	worker := wkWorker(t, config, box, f, a, n, wkSign)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	digest, _ := chainhash.NewHashFromStr(f.Verified.WorkHash)
	worker.hash = func(*wire.BlockHeader) chainhash.Hash { close(entered); <-release; return *digest }
	go func() { finished <- worker.Run(ctx) }()
	<-entered
	cancel()
	if err := box.Close(); !errors.Is(err, ErrInUse) {
		t.Fatal("closed store during active hash")
	}
	select {
	case err := <-finished:
		t.Fatalf("returned before real hash completed: %v", err)
	default:
	}
	_ = worker.Status()
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	entries, err := box.List(0, 10)
	if err != nil || len(entries) != 1 || entries[0].Authorization != nil || n.broadcasts != 0 || a.submits != 0 || worker.Status().Attempts != 1 {
		t.Fatal("cancelled hash lost work or performed new transport")
	}
	scan, exists, err := box.scan()
	if err != nil || !exists || scan.NextNonce != 1 {
		t.Fatal("found record and scan cursor were not atomic")
	}
}

func wkHarder(t *testing.T, f wkFixture) wkFixture {
	t.Helper()
	f.Job.Context.ProspectTarget = strings.Repeat("1", 64)
	job, err := prospectjob.New(f.Job.Context, f.Job.PublicKey, f.Job.Challenge, f.Job.IssuedAtMS)
	if err != nil {
		t.Fatal(err)
	}
	f.Job = job
	f.Template.Context = job.Context
	f.Candidate, err = prospectwork.Build(job, f.Template, f.Candidate.PayoutAddress, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func wkFF() chainhash.Hash {
	var hash chainhash.Hash
	for i := range hash {
		hash[i] = 255
	}
	return hash
}
func TestWorkerNonceExhaustionAndRenewal(t *testing.T) {
	for _, mode := range []string{"nonce", "renewal", "parent"} {
		t.Run(mode, func(t *testing.T) {
			f := wkHarder(t, wkLoad(t))
			config := wkConfig(f)
			box, err := CreateOutbox(filepath.Join(t.TempDir(), "box"), config)
			if err != nil {
				t.Fatal(err)
			}
			defer box.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			now := time.UnixMilli(f.Job.IssuedAtMS + 1000)
			a := &wkAuthority{job: f.Job}
			n := &wkNode{template: f.Template}
			worker := wkWorker(t, config, box, f, a, n, wkSign)
			worker.now = func() time.Time { return now }
			worker.wait = func(context.Context, time.Duration) error { return nil }
			if mode == "nonce" {
				scan := wkScan(f)
				scan.NextNonce = math.MaxUint32
				if err := box.saveScan(&scan); err != nil {
					t.Fatal(err)
				}
			}
			a.issue = func() prospectjob.Job {
				if a.issues == 1 {
					return f.Job
				}
				job, err := prospectjob.New(f.Job.Context, f.Job.PublicKey, strings.Repeat("22", 32), now.UnixMilli())
				if err != nil {
					t.Fatal(err)
				}
				return job
			}
			if mode == "parent" {
				n.recheck = func() error {
					if n.rechecks == 1 {
						return errors.New("stale")
					}
					return nil
				}
			}
			var nonces []uint32
			var roots []chainhash.Hash
			worker.hash = func(header *wire.BlockHeader) chainhash.Hash {
				nonces = append(nonces, header.Nonce)
				roots = append(roots, header.MerkleRoot)
				if len(nonces) == 1 {
					if mode == "renewal" {
						now = now.Add(5 * time.Minute)
					} else if mode == "parent" {
						now = now.Add(6 * time.Second)
					}
				} else {
					cancel()
				}
				return wkFF()
			}
			if err := worker.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if len(nonces) != 2 {
				t.Fatal("wrong attempt count")
			}
			if mode == "nonce" {
				if nonces[0] != math.MaxUint32 || nonces[1] != 0 || roots[0] == roots[1] {
					t.Fatal("nonce rollover did not rebuild extra nonce")
				}
				scan, _, _ := box.scan()
				if scan.ExtraNonce != 1 || scan.NextNonce != 1 {
					t.Fatal("nonce rollover cursor")
				}
			} else if a.issues != 2 || roots[0] == roots[1] {
				t.Fatal("renewal reused old job work")
			}
		})
	}
}

func TestWorkerUnresolvedBlockDoesNotStarveNewHash(t *testing.T) {
	f, config, box, _ := wkSetup(t)
	scan := wkScan(f)
	if err := box.saveScan(&scan); err != nil {
		t.Fatal(err)
	}
	scan.NextNonce = 1
	entry := Entry{Version: 1, WorkHash: f.Verified.WorkHash, Job: f.Job, Candidate: f.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"}
	if _, err := box.found(entry, scan); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &wkAuthority{job: f.Job}
	n := &wkNode{template: f.Template, broadcast: func(string) error { return &prospectnode.SubmissionError{Code: "rejected"} }, confirm: func() error { return errors.New("not canonical") }}
	worker := wkWorker(t, config, box, f, a, n, wkSign)
	digest, _ := chainhash.NewHashFromStr(f.Verified.WorkHash)
	worker.hash = func(header *wire.BlockHeader) chainhash.Hash {
		if header.Nonce == 0 {
			return *digest
		}
		cancel()
		return wkFF()
	}
	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}
	entries, err := box.List(0, 10)
	if err != nil || len(entries) != 2 || entries[0].BlockState != "submitted" || entries[0].AdmissionState != "expired" || entries[0].NextAttemptMS == 0 || worker.Status().Attempts != 1 {
		t.Fatalf("unresolved evidence starved/disappeared: %+v %v", entries, err)
	}
}

func TestWorkerRequiresV2AndBoundAuthority(t *testing.T) {
	f, config, box, _ := wkSetup(t)
	a := &wkAuthority{job: f.Job, version: 1}
	n := &wkNode{template: f.Template}
	if _, err := New(config, a, n, wkSign, box); err == nil {
		t.Fatal("accepted v1 authority")
	}
	a.version = 2
	a.origin = "http://127.0.0.1:29441"
	if _, err := New(config, a, n, wkSign, box); err == nil {
		t.Fatal("changed outbox authority")
	}
}

func TestWorkerBlockAndProspectTargetsAreIndependent(t *testing.T) {
	for _, mode := range []string{"block-only", "prospect-only"} {
		t.Run(mode, func(t *testing.T) {
			f := wkLoad(t)
			if mode == "block-only" {
				f = wkHarder(t, f)
			}
			config := wkConfig(f)
			box, err := CreateOutbox(filepath.Join(t.TempDir(), "box"), config)
			if err != nil {
				t.Fatal(err)
			}
			defer box.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a := &wkAuthority{job: f.Job}
			n := &wkNode{template: f.Template}
			signCalls := 0
			signer := func(ctx context.Context, c prospect.Context, p prospect.Proof) (gymauth.Authorization, error) {
				signCalls++
				if mode == "block-only" {
					t.Fatal("block-only win called signer")
				}
				return wkSign(ctx, c, p)
			}
			n.confirm = func() error {
				if mode == "prospect-only" {
					t.Fatal("prospect-only share queried canonical block")
				}
				cancel()
				return nil
			}
			a.submit = func(prospectjob.Job, prospect.Proof, gymauth.Authorization) (prospectservice.AdmissionResult, error) {
				if mode == "block-only" {
					t.Fatal("block-only win attempted fighter admission")
				}
				cancel()
				return prospectservice.AdmissionResult{}, &prospectservice.ClientError{Code: "expired"}
			}
			worker := wkWorker(t, config, box, f, a, n, signer)
			if mode == "block-only" {
				digest, _ := chainhash.NewHashFromStr(wkLoad(t).Verified.WorkHash)
				worker.hash = func(*wire.BlockHeader) chainhash.Hash { return *digest }
			} else {
				worker.hash = func(*wire.BlockHeader) chainhash.Hash { return wkFF() }
			}
			if err := worker.Run(ctx); err != nil {
				t.Fatal(err)
			}
			entries, err := box.List(0, 10)
			if err != nil || len(entries) != 1 {
				t.Fatal("expected one classified record")
			}
			if mode == "block-only" {
				if !entries[0].BlockWin || entries[0].ProspectWin || n.broadcasts != 1 || signCalls != 0 || a.submits != 0 {
					t.Fatal("block-only threshold was treated as prospect")
				}
			} else if entries[0].BlockWin || !entries[0].ProspectWin || n.broadcasts != 0 || signCalls != 1 || a.submits != 1 {
				t.Fatal("prospect-only threshold was treated as block")
			}
		})
	}
}

func TestWorkerRetainedCapacityPausesBeforeMoreWork(t *testing.T) {
	f := wkLoad(t)
	config := wkConfig(f)
	config.MaxRecords = 1
	box, err := CreateOutbox(filepath.Join(t.TempDir(), "box"), config)
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &wkAuthority{job: f.Job}
	n := &wkNode{template: f.Template}
	a.submit = func(job prospectjob.Job, p prospect.Proof, auth gymauth.Authorization) (prospectservice.AdmissionResult, error) {
		return wkReceipt(f, auth), nil
	}
	worker := wkWorker(t, config, box, f, a, n, wkSign)
	worker.wait = func(context.Context, time.Duration) error {
		if worker.Status().LastError != "capacity" {
			t.Fatal("unexpected pause")
		}
		cancel()
		return context.Canceled
	}
	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if worker.Status().Attempts != 1 || a.issues != 1 || worker.Status().Records != 1 {
		t.Fatal("full retained outbox kept issuing or hashing")
	}
}

func TestWorkerRejectsAdapterPolicyBeforeProposal(t *testing.T) {
	f, config, box, _ := wkSetup(t)
	changed := wkHarder(t, f)
	a := &wkAuthority{job: changed.Job}
	n := &wkNode{template: changed.Template}
	worker := wkWorker(t, config, box, f, a, n, wkSign)
	if _, err := worker.prepare(context.Background()); err == nil {
		t.Fatal("accepted job with unconfigured prospect target")
	}
}

func TestWorkerExtraNonceExhaustionRequestsFreshJob(t *testing.T) {
	f := wkHarder(t, wkLoad(t))
	config := wkConfig(f)
	box, err := CreateOutbox(filepath.Join(t.TempDir(), "box"), config)
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	scan := wkScan(f)
	scan.ExtraNonce = math.MaxUint64
	scan.NextNonce = 1 << 32
	scan.Candidate, err = prospectwork.Build(scan.Job, scan.Template, config.PayoutAddress, scan.ExtraNonce)
	if err != nil {
		t.Fatal(err)
	}
	if err := box.saveScan(&scan); err != nil {
		t.Fatal(err)
	}
	fresh, err := prospectjob.New(f.Job.Context, f.Job.PublicKey, strings.Repeat("22", 32), f.Job.IssuedAtMS)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &wkAuthority{job: fresh}
	n := &wkNode{template: f.Template}
	worker := wkWorker(t, config, box, f, a, n, wkSign)
	worker.hash = func(header *wire.BlockHeader) chainhash.Hash {
		if header.Nonce != 0 {
			t.Fatal("new job nonce was not reset")
		}
		cancel()
		return wkFF()
	}
	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}
	current, exists, err := box.scan()
	if err != nil || !exists || current.ExtraNonce != 0 || current.NextNonce != 1 || current.Job.ID != fresh.ID || a.issues != 1 {
		t.Fatal("extra nonce overflow reused old work")
	}
}

func TestWorkerCancellationDrainsSignerWithoutSubmitting(t *testing.T) {
	f, config, box, _ := wkSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	a := &wkAuthority{job: f.Job}
	n := &wkNode{template: f.Template}
	entered, release := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	signer := func(ctx context.Context, c prospect.Context, p prospect.Proof) (gymauth.Authorization, error) {
		close(entered)
		<-release
		return wkSign(context.Background(), c, p)
	}
	worker := wkWorker(t, config, box, f, a, n, signer)
	go func() { result <- worker.Run(ctx) }()
	<-entered
	cancel()
	if err := box.Close(); !errors.Is(err, ErrInUse) {
		t.Fatal("closed outbox while signer still active")
	}
	select {
	case err := <-result:
		t.Fatalf("Run abandoned signer: %v", err)
	default:
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	entries, err := box.List(0, 10)
	if err != nil || len(entries) != 1 || entries[0].Authorization == nil || entries[0].AdmissionState != "pending" || a.submits != 0 {
		t.Fatal("cancelled signer result lost or sent after shutdown")
	}
}

func TestWorkerFreshBlockBypassesOlderDueSigner(t *testing.T) {
	f, config, box, _ := wkSetup(t)
	scan := wkScan(f)
	if err := box.saveScan(&scan); err != nil {
		t.Fatal(err)
	}
	first := Entry{Version: 1, WorkHash: f.Verified.WorkHash, Job: f.Job, Candidate: f.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"}
	scan.NextNonce = 1
	if _, err := box.found(first, scan); err != nil {
		t.Fatal(err)
	}
	second := Entry{Version: 1, WorkHash: wkFF().String(), Job: f.Job, Candidate: withNonce(f.Candidate, 1), ProspectWin: true, BlockState: "not-applicable", AdmissionState: "needs-signature"}
	scan.NextNonce = 2
	if _, err := box.found(second, scan); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &wkAuthority{job: f.Job}
	n := &wkNode{template: f.Template, confirm: func() error { return errors.New("not canonical") }}
	nonceOf := func(header string) uint32 {
		raw, err := hex.DecodeString(header)
		if err != nil {
			t.Fatal(err)
		}
		return binary.LittleEndian.Uint32(raw[76:80])
	}
	var broadcasts []uint32
	n.broadcast = func(block string) error {
		nonce := nonceOf(block[:160])
		broadcasts = append(broadcasts, nonce)
		if nonce == 2 {
			cancel()
		}
		return nil
	}
	signer := func(ctx context.Context, c prospect.Context, p prospect.Proof) (gymauth.Authorization, error) {
		if nonceOf(p.Header) == 1 {
			t.Fatal("old pending signer delayed new block broadcast")
		}
		return wkSign(ctx, c, p)
	}
	worker := wkWorker(t, config, box, f, a, n, signer)
	oldHash, _ := chainhash.NewHashFromStr(f.Verified.WorkHash)
	worker.hash = func(header *wire.BlockHeader) chainhash.Hash {
		if header.Nonce == 0 {
			return *oldHash
		}
		if header.Nonce != 2 {
			t.Fatal("unexpected recovery or scan")
		}
		return chainhash.Hash{2}
	}
	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(broadcasts, []uint32{0, 2}) {
		t.Fatalf("wrong block priority %v", broadcasts)
	}
}

func TestWorkerOwnsAdapterTemplateContainers(t *testing.T) {
	f := wkHarder(t, wkLoad(t))
	config := wkConfig(f)
	box, err := CreateOutbox(filepath.Join(t.TempDir(), "box"), config)
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &wkAuthority{job: f.Job}
	n := &wkNode{template: f.Template}
	worker := wkWorker(t, config, box, f, a, n, wkSign)
	worker.hash = func(*wire.BlockHeader) chainhash.Hash {
		*n.template.Block.CoinbaseValue = 0
		n.template.Block.CoinbaseAux.Flags = ""
		cancel()
		return wkFF()
	}
	if err := worker.Run(ctx); err != nil {
		t.Fatal("caller-owned template mutation reached active scan", err)
	}
	saved, exists, err := box.scan()
	if err != nil || !exists || *saved.Template.Block.CoinbaseValue == 0 || saved.Template.Block.CoinbaseAux.Flags == "" {
		t.Fatal("saved scan aliased adapter containers")
	}
}
