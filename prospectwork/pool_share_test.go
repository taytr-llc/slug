// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectwork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

type poolShareSource struct {
	Name          string                  `json:"name"`
	Job           prospectjob.ManifestJob `json:"job"`
	Template      Template                `json:"template"`
	PayoutAddress string                  `json:"payout_address"`
	ExtraNonce    uint64                  `json:"extra_nonce"`
	Expected      Candidate               `json:"expected"`
}

type poolShareCase struct {
	Name         string `json:"name"`
	Source       string `json:"source_candidate"`
	Nonce        uint32 `json:"nonce"`
	Header       string `json:"header"`
	WorkHash     string `json:"work_hash"`
	DuplicateKey string `json:"submission_key"`
	Prospect     bool   `json:"qualifies_prospect"`
	Block        bool   `json:"qualifies_block"`
	ShareTargets []struct {
		Relation string `json:"relation"`
		Target   string `json:"target"`
		Pool     bool   `json:"meets_target"`
	} `json:"share_targets"`
	QualifyingCandidate *Candidate `json:"qualifying_candidate"`
}

type poolShareCorpus struct {
	Version uint32            `json:"version"`
	Sources []poolShareSource `json:"sources"`
	Cases   []poolShareCase   `json:"cases"`
}

func readPoolShareCorpus(t *testing.T) poolShareCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/pool_share_reference/pool_share_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("independent pool-share corpus changed")
	}
	var corpus poolShareCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	targets := 0
	for _, tc := range corpus.Cases {
		targets += len(tc.ShareTargets)
	}
	if corpus.Version != 1 || len(corpus.Sources) != 3 || len(corpus.Cases) != 5 || targets != 13 {
		t.Fatal("independent corpus lost a source, header or target case")
	}
	return corpus
}

func (c poolShareCorpus) source(t *testing.T, name string) poolShareSource {
	t.Helper()
	for _, source := range c.Sources {
		if source.Name == name {
			return source
		}
	}
	t.Fatalf("missing independent source %q", name)
	return poolShareSource{}
}

func preparePoolShare(t *testing.T, source poolShareSource, target string) *ManifestWork {
	t.Helper()
	w, err := PrepareManifestWork(source.Job, source.Template, source.PayoutAddress, source.ExtraNonce, target)
	if err != nil || w == nil {
		t.Fatalf("prepare independent work: %v", err)
	}
	return w
}

func wantPoolShare(source poolShareSource, tc poolShareCase, pool bool) ShareCheck {
	return ShareCheck{
		JobID: source.Job.ID, GymID: source.Job.GymID, Nonce: tc.Nonce,
		Header: tc.Header, WorkHash: tc.WorkHash, DuplicateKey: tc.DuplicateKey,
		Pool: pool, Prospect: tc.Prospect, Block: tc.Block,
		Candidate: tc.QualifyingCandidate,
	}
}

