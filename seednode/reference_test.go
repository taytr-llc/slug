// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

const referenceRoot = "../gymseed/testdata/reference"

func readReference(t *testing.T, path, want string, out any) {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(referenceRoot, path))
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != want {
		t.Fatal("independent reference changed", path)
	}
	if e = json.Unmarshal(raw, out); e != nil {
		t.Fatal(e)
	}
}
func TestIndependentPublicationAndRPCAmounts(t *testing.T) {
	var ref struct {
		Crypto struct {
			Publication struct{ Payload, RawTx, TxID, RawTxDigest string } `json:"publication"`
		} `json:"crypto"`
		Amounts []struct {
			Token    string
			Accepted bool
			Satoshis int64
		} `json:"rpc_amount_cases"`
	}
	// Explicit tags preserve the frozen snake_case wire names.
	var corpus map[string]json.RawMessage
	readReference(t, "seed_v1.json", "2b8d78e8ef665e372543eb87bb5d25a975026a0ae946b15a871ac97bbf2153f0", &corpus)
	if e := json.Unmarshal(corpus["rpc_amount_cases"], &ref.Amounts); e != nil {
		t.Fatal(e)
	}
	for _, c := range ref.Amounts {
		got, e := amountSatoshis([]byte(c.Token))
		if (e == nil) != c.Accepted || e == nil && got != c.Satoshis {
			t.Fatalf("amount %q: got=%d err=%v expected=%d accepted=%v", c.Token, got, e, c.Satoshis, c.Accepted)
		}
	}
	var crypto struct {
		Publication struct {
			Payload string `json:"payload"`
			Raw     string `json:"raw_tx"`
			TxID    string `json:"txid"`
			Digest  string `json:"raw_tx_digest"`
		} `json:"publication"`
	}
	if e := json.Unmarshal(corpus["crypto"], &crypto); e != nil {
		t.Fatal(e)
	}
	var blocks struct {
		Coin FundingCoin `json:"coin"`
	}
	readReference(t, "fixtures/block_index.json", "2614db26e86aa553d51659981c295fbcb9d83a8d7681ec4ac28bbea14430ab69", &blocks)
	raw, e := hex.DecodeString(crypto.Publication.Payload)
	if e != nil || len(raw) != 73 {
		t.Fatal("reference carrier")
	}
	var carrier [73]byte
	copy(carrier[:], raw)
	tx, got, e := publication(carrier, blocks.Coin)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(got)
	if hex.EncodeToString(got) != crypto.Publication.Raw || tx.TxHash().String() != crypto.Publication.TxID || hex.EncodeToString(h[:]) != crypto.Publication.Digest {
		t.Fatal("publication differs from independently derived BIP143/RFC6979 bytes")
	}
}

type referenceTransport func(*http.Request) (*http.Response, error)

func (f referenceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestIndependentFullAnchorBodies(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-work independent bodies")
	}
	var index struct {
		Anchor  Anchor `json:"anchor"`
		Maximum Anchor `json:"maximum_anchor"`
		Blocks  []struct {
			Name, Hash, Previous, Header, File, SHA256 string
			Height                                     uint32
			Bytes                                      int
			Merkle                                     string `json:"merkle_root"`
		} `json:"blocks"`
	}
	readReference(t, "fixtures/block_index.json", "2614db26e86aa553d51659981c295fbcb9d83a8d7681ec4ac28bbea14430ab69", &index)
	for _, a := range []Anchor{index.Anchor, index.Maximum} {
		t.Run(a.Hash, func(t *testing.T) {
			var body []byte
			var parent string
			for _, row := range index.Blocks {
				if row.Hash != a.Hash {
					continue
				}
				var e error
				body, e = os.ReadFile(filepath.Join(referenceRoot, row.File))
				if e != nil {
					t.Fatal(e)
				}
				h := sha256.Sum256(body)
				if len(body) != row.Bytes || hex.EncodeToString(h[:]) != row.SHA256 {
					t.Fatal("frozen body changed")
				}
				parent = row.Previous
			}
			if len(body) == 0 {
				t.Fatal("anchor body missing")
			}
			calls := 0
			rpc := &nodeRPC{endpoint: "http://127.0.0.1:1", client: &http.Client{Transport: referenceTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				var q struct {
					ID     uint64
					Method string
					Params []json.RawMessage
				}
				if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
					t.Fatal(e)
				}
				var result any
				switch q.Method {
				case "getblockhash":
					var height uint32
					if json.Unmarshal(q.Params[0], &height) != nil {
						t.Fatal("height")
					}
					if height == a.Height {
						result = a.Hash
					} else if height+1 == a.Height {
						result = parent
					} else {
						t.Fatal("unexpected height", height)
					}
				case "getblock":
					result = hex.EncodeToString(body)
				default:
					t.Fatal("unexpected RPC", q.Method)
				}
				raw, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": q.ID, "result": result, "error": nil})
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw))}, nil
			})}}
			p := &Publisher{nodes: [2]*nodeRPC{rpc, nil}, checked: map[string]string{}}
			if e := p.verifyAnchor(context.Background(), a); e != nil {
				t.Fatal(e)
			}
			if len(p.checked) != 1 || calls < 4 {
				t.Fatal("full block was not checked with one normal header hash")
			}
			changed := a
			payload, _ := hex.DecodeString(changed.Payload)
			payload[len(payload)-1] ^= 1
			changed.Payload = hex.EncodeToString(payload)
			if e := p.verifyAnchor(context.Background(), changed); e == nil {
				t.Fatal("compact Merkle proof falsely established changed carrier payload")
			}
			if len(p.checked) != 1 {
				t.Fatal("repeat header consumed fresh hash work")
			}
		})
	}
}

func TestFinalMembershipRejectsAboveTargetReplacement(t *testing.T) {
	// The captured observation had A=2, T=3 and tip=63. Both scenarios retain
	// A/T; only the ordinary extension retains the depth witness at height63.
	a := Anchor{Height: 2, Hash: strings.Repeat("11", 32)}
	target := HeaderRef{Height: 3, Hash: strings.Repeat("22", 32)}
	tip := Tip{Height: 63, Hash: strings.Repeat("33", 32)}
	for _, replace := range []bool{false, true} {
		seen := []uint32{}
		rpc := &nodeRPC{endpoint: "http://127.0.0.1:1", client: &http.Client{Transport: referenceTransport(func(r *http.Request) (*http.Response, error) {
			var q struct {
				ID     uint64
				Method string
				Params []uint32
			}
			if e := json.NewDecoder(r.Body).Decode(&q); e != nil || q.Method != "getblockhash" || len(q.Params) != 1 {
				t.Fatal("membership RPC")
			}
			height := q.Params[0]
			seen = append(seen, height)
			hash := map[uint32]string{a.Height: a.Hash, target.Height: target.Hash, tip.Height: tip.Hash}[height]
			if replace && height == tip.Height {
				hash = strings.Repeat("44", 32)
			}
			raw, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": q.ID, "result": hash, "error": nil})
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw))}, nil
		})}}
		p := &Publisher{nodes: [2]*nodeRPC{rpc, nil}}
		e := p.finalMembership(context.Background(), a, target, tip)
		if (e != nil) != replace || !reflect.DeepEqual(seen, []uint32{2, 3, 63}) {
			t.Fatalf("replace=%v err=%v heights=%v", replace, e, seen)
		}
	}
}
