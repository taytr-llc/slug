// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gymauth_test

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/wire/v2"
)

type referenceFixture struct {
	Name                  string                `json:"name"`
	TestOnlyPrivateKey    string                `json:"test_only_private_key"`
	Context               prospect.Context      `json:"context"`
	Proof                 prospect.Proof        `json:"proof"`
	Authorization         gymauth.Authorization `json:"authorization"`
	ExpectedGymID         string                `json:"expected_gym_id"`
	ExpectedDigest        string                `json:"expected_digest"`
	ExpectedVerified      prospect.Verified     `json:"expected_verified"`
	InvalidHighSSignature string                `json:"invalid_high_s_signature"`
	CommitmentScript      string                `json:"commitment_script"`
}

func referenceFixtures(t *testing.T) []referenceFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/authorizations_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []referenceFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 2 || len(fixtures[0].Proof.MerkleBranch) != 0 || len(fixtures[1].Proof.MerkleBranch) != 2 {
		t.Fatal("independent fixtures must cover two wallets and empty/two-level Merkle branches")
	}
	return fixtures
}

func referenceHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestReferenceIndependentAuthorizations(t *testing.T) {
	for _, fixture := range referenceFixtures(t) {
		t.Run(fixture.Name, func(t *testing.T) {
			gym, err := gymauth.GymID(fixture.Context, fixture.Authorization.PublicKey)
			if err != nil || gym != fixture.ExpectedGymID || gym != fixture.Proof.GymID {
				t.Fatalf("gym = %s, %v; want independent %s", gym, err, fixture.ExpectedGymID)
			}
			digest, err := gymauth.AdmissionDigest(fixture.Context, fixture.Proof, fixture.Authorization.PublicKey)
			if err != nil || hex.EncodeToString(digest[:]) != fixture.ExpectedDigest {
				t.Fatalf("digest = %x, %v; want independent %s", digest, err, fixture.ExpectedDigest)
			}
			if err := gymauth.VerifyAuthorization(fixture.Context, fixture.Proof, fixture.Authorization); err != nil {
				t.Fatalf("independently signed authorization rejected: %v", err)
			}
			// Scalars 1 and 2 are intentionally public, non-wallet test inputs.
			private, public := btcec.PrivKeyFromBytes(referenceHex(t, fixture.TestOnlyPrivateKey))
			if hex.EncodeToString(public.SerializeCompressed()) != fixture.Authorization.PublicKey {
				t.Fatal("Go public key differs from independent secp256k1 arithmetic")
			}
			signed, err := gymauth.SignAdmission(fixture.Context, fixture.Proof, private)
			if err != nil || signed != fixture.Authorization {
				t.Fatalf("signature differs from independent RFC6979/low-S DER: got %+v, %v; want %+v", signed, err, fixture.Authorization)
			}
			script, err := prospect.CommitmentScript(fixture.Context, gym)
			if err != nil || hex.EncodeToString(script) != fixture.CommitmentScript {
				t.Fatalf("gym-bound commitment = %x, %v; want %s", script, err, fixture.CommitmentScript)
			}
			verified, err := prospect.Verify(fixture.Context, fixture.Proof)
			if wire.TestPoW {
				if err == nil || verified != (prospect.Verified{}) {
					t.Fatalf("cheap profile returned production evidence: %+v, %v", verified, err)
				}
			} else if err != nil || verified != fixture.ExpectedVerified {
				t.Fatalf("work or DNA differs from independent production oracle: got %+v, %v; want %+v", verified, err, fixture.ExpectedVerified)
			}
		})
	}
}

