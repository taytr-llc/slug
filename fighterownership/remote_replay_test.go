//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/chainhash/v2"
)

func TestOwnershipRemoteGenesisUsesPublicRowsOnly(t *testing.T) {
	ctx := context.Background()
	original, local, node, _ := cutoffGenesisFixture(t)
	handler, e := NewPublicReadServer(original.pin, local.publisher, original, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer handler.Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	reader, e := NewPublicReader(ctx, original.pin, PublicReadConfig{URL: server.URL + publicReadPath}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	remote, e := NewRemotePublicSource(ctx, original.pin, reader)
	if e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	rebuilt, e := Create(filepath.Join(root, "rebuilt-public"), original.pin, StoreIdentity{strings.Repeat("79", 32)})
	if e != nil {
		t.Fatal(e)
	}
	defer rebuilt.Close()
	replay, e := rebuilt.BeginReplay(ctx, remote, node)
	if e != nil {
		t.Fatal(e)
	}
	p := boundedReplayFinish(t, replay)
	if p.Head == nil || p.Head.AdmissionSequence != 0 || p.Head.ChainHash != original.pin.Source.Genesis {
		t.Fatal("public replay")
	}
	if rebuilt.meta.FinalitySequence != 0 || len(rebuilt.frames) != 0 || len(rebuilt.frameBodies) != 0 {
		t.Fatal("remote reconstruction signed as source")
	}
	if _, _, e = rebuilt.RecordChainObservation(ctx, remote, node); e == nil {
		t.Fatal("remote source acquired publication authority")
	}
	if len(replay.states) != 0 || len(replay.branches) != 0 || len(replay.verified) != 0 {
		t.Fatal("completed handle retained replay buffers")
	}
	if e = reader.Close(); e != nil {
		t.Fatal(e)
	}
	h, e := rebuilt.AcquireHead(ctx, *p.Head)
	if e != nil {
		t.Fatal(e)
	}
	if e = h.Check(); e == nil {
		t.Fatal("closed original reader was substituted")
	}
}

func TestOwnershipRemoteUnpublishedFinalityCannotSign(t *testing.T) {
	// Conditional arithmetic/model control, not a normal-work branch fixture.
	// It tests the disclosure/signing boundary after hypothetical successful scan.
	corpus, pins := coreReference(t)
	var admission AdmissionStatement
	var final EffectiveState
	var pin Pin
	for _, v := range corpus.Positives {
		if v.Name == "v1/zero256-unawarded" && v.Family == "admission" {
			json.Unmarshal(v.Value, &admission)
			pin = pins[admission.PinID]
		}
		if v.Name == "v1/zero256-unawarded/final" && v.Family == "state" {
			json.Unmarshal(v.Value, &final)
		}
	}
	if final.Generation == 0 {
		t.Fatal("independent conditional final state absent")
	}
	path := filepath.Join(t.TempDir(), "conditional")
	s, e := Create(path, pin, StoreIdentity{strings.Repeat("7a", 32)})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.admissions[1] = admission
	b := &replayBranch{target: ReplayTarget{AdmissionSequence: 1, AdmissionHead: final.BirthAdmissionID, ChainHeight: 160, ChainHash: strings.Repeat("52", 32)}, effective: map[string]EffectiveState{final.ProspectID: final}, conflicted: map[string]EffectiveState{}}
	r := &Replay{store: s, source: &PublicSource{reader: &PublicReader{}}, branches: []*replayBranch{b}, branch: 1, official: map[string]EffectiveState{}}
	revision := s.meta.Revision
	if _, e = r.finish(context.Background()); !errors.Is(e, ErrUnavailable) {
		t.Fatal("remote reconstruction fabricated source finality", e)
	}
	if s.meta.Revision != revision || s.meta.CurrentHead != nil || len(s.frames) != 0 || len(s.frameBodies) != 0 {
		t.Fatal("remote incomplete result mutated authority")
	}
}
func TestOwnershipMerkleWorkIsCancellable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ids := make([]chainhash.Hash, 8192)
	if _, e := blockMerkleTree(ctx, ids); !errors.Is(e, context.Canceled) {
		t.Fatal("tree ignored cancellation", e)
	}
}
