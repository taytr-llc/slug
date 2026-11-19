// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"bytes"
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

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

const poolTestToken = "public-test-session-not-a-real-credential"

type testSource struct {
	Name          string                  `json:"name"`
	Job           prospectjob.ManifestJob `json:"job"`
	Template      prospectwork.Template   `json:"template"`
	PayoutAddress string                  `json:"payout_address"`
	ExtraNonce    uint64                  `json:"extra_nonce"`
	Expected      prospectwork.Candidate  `json:"expected"`
}

type testCase struct {
	Name         string `json:"name"`
	Source       string `json:"source_candidate"`
	Nonce        uint32 `json:"nonce"`
	Header       string `json:"header"`
	WorkHash     string `json:"work_hash"`
	DuplicateKey string `json:"submission_key"`
	Block        bool   `json:"qualifies_block"`
	Prospect     bool   `json:"qualifies_prospect"`
	ShareTargets []struct {
		Target   string `json:"target"`
		Pool     bool   `json:"meets_target"`
		Relation string `json:"relation"`
	} `json:"share_targets"`
	Candidate *prospectwork.Candidate `json:"qualifying_candidate"`
}

type testCorpus struct {
	Sources []testSource `json:"sources"`
	Cases   []testCase   `json:"cases"`
}

func poolFixture(t *testing.T) testCorpus {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-PoW corpus; refusal checked separately")
	}
	raw, err := os.ReadFile("../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("frozen independent work corpus changed")
	}
	var corpus testCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	return corpus
}

func (c testCorpus) source(t *testing.T, name string) testSource {
	t.Helper()
	for _, source := range c.Sources {
		if source.Name == name {
			return source
		}
	}
	t.Fatalf("missing frozen source %q", name)
	return testSource{}
}

func (c testCorpus) caseByName(t *testing.T, name string) testCase {
	t.Helper()
	for _, tc := range c.Cases {
		if tc.Name == name {
			return tc
		}
	}
	t.Fatalf("missing frozen case %q", name)
	return testCase{}
}

func seedPoolAuthority(t *testing.T) (*registry.ManifestStore, registry.ManifestAuthorityPin) {
	t.Helper()
	var f struct {
		Pin    registry.ManifestAuthorityPin `json:"pin"`
		Marker string                        `json:"marker_canonical"`
		Rows   struct {
			Meta          string `json:"meta"`
			Jobs, Records []struct{ Key, Value string }
			Sequences     []struct {
				KeyHex string `json:"key_hex"`
				Value  string `json:"value"`
			}
		} `json:"database_rows"`
	}
	raw, err := os.ReadFile("../registry/testdata/manifest/reference/manifest_admission_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "d32a8653c76c643addb3df091ca8d7dfdba66c2486f2ca305e718113f71d5794" {
		t.Fatal("frozen independent authority corpus changed")
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "issuer")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SLUG-REGISTRY.json"), []byte(f.Marker), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: true})
	if err != nil {
		t.Fatal(err)
	}
	batch := new(leveldb.Batch)
	batch.Put([]byte("meta"), []byte(f.Rows.Meta))
	for _, rows := range [][]struct{ Key, Value string }{f.Rows.Jobs, f.Rows.Records} {
		for _, row := range rows {
			batch.Put([]byte(row.Key), []byte(row.Value))
		}
	}
	for _, row := range f.Rows.Sequences {
		key, err := hex.DecodeString(row.KeyHex)
		if err != nil {
			t.Fatal(err)
		}
		batch.Put(key, []byte(row.Value))
	}
	if err := db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := registry.OpenManifestAuthority(path, f.Pin, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, f.Pin
}

type testPoolSessions struct {
	key     string
	revoked bool
}

func (s *testPoolSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	check := func() error {
		if token != poolTestToken || s.revoked {
			return errors.New("revoked " + token)
		}
		return ctx.Err()
	}
	if err := check(); err != nil {
		return err
	}
	return fn(s.key, check)
}

type testPoolNode struct {
	template                                         prospectwork.Template
	want                                             *testCase
	currentErr, submitErr, canonicalErr              error
	onCurrent                                        func()
	onSubmit                                         func()
	onCanonical                                      func()
	proposals, currents, broadcasts, canonicalChecks int
}

func (n *testPoolNode) ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error) {
	return n.template, nil
}
func (n *testPoolNode) Proposal(context.Context, prospect.Context, string) error {
	n.proposals++
	return nil
}
func (n *testPoolNode) RecheckContext(context.Context, prospect.Context) error {
	n.currents++
	if n.onCurrent != nil {
		n.onCurrent()
	}
	return n.currentErr
}
func (n *testPoolNode) SubmitBlock(_ context.Context, _ prospect.Context, block string) error {
	n.broadcasts++
	if n.want != nil && (n.want.Candidate == nil || block != n.want.Candidate.BlockHex) {
		return errors.New("different block or payout")
	}
	if n.onSubmit != nil {
		n.onSubmit()
	}
	return n.submitErr
}
func (n *testPoolNode) CanonicalBlock(_ context.Context, _ prospect.Context, proof prospect.Proof, hash string) error {
	n.canonicalChecks++
	if n.want != nil && (!reflect.DeepEqual(proof, n.want.Candidate.Proof) || hash != n.want.WorkHash) {
		return errors.New("not exact locally checked winner")
	}
	if n.onCanonical != nil {
		n.onCanonical()
	}
	return n.canonicalErr
}

