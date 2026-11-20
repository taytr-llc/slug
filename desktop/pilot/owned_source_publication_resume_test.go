// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

func TestOwnedSourceRuntimeChainResumeBeforeAdmissions(t *testing.T) {
	r, _, _ := ownedStartupSource(t, false)
	ctx := context.Background()
	o := r.owned
	prepared, e := o.publisher.PrepareChainPublication(ctx, registry.PublicChainReference{Height: 0, Hash: o.config.Ownership.Source.Genesis, PreviousChainEntryID: strings.Repeat("0", 64)})
	if e != nil {
		t.Fatal(e)
	}
	want, _ := json.Marshal(prepared.Entry())
	// Use the frozen template as public input only. A fresh real issuer job
	// binds a published fixture key. The maximum test prospect target makes
	// its nonce-zero work deterministically eligible; RecordManifestJobChecked
	// and PublishNext still perform their actual normal-work verification.
	raw, e := os.ReadFile("../../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("frozen template input changed")
	}
	var corpus struct {
		Sources []struct {
			Template      prospectwork.Template `json:"template"`
			PayoutAddress string                `json:"payout_address"`
			ExtraNonce    uint64                `json:"extra_nonce"`
		} `json:"sources"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Sources) != 3 {
		t.Fatal("template corpus")
	}
	input := corpus.Sources[0]
	input.Template.Context.ProspectTarget = strings.Repeat("f", 64)
	keyBytes := make([]byte, 32)
	keyBytes[31] = 1
	key, _ := btcec.PrivKeyFromBytes(keyBytes)
	defer key.Zero()
	public := hex.EncodeToString(key.PubKey().SerializeCompressed())
	job, e := r.issuer.IssueManifestJob(input.Template.Context, public, func() error { return nil })
	if e != nil {
		t.Fatal("fresh fixture job", e)
	}
	candidate, e := prospectwork.BuildManifest(job, input.Template, input.PayoutAddress, input.ExtraNonce)
	if e != nil {
		t.Fatal("fresh public candidate", e)
	}
	authorization, e := gymauth.SignAdmission(job.Context, candidate.Proof, key)
	if e != nil {
		t.Fatal(e)
	}
	_, inserted, e := r.issuer.RecordManifestJobChecked(candidate.Proof, authorization, func(c prospect.Context, _ prospect.Proof, _ prospect.Verified) (registry.Eligibility, error) {
		return registry.Eligibility{RuleVersion: registry.EligibilityRuleVersion2, Source: "current-parent", Hash: c.ParentHash, Height: c.ParentHeight}, nil
	})
	if e != nil || !inserted {
		t.Fatal("actual admission behind pending chain", e)
	}
	status, e := o.publisher.Status(ctx)
	if e != nil || status.AcceptedSequence != 1 || status.SignedAdmissionSequence != 0 || !status.PublicEvidencePending {
		t.Fatal("fixture lacks actual publication gap", e)
	}
	if _, e = o.publisher.PublishNext(ctx); e != registry.ErrBusy {
		t.Fatal("fixture did not block old admission-first order", e)
	}
	progress, e := r.RefreshOwnership(ctx, 1)
	if e != nil || progress.WorkID == "" || progress.Ready {
		t.Fatal("resume-before-admissions failed or invented complete head", e)
	}
	status, e = o.publisher.Status(ctx)
	if e != nil || status.PublicationSequence != 3 || status.SignedAdmissionSequence != 1 || status.PublicEvidencePending {
		t.Fatal("retained chain/admission/new observation order", status, e)
	}
	first, e := o.publisher.Publication(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	first.Signature = ""
	got, _ := json.Marshal(first)
	if !bytes.Equal(want, got) {
		t.Fatal("runtime retargeted or retimestamped original chain intent")
	}
	second, e := o.publisher.Publication(ctx, 2)
	if e != nil || second.Kind != "admission" {
		t.Fatal("admission skipped original chain", e)
	}
	third, e := o.publisher.Publication(ctx, 3)
	if e != nil || third.Kind != "chain" {
		t.Fatal("latest observation missing", e)
	}
	if _, e = o.store.Head(ctx); e != fighterownership.ErrUnavailable {
		t.Fatal("partial replay became complete authority", e)
	}
}
