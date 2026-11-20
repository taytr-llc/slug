//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

// This uses the independently frozen, previously normal-work-qualified actual
// sequence-one public record. Test servers supply its exact public bytes; the
// product must verify header work and admission again. No private issuer DB,
// fixture signer, local authority callback, mining, or expected Go hashes.
func TestOwnershipOperatorActualAdmissionPublicReplay(t *testing.T) {
	ctx := context.Background()
	corpus := operatorCoreCorpus(t)
	x := corpus.Compositions["v1/operator_v2_distinct"]
	profile, e := registry.NewOperatorProfile(x.Pin)
	if e != nil {
		t.Fatal(e)
	}
	record := x.Admission.Record
	if len(record.Proof.MerkleBranch) != 0 || record.Context.ParentHeight != 0 {
		t.Fatal("one-block input selection")
	}
	header, e := hex.DecodeString(record.Proof.Header)
	if e != nil {
		t.Fatal(e)
	}
	coinbase, e := hex.DecodeString(record.Proof.Coinbase)
	if e != nil {
		t.Fatal(e)
	}
	block := append(append(append([]byte{}, header...), 1), coinbase...)
	var genesisHeader, genesisBlock bytes.Buffer
	if e = chaincfg.MainNetParams.GenesisBlock.Header.Serialize(&genesisHeader); e != nil {
		t.Fatal(e)
	}
	if e = chaincfg.MainNetParams.GenesisBlock.Serialize(&genesisBlock); e != nil {
		t.Fatal(e)
	}
	nodeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     uint64            `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&q) != nil {
			http.Error(w, "invalid", 400)
			return
		}
		var result any
		switch q.Method {
		case "getcurrentnet":
			result = x.Pin.Source.Network
		case "getchainobservation":
			result = ChainObservation{Version: 1, InstanceID: strings.Repeat("a8", 32), Revision: "2", Hash: record.Verified.WorkHash, Height: 1}
		case "getblockhash":
			var height uint32
			if len(q.Params) != 1 || json.Unmarshal(q.Params[0], &height) != nil || height > 1 {
				http.Error(w, "height", 400)
				return
			}
			if height == 0 {
				result = x.Pin.Source.Genesis
			} else {
				result = record.Verified.WorkHash
			}
		case "getblockheader", "getblock":
			var hash string
			if len(q.Params) < 1 || json.Unmarshal(q.Params[0], &hash) != nil {
				http.Error(w, "hash", 400)
				return
			}
			switch hash {
			case x.Pin.Source.Genesis:
				if q.Method == "getblock" {
					result = hex.EncodeToString(genesisBlock.Bytes())
				} else {
					result = hex.EncodeToString(genesisHeader.Bytes())
				}
			case record.Verified.WorkHash:
				if q.Method == "getblock" {
					result = hex.EncodeToString(block)
				} else {
					result = record.Proof.Header
				}
			default:
				http.Error(w, "unknown", 404)
				return
			}
		default:
			http.Error(w, "method", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(struct {
			Result any    `json:"result"`
			Error  any    `json:"error"`
			ID     uint64 `json:"id"`
		}{result, nil, q.ID})
	}))
	defer nodeServer.Close()
	node, e := NewOperatorOwnedChainSource(ctx, profile, prospectnode.RPCConfig{Version: 1, URL: nodeServer.URL, Username: "public-fixture", Password: "public-fixture"}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer node.Close()
	var publication PublicationEntry
	publicationID := ""
	for _, row := range corpus.Positives {
		if row.Family == "publication" && row.Name == "v1/operator_v2_distinct/admission" {
			if e = json.Unmarshal(row.Value, &publication); e != nil {
				t.Fatal(e)
			}
			publicationID = row.IDs["publication_id"]
		}
	}
	if publicationID == "" {
		t.Fatal("independent admission publication absent")
	}
	status := PublicReadStatus{Version: 2, PinID: profile.ID(), Publication: registry.PublicationStatus{Version: 1, PinID: profile.ID(), AcceptedSequence: 1, SignedAdmissionSequence: 1, AdmissionHead: x.State.BirthAdmissionID, PublicationSequence: 1, PublicationHead: publicationID}, FinalityBodyHead: zeroID, FinalityHead: zeroID}
	publicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q publicReadRequest
		if r.URL.Path != "/slug/ownership/v2" || json.NewDecoder(r.Body).Decode(&q) != nil || q.validateSelected(profile.ID(), 2) != nil {
			http.Error(w, "request", 400)
			return
		}
		var result any
		switch q.Method {
		case "status":
			result = status
		case "admission":
			if q.Sequence != 1 {
				http.Error(w, "sequence", 400)
				return
			}
			result = x.Admission
		case "publication":
			if q.Sequence != 1 {
				http.Error(w, "sequence", 400)
				return
			}
			result = publication
		default:
			http.Error(w, "method", 400)
			return
		}
		raw, _ := json.Marshal(result)
		response, _ := json.Marshal(publicReadResponse{Version: 2, PinID: profile.ID(), ID: q.ID, Method: q.Method, Result: raw, Error: ""})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}))
	defer publicServer.Close()
	reader, e := NewOperatorPublicReader(ctx, profile, PublicReadConfig{URL: publicServer.URL + "/slug/ownership/v2"}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	source, e := NewOperatorRemotePublicSource(ctx, profile, reader)
	if e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	store, e := CreateOperatorReplica(filepath.Join(root, "public-only"), profile, StoreIdentity{strings.Repeat("a9", 32)})
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	replay, e := store.BeginReplay(ctx, source, node)
	if e != nil {
		t.Fatal(e)
	}
	progress := boundedReplayFinish(t, replay)
	if progress.Head == nil || progress.Head.AdmissionSequence != 1 || progress.Head.ChainHeight != 1 || progress.Head.ChainHash != record.Verified.WorkHash {
		t.Fatal("actual public admission replay head")
	}
	capture, e := store.CaptureOwned(ctx, record.Authorization.PublicKey)
	if e != nil {
		t.Fatal(e)
	}
	item, e := store.OwnedItem(ctx, capture, record.Verified.ProspectID)
	if e != nil {
		t.Fatal(e)
	}
	var final FinalStateEvidence
	if e = json.Unmarshal(x.State.Evidence, &final); e != nil {
		t.Fatal(e)
	}
	if item.State.StateID != final.PriorStateID || item.State.Generation != 0 || item.State.OwnerPublicKey != record.Authorization.PublicKey {
		t.Fatal("independent original owner/state")
	}
	original, _ := json.Marshal(x.Admission)
	received, _ := json.Marshal(item.Admission)
	if !bytes.Equal(original, received) || store.operator != nil || len(store.frames) != 0 || len(store.frameBodies) != 0 {
		t.Fatal("birth evidence changed or replica signed")
	}
}
