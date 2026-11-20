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
	"sync"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

// This exercises real registry synchronization and the concrete RPC boundary.
// Non-genesis tips are controlled transport observations, not normally replayed
// ownership evidence. No test advertises a ready Head from publication alone.
type chainResumeNode struct {
	mu                          sync.Mutex
	pin                         Pin
	observation                 ChainObservation
	observations                int
	moveAt                      int
	missing, reorg, headerMoves bool
	header                      string
}

func (f *chainResumeNode) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var call struct {
		ID     uint64            `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096)).Decode(&call) != nil {
		http.Error(w, "request", 400)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var result any
	switch call.Method {
	case "getcurrentnet":
		result = f.pin.Source.Network
	case "getblockhash":
		var height uint32
		if len(call.Params) != 1 || json.Unmarshal(call.Params[0], &height) != nil {
			http.Error(w, "height", 400)
			return
		}
		result = f.pin.Source.Genesis
		if height > 0 {
			result = strings.Repeat("11", 32)
			if f.reorg {
				result = strings.Repeat("22", 32)
			}
		}
	case "getblockheader":
		if f.missing {
			http.Error(w, "missing", 503)
			return
		}
		result = f.header
		if f.headerMoves {
			f.observation.Revision = "4"
			f.headerMoves = false
		}
	case "getchainobservation":
		f.observations++
		if f.observations == f.moveAt {
			f.observation.Height = 1
			f.observation.Hash = strings.Repeat("11", 32)
			f.observation.Revision = "2"
		}
		result = f.observation
	default:
		http.Error(w, "method", 400)
		return
	}
	json.NewEncoder(w).Encode(struct {
		Result any    `json:"result"`
		Error  any    `json:"error"`
		ID     uint64 `json:"id"`
	}{result, nil, call.ID})
}

func chainResumeFixture(t *testing.T) (*Store, **registry.ManifestStore, *PublicSource, *OwnedChainSource, *chainResumeNode, string) {
	t.Helper()
	ctx := context.Background()
	pin := chainReferencePin(t)
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "authority")
	a, e := registry.CreatePublicManifestAuthority(path, fighterdescriptor.CandidateDescriptor(), pin)
	if e != nil {
		t.Fatal(e)
	}
	owner := &a
	t.Cleanup(func() {
		if e := (*owner).Close(); e != nil {
			t.Error(e)
		}
	})
	p, e := a.PublicPublication(pin)
	if e != nil {
		t.Fatal(e)
	}
	source, e := NewPublicSource(ctx, pin, p)
	if e != nil {
		t.Fatal(e)
	}
	s, e := Create(filepath.Join(root, "core"), pin, StoreIdentity{StoreID: strings.Repeat("75", 32)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	var header bytes.Buffer
	if e = chaincfg.MainNetParams.GenesisBlock.Header.Serialize(&header); e != nil {
		t.Fatal(e)
	}
	f := &chainResumeNode{pin: pin, observation: ChainObservation{1, strings.Repeat("73", 32), "1", pin.Source.Genesis, 0}, header: hex.EncodeToString(header.Bytes())}
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	node, e := NewOwnedChainSource(ctx, pin, prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "fixture", Password: "fixture"}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := node.Close(); e != nil {
			t.Error(e)
		}
	})
	return s, owner, source, node, f, path
}

func chainUnsigned(t *testing.T, v PublicationEntry) []byte {
	t.Helper()
	v.Signature = ""
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}

func TestOwnershipChainPublicationMovingTipExactReopen(t *testing.T) {
	ctx := context.Background()
	s, owner, source, node, f, path := chainResumeFixture(t)
	f.mu.Lock()
	f.moveAt = 2
	f.mu.Unlock() // First Observe succeeds; post-stage Check moves.
	if _, _, e := s.RecordChainObservation(ctx, source, node); e != ErrStale {
		t.Fatal("moving tip did not leave staged intent", e)
	}
	pending, e := source.publisher.PendingChainPublication(ctx)
	if e != nil || pending == nil {
		t.Fatal("missing retained intent", e)
	}
	want := chainUnsigned(t, pending.Entry())
	if _, e = source.publisher.PublishNext(ctx); e != registry.ErrBusy {
		t.Fatal("generic publisher signed chain", e)
	}
	if e = (*owner).Close(); e != nil {
		t.Fatal(e)
	}
	*owner, e = registry.OpenPublicManifestAuthority(path, s.pin)
	if e != nil {
		t.Fatal(e)
	}
	p, e := (*owner).PublicPublication(s.pin)
	if e != nil {
		t.Fatal(e)
	}
	source, e = NewPublicSource(ctx, s.pin, p)
	if e != nil {
		t.Fatal(e)
	}
	next, resumed, e := s.ResumeChainPublication(ctx, source, node)
	if e != nil || !resumed || next == nil {
		t.Fatal("exact original resume", e)
	}
	entry, e := p.Publication(ctx, 1)
	if e != nil || !bytes.Equal(chainUnsigned(t, entry), want) {
		t.Fatal("old bytes/time changed", e)
	}
	if e = registry.ValidatePublicPublication(s.pin, entry); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Head(ctx); e != ErrUnavailable {
		t.Fatal("publication minted ownership head", e)
	}
	again, resumed, e := s.ResumeChainPublication(ctx, next, node)
	if e != nil || resumed || again != next {
		t.Fatal("absent intent invented work", e)
	}
	fresh, latest, e := s.RecordChainObservation(ctx, next, node)
	if e != nil || latest.Sequence != 2 || fresh.status.PublicationSequence != 2 {
		t.Fatal("latest separate entry", e)
	}
	var ref registry.PublicChainReference
	if json.Unmarshal(latest.Payload, &ref) != nil || ref.Height != 1 || ref.Hash != strings.Repeat("11", 32) {
		t.Fatal("latest tip replaced old intent")
	}
	oldID, _ := registry.PublicPublicationID(s.pin, entry)
	if ref.PreviousChainEntryID != oldID {
		t.Fatal("chain link lost original entry")
	}
}

func TestOwnershipChainPublicationRecordResumesAtMostOne(t *testing.T) {
	ctx := context.Background()
	s, _, source, node, f, _ := chainResumeFixture(t)
	pending, e := source.publisher.PrepareChainPublication(ctx, registry.PublicChainReference{Height: 0, Hash: s.pin.Source.Genesis, PreviousChainEntryID: zeroID})
	if e != nil {
		t.Fatal(e)
	}
	want := chainUnsigned(t, pending.Entry())
	f.mu.Lock()
	f.observation = ChainObservation{1, strings.Repeat("73", 32), "2", strings.Repeat("11", 32), 1}
	f.mu.Unlock()
	next, latest, e := s.RecordChainObservation(ctx, source, node)
	if e != nil || latest.Sequence != 2 || next.status.PublicationSequence != 2 {
		t.Fatal("bounded old+new publication", e)
	}
	first, e := source.publisher.Publication(ctx, 1)
	if e != nil || !bytes.Equal(chainUnsigned(t, first), want) {
		t.Fatal("original body changed", e)
	}
	if pending, e := source.publisher.PendingChainPublication(ctx); e != nil || pending != nil {
		t.Fatal("pending not closed", e)
	}
}

func TestOwnershipChainPublicationResumeRefusals(t *testing.T) {
	for _, kind := range []string{"missing", "reorg", "moving", "too-short", "foreign", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, _, source, node, f, _ := chainResumeFixture(t)
			pending, e := source.publisher.PrepareChainPublication(ctx, registry.PublicChainReference{Height: 1, Hash: strings.Repeat("11", 32), PreviousChainEntryID: zeroID})
			if e != nil {
				t.Fatal(e)
			}
			want := chainUnsigned(t, pending.Entry())
			f.mu.Lock()
			f.observation = ChainObservation{1, strings.Repeat("73", 32), "2", strings.Repeat("33", 32), 2}
			candidate := source
			switch kind {
			case "missing":
				f.missing = true
			case "reorg":
				f.reorg = true
			case "moving":
				f.headerMoves = true
			case "too-short":
				f.observation.Height = 0
				f.observation.Hash = s.pin.Source.Genesis
			case "foreign":
				copy := *source
				copy.pin.RuleID = strings.Repeat("99", 32)
				candidate = &copy
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			f.mu.Unlock()
			if next, resumed, e := s.ResumeChainPublication(ctx, candidate, node); e == nil || resumed || next != nil {
				t.Fatal("refusal disclosed success", kind, e)
			}
			kept, e := source.publisher.PendingChainPublication(context.Background())
			if e != nil || kept == nil || !bytes.Equal(chainUnsigned(t, kept.Entry()), want) {
				t.Fatal("refusal changed intent", e)
			}
			status, e := source.publisher.Status(context.Background())
			if e != nil || status.PublicationSequence != 0 {
				t.Fatal("refusal advanced frontier", e)
			}
			if _, e = s.Head(context.Background()); !errors.Is(e, ErrUnavailable) {
				t.Fatal("refusal minted head", e)
			}
		})
	}
}