// The five headers, normal Argon digests, duplicate keys and full candidate
// bytes are independently frozen Python outputs. No runtime-derived expected
// PoW or candidate is used, and the synthetic inputs make no admission claim.
func TestPoolShareIndependentReference(t *testing.T) {
	corpus := readPoolShareCorpus(t)
	if wire.TestPoW {
		t.Skip("normal vectors; explicit profile refusal is tested separately")
	}
	var sawAllMiss, sawBlockWithoutPool, sawProspectWithoutBlock bool
	keys := make(map[string]string)
	for _, tc := range corpus.Cases {
		source := corpus.source(t, tc.Source)
		for _, target := range tc.ShareTargets {
			t.Run(tc.Name+"/"+target.Relation, func(t *testing.T) {
				w := preparePoolShare(t, source, target.Target)
				if w.Job() != source.Job || w.PoolTarget() != target.Target || !reflect.DeepEqual(w.Candidate(), source.Expected) {
					t.Fatal("preparation changed the original job, pool target or complete nonce-zero candidate")
				}
				got, err := w.CheckNonce(context.Background(), tc.Nonce)
				if err != nil || !reflect.DeepEqual(got, wantPoolShare(source, tc, target.Pool)) {
					t.Fatalf("fixed header/targets/full candidate mismatch: error=%v\ngot=%+v\nwant=%+v", err, got, wantPoolShare(source, tc, target.Pool))
				}
				if !reflect.DeepEqual(w.Candidate(), source.Expected) || w.Job() != source.Job {
					t.Fatal("nonce checking changed retained preparation")
				}
				// Reassigning only the pool target must not change duplicate
				// identity, the checked bytes, or the other two classifications.
				if previous, ok := keys[tc.Header]; ok && previous != got.DuplicateKey {
					t.Fatal("pool target changed the duplicate key")
				}
				keys[tc.Header] = got.DuplicateKey
				sawAllMiss = sawAllMiss || !got.Pool && !got.Prospect && !got.Block && got.Candidate == nil
				sawBlockWithoutPool = sawBlockWithoutPool || got.Block && !got.Pool && got.Candidate != nil
				sawProspectWithoutBlock = sawProspectWithoutBlock || got.Prospect && !got.Block && got.Candidate != nil
			})
		}
	}
	if !sawAllMiss || !sawBlockWithoutPool || !sawProspectWithoutBlock {
		t.Fatal("target-independence or all-three-miss coverage missing")
	}
	unique := make(map[string]bool)
	for _, key := range keys {
		unique[key] = true
	}
	if len(keys) != 5 || len(unique) != 5 {
		t.Fatal("distinct fixed headers did not retain distinct duplicate keys")
	}
}

func TestPoolShareOwnedInputsAndOutputs(t *testing.T) {
	if wire.TestPoW {
		t.Skip("profile refusal covered separately")
	}
	corpus := readPoolShareCorpus(t)
	source := corpus.source(t, "witness-odd-two-level-dependent")
	var tc poolShareCase
	for _, candidate := range corpus.Cases {
		if candidate.Source == source.Name {
			tc = candidate
			break
		}
	}
	if tc.QualifyingCandidate == nil || len(source.Expected.Proof.MerkleBranch) == 0 {
		t.Fatal("ownership fixture requires a full witness candidate with a branch")
	}
	// Own the mutable caller inputs separately from the immutable expectations.
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var input poolShareSource
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	target := tc.ShareTargets[1]
	w := preparePoolShare(t, input, target.Target)
	input.Job.ID = "changed"
	input.Job.Context.ProspectTarget = "changed"
	input.Template.Context.ParentHash = "changed"
	*input.Template.Block.CoinbaseValue = 0
	input.Template.Block.CoinbaseAux.Flags = "changed"
	input.Template.Block.Mutable[0] = "changed"
	input.Template.Block.Capabilities[0] = "changed"
	for i := range input.Template.Block.Transactions {
		input.Template.Block.Transactions[i].Data = "changed"
		for j := range input.Template.Block.Transactions[i].Depends {
			input.Template.Block.Transactions[i].Depends[j] = -1
		}
	}
	job := w.Job()
	job.ID = "changed"
	job.Context.ProspectTarget = "changed"
	copy := w.Candidate()
	copy.BlockHex, copy.Proof.Header, copy.Proof.Coinbase = "changed", "changed", "changed"
	copy.Proof.MerkleBranch[0] = "changed"
	if w.Job() != source.Job || w.PoolTarget() != target.Target || !reflect.DeepEqual(w.Candidate(), source.Expected) {
		t.Fatal("caller input or getter mutation changed retained work")
	}
	first, err := w.CheckNonce(context.Background(), tc.Nonce)
	if err != nil || !reflect.DeepEqual(first, wantPoolShare(source, tc, target.Pool)) {
		t.Fatal("mutating caller inputs changed checked work", err)
	}
	first.Candidate.Proof.MerkleBranch[0] = "changed"
	first.Candidate.Proof.Coinbase = "changed"
	first.Candidate.BlockHex = "changed"
	second, err := w.CheckNonce(context.Background(), tc.Nonce)
	if err != nil || !reflect.DeepEqual(second, wantPoolShare(source, tc, target.Pool)) || !reflect.DeepEqual(w.Candidate(), source.Expected) {
		t.Fatal("returned qualifying candidate aliases retained work or a later result", err)
	}
	// Concurrent getter copies must also have independently mutable branches.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidate := w.Candidate()
			if !reflect.DeepEqual(candidate, source.Expected) {
				t.Error("concurrent getter observed a changed retained candidate")
			}
			candidate.Proof.MerkleBranch[0] = "mutated owned copy"
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(w.Candidate(), source.Expected) {
		t.Fatal("concurrent getter mutation changed retained work")
	}
}