func TestReferenceEquivalentRepresentations(t *testing.T) {
	fixture := referenceFixtures(t)[1]
	context, proof, authorization := fixture.Context, fixture.Proof, fixture.Authorization
	context.Genesis = strings.ToUpper(context.Genesis)
	context.ProspectTarget = strings.ToUpper(context.ProspectTarget)
	context.ParentHash = strings.ToUpper(context.ParentHash)
	// These are still valid trusted bounds. They are intentionally absent from
	// the signature, so a refreshed node upper bound needs no new wallet action.
	context.MinTimestamp--
	context.MaxTimestamp++
	proof.GymID = strings.ToUpper(proof.GymID)
	proof.Header = strings.ToUpper(proof.Header)
	proof.Coinbase = strings.ToUpper(proof.Coinbase)
	proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	for i := range proof.MerkleBranch {
		proof.MerkleBranch[i] = strings.ToUpper(proof.MerkleBranch[i])
	}
	authorization.PublicKey = strings.ToUpper(authorization.PublicKey)
	authorization.Signature = strings.ToUpper(authorization.Signature)
	digest, err := gymauth.AdmissionDigest(context, proof, authorization.PublicKey)
	if err != nil || hex.EncodeToString(digest[:]) != fixture.ExpectedDigest {
		t.Fatalf("equivalent bytes or time-bound refresh changed digest: %x, %v", digest, err)
	}
	if err := gymauth.VerifyAuthorization(context, proof, authorization); err != nil {
		t.Fatalf("equivalent representation rejected: %v", err)
	}
}

func TestReferenceAuthorizationBindings(t *testing.T) {
	fixtures := referenceFixtures(t)
	fixture := fixtures[1]
	mutations := []struct {
		name string
		edit func(*prospect.Context, *prospect.Proof, *gymauth.Authorization)
	}{
		{"network", func(c *prospect.Context, _ *prospect.Proof, _ *gymauth.Authorization) { c.Network++ }},
		{"genesis", func(c *prospect.Context, _ *prospect.Proof, _ *gymauth.Authorization) {
			c.Genesis = strings.Repeat("a", 64)
		}},
		{"target policy", func(c *prospect.Context, _ *prospect.Proof, _ *gymauth.Authorization) {
			c.ProspectTarget = strings.Repeat("e", 64)
		}},
		{"DNA version", func(c *prospect.Context, _ *prospect.Proof, _ *gymauth.Authorization) { c.DNAVersion++ }},
		{"proof version", func(_ *prospect.Context, p *prospect.Proof, _ *gymauth.Authorization) { p.Version++ }},
		{"gym", func(_ *prospect.Context, p *prospect.Proof, _ *gymauth.Authorization) {
			p.GymID = fixtures[0].ExpectedGymID
		}},
		{"wallet", func(_ *prospect.Context, _ *prospect.Proof, a *gymauth.Authorization) {
			a.PublicKey = fixtures[0].Authorization.PublicKey
		}},
		{"other signature", func(_ *prospect.Context, _ *prospect.Proof, a *gymauth.Authorization) {
			a.Signature = fixtures[0].Authorization.Signature
		}},
		{"header nonce", func(_ *prospect.Context, p *prospect.Proof, _ *gymauth.Authorization) {
			p.Header = p.Header[:158] + "01"
		}},
		{"coinbase byte", func(_ *prospect.Context, p *prospect.Proof, _ *gymauth.Authorization) {
			p.Coinbase = "03" + p.Coinbase[2:]
		}},
		{"coinbase length", func(_ *prospect.Context, p *prospect.Proof, _ *gymauth.Authorization) { p.Coinbase += "00" }},
		{"branch sibling", func(_ *prospect.Context, p *prospect.Proof, _ *gymauth.Authorization) {
			p.MerkleBranch[0] = strings.Repeat("b", 64)
		}},
		{"branch order", func(_ *prospect.Context, p *prospect.Proof, _ *gymauth.Authorization) {
			p.MerkleBranch[0], p.MerkleBranch[1] = p.MerkleBranch[1], p.MerkleBranch[0]
		}},
		{"branch count", func(_ *prospect.Context, p *prospect.Proof, _ *gymauth.Authorization) {
			p.MerkleBranch = p.MerkleBranch[:1]
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			context, proof, authorization := fixture.Context, fixture.Proof, fixture.Authorization
			proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
			mutation.edit(&context, &proof, &authorization)
			if err := gymauth.VerifyAuthorization(context, proof, authorization); err == nil {
				t.Fatal("changed authorization binding accepted")
			}
		})
	}
}

