//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slugwallet/chain"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/rpc/legacyrpc"
	"github.com/taytr-llc/slugwallet/wallet"
	_ "github.com/taytr-llc/slugwallet/walletdb/bdb"
)

// This password protects published test material only. The restored fixture
// intentionally chooses its own password rather than using the active pilot's.
const integrationFixturePassword = "PUBLIC-DESKTOP-RESTORE-FIXTURE-ONLY"

type integrationAddress struct {
	Path         string `json:"path"`
	Address      string `json:"address"`
	ScriptPubKey string `json:"script_pub_key"`
}

type integrationFixture struct {
	Name      string               `json:"name"`
	Export    recovery.Export      `json:"export"`
	Addresses []integrationAddress `json:"addresses"`
}

func integrationOracle(t *testing.T) integrationFixture {
	t.Helper()
	raw, err := os.ReadFile("../../../slugwallet/recovery/testdata/recovery_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		FixtureOnly bool                 `json:"fixture_only"`
		Cases       []integrationFixture `json:"cases"`
	}
	if json.Unmarshal(raw, &fixtures) != nil || !fixtures.FixtureOnly {
		t.Fatal("independent recovery fixture unavailable")
	}
	for _, fixture := range fixtures.Cases {
		if fixture.Name == "zero256" {
			raw, err := os.ReadFile("../../../slugwallet/integration/testdata/recovery_highwater_v1.json")
			var highWater struct {
				FixtureOnly bool                 `json:"fixture_only"`
				Entropy     string               `json:"entropy"`
				Addresses   []integrationAddress `json:"addresses"`
			}
			if err != nil || json.Unmarshal(raw, &highWater) != nil || !highWater.FixtureOnly || highWater.Entropy != strings.Repeat("0", 64) {
				t.Fatal("independent zero256 high-water fixture unavailable")
			}
			fixture.Addresses = append(fixture.Addresses, highWater.Addresses...)
			return fixture
		}
	}
	t.Fatal("published zero256 independent fixture missing")
	return integrationFixture{}
}

func (fixture integrationFixture) address(t *testing.T, path string) (string, string) {
	t.Helper()
	for _, address := range fixture.Addresses {
		if address.Path == path {
			return address.Address, address.ScriptPubKey
		}
	}
	t.Fatal("independent address missing:", path)
	return "", ""
}

type integrationRPC struct {
	url    string
	client *http.Client
}

