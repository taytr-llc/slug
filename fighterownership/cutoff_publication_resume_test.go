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
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

type cutoffResumeFixture struct {
	store     *Store
	authority *registry.ManifestStore
	source    *PublicSource
	node      *OwnedChainSource
	server    *httptest.Server
	root      string
	requests  atomic.Uint64
}

func newCutoffResumeFixture(t *testing.T) *cutoffResumeFixture {
	t.Helper()
	f := new(cutoffResumeFixture)
	t.Cleanup(func() {
		if f.node != nil {
			if e := f.node.Close(); e != nil {
				t.Error(e)
			}
		}
		if f.server != nil {
			f.server.Close()
		}
		if f.store != nil {
			if e := f.store.Close(); e != nil {
				t.Error(e)
			}
		}
		if f.authority != nil {
			if e := f.authority.Close(); e != nil {
				t.Error(e)
			}
		}
	})
	ctx := context.Background()
	pin := chainReferencePin(t)
	var e error
	f.root, e = filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	f.authority, e = registry.CreatePublicManifestAuthority(filepath.Join(f.root, "authority"), fighterdescriptor.CandidateDescriptor(), pin)
	if e != nil {
		t.Fatal(e)
	}
	publisher, e := f.authority.PublicPublication(pin)
	if e != nil {
		t.Fatal(e)
	}
	f.source, e = NewPublicSource(ctx, pin, publisher)
	if e != nil {
		t.Fatal(e)
	}
	f.store, e = Create(filepath.Join(f.root, "ownership"), pin, StoreIdentity{StoreID: strings.Repeat("75", 32)})
	if e != nil {
		t.Fatal(e)
	}
	var block, header bytes.Buffer
	if e = chaincfg.MainNetParams.GenesisBlock.Serialize(&block); e != nil {
		t.Fatal(e)
	}
	if e = chaincfg.MainNetParams.GenesisBlock.Header.Serialize(&header); e != nil {
		t.Fatal(e)
	}
	observation := ChainObservation{1, strings.Repeat("73", 32), "1", pin.Source.Genesis, 0}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		var request struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request) != nil {
			http.Error(w, "request", 400)
			return
		}
		var result any
		switch request.Method {
		case "getcurrentnet":
			result = pin.Source.Network
		case "getblockhash":
			result = pin.Source.Genesis
		case "getchainobservation":
			result = observation
		case "getblockheader":
			result = hex.EncodeToString(header.Bytes())
		case "getblock":
			result = hex.EncodeToString(block.Bytes())
		default:
			http.Error(w, "unexpected method", 400)
			return
		}
		json.NewEncoder(w).Encode(struct {
			Result any    `json:"result"`
			Error  any    `json:"error"`
			ID     uint64 `json:"id"`
		}{result, nil, request.ID})
	}))
	f.node, e = NewOwnedChainSource(ctx, pin, prospectnode.RPCConfig{Version: 1, URL: f.server.URL, Username: "fixture", Password: "fixture"}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	return f
}

func (f *cutoffResumeFixture) stage(t *testing.T) (CutoffRecord, []byte) {
	t.Helper()
	ctx := context.Background()
	boundary, e := f.store.CaptureBoundary(ctx, f.source, f.node, strings.Repeat("71", 32), time.Now().Add(150*time.Millisecond).UnixMilli())
	if e != nil {
		t.Fatal(e)
	}
	replay, e := f.store.BeginCutoffReplay(ctx, f.source, f.node, boundary)
	if e != nil {
		t.Fatal(e)
	}
	progress := boundedReplayFinish(t, replay)
	if progress.Cutoff == nil {
		t.Fatal("normal historical replay returned no cutoff")
	}
	cutoff := *progress.Cutoff
	prepared, e := f.source.publisher.PrepareCutoffPublication(ctx, cutoff)
	if e != nil {
		t.Fatal(e)
	}
	// Interruption occurs after the durable pending reference, before CommitSigned.
	want, e := json.Marshal(prepared.Entry())
	if e != nil {
		t.Fatal(e)
	}
	return cutoff, want
}

func cutoffPendingBytes(t *testing.T, source *PublicSource) []byte {
	t.Helper()
	pending, e := source.publisher.PendingCutoffPublication(context.Background())
	if e != nil || pending == nil {
		t.Fatal("missing pending cutoff", e)
	}
	raw, e := json.Marshal(pending.Entry())
	if e != nil {
		t.Fatal(e)
	}
	return raw
}

