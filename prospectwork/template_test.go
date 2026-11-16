//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectwork

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

func copyFixture(t *testing.T, fixture templateReference) templateReference {
	t.Helper()
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var copied templateReference
	if json.Unmarshal(encoded, &copied) != nil {
		t.Fatal("fixture copy failed")
	}
	return copied
}

func TestTemplateStrictJSON(t *testing.T) {
	fixture := templateReferences(t)[1]
	encoded, _ := json.Marshal(fixture.Template.Block)
	valid := string(encoded)
	cases := map[string]string{
		"trailing":          valid + "{}",
		"duplicate":         strings.Replace(valid, `"version":`, `"version":1,"version":`, 1),
		"escaped duplicate": strings.Replace(valid, `"version":`, `"ver\u0073ion":1,"version":`, 1),
		"field alias":       strings.Replace(valid, `"version":`, `"VERSION":1,"version":`, 1),
		"unknown":           strings.Replace(valid, `"version":`, `"surprise":1,"version":`, 1),
		"aux alias":         strings.Replace(valid, `"flags":`, `"FLAGS":"","flags":`, 1),
		"nested duplicate":  strings.Replace(valid, `"flags":`, `"flags":"","flags":`, 1),
		"tx alias":          strings.Replace(valid, `"data":`, `"DATA":"","data":`, 1),
		"tx duplicate":      strings.Replace(valid, `"fee":`, `"fee":0,"fee":`, 1),
		"null metadata":     strings.Replace(valid, `"noncerange":`, `"workid":null,"noncerange":`, 1),
		"null depends":      strings.Replace(valid, `"depends":[]`, `"depends":null`, 1),
		"missing":           strings.Replace(valid, `"version":`, `"wrong_version":`, 1),
		"wrong type":        strings.Replace(valid, `"version":536870912`, `"version":"536870912"`, 1),
		"invalid utf8":      valid[:len(valid)-1] + ",\"workid\":\"\xff\"}",
		"excess whitespace": valid + strings.Repeat(" ", MaxTemplateSize),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseTemplate(fixture.Template.Context, []byte(data)); err == nil {
				t.Fatal("accepted malformed template JSON")
			}
		})
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	for _, key := range []string{"bits", "coinbaseaux", "coinbasevalue", "transactions", "target", "height", "weightlimit", "sigoplimit", "sizelimit"} {
		t.Run("required "+key, func(t *testing.T) {
			copy := make(map[string]json.RawMessage)
			for name, value := range fields {
				copy[name] = value
			}
			delete(copy, key)
			data, _ := json.Marshal(copy)
			if _, err := ParseTemplate(fixture.Template.Context, data); err == nil {
				t.Fatal("accepted missing critical field")
			}
		})
	}
}

