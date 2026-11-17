// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"testing"
)

// Synthetic transaction/headers here exercise journal semantics only. Their
// claimed block hashes are not normal-work or admitted chain evidence.
func syntheticPublication(t *testing.T, p Pin, r Record) Publication {
	t.Helper()
	payload, _ := carrier(r.Declaration.Declaration)
	outpoint := Outpoint{TxID: hash("synthetic outpoint"), Index: 0}
	prev, _ := chainhash.NewHashFromStr(outpoint.TxID)
	coin := FundingCoin{Outpoint: outpoint, ValueSatoshis: 50000, Script: p.FundingScript, BlockHeight: 1, BlockHash: hash("synthetic funding block")}
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: *prev, Index: 0}, nil, nil))
	tx.AddTxOut(wire.NewTxOut(0, append([]byte{txscript.OP_RETURN, 73}, payload[:]...)))
	tx.AddTxOut(wire.NewTxOut(49000, rawID(p.FundingScript)))
	one := make([]byte, 32)
	one[31] = 1
	key, _ := btcec.PrivKeyFromBytes(one)
	script := append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, rawID(p.FundingScript)[2:]...)
	script = append(script, txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
	fetch := txscript.NewCannedPrevOutputFetcher(rawID(p.FundingScript), coin.ValueSatoshis)
	sig, e := txscript.WitnessSignature(tx, txscript.NewTxSigHashes(tx, fetch), 0, coin.ValueSatoshis, script, txscript.SigHashAll, key, true)
	if e != nil {
		t.Fatal(e)
	}
	tx.TxIn[0].Witness = sig
	var raw bytes.Buffer
	tx.Serialize(&raw)
	return Publication{Version: 1, RawTx: hex.EncodeToString(raw.Bytes()), TxID: tx.TxHash().String(), Coin: coin, BuiltAtMS: r.PreparedAtMS, State: "retained"}
}
func TestSeedPublicationShapeAndCompactForeignAnchor(t *testing.T) {
	requireNormal(t)
	p, r := structuralRecord(t)
	pub := syntheticPublication(t, p, r)
	r.Publication = &pub
	if _, e := ValidateRecord(p, r); e != nil {
		t.Fatal("synthetic positive control", e)
	}
	bad := pub
	bad.Coin.ValueSatoshis++
	if _, e := ValidatePublication(p, bad); e == nil {
		t.Fatal("amount signature binding")
	}
	var tx wire.MsgTx
	tx.Deserialize(bytes.NewReader(rawID(pub.RawTx)))
	tx.TxOut[0].Value = 7 // Foreign payload may burn value; only local output must be zero.
	block := wire.NewMsgBlock(&wire.BlockHeader{MerkleRoot: tx.TxHash()})
	block.AddTransaction(&tx)
	payload, _ := carrier(r.Declaration.Declaration)
	a, e := firstAnchor(context.Background(), block, 2, hash("synthetic anchor"), payload)
	if e != nil || a == nil {
		t.Fatal("nonzero foreign carrier omitted", e)
	}
	if validateAnchor(p, *a) != nil {
		t.Fatal("compact observation")
	}
	badAnchor := *a
	badAnchor.TxCount = 2
	if validateAnchor(p, badAnchor) == nil {
		t.Fatal("invalid merkle count")
	}
	r.Anchor = a
	r.Cursor.Height = 2
	r.Cursor.Hash = a.Hash
	r.Cursor.CompleteToHeight = 2
	r.Cursor.CompleteToHash = a.Hash
	if _, e = ValidateRecord(p, r); e != nil {
		t.Fatal("anchor control", e)
	}
	r.Cursor.CompleteToHeight = 9
	r.Cursor.CompleteToHash = hash("unscanned tip")
	if _, e = ValidateRecord(p, r); e == nil {
		t.Fatal("unscanned firstness overclaim")
	}
}
func TestSeedRestartAndHistoricalFinalityStatusRules(t *testing.T) {
	requireNormal(t)
	p, r := structuralRecord(t)
	old := r.Cursor
	old.Height = 4
	old.Hash = hash("old scan branch")
	next := initialCursor(p, r.EventKey, 1)
	restart := ScanRestart{Generation: 1, Previous: old, ObservedAtMS: r.PreparedAtMS + 1, ObservedTip: Tip{Height: 5, Hash: hash("new tip")}, ReplacementHash: hash("replacement")}
	r.ScanRestarts = []ScanRestart{restart}
	r.Cursor = next
	if _, e := ValidateRecord(p, r); e != nil {
		t.Fatal("restart positive", e)
	}
	r.ScanRestarts[0].ReplacementHash = old.Hash
	if _, e := ValidateRecord(p, r); e == nil {
		t.Fatal("restart without branch mismatch")
	}
	ev := r.SourceReceipt.Input.Event
	absent := absentStatus(p, ev, ev.IncidentDeadlineMS)
	if !absent.EscalationDue || absent.IncidentAtMS != 0 {
		t.Fatal("absent incident provenance")
	}
	if _, e := ValidateStatus(p, absent); e != nil {
		t.Fatal(e)
	}
	absent.IncidentAtMS = ev.IncidentDeadlineMS
	if _, e := ValidateStatus(p, absent); e == nil {
		t.Fatal("invented absent incident")
	}
}

