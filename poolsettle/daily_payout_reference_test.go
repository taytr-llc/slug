// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestDailyPayoutIndependentReference(t *testing.T) {
	raw, err := os.ReadFile("../testdata/public-daily/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3" {
		t.Fatal("independent public daily reference changed")
	}
	type row struct {
		Name         string             `json:"name"`
		Valid        bool               `json:"valid"`
		Value        json.RawMessage    `json:"value"`
		CanonicalHex string             `json:"canonical_hex"`
		Request      DailyPayoutRequest `json:"request"`
	}
	var corpus struct {
		Payments struct {
			Balances   []row `json:"balances"`
			Pages      []row `json:"pages"`
			Requests   []row `json:"requests"`
			Traversals []struct {
				Name      string            `json:"name"`
				SlotCount int               `json:"slot_count"`
				Pages     []DailyPayoutPage `json:"pages"`
				PaidSum   string            `json:"paid_sum"`
			} `json:"traversals"`
		} `json:"payments"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Payments.Balances) != 41 || len(corpus.Payments.Pages) != 19 || len(corpus.Payments.Requests) != 6 || len(corpus.Payments.Traversals) != 6 {
		t.Fatal("reference inventory")
	}
	const gym = "a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5"
	decode := func(tc row, target any) error {
		b, err := hex.DecodeString(tc.CanonicalHex)
		if err != nil {
			t.Fatal(err)
		}
		return dailyDecode(b, MaxPayoutHistorySize, target)
	}
	for _, tc := range corpus.Payments.Balances {
		// These negatives exercise pinned transport selections, not the pure
		// balance shape. The service corpus tests cover those bindings.
		if strings.HasPrefix(tc.Name, "wrong-") {
			continue
		}
		t.Run("balance/"+tc.Name, func(t *testing.T) {
			var v struct {
				Version            uint32       `json:"version"`
				BindingID          string       `json:"binding_id"`
				SettlementConfigID string       `json:"settlement_config_id"`
				RecipientScript    string       `json:"recipient_script"`
				Balance            DailyBalance `json:"balance"`
				ObservedAtMS       int64        `json:"observed_at_ms"`
			}
			err := decode(tc, &v)
			if err == nil {
				err = ValidateDailyBalance(v.Balance, gym, v.ObservedAtMS)
			}
			if (err == nil) != tc.Valid {
				t.Fatal("independent balance acceptance", tc.Valid, err)
			}
			if err == nil && DailyBalanceFrom(v.Balance.Balance()) != v.Balance {
				t.Fatal("internal conversion lost exact cumulative totals")
			}
		})
	}
	for _, tc := range corpus.Payments.Requests {
		t.Run("request/"+tc.Name, func(t *testing.T) {
			var q DailyPayoutRequest
			err := decode(tc, &q)
			if err == nil {
				err = ValidateDailyPayoutRequest(q, MaxDailyPayoutBatches)
			}
			if (err == nil) != tc.Valid {
				t.Fatal("independent cursor acceptance", tc.Valid, err)
			}
		})
	}
	for _, tc := range corpus.Payments.Pages {
		if tc.Name == "wrong-money-binding" || tc.Name == "stale-response-revision" {
			continue // independently pinned response/request checks belong to client
		}
		t.Run("page/"+tc.Name, func(t *testing.T) {
			var p DailyPayoutPage
			err := decode(tc, &p)
			var got DailyPayoutPage
			if err == nil {
				got, err = ValidateDailyPayoutPage(p, gym)
			}
			if (err == nil) != tc.Valid {
				t.Fatal("independent page acceptance", tc.Valid, err)
			}
			if err != nil {
				return
			}
			b, err := json.Marshal(got)
			want, _ := hex.DecodeString(tc.CanonicalHex)
			if err != nil || !bytes.Equal(b, want) || !reflect.DeepEqual(got, p) {
				t.Fatal("exact independent page bytes", err)
			}
			if len(got.Items) > 0 {
				original := p.Items[0]
				got.Items[0].Payment.Atoms++
				if p.Items[0] != original {
					t.Fatal("returned page aliases supplied item slice")
				}
			}
		})
	}
	for _, tc := range corpus.Payments.Traversals {
		t.Run(tc.Name, func(t *testing.T) {
			var sum int64
			items := 0
			for i, p := range tc.Pages {
				if _, err := ValidateDailyPayoutPage(p, gym); err != nil {
					t.Fatal(err)
				}
				if i > 0 && (p.BeforeBatch != tc.Pages[i-1].NextBefore || p.Revision != tc.Pages[0].Revision || p.HighBatch != tc.Pages[0].HighBatch) {
					t.Fatal("independent traversal capture")
				}
				for _, row := range p.Items {
					sum += row.Payment.Atoms
					items++
				}
			}
			if items != tc.SlotCount || strconv.FormatInt(sum, 10) != tc.PaidSum || !tc.Pages[len(tc.Pages)-1].Done {
				t.Fatal("independent conditional traversal totals")
			}
		})
	}
}