func TestTemplateRejectsInconsistentAssertions(t *testing.T) {
	fixture := templateReferences(t)[2]
	cases := map[string]func(*Template){
		"network":                    func(v *Template) { v.Context.Network++ },
		"genesis":                    func(v *Template) { v.Context.Genesis = strings.Repeat("1", 64) },
		"dna":                        func(v *Template) { v.Context.DNAVersion++ },
		"parent":                     func(v *Template) { v.Block.PreviousHash = strings.Repeat("1", 64) },
		"height":                     func(v *Template) { v.Block.Height++ },
		"height overflow":            func(v *Template) { v.Context.ParentHeight = math.MaxUint32 },
		"version":                    func(v *Template) { v.Block.Version = 1 },
		"bits":                       func(v *Template) { v.Block.Bits = "207ffffe" },
		"bits prefix":                func(v *Template) { v.Block.Bits = "0x7fffff" },
		"target":                     func(v *Template) { v.Block.Target = strings.Repeat("f", 64) },
		"prospect target zero":       func(v *Template) { v.Context.ProspectTarget = strings.Repeat("0", 64) },
		"curtime":                    func(v *Template) { v.Block.CurTime = v.Block.MaxTime + 1 },
		"min time":                   func(v *Template) { v.Block.MinTime-- },
		"max time":                   func(v *Template) { v.Block.MaxTime++ },
		"nonce restriction":          func(v *Template) { v.Block.NonceRange = "00000001ffffffff" },
		"missing payout":             func(v *Template) { v.Block.CoinbaseValue = nil },
		"missing flags":              func(v *Template) { v.Block.CoinbaseAux = nil },
		"bad flags":                  func(v *Template) { v.Block.CoinbaseAux.Flags = "xx" },
		"coinbasetxn":                func(v *Template) { v.Block.CoinbaseTxn = &btcjson.GetBlockTemplateResultTx{} },
		"reward mismatch":            func(v *Template) { *v.Block.CoinbaseValue++ },
		"negative fee":               func(v *Template) { v.Block.Transactions[0].Fee = -1 },
		"overflow fee":               func(v *Template) { v.Block.Transactions[0].Fee = math.MaxInt64 },
		"weight limit":               func(v *Template) { v.Block.WeightLimit = math.MaxInt64 },
		"size limit":                 func(v *Template) { v.Block.SizeLimit = 0 },
		"sigop limit":                func(v *Template) { v.Block.SigOpLimit = 80001 },
		"tx weight":                  func(v *Template) { v.Block.Transactions[0].Weight++ },
		"sigop overflow":             func(v *Template) { v.Block.Transactions[0].SigOps = math.MaxInt64 },
		"txid":                       func(v *Template) { v.Block.Transactions[0].TxID = strings.Repeat("0", 64) },
		"wtxid":                      func(v *Template) { v.Block.Transactions[0].Hash = strings.Repeat("0", 64) },
		"trailing tx":                func(v *Template) { v.Block.Transactions[0].Data += "00" },
		"duplicate tx":               func(v *Template) { v.Block.Transactions[1] = v.Block.Transactions[0] },
		"invented dependency":        func(v *Template) { v.Block.Transactions[0].Depends = []int64{1} },
		"excess dependencies":        func(v *Template) { v.Block.Transactions[0].Depends = make([]int64, len(v.Block.Transactions)+1) },
		"missing witness commitment": func(v *Template) { v.Block.DefaultWitnessCommitment = "" },
		"wrong witness commitment":   func(v *Template) { v.Block.DefaultWitnessCommitment = strings.Repeat("0", 64) },
		"script instead of raw witness": func(v *Template) {
			v.Block.DefaultWitnessCommitment = "6a24aa21a9ed" + v.Block.DefaultWitnessCommitment
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			copy := copyFixture(t, fixture)
			mutate(&copy.Template)
			if err := ValidateTemplate(copy.Template); err == nil {
				t.Fatal("accepted inconsistent assertion")
			}
			if _, err := Build(copy.Job, copy.Template, copy.PayoutAddress, copy.ExtraNonce); err == nil {
				t.Fatal("Build trusted unvalidated template")
			}
		})
	}
}

func replaceTransaction(t *testing.T, entry *btcjson.GetBlockTemplateResultTx, change func(*wire.MsgTx)) {
	t.Helper()
	raw, _ := hex.DecodeString(entry.Data)
	var transaction wire.MsgTx
	if transaction.Deserialize(bytes.NewReader(raw)) != nil {
		t.Fatal("fixture tx decode")
	}
	change(&transaction)
	var serialized bytes.Buffer
	if transaction.Serialize(&serialized) != nil {
		t.Fatal("fixture tx encode")
	}
	entry.Data = hex.EncodeToString(serialized.Bytes())
	entry.TxID = transaction.TxHash().String()
	entry.Hash = transaction.WitnessHash().String()
	entry.Weight = blockchain.GetTransactionWeight(btcutil.NewTx(&transaction))
}

func TestTemplateDependenciesUseCompleteIndex(t *testing.T) {
	fixture := templateReferences(t)[1]
	// The first transaction spends an output of the later one, with all local
	// transaction metadata recomputed. This must not be mistaken for an
	// external UTXO simply because it has not yet been visited.
	later, err := chainhash.NewHashFromStr(fixture.Template.Block.Transactions[1].TxID)
	if err != nil {
		t.Fatal(err)
	}
	replaceTransaction(t, &fixture.Template.Block.Transactions[0], func(tx *wire.MsgTx) { tx.TxIn[0].PreviousOutPoint = wire.OutPoint{Hash: *later, Index: 0} })
	if err := ValidateTemplate(fixture.Template); err == nil {
		t.Fatal("accepted forward dependency")
	}
	fixture = templateReferences(t)[1]
	fixture.Template.Block.Transactions[1].Depends = []int64{}
	if err := ValidateTemplate(fixture.Template); err == nil {
		t.Fatal("accepted missing dependency")
	}
	fixture = templateReferences(t)[1]
	fixture.Template.Block.Transactions[1].Depends = []int64{1, 1}
	if err := ValidateTemplate(fixture.Template); err == nil {
		t.Fatal("accepted duplicate dependency")
	}
}

