// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

type dailyReferenceCase struct {
	Name       string                                               `json:"name"`
	Valid      bool                                                 `json:"valid"`
	Value      json.RawMessage                                      `json:"value"`
	Canonical  string                                               `json:"canonical_hex"`
	Request    poolsettle.DailyPayoutRequest                        `json:"request"`
	HTTPStatus int                                                  `json:"http_status"`
	Expected   struct{ Binding, Settlement, Recipient, Gym string } `json:"-"`
}
type dailyReference struct {
	Transport struct {
		Wire []struct {
			Name   string
			Client DailyClientConfig
			Trust  string `json:"trust_id"`
		} `json:"wire_identities"`
		Remote []struct {
			Client DailyClientConfig `json:"daily_client"`
		} `json:"remote_positives"`
		Envelopes []dailyReferenceCase `json:"envelope_cases"`
		Errors    []dailyReferenceCase `json:"error_cases"`
	} `json:"transport"`
	Payments struct{ Balances, Pages, Requests []dailyReferenceCase } `json:"payments"`
	Raw      []struct {
		Name, Family string
		Valid        bool
		Hex          string `json:"raw_hex"`
		Recipe       *struct {
			Prefix  string `json:"prefix_hex"`
			Padding string `json:"padding_hex"`
			Count   int    `json:"padding_count"`
		} `json:"raw_recipe"`
	} `json:"raw_cases"`
}

func readDailyReference(t *testing.T) dailyReference {
	t.Helper()
	raw, err := os.ReadFile("../testdata/public-daily/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3" {
		t.Fatal("independent reference changed")
	}
	var c dailyReference
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Transport.Wire) != 3 || len(c.Transport.Envelopes) != 14 || len(c.Transport.Errors) != 8 || len(c.Payments.Balances) != 41 || len(c.Payments.Pages) != 19 || len(c.Payments.Requests) != 6 || len(c.Raw) != 143 {
		t.Fatal("reference inventory")
	}
	return c
}
func dailyReferenceRaw(t *testing.T, c dailyReferenceCase) []byte {
	t.Helper()
	raw, err := hex.DecodeString(c.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func dailyReferenceClient(t *testing.T, c DailyClientConfig) *Client {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal source identity; cheap refusal separately")
	}
	v, err := NewDailyClient(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v
}
func dailyHTTPRaw(raw []byte, status int) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw))}
}

