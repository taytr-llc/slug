// External normal-PoW Path A actual. Only published synthetic custody inputs.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

type credentialFile struct {
	Version     uint32 `json:"version"`
	ConfigID    string `json:"config_id"`
	GymID       string `json:"gym_id"`
	Audience    string `json:"audience"`
	Generation  uint32 `json:"generation"`
	ExpiresAtMS int64  `json:"expires_at_ms"`
	Token       string `json:"token"`
}

func runActual(ctx context.Context, binary, root, reference, base string) (out map[string]any) {
	started := time.Now()
	out = map[string]any{"version": 1, "passed": false, "completed": false, "scope": "original schema14 DAILY attachment to real scheduled payout", "public_seed_mining": false}
	var controller *accountpilot.RemotePoolController
	var running *runningOperator
	var relay *rpcRelay
	var nodes []*node
	var clients []*poolservice.Client
	defer func() {
		if p := recover(); p != nil {
			out["failure_phase"] = phase
			out["failure"] = fmt.Sprintf("%.512s", p)
			fmt.Printf("Path A failed in %s: %.512s\n", phase, p)
		}
		clean := []string{}
		if controller != nil {
			if e := controller.Close(); e != nil {
				clean = append(clean, "controller")
			}
		}
		if e := running.stop(); e != nil {
			clean = append(clean, "operator")
		}
		out["runtime_completion"] = running.completionEvidence()
		for _, c := range clients {
			c.Close()
		}
		if relay != nil {
			if e := relay.close(); e != nil {
				clean = append(clean, "relay")
			}
			relay = nil
		}
		reports := []any{}
		for i := len(nodes) - 1; i >= 0; i-- {
			nodes[i].stop()
			reports = append(reports, nodes[i].cleanupReport())
			if !nodes[i].clean() {
				clean = append(clean, "node")
			}
		}
		out["node_cleanup"] = reports
		out["cleanup_errors"] = clean
		out["clean_join"] = len(clean) == 0
		out["seconds"] = time.Since(started).Seconds()
		out["passed"] = out["completed"] == true && len(clean) == 0
		write(filepath.Join(root, "actual-report.json"), out)
	}()
	phase = "independent public fixture selection"
	ref, mnemonic := loadSelection(reference, base)
	private := filepath.Join(root, "private-fixture")
	check(os.Mkdir(private, 0700), "fresh synthetic private fixture")
	s, c, due := actualSelect(ref, private)
	out["due_ms"] = due
	write(filepath.Join(root, "selected-public-setup.json"), s)
	for _, role := range []string{"admission", "publication", "cutoff"} {
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		privateWrite(filepath.Join(private, role+".seed"), seed[:])
		clear(seed[:])
	}
	privateWrite(filepath.Join(private, "unlock.pass"), []byte(privatePass))
	phase = "two original copied normal chains"
	ports := uniquePorts(2)
	a := startNode(ctx, binary, root, "node-primary", ports[0], ports[1], poolsettlefixture.PoolAddress())
	nodes = append(nodes, a)
	b := startNode(ctx, binary, root, "node-peer", ports[1], ports[0], poolsettlefixture.PoolAddress())
	nodes = append(nodes, b)
	ready(ctx, a)
	ready(ctx, b)
	tip := sameTip(ctx, a, b)
	for _, n := range nodes {
		wait(ctx, 30*time.Second, func() bool { var p []any; return n.rpc.call(ctx, "getpeerinfo", nil, &p) == nil && len(p) > 0 })
		peers(ctx, n)
	}
	var height uint32
	a.rpc.must(ctx, "getblockcount", nil, &height)
	require(height == 2 && block(ctx, a, tip).Header.Bits == 0x207fffff, "original baseline tip/real target")
	out["initial_height"] = height
	out["initial_tip"] = tip
	relay = startRPCRelay(ctx, a.rpc.endpoint)
	operatorRPC := prospectnode.RPCConfig{Version: 1, URL: relay.url, Username: "fixture", Password: "public-pool-actual"}
	walletRPC := operatorRPC
	walletRPC.URL = a.rpc.endpoint
	phase = "original daily operator owners"
	running = constructRuntime(ctx, root, private, s, operatorRPC, true, mnemonic)
	mnemonic = ""
	cid, e := pilot.DailyRemotePoolConfigID(c)
	check(e, "actual remote ID")
	in := pilot.RemotePoolInputs{AttachmentRoot: filepath.Join(root, "remote-attachment"), Config: c, NodeRPC: walletRPC, PoolCredentialPath: filepath.Join(private, "pool.credential"), AdmissionCredentialPath: filepath.Join(private, "admission.credential")}
	gym := ref.Fixture.ExpectedLocal.Recipient.Config.Identity.GymID
	require(s.Miners.Enrollments[0].GymID == gym, "selected original gym sorted first")
	for i, en := range s.Miners.Enrollments {
		token := hx(digest([]byte("SLUG Set79 Path A pool " + en.GymID)))
		expires := time.Now().Add(time.Hour).UnixMilli()
		cred, e := running.runtime.RotateCredential(ctx, en.GymID, poolminer.PoolAudience, 0, token, expires)
		check(e, "original pool enrollment")
		pc, e := poolservice.NewDailyClient(clientConfig(s, c, i, token))
		check(e, "real selected daily client")
		clients = append(clients, pc)
		if i == 0 {
			raw, e := json.Marshal(credentialFile{2, cid, gym, poolminer.PoolAudience, cred.Generation, expires, token})
			check(e, "pool credential bytes")
			privateWrite(in.PoolCredentialPath, append(raw, '\n'))
		}
	}
	token := hx(digest([]byte("SLUG Set79 Path A admission " + gym)))
	expires := time.Now().Add(time.Hour).UnixMilli()
	cred, e := running.runtime.RotateCredential(ctx, gym, poolminer.AdmissionAudience, 0, token, expires)
	check(e, "original admission enrollment")
	raw, e := json.Marshal(credentialFile{2, cid, gym, poolminer.AdmissionAudience, cred.Generation, expires, token})
	check(e, "admission credential bytes")
	privateWrite(in.AdmissionCredentialPath, append(raw, '\n'))
	running.run(ctx, s.Ingress.Origin)
	wait(ctx, 60*time.Second, func() bool {
		v := running.status(ctx)
		return v.Capacity.Mode == "enabled" && v.Public.Ready
	})
	write(filepath.Join(root, "01-runtime-and-selection.json"), map[string]any{"setup": s, "remote": c, "due_ms": due, "initial_height": height})
	phase = "actual original wallet login"
	view := openController(ctx, filepath.Join(root, "owned-gym"), ref.Fixture, in, &controller)
	before := snapshot(ctx, controller)
	write(filepath.Join(root, "02-stopped-original-view.json"), before)
	var mempool []string
	a.rpc.must(ctx, "getrawmempool", nil, &mempool)
	require(len(mempool) == 0, "fee-free initial worker rewards")
	phase = "explicit actual worker Start and joined Stop"
	check(controller.Start(ctx, view), "original explicit Start")
	wait(ctx, 240*time.Second, func() bool {
		snap := snapshot(ctx, controller)
		requireNoControllerFailure(snap)
		require(snap.View != nil && snap.View.ViewID == view, "original worker view")
		status := running.status(ctx)
		var h uint32
		a.rpc.must(ctx, "getblockcount", nil, &h)
		require(h >= 2 && h <= 4, "at most two worker blocks")
		return (h > 2 && snap.Remote.Records > 0 && snap.Remote.Pending == 0 && status.Capacity.Submissions > 0) || (status.Capacity.Mode == "draining" && status.Capacity.Submissions == 2)
	})
	check(controller.Stop(ctx, view), "original Stop joins hash/delivery")
	stopped := snapshot(ctx, controller)
	require(stopped.View != nil && stopped.View.ViewID == view && stopped.Remote.HashesPerSecondMilli == 0, "same joined original child")
	requireNoControllerFailure(stopped)
	write(filepath.Join(root, "03-work-stopped.json"), stopped)
	check(controller.Logout(ctx, view), "original Logout")
	require(controller.Start(ctx, view) != nil, "stale original view refused")
	check(controller.Close(), "first controller joined")
	controller = nil
	phase = "closed actual worker receipts"
	ev := auditRows(ctx, in.AttachmentRoot, c, clients[0], a)
	write(filepath.Join(root, "04-closed-rows.json"), ev)
	econ := selectedEconomics(ref, len(ev.Accepted))
	inputs := funding(ctx, a, s, ev.Accepted)
	phase = "same original stopped reopen"
	next := openController(ctx, filepath.Join(root, "owned-gym"), ref.Fixture, in, &controller)
	require(next != view && controller.Start(ctx, view) != nil, "replacement view never adopts original capability")
	reopened := snapshot(ctx, controller)
	require(reopened.Remote.Records == uint64(len(ev.Rows)) && reopened.Remote.Pending == stopped.Remote.Pending && reopened.Remote.Attempts == 0, "same stopped retained outbox")
	write(filepath.Join(root, "05-stopped-reopen.json"), reopened)
	phase = "normal maturity before real selected due"
	var last uint32
	for _, r := range ev.Accepted {
		if r.BlockHeight > last {
			last = r.BlockHeight
		}
	}
	for {
		running.checkRunning()
		a.rpc.must(ctx, "getblockcount", nil, &height)
		if height >= last+100 {
			break
		}
		require(time.Now().UnixMilli() < due, "maturity must precede immutable due")
		generate(ctx, a, b)
	}
	wait(ctx, 30*time.Second, func() bool {
		v := running.status(ctx)
		return v.RoundCode == "" && v.Rounds.Through == uint64(econ.AcceptedRewards) && v.Rounds.Rounds == uint64(econ.AcceptedRewards)
	})
	var mature accountpilot.RemotePoolControllerSnapshot
	wait(ctx, 30*time.Second, func() bool {
		running.checkRunning()
		mature = snapshot(ctx, controller)
		return mature.Remote.WalletState == "ready" && mature.Remote.WalletHeight >= height
	})
	calls, _, relayErr := relay.sends()
	check(relayErr, "transparent relay")
	require(len(calls) == 0 && time.Now().UnixMilli() < due, "complete funding before due without early payment")
	write(filepath.Join(root, "06-mature-before-due.json"), map[string]any{"snapshot": mature, "due_ms": due, "observed_at_ms": time.Now().UnixMilli(), "height": height, "economics": econ})
	phase = "real scheduler signed broadcast"
	var send observedSend
	wait(ctx, 240*time.Second, func() bool {
		running.checkRunning()
		calls, _, e := relay.sends()
		check(e, "observed original node RPC")
		require(len(calls) <= 1, "one signed payment only")
		if len(calls) == 1 {
			send = calls[0]
			return true
		}
		return false
	})
	require(send.AtMS >= due, "real due admitted payment")
	payment := validatePayment(send, s, inputs, econ)
	confirmBlock := generate(ctx, a, b)
	require(minedTx(ctx, a, confirmBlock, send.TxID).TxHash().String() == send.TxID, "actual on-chain payment")
	confirmedAt := time.Now().UnixMilli()
	write(filepath.Join(root, "07-confirmed-payment.json"), map[string]any{"payment": payment, "confirmation_block": confirmBlock, "confirmed_at_ms": confirmedAt})
	phase = "authenticated daily money and attached wallet observation"
	balances, pages := waitMoney(ctx, clients, s, econ, send.TxID, confirmedAt, filepath.Join(root, "money-read-evidence.json"))
	var finalWallet accountpilot.RemotePoolControllerSnapshot
	wait(ctx, 60*time.Second, func() bool {
		running.checkRunning()
		finalWallet = snapshot(ctx, controller)
		return finalWallet.Remote.WalletState == "ready" && finalWallet.Remote.WalletHeight >= height+1 && finalWallet.Remote.SpendableAtoms == mature.Remote.SpendableAtoms+econ.MinerAtoms
	})
	require(finalWallet.View != nil && finalWallet.View.ViewID == next && finalWallet.Remote.Attempts == 0, "stopped original wallet observes exact payment")
	write(filepath.Join(root, "07-payment-and-wallet.json"), map[string]any{"payment": payment, "balances": balances, "pages": pages, "wallet_before": mature, "wallet_after": finalWallet, "confirmation_block": confirmBlock})
	check(controller.Logout(ctx, next), "reopened original Logout")
	check(controller.Close(), "reopened original controller joined")
	controller = nil
	finalRows, finalOutbox := readOutbox(in.AttachmentRoot, c)
	require(reflect.DeepEqual(ev.Rows, finalRows) && ev.Outbox == finalOutbox, "stopped reopen preserves exact rows, metadata, scan/cursor and issuance hold")
	write(filepath.Join(root, "outbox-after.json"), finalOutbox)
	phase = "joined original financial audit"
	check(running.stop(), "operator joined before public audit")
	closed := auditOperator(ctx, root, s)
	verifyFinal(closed, s, ev, econ, send)
	write(filepath.Join(root, "08-final-public-audit.json"), closed)
	calls, attempts, e := relay.sends()
	check(e, "relay final status")
	require(len(calls) == 1 && len(attempts) >= 1 && len(attempts) <= 16, "bounded exact original sends")
	for _, v := range attempts {
		require(v.Raw == send.Raw, "all attempts original signed payment")
	}
	write(filepath.Join(root, "09-complete.json"), map[string]any{"completed": true, "accepted_rewards": len(ev.Accepted), "accepted_fighters": ev.AcceptedFighters, "pending_fighters": ev.PendingFighters, "original_signatures": ev.Authorizations, "stopped_reopen": true, "paid_atoms": econ.MinerAtoms, "fee_atoms": econ.FeeAtoms, "send_count": len(calls), "attempts": attempts, "final_audit_unchanged": closed.ReadOnlyUnchanged})
	a.rpc.must(ctx, "getblockcount", nil, &height)
	require(height <= 105 && sameTip(ctx, a, b) != "", "bounded final canonical chain")
	out["final_height"] = height
	out["accepted_rewards"] = len(ev.Accepted)
	out["miner_atoms"] = econ.MinerAtoms
	out["fee_atoms"] = econ.FeeAtoms
	out["stopped_reopen"] = true
	out["original_wallet_received"] = true
	out["scheduled_payment"] = true
	out["completed"] = true
	return
}
func main() {
	nodeBinary := flag.String("node", "", "normal node binary")
	root := flag.String("artifacts", "", "fresh copied fixture root")
	reference := flag.String("reference", "", "qualified actual reference")
	base := flag.String("base-reference", "", "qualified published base corpus")
	flag.Parse()
	require(flag.NArg() == 0 && !wire.TestPoW, "normal actual profile")
	for _, p := range []string{*nodeBinary, *root, *reference, *base} {
		require(filepath.IsAbs(p) && filepath.Clean(p) == p, "absolute frozen input")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	out := runActual(ctx, *nodeBinary, *root, *reference, *base)
	if out["passed"] != true {
		os.Exit(1)
	}
	fmt.Println("daily original-wallet scheduled payout actual passed")
}