type poolHarness struct {
	pool             *Pool
	path             string
	config           Config
	authority        *registry.ManifestStore
	node             *testPoolNode
	sessions         *testPoolSessions
	source           testSource
	ordinary, winner testCase
	now              int64
}

func setupPool(t *testing.T, target string, maxSubmissions uint64) *poolHarness {
	t.Helper()
	f := poolFixture(t)
	source := f.source(t, "coinbase-only")
	if target == "" {
		target = strings.Repeat("f", 64)
	}
	if maxSubmissions == 0 {
		maxSubmissions = 16
	}
	authority, pin := seedPoolAuthority(t)
	c := source.Job.Context
	h := &poolHarness{
		path: filepath.Join(t.TempDir(), "pool"), authority: authority, source: source,
		ordinary: f.caseByName(t, "coinbase-only-nonce-0"), winner: f.caseByName(t, "coinbase-only-nonce-4294967295"),
		now:      source.Job.IssuedAtMS + 1000,
		sessions: &testPoolSessions{key: source.Job.PublicKey},
		node:     &testPoolNode{template: source.Template, canonicalErr: errors.New("not canonical")},
		config:   Config{Version: 1, AuthorityPin: pin, Policy: prospectnode.Policy{Version: 1, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}, PayoutAddress: source.PayoutAddress, PoolTarget: target, MaxWorks: 16, MaxSubmissions: maxSubmissions, MaxBytes: 32 << 20},
	}
	h.node.want = &h.winner
	var err error
	h.pool, err = Create(h.path, h.config, h.authority, h.node, h.sessions)
	if err != nil {
		t.Fatal(err)
	}
	h.pool.now = func() time.Time { return time.UnixMilli(h.now) }
	// Seed only independently frozen issued inputs. The source is a real
	// ManifestStore, and saveWork still checks it and builds its own candidate.
	if err := h.pool.saveWork(savedWork{Job: source.Job, Template: source.Template}, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.pool.Close() })
	return h
}

func (h *poolHarness) reopen(t *testing.T) {
	t.Helper()
	if err := h.pool.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	h.pool, err = Open(h.path, h.config, h.authority, h.node, h.sessions)
	if err != nil {
		t.Fatal(err)
	}
	h.pool.now = func() time.Time { return time.UnixMilli(h.now) }
}

func assertFrozenSubmission(t *testing.T, got Submission, tc testCase, source testSource, target string) {
	t.Helper()
	if got.JobID != source.Job.ID || got.GymID != source.Job.GymID || got.Nonce != tc.Nonce || got.Header != tc.Header || got.WorkHash != tc.WorkHash || got.DuplicateKey != tc.DuplicateKey || got.Block != tc.Block || got.Prospect != tc.Prospect || got.PoolTarget != target {
		t.Fatalf("submission differs from independent work evidence: %+v", got)
	}
}

