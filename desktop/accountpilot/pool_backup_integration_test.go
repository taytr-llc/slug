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
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/chain"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/rpc/legacyrpc"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
)

// This independent restorer receives only an exported file, a fresh directory,
// public expectations and the already-owned node. It never receives the active
// wallet path or invokes the desktop fixture wallet constructors.
const poolRestorePassword = "PUBLIC-SHARED-POOL-RESTORE-TEST-ONLY"

type poolBackupPublic struct {
	Fixture              string `json:"fixture"`
	GymID                string `json:"gym_id"`
	Address              string `json:"address"`
	Verified             bool   `json:"verified"`
	OriginalInaccessible bool   `json:"original_database_inaccessible"`
	RestoredAtoms        int64  `json:"restored_atoms"`
	PaymentTransaction   string `json:"pool_payment_transaction"`
	SpendTransaction     string `json:"restored_spend_transaction"`
}

type poolRecoveryOracle struct {
	Root, Gym, Key, Address, Script string
}

func poolBackupOracle(t *testing.T, fixture string) poolRecoveryOracle {
	t.Helper()
	raw, err := os.ReadFile("../../../slugwallet/recovery/testdata/recovery_v1.json")
	if err != nil || hex.EncodeToString(sumPoolBackup(raw)) != "d5141f4a8ee4378d77d483b09f6ecd809f8b328f82e79a69ff83164e73bc799a" {
		t.Fatal("independent published recovery corpus changed")
	}
	defer clear(raw)
	var corpus struct {
		Cases []struct {
			Name   string `json:"name"`
			Export struct {
				Manifest struct {
					Root string `json:"root_public_key"`
					Gym  string `json:"gym_id"`
					Key  string `json:"gym_public_key"`
				} `json:"manifest"`
			} `json:"export"`
			Addresses []struct {
				Path, Address string
				Script        string `json:"script_pub_key"`
			} `json:"addresses"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("public recovery oracle unavailable")
	}
	for _, c := range corpus.Cases {
		if c.Name == fixture {
			for _, a := range c.Addresses {
				if a.Path == "m/84'/0'/0'/0/0" {
					return poolRecoveryOracle{c.Export.Manifest.Root, c.Export.Manifest.Gym, c.Export.Manifest.Key, a.Address, a.Script}
				}
			}
		}
	}
	t.Fatal("public recovery oracle missing fixture")
	return poolRecoveryOracle{}
}
func sumPoolBackup(raw []byte) []byte { sum := sha256.Sum256(raw); return sum[:] }

func poolActualBackup(t *testing.T, ctx context.Context, c *PoolController, root, artifacts string, node *poolActualRPC, paid map[string]int64) poolBackupPublic {
	t.Helper()
	originalFixture := c.Snapshot().Account.Fixture
	fixture := originalFixture
	if paid[fixture] == 0 {
		fixture = "zero256"
	}
	if paid[fixture] == 0 {
		fixture = "incrementing256"
	}
	if paid[fixture] <= 0 {
		t.Fatal("actual funded recovery requires a paid wallet")
	}
	poolActualLogout(t, ctx, c)
	before := poolActualLogin(t, ctx, c, fixture)
	want := poolBackupOracle(t, fixture)
	if before.GymID != want.Gym || before.PayoutAddress != want.Address || !before.Account.CanExport {
		t.Fatal("funded original recovery identity or capability")
	}
	path := filepath.Join(artifacts, "shared-wallet-recovery.json")
	selection, err := recoveryfile.Capture(path, []string{root}, recoveryfile.CreateMode)
	if err != nil {
		t.Fatal("select export", err)
	}
	err = c.ExportPoolRecovery(ctx, before.View.ViewID, selection.Pin())
	closeErr := selection.Close()
	if err != nil || closeErr != nil {
		t.Fatal("actual retained wallet export failed", err, closeErr)
	}
	if stat, err := os.Stat(path); err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("export must be owner-only")
	}
	if c.Snapshot().BackupState != "idle" {
		t.Fatal("export must not claim verified restoration")
	}
	read, err := recoveryfile.Capture(path, []string{root}, recoveryfile.ReadMode)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	if err = c.VerifyPoolBackup(ctx, before.View.ViewID, read.Pin()); err != nil {
		read.Close()
		t.Fatal("start actual backup verification", err)
	}
	poolActualWait(t, ctx, "shared-wallet fresh backup verification", func(context.Context) bool {
		s := c.Snapshot()
		if s.BackupState == "failed" || s.BackupState == "cancelled" {
			t.Fatal("shared backup verification", s.BackupState, s.BackupError)
		}
		return s.BackupState == "succeeded"
	})
	if err = read.Close(); err != nil {
		t.Fatal(err)
	}
	checked := c.Snapshot()
	if checked.State != "stopped" || checked.Attempts != 0 || checked.PayoutAddress != want.Address {
		t.Fatal("verification started mining or changed recipient")
	}
	// Close the only original wallet handle, then remove its expected pathname.
	// The independent restorer below cannot use that database as a fallback.
	poolActualLogout(t, ctx, c)
	original := filepath.Join(root, "shared", "gyms", before.Account.ID, "wallet")
	retained := original + ".retained-for-acceptance"
	if err = os.Rename(original, retained); err != nil {
		t.Fatal("make original unavailable", err)
	}
	restoredOriginal := false
	defer func() {
		if !restoredOriginal {
			if e := os.Rename(retained, original); e != nil {
				t.Error("retain original wallet after acceptance", e)
			}
		}
	}()
	if _, err = os.Lstat(original); !os.IsNotExist(err) {
		t.Fatal("original database remained available")
	}
	result := poolIndependentRestoreSpend(t, ctx, path, filepath.Join(artifacts, "independent-restored-wallet"), node, want, paid[fixture])
	result.Fixture = fixture
	result.Verified = true
	result.OriginalInaccessible = true
	if err = os.Rename(retained, original); err != nil {
		t.Fatal("return unchanged original wallet", err)
	}
	restoredOriginal = true
	_ = poolActualLogin(t, ctx, c, originalFixture)
	t.Logf("shared backup verified; independent file-only restore recovered %d atoms and confirmed spend %s with original database inaccessible", result.RestoredAtoms, result.SpendTransaction)
	return result
}

func poolIndependentRestoreSpend(t *testing.T, ctx context.Context, path, fresh string, node *poolActualRPC, want poolRecoveryOracle, paid int64) poolBackupPublic {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > recovery.MaxExportSize {
		t.Fatal("selected public-fixture export unavailable")
	}
	defer clear(raw)
	bundle, err := recovery.ParseExport(raw)
	if err != nil {
		t.Fatal("strict selected backup parse")
	}
	defer func() { bundle.Mnemonic = "" }()
	if bundle.Manifest.RootPublicKey != want.Root || bundle.Manifest.GymID != want.Gym || bundle.Manifest.GymPublicKey != want.Key {
		t.Fatal("export identity differs from independent oracle")
	}
	nativeFound := false
	for _, a := range bundle.Manifest.Accounts {
		if a.Purpose == 84 && a.Coin == 0 && a.Account == 0 {
			if a.NextExternal != 1 {
				t.Fatal("export lost exact native84 recipient cursor")
			}
			nativeFound = true
		}
	}
	if !nativeFound {
		t.Fatal("missing native84 account metadata")
	}
	loader := wallet.NewLoader(&chaincfg.MainNetParams, fresh, false, 5*time.Second, 20, wallet.WithWalletSyncRetryInterval(50*time.Millisecond))
	w, err := loader.RestoreRecoveryWallet([]byte(poolRestorePassword), []byte(poolRestorePassword), bundle)
	if err != nil {
		t.Fatal("independent file-only restore", err)
	}
	var backend *chain.RPCClient
	var server *legacyrpc.Server
	var once sync.Once
	closeWallet := func() {
		once.Do(func() {
			if server != nil {
				server.Stop()
			}
			if err := loader.UnloadWallet(); err != nil {
				t.Error("independent restored wallet close", err)
			}
			if backend != nil {
				backend.Stop()
				backend.WaitForShutdown()
			}
		})
	}
	defer closeWallet()
	backend, err = chain.NewRPCClient(&chaincfg.MainNetParams, node.address, node.user, node.password, nil, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = backend.Start(ctx); err != nil {
		t.Fatal("independent restored chain attachment", err)
	}
	w.SynchronizeRPC(backend)
	var height int32
	if err = node.call(ctx, "getblockcount", nil, &height); err != nil {
		t.Fatal(err)
	}
	poolActualWait(t, ctx, "independent restored wallet synchronizes", func(context.Context) bool { return w.ChainSynced() && w.SyncedTo().Height == height })
	gym, err := w.GymIdentity(ctx)
	if err != nil || gym.GymID != want.Gym || gym.PublicKey != want.Key {
		t.Fatal("restored gym differs from independent oracle")
	}
	if err = w.Unlock([]byte(poolRestorePassword), nil); err != nil {
		t.Fatal("unlock published restored fixture", err)
	}
	export, err := w.ExportRecovery(ctx)
	if err != nil {
		t.Fatal("verify restored metadata")
	}
	export.Mnemonic = ""
	if !reflect.DeepEqual(export.Manifest.Accounts, bundle.Manifest.Accounts) || export.Manifest.RootPublicKey != want.Root {
		t.Fatal("fresh restoration changed issued cursors or root")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server = legacyrpc.NewServer(&legacyrpc.Options{Username: "fixture", Password: poolRestorePassword, MaxPOSTClients: 2, MaxWebsocketClients: 0, AllowedMethods: []string{"listunspent", "listtransactions", "gettransaction", "sendtoaddress"}}, loader, []net.Listener{listener})
	server.RegisterWallet(w)
	server.SetChainServer(backend)
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	call := func(method string, args []any, out any) {
		t.Helper()
		if err := poolRestoredRPC(ctx, client, "http://"+listener.Addr().String(), method, args, out); err != nil {
			t.Fatal("restored fixture RPC", method, err)
		}
	}
	var utxos []btcjson.ListUnspentResult
	call("listunspent", []any{1, 9999999}, &utxos)
	var amount int64
	payment := ""
	funding := make(map[wire.OutPoint]int64)
	for _, u := range utxos {
		a, e := btcutil.NewAmount(u.Amount)
		if e != nil || a <= 0 || u.Address != want.Address || u.ScriptPubKey != want.Script || u.Confirmations < 1 {
			t.Fatal("restored payout UTXO differs from independent recipient")
		}
		hash, err := chainhash.NewHashFromStr(u.TxID)
		if err != nil {
			t.Fatal("invalid restored funding transaction")
		}
		outpoint := wire.OutPoint{Hash: *hash, Index: u.Vout}
		if _, duplicate := funding[outpoint]; duplicate {
			t.Fatal("duplicate restored funding outpoint")
		}
		funding[outpoint] = int64(a)
		amount += int64(a)
		if payment != "" && payment != u.TxID {
			t.Fatal("unexpected extra funding transaction")
		}
		payment = u.TxID
	}
	if amount != paid || payment == "" {
		t.Fatal("fresh restore lost exact paid wallet funds", amount, paid)
	}
	var history []btcjson.ListTransactionsResult
	call("listtransactions", []any{"*", 100, 0}, &history)
	found := false
	for _, row := range history {
		if row.TxID == payment && row.Category == "receive" {
			found = true
		}
	}
	if !found {
		t.Fatal("fresh restore lost pool payment history")
	}
	// One real restricted-test-server RPC spend to an independently published
	// external sink. No send operation is added to the desktop listener or UI.
	sinkRaw, err := os.ReadFile("../../../slugwallet/integration/testdata/addresses_v1.json")
	var sink struct {
		Address string `json:"external_sink"`
	}
	if err != nil || hex.EncodeToString(sumPoolBackup(sinkRaw)) != "f99f42e17cae250374277964621ac356e7d96d654e1ab74f5e986ae6c91b5bc8" || json.Unmarshal(sinkRaw, &sink) != nil || sink.Address == "" {
		t.Fatal("independent spend sink unavailable")
	}
	// Derived independently with Python Base58Check/SHA256 from the frozen
	// external_sink above: version 0x53, payload 06afd46b...4a37ecc.
	sinkScript, err := hex.DecodeString("76a91406afd46bcdfd22ef94ac122aa11f241244a37ecc88ac")
	if err != nil {
		t.Fatal("frozen sink script")
	}
	var spend string
	call("sendtoaddress", []any{sink.Address, 1}, &spend)
	poolActualConfirm(t, ctx, node, spend)
	poolActualWait(t, ctx, "restored spend wallet confirmation", func(context.Context) bool { return w.ChainSynced() && w.SyncedTo().Height == height+1 })
	var tx struct {
		Confirmations int64  `json:"confirmations"`
		TxID          string `json:"txid"`
		Hex           string `json:"hex"`
	}
	call("gettransaction", []any{spend}, &tx)
	if tx.Confirmations < 1 || tx.TxID != spend {
		t.Fatal("fresh restored wallet spend unconfirmed")
	}
	poolAssertRestoredSpend(t, w, tx.Hex, spend, funding, sinkScript, bundle.Manifest.Accounts)
	closeWallet()
	return poolBackupPublic{GymID: want.Gym, Address: want.Address, RestoredAtoms: amount, PaymentTransaction: payment, SpendTransaction: spend}
}

// The node has already confirmed this exact txid. Decode its signed bytes to
// prove which restored coins paid the fixed amount and where change went.
func poolAssertRestoredSpend(t *testing.T, w *wallet.Wallet, encoded, txID string, funding map[wire.OutPoint]int64, sinkScript []byte, accounts []recovery.Account) {
	t.Helper()
	raw, err := hex.DecodeString(encoded)
	if err != nil {
		t.Fatal("confirmed restored transaction encoding")
	}
	reader := bytes.NewReader(raw)
	var tx wire.MsgTx
	if tx.Deserialize(reader) != nil || reader.Len() != 0 || tx.TxHash().String() != txID || len(tx.TxIn) == 0 {
		t.Fatal("confirmed restored transaction bytes or identity")
	}
	seen := make(map[wire.OutPoint]bool)
	var inputAtoms, outputAtoms int64
	for _, input := range tx.TxIn {
		value, known := funding[input.PreviousOutPoint]
		if !known || seen[input.PreviousOutPoint] {
			t.Fatal("spend used an unknown or duplicate restored funding outpoint")
		}
		seen[input.PreviousOutPoint] = true
		inputAtoms += value
	}
	var sinks, changes int
	for _, output := range tx.TxOut {
		if output.Value <= 0 || output.Value > inputAtoms-outputAtoms {
			t.Fatal("restored spend output value exceeds its known funding")
		}
		outputAtoms += output.Value
		if bytes.Equal(output.PkScript, sinkScript) {
			sinks++
			if output.Value != 100000000 {
				t.Fatal("restored spend did not pay exactly one SLUG to the frozen sink")
			}
			continue
		}
		_, addresses, signatures, err := txscript.ExtractPkScriptAddrs(output.PkScript, &chaincfg.MainNetParams)
		if err != nil || len(addresses) != 1 || signatures != 1 {
			t.Fatal("restored spend has unsupported non-sink output")
		}
		managed, err := w.AddressInfo(addresses[0])
		if err != nil || !managed.Internal() || managed.Imported() {
			t.Fatal("restored spend change is not owned internal wallet derivation")
		}
		key, ok := managed.(waddrmgr.ManagedPubKeyAddress)
		if !ok {
			t.Fatal("restored change lacks deterministic key provenance")
		}
		scope, path, known := key.DerivationInfo()
		supported := false
		for _, account := range accounts {
			if scope.Purpose == account.Purpose && scope.Coin == account.Coin && path.InternalAccount == account.Account && path.Branch == 1 {
				supported = true
			}
		}
		if !known || !supported {
			t.Fatal("restored change derivation is outside the exported wallet profile")
		}
		changes++
	}
	if sinks != 1 || changes != 1 || inputAtoms <= outputAtoms {
		t.Fatal("restored spend lacks its exact sink, retained change, or positive fee")
	}
}

func poolRestoredRPC(ctx context.Context, client *http.Client, url, method string, args []any, out any) error {
	raw, err := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": 1, "method": method, "params": args})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.SetBasicAuth("fixture", poolRestorePassword)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 || response.StatusCode != 200 {
		return errors.New("bounded restored RPC unavailable")
	}
	var frame struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &frame) != nil || len(frame.Error) > 0 && string(frame.Error) != "null" {
		return errors.New("restored RPC rejected")
	}
	return json.Unmarshal(frame.Result, out)
}

func poolActualHeldExport(t *testing.T, ctx context.Context, c *PoolController, root, artifacts string) {
	t.Helper()
	s := c.Snapshot()
	if !s.Account.Held || s.Account.CanExport {
		t.Fatal("held account export capability")
	}
	path := filepath.Join(artifacts, "held-export-must-not-exist.json")
	selected, err := recoveryfile.Capture(path, []string{root}, recoveryfile.CreateMode)
	if err != nil {
		t.Fatal(err)
	}
	defer selected.Close()
	if c.ExportPoolRecovery(ctx, s.View.ViewID, selected.Pin()) == nil {
		t.Fatal("security hold permitted recovery export")
	}
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("held export published a file")
	}
}