func TestPoolSharePoolOnlyQualification(t *testing.T) {
	if wire.TestPoW {
		t.Skip("profile refusal covered separately")
	}
	corpus := readPoolShareCorpus(t)
	source := corpus.source(t, "modeled-all-three-miss")
	for _, tc := range corpus.Cases {
		if tc.Source != source.Name {
			continue
		}
		if tc.Nonce != 0 || tc.Prospect || tc.Block || tc.QualifyingCandidate != nil {
			t.Fatal("pool-only control no longer starts from the frozen all-three-miss header")
		}
		// Only the pool target changes to the independently frozen hash.
		// The nonce-zero candidate is also frozen, not rebuilt as an oracle.
		w := preparePoolShare(t, source, tc.WorkHash)
		want := wantPoolShare(source, tc, true)
		want.Candidate = &source.Expected
		got, err := w.CheckNonce(context.Background(), tc.Nonce)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("pool-only equality must expose its full candidate: %v\ngot=%+v\nwant=%+v", err, got, want)
		}
		return
	}
	t.Fatal("missing frozen all-three-miss header")
}

func TestPoolSharePreparationRefusals(t *testing.T) {
	if wire.TestPoW {
		t.Skip("profile refusal covered separately")
	}
	source := readPoolShareCorpus(t).Sources[0]
	badTargets := []string{
		"", strings.Repeat("0", 64), strings.Repeat("F", 64), strings.Repeat("f", 63),
		strings.Repeat("f", 65), "0x" + strings.Repeat("f", 62), "g" + strings.Repeat("f", 63),
		"-" + strings.Repeat("1", 63), " " + strings.Repeat("f", 63), strings.Repeat("f", 63) + "\n",
	}
	for _, target := range badTargets {
		if w, err := PrepareManifestWork(source.Job, source.Template, source.PayoutAddress, source.ExtraNonce, target); err == nil || w != nil {
			t.Fatalf("invalid pool target %q returned prepared work", target)
		}
	}
	target := strings.Repeat("f", 64)
	badJob := source.Job
	badJob.ID = strings.Repeat("0", 64)
	if w, err := PrepareManifestWork(badJob, source.Template, source.PayoutAddress, source.ExtraNonce, target); err == nil || w != nil {
		t.Fatal("invalid immutable job returned prepared work")
	}
	badTemplate := source.Template
	badTemplate.Context.ParentHeight++
	if w, err := PrepareManifestWork(source.Job, badTemplate, source.PayoutAddress, source.ExtraNonce, target); err == nil || w != nil {
		t.Fatal("mismatched template returned prepared work")
	}
	if w, err := PrepareManifestWork(source.Job, source.Template, "", source.ExtraNonce, target); err == nil || w != nil {
		t.Fatal("missing explicit payout returned prepared work")
	}
}

