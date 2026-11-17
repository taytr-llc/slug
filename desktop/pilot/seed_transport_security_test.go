//go:build !slugtestpow

package pilot

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestSeedTransportFixedCarrierSignatureAndMerkle(t *testing.T) {
	// A locally signed synthetic transaction tests byte/signature relationships;
	// its labelled outpoint/header is not claimed as admitted or normal-work data.
	d := gymseed.Declaration{EventKey: strings.Repeat("11", 32)}
	prev, _ := chainhash.NewHashFromStr(strings.Repeat("22", 32))
	fund, _ := hex.DecodeString("0014751e76e8199196d454941c45d1b3a323f1433bd6")
	payload, _ := hex.DecodeString("534c55475345454401" + d.EventKey + seedHash("SLUG candidate seed declaration id\x00v1\x00", d))
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: *prev}, nil, nil))
	tx.AddTxOut(wire.NewTxOut(0, append([]byte{0x6a, 73}, payload...)))
	tx.AddTxOut(wire.NewTxOut(49000, fund))
	one := make([]byte, 32)
	one[31] = 1
	key, _ := btcec.PrivKeyFromBytes(one)
	script := append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, fund[2:]...)
	script = append(script, txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
	fetch := txscript.NewCannedPrevOutputFetcher(fund, 50000)
	sig, e := txscript.WitnessSignature(tx, txscript.NewTxSigHashes(tx, fetch), 0, 50000, script, txscript.SigHashAll, key, true)
	if e != nil {
		t.Fatal(e)
	}
	tx.TxIn[0].Witness = sig
	var raw bytes.Buffer
	if tx.Serialize(&raw) != nil {
		t.Fatal("serialize")
	}
	p := gymseed.Publication{Version: 1, RawTx: hex.EncodeToString(raw.Bytes()), TxID: tx.TxHash().String(), Coin: gymseed.FundingCoin{Outpoint: gymseed.Outpoint{TxID: prev.String()}, ValueSatoshis: 50000, Script: hex.EncodeToString(fund), BlockHeight: 1, BlockHash: strings.Repeat("33", 32)}, BuiltAtMS: 1000, State: "retained"}
	if !seedPublicationShape(p, d, 1000) {
		t.Fatal("valid signed carrier refused")
	}
	for _, mutate := range []func(*gymseed.Publication){func(p *gymseed.Publication) { p.Coin.ValueSatoshis++ }, func(p *gymseed.Publication) { p.Coin.Script = "0014" + strings.Repeat("00", 20) }, func(p *gymseed.Publication) {
		b, _ := hex.DecodeString(p.RawTx)
		b[len(b)-6] ^= 1
		p.RawTx = hex.EncodeToString(b)
	}, func(p *gymseed.Publication) { p.RawTx += "00" }} {
		bad := p
		mutate(&bad)
		if seedPublicationShape(bad, d, 1000) {
			t.Fatal("invalid funding/signature accepted")
		}
	}
	root := tx.TxHash()
	var header bytes.Buffer
	h := wire.BlockHeader{MerkleRoot: root}
	_ = h.Serialize(&header)
	a := gymseed.Anchor{Header: hex.EncodeToString(header.Bytes()), TxID: root.String(), TxCount: 1, MerkleBranch: []string{}}
	if !seedAnchorMerkle(a) {
		t.Fatal("single transaction proof")
	}
	a.TxCount = 2
	if seedAnchorMerkle(a) {
		t.Fatal("missing sibling accepted")
	}
	a.TxCount = 1
	a.MerkleBranch = []string{root.String()}
	if seedAnchorMerkle(a) {
		t.Fatal("extra sibling accepted")
	}
	a.MerkleBranch = []string{}
	a.TxID = strings.Repeat("44", 32)
	if seedAnchorMerkle(a) {
		t.Fatal("wrong leaf accepted")
	}
}

func TestSeedTransportAuthenticatedDeclarationConflict(t *testing.T) {
	source := eventFixturePin(t)
	p, e := gymseed.CandidatePin(source)
	if e != nil {
		t.Fatal(e)
	}
	ev, e := gymevent.EventForDate(source, "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	id, _ := gymseed.ID(p)
	event, _ := gymseed.EventKey(p, ev)
	d := gymseed.Declaration{Version: 1, Network: uint32(chaincfg.MainNetParams.Net), Genesis: chaincfg.MainNetParams.GenesisHash.String(), StoreID: id, SourceStoreID: p.SourceStoreID, Event: ev, EventKey: event, SnapshotDigest: strings.Repeat("11", 32), ReceiptDigest: strings.Repeat("22", 32), PublisherChainID: p.PublisherChainID, FloorHeight: 0, FloorHash: p.Genesis, RulesID: p.RulesID, PublisherPublicKey: p.PublisherPublicKey, CheckpointPublicKey: p.CheckpointPublicKey}
	seed := sha256.Sum256([]byte("SLUG candidate seed declaration fixture\x00v1\x00"))
	key := ed25519.NewKeyFromSeed(seed[:])
	sign := func(d gymseed.Declaration) gymseed.SignedDeclaration {
		b, _ := json.Marshal(d)
		m := []byte("SLUG candidate seed declaration\x00v1\x00")
		m = binary.LittleEndian.AppendUint32(m, uint32(len(b)))
		m = append(m, b...)
		return gymseed.SignedDeclaration{Version: 1, Declaration: d, Signature: hex.EncodeToString(ed25519.Sign(key, m))}
	}
	if !seedSignedShape(sign(d)) {
		t.Fatal("canonical declaration")
	}
	d.CheckpointPublicKey = strings.Repeat("34", 32)
	d.RulesID = strings.Repeat("56", 32)
	if !seedSignedShape(sign(d)) {
		t.Fatal("authenticated alternate must remain usable as conflict evidence")
	}
	bad := sign(d)
	bad.Declaration.SnapshotDigest = strings.Repeat("78", 32)
	if seedSignedShape(bad) {
		t.Fatal("tampered declaration")
	}
}
