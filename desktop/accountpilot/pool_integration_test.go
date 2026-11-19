//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

// This client reads only the disposable config created by NewPool in this test.
// No operator configuration, wallet secrets or external node enters the test.
type poolActualRPC struct {
	address, user, password string
	client                  *http.Client
}

func poolActualNode(t *testing.T, root string) *poolActualRPC {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(root, "shared", "primary.conf"))
	if e != nil {
		t.Fatal(e)
	}
	settings := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok {
			settings[k] = v
		}
	}
	address := settings["rpclisten"]
	host, _, e := net.SplitHostPort(address)
	if e != nil || host != "127.0.0.1" || settings["localwalletpilot"] != "1" || settings["txindex"] != "1" {
		t.Fatal("explicit owned loopback pool node required")
	}
	return &poolActualRPC{address, settings["rpcuser"], settings["rpcpass"], &http.Client{Timeout: 180 * time.Second, Transport: &http.Transport{Proxy: nil}}}
}
func (r *poolActualRPC) call(ctx context.Context, method string, params []any, out any) error {
	switch method {
	case "getblockcount", "getblockhash", "getrawmempool", "getblock":
	default:
		return errors.New("test RPC outside allowlist")
	}
	raw, e := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": 1, "method": method, "params": params})
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+r.address, bytes.NewReader(raw))
	if e != nil {
		return e
	}
	req.SetBasicAuth(r.user, r.password)
	req.Header.Set("Content-Type", "application/json")
	response, e := r.client.Do(req)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	raw, e = io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if e != nil {
		return e
	}
	var frame struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if response.StatusCode != 200 || json.Unmarshal(raw, &frame) != nil || len(frame.Error) > 0 && string(frame.Error) != "null" {
		return errors.New("owned node RPC failed")
	}
	return json.Unmarshal(frame.Result, out)
}
func poolActualWait(t *testing.T, ctx context.Context, label string, fn func(context.Context) bool, diagnostics ...func() any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	for {
		if ctx.Err() == nil && fn(ctx) {
			return
		}
		select {
		case <-ctx.Done():
			for _, diagnostic := range diagnostics {
				t.Logf("%s last observation: %+v", label, diagnostic())
			}
			t.Fatal(label, "deadline", ctx.Err())
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func poolActualBalanceWait(t *testing.T, ctx context.Context, label string, c *PoolController, accept func(pilot.PoolSnapshot) bool) {
	t.Helper()
	var last pilot.PoolSnapshot
	var lastError string
	var elapsedMS int64
	diagnostic := func() any {
		return struct {
			Account, State, BalanceState, BalanceError, RefreshError string
			RefreshMS                                                int64
		}{last.Account.State, last.State, last.Balance.State, last.Balance.Error, lastError, elapsedMS}
	}
	nextLog := time.Now().Add(30 * time.Second)
	poolActualWait(t, ctx, label, func(ctx context.Context) bool {
		started := time.Now()
		err := c.RefreshPoolBalance(ctx)
		elapsedMS = time.Since(started).Milliseconds()
		lastError = ""
		if err != nil {
			lastError = err.Error()
		}
		last = c.Snapshot()
		if accept(last) {
			return true
		}
		if time.Now().After(nextLog) {
			t.Logf("%s pending: %+v", label, diagnostic())
			nextLog = time.Now().Add(30 * time.Second)
		}
		return false
	}, diagnostic)
}
func poolActualLogin(t *testing.T, ctx context.Context, c *PoolController, fixture string) pilot.PoolSnapshot {
	t.Helper()
	if e := c.LoginFixture(fixture); e != nil {
		t.Fatal("login dispatch", e)
	}
	var s pilot.PoolSnapshot
	poolActualWait(t, ctx, "signed in", func(ctx context.Context) bool {
		s = c.Snapshot()
		if s.Account.State == "failed" {
			t.Fatal("login failed", s.Account.LastError)
		}
		return s.Account.State == "signed_in"
	}, func() any {
		return struct{ Account, State, Wallet, Error string }{s.Account.State, s.State, s.WalletState, s.Account.LastError}
	})
	if s.View == nil || s.Account.Fixture != fixture || s.State != "stopped" || s.Attempts != 0 || s.Account.CanExport || s.BackupState != "unavailable" {
		t.Fatal("explicit stopped pool view", s.State, s.Account.State)
	}
	return s
}
func poolActualLogout(t *testing.T, ctx context.Context, c *PoolController) {
	t.Helper()
	if e := c.Logout(); e != nil {
		t.Fatal(e)
	}
	poolActualWait(t, ctx, "signed out", func(ctx context.Context) bool {
		s := c.Snapshot()
		if s.Account.State == "failed" {
			t.Fatal("logout failed")
		}
		return s.Account.State == "signed_out" && s.View == nil
	})
}
func poolActualRecent(t *testing.T, ctx context.Context, c *PoolController) *gymstable.Page {
	t.Helper()
	// The dispatched proof job retains its request context. Keep one bounded
	// lifetime through both dispatch and polling instead of cancelling that
	// job as soon as the dispatch wait succeeds.
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	view := c.Snapshot().View.ViewID
	// Stop pauses the proof lane. Recent resumes just the selected verification,
	// never mining; all result polling remains on the original view.
	var job pilot.StableJob
	var err error
	poolActualWait(t, ctx, "recent dispatch", func(context.Context) bool { job, err = c.BeginStableRecent(ctx, view); return err == nil })
	poolActualWait(t, ctx, "recent page", func(ctx context.Context) bool {
		job, err = c.StableJob(ctx, view, job.JobID)
		if err != nil {
			t.Fatal("recent lookup", err)
		}
		if job.State == "failed" || job.State == "outcome_unknown" || job.State == "cancelled" {
			t.Fatal("recent verification", job.State, job.Error)
		}
		return job.State == "done"
	})
	if job.Page == nil || job.Page.Status.Pin.Version != gymstable.SharedPinVersion2 || job.Page.HasMore || job.Page.NextAfter != job.Page.Status.Checkpoint {
		t.Fatal("shared recent page")
	}
	return job.Page
}

func TestPoolDesktopActualSharedRuntime(t *testing.T) {
	node, artifacts := os.Getenv("SLUG_POOL_NODE"), os.Getenv("SLUG_POOL_DESKTOP_ARTIFACTS")
	if node == "" || artifacts == "" {
		t.Skip("explicit normal node and owned fixture artifact directory required")
	}
	if !filepath.IsAbs(node) || !filepath.IsAbs(artifacts) {
		t.Fatal("absolute test paths required")
	}
	root, e := os.MkdirTemp(artifacts, "pool-desktop-")
	if e != nil {
		t.Fatal(e)
	}
	t.Log("owned public-fixture runtime", root)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	config := pilot.Config{DataDir: root, NodeBinary: node}
	c, e := NewPool(ctx, config)
	if e != nil {
		t.Fatal("new shared runtime", e)
	}
	t.Cleanup(func() {
		if c != nil {
			if e := c.Close(); e != nil {
				t.Error("owned runtime shutdown", e)
			}
		}
	})
	rpc := poolActualNode(t, root)
	var height uint32
	if e = rpc.call(ctx, "getblockcount", nil, &height); e != nil || height != 0 {
		t.Fatal("provisioning must not mine", height, e)
	}
	if c.Snapshot().View != nil || c.Snapshot().Account.State != "signed_out" {
		t.Fatal("setup sessions must not sign the UI in")
	}
	t.Log("both recipient wallets provisioned; shared runtime remains signed out at genesis")
	type observed struct {
		Fixture  string             `json:"fixture"`
		Snapshot pilot.PoolSnapshot `json:"snapshot"`
		Page     *gymstable.Page    `json:"page"`
	}
	observations := []observed{}
	var aliceSequence uint64
	var aliceView, sourceID string
	var accepted uint64
	for _, fixture := range []string{"zero256", "incrementing256"} {
		s := poolActualLogin(t, ctx, c, fixture)
		if sourceID == "" {
			sourceID = s.SourceBindingID
		} else if s.SourceBindingID != sourceID {
			t.Fatal("account switch replaced shared source")
		}
		if fixture == "zero256" {
			aliceView = s.View.ViewID
		} else {
			if _, e = c.ManifestReceipt(ctx, aliceSequence); e == nil {
				t.Fatal("other gym disclosed Alice receipt")
			}
			if _, e = c.BeginStableInventory(ctx, aliceView, 0, 8); e == nil {
				t.Fatal("old view authorized inventory")
			}
		}
		var before uint32
		if e = rpc.call(ctx, "getblockcount", nil, &before); e != nil {
			t.Fatal(e)
		}
		if e = c.Start(); e != nil {
			t.Fatal("explicit start", e)
		}
		poolActualWait(t, ctx, "own accepted fighter and pool block", func(ctx context.Context) bool {
			s = c.Snapshot()
			if s.State == "failed" {
				t.Fatal("worker failed", s.LastError)
			}
			return s.AssignmentCount > 0 && s.Height > uint64(before)
		})
		c.Stop()
		s = c.Snapshot()
		if s.State != "stopped" {
			t.Fatal("stop did not join", s.State)
		}
		page := poolActualRecent(t, ctx, c)
		if len(page.Items) == 0 {
			t.Fatal("accepted fighter missing from own recent inventory")
		}
		for _, row := range page.Items {
			if row.Assignment.GymID != s.GymID {
				t.Fatal("foreign fighter disclosed")
			}
		}
		if fixture == "zero256" {
			aliceSequence = page.Items[0].Assignment.Sequence
		} else if page.Items[0].Assignment.Sequence <= aliceSequence {
			t.Fatal("shared source sequence was renumbered")
		}
		accepted = page.Status.ObservedSourceCount
		observations = append(observations, observed{fixture, s, page})
		t.Logf("%s stopped: height=%d local_assignments=%d shared_cursor=%d", fixture, s.Height, s.AssignmentCount, page.Status.ObservedSourceCount)
		poolActualLogout(t, ctx, c)
		var after uint32
		if e = rpc.call(ctx, "getblockcount", nil, &after); e != nil || after < uint32(s.Height) {
			t.Fatal("logout stopped or replaced parent chain")
		}
	}
	// Both stopped workers remain stopped. Explicit acceptance mining supplies
	// only descendants on this test's private loopback chain, never app bootstrap.
	var tip uint32
	if e = rpc.call(ctx, "getblockcount", nil, &tip); e != nil || tip == 0 || tip > 32 {
		t.Fatal("bounded worker mining", tip, e)
	}
	poolActualMine(t, ctx, rpc, 100)
	t.Log("explicit test descendants completed; waiting for the shared daily scheduler")

	_ = poolActualLogin(t, ctx, c, "zero256")
	confirmationNeeded := false
	poolActualBalanceWait(t, ctx, "mature daily payment prepared or confirmed", c, func(s pilot.PoolSnapshot) bool {
		if s.Balance.Value == nil {
			return false
		}
		b := s.Balance.Value.Balance
		confirmationNeeded = b.ReservedAtoms > 0
		return confirmationNeeded || b.PaidAtoms > 0
	})
	if confirmationNeeded {
		var payoutTxID string
		// A reservation is durable before its transaction is broadcast. Wait
		// for the isolated node to see that transaction before constructing the
		// confirmation template; otherwise the test can mine an empty block
		// and leave the later broadcast pending indefinitely.
		poolActualWait(t, ctx, "daily payment broadcast to the owned node", func(ctx context.Context) bool {
			var pending []string
			if err := rpc.call(ctx, "getrawmempool", nil, &pending); err != nil {
				t.Fatal("owned payout mempool observation", err)
			}
			if len(pending) == 0 {
				return false
			}
			if len(pending) != 1 {
				t.Fatal("owned node must contain only the single daily payout")
			}
			payoutTxID = pending[0]
			return true
		})
		t.Log("daily payment transaction observed by the owned node; waiting for its confirmation template")
		poolActualConfirm(t, ctx, rpc, payoutTxID)
	}
	t.Logf("daily payment observed; confirmation block required=%t", confirmationNeeded)

	paid := map[string]int64{}
	type settlementObservation struct {
		PaidAtoms            int64               `json:"paid_atoms"`
		ConfirmedUnpaidAtoms poolsettle.Fraction `json:"confirmed_unpaid_atoms"`
		CarriedReason        string              `json:"carried_reason"`
	}
	settlements := map[string]settlementObservation{}
	for _, fixture := range []string{"zero256", "incrementing256"} {
		if fixture != "zero256" {
			poolActualLogout(t, ctx, c)
			_ = poolActualLogin(t, ctx, c, fixture)
		}
		// A scheduler tick may publish the day's batch between the two gyms'
		// maturity heights. A later round then correctly carries to the next
		// daily batch. This app test requires real payment plus conserved locked
		// earnings, not simultaneous maturity of independently mined rewards.
		poolActualBalanceWait(t, ctx, "locked earnings and observed wallet settlement for "+fixture, c, func(s pilot.PoolSnapshot) bool {
			if s.Balance.Value == nil {
				return false
			}
			b := s.Balance.Value.Balance
			confirmed, ok := new(big.Rat).SetString(b.Confirmed.Numerator + "/" + b.Confirmed.Denominator)
			if !ok || confirmed.Sign() < 0 {
				t.Fatal("invalid exact confirmed earnings")
			}
			entitled := b.PaidAtoms > 0 || confirmed.Sign() > 0
			settledOrCarried := b.PaidAtoms > 0 || b.Reason == "daily-batch-complete"
			if entitled && settledOrCarried && b.ReservedAtoms == 0 && s.WalletState == "ready" && s.SpendableAtoms >= b.PaidAtoms {
				paid[fixture] = b.PaidAtoms
				settlements[fixture] = settlementObservation{b.PaidAtoms, b.Confirmed, b.Reason}
				return true
			}
			return false
		})
		s := c.Snapshot()
		if s.Attempts != 0 || s.State != "stopped" {
			t.Fatal("login or settlement started mining")
		}
		page := poolActualRecent(t, ctx, c)
		if page.Status.ObservedSourceCount != accepted {
			t.Fatal("test descendants created fighter admissions")
		}
		observations = append(observations, observed{fixture, s, page})
		t.Logf("%s settlement: paid_atoms=%d confirmed_unpaid_atoms=%s/%s reason=%s", fixture, paid[fixture], settlements[fixture].ConfirmedUnpaidAtoms.Numerator, settlements[fixture].ConfirmedUnpaidAtoms.Denominator, settlements[fixture].CarriedReason)
	}
	if paid["zero256"] == 0 && paid["incrementing256"] == 0 {
		t.Fatal("shared scheduler produced no actual wallet payout")
	}
	original := c.Snapshot()
	if e = c.ResetFixtureAccount(); e != nil {
		t.Fatal("reset dispatch", e)
	}
	poolActualWait(t, ctx, "reset retains wallet under hold", func(ctx context.Context) bool {
		s := c.Snapshot()
		if s.Account.State == "failed" {
			t.Fatal("reset failed")
		}
		return s.Account.State == "signed_in" && s.Account.Generation > original.Account.Generation
	})
	reset := c.Snapshot()
	if !reset.Account.Held || reset.Account.CanExport || reset.PayoutAddress != original.PayoutAddress || reset.GymID != original.GymID || reset.SourceBindingID != sourceID || reset.State != "stopped" {
		t.Fatal("reset changed identity or started mining")
	}
	if _, e = c.BeginStableInventory(ctx, original.View.ViewID, 0, 8); e == nil {
		t.Fatal("reset retained old view authority")
	}
	t.Log("verified reset retained the wallet and source, revoked the old view, and applied the security hold")
	if e = c.Close(); e != nil {
		t.Fatal("first close", e)
	}
	c = nil
	if connection, e := net.DialTimeout("tcp", rpc.address, time.Second); e == nil {
		connection.Close()
		t.Fatal("owned primary listener survived quit")
	}
	c, e = NewPool(ctx, config)
	if e != nil {
		t.Fatal("reopen exact retained pool", e)
	}
	t.Log("reopened the retained shared runtime; checking exact payment retry state")
	for _, fixture := range []string{"zero256", "incrementing256"} {
		s := poolActualLogin(t, ctx, c, fixture)
		if s.SourceBindingID != sourceID {
			t.Fatal("restart replaced source")
		}
		poolActualBalanceWait(t, ctx, "same daily payment after restart for "+fixture, c, func(current pilot.PoolSnapshot) bool {
			s = current
			return s.Balance.Value != nil && s.Balance.Value.Balance.PaidAtoms == paid[fixture]
		})
		page := poolActualRecent(t, ctx, c)
		if page.Status.ObservedSourceCount != accepted {
			t.Fatal("restart changed admission history")
		}
		observations = append(observations, observed{fixture, s, page})
		poolActualLogout(t, ctx, c)
	}
	if e = c.Close(); e != nil {
		t.Fatal("final close", e)
	}
	c = nil
	report := struct {
		Version      uint32                           `json:"version"`
		SourceID     string                           `json:"source_binding_id"`
		Paid         map[string]int64                 `json:"paid_atoms"`
		Settlements  map[string]settlementObservation `json:"settlement_observations"`
		Observations []observed                       `json:"observations"`
	}{1, sourceID, paid, settlements, observations}
	raw, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "public-result.json"), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
}

// A mempool observation does not imply inclusion in the node's cached GBT.
// Require the exact observed payout in both the unmined candidate and the
// resulting canonical block before allowing the wallet-payment assertion.
func poolActualConfirm(t *testing.T, parent context.Context, rpc *poolActualRPC, payoutTxID string) {
	t.Helper()
	if raw, err := hex.DecodeString(payoutTxID); err != nil || len(raw) != 32 {
		t.Fatal("invalid observed payout transaction ID")
	}
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	client, err := prospectnode.NewClient(prospectnode.RPCConfig{Version: 1, URL: "http://" + rpc.address, Username: rpc.user, Password: rpc.password})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	addr, err := address.NewAddressWitnessPubKeyHash(bytes.Repeat([]byte{0x45}, 20), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	policy := prospectnode.Policy{Version: 1, Network: uint32(chaincfg.MainNetParams.Net), Genesis: chaincfg.MainNetParams.GenesisHash.String(), DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}
	var template prospectwork.Template
	poolActualWait(t, ctx, "resolved template includes the exact daily payout", func(ctx context.Context) bool {
		var err error
		template, err = client.ResolveTemplate(ctx, policy)
		if err != nil {
			return false
		}
		for _, transaction := range template.Block.Transactions {
			if transaction.TxID == payoutTxID {
				return true
			}
		}
		return false
	})
	job, err := prospectjob.New(template.Context, "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798", strings.Repeat("45", 32), time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	for attempt := uint64(0); attempt < 64; attempt++ {
		candidate, err := prospectwork.Build(job, template, addr.EncodeAddress(), attempt)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := hex.DecodeString(candidate.BlockHex)
		var block wire.MsgBlock
		if err != nil || block.Deserialize(bytes.NewReader(raw)) != nil {
			t.Fatal("payout confirmation candidate decode")
		}
		included := false
		for _, transaction := range block.Transactions[1:] {
			included = included || transaction.TxHash().String() == payoutTxID
		}
		if !included {
			t.Fatal("payout confirmation candidate omitted the observed transaction")
		}
		target := blockchain.CompactToBig(block.Header.Bits)
		for nonce := uint32(0); nonce < 64; nonce++ {
			if ctx.Err() != nil {
				t.Fatal("payout confirmation deadline", ctx.Err())
			}
			block.Header.Nonce = nonce
			hash := block.Header.BlockHash()
			if blockchain.HashToBig(&hash).Cmp(target) > 0 {
				continue
			}
			var encoded bytes.Buffer
			if err := block.Serialize(&encoded); err != nil {
				t.Fatal(err)
			}
			if err := client.SubmitBlock(ctx, template.Context, hex.EncodeToString(encoded.Bytes())); err != nil {
				t.Fatal("submit payout confirmation", err)
			}
			var canonical string
			if err := rpc.call(ctx, "getblockhash", []any{template.Context.ParentHeight + 1}, &canonical); err != nil || canonical != hash.String() {
				t.Fatal("payout confirmation block did not become canonical", err)
			}
			var observed struct {
				Hash string   `json:"hash"`
				Tx   []string `json:"tx"`
			}
			if err := rpc.call(ctx, "getblock", []any{canonical, 1}, &observed); err != nil || observed.Hash != canonical {
				t.Fatal("observe submitted payout confirmation block", err)
			}
			for _, txID := range observed.Tx {
				if txID == payoutTxID {
					t.Logf("exact payout transaction %s confirmed in canonical block %s", payoutTxID, canonical)
					return
				}
			}
			t.Fatal("canonical confirmation block omitted the observed payout")
		}
	}
	t.Fatal("payout confirmation work retry limit")
}

// Explicit test-only descendant work uses ordinary unregistered templates and
// an unrelated published sink. It never adds pool shares, fighter admissions,
// wallet funds or a fallback mining address to the app configuration.
func poolActualMine(t *testing.T, ctx context.Context, rpc *poolActualRPC, count int) {
	t.Helper()
	if count < 1 || count > 100 {
		t.Fatal("bounded descendant count")
	}
	client, e := prospectnode.NewClient(prospectnode.RPCConfig{Version: 1, URL: "http://" + rpc.address, Username: rpc.user, Password: rpc.password})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	addr, e := address.NewAddressWitnessPubKeyHash(bytes.Repeat([]byte{0x45}, 20), &chaincfg.MainNetParams)
	if e != nil {
		t.Fatal(e)
	}
	policy := prospectnode.Policy{Version: 1, Network: uint32(chaincfg.MainNetParams.Net), Genesis: chaincfg.MainNetParams.GenesisHash.String(), DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}
	for n := 0; n < count; n++ {
		won := false
		for attempt := 0; attempt < 64; attempt++ {
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			template, e := client.ResolveTemplate(ctx, policy)
			if e != nil {
				time.Sleep(30 * time.Millisecond)
				continue
			}
			job, e := prospectjob.New(template.Context, "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798", strings.Repeat("45", 32), time.Now().UnixMilli())
			if e != nil {
				t.Fatal(e)
			}
			candidate, e := prospectwork.Build(job, template, addr.EncodeAddress(), uint64(attempt))
			if e != nil {
				t.Fatal(e)
			}
			raw, e := hex.DecodeString(candidate.BlockHex)
			var block wire.MsgBlock
			if e != nil || block.Deserialize(bytes.NewReader(raw)) != nil {
				t.Fatal("explicit template decode")
			}
			target := blockchain.CompactToBig(block.Header.Bits)
			var hash string
			for nonce := uint32(0); nonce < 64; nonce++ {
				if ctx.Err() != nil {
					t.Fatal(ctx.Err())
				}
				block.Header.Nonce = nonce
				h := block.Header.BlockHash()
				if blockchain.HashToBig(&h).Cmp(target) <= 0 {
					hash = h.String()
					break
				}
			}
			if hash == "" {
				continue
			}
			var encoded bytes.Buffer
			if e = block.Serialize(&encoded); e != nil {
				t.Fatal(e)
			}
			if e = client.SubmitBlock(ctx, template.Context, hex.EncodeToString(encoded.Bytes())); e != nil {
				continue
			}
			var canonical string
			if e = rpc.call(ctx, "getblockhash", []any{template.Context.ParentHeight + 1}, &canonical); e == nil && canonical == hash {
				won = true
				break
			}
		}
		if !won {
			t.Fatal("explicit descendant work retry limit")
		}
	}
}