func TestPoolShareUnpreparedAndCancelled(t *testing.T) {
	for _, w := range []*ManifestWork{nil, {}} {
		if w.Job() != (prospectjob.ManifestJob{}) || w.PoolTarget() != "" || !reflect.DeepEqual(w.Candidate(), Candidate{}) {
			t.Fatal("unprepared work exposed preparation data")
		}
		if got, err := w.CheckNonce(context.Background(), 0); err == nil || !reflect.DeepEqual(got, ShareCheck{}) {
			t.Fatal("unprepared work returned a share check")
		}
	}
	if wire.TestPoW {
		return
	}
	corpus := readPoolShareCorpus(t)
	tc := corpus.Cases[0]
	source := corpus.source(t, tc.Source)
	target := tc.ShareTargets[1]
	w := preparePoolShare(t, source, target.Target)
	if got, err := w.CheckNonce(nil, tc.Nonce); err == nil || !reflect.DeepEqual(got, ShareCheck{}) {
		t.Fatal("nil context returned a share check")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := w.CheckNonce(cancelled, tc.Nonce); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, ShareCheck{}) {
		t.Fatal("cancelled context returned work or lost cancellation", err)
	}
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer cancelExpired()
	if got, err := w.CheckNonce(expired, tc.Nonce); !errors.Is(err, context.DeadlineExceeded) || !reflect.DeepEqual(got, ShareCheck{}) {
		t.Fatal("expired context returned work or lost deadline", err)
	}
	if got, err := w.CheckNonce(context.Background(), tc.Nonce); err != nil || !reflect.DeepEqual(got, wantPoolShare(source, tc, target.Pool)) {
		t.Fatal("refusal leaked a hash reservation or changed work", err)
	}
}

func TestPoolShareGlobalHashContention(t *testing.T) {
	if wire.TestPoW {
		t.Skip("profile refusal covered separately")
	}
	corpus := readPoolShareCorpus(t)
	first := corpus.Cases[0]
	source := corpus.source(t, first.Source)
	target := first.ShareTargets[1]
	works := []*ManifestWork{
		preparePoolShare(t, source, target.Target),
		preparePoolShare(t, corpus.source(t, "witness-odd-two-level-dependent"), strings.Repeat("f", 64)),
	}
	// Occupy the actual private hash slot, then contend from distinct work
	// objects. This proves nonqueued admission deterministically, without a
	// timing race, a fake hasher or another 256MiB Argon allocation.
	func() {
		select {
		case shareHashSlot <- struct{}{}:
		default:
			t.Fatal("hash slot was not released by an earlier check")
		}
		defer func() { <-shareHashSlot }()
		type response struct {
			check ShareCheck
			err   error
		}
		responses := make(chan response, 16)
		start := make(chan struct{})
		for i := 0; i < cap(responses); i++ {
			w := works[i%len(works)]
			go func() {
				<-start
				got, err := w.CheckNonce(context.Background(), first.Nonce)
				responses <- response{got, err}
			}()
		}
		close(start)
		for i := 0; i < cap(responses); i++ {
			got := <-responses
			if !errors.Is(got.err, ErrShareBusy) || !reflect.DeepEqual(got.check, ShareCheck{}) {
				t.Error("contended work returned a payload or did not refuse busy", got.err)
			}
		}
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		if got, err := works[0].CheckNonce(cancelled, first.Nonce); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, ShareCheck{}) {
			t.Error("contention hid already-known cancellation", err)
		}
	}()
	if got, err := works[0].CheckNonce(context.Background(), first.Nonce); err != nil || !reflect.DeepEqual(got, wantPoolShare(source, first, target.Pool)) {
		t.Fatal("contention altered retained work or left a queued reservation", err)
	}
}

func TestPoolShareRejectsCheapPoW(t *testing.T) {
	if !wire.TestPoW {
		return
	}
	for _, source := range readPoolShareCorpus(t).Sources {
		w, err := PrepareManifestWork(source.Job, source.Template, source.PayoutAddress, source.ExtraNonce, strings.Repeat("f", 64))
		if err == nil || w != nil || !strings.Contains(err.Error(), "slugtestpow is unsupported") {
			t.Fatal("cheap profile did not explicitly refuse preparation", err)
		}
	}
	// Even an internally marked prepared value must be refused before the
	// header can reach the incompatible test-profile hasher.
	w := &ManifestWork{prepared: true}
	if got, err := w.CheckNonce(context.Background(), 0); err == nil || !reflect.DeepEqual(got, ShareCheck{}) || !strings.Contains(err.Error(), "slugtestpow is unsupported") {
		t.Fatal("cheap profile did not explicitly refuse checking", err)
	}
}
