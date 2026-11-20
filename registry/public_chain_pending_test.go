//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestPublicPendingChainExactOwnerReopen(t *testing.T) {
	s, p, path, pin := publicTestStore(t)
	ctx := context.Background()
	if got, e := p.PendingChainPublication(ctx); e != nil || got != nil {
		t.Fatal("empty queue invented chain intent", e)
	}
	original, e := p.PrepareChainPublication(ctx, PublicChainReference{0, pin.Source.Genesis, publicZero})
	if e != nil {
		t.Fatal(e)
	}
	want, _ := json.Marshal(original.Entry())
	meta := s.public.meta
	got, e := p.PendingChainPublication(ctx)
	if e != nil || got == nil {
		t.Fatal("retained chain", e)
	}
	raw, _ := json.Marshal(got.Entry())
	if !bytes.Equal(want, raw) || s.public.meta != meta {
		t.Fatal("lookup mutated exact intent")
	}
	copy := got.Entry()
	copy.Payload[0] ^= 1
	raw, _ = json.Marshal(got.Entry())
	if !bytes.Equal(want, raw) {
		t.Fatal("public projection aliased retained intent")
	}
	_, foreign, _, _ := publicTestStore(t)
	if other, e := foreign.PendingChainPublication(ctx); e != nil || other != nil {
		t.Fatal("foreign owner acquired intent", e)
	}
	s.mu.Lock()
	_, e = p.PendingChainPublication(ctx)
	s.mu.Unlock()
	if e != ErrBusy {
		t.Fatal("real lock contention hidden", e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = p.PendingChainPublication(cancelled); !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled lookup", e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if e = got.Check(ctx); !errors.Is(e, ErrClosed) {
		t.Fatal("old owner survived close", e)
	}
	reopened, e := OpenPublicManifestAuthority(path, pin)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	pub, e := reopened.PublicPublication(pin)
	if e != nil {
		t.Fatal(e)
	}
	retained, e := pub.PendingChainPublication(ctx)
	if e != nil || retained == nil {
		t.Fatal("reopen lost original intent", e)
	}
	raw, _ = json.Marshal(retained.Entry())
	if !bytes.Equal(raw, want) {
		t.Fatal("reopen retimestamped or retargeted")
	}
}

func TestPublicPendingChainOtherKindUntouched(t *testing.T) {
	s, p, _, _ := publicTestStore(t)
	ctx := context.Background()
	// The typed chain getter must not interfere with the existing admission
	// publisher. Stage an actual independently supplied admission first.
	f := readManifestCorpus(t)
	random, e := hex.DecodeString(f.Random)
	if e != nil {
		t.Fatal(e)
	}
	s.random = bytes.NewReader(random[32:])
	s.now = func() time.Time { return time.UnixMilli(f.Issued) }
	for _, row := range f.Jobs {
		if _, e = s.IssueManifestJob(row.Job.Context, row.Job.PublicKey, func() error { return nil }); e != nil {
			t.Fatal(e)
		}
	}
	tc := f.Cases[0]
	s.now = func() time.Time { return time.UnixMilli(tc.Accepted) }
	if _, _, e = s.RecordManifestJobChecked(tc.Proof, tc.Authorization, manifestGuard(tc)); e != nil {
		t.Fatal(e)
	}
	a, e := p.admission(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	aid, e := PublicAdmissionID(p.pin, a)
	if e != nil {
		t.Fatal(e)
	}
	payload, _ := json.Marshal(PublicAdmissionReference{AdmissionSequence: 1, AdmissionID: aid})
	if _, e = p.stage(ctx, "admission", payload, aid); e != nil {
		t.Fatal(e)
	}
	before, e := s.db.Get([]byte("ownership/pending"), nil)
	if e != nil {
		t.Fatal(e)
	}
	if got, e := p.PendingChainPublication(ctx); e != nil || got != nil {
		t.Fatal("non-chain exposed or blocked", e)
	}
	after, e := s.db.Get([]byte("ownership/pending"), nil)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("non-chain intent changed", e)
	}
	status, e := p.PublishNext(ctx)
	if e != nil || status.SignedAdmissionSequence != 1 || status.PublicEvidencePending {
		t.Fatal("original admission resume blocked", e)
	}
}
