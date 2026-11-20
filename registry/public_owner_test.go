//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospect"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func publicTestStore(t *testing.T) (*ManifestStore, *ManifestPublication, string, PublicManifestPin) {
	t.Helper()
	f := readManifestCorpus(t)
	pin, e := NewPublicManifestPin(f.Pin, strings.Repeat("72", 32))
	if e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "public")
	s, e := CreatePublicManifestAuthority(path, fighterdescriptor.CandidateDescriptor(), pin)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	p, e := s.PublicPublication(pin)
	if e != nil {
		t.Fatal(e)
	}
	return s, p, path, pin
}
func TestPublicBoundaryOriginalDeadlineAndOwnerClose(t *testing.T) {
	s, p, _, pin := publicTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	lease, e := p.BeginBoundary(ctx)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	select {
	case <-lease.cancelDone:
	case <-time.After(time.Second):
		t.Fatal("cancel did not join idle boundary")
	}
	id, _ := PublicManifestPinID(pin)
	now := time.Now().UnixMilli()
	first := PublicBoundaryObservation{now - 2, now - 1, strings.Repeat("71", 32), "1", 0, pin.Source.Genesis}
	second := first
	second.RequestStartMS = now
	second.ResponseEndMS = now + 1
	intent := PublicCutoffIntent{1, id, strings.Repeat("72", 32), now, first, second, 0, 0, publicZero, 0, publicZero, publicZero}
	if e = lease.CommitIntent(context.Background(), intent); e == nil {
		t.Fatal("fresh context extended original deadline")
	}
	if _, e = p.CutoffIntent(context.Background(), intent.EventID); !errors.Is(e, ErrNotFound) {
		t.Fatal("cancelled boundary persisted", e)
	}
	next, e := p.BeginBoundary(context.Background())
	if e != nil {
		t.Fatal("gate not released", e)
	}
	_ = next
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("owner Close hung on idle permit")
	}
}
func TestPublicPreparedChainExactIntentAndReopen(t *testing.T) {
	s, p, path, pin := publicTestStore(t)
	ctx := context.Background()
	ref := PublicChainReference{0, pin.Source.Genesis, publicZero}
	prepared, e := p.PrepareChainPublication(ctx, ref)
	if e != nil {
		t.Fatal(e)
	}
	entry := prepared.Entry()
	if _, e = p.PublishNext(ctx); !errors.Is(e, ErrBusy) {
		t.Fatal("generic publisher signed a chain assertion", e)
	}
	changed := entry
	changed.AttestedAtMS++
	changed.Signature, _ = publicSign("SLUG fighter ownership publication statement", changed)
	if _, e = prepared.CommitSigned(ctx, changed); !errors.Is(e, ErrConflict) {
		t.Fatal("changed intent accepted", e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = OpenPublicManifestAuthority(path, pin)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	p, e = s.PublicPublication(pin)
	if e != nil {
		t.Fatal(e)
	}
	reopened, e := p.PrepareChainPublication(ctx, ref)
	if e != nil {
		t.Fatal(e)
	}
	a, _ := json.Marshal(entry)
	b, _ := json.Marshal(reopened.Entry())
	if !bytes.Equal(a, b) {
		t.Fatal("durable intent resampled")
	}
	entry.Signature, _ = publicSign("SLUG fighter ownership publication statement", entry)
	saved, e := reopened.CommitSigned(ctx, entry)
	if e != nil || saved.AttestedAtMS != entry.AttestedAtMS {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if old, e := OpenManifestAuthority(path, pin.Source, false); e == nil {
		old.Close()
		t.Fatal("legacy authority opened public mode")
	}
	audit, e := OpenPublicManifestAuthorityAudit(path, pin)
	if e != nil {
		t.Fatal(e)
	}
	audit.Close()
}
func TestPublicBothAdmissionPathsAtomicOutbox(t *testing.T) {
	f := readManifestCorpus(t)
	s, p, path, pin := publicTestStore(t)
	random, e := hex.DecodeString(f.Random)
	if e != nil {
		t.Fatal(e)
	}
	s.random = bytes.NewReader(random[32:])
	now := f.Issued
	s.now = func() time.Time { return time.UnixMilli(now) }
	for _, v := range f.Jobs {
		got, e := s.IssueManifestJob(v.Job.Context, v.Job.PublicKey, func() error { return nil })
		if e != nil || got != v.Job {
			t.Fatal("old exact job", e)
		}
	}
	for n, tc := range f.Cases[:2] {
		now = tc.Accepted
		var got ManifestRecord
		var inserted bool
		if n == 0 {
			got, inserted, e = s.RecordManifestJobChecked(tc.Proof, tc.Authorization, func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
				return tc.Eligibility, nil
			})
		} else {
			var prepared *PreparedManifestAdmission
			prepared, e = s.PrepareManifestAdmission(tc.Proof, tc.Authorization)
			if e == nil {
				got, inserted, e = s.CommitManifestAdmission(prepared, tc.Eligibility, func() error { return nil })
			}
		}
		if e != nil || !inserted || got.Sequence != uint64(n+1) {
			t.Fatal("admission", n, e)
		}
		if _, e = s.db.Get(publicCounterKey("ownership/outbox/", uint32(got.Sequence)), nil); e != nil {
			t.Fatal("missing atomic outbox", e)
		}
	}
	status, e := p.Status(context.Background())
	if e != nil || status.AcceptedSequence != 2 || !status.PublicEvidencePending || status.SignedAdmissionSequence != 0 {
		t.Fatal(status, e)
	}
	for n := 0; n < 2; n++ {
		status, e = p.PublishNext(context.Background())
		if e != nil {
			t.Fatal(e)
		}
	}
	if status.PublicEvidencePending || status.SignedAdmissionSequence != 2 || status.PublicationSequence != 2 {
		t.Fatal(status)
	}
	// A deliberately failed atomic admission batch cannot manufacture its outbox.
	next := s.meta
	batch := new(leveldb.Batch)
	fake := f.Cases[0].Record
	fake.Sequence = 3
	if e = s.appendPublicAdmission(batch, fake); e != nil {
		t.Fatal(e)
	}
	original := s.writeBatch
	s.writeBatch = func(*leveldb.Batch, *opt.WriteOptions) error { return errors.New("injected sync failure") }
	next.LastSequence = 3
	if e = s.commit(batch, next); !errors.Is(e, ErrPoisoned) {
		t.Fatal(e)
	}
	s.writeBatch = original
	s.Close()
	s, e = OpenPublicManifestAuthority(path, pin)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.db.Get(publicCounterKey("ownership/outbox/", 3), nil); !errors.Is(e, leveldb.ErrNotFound) {
		t.Fatal("uncommitted outbox appeared", e)
	}
}
