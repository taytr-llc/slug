// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func replayFixture(t *testing.T) state {
	t.Helper()
	a, b := modelThirdsCoin(strings.Repeat("aa", 32)), modelThirdsCoin(strings.Repeat("bb", 32))
	first := modelBatch(t, []Coin{a}, "2026-11-18")
	second := modelBatch(t, []Coin{*first.Change, b}, "2026-11-19")
	return state{Config: modelConfig(), Credits: []Credit{{a.BlockHash, strings.Repeat("01", 32), a}, {b.BlockHash, strings.Repeat("02", 32), b}}, Batches: []Batch{first, second}, LastClockMS: second.CreatedAtMS}
}

func replayAll(t *testing.T, s state) *settlementReplay {
	t.Helper()
	r, err := newSettlementReplay(s.Config, s.LastClockMS, replayCounts{uint64(len(s.Credits)), uint64(len(s.Batches))})
	if err != nil {
		t.Fatal(err)
	}
	// Only valid distinct fixture inputs use false duplicate facts here. The
	// conditional-check controls below separately exercise each true fact.
	for i, c := range s.Credits {
		if err = r.addCredit(c, replayCreditCheck{Sequence: uint64(i) + 1}); err != nil {
			t.Fatal(err)
		}
	}
	for i, b := range s.Batches {
		if err = r.applyBatch(b, replayBatchCheck{Sequence: uint64(i) + 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err = r.complete(); err != nil {
		t.Fatal(err)
	}
	return r
}

// Published independent wallet transactions are reused byte-for-byte. The
// added claims below are explicit conditional 99/1 accounting inputs, not
// assertions that these fixture outpoints were mined, canonical or credited.
// The signed raw, unsigned body and txid are never derived by the new replay.
func TestSettlementReplayIndependentTransactions(t *testing.T) {
	raw, err := os.ReadFile("testdata/operator-wallet/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "c1f2d725fcdbc3af7ba1b0a30f4dd4dfc6983352ffb0b4f72d83f7c09ac7b087" {
		t.Fatal("independent fixture changed")
	}
	var corpus struct {
		Guards []struct {
			Fixture    string `json:"fixture"`
			Settlement Config `json:"settlement"`
		} `json:"guards"`
		Transactions []struct {
			Fixture string                       `json:"fixture"`
			Shape   struct{ Inputs, Payees int } `json:"shape"`
			Coins   []struct {
				Hash   string `json:"hash_wire"`
				Index  uint32 `json:"index"`
				Atoms  int64  `json:"atoms"`
				Script string `json:"script"`
			} `json:"coins"`
			Signed   string `json:"signed_hex"`
			Unsigned string `json:"unsigned_hex"`
			TxID     string `json:"txid"`
			Fee      int64  `json:"fee_atoms"`
		} `json:"transactions"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Guards) != 3 || len(corpus.Transactions) != 9 {
		t.Fatal("independent fixture inventory")
	}
	configs := make(map[string]Config)
	for _, g := range corpus.Guards {
		configs[g.Fixture] = g.Settlement
	}
	for _, row := range corpus.Transactions {
		t.Run(fmt.Sprintf("%s-%d", row.Fixture, row.Shape.Inputs), func(t *testing.T) {
			cfg := configs[row.Fixture]
			if len(row.Coins) != row.Shape.Inputs || row.Shape.Payees < 1 || row.Shape.Payees > 2 {
				t.Fatal("fixture shape")
			}
			const stamp int64 = 1794960000000 // 2026-11-18T00:00:00Z, independent UTC literal.
			const unknown = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
			s := state{Config: cfg, LastClockMS: stamp}
			inputs := make([]Coin, 0, len(row.Coins))
			payments := make([]Payment, 0, row.Shape.Payees)
			remaining := []Claim{}
			for i, c := range row.Coins {
				if c.Atoms != 1000000000 || c.Index != 0 || c.Script != cfg.ChangeScript {
					t.Fatal("fixture input changed")
				}
				wireHash, e := hex.DecodeString(c.Hash)
				if e != nil || len(wireHash) != 32 {
					t.Fatal("fixture wire hash")
				}
				for a, z := 0, len(wireHash)-1; a < z; a, z = a+1, z-1 {
					wireHash[a], wireHash[z] = wireHash[z], wireHash[a]
				}
				root := fmt.Sprintf("%064x", i+1)
				claims := []Claim{{RoundHash: root, RoundHeight: uint32(i + 1), Amount: Fraction{"10000000", "1"}}}
				carry := int64(990000000)
				if i < row.Shape.Payees {
					d := cfg.Destinations[i]
					claims = append(claims, Claim{RoundHash: root, RoundHeight: uint32(i + 1), GymID: d.GymID, Amount: Fraction{"100000000", "1"}})
					payments = append(payments, Payment{d.GymID, d.Script, 100000000})
					carry -= 100000000
				}
				claims = append(claims, Claim{RoundHash: root, RoundHeight: uint32(i + 1), GymID: unknown, Amount: Fraction{fmt.Sprint(carry), "1"}})
				coin := Coin{TxID: hex.EncodeToString(wireHash), Index: c.Index, Atoms: c.Atoms, Script: c.Script, Coinbase: true, BlockHash: root, BlockHeight: uint32(i + 1), Claims: claims}
				s.Credits = append(s.Credits, Credit{root, strings.Repeat("01", 32), coin})
				inputs = append(inputs, coin)
				feeClaim := claims[0]
				if i == 0 {
					feeClaim.Amount = Fraction{fmt.Sprint(int64(10000000) - row.Fee), "1"}
				}
				remaining = append(remaining, feeClaim, claims[len(claims)-1])
			}
			wantChange := Coin{TxID: row.TxID, Index: uint32(len(payments)), Atoms: int64(len(inputs))*1000000000 - int64(len(payments))*100000000 - row.Fee, Script: cfg.ChangeScript, Claims: remaining}
			batch := Batch{Day: "2026-11-18", CreatedAtMS: stamp, Inputs: inputs, Payments: payments, FeeAtoms: row.Fee, Change: &wantChange, TxID: row.TxID, Raw: row.Signed, State: "confirmed"}
			s.Batches = []Batch{batch}
			// Independently frozen unsigned bytes also bind the conditional
			// claims to the exact signed transaction shape under test.
			body, e := unsignedTransaction(inputs, payments, &wantChange)
			if e != nil || hex.EncodeToString(body) != row.Unsigned {
				t.Fatal("frozen unsigned body", e)
			}
			r := replayAll(t, s)
			want := []Coin{wantChange}
			if !reflect.DeepEqual(r.frontier(), want) {
				t.Fatal("independent exact change differs")
			}
			legacy, e := replayLegacyLiveCoins(s)
			if e != nil || !reflect.DeepEqual(legacy, want) {
				t.Fatal("legacy fixture mismatch", e)
			}
			for _, p := range payments {
				if r.accounting.paid[p.GymID] != 100000000 || r.accounting.reserved[p.GymID] != 0 {
					t.Fatal("payment total")
				}
			}
			if _, exists := r.accounting.gross[""]; exists {
				t.Fatal("operator claim exposed as miner gross")
			}
			got, e := r.balances(want, "", nil)
			old, oe := replayLegacyMakeBalances(s, want, "", nil)
			if e != oe || !reflect.DeepEqual(got, old) {
				t.Fatal("independent fixture projection parity", e, oe)
			}
		})
	}
}

func TestSettlementReplayV1Parity(t *testing.T) {
	base := replayFixture(t)
	cases := []struct {
		name   string
		mutate func(*state)
	}{
		{"valid", func(*state) {}},
		{"prepared", func(s *state) { s.Batches[1].State = "prepared" }},
		{"broadcast", func(s *state) { s.Batches[1].State = "broadcast" }},
		{"empty", func(s *state) { s.Credits = nil; s.Batches = nil; s.LastClockMS = 0 }},
		{"zero-reward", func(s *state) {
			c := modelCoin(strings.Repeat("33", 32), 0, []Claim{{Amount: Fraction{"0", "1"}}, {GymID: modelGymA, Amount: Fraction{"0", "1"}}})
			s.Credits = []Credit{{c.BlockHash, strings.Repeat("01", 32), c}}
			s.Batches = nil
			s.LastClockMS = 0
		}},
		{"credit-limit", func(s *state) { s.Config.MaxRounds = 1 }},
		{"batch-limit", func(s *state) { s.Config.MaxBatches = 1 }},
		{"duplicate-root", func(s *state) { s.Credits[1] = s.Credits[0] }},
		{"duplicate-outpoint", func(s *state) { s.Credits[1].Coin.TxID = s.Credits[0].Coin.TxID }},
		{"claim-origin", func(s *state) { s.Credits[1].Coin.Claims[0].RoundHeight++ }},
		{"claim-fraction", func(s *state) { s.Credits[1].Coin.Claims[1].Amount = Fraction{"2", "4"} }},
		{"claim-split", func(s *state) {
			s.Credits[1].Coin.Claims[0].Amount = Fraction{"1000000000", "100"}
			s.Credits[1].Coin.Claims[1].Amount = Fraction{"33000000034", "50"}
		}},
		{"day", func(s *state) { s.Batches[1].Day = s.Batches[0].Day }},
		{"clock", func(s *state) { s.LastClockMS-- }},
		{"pending-predecessor", func(s *state) { s.Batches[0].State = "prepared" }},
		{"fee", func(s *state) { s.Batches[1].FeeAtoms++ }},
		{"payment", func(s *state) { s.Batches[1].Payments[0].Atoms++ }},
		{"change", func(s *state) { s.Batches[1].Change.Claims[0].Amount = Fraction{"1", "1"} }},
		{"spent-root", func(s *state) { s.Batches[1].Inputs = []Coin{s.Credits[0].Coin} }},
		{"reordered-inputs", func(s *state) {
			s.Batches[1].Inputs[0], s.Batches[1].Inputs[1] = s.Batches[1].Inputs[1], s.Batches[1].Inputs[0]
		}},
		{"txid", func(s *state) { s.Batches[1].TxID = s.Batches[0].TxID }},
		{"raw", func(s *state) { s.Batches[1].Raw += "00" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := clone(base)
			tc.mutate(&s)
			before := clone(s)
			got, e := liveCoins(s)
			want, we := replayLegacyLiveCoins(s)
			if e != we || !reflect.DeepEqual(got, want) {
				t.Fatal("V1 replay behavior changed", e, we)
			}
			if e == nil {
				r := replayAll(t, s)
				if _, exists := r.accounting.gross[""]; exists {
					t.Fatal("operator fee entered miner projection")
				}
				// Confirmed and both pending delivery states, empty and zero
				// reward sources all compare the accumulator projection itself.
				for _, reason := range []string{"", "transaction-capacity", "funding-conflict"} {
					got, e := r.balances(nil, reason, nil)
					want, we := replayLegacyMakeBalances(s, nil, reason, nil)
					if e != we || !reflect.DeepEqual(got, want) {
						t.Fatal("accumulator balances differ", reason, e, we)
					}
				}
			}
			for _, reason := range []string{"", "transaction-capacity", "below-minimum", "funding-conflict"} {
				got, e := makeBalances(s, nil, reason, nil)
				want, we := replayLegacyMakeBalances(s, nil, reason, nil)
				if e != we || !reflect.DeepEqual(got, want) {
					t.Fatal("V1 balance projection changed", reason, e, we)
				}
			}
			if !reflect.DeepEqual(s, before) {
				t.Fatal("V1 input mutated")
			}
		})
	}
}

func TestSettlementReplayInterleavedOwnedProjection(t *testing.T) {
	s := replayFixture(t)
	r, e := newSettlementReplay(s.Config, s.LastClockMS, replayCounts{2, 2})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = r.addCredit(s.Credits[i], replayCreditCheck{Sequence: uint64(i) + 1}); e != nil {
			t.Fatal(e)
		}
		if e = r.applyBatch(s.Batches[i], replayBatchCheck{Sequence: uint64(i) + 1}); e != nil {
			t.Fatal(e)
		}
	}
	if e = r.complete(); e != nil {
		t.Fatal(e)
	}
	want, e := replayLegacyLiveCoins(s)
	if e != nil || !reflect.DeepEqual(r.frontier(), want) {
		t.Fatal("interleaved frontier order", e)
	}
	if r.last != (replayLastBatch{s.Batches[1].Day, s.Batches[1].CreatedAtMS, s.Batches[1].TxID, "confirmed"}) {
		t.Fatal("last batch metadata")
	}
	balances, e := r.balances(want, "", nil)
	expected := []Balance{
		{GymID: modelGymA, Confirmed: Fraction{"8", "25"}, Available: Fraction{"8", "25"}, PaidAtoms: 1320000001, Reason: "below-minimum"},
		{GymID: modelGymB, Confirmed: Fraction{"33", "50"}, Available: Fraction{"33", "50"}, PaidAtoms: 660000000, Reason: "below-minimum"},
	}
	if e != nil || !reflect.DeepEqual(balances, expected) {
		t.Fatal("literal fractional projection", e, balances)
	}
	owned := r.frontier()
	owned[0].Claims[0].Amount = Fraction{"0", "1"}
	s.Config.Destinations[0].Script = "changed"
	s.Credits[0].Coin.Claims[0].Amount = Fraction{"0", "1"}
	s.Batches[1].Change.Claims[0].Amount = Fraction{"0", "1"}
	if !reflect.DeepEqual(r.frontier(), want) || r.config.Destinations[0].Script == "changed" {
		t.Fatal("retained accumulator aliases caller")
	}
}

func replaySnapshot(r *settlementReplay) []byte {
	gross := make(map[string]string)
	for k, v := range r.accounting.gross {
		gross[k] = v.RatString()
	}
	raw, _ := json.Marshal(struct {
		Seen           replayCounts
		Last           replayLastBatch
		Coins          []Coin
		Gross          map[string]string
		Paid, Reserved map[string]int64
	}{r.seen, r.last, r.coins, gross, r.accounting.paid, r.accounting.reserved})
	return raw
}

func TestSettlementReplayConditionalRefusalsAtomic(t *testing.T) {
	s := replayFixture(t)
	for _, tc := range []struct {
		name  string
		check replayCreditCheck
	}{
		{"zero", replayCreditCheck{}}, {"gap", replayCreditCheck{Sequence: 2}},
		{"duplicate-round", replayCreditCheck{Sequence: 1, DuplicateRound: true}},
		{"duplicate-coin", replayCreditCheck{Sequence: 1, DuplicateCoin: true}},
	} {
		t.Run("credit-"+tc.name, func(t *testing.T) {
			r, e := newSettlementReplay(s.Config, s.LastClockMS, replayCounts{2, 2})
			if e != nil {
				t.Fatal(e)
			}
			before := replaySnapshot(r)
			if e = r.addCredit(s.Credits[0], tc.check); e != ErrCorrupt || !bytes.Equal(before, replaySnapshot(r)) {
				t.Fatal("rejected credit changed projection", e)
			}
			if e = r.complete(); e != ErrCorrupt {
				t.Fatal("incomplete traversal accepted")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		check  replayBatchCheck
		mutate func(*Batch)
	}{
		{"zero", replayBatchCheck{}, func(*Batch) {}},
		{"gap", replayBatchCheck{Sequence: 2}, func(*Batch) {}},
		{"duplicate-tx", replayBatchCheck{Sequence: 1, DuplicateTx: true}, func(*Batch) {}},
		{"duplicate-change", replayBatchCheck{Sequence: 1, DuplicateChange: true}, func(*Batch) {}},
		{"payment", replayBatchCheck{Sequence: 1}, func(b *Batch) { b.Payments[0].Atoms++ }},
		{"input-claim", replayBatchCheck{Sequence: 1}, func(b *Batch) { b.Inputs[0].Claims[0].Amount = Fraction{"1", "1"} }},
		{"signature", replayBatchCheck{Sequence: 1}, func(b *Batch) { b.Raw += "00" }},
		{"pending-predecessor", replayBatchCheck{Sequence: 1}, func(b *Batch) { b.State = "broadcast" }},
	} {
		t.Run("batch-"+tc.name, func(t *testing.T) {
			r, e := newSettlementReplay(s.Config, s.LastClockMS, replayCounts{2, 2})
			if e != nil {
				t.Fatal(e)
			}
			for i, c := range s.Credits {
				if e = r.addCredit(c, replayCreditCheck{Sequence: uint64(i) + 1}); e != nil {
					t.Fatal(e)
				}
			}
			before := replaySnapshot(r)
			b := clone(s.Batches[0])
			tc.mutate(&b)
			if e = r.applyBatch(b, tc.check); e != ErrCorrupt || !bytes.Equal(before, replaySnapshot(r)) {
				t.Fatal("rejected batch changed projection", e)
			}
			if e = r.applyBatch(s.Batches[0], replayBatchCheck{Sequence: 1}); e != nil {
				t.Fatal("valid retry of pure transition", e)
			}
			if e = r.complete(); e != ErrCorrupt {
				t.Fatal("missing final batch accepted")
			}
			if got, e := r.balances(nil, "", nil); e != ErrCorrupt || got != nil {
				t.Fatal("incomplete balances disclosed")
			}
		})
	}
	zero, e := newSettlementReplay(s.Config, 0, replayCounts{})
	if e != nil || zero.complete() != nil {
		t.Fatal("empty projection")
	}
	if zero.addCredit(s.Credits[0], replayCreditCheck{Sequence: 1}) != ErrCorrupt || zero.applyBatch(s.Batches[0], replayBatchCheck{Sequence: 1}) != ErrCorrupt {
		t.Fatal("rows beyond caller counts accepted")
	}
}
