// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectwork

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

func testManifestBuildJob(t *testing.T, f templateReference) prospectjob.ManifestJob {
	t.Helper()
	id, err := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	j, err := prospectjob.NewManifest(f.Job.Context, strings.Repeat("a1", 32), id, f.Job.PublicKey, f.Job.Challenge, f.Job.IssuedAtMS)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestManifestBuilderTypeAndProfile(t *testing.T) {
	for _, f := range templateReferences(t) {
		job := testManifestBuildJob(t, f)
		candidate, err := BuildManifest(job, f.Template, f.PayoutAddress, f.ExtraNonce)
		if wire.TestPoW {
			if err == nil || !reflect.DeepEqual(candidate, Candidate{}) {
				t.Fatal("cheap profile constructed candidate")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if candidate.Version != 1 || candidate.Proof.Version != 2 || candidate.JobID != job.ID || candidate.Proof.JobID != job.ID || candidate.Proof.GymID != job.GymID {
			t.Fatal("adapter changed candidate/proof format")
		}
		if candidate.JobID == f.Job.ID || candidate.BlockHex == f.Expected.BlockHex || candidate.Proof.Coinbase == f.Expected.Proof.Coinbase {
			t.Fatal("new typed job did not change committed work")
		}
		if candidate.CoinbaseValue != f.Expected.CoinbaseValue || candidate.PayoutAddress != f.Expected.PayoutAddress || candidate.TransactionCount != f.Expected.TransactionCount || candidate.Weight != f.Expected.Weight || candidate.SigOpCost != f.Expected.SigOpCost {
			t.Fatal("job binding altered payment/resource shape")
		}
		for _, edit := range []func(*prospectjob.ManifestJob){func(j *prospectjob.ManifestJob) { j.Version = 1 }, func(j *prospectjob.ManifestJob) { j.ID = f.Job.ID }, func(j *prospectjob.ManifestJob) { j.DescriptorID = strings.Repeat("0", 64) }, func(j *prospectjob.ManifestJob) { j.AuthorityID = strings.Repeat("0", 64) }} {
			bad := job
			edit(&bad)
			got, err := BuildManifest(bad, f.Template, f.PayoutAddress, f.ExtraNonce)
			if err == nil || !reflect.DeepEqual(got, Candidate{}) {
				t.Fatal("invalid typed job returned candidate")
			}
		}
		wrong := f.Template
		wrong.Context.ParentHeight++
		if got, err := BuildManifest(job, wrong, f.PayoutAddress, f.ExtraNonce); err == nil || !reflect.DeepEqual(got, Candidate{}) {
			t.Fatal("wrong template context returned candidate")
		}
		if got, err := BuildManifest(job, f.Template, "", f.ExtraNonce); err == nil || !reflect.DeepEqual(got, Candidate{}) {
			t.Fatal("missing explicit payout accepted")
		}
	}
}

func TestManifestBuilderConcurrentOwnership(t *testing.T) {
	if wire.TestPoW {
		t.Skip("profile refusal covered separately")
	}
	f := templateReferences(t)[2]
	job := testManifestBuildJob(t, f)
	before := job
	want, err := BuildManifest(job, f.Template, f.PayoutAddress, f.ExtraNonce)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := BuildManifest(job, f.Template, f.PayoutAddress, f.ExtraNonce)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Error("concurrent builder changed", err)
				return
			}
			if len(got.Proof.MerkleBranch) > 0 {
				got.Proof.MerkleBranch[0] = "changed"
			}
		}()
	}
	wg.Wait()
	got, err := BuildManifest(job, f.Template, f.PayoutAddress, f.ExtraNonce)
	if err != nil || !reflect.DeepEqual(got, want) || job != before {
		t.Fatal("returned branch or job shared mutable state", err)
	}
}
