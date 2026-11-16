package pilot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestAccountWalletExplicitFixtureIsolation(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-chain account fixture")
	}
	data, err := os.ReadFile("../../gymaccount/testdata/enrollment_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		FixtureOnly bool `json:"fixture_only"`
		Cases       []struct {
			FixtureName   string                         `json:"fixture_name"`
			Challenge     gymaccount.EnrollmentChallenge `json:"challenge"`
			Authorization gymaccount.Authorization       `json:"authorization"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil || !fixture.FixtureOnly || len(fixture.Cases) != 4 {
		t.Fatal("invalid independent public fixture", err)
	}
	for _, name := range []string{"zero256", "incrementing256"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "wallet")
			r, err := openAccountFixtureWallet(dir, name, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := r.Close(); err != nil {
					t.Error(err)
				}
			})
			expected, err := gymaccount.FixtureIdentity(name)
			if err != nil || r.Identity() != (walletIdentity{RootPublicKey: expected.RootPublicKey, GymPublicKey: expected.GymPublicKey, GymID: expected.GymID}) {
				t.Fatal("wrong public identity")
			}
			for _, v := range fixture.Cases {
				got, err := r.SignAccountChallenge(context.Background(), v.Challenge.AuthorityID, v.Challenge)
				if v.FixtureName == name {
					if err != nil || got != v.Authorization {
						t.Fatal("signer differs from independent fixture", err)
					}
				} else if err == nil || got != (gymaccount.Authorization{}) {
					t.Fatal("cross-account signing accepted")
				}
				if !r.wallet.Locked() {
					t.Fatal("short account signing left wallet unlocked")
				}
			}
			bundle, err := r.ExportRecovery(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			if name == "incrementing256" {
				if old, err := openFixtureWallet(dir, false); err == nil {
					old.Close()
					t.Fatal("old mode accepted second fixture")
				}
				wrongRestore := filepath.Join(t.TempDir(), "wrong-restore")
				if old, err := restoreFixtureWallet(wrongRestore, bundle); err == nil {
					old.Close()
					t.Fatal("old restore broadened")
				}
				if _, err := os.Stat(wrongRestore); !os.IsNotExist(err) {
					t.Fatal("wrong restore mutated destination")
				}
			}
			other := "zero256"
			if name == other {
				other = "incrementing256"
			}
			if wrong, err := openAccountFixtureWallet(dir, other, false); err == nil {
				wrong.Close()
				t.Fatal("wrong configured identity reopened")
			}
			r, err = openAccountFixtureWallet(dir, name, false)
			if err != nil {
				t.Fatal("reopen after mismatch", err)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			restored, err := restoreAccountFixtureWallet(filepath.Join(t.TempDir(), "restored"), name, bundle)
			if err != nil {
				t.Fatal(err)
			}
			if restored.Identity() != r.Identity() {
				t.Fatal("restore changed identity")
			}
			if err := restored.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAccountWalletRefusesUnknownWithoutCreation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wallet")
	if r, err := openAccountFixtureWallet(dir, "real-wallet", true); err == nil {
		r.Close()
		t.Fatal("unknown profile accepted")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("unsupported fixture created data")
	}
}
