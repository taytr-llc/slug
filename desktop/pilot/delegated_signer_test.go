package pilot

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

type delegatedTestAuthority struct {
	mu      sync.Mutex
	key     string
	revoked bool
}

func TestDelegatedAdmissionSignerRejectsCallbackProofMutation(t *testing.T) {
	raw, err := os.ReadFile("../../prospectjob/testdata/jobs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Context       prospect.Context      `json:"context"`
		Proof         prospect.Proof        `json:"proof"`
		PrivateKey    string                `json:"test_only_private_key"`
		Authorization gymauth.Authorization `json:"authorization"`
	}
	if json.Unmarshal(raw, &fixtures) != nil || len(fixtures) != 2 || len(fixtures[1].Proof.MerkleBranch) == 0 {
		t.Fatal("independent nonempty-branch fixture unavailable")
	}
	fixture := fixtures[1]
	private, err := hex.DecodeString(fixture.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := btcec.PrivKeyFromBytes(private)
	clear(private)
	defer key.Zero()
	authority := &delegatedTestAuthority{key: fixture.Authorization.PublicKey}
	config := &AccountConfig{Authorizer: authority, MiningToken: func(context.Context) (string, error) { return "public-fixture-mining-token", nil }}
	config.AdmissionSigner = func(_ context.Context, _ string, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
		proof.MerkleBranch[0] = strings.Repeat("29", 32)
		return gymauth.SignAdmission(initial, proof, key)
	}
	c := &Controller{account: config}
	c.manifest.Identity.GymPublicKey = fixture.Authorization.PublicKey
	before := append([]string(nil), fixture.Proof.MerkleBranch...)
	auth, err := c.delegatedAccountSigner()(context.Background(), fixture.Context, fixture.Proof)
	if err == nil || auth != (gymauth.Authorization{}) {
		t.Fatal("accepted callback signature for mutated evidence")
	}
	if !reflect.DeepEqual(before, fixture.Proof.MerkleBranch) {
		t.Fatal("callback mutated caller's proof")
	}
}

func (a *delegatedTestAuthority) revoke() { a.mu.Lock(); a.revoked = true; a.mu.Unlock() }
func (a *delegatedTestAuthority) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	if !a.mu.TryLock() {
		return errors.New("reentrant authority lease")
	}
	defer a.mu.Unlock()
	check := func() error {
		if ctx.Err() != nil || token != "public-fixture-mining-token" || a.revoked {
			return errors.New("denied")
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	return fn(a.key, check)
}

func TestDelegatedAdmissionSignerReauthorizesOutsideProviderCall(t *testing.T) {
	raw, err := os.ReadFile("../../prospectjob/testdata/jobs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Context       prospect.Context      `json:"context"`
		Proof         prospect.Proof        `json:"proof"`
		Authorization gymauth.Authorization `json:"authorization"`
	}
	if json.Unmarshal(raw, &fixtures) != nil || len(fixtures) < 2 {
		t.Fatal("independent authorization fixtures unavailable")
	}
	fixture := fixtures[0]
	for _, name := range []string{"success", "revoked-before", "revoked-during", "canceled", "wrong-key", "bad-signature", "provider-error", "invalid-proof"} {
		t.Run(name, func(t *testing.T) {
			authority := &delegatedTestAuthority{key: fixture.Authorization.PublicKey}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			called := false
			config := &AccountConfig{Authorizer: authority, MiningToken: func(context.Context) (string, error) { return "public-fixture-mining-token", nil }}
			config.AdmissionSigner = func(ctx context.Context, token string, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
				called = true
				// The vault must take its own lease without a caller lease blocking it.
				if err := authority.WithSession(ctx, token, func(key string, recheck func() error) error { return recheck() }); err != nil {
					return gymauth.Authorization{}, err
				}
				auth := fixture.Authorization
				switch name {
				case "revoked-during":
					authority.revoke()
				case "canceled":
					cancel()
				case "wrong-key":
					auth = fixtures[1].Authorization
				case "bad-signature":
					auth.Signature = "00"
				case "provider-error":
					return gymauth.Authorization{}, errors.New("private-provider-error-must-not-escape")
				}
				return auth, nil
			}
			c := &Controller{account: config}
			c.manifest.Identity.GymPublicKey = fixture.Authorization.PublicKey
			proof := fixture.Proof
			if name == "revoked-before" {
				authority.revoke()
			}
			if name == "invalid-proof" {
				proof.GymID = strings.Repeat("0", 64)
			}
			auth, err := c.delegatedAccountSigner()(ctx, fixture.Context, proof)
			if name == "success" {
				if err != nil || !called || auth != fixture.Authorization {
					t.Fatal("delegated signer failed or nested the lease", err)
				}
			} else {
				if err == nil || auth != (gymauth.Authorization{}) {
					t.Fatal("unavailable delegated signer leaked authorization")
				}
				if strings.Contains(err.Error(), "private-provider") {
					t.Fatal("provider error escaped")
				}
				if (name == "revoked-before" || name == "invalid-proof") && called {
					t.Fatal("provider called before admission preflight")
				}
			}
		})
	}
}
