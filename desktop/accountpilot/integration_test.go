//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package accountpilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/wallet"
	_ "github.com/taytr-llc/slugwallet/walletdb/bdb"
)

type recoveryReference struct {
	Name      string          `json:"name"`
	Export    recovery.Export `json:"export"`
	Addresses []struct {
		Path   string `json:"path"`
		Value  string `json:"address"`
		Script string `json:"script_pub_key"`
	} `json:"addresses"`
}

func recoveryReferences(t *testing.T) map[string]recoveryReference {
	t.Helper()
	raw, err := os.ReadFile("../../../slugwallet/recovery/testdata/recovery_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		FixtureOnly bool                `json:"fixture_only"`
		Cases       []recoveryReference `json:"cases"`
	}
	if json.Unmarshal(raw, &file) != nil || !file.FixtureOnly {
		t.Fatal("independent recovery fixtures unavailable")
	}
	result := make(map[string]recoveryReference)
	for _, fixture := range file.Cases {
		result[fixture.Name] = fixture
	}
	return result
}

func (f recoveryReference) payout(t *testing.T) (string, string) {
	t.Helper()
	for _, entry := range f.Addresses {
		if entry.Path == "m/44'/0'/0'/0/0" {
			return entry.Value, entry.Script
		}
	}
	t.Fatal("independent payout address absent")
	return "", ""
}

