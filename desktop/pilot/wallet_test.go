package pilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/wallet"
)

func TestFixtureWalletRetainedLifecycle(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal wallet profile only")
	}
	dir := filepath.Join(t.TempDir(), "wallet")
	r, err := openFixtureWallet(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Identity() != publishedWalletIdentity || !r.wallet.Locked() {
		t.Fatal("incorrect initial identity or lock state")
	}
	bundle, err := r.ExportRecovery(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !r.wallet.Locked() {
		t.Fatal("export retained unlock")
	}
	// Independent public zero256 BIP39 fixture, not an implementation golden.
	if bundle.Mnemonic != strings.Repeat("abandon ", 23)+"art" || bundle.Manifest.RootPublicKey != publishedWalletIdentity.RootPublicKey {
		t.Fatal("retained fixture differs")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.ExportRecovery(ctx); err == nil || !r.wallet.Locked() {
		t.Fatal("canceled export changed lock state")
	}
	if _, err := openFixtureWallet(dir, true); err == nil {
		t.Fatal("overwrote established wallet")
	}
	if err := r.wallet.Unlock([]byte(fixtureWalletPrivatePass), nil); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !r.wallet.Manager.IsLocked() {
		t.Fatal("closed runtime retained unlocked manager")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ExportRecovery(context.Background()); err == nil {
		t.Fatal("export from closed wallet")
	}
	reopened, err := openFixtureWallet(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := reopened.ExportRecovery(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if retained.Mnemonic != bundle.Mnemonic || reopened.Identity() != r.Identity() {
		t.Fatal("reopen lost retained identity")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := restoreFixtureWallet(filepath.Join(t.TempDir(), "restored"), retained)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.Identity() != publishedWalletIdentity {
		t.Fatal("file-only restore identity")
	}
	// Other valid public test entropy must never broaden this desktop profile.
	other, err := recovery.NewExport([32]byte{1}, recovery.DefaultAccounts())
	if err != nil {
		t.Fatal(err)
	}
	rejected := filepath.Join(t.TempDir(), "foreign")
	if _, err := restoreFixtureWallet(rejected, other); err == nil {
		t.Fatal("accepted different fixture")
	}
	if _, err := os.Stat(rejected); !os.IsNotExist(err) {
		t.Fatal("foreign bundle created a wallet directory")
	}
}
func TestFixtureWalletMissingSymlinkAndNetworkRefusal(t *testing.T) {
	if wire.TestPoW {
		if _, err := openFixtureWallet(filepath.Join(t.TempDir(), "wallet"), true); err == nil {
			t.Fatal("cheap wallet accepted")
		}
		return
	}
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := openFixtureWallet(dir, false); err == nil {
		t.Fatal("missing wallet recreated")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("missing wallet wrote files")
	}
	realDir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, link); err == nil {
		if _, err := openFixtureWallet(link, true); err == nil {
			t.Fatal("symlink wallet accepted")
		}
	}
	r, err := openFixtureWallet(filepath.Join(t.TempDir(), "wallet"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, endpoint := range []string{"http://example.com:29332", "http://localhost:29332", "https://127.0.0.1:29332", "http://127.0.0.1:29332/?x=1", "http://u:p@127.0.0.1:29332"} {
		if err := r.Attach(context.Background(), prospectnode.RPCConfig{Version: 1, URL: endpoint}); err == nil {
			t.Fatalf("accepted %s", endpoint)
		}
	}
	if r.backend != nil || r.rpc != nil {
		t.Fatal("invalid endpoint attached resources")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(realDir, wallet.WalletDBName)
	if err := os.WriteFile(db, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	linkedDir := t.TempDir()
	if err := os.Symlink(db, filepath.Join(linkedDir, wallet.WalletDBName)); err == nil {
		if _, err := openFixtureWallet(linkedDir, false); err == nil {
			t.Fatal("symlink database accepted")
		}
	}
}
func TestWalletHTTPActualRequestAndRedaction(t *testing.T) {
	var requests atomic.Uint64
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		u, p, ok := r.BasicAuth()
		if !ok || u != "fixture-user" || p != "private-marker" || r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("invalid RPC transport")
		}
		var request struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Method != "getbalance" || len(request.Params) != 2 {
			t.Error("unexpected method/params")
		}
		fmt.Fprintf(w, `{"id":%d,"result":10.5,"error":null}`, request.ID)
	}))
	defer h.Close()
	client := newWalletHTTPClient(h.URL, "fixture-user", "private-marker")
	var amount float64
	if err := client.call(context.Background(), "getbalance", []any{"default", 1}, &amount); err != nil || amount != 10.5 {
		t.Fatalf("actual HTTP RPC: amount %v error %v", amount, err)
	}
	for _, method := range []string{"stop", "dumpprivkey", "walletpassphrase", "signmessage", "sendtoaddress"} {
		if err := client.call(context.Background(), method, nil, &amount); err == nil {
			t.Fatal("private method sent")
		}
	}
	if requests.Load() != 1 || client.callCounts()["getbalance"] != 1 {
		t.Fatal("RPC dispatch count")
	}
	snapshot := client.callCounts()
	snapshot["getbalance"] = 99
	if client.callCounts()["getbalance"] != 1 {
		t.Fatal("mutable counters")
	}
}
func TestWalletHTTPBoundedErrorsAndNoRedirect(t *testing.T) {
	for _, mode := range []string{"error", "wrongid", "null", "oversize", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Uint64
			h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch mode {
				case "error":
					io.WriteString(w, `{"id":1,"result":null,"error":{"message":"private-marker"}}`)
				case "wrongid":
					io.WriteString(w, `{"id":2,"result":0,"error":null}`)
				case "null":
					io.WriteString(w, `{"id":1,"result":null,"error":null}`)
				case "oversize":
					io.WriteString(w, strings.Repeat(" ", maxWalletRPCResponse+1))
				case "redirect":
					http.Redirect(w, r, "/other?private-marker", 302)
				}
			}))
			defer h.Close()
			client := newWalletHTTPClient(h.URL, "fixture-user", "private-marker")
			var amount float64
			err := client.call(context.Background(), "getbalance", nil, &amount)
			if err == nil || strings.Contains(err.Error(), "private-marker") || strings.Contains(err.Error(), h.URL) {
				t.Fatalf("unsanitized/missing error: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatal("automatic retry or redirect")
			}
		})
	}
}
