//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospectjob"
)

type issueStoreFamily struct {
	Name    string
	Profile IssueProfile
	Work    struct {
		Job      prospectjob.ManifestJob
		Template json.RawMessage
	}
	Setup     struct{ Authority PublicManifestPin }
	RequestID string `json:"request_id"`
}

func issueStoreFixture(t *testing.T) (*ManifestStore, OperatorProfile, IssueProfile, string, issueStoreFamily) {
	t.Helper()
	x := issueReference(t)
	var f struct {
		Families []issueStoreFamily `json:"matching_families"`
	}
	if json.Unmarshal(x["fixtures"], &f) != nil || len(f.Families) == 0 {
		t.Fatal("families")
	}
	v := f.Families[0]
	p, err := NewOperatorProfile(v.Setup.Authority)
	if err != nil {
		t.Fatal(err)
	}
	owner := operatorStoreOwner(t, p)
	d, err := fighterdescriptor.ResolveCandidate(p.Pin().Source.DescriptorID)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "issuer")
	s, err := CreateRecoverableOperatorManifestAuthority(context.Background(), path, d, p, v.Profile, owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, p, v.Profile, path, v
}
func issueStoreIntent(t *testing.T, v issueStoreFamily) IssueIntent {
	t.Helper()
	id, err := IssueProfileID(v.Profile)
	if err != nil {
		t.Fatal(err)
	}
	// Template bytes come from the independent matching family unchanged.
	var compact bytes.Buffer
	if json.Compact(&compact, v.Work.Template) != nil {
		t.Fatal("template")
	}
	// The source's template digest is a plain SHA256, checked separately by its corpus tests.
	x := issueReference(t)
	var st struct {
		Pairs []struct {
			Value struct {
				Requests []struct{ Intent IssueIntent } `json:"source_requests"`
			}
		} `json:"state_pairs"`
	}
	if json.Unmarshal(x["states"], &st) != nil {
		t.Fatal("states")
	}
	for _, p := range st.Pairs {
		for _, r := range p.Value.Requests {
			if r.Intent.ProfileID == id && r.Intent.RequestID == v.RequestID {
				return r.Intent
			}
		}
	}
	t.Fatal("intent absent")
	return IssueIntent{}
}
func TestRecoverableIssuerExactReplaySealAndReopen(t *testing.T) {
	s, p, i, path, v := issueStoreFixture(t)
	ctx := context.Background()
	intent := issueStoreIntent(t, v)
	s.now = func() time.Time { return time.UnixMilli(v.Work.Job.IssuedAtMS) }
	challenge, _ := hex.DecodeString(v.Work.Job.Challenge)
	s.random = bytes.NewReader(challenge)
	prep, err := s.PrepareIssueRequest(ctx, intent, v.Work.Job.Context)
	if err != nil {
		t.Fatal(err)
	}
	r, j, err := s.CommitIssueRequest(prep, func() error { return nil })
	if err != nil || j != v.Work.Job || r.JobID != j.ID {
		t.Fatal("independent exact job", r, j, err)
	}
	for n := 0; n < 2; n++ {
		again, got, err := s.CommitIssueRequest(prep, func() error { return nil })
		if err != nil || again != r || got != j {
			t.Fatal("same request replay", err)
		}
	}
	if _, err := s.IssueManifestJob(j.Context, j.PublicKey, func() error { return nil }); err == nil {
		t.Fatal("legacy direct issue")
	}
	if _, err := s.PrepareManifestIssue(j.Context, j.PublicKey); err == nil {
		t.Fatal("legacy staged issue")
	}
	if _, err := s.CommitManifestIssue(nil, func() error { return nil }); err == nil {
		t.Fatal("legacy staged commit")
	}
	other := intent
	other.RequestID = strings.Repeat("2", 64)
	sealed, empty, err := s.SealIssueRequest(ctx, other, "parent_changed", func() error { return nil })
	if err != nil || empty.ID != "" || sealed.State != "sealed" {
		t.Fatal("seal", err)
	}
	s.now = func() time.Time { return time.UnixMilli(j.IssuedAtMS + 1) }
	repeated, _, err := s.SealIssueRequest(ctx, other, "intake_drained", func() error { return nil })
	if err != nil || repeated != sealed {
		t.Fatal("first seal changed", err)
	}
	altered := other
	altered.TemplateSHA256 = strings.Repeat("3", 64)
	if _, _, err := s.SealIssueRequest(ctx, altered, "parent_changed", func() error { return nil }); err != ErrConflict {
		t.Fatal("request collision", err)
	}
	if info, count, err := s.AuditIssueBinding(ctx, p.Pin().Source, i); err != nil || count != 2 || info.Jobs != 1 {
		t.Fatal(info, count, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if old, err := OpenOperatorManifestAuthorityAudit(path, p); err == nil {
		old.Close()
		t.Fatal("legacy adopted selected marker")
	}
	audit, err := OpenRecoverableOperatorManifestAuthorityAudit(ctx, path, p, i)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	if got, err := audit.TryIssueRequest(ctx, intent.RequestID); err != nil || got != r {
		t.Fatal("readonly replay", err)
	}
	if got, err := audit.TryIssueRequest(ctx, other.RequestID); err != nil || got != sealed {
		t.Fatal("readonly seal", err)
	}
}
func TestRecoverableIssuerAuditReverseAndUncertainCommit(t *testing.T) {
	s, p, i, path, v := issueStoreFixture(t)
	ctx := context.Background()
	intent := issueStoreIntent(t, v)
	prep, err := s.PrepareIssueRequest(ctx, intent, v.Work.Job.Context)
	if err != nil {
		t.Fatal(err)
	}
	original := s.writeBatch
	s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
		if err := original(b, o); err != nil {
			return err
		}
		return errors.New("lost write acknowledgment")
	}
	if _, _, err = s.CommitIssueRequest(prep, func() error { return nil }); !errors.Is(err, ErrPoisoned) {
		t.Fatal("uncertain commit", err)
	}
	if _, _, err = s.AuditIssueBinding(ctx, p.Pin().Source, i); !errors.Is(err, ErrPoisoned) {
		t.Fatal("poison refused", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	audit, err := OpenRecoverableOperatorManifestAuthorityAudit(ctx, path, p, i)
	if err != nil {
		t.Fatal(err)
	}
	r, err := audit.TryIssueRequest(ctx, intent.RequestID)
	if err != nil || r.State != "issued" {
		t.Fatal("uncertain row recovered", err)
	}
	if err = audit.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := leveldb.OpenFile(filepath.Join(path, databaseName), &opt.Options{Strict: opt.StrictAll, ErrorIfMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Delete(issueKey("issue/j/", r.JobID), &opt.WriteOptions{Sync: true})
	ce := db.Close()
	if errors.Join(err, ce) != nil {
		t.Fatal(err, ce)
	}
	if damaged, err := OpenRecoverableOperatorManifestAuthorityAudit(ctx, path, p, i); err == nil {
		damaged.Close()
		t.Fatal("missing reverse accepted")
	}
}
