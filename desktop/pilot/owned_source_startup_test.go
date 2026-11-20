// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/prospectnode"
)

func ownedStartupSource(t *testing.T, blockRead bool) (*PoolRuntime, *atomic.Uint64, <-chan struct{}) {
	t.Helper()
	r := ownedSourceRuntimeTest(t)
	r.ownedGym = &ownedGymRuntimeResources{}
	if e := r.openPoolIssuer(false); e != nil {
		t.Fatal(e)
	}
	o := r.owned
	var e error
	o.publisher, e = r.issuer.PublicPublication(o.config.Ownership)
	if e != nil {
		t.Fatal(e)
	}
	o.store, e = fighterownership.Create(filepath.Join(r.config.DataDir, ownedSourceDirectory), o.config.Ownership, o.config.Store)
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
	requests := new(atomic.Uint64)
	entered := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		var call struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if json.NewDecoder(req.Body).Decode(&call) != nil {
			http.Error(w, "request", 400)
			return
		}
		var result any
		switch call.Method {
		case "getcurrentnet":
			result = o.config.Ownership.Source.Network
		case "getblockhash":
			result = o.config.Ownership.Source.Genesis
		case "getchainobservation":
			result = fighterownership.ChainObservation{Version: 1, InstanceID: strings.Repeat("73", 32), Revision: "1", Hash: o.config.Ownership.Source.Genesis, Height: 0}
		case "getblockheader":
			result = hex.EncodeToString(header.Bytes())
		case "getblock":
			if blockRead {
				once.Do(func() { close(entered) })
				<-req.Context().Done()
				return
			}
			result = hex.EncodeToString(block.Bytes())
		default:
			http.Error(w, "unexpected method", 400)
			return
		}
		json.NewEncoder(w).Encode(struct {
			Result any    `json:"result"`
			Error  any    `json:"error"`
			ID     uint64 `json:"id"`
		}{result, nil, call.ID})
	}))
	t.Cleanup(server.Close)
	o.node, e = fighterownership.NewOwnedChainSource(context.Background(), o.config.Ownership, prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "fixture", Password: "fixture"}, r.life)
	if e != nil {
		t.Fatal(e)
	}
	return r, requests, entered
}

func TestOwnedGymStartupSourceFreshAndReopened(t *testing.T) {
	r, requests, _ := ownedStartupSource(t, false)
	ctx := context.Background()
	if _, e := r.owned.store.Head(ctx); !errors.Is(e, fighterownership.ErrUnavailable) {
		t.Fatal("fresh core unexpectedly ready", e)
	}
	expected := r.owned.config
	foreign := expected
	foreign.Store.StoreID = strings.Repeat("99", 32)
	before := requests.Load()
	if e := r.PrepareOwnedSource(ctx, foreign); e == nil || requests.Load() != before {
		t.Fatal("foreign setup pin reached source work", e)
	}
	if e := r.PrepareOwnedSource(ctx, expected); e != nil {
		t.Fatal("fresh startup replay", e)
	}
	first, e := r.owned.store.Head(ctx)
	if e != nil || first.ChainHeight != 0 || first.ChainHash != expected.Ownership.Source.Genesis {
		t.Fatal("genesis source not ready for league metadata", e)
	}
	if e = r.owned.store.Close(); e != nil {
		t.Fatal(e)
	}
	r.owned.store, e = fighterownership.Open(filepath.Join(r.config.DataDir, ownedSourceDirectory), expected.Ownership, expected.Store)
	if e != nil {
		t.Fatal(e)
	}
	r.owned.lastObservation = fighterownership.ChainObservation{}
	r.owned.progress = fighterownership.Progress{}
	if _, e = r.owned.store.Head(ctx); !errors.Is(e, fighterownership.ErrUnavailable) {
		t.Fatal("reopened core adopted stored readiness", e)
	}
	if e = r.PrepareOwnedSource(ctx, expected); e != nil {
		t.Fatal("reopened startup replay", e)
	}
	head, e := r.owned.store.Head(ctx)
	if e != nil || head.ChainHash != first.ChainHash || head.ChainHeight != first.ChainHeight || head.PublicationSequence <= first.PublicationSequence {
		t.Fatal("reopen skipped original observation/source audit", e)
	}
}

func TestOwnedGymStartupSourceCancellationJoins(t *testing.T) {
	r, _, entered := ownedStartupSource(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.PrepareOwnedSource(ctx, r.owned.config) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual replay did not reach controlled node read")
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal("original cancellation was lost", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled source work did not join")
	}
	if r.owned.replay != nil || r.owned.progress.Ready {
		t.Fatal("canceled startup retained live/ready replay")
	}
	if e := r.Close(); e != nil {
		t.Fatal("runtime cleanup failed after cancellation", e)
	}
}
