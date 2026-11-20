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

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

func TestOwnershipCompleteGenesisReplayReopenAndNoCachedAuthority(t *testing.T) {
	pin := chainReferencePin(t)
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	authority, e := registry.CreatePublicManifestAuthority(filepath.Join(root, "authority"), fighterdescriptor.CandidateDescriptor(), pin)
	if e != nil {
		t.Fatal(e)
	}
	defer authority.Close()
	publisher, e := authority.PublicPublication(pin)
	if e != nil {
		t.Fatal(e)
	}
	source, e := NewPublicSource(context.Background(), pin, publisher)
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
			http.Error(w, "invalid", 400)
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
			http.Error(w, "unexpected method", 400)
			return
		}
		json.NewEncoder(w).Encode(struct {
			Result any    `json:"result"`
			Error  any    `json:"error"`
			ID     uint64 `json:"id"`
		}{result, nil, req.ID})
	}))
	defer server.Close()
	node, e := NewOwnedChainSource(context.Background(), pin, prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "public-fixture", Password: "public-fixture"}, context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer node.Close()
	path := filepath.Join(root, "ownership")
	identity := StoreIdentity{strings.Repeat("75", 32)}
	store, e := Create(path, pin, identity)
	if e != nil {
		t.Fatal(e)
	}
	finish := func(s *Store) HeadRef {
		t.Helper()
		replay, e := s.BeginReplay(context.Background(), source, node)
		if e != nil {
			t.Fatal(e)
		}
		defer replay.Close()
		for n := 0; n < 16; n++ {
			progress, e := replay.Step(context.Background())
			if e != nil {
				t.Fatal("bounded replay step", n, e)
			}
			if progress.Ready {
				return *progress.Head
			}
		}
		t.Fatal("genesis replay did not finish")
		return HeadRef{}
	}
	head := finish(store)
	if head.AdmissionSequence != 0 || head.ChainHeight != 0 || head.ChainHash != pin.Source.Genesis {
		t.Fatal(head)
	}
	capture, e := store.CaptureOwned(context.Background(), "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")
	if e != nil {
		t.Fatal(e)
	}
	page, e := store.OwnedPage(context.Background(), capture, 0, 8)
	if e != nil || !page.Complete || len(page.Items) != 0 || page.Head != head {
		t.Fatal(page, e)
	}
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	store, e = Open(path, pin, identity)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if _, e = store.Head(context.Background()); !errors.Is(e, ErrUnavailable) {
		t.Fatal("reopen trusted cached work", e)
	}
	again := finish(store)
	if again != head {
		t.Fatal("step partition/reopen changed complete head")
	}
}
