// Copyright (c) 2026 The SLUG developers. ISC license.
package walletwithdraw

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestWithdrawalBoundedGrammarRefusals(t *testing.T) {
	c := journalReference(t)[0]
	d := *phaseValue(c, "prepared").Draft
	raw, _ := json.Marshal(d)
	for name, b := range map[string][]byte{
		"outer_null": []byte("null"), "leading_space": append([]byte(" "), raw...), "duplicate": bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1), "unknown": bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"unknown":0`), 1), "numeric_exponent": bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1e0`), 1), "omitted": bytes.Replace(raw, []byte(`"version":1,`), nil, 1), "null_inputs": bytes.Replace(raw, []byte(`"inputs":[`), []byte(`"inputs":null,"discard":[`), 1), "long_string": bytes.Replace(raw, []byte(d.UnsignedTxHex), []byte(strings.Repeat("0", 8193)), 1), "deep": []byte(strings.Repeat(`{"draft":`, 12) + `null` + strings.Repeat("}", 12)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := ParseDraft(b); e == nil {
				t.Fatal("accepted malformed draft")
			}
		})
	}
	for _, change := range []func(*Draft){func(v *Draft) { v.Inputs = append(v.Inputs, v.Inputs...) }, func(v *Draft) { v.TipHeight = math.MaxUint32 }, func(v *Draft) { v.Inputs[0].Script = strings.ToUpper(v.Inputs[0].Script) }, func(v *Draft) { v.Inputs[0].Atoms = maxMoney; v.TotalInputAtoms = maxMoney }, func(v *Draft) { v.Change.OutputIndex = 3 }} {
		copy := d
		copy.Inputs = append([]Input(nil), d.Inputs...)
		change(&copy)
		if _, e := DraftDigest(copy); e == nil {
			t.Fatal("accepted invalid typed draft")
		}
	}
	var st state
	oversized := `{"application":"x","version":1,"journal_id":"","binding_id":"","clock_high_water_ms":0,"records":[` + strings.Repeat(`{},`, 128) + `{}]}`
	if decode([]byte(oversized), MaxStateBytes, &st) == nil {
		t.Fatal("129 records decoded")
	}
}
func phaseValue(c journalCase, name string) Operation {
	for _, p := range c.Phases {
		if p.Name == name {
			return ownedOperation(p.Operation)
		}
	}
	panic(name)
}
func TestWithdrawalTransactionFramingRefusals(t *testing.T) {
	c := journalReference(t)[0]
	v := phaseValue(c, "signed")
	unsigned, _ := hex.DecodeString(v.Draft.UnsignedTxHex)
	signed, _ := hex.DecodeString(v.Signed.Raw)
	cases := []struct {
		name   string
		raw    []byte
		signed bool
	}{
		{"huge_input_count", append(append([]byte{}, unsigned[:4]...), []byte{255, 255, 255, 255, 255, 255, 255, 255, 255}...), false},
		{"nonminimal_input_count", append(append(append([]byte{}, unsigned[:4]...), 253, 1, 0), unsigned[5:]...), false},
		{"extra_flag", append([]byte(nil), signed...), true}, {"trailing", append(append([]byte(nil), signed...), 0), true}, {"truncated", signed[:len(signed)-1], true},
	}
	cases[2].raw[5] = 3
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if transactionFrame(c.raw, c.signed) {
				t.Fatal("framing accepted")
			}
		})
	}
	for n := 0; n < len(signed); n++ {
		if transactionFrame(signed[:n], true) {
			t.Fatalf("truncation%d accepted", n)
		}
	}
}
func TestWithdrawalStateAuditRefusals(t *testing.T) {
	c := journalReference(t)[0]
	id := Identity{c.Identity.JournalID, c.Identity.Pin}
	base := c.Phases[len(c.Phases)-1].State
	for _, mutate := range []func(*state){func(s *state) { s.ClockHighWaterMS-- }, func(s *state) { s.Records[0].Revision = 0 }, func(s *state) { s.Records[0].Sequence = 2 }, func(s *state) { s.Records[0].SignAuthorization.Generation = 9007199254740992 }, func(s *state) { s.Records[0].Signed.RawSHA256 = strings.Repeat("0", 64) }, func(s *state) { s.Records[0].Observation.BlockHash = "" }, func(s *state) { s.Records = append(s.Records, ownedOperation(s.Records[0])); s.Records[1].Sequence = 2 }, func(s *state) { s.Records = nil }} {
		s := base
		s.Records = []Operation{ownedOperation(base.Records[0])}
		mutate(&s)
		if validateState(context.Background(), s, id, c.BindingID) == nil {
			t.Fatal("audit accepted corruption")
		}
	}
}
