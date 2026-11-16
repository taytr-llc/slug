package pilot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/taytr-llc/slug/prospectnode"
)

type nodeRPC struct {
	url, user, password string
	client              *http.Client
}

type pilotNodes struct{ primary, peer *childNode }

var errPilotNodeClose = errors.New("pilot node cleanup failed")

func (n *pilotNodes) rpcConfig() prospectnode.RPCConfig {
	return prospectnode.RPCConfig{Version: 1, URL: n.primary.rpc.url, Username: n.primary.rpc.user, Password: n.primary.rpc.password}
}
func (n *pilotNodes) Close() error {
	return errors.Join(n.primary.stop(), n.peer.stop())
}
func (c *Controller) openNodes(ctx context.Context) (*pilotNodes, error) {
	ports := make([]uint16, 4)
	seen := map[uint16]bool{c.manifest.AuthorityPort: true}
	for i := range ports {
		for {
			port, err := unusedPort()
			if err != nil {
				return nil, errors.New("loopback unavailable")
			}
			if !seen[port] {
				ports[i], seen[port] = port, true
				break
			}
		}
	}
	peer, err := startNode(c.config.NodeBinary, c.config.DataDir, "peer", ports[0], ports[1], ports[3], true)
	if err != nil {
		return nil, err
	}
	primary, err := startNode(c.config.NodeBinary, c.config.DataDir, "primary", ports[2], 0, ports[1], false)
	if err != nil {
		if closeErr := peer.stop(); closeErr != nil {
			c.noteShutdown("pilot node startup cleanup failed")
			return nil, errors.Join(err, errPilotNodeClose)
		}
		return nil, err
	}
	nodes := &pilotNodes{primary: primary, peer: peer}
	cleanup := func(err error) error {
		if nodes.Close() != nil {
			c.noteShutdown("pilot node startup cleanup failed")
			return errors.Join(err, errPilotNodeClose)
		}
		return err
	}
	if err := waitNode(ctx, primary, true); err != nil {
		return nil, cleanup(err)
	}
	if err := waitNode(ctx, peer, false); err != nil {
		return nil, cleanup(err)
	}
	return nodes, nil
}

