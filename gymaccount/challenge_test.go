package gymaccount

import (
	"math"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcutil/v2/hdkeychain"
	"github.com/taytr-llc/slug/chaincfg/v2"
)

func boundedChallenge() EnrollmentChallenge {
	return EnrollmentChallenge{Version: 1, AuthorityID: strings.Repeat("01", 32), AccountID: strings.Repeat("02", 32), PrincipalID: strings.Repeat("03", 32), ChallengeID: strings.Repeat("04", 32), Generation: 1, Action: "enroll", Network: FixtureNetwork, Genesis: FixtureGenesis, Profile: FixtureProfile, RootPublicKey: "JgduaQiYjRcnjcD9t1MfTP1qo8eWaAWPQtS9nChXwtAeyTsoEGVZ8SjVqa5qxxvvKqX8tqC77wMSjaMHXNY8JSYEASdzX6PT31ktXqKPJvkhyy31", GymPublicKey: "03ec7488667fd544b4a1de8fb0ae03730c9a2dc346c652cbaa59a077dbeddca519", GymID: "a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5", IssuedAtMS: 1, ExpiresAtMS: 300001}
}
func TestChallengeCanonicalBounds(t *testing.T) {
	if _, err := ChallengeDigest(boundedChallenge()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*EnrollmentChallenge)
	}{
		{"version", func(c *EnrollmentChallenge) { c.Version = 2 }},
		{"generation", func(c *EnrollmentChallenge) { c.Generation = 0 }},
		{"arbitrary-action", func(c *EnrollmentChallenge) { c.Action = "sign" }},
		{"network", func(c *EnrollmentChallenge) { c.Network = 0 }},
		{"genesis", func(c *EnrollmentChallenge) { c.Genesis = strings.Repeat("00", 32) }},
		{"profile", func(c *EnrollmentChallenge) { c.Profile = "production" }},
		{"short-id", func(c *EnrollmentChallenge) { c.AccountID = c.AccountID[2:] }},
		{"uppercase-id", func(c *EnrollmentChallenge) { c.PrincipalID = strings.Repeat("AA", 32) }},
		{"oversize-id", func(c *EnrollmentChallenge) { c.ChallengeID = strings.Repeat("0", 1<<20) }},
		{"missing-root", func(c *EnrollmentChallenge) { c.RootPublicKey = "" }},
		{"oversize-root", func(c *EnrollmentChallenge) { c.RootPublicKey = strings.Repeat("1", 1<<20) }},
		{"root-checksum", func(c *EnrollmentChallenge) { c.RootPublicKey += "1" }},
		{"uppercase-public", func(c *EnrollmentChallenge) { c.GymPublicKey = strings.ToUpper(c.GymPublicKey) }},
		{"gym-mismatch", func(c *EnrollmentChallenge) { c.GymID = strings.Repeat("00", 32) }},
		{"zero-issued", func(c *EnrollmentChallenge) { c.IssuedAtMS = 0 }},
		{"negative-issued", func(c *EnrollmentChallenge) { c.IssuedAtMS = -1 }},
		{"no-lifetime", func(c *EnrollmentChallenge) { c.ExpiresAtMS = c.IssuedAtMS }},
		{"excess-lifetime", func(c *EnrollmentChallenge) { c.ExpiresAtMS++ }},
		{"overflow-range", func(c *EnrollmentChallenge) { c.IssuedAtMS = 1; c.ExpiresAtMS = math.MaxInt64 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := boundedChallenge()
			tc.change(&c)
			if _, err := ChallengeDigest(c); err == nil {
				t.Fatal("invalid challenge accepted")
			}
		})
	}
	c := boundedChallenge()
	c.AuthorityID = strings.Repeat("00", 32)
	c.AccountID = c.AuthorityID
	c.PrincipalID = c.AuthorityID
	c.ChallengeID = c.AuthorityID
	c.Generation = math.MaxUint64
	c.IssuedAtMS = math.MaxInt64 - MaxChallengeLifetimeMS
	c.ExpiresAtMS = math.MaxInt64
	if _, err := ChallengeDigest(c); err != nil {
		t.Fatal("valid fixed-width identifiers/times rejected", err)
	}
}
func TestChallengeRejectsNonRootAndForeignExtendedKeys(t *testing.T) {
	c := boundedChallenge()
	root, err := hdkeychain.NewKeyFromString(c.RootPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Zero()
	child, err := root.Derive(0)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Zero()
	c.RootPublicKey = child.String()
	if _, err := ChallengeDigest(c); err == nil {
		t.Fatal("non-root key accepted")
	}
	foreign, err := root.CloneWithVersion([]byte{0x04, 0x88, 0xb2, 0x1e})
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Zero()
	c.RootPublicKey = foreign.String()
	if _, err := ChallengeDigest(c); err == nil {
		t.Fatal("Bitcoin root accepted")
	}
	private, err := hdkeychain.NewMaster(make([]byte, 32), &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	defer private.Zero()
	c.RootPublicKey = private.String()
	if _, err := ChallengeDigest(c); err == nil {
		t.Fatal("private root accepted")
	}
}
func TestPrincipalExactIdentityAndBounds(t *testing.T) {
	a, err := PrincipalID("ab", "c")
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"a", "bc"}, {"AB", "c"}, {"ab ", "c"}, {"ab", "c "}} {
		b, err := PrincipalID(pair[0], pair[1])
		if err != nil || b == a {
			t.Fatal("principal boundary or normalization collision")
		}
	}
	for _, pair := range [][2]string{{"", "s"}, {"i", ""}, {strings.Repeat("a", 1025), "s"}, {"i", strings.Repeat("b", 1025)}, {string([]byte{0xff}), "s"}} {
		if _, err := PrincipalID(pair[0], pair[1]); err == nil {
			t.Fatal("invalid principal accepted")
		}
	}
	if _, err := PrincipalID(strings.Repeat("i", 1024), strings.Repeat("s", 1024)); err != nil {
		t.Fatal(err)
	}
}
