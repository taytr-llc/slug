//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

func cutoffGenesisFixture(t *testing.T) (*Store, *PublicSource, *OwnedChainSource, string) {
	t.Helper()
	ctx := context.Background()
	pin := chainReferencePin(t)
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	a, e := registry.CreatePublicManifestAuthority(filepath.Join(root, "authority"), fighterdescriptor.CandidateDescriptor(), pin)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.Close() })
	p, e := a.PublicPublication(pin)
	if e != nil {
		t.Fatal(e)
	}
	source, e := NewPublicSource(ctx, pin, p)
	if e != nil {
		t.Fatal(e)
	}
	var block, header bytes.Buffer
	chaincfg.MainNetParams.GenesisBlock.Serialize(&block)
	chaincfg.MainNetParams.GenesisBlock.Header.Serialize(&header)
	obs := ChainObservation{1, strings.Repeat("73", 32), "1", pin.Source.Genesis, 0}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "request", 400)
			return
		}
		var result any
		switch req.Method {
		case "getcurrentnet":
			result = pin.Source.Network
		case "getblockhash":
			result = pin.Source.Genesis
		case "getchainobservation":
			result = obs
		case "getblockheader":
			result = hex.EncodeToString(header.Bytes())
		case "getblock":
			result = hex.EncodeToString(block.Bytes())
		default:
			http.Error(w, "unexpected", 400)
			return
		}
		json.NewEncoder(w).Encode(struct {
			Result any    `json:"result"`
			Error  any    `json:"error"`
			ID     uint64 `json:"id"`
		}{result, nil, req.ID})
	}))
	t.Cleanup(server.Close)
	node, e := NewOwnedChainSource(ctx, pin, prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "fixture", Password: "fixture"}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { node.Close() })
	path := filepath.Join(root, "ownership")
	s, e := Create(path, pin, StoreIdentity{strings.Repeat("75", 32)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s, source, node, path
}
func boundedReplayFinish(t *testing.T, r *Replay) Progress {
	t.Helper()
	defer r.Close()
	for n := 0; n < 20; n++ {
		p, e := r.Step(context.Background())
		if e != nil {
			t.Fatal("step", n, e)
		}
		if p.Ready {
			return p
		}
	}
	t.Fatal("finite genesis replay did not finish")
	return Progress{}
}
func TestOwnershipCutoffOriginalBoundaryReopenAndPublish(t *testing.T) {
	ctx := context.Background()
	s, source, node, path := cutoffGenesisFixture(t)
	r, e := s.BeginReplay(ctx, source, node)
	if e != nil {
		t.Fatal(e)
	}
	initial := boundedReplayFinish(t, r)
	boundary, e := s.CaptureBoundary(ctx, source, node, strings.Repeat("71", 32), time.Now().Add(150*time.Millisecond).UnixMilli())
	if e != nil {
		t.Fatal(e)
	}
	if boundary.Intent.FinalitySequence != 0 || boundary.Intent.FinalityBodyHead != zeroID {
		t.Fatal("wrong frozen prefix")
	}
	r, e = s.BeginCutoffReplay(ctx, source, node, boundary)
	if e != nil {
		t.Fatal(e)
	}
	boundedReplayFinish(t, r)
	retained, e := s.Head(ctx)
	if e != nil || retained != *initial.Head {
		t.Fatal("historical replay invalidated/replaced ready live head", e)
	}
	r, e = s.BeginCutoffReplay(ctx, source, node, boundary)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Step(ctx); e != nil {
		t.Fatal(e)
	}
	r.Close()
	pin, id := s.pin, s.identity
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(path, pin, id)
	if e != nil {
		t.Fatal("open partial cutoff", e)
	}
	defer s.Close()
	r, e = s.BeginCutoffReplay(ctx, source, node, boundary)
	if e != nil {
		t.Fatal(e)
	}
	p := boundedReplayFinish(t, r)
	if p.Cutoff == nil || p.Cutoff.Boundary != boundary || p.Cutoff.Head != *initial.Head {
		t.Fatal("historical result changed")
	}
	if _, e = s.Head(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal("historical replay became live authority", e)
	}
	r, e = s.BeginReplay(ctx, source, node)
	if e != nil {
		t.Fatal(e)
	}
	boundedReplayFinish(t, r)
	before, e := s.Head(ctx)
	if e != nil {
		t.Fatal(e)
	}
	next, entry, e := s.PublishCutoff(ctx, source, boundary.Intent.EventID)
	if e != nil {
		t.Fatal(e)
	}
	if entry.Kind != "cutoff" {
		t.Fatal(entry.Kind)
	}
	_, again, e := s.PublishCutoff(ctx, next, boundary.Intent.EventID)
	if e != nil || again.Sequence != entry.Sequence || !bytes.Equal(again.Payload, entry.Payload) {
		t.Fatal("exact publication replay", e)
	}
	after, e := s.Head(ctx)
	if e != nil || after != before {
		t.Fatal("publication replaced live head", e)
	}
	cut, e := s.Cutoff(ctx, boundary.Intent.EventID)
	if e != nil || cut != *p.Cutoff {
		t.Fatal("stored cutoff", e)
	}
	if _, e = s.BeginCutoffReplay(ctx, next, node, SignedBoundary{}); e == nil {
		t.Fatal("missing boundary accepted")
	}
}
func TestOwnershipPreparedSnapshotRepeatsOriginalHeadCheck(t *testing.T) {
	ctx := context.Background()
	s, source, node, _ := cutoffGenesisFixture(t)
	r, e := s.BeginReplay(ctx, source, node)
	if e != nil {
		t.Fatal(e)
	}
	p := boundedReplayFinish(t, r)
	for _, kind := range []string{"head", "closed", "source"} {
		t.Run(kind, func(t *testing.T) {
			h, e := s.AcquireHead(ctx, *p.Head)
			if e != nil {
				t.Fatal(e)
			}
			if e = h.Check(); e != nil {
				t.Fatal(e)
			}
			s.mu.Lock()
			oldHead, oldSource := s.head, s.source
			switch kind {
			case "head":
				s.head.ReplayDigest = strings.Repeat("11", 32)
			case "closed":
				h.Close()
			case "source":
				s.source = &PublicSource{publisher: source.publisher, pin: source.pin, status: source.status}
			}
			s.mu.Unlock()
			if _, e = s.transferSnapshot(h, strings.Repeat("22", 32), "fixture"); !errors.Is(e, ErrStale) {
				t.Fatal("transfer used second snapshot", e)
			}
			if _, e = s.recoverySnapshot(h, strings.Repeat("22", 32)); !errors.Is(e, ErrStale) {
				t.Fatal("recovery used second snapshot", e)
			}
			s.mu.Lock()
			s.head, s.source = oldHead, oldSource
			s.mu.Unlock()
			h.Close()
		})
	}
}
