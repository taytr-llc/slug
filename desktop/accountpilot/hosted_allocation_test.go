// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"github.com/taytr-llc/slugwallet/wallet"
	"os"
	"path/filepath"
	"testing"
)

func TestHostedRecoveryOnlyExactPayloadAbsence(t *testing.T) {
	root := t.TempDir()
	absent, e := hostedWalletAbsent(root)
	if e != nil || !absent {
		t.Fatal(absent, e)
	}
	dir := filepath.Join(root, "wallet")
	if _, e := os.Lstat(dir); !os.IsNotExist(e) {
		t.Fatal("classification created wallet")
	}
	if e = os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	absent, e = hostedWalletAbsent(root)
	if e != nil || !absent {
		t.Fatal(absent, e)
	}
	path := filepath.Join(dir, wallet.WalletDBName)
	if e = os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	if _, e = hostedWalletAbsent(root); e == nil {
		t.Fatal("directory became wallet loss")
	}
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(t.TempDir(), "payload")
	if e = os.WriteFile(target, []byte("not a wallet"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(target, path); e != nil {
		t.Fatal(e)
	}
	if _, e = hostedWalletAbsent(root); e == nil {
		t.Fatal("symlink became wallet loss")
	}
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, []byte("not a wallet"), 0600); e != nil {
		t.Fatal(e)
	}
	absent, e = hostedWalletAbsent(root)
	if e != nil || absent {
		t.Fatal("present corrupt payload became loss", absent, e)
	}
	raw, e := os.ReadFile(path)
	if e != nil || string(raw) != "not a wallet" {
		t.Fatal("classification touched payload")
	}
}
