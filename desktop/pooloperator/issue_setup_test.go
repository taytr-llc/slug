// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

type issueOperatorReference struct {
	Fixtures struct {
		Wrappers []struct {
			Name      string
			Value     json.RawMessage
			Canonical string `json:"canonical_hex"`
			ID        string
		}
	}
	Framing struct {
		Cases []struct {
			Name, Target string
			Raw          string `json:"raw_hex"`
			Valid        bool
		}
	}
}

func loadIssueOperatorReference(t *testing.T) issueOperatorReference {
	t.Helper()
	raw, e := os.ReadFile("../../testdata/issue-recovery/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if len(raw) != 6134980 || hex.EncodeToString(h[:]) != "4bbe4eb9556ed5a894cee6ff565b0dcca805f27fea2d77cfe3a56242dc3dac91" {
		t.Fatal("independent issue reference changed")
	}
	var out issueOperatorReference
	if json.Unmarshal(raw, &out) != nil || len(out.Fixtures.Wrappers) != 52 || len(out.Framing.Cases) != 418 {
		t.Fatal("issue reference inventory")
	}
	return out
}

func TestRecoverableOperatorIndependentWrappers(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal immutable source identities")
	}
	ref := loadIssueOperatorReference(t)
	values := map[string][]byte{}
	ids := map[string]string{}
	for _, r := range ref.Fixtures.Wrappers {
		values[r.Name] = provisionHex(t, r.Canonical)
		ids[r.Name] = r.ID
	}
	count := 0
	for _, old := range loadDailyOperatorReference(t).Composition.Positives {
		want, ok := values[old.Name+"-setup"]
		if !ok {
			continue
		}
		count++
		t.Run(old.Name, func(t *testing.T) {
			s, e := ComposeRecoverableDailySetup(old.Setup)
			if e != nil {
				t.Fatal(e)
			}
			raw, e := MarshalRecoverableDailySetup(s)
			if e != nil || !bytes.Equal(raw, want) {
				t.Fatal("new wrapper changed inherited Setup2", e)
			}
			parsed, e := ParseRecoverableDailySetup(want)
			if e != nil || !reflect.DeepEqual(s, parsed) {
				t.Fatal("selected parse", e)
			}
			id, e := RecoverableDailySetupID(s)
			if e != nil || id != ids[old.Name+"-setup"] {
				t.Fatal("selected setup domain", e)
			}
			if _, e = ParseDailySetup(want); e == nil {
				t.Fatal("old parser accepted new wrapper")
			}
			if _, e = ParseRecoverableDailySetup(provisionHex(t, old.SetupHex)); e == nil {
				t.Fatal("new parser adopted old Setup")
			}
			if !bytes.Equal(append(append([]byte(nil), raw...), '\n'), values[old.Name+"-operator-intent"]) {
				t.Fatal("operator intent")
			}
			ready, e := recoverableReadyBytes(s)
			if e != nil || !bytes.Equal(ready, values[old.Name+"-operator-ready"]) {
				t.Fatal("operator READY", e)
			}
			var intent provisionIntent
			if provisionJSONFile(provisionHex(t, old.ProvisionIntent), 131072, &intent) != nil {
				t.Fatal("old intent")
			}
			got, e := issueProvisionIntentBytes(intent)
			if e != nil || !bytes.Equal(got, values[old.Name+"-provision-intent"]) {
				t.Fatal("provision intent", e)
			}
			material := dailyProvisionMaterial(t, provisionReference(t))
			if _, e = parseIssueProvisionIntent(got, old.Policy, material.Certificate); e != nil {
				t.Fatal("parse selected intent", e)
			}
			if _, e = parseProvisionIntent(got, old.Policy, material.Certificate); e == nil {
				t.Fatal("old intent parser adopted selected wrapper")
			}
			var pr provisionReady
			if provisionJSONFile(provisionHex(t, old.ProvisionReady), 1024, &pr) != nil {
				t.Fatal("old READY")
			}
			got, e = issueProvisionReadyBytes(pr, s)
			if e != nil || !bytes.Equal(got, values[old.Name+"-provision-ready"]) {
				t.Fatal("provision READY", e)
			}
			got, e = issueProvisionBackupBytes(provisionHex(t, old.Backup), s)
			if e != nil || !bytes.Equal(got, values[old.Name+"-published-fixture-backup"]) {
				t.Fatal("original published fixture backup wrapper", e)
			}
			// Complete outer profile agreement, rather than shape-only acceptance.
			parsed.IssueProfile.BaseSetupID = parsed.IssueProfile.SourceConfigID
			if ValidateRecoverableDailySetup(parsed) == nil {
				t.Fatal("wrong complete Setup binding")
			}
		})
	}
	if count != 3 {
		t.Fatal("selected composition count", count)
	}
}

func TestRecoverableOperatorIndependentSetupFraming(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal immutable source identities")
	}
	count := 0
	for _, r := range loadIssueOperatorReference(t).Framing.Cases {
		if r.Target != "one-enrollment-setup" {
			continue
		}
		count++
		t.Run(r.Name, func(t *testing.T) {
			_, e := ParseRecoverableDailySetup(provisionHex(t, r.Raw))
			if (e == nil) != r.Valid {
				t.Fatal("independent framing classification", e)
			}
		})
	}
	if count == 0 {
		t.Fatal("missing setup cases")
	}
}

func TestRecoverableOperatorCommandExplicitSelection(t *testing.T) {
	args := dailyCommandArguments(filepath.Join(t.TempDir(), "absent"))
	base, e := parseCommand(args)
	if e != nil {
		t.Fatal(e)
	}
	for _, flag := range []string{"--daily-recoverable-issue", "--daily-recoverable-issue=true"} {
		got, e := parseCommand(append(append([]string{}, args...), flag))
		if e != nil || !got.recoverableIssue || got.daily {
			t.Fatal("selected operator", e)
		}
		got.recoverableIssue = false
		if !reflect.DeepEqual(got, base) {
			t.Fatal("selector changed original inputs")
		}
	}
	p := []string{"verify", "--policy-file", "/fixture/policy", "--root", "/fixture/root", "--tls-certificate-file", "/fixture/cert", "--wallet-public-passphrase-file", "/fixture/pub", "--wallet-unlock-passphrase-file", "/fixture/unlock"}
	if got, e := parseProvisionCommand(append(append([]string{}, p...), "--daily-recoverable-issue")); e != nil || !got.recoverableIssue || got.daily {
		t.Fatal("selected provision", e)
	}
	for _, extra := range [][]string{
		{"--daily-recoverable-issue=false"}, {"--daily-recoverable-issue=TRUE"},
		{"--daily-recoverable-issue", "--daily-recoverable-issue"},
		{"--daily", "--daily-recoverable-issue"}, {"--daily=false", "--daily-recoverable-issue"},
		{"--daily-recoverable-issue=false", "--daily"}, {"--daily-recoverable-issue", "true"},
	} {
		if _, e := parseCommand(append(append([]string{}, args...), extra...)); !errors.Is(e, ErrCommandArguments) {
			t.Fatal("ambiguous operator mode", extra, e)
		}
		if _, e := parseProvisionCommand(append(append([]string{}, p...), extra...)); !errors.Is(e, ErrCommandArguments) {
			t.Fatal("ambiguous provision mode", extra, e)
		}
	}
}
