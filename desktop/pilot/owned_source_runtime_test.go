// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

func ownedSourceRuntimeTest(t *testing.T) *PoolRuntime {
	t.Helper()
	r := poolRuntimeTest(t)
	canonical, e := filepath.EvalSymlinks(r.config.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	r.config.DataDir = canonical
	pin, e := registry.NewPublicManifestPin(r.source.AuthorityPin, strings.Repeat("73", 32))
	if e != nil {
		t.Fatal(e)
	}
	r.owned = &ownedRuntimeResources{config: OwnedSourceConfig{Version: 1, Ownership: pin, Store: fighterownership.StoreIdentity{StoreID: strings.Repeat("74", 32)}, PublicReadPort: 19063}}
	return r
}

func TestOwnedSourceRuntimeSameTipObservationChanges(t *testing.T) {
	r := ownedSourceRuntimeTest(t)
	ctx := context.Background()
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
	chaincfg.MainNetParams.GenesisBlock.Serialize(&block)
	chaincfg.MainNetParams.GenesisBlock.Header.Serialize(&header)
	var revision atomic.Value
	revision.Store("1")
	var instance atomic.Value
	instance.Store(strings.Repeat("73", 32))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var v struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if json.NewDecoder(req.Body).Decode(&v) != nil {
			http.Error(w, "request", 400)
			return
		}
		var result any
		switch v.Method {
		case "getcurrentnet":
			result = o.config.Ownership.Source.Network
		case "getblockhash":
			result = o.config.Ownership.Source.Genesis
		case "getchainobservation":
			result = fighterownership.ChainObservation{Version: 1, InstanceID: instance.Load().(string), Revision: revision.Load().(string), Hash: o.config.Ownership.Source.Genesis, Height: 0}
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
		}{result, nil, v.ID})
	}))
	defer server.Close()
	o.node, e = fighterownership.NewOwnedChainSource(ctx, o.config.Ownership, prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "fixture", Password: "fixture"}, r.life)
	if e != nil {
		t.Fatal(e)
	}
	first, e := r.RefreshOwnership(ctx, 128)
	if e != nil || !first.Ready {
		t.Fatal("initial normal genesis replay", e)
	}
	again, e := r.RefreshOwnership(ctx, 128)
	if e != nil || !again.Ready || *again.Head != *first.Head {
		t.Fatal("unchanged observation not reusable", e)
	}
	for _, change := range []func(){func() { revision.Store("2") }, func() { instance.Store(strings.Repeat("74", 32)) }} {
		change()
		next, e := r.RefreshOwnership(ctx, 128)
		if e != nil || !next.Ready || next.Head == nil {
			t.Fatal("changed observation replay", e)
		}
		if next.Head.ChainHash != first.Head.ChainHash || next.Head.ChainHeight != first.Head.ChainHeight || next.Head.PublicationSequence <= first.Head.PublicationSequence {
			t.Fatal("same-tip observation reused stale head")
		}
		lease, e := o.store.AcquireHead(ctx, *next.Head)
		if e != nil {
			t.Fatal(e)
		}
		if e = lease.Check(); e != nil {
			t.Fatal("new observation not installed", e)
		}
		lease.Close()
		first = next
	}
}
func TestOwnedSourceRuntimeRetainedIdentity(t *testing.T) {
	r := ownedSourceRuntimeTest(t)
	if e := r.openPoolIssuer(false); e != nil {
		t.Fatal(e)
	}
	if r.source.AuthorityPin != r.owned.config.Ownership.Source {
		t.Fatal("source substituted")
	}
	if e := poolRuntimeRootFilesMode(r.config.DataDir, true); e != nil {
		t.Fatal(e)
	}
	if e := poolRuntimeRootFiles(r.config.DataDir); e == nil {
		t.Fatal("old route accepted owned store")
	}
	p, e := r.issuer.PublicPublication(r.owned.config.Ownership)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Status(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = r.issuer.Close(); e != nil {
		t.Fatal(e)
	}
	r.issuer = nil
	if e = r.openPoolIssuer(true); e != nil {
		t.Fatal("exact established reopen", e)
	}
	if e = r.issuer.Close(); e != nil {
		t.Fatal(e)
	}
	r.issuer = nil
	original := r.owned.config
	r.owned.config.Store.StoreID = strings.Repeat("75", 32)
	if e = r.openPoolIssuer(true); e == nil || r.issuer != nil {
		t.Fatal("marker supplied replacement StoreID")
	}
	r.owned.config = original
	if e = os.Remove(filepath.Join(r.config.DataDir, ownedSourceConfigName)); e != nil {
		t.Fatal(e)
	}
	if e = r.openPoolIssuer(true); e == nil || r.issuer != nil {
		t.Fatal("missing established trust recreated")
	}
}
func TestOwnedSourceRuntimeIntentBeforeDatabase(t *testing.T) {
	r := ownedSourceRuntimeTest(t)
	if e := writePoolRuntimeRecord(filepath.Join(r.config.DataDir, ownedSourceConfigName), r.owned.config); e != nil {
		t.Fatal(e)
	}
	if e := writePoolRuntimeRecord(filepath.Join(r.config.DataDir, poolRuntimeSourceName), r.source); e != nil {
		t.Fatal(e)
	}
	if e := r.openPoolIssuer(false); e != nil {
		t.Fatal("exact pre-database intent cannot resume", e)
	}
	if e := r.issuer.Close(); e != nil {
		t.Fatal(e)
	}
	r.issuer = nil
	// Established source absence is never the same as unfinished creation.
	if e := os.Rename(filepath.Join(r.config.DataDir, "issuer"), filepath.Join(t.TempDir(), "retained-issuer")); e != nil {
		t.Fatal(e)
	}
	if e := r.openPoolIssuer(true); e == nil {
		t.Fatal("missing established issuer recreated")
	}
}
func TestOwnedSourceRuntimeRejectsLegacyAuthority(t *testing.T) {
	r := ownedSourceRuntimeTest(t)
	old, e := registry.CreateManifestAuthority(filepath.Join(r.config.DataDir, "issuer"), r.marker.Network, r.marker.Genesis, r.marker.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	if e = old.Close(); e != nil {
		t.Fatal(e)
	}
	if e = r.openPoolIssuer(false); e == nil || r.issuer != nil {
		t.Fatal("legacy authority adopted")
	}
	if _, e = os.Lstat(filepath.Join(r.config.DataDir, ownedSourceConfigName)); !os.IsNotExist(e) {
		t.Fatal("legacy source blessed with public guard", e)
	}
}
