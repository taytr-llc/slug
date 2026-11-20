//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/taytr-llc/slug/prospectnode"
)

// This composition uses the real RPC adapter against controlled node responses.
// It proves exact request/observation binding, not a live-chain deployment.
func TestLateWinnerRealRPCBinding(t *testing.T) {
	for _, mode := range []string{"canonical", "wrong-parent", "wrong-winner", "wrong-header", "last-membership-reorg"} {
		t.Run(mode, func(t *testing.T) {
			h := setupPool(t, "", 0)
			h.now = h.source.Job.ExpiresAtMS
			initial := h.source.Job.Context
			var submissions, winnerReads, headerReads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var call struct {
					ID     uint64            `json:"id"`
					Method string            `json:"method"`
					Params []json.RawMessage `json:"params"`
				}
				if json.NewDecoder(r.Body).Decode(&call) != nil {
					t.Error("invalid RPC request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var result any
				switch call.Method {
				case "getcurrentnet":
					result = initial.Network
				case "getblockhash":
					var height uint32
					if len(call.Params) != 1 || json.Unmarshal(call.Params[0], &height) != nil {
						t.Error("malformed height query")
						return
					}
					switch height {
					case 0:
						result = initial.Genesis
					case initial.ParentHeight:
						result = initial.ParentHash
						if mode == "wrong-parent" {
							result = strings.Repeat("1", 64)
						}
					case initial.ParentHeight + 1:
						reads := winnerReads.Add(1)
						result = h.winner.WorkHash
						if mode == "wrong-winner" || mode == "last-membership-reorg" && reads > 1 {
							result = strings.Repeat("2", 64)
						}
					default:
						t.Errorf("unexpected height %d", height)
					}
				case "getblockheader":
					headerReads.Add(1)
					var requested string
					if len(call.Params) != 2 || json.Unmarshal(call.Params[0], &requested) != nil || requested != h.winner.WorkHash || string(call.Params[1]) != "false" {
						t.Error("canonical observation did not request the exact raw winning header")
					}
					result = h.winner.Header
					if mode == "wrong-header" {
						result = strings.Repeat("00", 80)
					}
				case "submitblock":
					submissions.Add(1)
					var block string
					if len(call.Params) != 1 || json.Unmarshal(call.Params[0], &block) != nil || h.winner.Candidate == nil || block != h.winner.Candidate.BlockHex {
						t.Error("broadcast differs from independently frozen payout-bound winning block")
					}
					// A successful processing acknowledgment is deliberately the
					// same in every case; only exact membership can grant credit.
					result = nil
				case "getblockchaininfo":
					result = map[string]any{"chain": "mainnet", "blocks": initial.ParentHeight,
						"bestblockhash": initial.ParentHash, "mediantime": initial.MinTimestamp - 1}
				case "getbestblock":
					result = map[string]any{"hash": initial.ParentHash, "height": initial.ParentHeight}
				case "getblocktemplate":
					result = h.source.Template.Block
				default:
					t.Errorf("unexpected RPC %s", call.Method)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": call.ID, "result": result, "error": nil})
			}))
			t.Cleanup(server.Close)
			client, err := prospectnode.NewClient(prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "fixture", Password: "public-fixture-rpc"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			h.pool.node = client
			got, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
			if err != nil {
				t.Fatal(err)
			}
			if submissions.Load() != 1 || got.WorkHash != h.winner.WorkHash || got.Header != h.winner.Header || got.BlockHeight != initial.ParentHeight+1 {
				t.Fatal("lost exact block binding or broadcast")
			}
			if mode == "canonical" {
				if got.AcceptedSequence != 1 || got.Acceptance != "canonical-late-winner" || got.BlockState != "canonical-observed" || headerReads.Load() != 1 || winnerReads.Load() != 2 {
					t.Fatalf("canonical late exception not backed by complete RPC observation: %+v", got)
				}
			} else if got.AcceptedSequence != 0 || got.AcceptedAtMS != 0 || got.Acceptance != "" || got.BlockState == "canonical-observed" {
				t.Fatalf("processing acknowledgment or mismatched canonical data granted credit: %+v", got)
			}
		})
	}
}