func TestOwnershipCutoffPublicationExactReopenNoHead(t *testing.T) {
	ctx := context.Background()
	f := newCutoffResumeFixture(t)
	cutoff, want := f.stage(t)
	pin, identity := f.store.pin, f.store.identity
	if _, e := f.source.publisher.PublishNext(ctx); e != registry.ErrBusy {
		t.Fatal("predecessor queue did not obstruct ordinary publication", e)
	}
	if e := f.store.Close(); e != nil {
		t.Fatal(e)
	}
	if e := f.authority.Close(); e != nil {
		t.Fatal(e)
	}
	var e error
	f.authority, e = registry.OpenPublicManifestAuthority(filepath.Join(f.root, "authority"), pin)
	if e != nil {
		t.Fatal(e)
	}
	publisher, e := f.authority.PublicPublication(pin)
	if e != nil {
		t.Fatal(e)
	}
	f.source, e = NewPublicSource(ctx, pin, publisher)
	if e != nil {
		t.Fatal(e)
	}
	f.store, e = Open(filepath.Join(f.root, "ownership"), pin, identity)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Head(ctx); e != ErrUnavailable {
		t.Fatal("reopen granted Head", e)
	}
	before := f.requests.Load()
	meta := f.store.meta
	next, resumed, e := f.store.ResumeCutoffPublication(ctx, f.source)
	if e != nil || !resumed || next == nil {
		t.Fatal("exact cutoff resume", e)
	}
	if f.requests.Load() != before || f.store.meta != meta || f.store.cutoffs[cutoff.EventID] != cutoff {
		t.Fatal("reference resume sampled node or changed signed cutoff/core metadata")
	}
	entry, e := publisher.Publication(ctx, 1)
	if e != nil || registry.ValidatePublicPublication(pin, entry) != nil {
		t.Fatal("signed reference", e)
	}
	entry.Signature = ""
	got, _ := json.Marshal(entry)
	if !bytes.Equal(got, want) {
		t.Fatal("old pending body/time changed")
	}
	stored, e := publisher.Cutoff(ctx, cutoff.EventID)
	if e != nil || stored != cutoff {
		t.Fatal("signed cutoff changed", e)
	}
	if _, e = f.store.Head(ctx); e != ErrUnavailable {
		t.Fatal("publication granted Head", e)
	}
	if _, e = f.store.Cutoff(ctx, cutoff.EventID); e != ErrUnavailable {
		t.Fatal("publication granted historical consumer authority", e)
	}
	if _, again, e := f.store.ResumeCutoffPublication(ctx, next); e != nil || again {
		t.Fatal("repeated recovery appended again", e)
	}
	status, e := publisher.Status(ctx)
	if e != nil || status.PublicationSequence != 1 {
		t.Fatal("reference count", e)
	}
	// Ordinary normal replay is still required before any current read succeeds.
	replay, e := f.store.BeginReplay(ctx, next, f.node)
	if e != nil {
		t.Fatal(e)
	}
	boundedReplayFinish(t, replay)
	if _, e = f.store.Head(ctx); e != nil {
		t.Fatal("ordinary replay not ready", e)
	}
}

func TestOwnershipCutoffPublicationRefusalsRetainIntent(t *testing.T) {
	f := newCutoffResumeFixture(t)
	cutoff, want := f.stage(t)
	for _, kind := range []string{"missing-body", "missing-boundary", "conflicting-body", "cancelled", "stale-source", "missing-source-intent"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			candidate := f.source
			expected := want
			if kind == "missing-source-intent" {
				root, e := filepath.EvalSymlinks(t.TempDir())
				if e != nil {
					t.Fatal(e)
				}
				authority, e := registry.CreatePublicManifestAuthority(filepath.Join(root, "other"), fighterdescriptor.CandidateDescriptor(), f.store.pin)
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() {
					if e := authority.Close(); e != nil {
						t.Error(e)
					}
				})
				publisher, e := authority.PublicPublication(f.store.pin)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = publisher.PrepareCutoffPublication(ctx, cutoff); e != nil {
					t.Fatal(e)
				}
				candidate, e = NewPublicSource(ctx, f.store.pin, publisher)
				if e != nil {
					t.Fatal(e)
				}
				expected = cutoffPendingBytes(t, candidate)
			}
			f.store.mu.Lock()
			old := f.store.cutoffs[cutoff.EventID]
			boundary := f.store.boundaries[cutoff.EventID]
			switch kind {
			case "missing-body":
				delete(f.store.cutoffs, cutoff.EventID)
			case "missing-boundary":
				delete(f.store.boundaries, cutoff.EventID)
			case "conflicting-body":
				changed := old
				changed.Signature = strings.Repeat("0", 128)
				f.store.cutoffs[cutoff.EventID] = changed
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "stale-source":
				copy := *candidate
				copy.status.PublicationSequence++
				candidate = &copy
			}
			f.store.mu.Unlock()
			defer func() {
				f.store.mu.Lock()
				f.store.cutoffs[cutoff.EventID] = old
				f.store.boundaries[cutoff.EventID] = boundary
				f.store.mu.Unlock()
			}()
			if next, resumed, e := f.store.ResumeCutoffPublication(ctx, candidate); e == nil || resumed || next != nil {
				t.Fatal("invalid recovery succeeded", kind, e)
			}
			if !bytes.Equal(cutoffPendingBytes(t, candidate), expected) || !bytes.Equal(cutoffPendingBytes(t, f.source), want) {
				t.Fatal("refusal replaced pending intent")
			}
			status, e := f.source.publisher.Status(context.Background())
			if e != nil || status.PublicationSequence != 0 {
				t.Fatal("refusal advanced source", e)
			}
			if _, e = f.store.Head(context.Background()); !errors.Is(e, ErrUnavailable) {
				t.Fatal("refusal granted Head", e)
			}
		})
	}
}
