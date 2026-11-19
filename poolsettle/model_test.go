// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

var modelGymA = strings.Repeat("11", 32)
var modelGymB = strings.Repeat("22", 32)

// Public scalar fixtures only. These keys must never receive real funds.
func modelKey(n byte) (*btcec.PrivateKey, string) {
	var scalar [32]byte
	scalar[31] = n
	key, pub := btcec.PrivKeyFromBytes(scalar[:])
	script := append([]byte{0, 20}, address.Hash160(pub.SerializeCompressed())...)
	return key, hex.EncodeToString(script)
}

func modelConfig() Config {
	_, pool := modelKey(1)
	_, a := modelKey(2)
	_, b := modelKey(3)
	return Config{Version: Version1, ChangeScript: pool, Destinations: []Destination{{modelGymA, a}, {modelGymB, b}}, MaxRounds: 256, MaxBatches: 366}
}

func modelCoin(root string, atoms int64, claims []Claim) Coin {
	_, script := modelKey(1)
	for i := range claims {
		claims[i].RoundHash = root
		claims[i].RoundHeight = 1
	}
	return Coin{TxID: root, Atoms: atoms, Script: script, Coinbase: true, BlockHash: root, BlockHeight: 1, Claims: claims}
}

func modelThirdsCoin(root string) Coin {
	// Independently calculated from reward 1,000,000,001, shares A2/B1,
	// miner fraction 99/100 and pool fraction 1/100. This is arithmetic
	// evidence, not a proof that the modeled coin was mined or accepted.
	return modelCoin(root, 1000000001, []Claim{
		{Amount: Fraction{"1000000001", "100"}},
		{GymID: modelGymA, Amount: Fraction{"33000000033", "50"}},
		{GymID: modelGymB, Amount: Fraction{"33000000033", "100"}},
	})
}

func modelTotals(t *testing.T, coin *Coin) map[string]string {
	t.Helper()
	result := make(map[string]*big.Rat)
	for _, claim := range coin.Claims {
		if result[claim.GymID] == nil {
			result[claim.GymID] = new(big.Rat)
		}
		amount, err := rat(claim.Amount)
		if err != nil {
			t.Fatal(err)
		}
		result[claim.GymID].Add(result[claim.GymID], amount)
	}
	strings := make(map[string]string)
	for gym, amount := range result {
		strings[gym] = amount.RatString()
	}
	return strings
}

func TestModelFractionalCarryAndFeeOnce(t *testing.T) {
	cfg := modelConfig()
	first := modelThirdsCoin(strings.Repeat("aa", 32))
	payment, change, reason, err := plan([]Coin{first}, cfg, 1000)
	if err != nil || reason != "" || len(payment) != 2 || payment[0].Atoms != 660000000 || payment[1].Atoms != 330000000 || change == nil || change.Atoms != 9999001 {
		t.Fatalf("first settlement: %v %+v %+v %q", err, payment, change, reason)
	}
	wantFirst := map[string]string{"": "999900001/100", modelGymA: "33/50", modelGymB: "33/100"}
	if !sameValue(modelTotals(t, change), wantFirst) {
		t.Fatalf("first carry: %v", modelTotals(t, change))
	}
	change.TxID = strings.Repeat("cc", 32)
	second := modelThirdsCoin(strings.Repeat("bb", 32))
	payment, next, reason, err := plan([]Coin{*change, second}, cfg, 1000)
	if err != nil || reason != "" || len(payment) != 2 || payment[0].Atoms != 660000001 || payment[1].Atoms != 330000000 || next == nil || next.Atoms != 19998001 {
		t.Fatalf("second settlement: %v %+v %+v %q", err, payment, next, reason)
	}
	wantSecond := map[string]string{"": "999900001/50", modelGymA: "8/25", modelGymB: "33/50"}
	if !sameValue(modelTotals(t, next), wantSecond) {
		t.Fatalf("second carry: %v", modelTotals(t, next))
	}
	// The original claims are owned values and were not debited in place.
	if first.Claims[1].Amount != (Fraction{"33000000033", "50"}) || change.Claims[1].Amount != (Fraction{"33", "50"}) {
		t.Fatal("plan mutated supplied claims")
	}
}

