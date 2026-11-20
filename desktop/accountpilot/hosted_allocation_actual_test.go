//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

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
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/chain"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
)

type hostedAllocationOracle struct {
	hostedActualOracle
	Receive [2]pilot.HostedReceiveAddress
	Change  [2]struct{ Address, Script string }
}

func hostedAllocationID(label string) string {
	sum := sha256.Sum256([]byte("SLUG Set72 actual public operation\x00" + label))
	return hex.EncodeToString(sum[:])
}
func hostedAllocationOracleFor(t *testing.T, fixture string) hostedAllocationOracle {
	t.Helper()
	want := hostedAllocationOracle{hostedActualOracle: hostedActualOracleFor(t, fixture)}
	raw, e := os.ReadFile("../../../slugwallet/wallet/testdata/hosted-allocation/reference.json")
	if e != nil || hex.EncodeToString(sumPoolBackup(raw)) != "221fc42f6848fed27912a43454a1fa9ebe2d4e16b383aad9e294c31316424e6b" {
		t.Fatal("independent allocation input changed")
	}
	var corpus struct {
		Bindings []struct{ Binding wallet.AllocationBinding }
		States   []struct {
			Name     string
			Snapshot struct {
				LocalLinks []waddrmgr.HostedAllocationRecord `json:"local_links"`
			}
		}
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("independent allocation decode")
	}
	found := false
	for _, b := range corpus.Bindings {
		if b.Binding.InitialManifest.GymID == want.Identity.Gym {
			want.Manifest = b.Binding.InitialManifest
			found = true
		}
	}
	if !found {
		t.Fatal("independent initial manifest missing")
	}
	// Index2's independent derivation is reused from a modeled burned row; the
	// real acceptance must consume it. No modeled operation identity is adopted.
	for _, state := range corpus.States {
		if state.Name != fixture+"/burned-receive" {
			continue
		}
		for _, l := range state.Snapshot.LocalLinks {
			if l.Purpose == 84 && l.Branch == 0 && l.SelectedIndex >= 1 && l.SelectedIndex <= 2 {
				want.Receive[l.SelectedIndex-1] = pilot.HostedReceiveAddress{Address: l.Address, Script: l.Script, Purpose: 84, Coin: 0, Account: 0, Branch: 0, Index: l.SelectedIndex}
			}
		}
	}
	raw, e = os.ReadFile("../../../slugwallet/wallet/testdata/phase-a-recovery/addresses-v1.json")
	if e != nil || hex.EncodeToString(sumPoolBackup(raw)) != "1558bf4a134ab01acfe84fdbb2edc3e8af842b602cb5150feaa9ce2df26b9cf7" {
		t.Fatal("independent Phase-A address input changed")
	}
	var addresses struct {
		Cases []struct {
			Name     string
			Branches []struct {
				Purpose, Branch uint32
				Addresses       []struct {
					Index   uint32
					Address string
					Script  string `json:"script_pub_key"`
				}
			}
		}
	}
	if json.Unmarshal(raw, &addresses) != nil {
		t.Fatal("independent address decode")
	}
	for _, f := range addresses.Cases {
		if f.Name != fixture {
			continue
		}
		for _, b := range f.Branches {
			if b.Purpose != 84 || b.Branch != 1 {
				continue
			}
			for _, a := range b.Addresses {
				if a.Index < 2 {
					want.Change[a.Index].Address, want.Change[a.Index].Script = a.Address, a.Script
				}
			}
		}
	}
	for i := range want.Receive {
		if want.Receive[i].Address == "" || want.Receive[i].Script == "" || want.Change[i].Address == "" || want.Change[i].Script == "" {
			t.Fatal("independent next-address input missing")
		}
	}
	for i := range want.Manifest.Accounts {
		if want.Manifest.Accounts[i].Purpose == 84 {
			want.Manifest.Accounts[i].NextExternal = 3
			want.Manifest.Accounts[i].NextInternal = 2
		}
	}
	want.ChangeAddress, want.ChangeScript = want.Change[1].Address, want.Change[1].Script
	return want
}
func hostedAllocationReceive(t *testing.T, ctx context.Context, c *PoolController, s pilot.HostedRecoverySnapshot, index uint32, want pilot.HostedReceiveAddress) pilot.HostedReceiveJob {
	t.Helper()
	label := fmt.Sprintf("%s-receive-%d", s.Account.Fixture, index)
	action, operation := hostedAllocationID(label+"-action"), hostedAllocationID(label+"-operation")
	view := s.Healthy.Pool.View.ViewID
	j, e := c.BeginHostedReceive(ctx, view, action, operation)
	if e != nil {
		t.Fatal("receive admission", e)
	}
	poolActualWait(t, ctx, "exact receive acknowledgment", func(work context.Context) bool {
		var e error
		j, e = c.LookupHostedReceive(work, view, action, operation)
		if e != nil {
			t.Fatal("receive lookup", e)
		}
		if j.State == "running" {
			return false
		}
		if j.State != "ready" || j.Address == nil {
			t.Fatal("receive terminal", j.State, j.Error)
		}
		return true
	})
	got := *j.Address
	revision := got.InventoryRevision
	got.InventoryRevision = 0
	if got != want || revision == 0 || j.ActionID != action || j.OperationID != operation || j.ViewID != view || j.Allocation != s.Healthy.Allocation {
		t.Fatal("receive differs from independent path or original scope")
	}
	return j
}
func hostedAllocationWithdraw(t *testing.T, ctx context.Context, c *PoolController, s pilot.PoolSnapshot, fixture string, index int, inputAtoms int64) pilot.WithdrawalOperation {
	t.Helper()
	label := fmt.Sprintf("%s-withdraw-%d", fixture, index)
	q := pilot.WithdrawalRequest{ID: 1, Version: 1, Command: "withdrawal_prepare", ViewID: s.View.ViewID, OperationID: hostedAllocationID(label), ActionID: hostedAllocationID(label + "-prepare"), Intent: &pilot.WithdrawalIntent{Destination: hostedActualSink, RecipientScript: hostedActualSinkScript, AmountAtoms: hostedActualSendAtoms, FeeRateAtomsPerKB: 1000, FeeLimitAtoms: 100000}}
	prepared := hostedActualAction(t, ctx, c, q)
	d := prepared.Draft
	if prepared.Stage != "prepared" || d == nil || d.InputCount != 1 || d.InputAtoms != inputAtoms || d.FeeAtoms != hostedActualFee || d.ChangeAtoms != inputAtoms-hostedActualSendAtoms-hostedActualFee || d.ChangeOutputIndex != 1 || d.RecipientOutputIndex != 0 {
		t.Fatal("reviewed exact one-input arithmetic/order differs")
	}
	review := pilot.WithdrawalRequest{ID: 3, Version: 1, Command: "withdrawal_review", ViewID: q.ViewID, OperationID: q.OperationID, ActionID: hostedAllocationID(label + "-send"), ReviewID: hostedAllocationID(label + "-review"), ReviewKind: "send", DraftDigest: d.DraftDigest}
	r := c.HandleWithdrawal(ctx, review)
	if !r.OK || r.Review == nil || r.Review.Draft != *d || r.Review.TxID != "" {
		t.Fatal("original exact review", r.Error)
	}
	review.Command, review.ReviewKind = "withdrawal_approve", ""
	result := hostedActualAction(t, ctx, c, review)
	if result.Stage != "submitted" || result.TxID != d.UnsignedTxID || result.Dispatch == nil || result.Dispatch.Attempt != 1 || result.Observation == nil || result.Observation.State != "mempool" {
		t.Fatal("approved exact payment not observed once")
	}
	return result
}

