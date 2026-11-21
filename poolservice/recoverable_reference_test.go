// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"strings"
	"testing"
)

type recoveryCase struct {
	Name           string
	Valid          bool
	Value          json.RawMessage
	Canonical      string `json:"canonical_hex"`
	Profile        registry.IssueProfile
	Request        poolsettle.DailyPayoutRequest
	HTTPStatus     int  `json:"http_status"`
	StatusAsserted bool `json:"http_status_asserted"`
}
type recoveryReference struct {
	Fixtures struct {
		Conditional []struct {
			Name   string
			Client RecoverableDailyClientConfig
			Trust  string `json:"trust_id"`
		} `json:"conditional_transport"`
		Matching []struct {
			Name      string
			Profile   registry.IssueProfile
			RequestID string `json:"request_id"`
			Setup     struct{ Pool poolshare.Config }
			Work      poolshare.MiningWork
		} `json:"matching_families"`
		Remote []struct {
			Config struct {
				IssueProfile registry.IssueProfile `json:"issue_profile"`
			}
		} `json:"remote"`
		Wrappers []recoveryCase `json:"wrappers"`
	} `json:"fixtures"`
	States struct {
		Responses []recoveryCase `json:"response_cases"`
	} `json:"states"`
	Wire struct {
		Responses []recoveryCase `json:"response_cases"`
		Errors    []recoveryCase `json:"error_cases"`
	} `json:"wire"`
	Framing struct {
		Cases []struct {
			Name, Target string
			Valid        bool
			RawHex       string `json:"raw_hex"`
			RawBytes     int    `json:"raw_bytes"`
			RawSHA       string `json:"raw_sha256"`
			Recipe       *struct {
				Prefix string `json:"prefix_byte_hex"`
				Repeat int
				Suffix string `json:"suffix_hex"`
			} `json:"raw_recipe"`
		}
	} `json:"framing"`
}

