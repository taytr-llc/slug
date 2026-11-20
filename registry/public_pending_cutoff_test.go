//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPublicPendingCutoffExactReadAndReopen(t *testing.T) {
	s, p, path, pin := publicTestStore(t)
	ctx := context.Background()
	if got, e := p.PendingCutoffPublication(ctx); e != nil || got != nil {
		t.Fatal("empty queue invented cutoff", e)
	}
	// This registry-only fixture models the pending reference. It deliberately
	// supplies no signed cutoff, so the read handle alone cannot complete it.
	payload, _ := json.Marshal(PublicCutoffReference{CutoffID: strings.Repeat("71", 32)})
	if _, e := p.stage(ctx, "cutoff", payload, ""); e != nil {
		t.Fatal(e)
	}
	original, e := p.PendingCutoffPublication(ctx)
	if e != nil || original == nil {
		t.Fatal("missing exact pending cutoff", e)
	}
	want, _ := json.Marshal(original.Entry())
	meta := s.public.meta
	projection := original.Entry()
	projection.Payload[0] ^= 1
	got, _ := json.Marshal(original.Entry())
	if !bytes.Equal(got, want) || s.public.meta != meta {
		t.Fatal("projection aliased or mutated pending intent")
	}
	s.mu.Lock()
	_, e = p.PendingCutoffPublication(ctx)
	s.mu.Unlock()
	if e != ErrBusy {
		t.Fatal("real contention hidden", e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = p.PendingCutoffPublication(cancelled); !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled getter", e)
	}
	entry := original.Entry()
	entry.Signature, e = publicSign("SLUG fighter ownership publication statement", entry)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = original.CommitCutoffSigned(ctx, entry, PublicCutoffRecord{}); e == nil {
		t.Fatal("pending reference authorized a missing signed cutoff")
	}
	if s.public.meta != meta {
		t.Fatal("failed completion changed metadata")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if e = original.Check(ctx); !errors.Is(e, ErrClosed) {
		t.Fatal("closed original handle survived", e)
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
	retained, e := p.PendingCutoffPublication(ctx)
	if e != nil || retained == nil {
		t.Fatal("reopen lost pending reference", e)
	}
	got, _ = json.Marshal(retained.Entry())
	if !bytes.Equal(got, want) {
		t.Fatal("reopen changed body/time")
	}
}

func TestPublicPendingCutoffLeavesOtherKind(t *testing.T) {
	s, p, _, pin := publicTestStore(t)
	ctx := context.Background()
	chain, e := p.PrepareChainPublication(ctx, PublicChainReference{Height: 0, Hash: pin.Source.Genesis, PreviousChainEntryID: publicZero})
	if e != nil {
		t.Fatal(e)
	}
	want, _ := json.Marshal(chain.Entry())
	meta := s.public.meta
	if got, e := p.PendingCutoffPublication(ctx); e != nil || got != nil {
		t.Fatal("cutoff getter consumed another kind", e)
	}
	if _, e = chain.CommitCutoffSigned(ctx, chain.Entry(), PublicCutoffRecord{}); e != ErrPublicOwnership {
		t.Fatal("chain handle accepted cutoff completion", e)
	}
	got, _ := json.Marshal(chain.Entry())
	if !bytes.Equal(got, want) || s.public.meta != meta || chain.Check(ctx) != nil {
		t.Fatal("other pending kind changed")
	}
}