func TestDailyTransportIndependentBindings(t *testing.T) {
	c := readDailyReference(t)
	if wire.TestPoW {
		t.Skip("normal source policy")
	}
	for _, row := range c.Transport.Wire {
		t.Run(row.Name, func(t *testing.T) {
			got, err := DailyTransportTrustID(row.Client)
			if err != nil || got != row.Trust {
				t.Fatal("independent trust", got, err)
			}
			changed := row.Client
			changed.Token = strings.Repeat("cc", 32)
			again, err := DailyTransportTrustID(changed)
			if err != nil || again != got {
				t.Fatal("token changed public identity")
			}
			changed.RecipientScript = "0014" + strings.Repeat("ed", 20)
			again, err = DailyTransportTrustID(changed)
			if err != nil || again == got {
				t.Fatal("recipient omitted from trust")
			}
			if _, err = ValidateClientConfig(row.Client.base()); err == nil {
				t.Fatal("legacy client accepted daily")
			}
			if _, err = BindingID(row.Client.Source); err == nil {
				t.Fatal("legacy source accepted daily")
			}
			changed = row.Client
			changed.Version = Version1
			if _, err = ValidateDailyClientConfig(changed); err == nil {
				t.Fatal("daily accepted legacy")
			}
		})
	}
	for _, row := range c.Transport.Envelopes {
		t.Run(row.Name, func(t *testing.T) {
			index := 0
			for i, w := range c.Transport.Wire {
				if strings.HasPrefix(row.Name, w.Name) {
					index = i
				}
			}
			owner := dailyReferenceClient(t, c.Transport.Wire[index].Client)
			raw := dailyReferenceRaw(t, row)
			var work DailyWorkResponse
			var receipt DailySubmissionResponse
			good := false
			if bytes.Contains(row.Value, []byte(`"work":`)) {
				err := decodeDailyStrict(raw, MaxWorkResponseSize, &work)
				good = err == nil && owner.dailyBinding(work.Version, work.BindingID, work.SettlementConfigID, work.RecipientScript)
				if good {
					_, err = PrepareDailyWork(owner.SourceConfig(), owner.PublicKey(), work.Work)
					good = err == nil
				}
			} else {
				err := decodeDailyStrict(raw, MaxResponseSize, &receipt)
				good = err == nil && owner.dailyBinding(receipt.Version, receipt.BindingID, receipt.SettlementConfigID, receipt.RecipientScript)
			}
			if good != row.Valid {
				t.Fatalf("binding validity %v != %v", good, row.Valid)
			}
		})
	}
}
func TestDailyTransportIndependentMoneyPages(t *testing.T) {
	ref := readDailyReference(t)
	c := ref.Transport.Remote[0].Client
	// Conditional byte checks use no client/network/source authority.
	var zero DailyBalanceResponse
	if json.Unmarshal(ref.Payments.Balances[0].Value, &zero) != nil {
		t.Fatal("empty balance")
	}
	owner := &Client{binding: zero.BindingID, gym: zero.Balance.GymID, daily: &dailyClientBinding{c.SettlementConfigID, c.RecipientScript}}
	for _, row := range ref.Payments.Balances {
		t.Run("balance/"+row.Name, func(t *testing.T) {
			var r DailyBalanceResponse
			good := decodeDailyBalance(dailyReferenceRaw(t, row), &r) == nil && owner.dailyBinding(r.Version, r.BindingID, r.SettlementConfigID, r.RecipientScript) && poolsettle.ValidateDailyBalance(r.Balance, owner.gym, r.ObservedAtMS) == nil
			if good != row.Valid {
				t.Fatalf("valid=%v want%v", good, row.Valid)
			}
		})
	}
	for _, row := range ref.Payments.Pages {
		t.Run("page/"+row.Name, func(t *testing.T) {
			var r poolsettle.DailyPayoutPage
			good := decodeDailyPage(dailyReferenceRaw(t, row), &r) == nil && owner.dailyBinding(r.Version, r.BindingID, r.SettlementConfigID, r.RecipientScript)
			if good {
				_, err := poolsettle.ValidateDailyPayoutPage(r, owner.gym)
				good = err == nil
			}
			if good {
				q := row.Request
				if q.Revision == 0 {
					good = r.BeforeBatch == r.HighBatch+1
				} else {
					good = r.Revision == q.Revision && r.HighBatch == q.HighBatch && r.BeforeBatch == q.BeforeBatch
				}
			}
			if good != row.Valid {
				t.Fatalf("valid=%v want%v", good, row.Valid)
			}
		})
	}
	for _, row := range ref.Payments.Requests {
		t.Run("request/"+row.Name, func(t *testing.T) {
			var q poolsettle.DailyPayoutRequest
			good := decodeDailyPageRequest(dailyReferenceRaw(t, row), &q) == nil && poolsettle.ValidateDailyPayoutRequest(q, poolsettle.MaxDailyPayoutBatches) == nil
			if good != row.Valid {
				t.Fatalf("valid=%v want%v", good, row.Valid)
			}
		})
	}
}
func TestDailyTransportIndependentFraming(t *testing.T) {
	ref := readDailyReference(t)
	count := 0
	for _, row := range ref.Raw {
		switch row.Family {
		case "work-response", "receipt-response", "error-response", "balance-response", "page-response", "page-request":
		default:
			continue
		}
		count++
		t.Run(row.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(row.Hex)
			if err != nil {
				t.Fatal(err)
			}
			if row.Recipe != nil {
				raw, err = hex.DecodeString(row.Recipe.Prefix)
				if err != nil {
					t.Fatal(err)
				}
				padding, err := hex.DecodeString(row.Recipe.Padding)
				if err != nil || len(padding) != 1 || row.Recipe.Count > MaxWorkResponseSize+1 {
					t.Fatal("bounded recipe")
				}
				raw = append(raw, bytes.Repeat(padding, row.Recipe.Count)...)
			}
			switch row.Family {
			case "work-response":
				err = decodeDailyStrict(raw, MaxWorkResponseSize, &DailyWorkResponse{})
			case "receipt-response":
				err = decodeDailyStrict(raw, MaxResponseSize, &DailySubmissionResponse{})
			case "error-response":
				err = decodeDailyError(raw, &ErrorResponse{})
			case "balance-response":
				err = decodeDailyBalance(raw, &DailyBalanceResponse{})
			case "page-response":
				err = decodeDailyPage(raw, &poolsettle.DailyPayoutPage{})
			case "page-request":
				err = decodeDailyPageRequest(raw, &poolsettle.DailyPayoutRequest{})
			}
			if (err == nil) != row.Valid {
				t.Fatalf("framing accepted=%v want%v", err == nil, row.Valid)
			}
		})
	}
	if count != 72 {
		t.Fatal("HTTP vector count", count)
	}
	for _, row := range ref.Transport.Errors {
		t.Run("error/"+row.Name, func(t *testing.T) {
			var e ErrorResponse
			good := decodeDailyError(dailyReferenceRaw(t, row), &e) == nil && e.Version == Version2 && dailyErrorStatus(e.Error.Code) != 0 && e.Error.OutcomeUnknown == (e.Error.Code == "outcome_unknown")
			if good && row.HTTPStatus != 0 {
				good = dailyErrorStatus(e.Error.Code) == row.HTTPStatus
			}
			if good != row.Valid {
				t.Fatal("error case", good, row.Valid)
			}
		})
	}
	for _, prefix := range []string{"\u00a0", "\v", "\f"} {
		if decodeDailyStrict([]byte(prefix+"{}"), MaxRequestSize, &struct{}{}) == nil {
			t.Fatal("non-ASCII framing")
		}
	}
}
func TestDailyClientUnknownIssueNeverRetries(t *testing.T) {
	ref := readDailyReference(t)
	c := dailyReferenceClient(t, ref.Transport.Wire[0].Client)
	calls := 0
	c.http.Transport = clientTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/pool/v2/issue" {
			t.Error("cross-mode path")
		}
		return nil, errors.New("lost response")
	})
	_, err := c.Issue(context.Background())
	clientError(t, err, "outcome_unknown", true)
	if calls != 1 {
		t.Fatal("automatic issue retry")
	}
	for _, row := range ref.Transport.Errors {
		if !row.Valid {
			continue
		}
		raw := dailyReferenceRaw(t, row)
		c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) { calls++; return dailyHTTPRaw(raw, row.HTTPStatus), nil })
		before := calls
		_, err = c.Issue(context.Background())
		var got *ClientError
		if !errors.As(err, &got) || calls != before+1 {
			t.Fatal("error transport/retry", err)
		}
	}
	c.http.Transport = clientTransport(func(*http.Request) (*http.Response, error) {
		calls++
		raw := append([]byte{}, ref.Transport.Envelopes[0].Value...)
		var v DailyWorkResponse
		_ = json.Unmarshal(raw, &v)
		v.RecipientScript = "0014" + strings.Repeat("ab", 20)
		b, _ := json.Marshal(v)
		return dailyHTTPRaw(b, 200), nil
	})
	_, err = c.Issue(context.Background())
	clientError(t, err, "outcome_unknown", true)
	_, err = c.Work(context.Background(), strings.Repeat("1", 64))
	clientError(t, err, "unavailable", false)
	before := calls
	if _, err = c.Payouts(context.Background()); err == nil || calls != before {
		t.Fatal("legacy history used daily owner")
	}
}
func TestDailyTransportCheapRefuses(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile control")
	}
	ref := readDailyReference(t)
	c := ref.Transport.Wire[0].Client
	if _, err := NewDailyClient(c); err == nil {
		t.Fatal("cheap daily client")
	}
	if _, err := NewDailyServer(context.Background(), ServerConfig{Version: Version2, Listen: "127.0.0.1:0"}, nil, nil, nil); err != poolshare.ErrProfile {
		t.Fatal("cheap daily server", err)
	}
}