func TestReferenceCanonicalSignatures(t *testing.T) {
	fixture := referenceFixtures(t)[0]
	der := referenceHex(t, fixture.Authorization.Signature)
	// The independently generated first signature has an unpadded positive R.
	// Add a redundant sign byte while retaining the same mathematical scalar.
	if der[0] != 0x30 || der[2] != 0x02 || der[4] == 0 || der[4]&0x80 != 0 {
		t.Fatal("fixture no longer covers unnecessary DER sign padding")
	}
	redundantR := append([]byte{0x30, der[1] + 1, 0x02, der[3] + 1, 0x00}, der[4:]...)
	mutations := []struct {
		name string
		edit func(*gymauth.Authorization)
	}{
		{"version zero", func(a *gymauth.Authorization) { a.Version = 0 }},
		{"unknown version", func(a *gymauth.Authorization) { a.Version = 2 }},
		{"high S mathematical twin", func(a *gymauth.Authorization) { a.Signature = fixture.InvalidHighSSignature }},
		{"DER redundant R padding", func(a *gymauth.Authorization) { a.Signature = hex.EncodeToString(redundantR) }},
		{"DER trailing byte", func(a *gymauth.Authorization) { a.Signature += "00" }},
		{"DER zero R", func(a *gymauth.Authorization) { a.Signature = "3006020100020101" }},
		{"DER zero S", func(a *gymauth.Authorization) { a.Signature = "3006020101020100" }},
		{"DER negative R", func(a *gymauth.Authorization) { a.Signature = "3006020180020101" }},
		{"raw signature", func(a *gymauth.Authorization) { a.Signature = strings.Repeat("01", 64) }},
		{"empty signature", func(a *gymauth.Authorization) { a.Signature = "" }},
		{"odd signature hex", func(a *gymauth.Authorization) { a.Signature += "0" }},
		{"nonhex signature", func(a *gymauth.Authorization) { a.Signature = "zz" }},
		{"signature prefix", func(a *gymauth.Authorization) { a.Signature = "0x" + a.Signature }},
		{"short public key", func(a *gymauth.Authorization) { a.PublicKey = a.PublicKey[2:] }},
		{"uncompressed public key", func(a *gymauth.Authorization) {
			a.PublicKey = "04" + a.PublicKey[2:] + "483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8"
		}},
		{"hybrid public key", func(a *gymauth.Authorization) {
			a.PublicKey = "06" + a.PublicKey[2:] + "483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8"
		}},
		{"point outside field", func(a *gymauth.Authorization) { a.PublicKey = "02" + strings.Repeat("ff", 32) }},
		{"public key prefix", func(a *gymauth.Authorization) { a.PublicKey = "0x" + a.PublicKey }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			authorization := fixture.Authorization
			mutation.edit(&authorization)
			if err := gymauth.VerifyAuthorization(fixture.Context, fixture.Proof, authorization); err == nil {
				t.Fatal("noncanonical or invalid authorization accepted")
			}
		})
	}
}

func TestReferenceProofInputBounds(t *testing.T) {
	fixture := referenceFixtures(t)[0]
	mutations := []struct {
		name string
		edit func(*prospect.Proof)
	}{
		{"header short", func(p *prospect.Proof) { p.Header = p.Header[2:] }},
		{"header long", func(p *prospect.Proof) { p.Header += "00" }},
		{"header nonhex", func(p *prospect.Proof) { p.Header = strings.Repeat("z", 160) }},
		{"coinbase too large", func(p *prospect.Proof) { p.Coinbase = strings.Repeat("00", 16385) }},
		{"coinbase odd hex", func(p *prospect.Proof) { p.Coinbase += "0" }},
		{"coinbase nonhex", func(p *prospect.Proof) { p.Coinbase = "zz" }},
		{"branch too long", func(p *prospect.Proof) {
			p.MerkleBranch = make([]string, 21)
			for i := range p.MerkleBranch {
				p.MerkleBranch[i] = strings.Repeat("01", 32)
			}
		}},
		{"branch wrong size", func(p *prospect.Proof) { p.MerkleBranch = []string{"01"} }},
		{"branch nonhex", func(p *prospect.Proof) { p.MerkleBranch = []string{strings.Repeat("z", 64)} }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			proof := fixture.Proof
			mutation.edit(&proof)
			if _, err := gymauth.AdmissionDigest(fixture.Context, proof, fixture.Authorization.PublicKey); err == nil {
				t.Fatal("malformed or oversized proof entered the signing digest")
			}
			if err := gymauth.VerifyAuthorization(fixture.Context, proof, fixture.Authorization); err == nil {
				t.Fatal("malformed or oversized proof authorization accepted")
			}
		})
	}
	if _, err := gymauth.SignAdmission(fixture.Context, fixture.Proof, nil); err == nil {
		t.Fatal("nil signing key accepted")
	}
}
