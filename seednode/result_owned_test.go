// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/wire/v2"
)

type resultRPCRequest struct {
	ID     uint64
	Method string
	Params []json.RawMessage
}
type resultHarness struct {
	p         *ResultPublisher
	coin      FundingCoin
	tip       Tip
	canonical map[uint32]string
	headers   map[string]string
	bodies    map[string]string
	sent      []string
	calls     []string
	hook      func(resultRPCRequest) error
	closed    chan struct{}
}

// This is a synthetic owned-process/RPC boundary fixture, not a launched node
// or a claim of chain acceptance. Configuration/marker checks remain real.
func newResultHarness(t *testing.T) *resultHarness {
	t.Helper()
	ref, _, coin := resultFixture(t)
	var pin map[string]json.RawMessage
	ref.value(t, "pin", &pin)
	for {
		if _, ok := pin["participants"]; ok {
			break
		}
		var next map[string]json.RawMessage
		if json.Unmarshal(pin["source_pin"], &next) != nil || next == nil {
			t.Fatal("event pin fixture")
		}
		pin = next
	}
	raw, _ := json.Marshal(pin)
	var source gymevent.Pin
	if json.Unmarshal(raw, &source) != nil {
		t.Fatal("source decode")
	}
	id, err := ChainIdentityFor(source)
	if err != nil {
		t.Fatal(err)
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	binding := OwnerBinding{Home: home, Source: source}
	var configs [2]ownedConfig
	for i, name := range []string{"primary", "peer"} {
		for _, suffix := range []string{"-chain", "-logs"} {
			if e := os.Mkdir(filepath.Join(home, name+suffix), 0700); e != nil {
				t.Fatal(e)
			}
		}
		rpc, connect, tail := "127.0.0.1:12341", "127.0.0.1:12343", "nolisten=1\n"
		if i == 1 {
			rpc, connect, tail = "127.0.0.1:12342", "127.0.0.1:12344", "listen=127.0.0.1:12343\n"
		}
		path := filepath.Join(home, name+".conf")
		config := "rpcuser=local-pilot\nrpcpass=" + strings.Repeat("01", 32) + "\nnotls=1\nrpclisten=" + rpc + "\ndatadir=" + filepath.Join(home, name+"-chain") + "\nlogdir=" + filepath.Join(home, name+"-logs") + "\nnodnsseed=1\nlocalwalletpilot=1\nconnect=" + connect + "\n" + tail
		if e := os.WriteFile(path, []byte(config), 0600); e != nil {
			t.Fatal(e)
		}
		proc := OwnedProcess{Cmd: &exec.Cmd{Path: binary, Args: []string{binary, "--configfile=" + path}, Process: &os.Process{Pid: os.Getpid() + i}}, Done: closed}
		configs[i], err = configFor(home, name, proc)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			binding.Primary = proc
		} else {
			binding.Peer = proc
		}
	}
	for _, entry := range source.Participants {
		sp := entry.StablePin
		if sp.AccountID != id.PublisherAccountID {
			continue
		}
		marker := map[string]any{"version": 1, "mode": "local-manifest-pilot", "phase": "ready", "account_authority_id": sp.AccountAuthorityID, "account_id": sp.AccountID, "fixture": "zero256", "network": id.Network, "genesis": id.Genesis, "activation_height": 62, "recovery_profile": sp.Identity.Profile, "authority_port": 12345, "descriptor_id": sp.Source.DescriptorID, "identity": map[string]string{"root_public_key": sp.Identity.RootPublicKey, "gym_public_key": sp.Identity.GymPublicKey, "gym_id": sp.Identity.GymID}, "payout": "fixture", "authority_pin": sp.Source, "transport_trust_id": "fixture"}
		raw, _ := json.Marshal(marker)
		for _, name := range []string{"SLUG-MANIFEST-PILOT.json", "SLUG-MANIFEST-PILOT-READY.json", "SLUG-MANIFEST-PILOT-CHAINS.json"} {
			if e := os.WriteFile(filepath.Join(home, name), raw, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	owner := &Publisher{ctx: ctx, cancel: cancel, watchDone: make(chan struct{}), binding: binding, identity: id, configs: configs, checked: map[string]string{}}
	h := &resultHarness{p: &ResultPublisher{owner: owner}, coin: coin, tip: Tip{Height: 300, Hash: strings.Repeat("11", 32)}, canonical: map[uint32]string{0: id.Genesis, coin.BlockHeight: coin.BlockHash}, headers: map[string]string{}, bodies: map[string]string{}, closed: closed}
	h.canonical[h.tip.Height] = h.tip.Hash
	for i := 0; i < 2; i++ {
		idx := i
		owner.nodes[i] = &nodeRPC{endpoint: "http://127.0.0.1:1", client: &http.Client{Transport: referenceTransport(func(r *http.Request) (*http.Response, error) {
			var q resultRPCRequest
			if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
				return nil, e
			}
			h.calls = append(h.calls, q.Method)
			if h.hook != nil {
				if e := h.hook(q); e != nil {
					return nil, e
				}
			}
			var result any
			switch q.Method {
			case "getcurrentnet":
				result = id.Network
			case "getblockhash":
				var height uint32
				_ = json.Unmarshal(q.Params[0], &height)
				result = h.canonical[height]
			case "getbestblock":
				result = h.tip
			case "getblockchaininfo":
				status := "defined"
				if h.tip.Height+1 >= 62 {
					status = "active"
				}
				deps := map[string]any{}
				for _, n := range []string{"csv", "segwit", "taproot"} {
					deps[n] = map[string]any{"status": status, "always_active_height": 62}
				}
				result = map[string]any{"chain": "mainnet", "blocks": h.tip.Height, "bestblockhash": h.tip.Hash, "bip9_softforks": deps}
			case "getpeerinfo":
				addr := "127.0.0.1:12343"
				if idx == 1 {
					addr = "127.0.0.1:12399"
				}
				result = []any{map[string]any{"addr": addr, "inbound": idx == 1}}
			case "gettxout":
				result = map[string]any{"bestblock": h.tip.Hash, "confirmations": h.tip.Height - h.coin.BlockHeight + 1, "value": json.Number("0.001"), "scriptPubKey": map[string]any{"hex": h.coin.Script}, "coinbase": h.coin.Coinbase}
			case "sendrawtransaction":
				var raw string
				_ = json.Unmarshal(q.Params[0], &raw)
				h.sent = append(h.sent, raw)
				data, _ := hex.DecodeString(raw)
				tx := new(wire.MsgTx)
				if tx.Deserialize(bytes.NewReader(data)) != nil {
					t.Fatal("dispatch bytes")
				}
				result = tx.TxHash().String()
			case "getblock":
				var hash string
				_ = json.Unmarshal(q.Params[0], &hash)
				result = h.bodies[hash]
			case "getblockheader":
				var hash string
				_ = json.Unmarshal(q.Params[0], &hash)
				var verbose bool
				_ = json.Unmarshal(q.Params[1], &verbose)
				if verbose {
					result = map[string]any{"hash": hash, "height": h.tip.Height}
				} else {
					result = h.headers[hash]
				}
			default:
				t.Fatal("unexpected RPC", q.Method)
			}
			raw, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": q.ID, "result": result, "error": nil})
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw))}, nil
		})}}
	}
	go func() {
		defer close(owner.watchDone)
		select {
		case <-ctx.Done():
		case <-closed:
		}
		cancel()
	}()
	t.Cleanup(func() { _ = h.p.Close() })
	if e := owner.checkOwner(); e != nil {
		t.Fatal("synthetic owner setup", e)
	}
	return h
}