// This fixed read-only RPC is intentionally separate from the older helper's
// smaller allowlist. It can read only one exact public transaction from the
// fresh fixture node and never writes/sends/opens a wallet.
func hostedAllocationRaw(ctx context.Context, node *poolActualRPC, txid string, out *string) error {
	if len(txid) != 64 || out == nil {
		return errors.New("invalid owned transaction selector")
	}
	raw, e := hex.DecodeString(txid)
	if e != nil || hex.EncodeToString(raw) != txid {
		return errors.New("invalid owned transaction selector")
	}
	body, e := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": 1, "method": "getrawtransaction", "params": []any{txid, 0}})
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+node.address, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.SetBasicAuth(node.user, node.password)
	req.Header.Set("Content-Type", "application/json")
	response, e := node.client.Do(req)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	raw, e = io.ReadAll(io.LimitReader(response.Body, 16385))
	if e != nil || len(raw) > 16384 {
		return errors.New("owned transaction response bound")
	}
	var frame struct {
		Result string          `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if response.StatusCode != 200 || json.Unmarshal(raw, &frame) != nil || len(frame.Error) > 0 && string(frame.Error) != "null" || len(frame.Result) > 8192 {
		return errors.New("owned transaction read failed")
	}
	*out = frame.Result
	return nil
}

func hostedAllocationTx(t *testing.T, ctx context.Context, node *poolActualRPC, op pilot.WithdrawalOperation, previous string, vout uint32, changeScript string) {
	t.Helper()
	var encoded string
	if e := hostedAllocationRaw(ctx, node, op.TxID, &encoded); e != nil {
		t.Fatal("exact payment read", e)
	}
	raw, e := hex.DecodeString(encoded)
	var tx wire.MsgTx
	if e != nil || len(raw) > 4096 || tx.Deserialize(bytes.NewReader(raw)) != nil || tx.TxHash().String() != op.TxID {
		t.Fatal("exact signed payment framing")
	}
	hash, e := chainhash.NewHashFromStr(previous)
	if e != nil || len(tx.TxIn) != 1 || tx.TxIn[0].PreviousOutPoint != (wire.OutPoint{Hash: *hash, Index: vout}) || len(tx.TxIn[0].Witness) != 2 || len(tx.TxOut) != 2 {
		t.Fatal("payment input not exact prior owned coin")
	}
	if tx.TxOut[0].Value != hostedActualSendAtoms || hex.EncodeToString(tx.TxOut[0].PkScript) != hostedActualSinkScript || tx.TxOut[1].Value != op.Draft.ChangeAtoms || hex.EncodeToString(tx.TxOut[1].PkScript) != changeScript {
		t.Fatal("signed sink/change differs from independent scripts and integer amounts")
	}
}
func hostedAllocationLogin(t *testing.T, ctx context.Context, c *PoolController, fixture, kind string) pilot.HostedRecoverySnapshot {
	t.Helper()
	if e := c.LoginFixture(fixture); e != nil {
		t.Fatal("hosted login", e)
	}
	var s pilot.HostedRecoverySnapshot
	poolActualWait(t, ctx, "hosted selected owner", func(context.Context) bool {
		s = c.HostedRecoverySnapshot()
		if s.Account.State == "failed" {
			t.Fatal("hosted login failed", s.Error, s.Account.LastError)
		}
		return s.Account.State == "signed_in"
	})
	if e := pilot.ValidateHostedRecoverySnapshot(s); e != nil || s.Kind != kind || s.Account.Fixture != fixture {
		t.Fatal("wrong selected owner", s.Kind, e)
	}
	c.mu.Lock()
	signed := c.session.Kind == "login"
	c.mu.Unlock()
	if !signed {
		t.Fatal("fixture did not use signed login")
	}
	return s
}
func hostedAllocationVerify(t *testing.T, ctx context.Context, c *PoolController, root, path, view, want string) {
	t.Helper()
	selected, e := recoveryfile.Capture(path, []string{root}, recoveryfile.ReadMode)
	if e != nil {
		t.Fatal(e)
	}
	e = c.VerifyHostedBackup(ctx, pilot.HostedMissingOwner, view, selected.Pin())
	e = errors.Join(e, selected.Close())
	if e != nil {
		t.Fatal("missing-wallet verify dispatch", e)
	}
	poolActualWait(t, ctx, "file-only exact inventory verification", func(context.Context) bool {
		s := c.HostedRecoverySnapshot()
		if s.Recovery == nil {
			t.Fatal("recovery owner vanished")
		}
		b := s.Recovery.Backup
		if b.State == "running" {
			return false
		}
		if want == "" {
			if b.State != "succeeded" || b.Phase != "complete" || b.Error != "" {
				t.Fatal("current backup failed", b.State, b.Error)
			}
		} else if b.State != "failed" || b.Error != want {
			t.Fatal("stale backup did not fail exactly", b.State, b.Error)
		}
		return true
	})
}
func hostedAllocationExport(t *testing.T, ctx context.Context, c *PoolController, root, path string, s pilot.HostedRecoverySnapshot, want recovery.Manifest) {
	t.Helper()
	selected, e := recoveryfile.Capture(path, []string{root}, recoveryfile.CreateMode)
	if e != nil {
		t.Fatal(e)
	}
	e = c.ExportHostedRecovery(ctx, pilot.HostedMissingOwner, s.Recovery.View.ViewID, selected.Pin())
	e = errors.Join(e, selected.Close())
	if e != nil {
		t.Fatal("wallet-absent current export", e)
	}
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm() != 0600 || info.Size() > recovery.MaxExportSize {
		t.Fatal("export file permissions/bounds")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("private export read")
	}
	bundle, e := recovery.ParseExport(raw)
	clear(raw)
	if e != nil {
		t.Fatal("export parse")
	}
	bundle.Mnemonic = ""
	if !reflect.DeepEqual(bundle.Manifest, want) {
		t.Fatal("export omitted retained receive/change highwaters")
	}
	if c.HostedRecoverySnapshot().Recovery.Backup.State != "idle" {
		t.Fatal("export asserted verification success")
	}
}
func hostedAllocationStaleFile(t *testing.T, path, fixture string) {
	t.Helper()
	raw, e := os.ReadFile("../../../slugwallet/recovery/testdata/recovery_v1.json")
	if e != nil || hex.EncodeToString(sumPoolBackup(raw)) != "d5141f4a8ee4378d77d483b09f6ecd809f8b328f82e79a69ff83164e73bc799a" {
		t.Fatal("published fixture input changed")
	}
	var corpus struct {
		Cases []struct {
			Name   string
			Export recovery.Export
		}
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("published fixture input")
	}
	clear(raw)
	for _, f := range corpus.Cases {
		if f.Name != fixture {
			continue
		}
		f.Export.Manifest = hostedActualOracleFor(t, fixture).Manifest
		encoded, e := recovery.MarshalExport(f.Export)
		f.Export.Mnemonic = ""
		if e != nil {
			t.Fatal(e)
		}
		defer clear(encoded)
		file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			t.Fatal(e)
		}
		_, e = file.Write(encoded)
		e = errors.Join(e, file.Sync(), file.Close())
		if e != nil {
			t.Fatal(e)
		}
		return
	}
	t.Fatal("fixture missing")
}

// Independent file-only restore receives no installed wallet path or database.
func hostedAllocationRestoreSpend(t *testing.T, ctx context.Context, path, fresh string, node *poolActualRPC, want hostedActualOracle, withdrawal pilot.WithdrawalOperation) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > recovery.MaxExportSize {
		t.Fatal("selected backup unavailable")
	}
	bundle, err := recovery.ParseExport(raw)
	clear(raw)
	if err != nil || bundle.Manifest.RootPublicKey != want.Identity.Root || bundle.Manifest.GymID != want.Identity.Gym || !reflect.DeepEqual(bundle.Manifest, want.Manifest) {
		t.Fatal("independent backup identity")
	}
	defer func() { bundle.Mnemonic = "" }()
	loader := wallet.NewLoader(&chaincfg.MainNetParams, fresh, true, 5*time.Second, 0, wallet.WithWalletSyncRetryInterval(50*time.Millisecond))
	session, err := loader.BeginPhaseARecovery(ctx, []byte(poolRestorePassword), []byte(poolRestorePassword), bundle)
	if err != nil {
		t.Fatal("independent schema12 begin", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error("schema12 owner close", err)
		}
	}()
	source, err := wallet.NewRecoveryRPCSource(wallet.RecoveryRPCConfig{URL: "ws://" + node.address + "/ws", Username: node.user, Password: node.password})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	}()
	progress, err := session.Status(ctx)
	for err == nil && !progress.Ready {
		progress, err = session.Step(ctx, source)
	}
	if err != nil || progress.Phase != "complete" || progress.Target != 165 || progress.Derived != 165 || progress.Issued != 5 {
		t.Fatal("independent bounded restoration", err)
	}
	tip, err := source.BestBlock(ctx)
	if err != nil || tip.Height != progress.ScannedHeight || tip.Hash.String() != progress.ScannedHash {
		t.Fatal("independent restore checkpoint", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	w, err := loader.OpenExistingWallet([]byte(poolRestorePassword), false)
	if err != nil {
		t.Fatal("open completed schema12", err)
	}
	var backend *chain.RPCClient
	defer func() {
		if err := loader.UnloadWallet(); err != nil {
			t.Error("restored wallet unload", err)
		}
		if backend != nil {
			backend.Stop()
			backend.WaitForShutdown()
		}
	}()
	backend, err = chain.NewRPCClient(&chaincfg.MainNetParams, node.address, node.user, node.password, nil, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = backend.Start(ctx); err != nil {
		t.Fatal(err)
	}
	w.SynchronizeRPC(backend)
	poolActualWait(t, ctx, "independent live sync", func(context.Context) bool {
		return w.ChainSynced() && w.SyncedTo().Height == tip.Height && w.SyncedTo().Hash == tip.Hash
	})
	identity, err := w.GymIdentity(ctx)
	if err != nil || identity.GymID != want.Identity.Gym || identity.PublicKey != want.Identity.Key || w.Manager.RecoveryInventoryLimit() != 32768 {
		t.Fatal("restored schema12 identity/profile", err)
	}
	utxos, err := w.ListUnspent(1, 9999999, "")
	if err != nil || len(utxos) != 1 {
		t.Fatal("restored exact change coin missing", err)
	}
	u := utxos[0]
	amount, err := btcutil.NewAmount(u.Amount)
	if err != nil || int64(amount) != withdrawal.Draft.ChangeAtoms || u.Address != want.ChangeAddress || u.ScriptPubKey != want.ChangeScript || u.TxID != withdrawal.TxID || u.Vout != uint32(withdrawal.Draft.ChangeOutputIndex) {
		t.Fatal("restored coin differs from frozen internal1 and actual approved payment")
	}
	history, err := w.ListTransactions(0, 8)
	if err != nil {
		t.Fatal("restored public history")
	}
	found := false
	for _, row := range history {
		if row.TxID == withdrawal.TxID {
			found = true
		}
	}
	if !found {
		t.Fatal("restored wallet lost prior withdrawal history")
	}
	if err = w.Unlock([]byte(poolRestorePassword), nil); err != nil {
		t.Fatal(err)
	}
	sink, _ := hex.DecodeString(hostedActualSinkScript)
	paid := int64(amount) - hostedActualFee
	tx, err := w.SendOutputs([]*wire.TxOut{{Value: paid, PkScript: sink}}, &waddrmgr.KeyScopeBIP0084, 0, 1, 1000, wallet.CoinSelectionLargest, "public Set72 restore proof")
	if err != nil {
		t.Fatal("controlled restored spend", err)
	}
	previous, err := chainhash.NewHashFromStr(withdrawal.TxID)
	if err != nil || len(tx.TxIn) != 1 || tx.TxIn[0].PreviousOutPoint != (wire.OutPoint{Hash: *previous, Index: uint32(withdrawal.Draft.ChangeOutputIndex)}) || len(tx.TxIn[0].Witness) != 2 || len(tx.TxOut) != 1 || tx.TxOut[0].Value != paid || !bytes.Equal(tx.TxOut[0].PkScript, sink) {
		t.Fatal("restored spend did not consume exact recovered coin into exact sink/fee")
	}
	spend := tx.TxHash().String()
	poolActualConfirm(t, ctx, node, spend)
	poolActualWait(t, ctx, "restored sweep confirms", func(context.Context) bool { return w.ChainSynced() && w.SyncedTo().Height == tip.Height+1 })
	remaining, err := w.CalculateBalance(1)
	if err != nil || remaining != 0 {
		t.Fatal("restored exact sweep not reflected", err)
	}
	hostedActualWrite(t, filepath.Join(filepath.Dir(fresh), "restore-public.json"), struct {
		Version                            uint32                  `json:"version"`
		Progress                           wallet.RecoveryProgress `json:"schema12_progress"`
		RestoredAtoms, SentAtoms, FeeAtoms int64
		FundingTxID, SpendTxID             string
	}{1, progress, int64(amount), paid, hostedActualFee, withdrawal.TxID, spend})
	return spend
}

func TestHostedAllocationRecoveryActual(t *testing.T) {
	artifacts, binary := os.Getenv("SLUG_HOSTED_ALLOCATION_ACTUAL_ROOT"), os.Getenv("SLUG_HOSTED_ALLOCATION_ACTUAL_NODE")
	if artifacts == "" || binary == "" {
		t.Skip("explicit fresh fixture artifacts and normal node required")
	}
	if !filepath.IsAbs(artifacts) || !filepath.IsAbs(binary) {
		t.Fatal("absolute owned inputs required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	root := filepath.Join(artifacts, "hosted-allocation")
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("entirely fresh root required")
	}
	cfg := pilot.Config{DataDir: root, NodeBinary: binary}
	c, e := NewHostedRecoveryPool(ctx, cfg, pilot.PoolOptions{})
	if e != nil {
		t.Fatal("fresh guarded hosted setup", e)
	}
	defer func() {
		if c != nil {
			if e := c.Close(); e != nil {
				t.Error("actual owner close", e)
			}
		}
	}()
	init, ready, guard := c.hosted.init, c.hosted.ready, c.hosted.allocation.guard
	if !hostedReadyMatches(init, ready) || c.HostedRecoverySnapshot().Kind != "inactive" || c.HostedRecoverySnapshot().Mode != pilot.HostedRecoveryPoolMode {
		t.Fatal("wrong explicit fresh mode")
	}
	wants := map[string]hostedAllocationOracle{}
	scopes := map[string]pilot.HostedAllocationScope{}
	accounts := map[string]string{}
	receives := map[string][]pilot.HostedReceiveJob{}
	funding := map[string]string{}
	withdrawals := map[string][]pilot.WithdrawalOperation{}
	node := poolActualNode(t, root)
	defer node.client.CloseIdleConnections()
	for _, f := range hostedFixtures {
		wants[f] = hostedAllocationOracleFor(t, f)
		s := hostedAllocationLogin(t, ctx, c, f, "healthy")
		scopes[f] = s.Healthy.Allocation
		accounts[f] = s.Account.ID
		if s.Healthy.Pool.State != "stopped" || s.Healthy.Pool.Attempts != 0 || s.Healthy.Pool.PayoutAddress != wants[f].Identity.Address || s.Healthy.Pool.SourceBindingID != ready.SourceBindingID {
			t.Fatal("fresh installed source/recipient changed")
		}
		for i := uint32(1); i <= 2; i++ {
			receives[f] = append(receives[f], hostedAllocationReceive(t, ctx, c, s, i, wants[f].Receive[i-1]))
		}
		recipient := wants[f].Identity
		recipient.Address, recipient.Script = wants[f].Receive[1].Address, wants[f].Receive[1].Script
		funding[f] = hostedActualFund(t, ctx, node, recipient)
		poolActualLogout(t, ctx, c)
	}
	t.Log("both signed fixture logins issued receive1/2 and real coinbases paid independently expected receive2; adding explicit maturity descendants")
	poolActualMine(t, ctx, node, 100)
	for _, f := range hostedFixtures {
		s := hostedAllocationLogin(t, ctx, c, f, "healthy")
		input, previous, vout := hostedActualFundingAtoms, funding[f], uint32(0)
		poolActualWait(t, ctx, "mature guarded received coin", func(context.Context) bool { return c.Snapshot().SpendableAtoms == input })
		for i := 0; i < 2; i++ {
			op := hostedAllocationWithdraw(t, ctx, c, s.Healthy.Pool, f, i, input)
			hostedAllocationTx(t, ctx, node, op, previous, vout, wants[f].Change[i].Script)
			poolActualConfirm(t, ctx, node, op.TxID)
			poolActualWait(t, ctx, "guarded confirmed change", func(context.Context) bool { return c.Snapshot().SpendableAtoms == op.Draft.ChangeAtoms })
			detail := hostedActualActivity(t, ctx, c, s.Healthy.Pool, f, op.TxID)
			if detail.Item.SpentAtoms != input || detail.Item.ChangeAtoms != op.Draft.ChangeAtoms || detail.Item.FeeAtoms == nil || *detail.Item.FeeAtoms != hostedActualFee || len(detail.Outputs) != 2 || detail.Outputs[1].Address != wants[f].Change[i].Address || !detail.Outputs[1].Owned || !detail.Outputs[1].Change {
				t.Fatal("typed activity differs from exact guarded change")
			}
			withdrawals[f] = append(withdrawals[f], op)
			input, previous, vout = op.Draft.ChangeAtoms, op.TxID, uint32(op.Draft.ChangeOutputIndex)
		}
		poolActualLogout(t, ctx, c)
		t.Logf("%s two exact reviewed/confirmed changes complete; no manual export has occurred", f)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c = nil
	if conn, e := net.DialTimeout("tcp", node.address, time.Second); e == nil {
		conn.Close()
		t.Fatal("owned node survived full close")
	}
	original := map[string]string{}
	before := map[string]map[string]string{}
	for _, f := range hostedFixtures {
		original[f] = filepath.Join(root, "shared", "gyms", accounts[f], "wallet")
		before[f] = hostedActualTree(t, original[f])
	}
	// First exercise exact DB absence with the unaffected fixture still present.
	zeroDB := filepath.Join(original["zero256"], wallet.WalletDBName)
	savedZero := filepath.Join(artifacts, "closed-zero-wallet.db")
	if e = os.Rename(zeroDB, savedZero); e != nil {
		t.Fatal(e)
	}
	c, e = NewHostedRecoveryPool(ctx, cfg, pilot.PoolOptions{})
	if e != nil {
		t.Fatal("missing exact wallet DB restart", e)
	}
	if e = c.LoginFixture("incrementing256"); e != nil {
		t.Fatal("intact selection dispatch", e)
	}
	poolActualWait(t, ctx, "intact selection unavailable", func(context.Context) bool {
		s := c.HostedRecoverySnapshot()
		if s.Account.State == "signed_in" {
			t.Fatal("intact fixture silently became recovery-only")
		}
		return s.Account.State == "failed"
	})
	refused := c.HostedRecoverySnapshot()
	if refused.Kind != "inactive" || refused.Recovery != nil || refused.Healthy != nil || refused.Error != "hosted_recovery_unavailable" {
		t.Fatal("intact fixture absence classification")
	}
	poolActualLogout(t, ctx, c)
	zero := hostedAllocationLogin(t, ctx, c, "zero256", "recovery_only")
	if zero.Recovery.Allocation != scopes["zero256"] || zero.Recovery.LocalWallet != "missing" {
		t.Fatal("missing owner changed retained scope")
	}
	files := map[string]string{}
	manifests := map[string]recovery.Manifest{}
	files["zero256"] = filepath.Join(artifacts, "zero256-current-recovery.json")
	hostedAllocationExport(t, ctx, c, root, files["zero256"], zero, wants["zero256"].Manifest)
	manifests["zero256"] = wants["zero256"].Manifest
	stale := filepath.Join(artifacts, "zero256-independent-stale-1-1.json")
	hostedAllocationStaleFile(t, stale, "zero256")
	hostedAllocationVerify(t, ctx, c, root, stale, zero.Recovery.View.ViewID, "backup_mismatch")
	hostedAllocationVerify(t, ctx, c, root, files["zero256"], zero.Recovery.View.ViewID, "")
	if _, e := os.Lstat(zeroDB); !os.IsNotExist(e) {
		t.Fatal("missing wallet DB recreated")
	}
	c.mu.Lock()
	oldSession := c.session
	c.mu.Unlock()
	if e = c.ResetFixtureAccount(); e != nil {
		t.Fatal("recovery-only reset dispatch", e)
	}
	var held pilot.HostedRecoverySnapshot
	poolActualWait(t, ctx, "held same recovery owner", func(context.Context) bool {
		held = c.HostedRecoverySnapshot()
		if held.Account.State == "failed" {
			t.Fatal("reset failed", held.Account.LastError)
		}
		return held.Kind == "recovery_only" && held.Account.Held
	})
	if held.Recovery.Allocation != zero.Recovery.Allocation || held.Account.ID != zero.Account.ID || held.Account.Generation != zero.Account.Generation+1 || held.Recovery.View.ViewID == zero.Recovery.View.ViewID || held.Account.CanExport {
		t.Fatal("reset replaced recovery binding or did not revoke view")
	}
	if e = c.store.WithSession(ctx, oldSession.Token, gymaccount.View, func(_ gymaccount.Account, check func() error) error { return check() }); !errors.Is(e, gymaccount.ErrUnauthorized) {
		t.Fatal("old signed session survived reset", e)
	}
	heldPath := filepath.Join(artifacts, "held-must-not-exist.json")
	selected, e := recoveryfile.Capture(heldPath, []string{root}, recoveryfile.CreateMode)
	if e != nil {
		t.Fatal(e)
	}
	exportErr := c.ExportHostedRecovery(ctx, pilot.HostedMissingOwner, held.Recovery.View.ViewID, selected.Pin())
	closeErr := selected.Close()
	if exportErr == nil || closeErr != nil {
		t.Fatal("held recovery export allowed or cleanup failed", closeErr)
	}
	if _, e = os.Lstat(heldPath); !os.IsNotExist(e) {
		t.Fatal("held recovery wrote destination")
	}
	selected, e = recoveryfile.Capture(stale, []string{root}, recoveryfile.ReadMode)
	if e != nil {
		t.Fatal(e)
	}
	staleViewErr := c.VerifyHostedBackup(ctx, pilot.HostedMissingOwner, zero.Recovery.View.ViewID, selected.Pin())
	closeErr = selected.Close()
	if staleViewErr == nil || closeErr != nil {
		t.Fatal("old recovery view survived")
	}
	// Verification remains allowed during D72; its exact current manifest still
	// succeeds despite the missing wallet and newly held account.
	hostedAllocationVerify(t, ctx, c, root, files["zero256"], held.Recovery.View.ViewID, "")
	poolActualLogout(t, ctx, c)
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c = nil
	t.Log("exact DB loss, intact selection refusal, inventory-only export, stale-file refusal and held recovery verified")
	// Exercise whole-directory absence independently for the other fixture.
	savedIncrement := filepath.Join(artifacts, "closed-incrementing-wallet")
	if e = os.Rename(original["incrementing256"], savedIncrement); e != nil {
		t.Fatal(e)
	}
	c, e = NewHostedRecoveryPool(ctx, cfg, pilot.PoolOptions{})
	if e != nil {
		t.Fatal("two missing wallets restart", e)
	}
	increment := hostedAllocationLogin(t, ctx, c, "incrementing256", "recovery_only")
	if increment.Recovery.Allocation != scopes["incrementing256"] {
		t.Fatal("other recovery scope changed")
	}
	files["incrementing256"] = filepath.Join(artifacts, "incrementing256-current-recovery.json")
	hostedAllocationExport(t, ctx, c, root, files["incrementing256"], increment, wants["incrementing256"].Manifest)
	manifests["incrementing256"] = wants["incrementing256"].Manifest
	hostedAllocationVerify(t, ctx, c, root, files["incrementing256"], increment.Recovery.View.ViewID, "")
	if _, e = os.Lstat(original["incrementing256"]); !os.IsNotExist(e) {
		t.Fatal("missing wallet directory recreated")
	}
	// A separately owned file-only scanner borrows only the already-audited node
	// source. Original wallet payloads remain unavailable to it throughout.
	_, _, closeSource, e := c.hosted.allocation.archive.BeginSource(ctx)
	if e != nil {
		t.Fatal("owned archive source", e)
	}
	sourceOpen := true
	defer func() {
		if sourceOpen {
			if e := closeSource(); e != nil {
				t.Error(e)
			}
		}
	}()
	restoreNode := poolActualNode(t, root)
	defer restoreNode.client.CloseIdleConnections()
	spend := hostedAllocationRestoreSpend(t, ctx, files["zero256"], filepath.Join(artifacts, "independent-schema12"), restoreNode, wants["zero256"].hostedActualOracle, withdrawals["zero256"][1])
	if e = closeSource(); e != nil {
		t.Fatal(e)
	}
	sourceOpen = false
	poolActualLogout(t, ctx, c)
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c = nil
	if !reflect.DeepEqual(before["incrementing256"], hostedActualTree(t, savedIncrement)) {
		t.Fatal("closed original directory was changed")
	}
	if e = os.Rename(savedZero, zeroDB); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before["zero256"], hostedActualTree(t, original["zero256"])) {
		t.Fatal("closed original DB or residual directory was changed")
	}
	if e = os.Rename(savedIncrement, original["incrementing256"]); e != nil {
		t.Fatal(e)
	}
	t.Log("both production schema12 verifications and independent controlled recovered spend complete; closed originals unchanged")
	c, e = NewHostedRecoveryPool(ctx, cfg, pilot.PoolOptions{})
	if e != nil {
		t.Fatal("exact original guarded wallet reopen", e)
	}
	if !reflect.DeepEqual(init, c.hosted.init) || !reflect.DeepEqual(ready, c.hosted.ready) || !reflect.DeepEqual(guard, c.hosted.allocation.guard) {
		t.Fatal("reopen repinned established authority")
	}
	for _, f := range hostedFixtures {
		s := hostedAllocationLogin(t, ctx, c, f, "healthy")
		if s.Healthy.Allocation != scopes[f] || s.Account.ID != accounts[f] || s.Account.Held != (f == "zero256") {
			t.Fatal("reopened fixture identity/hold changed")
		}
		balance := withdrawals[f][1].Draft.ChangeAtoms
		if f == "zero256" {
			balance = 0
		}
		poolActualWait(t, ctx, "original wallet exact final balance", func(context.Context) bool { return c.Snapshot().SpendableAtoms == balance })
		for _, op := range withdrawals[f] {
			for repeat := 0; repeat < 2; repeat++ {
				r := c.HandleWithdrawal(ctx, pilot.WithdrawalRequest{ID: 10, Version: 1, Command: "withdrawal_operation", ViewID: s.Healthy.Pool.View.ViewID, OperationID: op.OperationID})
				if !r.OK || r.Operation == nil || !reflect.DeepEqual(*r.Operation, op) {
					t.Fatal("submitted exact operation changed across reopen", r.Error)
				}
			}
		}
		op := withdrawals[f][1]
		retry := pilot.WithdrawalRequest{ID: 12, Version: 1, Command: "withdrawal_review", ViewID: s.Healthy.Pool.View.ViewID, OperationID: op.OperationID, ActionID: hostedAllocationID(f + "-submitted-retry"), ReviewID: hostedAllocationID(f + "-submitted-review"), ReviewKind: "retry", DraftDigest: op.Draft.DraftDigest, TxID: op.TxID}
		if r := c.HandleWithdrawal(ctx, retry); r.OK {
			t.Fatal("submitted payment accepted a new retry authorization")
		}
		if f == "zero256" {
			q := pilot.WithdrawalRequest{ID: 13, Version: 1, Command: "withdrawal_prepare", ViewID: s.Healthy.Pool.View.ViewID, OperationID: hostedAllocationID("held-payment"), ActionID: hostedAllocationID("held-payment-action"), Intent: &pilot.WithdrawalIntent{Destination: hostedActualSink, RecipientScript: hostedActualSinkScript, AmountAtoms: hostedActualSendAtoms, FeeRateAtomsPerKB: 1000, FeeLimitAtoms: 100000}}
			if r := c.HandleWithdrawal(ctx, q); r.OK || r.Error != "held" {
				t.Fatal("held installed withdrawal not refused", r.Error)
			}
		}
		r := c.HandleWithdrawal(ctx, pilot.WithdrawalRequest{ID: 14, Version: 1, Command: "withdrawal_operation", ViewID: s.Healthy.Pool.View.ViewID, OperationID: op.OperationID})
		if !r.OK || r.Operation == nil || !reflect.DeepEqual(*r.Operation, op) || c.Snapshot().SpendableAtoms != balance {
			t.Fatal("submitted refusal changed payment/debit")
		}
		if s.Healthy.Pool.Attempts != 0 || s.Healthy.Pool.AssignmentCount != 0 || s.Healthy.Pool.State != "stopped" {
			t.Fatal("recovery work started app mining")
		}
		poolActualLogout(t, ctx, c)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c = nil
	finalNode := poolActualNode(t, root)
	defer finalNode.client.CloseIdleConnections()
	if conn, e := net.DialTimeout("tcp", finalNode.address, time.Second); e == nil {
		conn.Close()
		t.Fatal("final owned node listener remains")
	}
	for _, f := range hostedFixtures {
		hostedActualWrite(t, filepath.Join(artifacts, f+"-public-manifest.json"), manifests[f])
	}
	hostedActualWrite(t, filepath.Join(artifacts, "actual-public-result.json"), struct {
		Version                                                                                                                                                                                         uint32                                 `json:"version"`
		Mode                                                                                                                                                                                            string                                 `json:"mode"`
		Scopes                                                                                                                                                                                          map[string]pilot.HostedAllocationScope `json:"scopes"`
		Receives                                                                                                                                                                                        map[string][]pilot.HostedReceiveJob    `json:"receives"`
		Funding                                                                                                                                                                                         map[string]string                      `json:"funding_txids"`
		Withdrawals                                                                                                                                                                                     map[string][]pilot.WithdrawalOperation `json:"withdrawals"`
		Manifests                                                                                                                                                                                       map[string]recovery.Manifest           `json:"current_manifests"`
		RestoredSpend                                                                                                                                                                                   string                                 `json:"restored_spend_txid"`
		FreshSetup, BothVerified, NoPrelossExport, DBLoss, DirectoryLoss, IntactRefused, OriginalsUnchanged, StaleFileRefused, HeldRecovery, OriginalSessionRevoked, Reopened, SubmittedReplayUnchanged bool
	}{1, pilot.HostedRecoveryPoolMode, scopes, receives, funding, withdrawals, manifests, spend, true, true, true, true, true, true, true, true, true, true, true, true})
}
