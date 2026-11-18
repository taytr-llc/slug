// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

const resultReferenceRoot = "../gymresult/testdata/reference/"
const resultCorpusSHA = "07c00eef8d87ae9ae5dcd2d120a5dfd1f7b1ba33a1447b773cfd4d5194fd1a94"

type resultReference struct {
	Core struct {
		Facts struct {
			Carrier          string `json:"carrier"`
			FundingPublicKey string `json:"funding_public_key"`
			FundingScript    string `json:"funding_script"`
			AlternateRaw     string `json:"alternate_raw_tx"`
			Transaction      struct {
				Raw    string `json:"raw_tx"`
				TxID   string `json:"txid"`
				Digest string `json:"raw_tx_digest"`
			} `json:"transaction"`
		} `json:"facts"`
		Positive []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"positive"`
	} `json:"core"`
	Proof struct {
		Blocks []struct {
			Name   string `json:"name"`
			Raw    string `json:"raw_block"`
			Accept bool   `json:"structural_accept"`
			Normal bool   `json:"normal_pow_checked"`
		} `json:"block_controls"`
	} `json:"proof"`
}

func readResultReference(t *testing.T) resultReference {
	t.Helper()
	raw, err := os.ReadFile(resultReferenceRoot + "corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != resultCorpusSHA {
		t.Fatal("frozen independent result corpus changed")
	}
	var ref resultReference
	if err = json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	return ref
}

func (r resultReference) value(t *testing.T, name string, out any) {
	t.Helper()
	for _, row := range r.Core.Positive {
		if row.Name == name {
			if err := json.Unmarshal(row.Value, out); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("missing independent positive", name)
}

func resultFixture(t *testing.T) (resultReference, [73]byte, FundingCoin) {
	t.Helper()
	ref := readResultReference(t)
	raw, err := hex.DecodeString(ref.Core.Facts.Carrier)
	if err != nil || len(raw) != 73 {
		t.Fatal("carrier fixture")
	}
	var carrier [73]byte
	copy(carrier[:], raw)
	var coin FundingCoin
	ref.value(t, "funding-coin", &coin)
	return ref, carrier, coin
}

func TestResultIndependentTransactionSignaturesAndPurpose(t *testing.T) {
	ref, carrier, coin := resultFixture(t)
	tx, raw, err := resultPublication(carrier, coin)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(raw) != ref.Core.Facts.Transaction.Raw || tx.TxHash().String() != ref.Core.Facts.Transaction.TxID || hex.EncodeToString(digest[:]) != ref.Core.Facts.Transaction.Digest {
		t.Fatal("independent BIP143/RFC6979 transaction mismatch")
	}
	for _, encoded := range []string{ref.Core.Facts.Transaction.Raw, ref.Core.Facts.AlternateRaw} {
		retained, _ := hex.DecodeString(encoded)
		parsed, e := validateResultTransaction(carrier, coin, retained)
		if e != nil || parsed.TxHash() != tx.TxHash() {
			t.Fatal("valid retained signature rejected", e)
		}
	}
	if ref.Core.Facts.AlternateRaw == ref.Core.Facts.Transaction.Raw {
		t.Fatal("alternate witness control identical")
	}
	key, script, _ := resultFundingKey()
	if hex.EncodeToString(key.PubKey().SerializeCompressed()) != ref.Core.Facts.FundingPublicKey || hex.EncodeToString(script) != ref.Core.Facts.FundingScript {
		t.Fatal("key2 identity mismatch")
	}
	// These are validation controls, not newly signed expectations.
	for name, mutate := range map[string]func(*wire.MsgTx){
		"fee":           func(x *wire.MsgTx) { x.TxOut[1].Value++ },
		"input":         func(x *wire.MsgTx) { x.TxIn[0].PreviousOutPoint.Index++ },
		"key1 change":   func(x *wire.MsgTx) { _, s, _ := fundingKey(); x.TxOut[1].PkScript = s },
		"wrong purpose": func(x *wire.MsgTx) { x.TxOut[0].PkScript[2] = 'X' },
		"signature":     func(x *wire.MsgTx) { x.TxIn[0].Witness[0][5] ^= 1 },
		"sighash":       func(x *wire.MsgTx) { w := x.TxIn[0].Witness[0]; w[len(w)-1] = byte(txscript.SigHashNone) },
		"sequence":      func(x *wire.MsgTx) { x.TxIn[0].Sequence-- },
		"locktime":      func(x *wire.MsgTx) { x.LockTime = 1 },
		"version":       func(x *wire.MsgTx) { x.Version = 1 },
		"scriptSig":     func(x *wire.MsgTx) { x.TxIn[0].SignatureScript = []byte{0} },
		"output count":  func(x *wire.MsgTx) { x.AddTxOut(wire.NewTxOut(0, nil)) },
	} {
		t.Run(name, func(t *testing.T) {
			x := tx.Copy()
			mutate(x)
			var b bytes.Buffer
			_ = x.Serialize(&b)
			if _, e := validateResultTransaction(carrier, coin, b.Bytes()); e == nil {
				t.Fatal("accepted mutation")
			}
		})
	}
	for _, bad := range [][]byte{append(append([]byte{}, raw...), 0), make([]byte, MaxTransactionSize+1), nil} {
		if _, e := validateResultTransaction(carrier, coin, bad); e == nil {
			t.Fatal("accepted malformed bytes")
		}
	}
	for _, pos := range []int{9, 41} {
		c := carrier
		clear(c[pos : pos+32])
		if _, _, e := resultPublication(c, coin); e == nil {
			t.Fatal("accepted zero commitment half")
		}
		if _, e := validateResultTransaction(c, coin, raw); e == nil {
			t.Fatal("accepted zero commitment retained tx")
		}
	}
	c := carrier
	copy(c[:8], "SLUGSEED")
	if _, _, e := resultPublication(c, coin); e == nil {
		t.Fatal("accepted seed carrier")
	}
	_, seedScript, _ := fundingKey()
	badCoin := coin
	badCoin.Script = hex.EncodeToString(seedScript)
	if _, _, e := resultPublication(carrier, badCoin); e == nil {
		t.Fatal("accepted key1 coin")
	}
}

func TestResultIndependentCheckpointDomain(t *testing.T) {
	ref := readResultReference(t)
	var expected chaincfg.SignedCheckpoint
	ref.value(t, "SignedCheckpoint", &expected)
	got, err := resultCheckpoint(ChainIdentity{Network: uint32(expected.Network), Genesis: expected.GenesisHash.String()}, Anchor{Height: uint32(expected.Height), Hash: expected.Hash.String()})
	if err != nil || !reflect.DeepEqual(got, expected) {
		t.Fatal("independent result checkpoint differs", err)
	}
}

func TestResultIndependentWitnessAndCoinbaseControls(t *testing.T) {
	ref := readResultReference(t)
	if len(ref.Proof.Blocks) != 10 {
		t.Fatal("witness case inventory")
	}
	for _, row := range ref.Proof.Blocks {
		t.Run(row.Name, func(t *testing.T) {
			if row.Normal {
				t.Fatal("modeled bodies must not claim normal PoW")
			}
			raw, err := hex.DecodeString(row.Raw)
			if err != nil {
				t.Fatal(err)
			}
			r := bytes.NewReader(raw)
			b := new(wire.MsgBlock)
			e := b.Deserialize(r)
			ok := e == nil && r.Len() == 0 && validateResultBody(b) == nil
			if ok != row.Accept {
				t.Fatalf("structural accept=%v want=%v decode=%v", ok, row.Accept, e)
			}
		})
	}
}

func TestResultNilAndProfile(t *testing.T) {
	_, err := AttachOwnedResult(context.Background(), OwnerBinding{})
	if wire.TestPoW && !errors.Is(err, ErrProfile) || !wire.TestPoW && err == nil {
		t.Fatal("invalid ownership/profile accepted", err)
	}
	var p *ResultPublisher
	if _, err = p.Observe(context.Background()); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if p.Identity() != (ResultIdentity{}) || p.Close() != nil {
		t.Fatal("nil lifecycle")
	}
	p = &ResultPublisher{}
	if _, err = p.CanonicalHash(context.Background(), 0); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if _, err = p.Block(context.Background(), 0); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if _, err = p.Unspent(context.Background(), Outpoint{}); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if _, err = p.BuildPublication(context.Background(), [73]byte{}, FundingCoin{}); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if _, err = p.SubmitPublication(context.Background(), [73]byte{}, FundingCoin{}, nil); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if _, _, err = p.AttestInclusion(context.Background(), Anchor{}); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if _, err = p.ExportHeader(context.Background(), HeaderRef{}); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if _, err = p.ExportBlock(context.Background(), HeaderRef{}); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
}

func TestResultWorkBudgetChargesFailedNormalHash(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal profile only")
	}
	p := &ResultPublisher{owner: &Publisher{checked: map[string]string{}}}
	h := chaincfg.MainNetParams.GenesisBlock.Header
	if e := p.owner.verifyHeader(context.Background(), &h, strings.Repeat("01", 32)); !errors.Is(e, ErrUnavailable) || len(p.owner.checked) != 1 {
		t.Fatal("failed attempt uncharged", e)
	}
	for i := 1; i < MaxStepBlocks; i++ {
		p.owner.checked[strings.Repeat("x", i)] = strings.Repeat("02", 32)
	}
	if e := p.owner.verifyHeader(context.Background(), &h, chaincfg.MainNetParams.GenesisHash.String()); e != nil {
		t.Fatal("checked hash not reused", e)
	}
	h.Nonce++
	if e := p.owner.verifyHeader(context.Background(), &h, strings.Repeat("03", 32)); !errors.Is(e, ErrCapacity) {
		t.Fatal("ninth attempt", e)
	}
}