func TestPoolLateWinnerRecoveryAndNoBirth(t *testing.T) {
	h := setupPool(t, "", 1)
	before, err := h.authority.Info()
	if err != nil {
		t.Fatal(err)
	}
	h.now = h.source.Job.ExpiresAtMS
	first, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	assertFrozenSubmission(t, first, h.winner, h.source, h.config.PoolTarget)
	if first.Sequence != 1 || first.AcceptedSequence != 0 || first.AcceptedAtMS != 0 || first.BlockState != "processed" || first.ReceivedAtMS != h.now || h.node.broadcasts != 1 {
		t.Fatalf("processing alone granted late credit or lost evidence: %+v", first)
	}
	h.reopen(t)
	saved, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || saved != first {
		t.Fatalf("pending restart changed first evidence: %+v %v", saved, err)
	}
	h.now++
	h.node.canonicalErr = nil
	accepted, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Sequence != first.Sequence || accepted.ReceivedAtMS != first.ReceivedAtMS || accepted.AcceptedSequence != 1 || accepted.AcceptedAtMS != h.now || accepted.Acceptance != "canonical-late-winner" || accepted.BlockState != "canonical-observed" || accepted.BlockHeight != h.source.Job.Context.ParentHeight+1 {
		t.Fatalf("exact late winner not credited once: %+v", accepted)
	}
	h.reopen(t)
	h.now++
	replay, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Sequence != accepted.Sequence || replay.AcceptedSequence != accepted.AcceptedSequence || replay.AcceptedAtMS != accepted.AcceptedAtMS || replay.ReceivedAtMS != accepted.ReceivedAtMS {
		t.Fatal("replay changed immutable acceptance")
	}
	after, err := h.authority.Info()
	if err != nil || before != after {
		t.Fatal("pool late exception changed fighter authority", err)
	}
	if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce); err == nil {
		t.Fatal("ordinary late share accepted")
	}
}

func TestPoolOrdinaryExpiryCurrentnessAndHistoricalDuplicate(t *testing.T) {
	for _, mode := range []string{"boundary", "guard-crosses-expiry", "node-unavailable", "clock-rollback"} {
		t.Run(mode, func(t *testing.T) {
			h := setupPool(t, "", 0)
			switch mode {
			case "boundary":
				h.now = h.source.Job.ExpiresAtMS
			case "guard-crosses-expiry":
				h.now = h.source.Job.ExpiresAtMS - 1
				h.node.onCurrent = func() { h.now++ }
			case "node-unavailable":
				h.node.currentErr = errors.New("RPC failure")
			case "clock-rollback":
				h.now = h.source.Job.IssuedAtMS
			}
			if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce); err == nil {
				t.Fatal("ineligible ordinary share accepted")
			}
			if _, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce); !errors.Is(err, ErrNotFound) {
				t.Fatal("rejected share left receipt", err)
			}
			if h.node.broadcasts != 0 || h.node.canonicalChecks != 0 {
				t.Fatal("ordinary share entered block exception")
			}
			if mode == "boundary" {
				h.reopen(t)
				h.now--
				if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce); !errors.Is(err, ErrClock) {
					t.Fatal("restart forgot observed expiry clock", err)
				}
			}
		})
	}
	h := setupPool(t, "", 0)
	h.now = h.source.Job.ExpiresAtMS - 1
	got, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	assertFrozenSubmission(t, got, h.ordinary, h.source, h.config.PoolTarget)
	if !got.Pool || got.AcceptedSequence != 1 || got.Acceptance != "current-parent" || got.AcceptedAtMS != h.now {
		t.Fatal("timely pool share not accepted", got)
	}
	h.now = h.source.Job.ExpiresAtMS + 100
	h.node.currentErr = errors.New("offline")
	h.reopen(t)
	replay, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if err != nil || replay != got {
		t.Fatal("historical duplicate changed after expiry", err)
	}
}