func waitAccount(t *testing.T, c *Controller, duration time.Duration, ready func(pilot.Snapshot) bool) pilot.Snapshot {
	t.Helper()
	deadline := time.Now().Add(duration)
	for {
		state := c.Snapshot()
		if ready(state) {
			return state
		}
		if state.State == "failed" || state.Account == nil || state.Account.State == "failed" {
			t.Fatalf("account pilot failed: %+v", state)
		}
		if time.Now().After(deadline) {
			t.Fatalf("account pilot state deadline: %+v", state)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func accountSignedIn(s pilot.Snapshot) bool {
	return s.Account != nil && s.Account.State == "signed_in" && s.GymID != ""
}
func accountSignedOut(s pilot.Snapshot) bool {
	return s.Account != nil && s.Account.State == "signed_out"
}

func assertAccountIdentity(t *testing.T, value pilot.Snapshot, fixture recoveryReference) {
	t.Helper()
	payout, _ := fixture.payout(t)
	if !accountSignedIn(value) || value.Mode != pilot.AccountMode || value.PayoutAddress != payout ||
		value.GymID != fixture.Export.Manifest.GymID || value.GymPublicKey != fixture.Export.Manifest.GymPublicKey {
		t.Fatalf("account does not match independent %s fixture", fixture.Name)
	}
}

func fixtureBundle(t *testing.T, path string, expected recoveryReference) recovery.Export {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("retained export is not a private regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := recovery.ParseExport(raw)
	clear(raw)
	if err != nil || bundle.Mnemonic != expected.Export.Mnemonic || bundle.Manifest.RootPublicKey != expected.Export.Manifest.RootPublicKey ||
		bundle.Manifest.GymPublicKey != expected.Export.Manifest.GymPublicKey || bundle.Manifest.GymID != expected.Export.Manifest.GymID {
		t.Fatal("actual retained export differs from independent fixture")
	}
	return bundle
}

func accountToken(t *testing.T, c *Controller) (string, string) {
	t.Helper()
	c.credentials.Lock()
	defer c.credentials.Unlock()
	return c.session.Token, c.session.Account.ID
}

func assertPrivateSnapshot(t *testing.T, c *Controller, phrases ...string) {
	t.Helper()
	raw, err := json.Marshal(c.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	c.credentials.Lock()
	tokens := []string{c.session.Token, c.mining.Token}
	c.credentials.Unlock()
	for _, secret := range append(tokens, phrases...) {
		if secret != "" && bytes.Contains(raw, []byte(secret)) {
			t.Fatal("public account snapshot exposed private helper material")
		}
	}
}

func checkAccountListenersClosed(t *testing.T, root string) {
	t.Helper()
	var ports []string
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if strings.HasSuffix(path, ".conf") {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, line := range strings.Split(string(raw), "\n") {
				for _, prefix := range []string{"rpclisten=", "listen="} {
					if strings.HasPrefix(line, prefix) {
						ports = append(ports, strings.TrimPrefix(line, prefix))
					}
				}
			}
		}
		if entry.Name() == "SLUG-PILOT.json" {
			raw, _ := os.ReadFile(path)
			var marker struct {
				AuthorityPort uint16 `json:"authority_port"`
			}
			if json.Unmarshal(raw, &marker) == nil && marker.AuthorityPort != 0 {
				ports = append(ports, "127.0.0.1:"+strconv.Itoa(int(marker.AuthorityPort)))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Error("listener inventory:", walkErr)
	}
	for _, address := range ports {
		connection, err := net.DialTimeout("tcp", address, 150*time.Millisecond)
		if err == nil {
			connection.Close()
			t.Errorf("owned account listener survived Close at %s", address)
		}
	}
	t.Logf("verified %d retained node/authority listener addresses are closed", len(ports))
}

// This bounded production-PoW gate exercises the new account boundary. Prior
// set-25/26 gates establish 100-block maturity and restored spending; this test
// does not pretend that another first-block receipt repeats those fund tests.
func TestAccountPilotNormalPoW(t *testing.T) {
	binary := os.Getenv("SLUG_DESKTOP_NODE")
	if binary == "" {
		t.Skip("set SLUG_DESKTOP_NODE to an immutable normal-PoW node")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("absolute immutable node executable required")
	}
	rawBinary, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	nodeHash := fmt.Sprintf("%x", sha256.Sum256(rawBinary))
	t.Log("normal node SHA256", nodeHash)
	root := t.TempDir()
	if artifacts := os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"); artifacts != "" {
		if !filepath.IsAbs(artifacts) {
			t.Fatal("absolute artifact directory required")
		}
		root, err = os.MkdirTemp(artifacts, "account-normal-")
		if err != nil {
			t.Fatal(err)
		}
		t.Log("public-fixture runtime artifacts", root)
	}
	config := pilot.Config{DataDir: filepath.Join(root, "LocalAccountPilot"), NodeBinary: binary}
	c, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error("account close:", err)
		}
		checkAccountListenersClosed(t, config.DataDir)
	})
	references := recoveryReferences(t)
	aliceFixture, bobFixture := references["zero256"], references["incrementing256"]
	if !accountSignedOut(c.Snapshot()) {
		t.Fatal("new account helper was already signed in")
	}
	if err := c.LoginFixture("zero256"); err != nil {
		t.Fatal(err)
	}
	alice := waitAccount(t, c, 40*time.Second, accountSignedIn)
	assertAccountIdentity(t, alice, aliceFixture)
	if alice.State != "stopped" || alice.Attempts != 0 || alice.Height != 0 || alice.ProspectCount != 0 {
		t.Fatal("login/Prepare silently mined")
	}
	loginBeforeReset, aliceID := accountToken(t, c)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitAccount(t, c, 45*time.Second, func(s pilot.Snapshot) bool { return s.Height >= 1 && s.ProspectCount >= 1 })
	c.Stop()
	first := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "stopped" })
	if len(first.Prospects) == 0 {
		t.Fatal("normal worker produced no receipt")
	}
	receipt, err := c.Receipt(first.Prospects[0].Sequence)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Authorization == nil || receipt.Birth == nil || receipt.Verified.GymID != aliceFixture.Export.Manifest.GymID ||
		gymauth.VerifyAuthorization(receipt.Context, receipt.Proof, *receipt.Authorization) != nil {
		t.Fatal("account receipt lacks original gym authorization")
	}
	verified, err := prospect.Verify(receipt.Context, receipt.Proof)
	if err != nil || verified != receipt.Verified {
		t.Fatal("actual account work failed full production verification:", err)
	}
	coinbaseRaw, _ := hex.DecodeString(receipt.Proof.Coinbase)
	var coinbase wire.MsgTx
	if coinbase.Deserialize(bytes.NewReader(coinbaseRaw)) != nil {
		t.Fatal("invalid receipt coinbase")
	}
	_, payoutScript := aliceFixture.payout(t)
	paid := false
	for _, output := range coinbase.TxOut {
		if output.Value >= 10e8 && hex.EncodeToString(output.PkScript) == payoutScript {
			paid = true
		}
	}
	if !paid {
		t.Fatal("account work did not pay independent wallet address")
	}
	exportA := filepath.Join(root, "alice-retained-recovery.json")
	if err := c.ExportRecovery(exportA); err != nil {
		t.Fatal("actual retained export:", err)
	}
	bundleA := fixtureBundle(t, exportA, aliceFixture)
	if err := c.VerifyBackup(exportA); err != nil {
		t.Fatal(err)
	}
	backup := waitAccount(t, c, time.Minute, func(s pilot.Snapshot) bool { return s.BackupState == "succeeded" || s.BackupState == "failed" })
	if backup.BackupState != "succeeded" || backup.Attempts != first.Attempts || backup.ProspectCount != first.ProspectCount {
		t.Fatal("file-only backup verification failed or mined:", backup.BackupError)
	}
	assertPrivateSnapshot(t, c, aliceFixture.Export.Mnemonic, bobFixture.Export.Mnemonic)
	if err := c.Logout(); err != nil {
		t.Fatal(err)
	}
	waitAccount(t, c, 30*time.Second, accountSignedOut)
	if _, err := c.Receipt(receipt.Sequence); err == nil {
		t.Fatal("signed-out account could read scoped receipt")
	}
	if err := c.store.WithSession(context.Background(), loginBeforeReset, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); err == nil {
		t.Fatal("logout retained old login authority")
	}
	if err := c.LoginFixture("zero256"); err != nil {
		t.Fatal(err)
	}
	relogin := waitAccount(t, c, 30*time.Second, accountSignedIn)
	assertAccountIdentity(t, relogin, aliceFixture)
	if relogin.Account.ID != aliceID || relogin.Attempts != first.Attempts || relogin.ProspectCount != first.ProspectCount {
		t.Fatal("relogin replaced wallet/gym/work history")
	}
	loginBeforeReset, _ = accountToken(t, c)
	if err := c.ResetFixtureAccount(); err != nil {
		t.Fatal(err)
	}
	held := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool {
		return accountSignedIn(s) && s.Account.Generation > relogin.Account.Generation
	})
	assertAccountIdentity(t, held, aliceFixture)
	if !held.Account.Held || held.Account.CanExport || held.Account.ID != aliceID || held.Account.Generation != relogin.Account.Generation+1 ||
		held.Account.HoldUntilMS < time.Now().Add(23*time.Hour+59*time.Minute).UnixMilli() || held.Account.HoldUntilMS > time.Now().Add(24*time.Hour).UnixMilli() {
		t.Fatal("reset lost original identity or approved 24-hour hold")
	}
	if err := c.store.WithSession(context.Background(), loginBeforeReset, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); err == nil {
		t.Fatal("reset retained prior-generation session")
	}
	deniedExport := filepath.Join(root, "must-not-export-during-hold.json")
	if err := c.ExportRecovery(deniedExport); err == nil {
		t.Fatal("post-reset hold allowed recovery export")
	}
	if _, err := os.Stat(deniedExport); !os.IsNotExist(err) {
		t.Fatal("held export published a file")
	}
	if err := c.Start(); err != nil {
		t.Fatal("same-wallet mining prohibited during hold:", err)
	}
	waitAccount(t, c, 45*time.Second, func(s pilot.Snapshot) bool { return s.ProspectCount > first.ProspectCount })
	c.Stop()
	afterHoldMine := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "stopped" })
	if afterHoldMine.Account.HoldUntilMS != held.Account.HoldUntilMS {
		t.Fatal("mining renewed the reset hold")
	}
	original, err := c.Receipt(receipt.Sequence)
	if err != nil || !reflect.DeepEqual(original, receipt) {
		t.Fatal("reset/retry rewrote historical receipt or birth")
	}
	if err := c.Logout(); err != nil {
		t.Fatal(err)
	}
	waitAccount(t, c, 30*time.Second, accountSignedOut)
	if err := c.LoginFixture("incrementing256"); err != nil {
		t.Fatal(err)
	}
	bob := waitAccount(t, c, 40*time.Second, accountSignedIn)
	assertAccountIdentity(t, bob, bobFixture)
	if bob.Account.ID == aliceID || bob.GymID == alice.GymID || bob.State != "stopped" || bob.Attempts != 0 || bob.ProspectCount != 0 {
		t.Fatal("second account adopted first wallet/work or auto-mined")
	}
	if _, err := c.Receipt(receipt.Sequence); err == nil {
		t.Fatal("B read A's receipt sequence")
	}
	if err := c.VerifyBackup(exportA); err == nil {
		cross := waitAccount(t, c, time.Minute, func(s pilot.Snapshot) bool { return s.BackupState == "failed" || s.BackupState == "succeeded" })
		if cross.BackupState != "failed" {
			t.Fatal("B accepted A's recovery file")
		}
	}
	protected := filepath.Join(config.DataDir, "gyms", aliceID, "SLUG-PILOT.json")
	before, err := os.ReadFile(protected)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ExportRecovery(protected); err == nil {
		t.Fatal("B export could overwrite sibling account state")
	}
	after, _ := os.ReadFile(protected)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected export changed sibling account state")
	}
	exportB := filepath.Join(root, "bob-retained-recovery.json")
	if err := c.ExportRecovery(exportB); err != nil {
		t.Fatal(err)
	}
	_ = fixtureBundle(t, exportB, bobFixture)
	assertPrivateSnapshot(t, c, aliceFixture.Export.Mnemonic, bobFixture.Export.Mnemonic)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	checkAccountListenersClosed(t, config.DataDir)

	// Restore receives the ACTUAL later export only. Make the original database
	// inaccessible, choose new public fixture passwords, and prove the same
	// independently derived enrollment signature without a seed constructor.
	originalWallet := filepath.Join(config.DataDir, "gyms", aliceID, "wallet")
	unavailable := originalWallet + ".inaccessible-for-restore"
	if err := os.Rename(originalWallet, unavailable); err != nil {
		t.Fatal(err)
	}
	restoredDir := filepath.Join(root, "file-only-restored-alice")
	loader := wallet.NewLoader(&chaincfg.MainNetParams, restoredDir, true, 5*time.Second, 20)
	password := []byte("PUBLIC-ACCOUNT-RESTORE-FIXTURE-ONLY")
	restored, err := loader.RestoreRecoveryWallet(password, password, bundleA)
	if err != nil {
		t.Fatal("file-only fixture restore:", err)
	}
	t.Cleanup(func() {
		if _, loaded := loader.LoadedWallet(); loaded {
			if err := loader.UnloadWallet(); err != nil {
				t.Error("restored fixture close:", err)
			}
		}
	})
	if err := restored.Unlock(password, nil); err != nil {
		t.Fatal(err)
	}
	identity, err := restored.GymIdentity(context.Background())
	if err != nil || identity.GymID != aliceFixture.Export.Manifest.GymID || identity.PublicKey != aliceFixture.Export.Manifest.GymPublicKey {
		t.Fatal("restored fixture changed gym")
	}
	reference := accountReferences(t)["zero256"]
	authorization, err := restored.SignAccountChallenge(context.Background(), reference.Challenge.AuthorityID, reference.Challenge)
	if err != nil || authorization != reference.Authorization {
		t.Fatal("restored fixture did not reproduce independent enrollment signature")
	}
	if err := loader.UnloadWallet(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(unavailable, originalWallet); err != nil {
		t.Fatal(err)
	}
	c, err = New(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.LoginFixture("zero256"); err != nil {
		t.Fatal(err)
	}
	reopened := waitAccount(t, c, 30*time.Second, accountSignedIn)
	if reopened.Account.ID != aliceID || reopened.Account.Generation != held.Account.Generation || reopened.Account.HoldUntilMS != held.Account.HoldUntilMS || !reopened.Account.Held {
		t.Fatal("reopen forgot reset or original identity")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(originalWallet, unavailable); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(unavailable, originalWallet)
	c, err = New(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.LoginFixture("zero256"); err != nil {
		t.Fatal(err)
	}
	failed := waitAccount(t, c, 15*time.Second, func(s pilot.Snapshot) bool { return s.Account != nil && s.Account.State == "failed" })
	if failed.GymID != "" {
		t.Fatal("missing established wallet returned another identity")
	}
	if _, err := os.Stat(originalWallet); !os.IsNotExist(err) {
		t.Fatal("missing established wallet was regenerated from public seed")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	checkAccountListenersClosed(t, config.DataDir)
	result := map[string]any{"node_sha256": nodeHash, "account_a": aliceID, "account_b": bob.Account.ID,
		"gym_a": alice.GymID, "gym_b": bob.GymID, "first_receipt": receipt.Verified.ProspectID,
		"first_birth_ms": receipt.Birth.BornAtMS, "reset_generation": held.Account.Generation,
		"hold_until_ms": held.Account.HoldUntilMS, "normal_pow": true, "login_prepare_no_mining": true,
		"same_wallet_reset": true, "mining_during_hold": true, "export_during_hold_rejected": true,
		"cross_account_isolation": true, "file_only_restore": true, "missing_wallet_rejected": true}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "result.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("two independent fixtures, normal admission, retained backup, reset hold, same-wallet restore and isolation passed")
}