func TestBuildChecksAddedCoinbaseLimitsAndPayout(t *testing.T) {
	fixture := templateReferences(t)[0]
	for name, mutate := range map[string]func(*Template){
		"bytes":  func(v *Template) { v.Block.SizeLimit = int64(fixture.ExpectedSerializedSize - 1) },
		"weight": func(v *Template) { v.Block.WeightLimit = fixture.Expected.Weight - 1 },
		"sigops": func(v *Template) { v.Block.SigOpLimit = 3 },
		"script": func(v *Template) { v.Block.CoinbaseAux.Flags = strings.Repeat("00", 100) },
	} {
		t.Run(name, func(t *testing.T) {
			copy := copyFixture(t, fixture)
			mutate(&copy.Template)
			if err := ValidateTemplate(copy.Template); err != nil {
				t.Fatalf("fixture should fit before coinbase construction: %v", err)
			}
			if _, err := Build(copy.Job, copy.Template, copy.PayoutAddress, copy.ExtraNonce); err == nil {
				t.Fatal("accepted oversized constructed coinbase/block")
			}
		})
	}
	for _, address := range []string{"", "OP_TRUE", fixture.Job.PublicKey, "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", strings.Repeat("a", 129)} {
		if _, err := Build(fixture.Job, fixture.Template, address, fixture.ExtraNonce); err == nil {
			t.Fatal("accepted absent/unsafe/foreign payout")
		}
	}
}

func TestBuildJobBindingAndOwnedOutput(t *testing.T) {
	fixture := templateReferences(t)[2]
	before := copyFixture(t, fixture)
	candidate, err := Build(fixture.Job, fixture.Template, fixture.PayoutAddress, math.MaxUint64)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fixture, before) {
		t.Fatal("Build mutated caller template")
	}
	saved := candidate
	saved.Proof.MerkleBranch = append([]string(nil), candidate.Proof.MerkleBranch...)
	fixture.Template.Block.CoinbaseAux.Flags = ""
	fixture.Template.Block.Transactions[0].Data = ""
	*fixture.Template.Block.CoinbaseValue = 0
	if !reflect.DeepEqual(candidate, saved) {
		t.Fatal("candidate retained mutable caller state")
	}
	fixture = templateReferences(t)[0]
	for name, mutate := range map[string]func(*prospectjob.Job){
		"ID":       func(job *prospectjob.Job) { job.ID = strings.Repeat("0", 64) },
		"parent":   func(job *prospectjob.Job) { job.Context.ParentHash = strings.Repeat("1", 64) },
		"policy":   func(job *prospectjob.Job) { job.Context.ProspectTarget = strings.Repeat("e", 64) },
		"old time": func(job *prospectjob.Job) { job.Context.MaxTimestamp = uint32(fixture.Template.Block.CurTime - 1) },
	} {
		t.Run(name, func(t *testing.T) {
			job := fixture.Job
			mutate(&job)
			if name != "ID" {
				var err error
				job, err = prospectjob.New(job.Context, job.PublicKey, job.Challenge, job.IssuedAtMS)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Build(job, fixture.Template, fixture.PayoutAddress, 0); err == nil {
				t.Fatal("accepted wrong job binding")
			}
		})
	}
	// An advancing current upper bound is compatible with the immutable job
	// interval; the selected header timestamp must remain inside both.
	fixture.Template.Context.MaxTimestamp++
	fixture.Template.Block.MaxTime++
	if _, err := Build(fixture.Job, fixture.Template, fixture.PayoutAddress, 0); err != nil {
		t.Fatal(err)
	}
}