func TestModelDeferralsAndUnknownDestination(t *testing.T) {
	cfg := modelConfig()
	cases := []struct {
		name   string
		coin   Coin
		fee    int64
		reason string
	}{
		{"fee budget", modelThirdsCoin(strings.Repeat("aa", 32)), 10000001, "fee-budget"},
		{"carry under minimum", modelCoin(strings.Repeat("aa", 32), 100000000, []Claim{{Amount: Fraction{"1000000", "1"}}, {GymID: modelGymA, Amount: Fraction{"99000000", "1"}}}), 1000, "below-minimum"},
		{"miner dust retained", modelCoin(strings.Repeat("aa", 32), 100000002, []Claim{{Amount: Fraction{"1", "1"}}, {GymID: modelGymA, Amount: Fraction{"200000001", "2"}}, {GymID: modelGymB, Amount: Fraction{"1", "2"}}}), 1, "change-dust"},
		{"unknown destination", modelCoin(strings.Repeat("aa", 32), 200000000, []Claim{{Amount: Fraction{"2000000", "1"}}, {GymID: strings.Repeat("33", 32), Amount: Fraction{"198000000", "1"}}}), 1000, "destination-unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payments, change, reason, err := plan([]Coin{tc.coin}, cfg, tc.fee)
			if err != nil || reason != tc.reason || len(payments) != 0 || change != nil {
				t.Fatalf("unexpected deferral: %v %q %+v %+v", err, reason, payments, change)
			}
		})
	}
	coin := modelCoin(strings.Repeat("aa", 32), 300000000, []Claim{{Amount: Fraction{"3000000", "1"}}, {GymID: modelGymA, Amount: Fraction{"198000000", "1"}}, {GymID: strings.Repeat("33", 32), Amount: Fraction{"99000000", "1"}}})
	payments, change, reason, err := plan([]Coin{coin}, cfg, 1000)
	if err != nil || reason != "" || len(payments) != 1 || payments[0].Atoms != 198000000 || change.Atoms != 101999000 {
		t.Fatalf("unknown carry must not block known recipient: %v %q %+v %+v", err, reason, payments, change)
	}
	if modelTotals(t, change)[strings.Repeat("33", 32)] != "99000000" {
		t.Fatal("unmapped claim lost")
	}
}

func TestModelCanonicalConfigAndFractions(t *testing.T) {
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Destinations[0], c.Destinations[1] = c.Destinations[1], c.Destinations[0] },
		func(c *Config) { c.Destinations[1].GymID = c.Destinations[0].GymID },
		func(c *Config) { c.Destinations[1].Script = c.Destinations[0].Script },
		func(c *Config) { c.Destinations[0].Script = c.ChangeScript },
		func(c *Config) { c.ChangeScript = "0014ff" },
		func(c *Config) { c.MaxRounds = 257 },
		func(c *Config) { c.MaxBatches = 367 },
	} {
		cfg := modelConfig()
		mutate(&cfg)
		if validateConfig(cfg) == nil {
			t.Fatalf("invalid configuration accepted: %+v", cfg)
		}
	}
	for _, fraction := range []Fraction{{"2", "4"}, {"0", "2"}, {"01", "1"}, {"+1", "1"}, {"-1", "1"}, {"1", "0"}, {"1", "-1"}, {strings.Repeat("1", 4097), "1"}} {
		if _, err := rat(fraction); err == nil {
			t.Fatalf("noncanonical fraction accepted: %+v", fraction)
		}
	}
	if r, err := rat(Fraction{"0", "1"}); err != nil || r.Sign() != 0 {
		t.Fatal("canonical zero rejected")
	}
}

func modelSign(t *testing.T, inputs []Coin, payments []Payment, change *Coin) []byte {
	t.Helper()
	raw, err := unsignedTransaction(inputs, payments, change)
	if err != nil {
		t.Fatal(err)
	}
	tx := new(wire.MsgTx)
	if tx.Deserialize(bytes.NewReader(raw)) != nil {
		t.Fatal("decode unsigned")
	}
	key, scriptText := modelKey(1)
	script, _ := hex.DecodeString(scriptText)
	code := append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, script[2:]...)
	code = append(code, txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for i, input := range inputs {
		fetcher.AddPrevOut(tx.TxIn[i].PreviousOutPoint, wire.NewTxOut(input.Atoms, script))
	}
	hashes := txscript.NewTxSigHashes(tx, fetcher)
	for i, input := range inputs {
		witness, err := txscript.WitnessSignature(tx, hashes, i, input.Atoms, code, txscript.SigHashAll, key, true)
		if err != nil {
			t.Fatal(err)
		}
		tx.TxIn[i].Witness = witness
	}
	var encoded bytes.Buffer
	if tx.Serialize(&encoded) != nil {
		t.Fatal("encode signed")
	}
	return encoded.Bytes()
}

func modelBatch(t *testing.T, inputs []Coin, day string) Batch {
	t.Helper()
	payments, change, reason, err := plan(inputs, modelConfig(), 1000)
	if err != nil || reason != "" {
		t.Fatalf("plan: %v %s", err, reason)
	}
	raw := modelSign(t, inputs, payments, change)
	txid, err := validateSigned(inputs, payments, change, 1000, raw)
	if err != nil {
		t.Fatal(err)
	}
	if change != nil {
		change.TxID = txid
	}
	stamp, err := time.Parse("2006-01-02", day)
	if err != nil {
		t.Fatal(err)
	}
	return Batch{Day: day, CreatedAtMS: stamp.UnixMilli(), Inputs: inputs, Payments: payments, FeeAtoms: 1000, Change: change, TxID: txid, Raw: hex.EncodeToString(raw), State: "confirmed"}
}