func newIntegrationRPC(url string) integrationRPC {
	return integrationRPC{url: url, client: &http.Client{
		Transport: &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true},
		Timeout:   2 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (rpc integrationRPC) call(method string, params []any, result any) error {
	raw, err := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": 1, "method": method, "params": params})
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, rpc.url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.SetBasicAuth("fixture", integrationFixturePassword)
	request.Header.Set("Content-Type", "application/json")
	response, err := rpc.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(raw) > 8<<20 {
		return fmt.Errorf("fixture RPC response unavailable")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fixture RPC HTTP %d", response.StatusCode)
	}
	var envelope struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.ID != 1 {
		return fmt.Errorf("invalid fixture RPC envelope")
	}
	if envelope.Error != nil {
		return fmt.Errorf("fixture RPC %s (%d): %s", method, envelope.Error.Code, envelope.Error.Message)
	}
	if result != nil {
		return json.Unmarshal(envelope.Result, result)
	}
	return nil
}

func (rpc integrationRPC) must(t *testing.T, method string, params []any, result any) {
	t.Helper()
	if err := rpc.call(method, params, result); err != nil {
		t.Fatal(err)
	}
}

type integrationNode struct {
	rpc  integrationRPC
	cmd  *exec.Cmd
	done chan error
	once sync.Once
	peer *integrationNode
}

// Reopen only the isolated chain created by this test's closed pilot. The
// original wallet database is not an input to the restored wallet or node.
func integrationStartRetainedNode(t *testing.T, binary, chainDirectory, payout string) *integrationNode {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	peerAddress := listener.Addr().String()
	listener.Close()
	peerDirectory := filepath.Join(filepath.Dir(chainDirectory), "peer-chain")
	peer := integrationStartNode(t, binary, peerDirectory, payout, peerAddress, "127.0.0.1:0")
	node := integrationStartNode(t, binary, chainDirectory, payout, "", peerAddress)
	node.peer = peer
	deadline := time.Now().Add(2 * time.Minute)
	for {
		var primaryHeight, peerHeight int32
		var peers []struct{ Addr string }
		if node.rpc.call("getblockcount", nil, &primaryHeight) == nil && peer.rpc.call("getblockcount", nil, &peerHeight) == nil &&
			primaryHeight == peerHeight && node.rpc.call("getpeerinfo", nil, &peers) == nil && len(peers) == 1 && peers[0].Addr == peerAddress {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("retained fixture node pair failed to synchronize")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return node
}

func integrationStartNode(t *testing.T, binary, chainDirectory, payout, listen, connect string) *integrationNode {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	directory := t.TempDir()
	configPath := filepath.Join(directory, "fixture.conf")
	config := fmt.Sprintf("rpcuser=fixture\nrpcpass=%s\nnotls=1\nrpclisten=127.0.0.1:%d\nnodnsseed=1\nconnect=%s\ndatadir=%s\nlogdir=%s\nminingaddr=%s\n", integrationFixturePassword, port, connect, chainDirectory, directory, payout)
	if listen == "" {
		config += "nolisten=1\n"
	} else {
		config += "listen=" + listen + "\n"
	}
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(directory, "node.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "--configfile="+configPath)
	command.Stdout, command.Stderr = log, log
	if err := command.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	node := &integrationNode{rpc: newIntegrationRPC(fmt.Sprintf("http://127.0.0.1:%d", port)), cmd: command, done: make(chan error, 1)}
	go func() { node.done <- command.Wait(); log.Close() }()
	t.Cleanup(func() { node.close(t) })
	deadline := time.Now().Add(20 * time.Second)
	for {
		var magic uint32
		if node.rpc.call("getcurrentnet", nil, &magic) == nil {
			if magic != recovery.Network {
				t.Fatal("restoration node has wrong network")
			}
			break
		}
		if time.Now().After(deadline) {
			raw, _ := os.ReadFile(logPath)
			t.Fatalf("fixture restoration node startup: %s", raw)
		}
		time.Sleep(50 * time.Millisecond)
	}
	var genesis string
	node.rpc.must(t, "getblockhash", []any{0}, &genesis)
	if genesis != recovery.Genesis {
		t.Fatal("restoration node has wrong genesis")
	}
	return node
}

func (node *integrationNode) close(t *testing.T) {
	t.Helper()
	node.once.Do(func() {
		_ = node.rpc.call("stop", nil, nil)
		select {
		case err := <-node.done:
			if err != nil {
				t.Error("restoration node shutdown:", err)
			}
		case <-time.After(20 * time.Second):
			_ = node.cmd.Process.Kill()
			<-node.done
			t.Error("restoration node needed forced shutdown")
		}
		node.rpc.client.CloseIdleConnections()
		if node.peer != nil {
			node.peer.close(t)
		}
	})
}

type integrationRestoredWallet struct {
	wallet *wallet.Wallet
	loader *wallet.Loader
	server *legacyrpc.Server
	rpc    integrationRPC
	once   sync.Once
}

func integrationRestoreFile(t *testing.T, path string, node *integrationNode) *integrationRestoredWallet {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > recovery.MaxExportSize {
		t.Fatal("exported fixture file unavailable")
	}
	bundle, err := recovery.ParseExport(raw)
	if err != nil {
		t.Fatal("strict retained file parsing:", err)
	}
	loader := wallet.NewLoader(&chaincfg.MainNetParams, filepath.Join(t.TempDir(), "restored-only-from-file"), false, 10*time.Second, 20,
		wallet.WithWalletSyncRetryInterval(50*time.Millisecond))
	w, err := loader.RestoreRecoveryWallet([]byte(integrationFixturePassword), []byte(integrationFixturePassword), bundle)
	if err != nil {
		t.Fatal("fresh file-only wallet restore:", err)
	}
	session := &integrationRestoredWallet{wallet: w, loader: loader}
	t.Cleanup(func() { session.close(t) })
	backend, err := chain.NewRPCClient(&chaincfg.MainNetParams, strings.TrimPrefix(node.rpc.url, "http://"), "fixture", integrationFixturePassword, nil, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := backend.Start(ctx); err != nil {
		backend.Stop()
		backend.WaitForShutdown()
		t.Fatal("restored wallet connection:", err)
	}
	w.SynchronizeRPC(backend)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		backend.Stop()
		backend.WaitForShutdown()
		t.Fatal(err)
	}
	// This independently owned test server exercises a restored RPC spend.
	// The desktop's restricted production listener never gains send methods.
	session.server = legacyrpc.NewServer(&legacyrpc.Options{Username: "fixture", Password: integrationFixturePassword, MaxPOSTClients: 4, MaxWebsocketClients: 0}, loader, []net.Listener{listener})
	session.server.RegisterWallet(w)
	session.server.SetChainServer(backend)
	session.rpc = newIntegrationRPC("http://" + listener.Addr().String())
	return session
}

func (session *integrationRestoredWallet) close(t *testing.T) {
	t.Helper()
	session.once.Do(func() {
		if session.server != nil {
			session.server.Stop()
		}
		if err := session.loader.UnloadWallet(); err != nil {
			t.Error("restored wallet close:", err)
		}
		if session.rpc.client != nil {
			session.rpc.client.CloseIdleConnections()
		}
	})
}

func integrationWaitSync(t *testing.T, session *integrationRestoredWallet, height int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if session.wallet.ChainSynced() && session.wallet.Manager.SyncedTo().Height == height {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("restored wallet failed to synchronize to retained tip")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

type integrationUTXO struct {
	TxID          string  `json:"txid"`
	Vout          uint32  `json:"vout"`
	Address       string  `json:"address"`
	Script        string  `json:"scriptPubKey"`
	Amount        float64 `json:"amount"`
	Confirmations int64   `json:"confirmations"`
}

func integrationNormalizeUTXOs(t *testing.T, credits []integrationUTXO) []string {
	t.Helper()
	result := make([]string, 0, len(credits))
	for _, credit := range credits {
		amount, err := btcutil.NewAmount(credit.Amount)
		if err != nil {
			t.Fatal("invalid fixture amount:", err)
		}
		result = append(result, fmt.Sprintf("%s:%d:%s:%s:%d:%d", credit.TxID, credit.Vout, credit.Address, credit.Script, amount, credit.Confirmations))
	}
	sort.Strings(result)
	return result
}