func (r *nodeRPC) call(ctx context.Context, method string, result any) error {
	return r.callParams(ctx, method, []any{}, result)
}
func (r *nodeRPC) callParams(ctx context.Context, method string, params []any, result any) error {
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": 1, "method": method, "params": params})
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(raw))
	if e != nil {
		return errors.New("node unavailable")
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(r.user, r.password)
	resp, e := r.client.Do(req)
	if e != nil {
		return errors.New("node unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("node unavailable")
	}
	body, e := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if e != nil || len(body) > 1<<20 {
		return errors.New("node unavailable")
	}
	var reply struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &reply) != nil || reply.ID != 1 || string(reply.Error) != "null" || len(reply.Result) == 0 {
		return errors.New("node unavailable")
	}
	if result != nil && json.Unmarshal(reply.Result, result) != nil {
		return errors.New("node unavailable")
	}
	return nil
}
func newRPC(port uint16, user, password string) *nodeRPC {
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, ResponseHeaderTimeout: 3 * time.Second, MaxResponseHeaderBytes: 8192}
	return &nodeRPC{url: "http://127.0.0.1:" + strconv.Itoa(int(port)), user: user, password: password, client: &http.Client{Transport: tr, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type childNode struct {
	cmd     *exec.Cmd
	done    chan struct{}
	log     *os.File
	rpc     *nodeRPC
	waitErr error // Published by closing done after cmd.Wait returns.
}

func startNode(binary, root, name string, rpcPort, peerPort, connectPort uint16, listen bool) (*childNode, error) {
	secret := make([]byte, 32)
	if _, e := rand.Read(secret); e != nil {
		return nil, errors.New("node startup failed")
	}
	password := hex.EncodeToString(secret)
	data, logs := filepath.Join(root, name+"-chain"), filepath.Join(root, name+"-logs")
	for _, p := range []string{data, logs} {
		if plainChild(p, true) != nil {
			return nil, errors.New("invalid pilot data")
		}
	}
	for _, p := range []string{filepath.Join(data, "mainnet"), filepath.Join(data, "mainnet", "blocks_ffldb"), filepath.Join(data, "mainnet", "blocks_ffldb", "metadata")} {
		if plainChild(p, true) != nil {
			return nil, errors.New("invalid pilot chain path")
		}
	}
	// The worker constructs its coinbase using the persisted wallet payout.
	// GBT coinbasevalue mode needs no temporary or fallback mining address.
	config := fmt.Sprintf("rpcuser=local-pilot\nrpcpass=%s\nnotls=1\nrpclisten=127.0.0.1:%d\ndatadir=%s\nlogdir=%s\nnodnsseed=1\nlocalwalletpilot=1\nconnect=127.0.0.1:%d\n", password, rpcPort, data, logs, connectPort)
	if listen {
		config += fmt.Sprintf("listen=127.0.0.1:%d\n", peerPort)
	} else {
		config += "nolisten=1\n"
	}
	configPath := filepath.Join(root, name+".conf")
	if e := atomicBytes(configPath, []byte(config)); e != nil {
		return nil, errors.New("node configuration failed")
	}
	if plainChild(filepath.Join(root, name+"-process.log"), false) != nil {
		return nil, errors.New("invalid pilot log path")
	}
	log, e := os.OpenFile(filepath.Join(root, name+"-process.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if e != nil {
		return nil, errors.New("node startup failed")
	}
	cmd := exec.Command(binary, "--configfile="+configPath)
	cmd.Stdin = nil // Never inherit the supervisor control pipe.
	cmd.Stdout = log
	cmd.Stderr = log
	if e := cmd.Start(); e != nil {
		log.Close()
		return nil, errors.New("node startup failed")
	}
	child := &childNode{cmd: cmd, done: make(chan struct{}), log: log, rpc: newRPC(rpcPort, "local-pilot", password)}
	go func() { child.waitErr = cmd.Wait(); close(child.done) }()
	return child, nil
}
func (child *childNode) stop() (resultErr error) {
	defer func() {
		if child.log.Close() != nil {
			resultErr = errors.Join(resultErr, errors.New("node log close failed"))
		}
		if child.waitErr != nil {
			resultErr = errors.Join(resultErr, errors.New("node exited unsuccessfully"))
		}
	}()
	defer child.rpc.client.CloseIdleConnections()
	select {
	case <-child.done:
		return nil
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	_ = child.rpc.call(ctx, "stop", nil)
	cancel()
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	retry := time.NewTicker(250 * time.Millisecond)
	defer retry.Stop()
	for {
		select {
		case <-child.done:
			return nil
		case <-retry.C:
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = child.rpc.call(ctx, "stop", nil)
			cancel()
		case <-timer.C:
			_ = child.cmd.Process.Kill()
			<-child.done
			return errors.New("node required forced shutdown")
		}
	}
}
func waitNode(ctx context.Context, node *childNode, peers bool) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var info struct {
			Chain       string `json:"chain"`
			Blocks      int64  `json:"blocks"`
			Deployments map[string]struct {
				Status             string `json:"status"`
				AlwaysActiveHeight uint32 `json:"always_active_height"`
			} `json:"bip9_softforks"`
		}
		if node.rpc.call(ctx, "getblockchaininfo", &info) == nil && info.Chain == "mainnet" {
			var network uint32
			var first string
			if node.rpc.call(ctx, "getcurrentnet", &network) != nil || node.rpc.callParams(ctx, "getblockhash", []any{0}, &first) != nil || network != policy().Network || first != genesis || info.Blocks < 0 || info.Blocks > int64(^uint32(0))-1 {
				return errors.New("pilot node identity mismatch")
			}
			for _, name := range []string{"csv", "segwit", "taproot"} {
				deployment, ok := info.Deployments[name]
				if !ok || deployment.AlwaysActiveHeight != 62 || (deployment.Status == "active") != (info.Blocks+1 >= 62) {
					return errors.New("pilot node activation rules mismatch")
				}
				switch deployment.Status {
				case "defined", "started", "locked_in", "active", "failed":
				default:
					return errors.New("pilot node activation state invalid")
				}
			}
			if !peers {
				return nil
			}
			var current []struct {
				Addr string `json:"addr"`
			}
			if node.rpc.call(ctx, "getpeerinfo", &current) == nil && len(current) > 0 {
				for _, p := range current {
					if !strings.HasPrefix(p.Addr, "127.0.0.1:") {
						return errors.New("nonlocal pilot peer")
					}
				}
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("node startup timed out")
		case <-node.done:
			return errors.New("node exited")
		case <-ticker.C:
		}
	}
}
