// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
)

type dailyOperatorVector struct {
	Name            string
	Policy          ProvisionPolicy
	Setup           Setup
	PolicyID        string            `json:"policy_id"`
	SetupID         string            `json:"setup_id"`
	NamespaceIDs    map[string]string `json:"namespace_ids"`
	PolicyHex       string            `json:"policy_hex"`
	SetupHex        string            `json:"setup_hex"`
	OperatorIntent  string            `json:"operator_intent_hex"`
	OperatorReady   string            `json:"operator_ready_hex"`
	ProvisionIntent string            `json:"provision_intent_hex"`
	ProvisionReady  string            `json:"provision_ready_hex"`
	Backup          string            `json:"published_fixture_backup_hex"`
}
type dailyOperatorReference struct {
	Composition struct {
		Positives   []dailyOperatorVector
		PolicyCases []struct {
			Name      string
			Valid     bool
			Canonical string `json:"canonical_hex"`
		} `json:"policy_cases"`
		Refusals []struct {
			Name   string
			Policy ProvisionPolicy
		} `json:"composition_refusals"`
	}
	Raw []struct {
		Name, Family string
		Valid        bool
		Raw          string `json:"raw_hex"`
	} `json:"raw_cases"`
}

func loadDailyOperatorReference(t *testing.T) dailyOperatorReference {
	t.Helper()
	raw, e := os.ReadFile("../../testdata/public-daily/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if len(raw) != 3176583 || hex.EncodeToString(sum[:]) != "a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3" {
		t.Fatal("public daily reference changed")
	}
	var v dailyOperatorReference
	if json.Unmarshal(raw, &v) != nil || len(v.Composition.Positives) != 5 || len(v.Composition.PolicyCases) != 19 || len(v.Raw) != 143 {
		t.Fatal("reference inventory")
	}
	return v
}

func dailyProvisionMaterial(t *testing.T, refs provisionCorpus) provisionVector {
	t.Helper()
	// The frozen public DAILY recipe selects this original wallet, not the
	// provision corpus's first entry. Keep its material, certificate and entropy
	// identity together when composing bytes or exercising real provisioning.
	const name = "official-leading-zero-account"
	var selected provisionVector
	matches := 0
	for _, v := range refs.Positives {
		if v.Name == name {
			selected = v
			matches++
		}
	}
	if matches != 1 {
		t.Fatal("independent daily provision material selection", name, matches)
	}
	return selected
}

func TestDailyOperatorIndependentComposition(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal immutable chain/profile identities")
	}
	refs := loadDailyOperatorReference(t)
	old := dailyProvisionMaterial(t, provisionReference(t))
	for _, v := range refs.Composition.Positives {
		t.Run(v.Name, func(t *testing.T) {
			p, e := ParseDailyProvisionPolicy(provisionHex(t, v.PolicyHex))
			if e != nil || !reflect.DeepEqual(p, v.Policy) {
				t.Fatal("Policy2", e)
			}
			if _, e = ParseProvisionPolicy(provisionHex(t, v.PolicyHex)); e == nil {
				t.Fatal("legacy Policy accepted daily")
			}
			pid, e := DailyProvisionPolicyID(p)
			if e != nil || pid != v.PolicyID {
				t.Fatal("Policy2 ID", e)
			}
			for role, want := range v.NamespaceIDs {
				if provisionNamespaceVersion(p.RequestID, role, 2) != want {
					t.Fatal("namespace", role)
				}
			}
			s, e := ComposeDailyProvisionSetup(p, old.Material, old.Certificate)
			if e != nil || !reflect.DeepEqual(s, v.Setup) {
				t.Fatal("composed Setup2", e)
			}
			raw, e := MarshalDailySetup(s)
			if e != nil || !bytes.Equal(raw, provisionHex(t, v.SetupHex)) {
				t.Fatal("Setup2 bytes", e)
			}
			copy, e := ParseDailySetup(raw)
			if e != nil || !reflect.DeepEqual(copy, s) {
				t.Fatal("Setup2 parse", e)
			}
			id, e := DailySetupID(s)
			if e != nil || id != v.SetupID {
				t.Fatal("Setup2 ID", e)
			}
			if _, e = ParseSetup(raw); e == nil {
				t.Fatal("legacy Setup accepted daily")
			}
			ready, e := readyBytesVersion(s, 2)
			if e != nil || !bytes.Equal(ready, provisionHex(t, v.OperatorReady)) {
				t.Fatal("operator READY2", e)
			}
			intent, e := parseProvisionIntent(provisionHex(t, v.ProvisionIntent), p, old.Certificate)
			if e != nil || intent.Version != 2 {
				t.Fatal("provision INTENT2", e)
			}
			pr, e := makeProvisionReady(p, s, old.Material.CreatedAtMS)
			if e != nil {
				t.Fatal(e)
			}
			prRaw, e := provisionJSON(pr, 1024)
			if e != nil || !bytes.Equal(prRaw, provisionHex(t, v.ProvisionReady)) {
				t.Fatal("provision READY2", e)
			}
			entropy, e := recovery.Entropy(old.Backup.Mnemonic)
			if e != nil {
				t.Fatal(e)
			}
			backup, e := provisionBackupBytes(entropy, p, s, old.Material.CreatedAtMS)
			clear(entropy[:])
			defer clear(backup)
			if e != nil || !bytes.Equal(backup, provisionHex(t, v.Backup)) {
				t.Fatal("daily original BIP39 backup", e)
			}
			bound, e := DailyCapacityEnvelope(s)
			if e != nil || bound.OrdinaryMaximum != 2 || bound.Reservation.Submissions != 3 || bound.Reservation.Blocks != 3 {
				t.Fatal("derived ordinary/recovery headroom", bound, e)
			}
			copy.Miners.Enrollments[0].GymID = "changed"
			if ValidateDailySetup(copy) == nil {
				t.Fatal("unbound enrollment")
			}
		})
	}
	for _, x := range refs.Composition.PolicyCases {
		t.Run(x.Name, func(t *testing.T) {
			_, e := ParseDailyProvisionPolicy(provisionHex(t, x.Canonical))
			if (e == nil) != x.Valid {
				t.Fatal("policy classification", e)
			}
		})
	}
	for _, x := range refs.Composition.Refusals {
		t.Run(x.Name, func(t *testing.T) {
			if _, e := ComposeDailyProvisionSetup(x.Policy, old.Material, old.Certificate); e == nil {
				t.Fatal("composition refusal")
			}
		})
	}
}
func TestDailyOperatorIndependentFraming(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal immutable chain/profile identities")
	}
	count := 0
	for _, x := range loadDailyOperatorReference(t).Raw {
		if x.Family != "policy" && x.Family != "setup" {
			continue
		}
		count++
		t.Run(x.Name, func(t *testing.T) {
			var e error
			if x.Family == "policy" {
				_, e = ParseDailyProvisionPolicy(provisionHex(t, x.Raw))
			} else {
				_, e = ParseDailySetup(provisionHex(t, x.Raw))
			}
			if (e == nil) != x.Valid {
				t.Fatal("framing classification", e)
			}
		})
	}
	if count == 0 {
		t.Fatal("no selected raw cases")
	}
}
