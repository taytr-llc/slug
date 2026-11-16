package bridge

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
)

func TestAccountSnapshotRejectsCredentialAndAuthorityFields(t *testing.T) {
	s := &pilot.Snapshot{Mode: pilot.AccountMode, State: "stopped", WalletState: "closed", BackupState: "idle", Prospects: []pilot.Prospect{}, Account: &pilot.AccountSnapshot{State: "signed_out"}}
	raw, _ := json.Marshal(pilot.Response{ID: 1, OK: true, Snapshot: s})
	if _, err := decode(raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"token":"secret",`, `"mnemonic":"secret",`, `"verified":true,`, `"private_key":"secret",`} {
		bad := bytes.Replace(raw, []byte(`"account":{`), []byte(`"account":{`+field), 1)
		if _, err := decode(bad); err == nil {
			t.Fatal("private or unknown account field accepted")
		}
	}
	for _, mutate := range []func(*pilot.AccountSnapshot){
		func(a *pilot.AccountSnapshot) { a.State = "unrecognized" },
		func(a *pilot.AccountSnapshot) { a.Fixture = "production" },
		func(a *pilot.AccountSnapshot) { a.HoldUntilMS = -1 },
		func(a *pilot.AccountSnapshot) { a.CanExport = true },
	} {
		copy := *s.Account
		mutate(&copy)
		s.Account = &copy
		bad, _ := json.Marshal(pilot.Response{ID: 1, OK: true, Snapshot: s})
		if _, err := decode(bad); err == nil {
			t.Fatal("invalid account projection accepted")
		}
		s.Account = &pilot.AccountSnapshot{State: "signed_out"}
	}
}

func TestAccountProjectionCoherentIdentityAndHold(t *testing.T) {
	identity, err := gymaccount.FixtureIdentity("zero256")
	if err != nil {
		t.Fatal(err)
	}
	valid := func() *pilot.Snapshot {
		return &pilot.Snapshot{Mode: pilot.AccountMode, State: "stopped", WalletState: "closed", BackupState: "idle", Prospects: []pilot.Prospect{}, GymID: identity.GymID, GymPublicKey: identity.GymPublicKey, Account: &pilot.AccountSnapshot{State: "signed_in", Fixture: "zero256", ID: strings.Repeat("c", 64), Generation: 1, CanExport: true}}
	}
	for _, change := range []func(*pilot.Snapshot){
		func(s *pilot.Snapshot) { s.Account.ID = "" },
		func(s *pilot.Snapshot) { s.Account.Fixture = "" },
		func(s *pilot.Snapshot) { s.Account.Generation = 0 },
		func(s *pilot.Snapshot) { s.Account.Fixture = "incrementing256" },
		func(s *pilot.Snapshot) { s.GymPublicKey = "" },
		func(s *pilot.Snapshot) { s.GymID = strings.Repeat("f", 64) },
		func(s *pilot.Snapshot) { s.Account.Held = true; s.Account.HoldUntilMS = 1700000000000 },
		func(s *pilot.Snapshot) { s.Account.Held = true; s.Account.CanExport = false },
		func(s *pilot.Snapshot) { s.Account.HoldUntilMS = 8640000000000001 },
		func(s *pilot.Snapshot) { s.Account = &pilot.AccountSnapshot{State: "signed_out"} },
		func(s *pilot.Snapshot) {
			s.GymID = ""
			s.GymPublicKey = ""
			s.Account = &pilot.AccountSnapshot{State: "expired", Generation: 1}
		},
		func(s *pilot.Snapshot) {
			s.GymID = ""
			s.GymPublicKey = ""
			s.SpendableAtoms = 1
			s.Account = &pilot.AccountSnapshot{State: "signed_out"}
		},
	} {
		s := valid()
		change(s)
		raw, _ := json.Marshal(pilot.Response{ID: 1, OK: true, Snapshot: s})
		if _, err := decode(raw); err == nil {
			t.Fatal("contradictory or mixed account projection accepted")
		}
	}
	for _, name := range []string{"zero256", "incrementing256"} {
		s := valid()
		identity, _ := gymaccount.FixtureIdentity(name)
		s.Account.Fixture = name
		s.GymID = identity.GymID
		s.GymPublicKey = identity.GymPublicKey
		s.Account.Held = true
		s.Account.CanExport = false
		s.Account.HoldUntilMS = 1700000000000
		raw, _ := json.Marshal(pilot.Response{ID: 1, OK: true, Snapshot: s})
		if _, err := decode(raw); err != nil {
			t.Fatalf("coherent fixture projection rejected: %v", err)
		}
	}
}
