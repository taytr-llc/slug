//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package pilot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymauth"
)

func TestPilotRequiresActivatedNodeMetadata(t *testing.T) {
	for _, kind := range []string{"current60", "current61", "current62", "old-binary", "wrong-network", "wrong-genesis", "inconsistent-status"} {
		t.Run(kind, func(t *testing.T) {
			height := 61
			if kind == "current60" {
				height = 60
			}
			if kind == "current62" {
				height = 62
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Method string `json:"method"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid request")
					return
				}
				var result any
				switch request.Method {
				case "getcurrentnet":
					result = policy().Network
					if kind == "wrong-network" {
						result = uint32(1)
					}
				case "getblockhash":
					result = genesis
					if kind == "wrong-genesis" {
						result = "other"
					}
				case "getblockchaininfo":
					status := "active"
					if height < 61 || kind == "inconsistent-status" {
						status = "defined"
					}
					override := 62
					if kind == "old-binary" {
						override = 0
					}
					forks := map[string]any{}
					for _, name := range []string{"csv", "segwit", "taproot"} {
						forks[name] = map[string]any{"status": status, "always_active_height": override}
					}
					result = map[string]any{"chain": "mainnet", "blocks": height, "bip9_softforks": forks}
				default:
					t.Error("unexpected request", request.Method)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "error": nil, "result": result})
			}))
			defer server.Close()
			n := &childNode{done: make(chan struct{}), rpc: &nodeRPC{url: server.URL, client: server.Client()}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := waitNode(ctx, n, false)
			valid := kind == "current60" || kind == "current61" || kind == "current62"
			if (err == nil) != valid {
				t.Fatal("wrong activation readiness", err)
			}
		})
	}
}

func TestBackupReaderRejectsSelectedSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "fixture.json")
	if err := os.WriteFile(real, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "chosen.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readBackup(link); err == nil {
		t.Fatal("selected path became a symlink")
	}
}

func TestBackupKeyControlChallengeDoesNotNeedCurrentChainOrPoW(t *testing.T) {
	wallet, err := openFixtureWallet(filepath.Join(t.TempDir(), "wallet"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer wallet.Close()
	ctx := context.Background()
	initial, proof, err := backupSigningProof(localTip{Hash: genesis, Height: 0}, publishedWalletIdentity)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := wallet.Signer(ctx, policy())
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := signer(ctx, initial, proof)
	if err != nil {
		t.Fatal(err)
	}
	if err := gymauth.VerifyAuthorization(initial, proof, authorization); err != nil {
		t.Fatal(err)
	}
	proof.Header = "00" + proof.Header[2:]
	if err := gymauth.VerifyAuthorization(initial, proof, authorization); err == nil {
		t.Fatal("changed challenge retained authorization")
	}
}