func TestResultOwnedPublicationRetainsAlternateBytes(t *testing.T) {
	ref, carrier, coin := resultFixture(t)
	h := newResultHarness(t)
	if got := h.p.Identity(); got.Purpose != "candidate_result_v1" || got.FundingPublicKey != ref.Core.Facts.FundingPublicKey || got.FundingScript != coin.Script {
		t.Fatal("purpose identity")
	}
	raw, e := h.p.BuildPublication(context.Background(), carrier, coin)
	if e != nil || hex.EncodeToString(raw) != ref.Core.Facts.Transaction.Raw {
		t.Fatal("build", e)
	}
	alternate, _ := hex.DecodeString(ref.Core.Facts.AlternateRaw)
	txid, e := h.p.SubmitPublication(context.Background(), carrier, coin, alternate)
	if e != nil || txid != ref.Core.Facts.Transaction.TxID || !reflect.DeepEqual(h.sent, []string{ref.Core.Facts.AlternateRaw}) {
		t.Fatal("retained alternate submission", e)
	}
	for _, height := range []uint32{60, 99} {
		h.tip.Height = height
		h.canonical[height] = h.tip.Hash
		if _, e = h.p.BuildPublication(context.Background(), carrier, coin); !errors.Is(e, ErrNotMature) {
			t.Fatalf("immature height %d: %v", height, e)
		}
	}
	h.tip.Height = 100
	h.canonical[100] = h.tip.Hash
	if _, e = h.p.BuildPublication(context.Background(), carrier, coin); e != nil {
		t.Fatal("prospective maturity boundary", e)
	}
}

func TestResultOwnedDispatchUnknownAndNoRetry(t *testing.T) {
	ref, carrier, coin := resultFixture(t)
	raw, _ := hex.DecodeString(ref.Core.Facts.AlternateRaw)
	for _, mode := range []string{"lost reply", "cancel after dispatch", "config after dispatch"} {
		t.Run(mode, func(t *testing.T) {
			h := newResultHarness(t)
			dispatch := 0
			h.hook = func(q resultRPCRequest) error {
				if q.Method == "sendrawtransaction" {
					dispatch++
					switch mode {
					case "lost reply":
						return io.ErrUnexpectedEOF
					case "cancel after dispatch":
						h.p.owner.cancel()
					case "config after dispatch":
						return os.WriteFile(h.p.owner.configs[0].path, []byte("changed"), 0600)
					}
				}
				return nil
			}
			if _, e := h.p.SubmitPublication(context.Background(), carrier, coin, raw); !errors.Is(e, ErrOutcomeUnknown) || dispatch != 1 {
				t.Fatal("dispatch uncertainty/retry", e, dispatch)
			}
		})
	}
	h := newResultHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := h.p.SubmitPublication(ctx, carrier, coin, raw); !errors.Is(e, ErrOffline) || len(h.sent) != 0 {
		t.Fatal("cancelled dispatch", e)
	}
	bad := append([]byte{}, raw...)
	bad[0] ^= 1
	if _, e := h.p.SubmitPublication(context.Background(), carrier, coin, bad); !errors.Is(e, ErrInvalid) || len(h.sent) != 0 {
		t.Fatal("malformed dispatch", e)
	}
}

