//go:build slugtestclock && !slugtestpow && !race

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func careerFixtureRaw(s string) []byte { b, _ := hex.DecodeString(s); return b }
func careerFixtureHash(domain, key string) string {
	h := sha256.Sum256([]byte(domain + "\x00" + key))
	return hex.EncodeToString(h[:])
}

// Pure modeled storage stress data, with real published fixture signatures.
// No RPC, proof-of-work, real funding, dispatch or admission is claimed. Source
// rows are separately fully audited by the concrete Bout/Result constructors.
func careerFinalizeModeled(t *testing.T, p gymresult.Pin, r gymresult.Record, n int, activationMS int64) gymresult.Record {
	t.Helper()
	r.PreparedAtMS = max(r.PreparedAtMS, activationMS+1)
	payload, e := gymresult.Carrier(r.Statement.Statement)
	if e != nil {
		t.Fatal(e)
	}
	coin := gymresult.FundingCoin{Outpoint: gymresult.Outpoint{TxID: careerFixtureHash("career modeled coin", r.ResultKey), Index: uint32(n)}, ValueSatoshis: 100000, Script: p.FundingScript, BlockHeight: 1, BlockHash: careerFixtureHash("career modeled funding block", r.ResultKey), Coinbase: true}
	var scalar [32]byte
	scalar[31] = 2
	k, _ := btcec.PrivKeyFromBytes(scalar[:])
	defer k.Zero()
	ch, _ := chainhash.NewHashFromStr(coin.Outpoint.TxID)
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(ch, coin.Outpoint.Index), nil, nil))
	script := careerFixtureRaw(coin.Script)
	code := append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, script[2:]...)
	code = append(code, txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
	tx.AddTxOut(wire.NewTxOut(0, append([]byte{txscript.OP_RETURN, 73}, payload[:]...)))
	tx.AddTxOut(wire.NewTxOut(coin.ValueSatoshis-1000, script))
	fetch := txscript.NewCannedPrevOutputFetcher(script, coin.ValueSatoshis)
	witness, e := txscript.WitnessSignature(tx, txscript.NewTxSigHashes(tx, fetch), 0, coin.ValueSatoshis, code, txscript.SigHashAll, k, true)
	if e != nil {
		t.Fatal(e)
	}
	tx.TxIn[0].Witness = witness
	var raw bytes.Buffer
	if e = tx.Serialize(&raw); e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw.Bytes())
	r.Publication = &gymresult.Publication{Version: 1, RawTx: hex.EncodeToString(raw.Bytes()), TxID: tx.TxHash().String(), Coin: coin, BuiltAtMS: r.PreparedAtMS, DispatchCount: 1, FirstDispatchAtMS: r.PreparedAtMS + 1, LastDispatchAtMS: r.PreparedAtMS + 1, State: "submitted"}
	height := uint32(1000 + n)
	h := wire.BlockHeader{MerkleRoot: tx.TxHash()}
	var hb bytes.Buffer
	if e = h.Serialize(&hb); e != nil {
		t.Fatal(e)
	}
	anchor := gymresult.Anchor{Height: height, Hash: careerFixtureHash("career modeled inclusion", r.ResultKey), Header: hex.EncodeToString(hb.Bytes()), TxID: r.Publication.TxID, RawTxDigest: hex.EncodeToString(digest[:]), RawTxSize: uint32(raw.Len()), TxCount: 1, Payload: hex.EncodeToString(payload[:]), MerkleBranch: []string{}}
	r.Inclusion = &gymresult.Inclusion{Version: 1, Anchor: anchor, ObservedAtMS: r.PreparedAtMS + 2, ObservedTip: gymresult.Tip{Height: height, Hash: anchor.Hash}}
	genesis, _ := chainhash.NewHashFromStr(p.Genesis)
	ah, _ := chainhash.NewHashFromStr(anchor.Hash)
	cp := chaincfg.SignedCheckpoint{Version: 1, Network: wire.BitcoinNet(p.Network), GenesisHash: *genesis, Height: int32(height), Hash: *ah}
	message, e := cp.SigningBytes()
	if e != nil {
		t.Fatal(e)
	}
	seed := sha256.Sum256([]byte("SLUG candidate result checkpoint fixture\x00v1\x00"))
	secret := ed25519.NewKeyFromSeed(seed[:])
	cp.Signature = ed25519.Sign(secret, message)
	clear(secret)
	r.Checkpoint = &cp
	tip := gymresult.Tip{Height: height + 60, Hash: careerFixtureHash("career modeled final tip", r.ResultKey)}
	r.Finality = &gymresult.Finality{Checkpoint: cp, Inclusion: *r.Inclusion, ObservedAtMS: r.PreparedAtMS + 3, ObservedTip: tip, Descendants: 60}
	r.Cursor.Height = int32(height)
	r.Cursor.Hash = anchor.Hash
	r.Cursor.CompleteToHeight = int32(height)
	r.Cursor.CompleteToHash = anchor.Hash
	r.Observation = gymresult.Observation{ObservedAtMS: r.Finality.ObservedAtMS, ChainObservedAtMS: r.Finality.ObservedAtMS, Tip: &tip, NodeState: "online", Descendants: 60, CurrentFinality: true}
	if _, e = gymresult.ValidateRecord(p, r); e != nil {
		t.Fatal("modeled finality validation", n, e)
	}
	return r
}
func careerPutFinalized(t *testing.T, b *leveldb.Batch, p gymresult.Pin, r gymresult.Record) {
	t.Helper()
	raw, e := gymresult.MarshalRecord(p, r)
	if e != nil {
		t.Fatal(e)
	}
	b.Put(append([]byte{'r'}, careerFixtureRaw(r.ResultKey)...), raw)
	var index [4]byte
	binary.LittleEndian.PutUint32(index[:], r.Publication.Coin.Outpoint.Index)
	key := append(append([]byte{'u'}, careerFixtureRaw(r.Publication.Coin.Outpoint.TxID)...), index[:]...)
	reservation := struct {
		Version   uint32 `json:"version"`
		ResultKey string `json:"result_key"`
		TxID      string `json:"txid"`
	}{1, r.ResultKey, r.Publication.TxID}
	raw, e = json.Marshal(reservation)
	if e != nil {
		t.Fatal(e)
	}
	b.Put(key, raw)
}
