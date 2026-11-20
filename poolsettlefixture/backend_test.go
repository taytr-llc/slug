// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlefixture

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestFixtureAmountsAreExactAndBounded(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		atoms int64
	}{{"10", 1000000000}, {"0.00000001", 1}, {"1e-8", 1}, {"2.1e7", 2100000000000000}, {"0", 0}} {
		got, err := amount([]byte(tc.raw))
		if err != nil || got != tc.atoms {
			t.Fatalf("exact decimal %s = %d, %v", tc.raw, got, err)
		}
	}
	for _, raw := range []string{"1e1000000000", "1e-1000000000", "0.000000001", "21000001", "-1", "1/2", "01", "\"1\"", "NaN", "", "1e+33"} {
		if _, err := amount([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid or non-atomic amount %q", raw)
		}
	}
}

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// This is a controlled RPC model, not mined funding or a cryptographic golden.
// The change transaction stays canonical while its original round changes.
func TestChangeRetainsOriginalRoundFundingGate(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal fixture backend only")
	}
	rootHash, tipHash, blockHash := strings.Repeat("1", 64), strings.Repeat("a", 64), strings.Repeat("b", 64)
	previous := chainhash.Hash{9}
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&previous, 0), nil, nil))
	script, _ := hex.DecodeString(PoolScript())
	tx.AddTxOut(wire.NewTxOut(100000000, script))
	var encoded bytes.Buffer
	if err := tx.Serialize(&encoded); err != nil {
		t.Fatal(err)
	}
	coin := poolsettle.Coin{TxID: tx.TxHash().String(), Index: 0, Atoms: 100000000, Script: PoolScript(), Claims: []poolsettle.Claim{{RoundHash: rootHash, RoundHeight: 1, Amount: poolsettle.Fraction{Numerator: "100000000", Denominator: "1"}}}}
	for _, mode := range []string{"canonical", "original-orphan", "source-rpc-error", "malformed-membership"} {
		t.Run(mode, func(t *testing.T) {
			b := &Backend{endpoint: "http://127.0.0.1:1234", http: &http.Client{}}
			b.http.Transport = fixtureTransport(func(req *http.Request) (*http.Response, error) {
				var q struct {
					ID     uint64            `json:"id"`
					Method string            `json:"method"`
					Params []json.RawMessage `json:"params"`
				}
				if json.NewDecoder(req.Body).Decode(&q) != nil {
					t.Fatal("modeled request")
				}
				var result any
				var refusal *rpcError
				switch q.Method {
				case "getcurrentnet":
					result = uint32(chaincfg.MainNetParams.Net)
				case "getbestblock":
					result = map[string]any{"hash": tipHash, "height": 100}
				case "getblockchaininfo":
					deployments := map[string]any{}
					for _, name := range []string{"csv", "segwit", "taproot"} {
						deployments[name] = map[string]any{"status": "active", "always_active_height": 62}
					}
					result = map[string]any{"chain": "mainnet", "blocks": 100, "bestblockhash": tipHash, "bip9_softforks": deployments}
				case "getblockhash":
					var height uint32
					if len(q.Params) != 1 || json.Unmarshal(q.Params[0], &height) != nil {
						t.Fatal("modeled height")
					}
					switch height {
					case 0:
						result = chaincfg.MainNetParams.GenesisHash.String()
					case 1:
						result = rootHash
						if mode == "original-orphan" {
							result = strings.Repeat("2", 64)
						}
						if mode == "source-rpc-error" {
							result = nil
							refusal = &rpcError{Code: -32603, Message: "modeled failure"}
						}
						if mode == "malformed-membership" {
							result = "invalid"
						}
					case 90:
						result = blockHash
					case 100:
						result = tipHash
					default:
						t.Fatalf("unexpected modeled height%d", height)
					}
				case "getrawtransaction":
					result = map[string]any{"hex": hex.EncodeToString(encoded.Bytes()), "txid": coin.TxID, "blockhash": blockHash, "confirmations": 11}
				case "getblockheader":
					result = map[string]any{"hash": blockHash, "height": 90}
				case "gettxout":
					result = map[string]any{"bestblock": tipHash, "confirmations": 11, "value": 1, "coinbase": false, "scriptPubKey": map[string]any{"hex": PoolScript()}}
				case "gettxspendingprevout":
					result = []map[string]any{{"txid": coin.TxID, "vout": 0}}
				default:
					t.Fatalf("unexpected modeled RPC %s", q.Method)
				}
				raw, err := json.Marshal(map[string]any{"id": q.ID, "result": result, "error": refusal})
				if err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw))}, nil
			})
			funding, err := b.Inspect(context.Background(), []poolsettle.Coin{coin})
			switch mode {
			case "canonical":
				if err != nil || len(funding) != 1 || funding[0].State != "ready" {
					t.Fatalf("canonical change: %+v %v", funding, err)
				}
			case "original-orphan":
				if err != nil || len(funding) != 1 || funding[0].State != "funding-conflict" {
					t.Fatalf("orphaned original funding: %+v %v", funding, err)
				}
			default:
				if err == nil || len(funding) != 0 {
					t.Fatalf("unavailable/malformed evidence became chain observation: %+v %v", funding, err)
				}
			}
		})
	}
}

func TestFixturePublicIdentityAndEndpointScope(t *testing.T) {
	// Standard secp256k1 scalar3 public point, independently published.
	if PoolPublicKey() != "02f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f9" {
		t.Fatal("fixture key changed")
	}
	for _, endpoint := range []string{"https://127.0.0.1:1234", "http://example.com:1234", "http://localhost:1234", "http://127.0.0.1:0", "http://127.0.0.1:1234/path", "http://127.0.0.1:1234?x=1"} {
		cfg := prospectnode.RPCConfig{Version: 1, URL: endpoint, Username: "fixture", Password: "fixture"}
		if b, err := New(context.Background(), cfg); err == nil || b != nil {
			t.Fatalf("accepted nonfixture endpoint %s", endpoint)
		}
	}
}