func TestPoolBlockOnlyNeedsCanonicalAndBroadcastIsDurable(t *testing.T) {
	f := poolFixture(t)
	winner := f.caseByName(t, "coinbase-only-nonce-4294967295")
	target := ""
	for _, row := range winner.ShareTargets {
		if !row.Pool {
			target = row.Target
		}
	}
	if target == "" {
		t.Fatal("no independent below-hash target")
	}
	h := setupPool(t, target, 0)
	h.node.onSubmit = func() {
		raw, err := h.pool.db.Get(recordKey(h.winner.DuplicateKey), nil)
		if err != nil {
			t.Fatal("broadcast preceded durable intent", err)
		}
		var saved Submission
		if json.Unmarshal(raw, &saved) != nil || saved.AcceptedSequence != 0 || saved.BlockState != "pending" {
			t.Fatal("block-only win got pre-canonical credit")
		}
	}
	first, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || first.Pool || first.AcceptedSequence != 0 {
		t.Fatal("block-only processing credited pool miss", first, err)
	}
	h.node.onSubmit = nil
	h.node.canonicalErr = nil
	h.now++
	got, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || got.AcceptedSequence != 1 || got.Acceptance != "canonical-winning-block" || got.Pool {
		t.Fatal("canonical block-only winner not included", got, err)
	}

	h = setupPool(t, "", 0)
	h.node.onSubmit = func() {
		raw, err := h.pool.db.Get(recordKey(h.winner.DuplicateKey), nil)
		if err != nil {
			t.Fatal(err)
		}
		var saved Submission
		if json.Unmarshal(raw, &saved) != nil || saved.AcceptedSequence != 1 || saved.Acceptance != "current-parent" {
			t.Fatal("timely accepted sequence was not durable before broadcast")
		}
		h.now = h.source.Job.ExpiresAtMS + 100
	}
	got, err = h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || got.AcceptedSequence != 1 || got.AcceptedAtMS >= h.source.Job.ExpiresAtMS || got.Acceptance != "current-parent" {
		t.Fatal("broadcast delay changed timely acceptance", got, err)
	}
}

func TestPoolLateUnknownAndRejectedAreNotCanonical(t *testing.T) {
	for _, tc := range []struct {
		code, state string
		unknown     bool
	}{{"outcome_unknown", "unknown", true}, {"rejected", "rejected", false}} {
		t.Run(tc.state, func(t *testing.T) {
			h := setupPool(t, "", 0)
			h.now = h.source.Job.ExpiresAtMS + 1
			h.node.submitErr = &prospectnode.SubmissionError{Code: tc.code, OutcomeUnknown: tc.unknown}
			got, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
			if err != nil || got.AcceptedSequence != 0 || got.BlockState != tc.state {
				t.Fatal("noncanonical block state granted credit", got, err)
			}
			h.reopen(t)
			saved, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
			if err != nil || saved != got {
				t.Fatal("delivery evidence lost at restart", err)
			}
		})
	}
}

func TestPoolSessionGuardsAndIssue(t *testing.T) {
	h := setupPool(t, "", 0)
	for _, mode := range []string{"wrong-gym", "revoked-final", "cancelled-final"} {
		t.Run(mode, func(t *testing.T) {
			key := h.sessions.key
			defer func() { h.sessions.key = key; h.sessions.revoked = false; h.node.onCurrent = nil }()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "wrong-gym":
				h.sessions.key = "02c6047f9441ed7d6d3045406e95c07cd85aef011134fd65ddfa65673a7ee568f"
			case "revoked-final":
				h.node.onCurrent = func() { h.sessions.revoked = true }
			case "cancelled-final":
				h.node.onCurrent = cancel
			}
			_, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
			if err == nil || strings.Contains(err.Error(), poolTestToken) {
				t.Fatal("session failure accepted or leaked token", err)
			}
		})
	}
	if h.node.broadcasts != 0 {
		t.Fatal("unauthorized block transport")
	}
	h.pool.now = time.Now
	issued, err := h.pool.Issue(context.Background(), poolTestToken)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := h.authority.GetManifestJob(issued.Job.ID)
	if err != nil || stored != issued.Job || issued.Job.ID == h.source.Job.ID || issued.Job.ExpiresAtMS-issued.Job.IssuedAtMS != prospectjob.LifetimeMS || issued.PoolTarget != h.config.PoolTarget || issued.Candidate.PayoutAddress != h.config.PayoutAddress {
		t.Fatal("issue not bound to real immutable source", err)
	}
	if h.node.proposals != 1 {
		t.Fatal("new work omitted proposal validation")
	}
	issued.Candidate.Proof.MerkleBranch = append(issued.Candidate.Proof.MerkleBranch, "mutated")
	recovered, err := h.pool.Work(context.Background(), poolTestToken, issued.Job.ID)
	if err != nil || reflect.DeepEqual(recovered.Candidate.Proof.MerkleBranch, issued.Candidate.Proof.MerkleBranch) {
		t.Fatal("issued candidate aliases saved work", err)
	}
	iterator := h.pool.db.NewIterator(nil, nil)
	defer iterator.Release()
	for iterator.Next() {
		if bytes.Contains(iterator.Value(), []byte(poolTestToken)) {
			t.Fatal("bearer persisted")
		}
	}
	if err := iterator.Error(); err != nil {
		t.Fatal(err)
	}
}