func TestModelReplayAndSignedTransactionBinding(t *testing.T) {
	a, b := modelThirdsCoin(strings.Repeat("aa", 32)), modelThirdsCoin(strings.Repeat("bb", 32))
	first := modelBatch(t, []Coin{a}, "2026-11-18")
	second := modelBatch(t, []Coin{*first.Change, b}, "2026-11-19")
	s := state{Config: modelConfig(), Credits: []Credit{{a.BlockHash, strings.Repeat("01", 32), a}, {b.BlockHash, strings.Repeat("02", 32), b}}, Batches: []Batch{first, second}, LastClockMS: second.CreatedAtMS}
	coins, err := liveCoins(s)
	if err != nil || len(coins) != 1 || !sameValue(coins[0], *second.Change) {
		t.Fatalf("replay: %v %+v", err, coins)
	}
	for _, mutate := range []func(*Batch){
		func(b *Batch) { b.Day = first.Day },
		func(b *Batch) { b.FeeAtoms++ },
		func(b *Batch) { b.Payments = append([]Payment(nil), b.Payments...); b.Payments[0].Atoms++ },
		func(b *Batch) { b.Inputs = []Coin{a} },
		func(b *Batch) { b.TxID = first.TxID },
		func(b *Batch) { b.Raw += "00" },
	} {
		bad := s
		bad.Batches = append([]Batch(nil), s.Batches...)
		mutate(&bad.Batches[1])
		if _, err := liveCoins(bad); err == nil {
			t.Fatal("altered settlement history accepted")
		}
	}
	badClock := s
	badClock.LastClockMS--
	if _, err := liveCoins(badClock); err == nil {
		t.Fatal("batch newer than clock high water accepted")
	}
	badPending := s
	badPending.Batches = append([]Batch(nil), s.Batches...)
	badPending.Batches[0].State = "prepared"
	if _, err := liveCoins(badPending); err == nil {
		t.Fatal("new payout following unresolved intent accepted")
	}
	raw, _ := hex.DecodeString(first.Raw)
	tx := new(wire.MsgTx)
	if tx.Deserialize(bytes.NewReader(raw)) != nil {
		t.Fatal("decode")
	}
	tx.TxIn[0].Witness[0][8] ^= 1
	var changed bytes.Buffer
	if tx.Serialize(&changed) != nil {
		t.Fatal("encode")
	}
	if _, err := validateSigned(first.Inputs, first.Payments, first.Change, 1000, changed.Bytes()); err == nil {
		t.Fatal("invalid signature accepted")
	}
}

func TestModelCreditUsesExactCoinbaseAndConfirmation(t *testing.T) {
	cfg := modelConfig()
	script, _ := hex.DecodeString(cfg.ChangeScript)
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: ^uint32(0)}, []byte{1, 1}, nil))
	tx.AddTxOut(wire.NewTxOut(1000000001, script))
	tx.AddTxOut(wire.NewTxOut(0, []byte{txscript.OP_RETURN}))
	var raw bytes.Buffer
	if tx.SerializeNoWitness(&raw) != nil {
		t.Fatal("serialize fixture")
	}
	root := strings.Repeat("aa", 32)
	header := strings.Repeat("00", 80)
	r := poolround.Round{Version: 1, Sequence: 1, BlockHash: root, BlockHeight: 1, FirstAccepted: 1, LastAccepted: 3, ShareKeys: []string{"a", "b", "c"}, RewardAtoms: 1000000001, Allocations: []poolround.Allocation{{GymID: modelGymA, Shares: 2, Numerator: "2000000002", Denominator: "3"}, {GymID: modelGymB, Shares: 1, Numerator: "1000000001", Denominator: "3"}}, Winner: poolshare.AccountingEntry{Share: poolshare.Submission{Header: header, WorkHash: root}, RewardAtoms: 1000000001, Proof: &prospect.Proof{Header: header, Coinbase: hex.EncodeToString(raw.Bytes())}}}
	o := &prospectnode.PoolBlockObservation{BlockHash: root, BlockHeight: 1, Canonical: true, TipHeight: 61, TipHash: strings.Repeat("bb", 32), Descendants: 60}
	credit, err := deriveCredit(poolround.Record{Round: r, Confirmation: o})
	if err != nil || credit.Coin.TxID != tx.TxHash().String() || credit.Coin.Script != cfg.ChangeScript || credit.Coin.Atoms != 1000000001 || credit.Coin.Claims[1].Amount != (Fraction{"33000000033", "50"}) {
		t.Fatalf("credit: %v %+v", err, credit)
	}
	if _, err := deriveCredit(poolround.Record{Round: r}); err == nil {
		t.Fatal("unconfirmed round credited")
	}
	bad := r
	bad.RewardAtoms--
	if _, err := deriveCredit(poolround.Record{Round: bad, Confirmation: o}); err == nil {
		t.Fatal("coinbase value mismatch credited")
	}
	changed := *o
	changed.Descendants = 59
	changed.TipHeight = 60
	if _, err := deriveCredit(poolround.Record{Round: r, Confirmation: &changed}); err == nil {
		t.Fatal("59-descendant round credited")
	}
}