func recoveryReferenceRead(t *testing.T) recoveryReference {
	t.Helper()
	raw, e := os.ReadFile("../testdata/issue-recovery/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if len(raw) != 6134980 || hex.EncodeToString(sum[:]) != "4bbe4eb9556ed5a894cee6ff565b0dcca805f27fea2d77cfe3a56242dc3dac91" {
		t.Fatal("independent issue reference changed")
	}
	var ref recoveryReference
	if json.Unmarshal(raw, &ref) != nil {
		t.Fatal("reference decode")
	}
	if len(ref.Fixtures.Conditional) != 3 || len(ref.Fixtures.Matching) != 3 || len(ref.States.Responses) != 20 || len(ref.Wire.Responses) != 120 || len(ref.Wire.Errors) != 8 || len(ref.Framing.Cases) != 418 {
		t.Fatal("reference inventory")
	}
	return ref
}
func recoveryRaw(t *testing.T, c recoveryCase) []byte {
	t.Helper()
	raw, e := hex.DecodeString(c.Canonical)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func recoveryClient(t *testing.T, c RecoverableDailyClientConfig) *Client {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal source and client; cheap refusal separately")
	}
	owner, e := NewRecoverableDailyClient(c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(owner.Close)
	return owner
}
func recoveryResponseClient(t *testing.T, ref recoveryReference) *Client {
	t.Helper()
	var r IssueResponse
	if json.Unmarshal(ref.States.Responses[0].Value, &r) != nil {
		t.Fatal("response fixture")
	}
	f := ref.Fixtures.Matching[0]
	return recoveryClient(t, RecoverableDailyClientConfig{Version: 1, IssueProfile: f.Profile, Base: DailyClientConfig{Version: 2, URL: "https://pool.example", Token: strings.Repeat("1", 64), PublicKey: r.PublicKey, Source: f.Setup.Pool, SettlementConfigID: r.SettlementConfigID, RecipientScript: r.RecipientScript}})
}
func TestRecoverableTransportIndependentBindings(t *testing.T) {
	ref := recoveryReferenceRead(t)
	if wire.TestPoW {
		t.Skip("normal selected source")
	}
	for _, row := range ref.Fixtures.Conditional {
		t.Run(row.Name, func(t *testing.T) {
			id, e := RecoverableDailyTransportTrustID(row.Client)
			if e != nil || id != row.Trust {
				t.Fatalf("trust %s %v", id, e)
			}
			owner := recoveryClient(t, row.Client)
			if owner.ProtocolVersion() != 3 || owner.CheckRecoverableDailySelection(owner.recoverable.selection) != nil {
				t.Fatal("selection")
			}
			changed := row.Client
			changed.Base.Token = strings.Repeat("c", 64)
			id, e = RecoverableDailyTransportTrustID(changed)
			if e != nil || id != row.Trust {
				t.Fatal("token changed selected trust")
			}
			changed = row.Client
			changed.IssueProfile.BaseSetupID = strings.Repeat("e", 64)
			id, e = RecoverableDailyTransportTrustID(changed)
			if e != nil || id == row.Trust {
				t.Fatal("profile omitted")
			}
			raw, e := MarshalRecoverableDailyClientConfig(row.Client)
			if e != nil {
				t.Fatal(e)
			}
			decoded, e := ParseRecoverableDailyClientConfig(raw)
			if e != nil || decoded != row.Client {
				t.Fatal("canonical config")
			}
			if _, e = ParseRecoverableDailyClientConfig(append(raw, '\n')); e == nil {
				t.Fatal("config framing relaxed")
			}
			for _, field := range []string{"source", "profile", "recipient", "trust", "base-trust", "url", "key"} {
				s := owner.recoverable.selection
				switch field {
				case "source":
					s.Source.MaxWorks--
				case "profile":
					s.IssueProfile.BaseSetupID = strings.Repeat("e", 64)
				case "recipient":
					s.RecipientScript = "0014" + strings.Repeat("e", 40)
				case "trust":
					s.TransportTrustID = strings.Repeat("e", 64)
				case "base-trust":
					s.BaseTransportTrustID = strings.Repeat("e", 64)
				case "url":
					s.URL = "https://other.example"
				case "key":
					s.PublicKey = "02" + strings.Repeat("e", 64)
				}
				if owner.CheckRecoverableDailySelection(s) == nil {
					t.Fatal("changed selection", field)
				}
			}
		})
	}
	for _, row := range ref.Fixtures.Wrappers {
		if !strings.HasSuffix(row.Name, "-client") {
			continue
		}
		var c RecoverableDailyClientConfig
		if json.Unmarshal(row.Value, &c) != nil {
			t.Fatal("wrapper")
		}
		raw, e := MarshalRecoverableDailyClientConfig(c)
		if e != nil || !bytes.Equal(raw, recoveryRaw(t, row)) {
			t.Fatal("independent wrapper bytes", row.Name, e)
		}
	}
}
func TestRecoverableIssueIndependentResults(t *testing.T) {
	ref := recoveryReferenceRead(t)
	owner := recoveryResponseClient(t, ref)
	id := ref.Fixtures.Matching[0].RequestID
	for _, row := range ref.States.Responses {
		t.Run(row.Name, func(t *testing.T) {
			var r IssueResponse
			good := decodeRecoverableIssue(recoveryRaw(t, row), &r) == nil
			if good {
				good = ValidateRecoverableIssueResponse(owner.recoverable.selection, id, r, strings.HasPrefix(row.Name, "lookup-")) == nil
			}
			if good != row.Valid {
				t.Fatalf("valid=%v want=%v", good, row.Valid)
			}
			if owner.CheckIssueResponse(id, r) == nil {
				t.Fatal("public bytes manufactured client provenance")
			}
		})
	}
}
func TestRecoverableTransportIndependentMoneyAndWork(t *testing.T) {
	ref := recoveryReferenceRead(t)
	if wire.TestPoW {
		t.Skip("normal source selection")
	}
	owners := map[string]*Client{}
	for _, f := range ref.Fixtures.Conditional {
		c := recoveryClient(t, f.Client)
		owners[f.Client.IssueProfile.SourceConfigID] = c
	}
	old := readDailyReference(t)
	remote := recoveryClient(t, RecoverableDailyClientConfig{1, old.Transport.Remote[0].Client, ref.Fixtures.Remote[0].Config.IssueProfile})
	owners[remote.binding] = remote
	for _, row := range ref.Wire.Responses {
		t.Run(row.Name, func(t *testing.T) {
			// The independently modeled wrong-profile rows intentionally carry
			// no copied Profile metadata. Select their original client by the
			// unchanged base binding, never by the corrupted issue_profile_id.
			sourceID := row.Profile.SourceConfigID
			if sourceID == "" {
				var binding struct {
					BindingID string `json:"binding_id"`
				}
				if row.Valid || !strings.Contains(row.Name, "-wrong-profile-") || json.Unmarshal(row.Value, &binding) != nil {
					t.Fatal("missing original case selection")
				}
				sourceID = binding.BindingID
			}
			c := owners[sourceID]
			if c == nil {
				t.Fatal("missing selected client")
			}
			raw := recoveryRaw(t, row)
			good := false
			switch {
			case strings.HasPrefix(row.Name, "balance-"):
				var r DailyBalanceResponse
				good = c.decodeSelectedBalance(raw, &r) == nil && poolsettle.ValidateDailyBalance(r.Balance, c.gym, r.ObservedAtMS) == nil
			case strings.HasPrefix(row.Name, "page-"):
				var r poolsettle.DailyPayoutPage
				good = c.decodeSelectedPage(raw, &r) == nil
				if good {
					_, e := poolsettle.ValidateDailyPayoutPage(r, c.gym)
					good = e == nil
				}
				if good {
					q := row.Request
					if q.Revision == 0 {
						good = r.BeforeBatch == r.HighBatch+1
					} else {
						good = r.Revision == q.Revision && r.HighBatch == q.HighBatch && r.BeforeBatch == q.BeforeBatch
					}
				}
			case bytes.Contains(row.Value, []byte(`"work":`)):
				var r DailyWorkResponse
				good = c.decodeSelectedWorkResponse(raw, &r) == nil
				if good {
					_, e := PrepareDailyWork(c.config.Source, c.config.PublicKey, r.Work)
					good = e == nil
				}
			default:
				var r DailySubmissionResponse
				good = c.decodeSelectedSubmissionResponse(raw, &r) == nil
			}
			if good != row.Valid {
				t.Fatalf("conditional envelope validity %v want%v", good, row.Valid)
			}
		})
	}
	for _, row := range ref.Wire.Errors {
		var r ErrorResponse
		good := decodeDailyError(recoveryRaw(t, row), &r) == nil && r.Version == 3 && dailyErrorStatus(r.Error.Code) != 0 && r.Error.OutcomeUnknown == (r.Error.Code == "outcome_unknown")
		if row.StatusAsserted {
			good = good && dailyErrorStatus(r.Error.Code) == row.HTTPStatus
		}
		if good != row.Valid {
			t.Fatal("error frame", row.Name, good, row.Valid)
		}
	}
}
func TestRecoverableTransportIndependentFraming(t *testing.T) {
	ref := recoveryReferenceRead(t)
	count := 0
	for _, row := range ref.Framing.Cases {
		if row.Target != "request" && !strings.HasPrefix(row.Target, "lookup-") && !strings.HasPrefix(row.Target, "existing-route-") && row.Target != "error-envelope" && row.Target != "coinbase-only-client" {
			continue
		}
		count++
		t.Run(row.Name, func(t *testing.T) {
			raw, e := hex.DecodeString(row.RawHex)
			if e != nil {
				t.Fatal(e)
			}
			if row.Recipe != nil {
				p, e := hex.DecodeString(row.Recipe.Prefix)
				if e != nil || len(p) != 1 || row.Recipe.Repeat < 0 || row.Recipe.Repeat > MaxWorkResponseSize+1 {
					t.Fatal("bounded recipe")
				}
				tail, e := hex.DecodeString(row.Recipe.Suffix)
				if e != nil {
					t.Fatal(e)
				}
				raw = append(bytes.Repeat(p, row.Recipe.Repeat), tail...)
			}
			sum := sha256.Sum256(raw)
			if len(raw) != row.RawBytes || hex.EncodeToString(sum[:]) != row.RawSHA {
				t.Fatal("raw identity")
			}
			switch {
			case row.Target == "request":
				e = decodeDailyStrict(raw, MaxRequestSize, &issueRequest{})
			case strings.HasPrefix(row.Target, "lookup-"):
				e = decodeRecoverableIssue(raw, &IssueResponse{})
			case row.Target == "existing-route-balance":
				e = decodeRecoverableBalance(raw, &RecoverableBalanceResponse{})
			case row.Target == "existing-route-page":
				e = decodeRecoverablePage(raw, &RecoverablePayoutPage{})
			case row.Target == "existing-route-work-receipt":
				e = decodeDailyStrict(raw, MaxWorkResponseSize, &RecoverableWorkResponse{})
			case row.Target == "error-envelope":
				e = decodeDailyError(raw, &ErrorResponse{})
			case row.Target == "coinbase-only-client":
				if wire.TestPoW {
					return
				}
				_, e = ParseRecoverableDailyClientConfig(raw)
			}
			if (e == nil) != row.Valid {
				t.Fatalf("framing %v want%v: %v", e == nil, row.Valid, e)
			}
		})
	}
	if count != 128 {
		t.Fatalf("selected framing inventory %d", count)
	}
}
