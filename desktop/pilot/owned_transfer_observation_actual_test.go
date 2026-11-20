//go:build slugownedobservationactual

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

type ownedObservationActualLog struct {
	mu sync.Mutex
	f  *os.File
	n  int
}

func (w *ownedObservationActualLog) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	size := len(b)
	if w.n < 16<<20 {
		keep := min(len(b), (16<<20)-w.n)
		_, e := w.f.Write(b[:keep])
		w.n += keep
		if e != nil {
			return 0, e
		}
	}
	return size, nil
}

type ownedObservationActualNode struct {
	cmd                       *exec.Cmd
	rpc                       *withdrawalNode
	config                    prospectnode.RPCConfig
	done                      chan error
	log                       *os.File
	once                      sync.Once
	clean, forced, unexpected bool
	waitError                 string
}

func (n *ownedObservationActualNode) close() {
	n.once.Do(func() {
		defer n.log.Close()
		defer n.rpc.close()
		select {
		case e := <-n.done:
			n.unexpected = true
			if e != nil {
				n.waitError = e.Error()
			}
			return
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = n.rpc.call(ctx, "stop", []any{})
		cancel()
		select {
		case e := <-n.done:
			if e != nil {
				n.waitError = e.Error()
			}
			n.clean = e == nil
		case <-time.After(12 * time.Second):
			n.forced = true
			_ = n.cmd.Process.Kill()
			select {
			case e := <-n.done:
				if e != nil {
					n.waitError = e.Error()
				}
			case <-time.After(3 * time.Second):
			}
		}
	})
}
func ownedObservationActualPort(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := l.Addr().String()
	if e = l.Close(); e != nil {
		t.Fatal(e)
	}
	return addr
}
func ownedObservationActualStart(t *testing.T, binary, root, name, listen, connect string) *ownedObservationActualNode {
	t.Helper()
	home := filepath.Join(root, name)
	if info, e := os.Stat(filepath.Join(home, "data/mainnet/blocks_ffldb")); e != nil || !info.IsDir() {
		t.Fatal("runner did not install allowlisted chain copy", e)
	}
	rpcAddress := ownedObservationActualPort(t)
	config := fmt.Sprintf("rpcuser=fixture\nrpcpass=public-observation-probe\nnotls=1\nrpclisten=%s\nlisten=%s\nconnect=%s\nnodnsseed=1\nlocalwalletpilot=1\ntxindex=1\ndatadir=%s\nlogdir=%s\n", rpcAddress, listen, connect, filepath.Join(home, "data"), filepath.Join(home, "logs"))
	path := filepath.Join(home, "fixture.conf")
	if e := os.WriteFile(path, []byte(config), 0600); e != nil {
		t.Fatal(e)
	}
	log, e := os.OpenFile(filepath.Join(home, "node.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	cfg := prospectnode.RPCConfig{Version: 1, URL: "http://" + rpcAddress, Username: "fixture", Password: "public-observation-probe"}
	rpc, e := newWithdrawalNode(cfg)
	if e != nil {
		log.Close()
		t.Fatal(e)
	}
	n := &ownedObservationActualNode{cmd: exec.Command(binary, "--configfile="+path), rpc: rpc, config: cfg, done: make(chan error, 1), log: log}
	n.cmd.Stdout = &ownedObservationActualLog{f: log}
	n.cmd.Stderr = n.cmd.Stdout
	if e = n.cmd.Start(); e != nil {
		rpc.close()
		log.Close()
		t.Fatal(e)
	}
	go func() { n.done <- n.cmd.Wait() }()
	t.Cleanup(n.close)
	return n
}
func ownedObservationActualWait(t *testing.T, ctx context.Context, label string, ready func() bool) {
	t.Helper()
	end := time.Now().Add(30 * time.Second)
	for time.Now().Before(end) && ctx.Err() == nil {
		if ready() {
			return
		}
		select {
		case <-ctx.Done():
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("bounded wait", label, ctx.Err())
}
func ownedObservationActualRead(t *testing.T, path, expected string, out any) {
	t.Helper()
	raw, e := os.ReadFile(path)
	if e != nil || len(raw) > 1<<20 || fmt.Sprintf("%x", sha256.Sum256(raw)) != expected {
		t.Fatal("locked public input", path, e)
	}
	if e = json.Unmarshal(raw, out); e != nil {
		t.Fatal(e)
	}
}
func ownedObservationActualRaw(t *testing.T, encoded string) fightercarrier.RawTransaction {
	t.Helper()
	raw, e := hex.DecodeString(encoded)
	if e != nil || len(raw) > 12000 {
		t.Fatal("old public raw frame", e)
	}
	var tx wire.MsgTx
	r := bytes.NewReader(raw)
	if e = tx.Deserialize(r); e != nil || r.Len() != 0 {
		t.Fatal(e)
	}
	var stripped bytes.Buffer
	if e = tx.SerializeNoWitness(&stripped); e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(stripped.Bytes())
	hash = sha256.Sum256(hash[:])
	for i, j := 0, 31; i < j; i, j = i+1, j-1 {
		hash[i], hash[j] = hash[j], hash[i]
	}
	return fightercarrier.RawTransaction{Raw: encoded, TxID: hex.EncodeToString(hash[:]), RawSHA256: fmt.Sprintf("%x", sha256.Sum256(raw))}
}

func TestOwnedTransferActualNodeObservation(t *testing.T) {
	if wire.TestPoW {
		t.Fatal("normal proof profile required")
	}
	root, binary, inputs := os.Getenv("SLUG_OBSERVATION_ROOT"), os.Getenv("SLUG_OBSERVATION_NODE"), os.Getenv("SLUG_OBSERVATION_INPUTS")
	if root == "" || binary == "" || inputs == "" {
		t.Fatal("explicit isolated runner required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var prior struct {
		Status       string                   `json:"status"`
		FinalHeight  uint32                   `json:"final_height"`
		RevealBlock  string                   `json:"reveal_block"`
		FinalHead    fighterownership.HeadRef `json:"final_head"`
		ConflictHead fighterownership.HeadRef `json:"conflict_head"`
		Transfers    []struct {
			CommitRaw      string `json:"commit_raw"`
			SpendRaw       string `json:"spend_raw"`
			TransferID     string `json:"transfer_id"`
			RevealPosition uint32 `json:"reveal_position"`
		} `json:"competing_transfers"`
	}
	ownedObservationActualRead(t, filepath.Join(inputs, "public-result.json"), "a9a63690be44099fcf71d4588384f35c0c3e84ce93e792a49ddc8141e94f721f", &prior)
	var identities struct {
		Pin fighterownership.Pin `json:"pin"`
	}
	identityHash := os.Getenv("SLUG_OBSERVATION_IDENTITY_SHA256")
	ownedObservationActualRead(t, filepath.Join(inputs, "preselected-public-identities.json"), identityHash, &identities)
	if prior.Status != "passed" || prior.FinalHeight != 103 || prior.ConflictHead.ChainHeight != 103 || prior.FinalHead.ChainHeight != 164 || len(prior.Transfers) != 2 {
		t.Fatal("wrong prior public scenario")
	}
	report := map[string]any{"version": 1, "status": "failed", "scope": "actual desktop observation leaf; no account/native authority or natural reorg claim", "mined_blocks": 0, "new_signatures": 0}
	var nodes []*ownedObservationActualNode
	t.Cleanup(func() {
		clean := len(nodes) == 2
		rows := []map[string]any{}
		for _, n := range nodes {
			n.close()
			clean = clean && n.clean && !n.forced && !n.unexpected
			rows = append(rows, map[string]any{"clean": n.clean, "forced_kill": n.forced, "unexpected_exit": n.unexpected, "wait_error": n.waitError})
		}
		report["node_cleanup"] = rows
		if !clean {
			t.Error("node cleanup incomplete")
		}
		if !t.Failed() && clean {
			report["status"] = "passed"
		}
		raw, e := json.MarshalIndent(report, "", "  ")
		if e != nil {
			t.Error(e)
			return
		}
		if e = os.WriteFile(filepath.Join(root, "observation-result.json"), append(raw, '\n'), 0600); e != nil {
			t.Error(e)
		}
	})
	p2pA, p2pB := ownedObservationActualPort(t), ownedObservationActualPort(t)
	a := ownedObservationActualStart(t, binary, root, "primary", p2pA, p2pB)
	nodes = append(nodes, a)
	b := ownedObservationActualStart(t, binary, root, "peer", p2pB, p2pA)
	nodes = append(nodes, b)
	for _, n := range nodes {
		ownedObservationActualWait(t, ctx, "original copied tip103", func() bool {
			var height uint32
			var hash string
			return n.rpc.read(ctx, "getblockcount", []any{}, &height) == nil && height == 103 && n.rpc.read(ctx, "getbestblockhash", []any{}, &hash) == nil && hash == prior.ConflictHead.ChainHash
		})
		ownedObservationActualWait(t, ctx, "owned literal peer", func() bool {
			var peers []struct {
				Addr string `json:"addr"`
			}
			if n.rpc.read(ctx, "getpeerinfo", []any{}, &peers) != nil || len(peers) == 0 || len(peers) > 2 {
				return false
			}
			for _, p := range peers {
				host, _, e := net.SplitHostPort(p.Addr)
				if e != nil || host != "127.0.0.1" {
					t.Fatal("foreign peer")
				}
			}
			return true
		})
		var pool []string
		if e := n.rpc.read(ctx, "getrawmempool", []any{false}, &pool); e != nil || len(pool) != 0 {
			t.Fatal("fresh mempool", e)
		}
	}
	chain, e := fighterownership.NewOwnedChainSource(ctx, identities.Pin, a.config, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer chain.Close()
	observer := &OwnedTransferActions{chain: chain, node: a.rpc}
	commits := []fightercarrier.Observation{}
	for _, v := range prior.Transfers {
		raw := ownedObservationActualRaw(t, v.CommitRaw)
		obs, e := observer.observeCarrier(ctx, raw, v.TransferID)
		if e != nil || obs.State != "canonical" || obs.BlockHeight != 103 || obs.BlockHash != prior.ConflictHead.ChainHash || obs.Confirmations != 1 {
			t.Fatal("actual canonical commit", obs, e)
		}
		commits = append(commits, obs)
	}
	report["commits_at103"] = commits
	first := prior.Transfers[0]
	spend := ownedObservationActualRaw(t, first.SpendRaw)
	var ack string
	if e = a.rpc.read(ctx, "sendrawtransaction", []any{spend.Raw}, &ack); e != nil || ack != spend.TxID {
		t.Fatal("exact saved reveal submission", ack, e)
	}
	obs, e := observer.observeCarrier(ctx, spend, first.TransferID)
	if e != nil || obs.State != "mempool" || obs.RawSHA256 != spend.RawSHA256 || obs.ObservedTxID != spend.TxID {
		t.Fatal("actual mempool observation", obs, e)
	}
	report["mempool_observation"] = obs
	entry, e := a.rpc.call(ctx, "getmempoolentry", []any{spend.TxID})
	if e != nil {
		t.Fatal(e)
	}
	report["mempool_entry"] = json.RawMessage(entry)
	// Restore only already-stored public fixture blocks; no generation or signing.
	for _, n := range nodes {
		if e = ownedObservationActualReconsider(ctx, n, prior.RevealBlock); e != nil {
			t.Fatal("restore saved branch", e)
		}
	}
	for _, n := range nodes {
		ownedObservationActualWait(t, ctx, "saved canonical tip164", func() bool {
			var height uint32
			var hash string
			return n.rpc.read(ctx, "getblockcount", []any{}, &height) == nil && height == 164 && n.rpc.read(ctx, "getbestblockhash", []any{}, &hash) == nil && hash == prior.FinalHead.ChainHash
		})
	}
	reveals := []fightercarrier.Observation{}
	for _, v := range prior.Transfers {
		raw := ownedObservationActualRaw(t, v.SpendRaw)
		obs, e := observer.observeCarrier(ctx, raw, v.TransferID)
		if e != nil || obs.State != "canonical" || obs.BlockHeight != 104 || obs.BlockHash != prior.RevealBlock || obs.TransactionIndex != v.RevealPosition || obs.Confirmations != 61 || obs.RawSHA256 != raw.RawSHA256 {
			t.Fatal("exact saved canonical reveal", obs, e)
		}
		reveals = append(reveals, obs)
		_, e = a.rpc.call(ctx, "getmempoolentry", []any{raw.TxID})
		var refusal *withdrawalRPCError
		if !errors.As(e, &refusal) || refusal.Code != -5 {
			t.Fatal("mined entry still present", e)
		}
	}
	report["reveals_at164"] = reveals
	report["final_height"] = 164
	report["final_hash"] = prior.FinalHead.ChainHash
}

// Administrative fixture transition only. The retained production observer
// continues to use its unchanged five-second bound for every observation RPC.
func ownedObservationActualReconsider(ctx context.Context, n *ownedObservationActualNode, hash string) error {
	if ctx == nil || n == nil || n.rpc == nil || n.config != n.rpc.config || !payoutNonzeroID(hash) {
		return errors.New("invalid fixture reconsideration")
	}
	work, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	// Construction revalidates the exact original literal-loopback endpoint.
	validated, e := newWithdrawalNode(n.config)
	if e != nil {
		return e
	}
	validated.close()
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 60 * time.Second, MaxResponseHeaderBytes: 8192, DisableCompression: true, DisableKeepAlives: true}
	client := &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	id := n.rpc.next.Add(1)
	if id == 0 {
		return errors.New("fixture request identity exhausted")
	}
	raw, e := json.Marshal(struct {
		JSONRPC string   `json:"jsonrpc"`
		ID      uint64   `json:"id"`
		Method  string   `json:"method"`
		Params  []string `json:"params"`
	}{"1.0", id, "reconsiderblock", []string{hash}})
	if e != nil {
		return e
	}
	request, e := http.NewRequestWithContext(work, http.MethodPost, n.config.URL, bytes.NewReader(raw))
	if e != nil {
		return e
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(n.config.Username, n.config.Password)
	response, e := client.Do(request)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "" {
		return errors.New("fixture reconsideration transport")
	}
	body, e := io.ReadAll(io.LimitReader(response.Body, 4097))
	if e != nil || len(body) > 4096 || withdrawalRPCJSON(body) != nil {
		return errors.New("fixture reconsideration response")
	}
	fields, e := withdrawalFields(body, "id")
	if e != nil {
		return e
	}
	for k := range fields {
		if k != "id" && k != "result" && k != "error" && k != "jsonrpc" {
			return errors.New("fixture reconsideration field")
		}
	}
	var actual uint64
	if withdrawalField(fields, "id", &actual) != nil || actual != id || !bytes.Equal(fields["result"], []byte("null")) || !bytes.Equal(fields["error"], []byte("null")) {
		return errors.New("fixture reconsideration rejected")
	}
	if _, ok := fields["jsonrpc"]; ok {
		var version string
		if withdrawalField(fields, "jsonrpc", &version) != nil || version != "1.0" && version != "2.0" {
			return errors.New("fixture reconsideration version")
		}
	}
	return work.Err()
}