func TestResultOwnedConfigurationAndCloseJoin(t *testing.T) {
	h := newResultHarness(t)
	work, end, e := h.p.begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	joined := make(chan struct{})
	go func() { _ = h.p.Close(); close(joined) }()
	select {
	case <-work.Done():
	case <-time.After(time.Second):
		t.Fatal("borrow cancellation")
	}
	select {
	case <-joined:
		t.Fatal("returned before active work joined")
	default:
	}
	end()
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("borrow join")
	}
	select {
	case <-h.closed:
		t.Fatal("borrow closed nodes")
	default:
	}
	if _, e = h.p.Observe(context.Background()); !errors.Is(e, ErrOffline) {
		t.Fatal("closed borrow", e)
	}
	h = newResultHarness(t)
	if e = os.WriteFile(h.p.owner.configs[0].path, []byte("changed"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = h.p.CanonicalHash(context.Background(), 0); e == nil {
		t.Fatal("changed config accepted")
	}
}

func TestResultHistoricalExportsAndNormalBody(t *testing.T) {
	if wire.TestPoW {
		t.Skip("retained normal-work body")
	}
	raw, e := os.ReadFile(resultReferenceRoot + "inputs/actual47/public-chain-evidence.json")
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "3a8416d1b27fd02c42ac215e8472e4c5a7f5ab72464e0ccd9ed2683b622ace0e" {
		t.Fatal("retained chain changed")
	}
	var rows []struct {
		Height uint32 `json:"height"`
		Hash   string `json:"hash"`
		Raw    string `json:"block_hex"`
	}
	if json.Unmarshal(raw, &rows) != nil || len(rows) != 176 {
		t.Fatal("retained chain inventory")
	}
	row := rows[1]
	body, _ := hex.DecodeString(row.Raw)
	b := new(wire.MsgBlock)
	if b.Deserialize(bytes.NewReader(body)) != nil {
		t.Fatal("body")
	}
	h := newResultHarness(t)
	h.bodies[row.Hash] = row.Raw
	h.headers[row.Hash] = headerHex(&b.Header)
	// Deliberately a different current branch at the same height. Historical
	// export must use the requested hash and never consult this canonical row.
	h.canonical[row.Height] = strings.Repeat("ab", 32)
	ref := HeaderRef{Height: row.Height, Hash: row.Hash}
	header, e := h.p.ExportHeader(context.Background(), ref)
	if e != nil || header.Header != headerHex(&b.Header) || len(h.p.owner.checked) != 0 {
		t.Fatal("historical header/no work", e)
	}
	got, e := h.p.ExportBlock(context.Background(), header)
	if e != nil || !bytes.Equal(got, body) || len(h.p.owner.checked) != 1 {
		t.Fatal("historical normal body", e)
	}
	for _, method := range h.calls {
		if method == "getblockhash" {
			t.Fatal("historical export substituted canonical branch")
		}
	}
	h.canonical[row.Height] = row.Hash
	h.canonical[0] = rows[0].Hash
	if _, e = h.p.Block(context.Background(), row.Height); e != nil || len(h.p.owner.checked) != 1 {
		t.Fatal("canonical block/reused work", e)
	}
	wrong := header
	wrong.Header = strings.Repeat("00", 80)
	if _, e = h.p.ExportHeader(context.Background(), wrong); e == nil {
		t.Fatal("wrong retained header")
	}
	if _, e = h.p.ExportBlock(context.Background(), wrong); e == nil {
		t.Fatal("wrong retained body header")
	}
}

func TestResultFinalMembershipDepthWitness(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		h := newResultHarness(t)
		anchor := Anchor{Height: 200, Hash: strings.Repeat("22", 32)}
		tip := Tip{Height: 260, Hash: strings.Repeat("33", 32)}
		h.canonical[200] = anchor.Hash
		h.canonical[260] = tip.Hash
		if replacement {
			h.canonical[260] = strings.Repeat("44", 32)
		}
		e := h.p.finalMembership(context.Background(), anchor, tip)
		if (e != nil) != replacement {
			t.Fatal("above-inclusion replacement", replacement, e)
		}
		if !reflect.DeepEqual(h.calls, []string{"getblockhash", "getblockhash"}) {
			t.Fatal("unexpected A+1 check", h.calls)
		}
	}
}
