//go:build !slugtestpow

package pilot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStableUpgradeNewRequiresOriginalRealLock(t *testing.T) {
	for _, mode := range []string{"missing", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			cfg := testConfig(t)
			a := manifestTestAccount()
			if err := os.Mkdir(cfg.DataDir, 0700); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"instance-lock", "primary-chain", "peer-chain"} {
				if err := os.Mkdir(filepath.Join(cfg.DataDir, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			m := manifestIdentity{Version: 1, Mode: ManifestMode, Phase: "new", AccountAuthorityID: a.Account.AuthorityID, AccountID: a.Account.AccountID, Fixture: "zero256", Network: policy().Network, Genesis: genesis, ActivationHeight: 62, RecoveryProfile: "slug-phase-a-fixture-bip39-v1", AuthorityPort: 37619, DescriptorID: manifestDescriptorID}
			if err := atomicJSON(filepath.Join(cfg.DataDir, manifestMarkerName), m); err != nil {
				t.Fatal(err)
			}
			var err error
			lock := filepath.Join(cfg.DataDir, "instance-lock")
			saved := filepath.Join(t.TempDir(), "saved-lock")
			if err = os.Rename(lock, saved); err != nil {
				t.Fatal(err)
			}
			if mode == "symlink" {
				if err = os.Symlink(saved, lock); err != nil {
					t.Fatal(err)
				}
			}
			if err = ValidateStableUpgradeChild(cfg, a.Account.AuthorityID, a.Account.AccountID); err == nil {
				t.Fatal("missing/aliased original lock accepted")
			}
			if mode == "missing" {
				if _, err = os.Lstat(lock); !os.IsNotExist(err) {
					t.Fatal("validation created lock")
				}
			}
		})
	}
}