func TestSeedFundingScanZeroOutputAndSpendOrder(t *testing.T) {
	requireNormal(t)
	p, _ := structuralRecord(t)
	zero := wire.NewMsgTx(2)
	zero.AddTxOut(wire.NewTxOut(0, rawID(p.FundingScript)))
	positive := wire.NewMsgTx(2)
	positive.AddTxOut(wire.NewTxOut(1294, rawID(p.FundingScript)))
	spend := wire.NewMsgTx(2)
	id := positive.TxHash()
	spend.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: id, Index: 0}, nil, nil))
	spend.AddTxOut(wire.NewTxOut(294, rawID(p.FundingScript)))
	b := wire.NewMsgBlock(&wire.BlockHeader{})
	b.AddTransaction(zero)
	b.AddTransaction(positive)
	coins := map[string]FundingCoin{}
	if e := updateFunding(context.Background(), p, coins, b, 1, hash("synthetic funding")); e != nil || len(coins) != 1 {
		t.Fatal("zero value must not consume positive inventory", len(coins), e)
	}
	b.Transactions = []*wire.MsgTx{spend}
	if e := updateFunding(context.Background(), p, coins, b, 2, hash("synthetic funding next")); e != nil || len(coins) != 1 {
		t.Fatal("spend/change inventory", len(coins), e)
	}
	if _, ok := coins[string(coinKey('f', Outpoint{TxID: id.String(), Index: 0}))]; ok {
		t.Fatal("spent funding retained")
	}
}

// This constructs a structurally signed model only. It deliberately does not
// claim that its synthetic block hashes satisfy normal proof of work.
func syntheticFinality(t *testing.T, p Pin, r Record, at int64) Record {
	t.Helper()
	pub := syntheticPublication(t, p, r)
	var tx wire.MsgTx
	if e := tx.Deserialize(bytes.NewReader(rawID(pub.RawTx))); e != nil {
		t.Fatal(e)
	}
	block := wire.NewMsgBlock(&wire.BlockHeader{MerkleRoot: tx.TxHash()})
	block.AddTransaction(&tx)
	payload, _ := carrier(r.Declaration.Declaration)
	a, e := anchorAt(block, 2, hash("synthetic anchor"), 0, 0, payload)
	if e != nil {
		t.Fatal(e)
	}
	r.Anchor = &a
	r.Cursor.Height, r.Cursor.CompleteToHeight = 2, 2
	r.Cursor.Hash, r.Cursor.CompleteToHash = a.Hash, a.Hash
	parent, _ := chainhash.NewHashFromStr(a.Hash)
	target := Target{Height: 3, Hash: hash("synthetic target"), Header: headerString(wire.BlockHeader{PrevBlock: *parent}), ObservedAtMS: at, ObservedTip: Tip{Height: 63, Hash: hash("synthetic mature tip")}}
	r.Target = &target
	targetHash, _ := chainhash.NewHashFromStr(target.Hash)
	genesis, _ := chainhash.NewHashFromStr(p.Genesis)
	cp := chaincfg.SignedCheckpoint{Version: 1, Network: wire.BitcoinNet(p.Network), GenesisHash: *genesis, Height: 3, Hash: *targetHash}
	message, e := cp.SigningBytes()
	if e != nil {
		t.Fatal(e)
	}
	cp.Signature = ed25519.Sign(fixtureKey(true), message)
	r.Checkpoint = &cp
	r.Finality = &Finality{Checkpoint: cp, ObservedAtMS: at, ObservedTip: target.ObservedTip, TargetDescendants: 60}
	r.Observation = observationFor(r, target.ObservedTip, at, true)
	if _, e := ValidateRecord(p, r); e != nil {
		t.Fatal("synthetic finality positive control", e)
	}
	return r
}
