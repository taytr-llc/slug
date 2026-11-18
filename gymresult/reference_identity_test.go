// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymbout"
	"os"
	"path/filepath"
	"testing"
)

func TestFrozenResultReferenceInventory(t *testing.T) {
	base := "testdata/reference"
	raw, e := os.ReadFile(filepath.Join(base, "file_index.json"))
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "12ebe693a757467df55e0cb474be62e1b2ed01516dad6112a9841dd26679fb8f" {
		t.Fatal("frozen reference index changed")
	}
	var idx struct {
		CorpusSHA256 string `json:"corpus_sha256"`
		FileCount    int    `json:"file_count"`
		Files        map[string]struct {
			SHA256 string `json:"sha256"`
			Bytes  int    `json:"bytes"`
		}
	}
	if json.Unmarshal(raw, &idx) != nil || idx.FileCount != 139 || len(idx.Files) != 139 || idx.CorpusSHA256 != "07c00eef8d87ae9ae5dcd2d120a5dfd1f7b1ba33a1447b773cfd4d5194fd1a94" {
		t.Fatal("reference inventory identity")
	}
	for name, row := range idx.Files {
		p := filepath.Join(base, name)
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(name, e)
		}
		h := sha256.Sum256(b)
		if len(b) != row.Bytes || hex.EncodeToString(h[:]) != row.SHA256 {
			t.Fatal("frozen bytes changed", name)
		}
	}
}
func TestFrozenResultIdentityEquations(t *testing.T) {
	f := loadReference(t)
	fact := func(k string) string {
		var s string
		if json.Unmarshal(f.Facts[k], &s) != nil {
			t.Fatal("fact", k)
		}
		return s
	}
	var statement Statement
	var source gymbout.ResultSource
	for _, p := range f.Positive {
		if p.Kind == "Statement" {
			if e := json.Unmarshal(p.Value, &statement); e != nil {
				t.Fatal(e)
			}
			break
		}
	}
	source = statement.Source
	id, e := ID(f.Pin)
	if e != nil || id != fact("store_id") || RulesID() != fact("rules_id") {
		t.Fatal("store/rules equations", e)
	}
	key, e := ResultKey(f.Pin, source.Source.CardID, source.Source.Bout.BoutID)
	if e != nil || key != fact("result_key") {
		t.Fatal("result key", e)
	}
	sd, e := gymbout.ResultSourceDigest(f.Pin.SourcePin, source)
	if e != nil || sd != fact("result_source_digest") {
		t.Fatal("source digest", e)
	}
	sid, e := StatementID(statement)
	if e != nil || sid != fact("statement_id") || hex.EncodeToString(statementMessage(statement)) != fact("statement_signing_bytes") {
		t.Fatal("statement framing", e)
	}
	carrier, e := Carrier(statement)
	if e != nil || hex.EncodeToString(carrier[:]) != fact("carrier") {
		t.Fatal("carrier", e)
	}
	for k, v := range map[string]string{"publisher_public_key": f.Pin.PublisherPublicKey, "checkpoint_public_key": f.Pin.CheckpointPublicKey, "funding_public_key": f.Pin.FundingPublicKey, "funding_script": f.Pin.FundingScript} {
		if v != fact(k) {
			t.Fatal("fixture identity", k)
		}
	}
	// Independent alternate valid witness bytes must remain acceptable. This is
	// signature verification only, never recreating the first transaction.
	var pub Publication
	for _, p := range f.Positive {
		if p.Kind == "Publication" {
			json.Unmarshal(p.Value, &pub)
			break
		}
	}
	pub.RawTx = fact("alternate_raw_tx")
	if validatePublication(f.Pin, pub) != nil {
		t.Fatal("alternate retained valid signature rejected")
	}
}
