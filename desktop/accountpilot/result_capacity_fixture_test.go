//go:build slugtestclock && !slugtestpow && !race

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymresult"
	"path/filepath"
	"testing"
)

// This constructs explicitly modeled prepared journal rows using the published
// candidate statement fixture. It is storage/latency stress data, not a new
// reference vector, admitted chain history or a production signing interface.
func resultCapacityPrepared(t *testing.T, p gymresult.Pin, r gymbout.Record, sourceCreated, created int64) gymresult.Record {
	t.Helper()
	raw, _ := json.Marshal(r)
	source := gymbout.ResultSource{Version: 1, StoreID: r.StoreID, StoreCreatedAtMS: sourceCreated, RecordDigest: boutCapacityHash("SLUG candidate bout record\x00v1\x00", boutCapacityLP(raw)), SourceDigest: r.SourceDigest, InputID: r.InputID, ResultDigest: r.ResultDigest, CreatedAtMS: r.CreatedAtMS, Source: r.Source, Summary: gymbout.ResultSummary{Winner: r.Result.Winner, Method: r.Result.Method, EndRound: r.Result.EndRound, EndTick: r.Result.EndTick}}
	id, e := gymresult.ID(p)
	if e != nil {
		t.Fatal(e)
	}
	key, e := gymresult.ResultKey(p, r.CardID, r.BoutID)
	if e != nil {
		t.Fatal(e)
	}
	st := gymresult.Statement{Version: 1, ResultKey: key, StoreID: id, SourceStoreID: p.SourceStoreID, Source: source, PublisherChainID: p.PublisherChainID, Network: p.Network, Genesis: p.Genesis, RulesID: p.RulesID, PublisherPublicKey: p.PublisherPublicKey, CheckpointPublicKey: p.CheckpointPublicKey}
	raw, _ = json.Marshal(st)
	seed := sha256.Sum256([]byte("SLUG candidate result statement fixture\x00v1\x00"))
	secret := ed25519.NewKeyFromSeed(seed[:])
	sig := ed25519.Sign(secret, append([]byte("SLUG candidate result statement\x00v1\x00"), boutCapacityLP(raw)...))
	clear(secret)
	out := gymresult.Record{Version: 1, StoreID: id, ResultKey: key, Statement: gymresult.SignedStatement{Version: 1, Statement: st, Signature: hex.EncodeToString(sig)}, PreparedAtMS: max(created, r.CreatedAtMS), Cursor: gymresult.Cursor{Height: -1, CompleteToHeight: -1, ScanDigest: boutCapacityHash("SLUG candidate result inclusion scan\x00v1\x00", boutCapacityRaw(id), boutCapacityRaw(key), make([]byte, 8), boutCapacityRaw(p.Genesis))}, ScanRestarts: []gymresult.ScanRestart{}, PriorInclusions: []gymresult.PriorInclusion{}, Conflicts: []gymresult.Conflict{}, Observation: gymresult.Observation{NodeState: "unknown"}}
	if _, e = gymresult.ValidateRecord(p, out); e != nil {
		t.Fatal("modeled prepared record", e)
	}
	return out
}
func resultCapacityWrite(t *testing.T, path string, batch *leveldb.Batch, counts map[string]any) {
	t.Helper()
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	meta, e := db.Get([]byte("meta"), nil)
	if e != nil {
		t.Fatal(e)
	}
	var old map[string]json.RawMessage
	if json.Unmarshal(meta, &old) != nil {
		t.Fatal("modeled meta")
	}
	for k, v := range counts {
		raw, _ := json.Marshal(v)
		needle := append([]byte(`"`+k+`":`), old[k]...)
		if bytes.Count(meta, needle) != 1 {
			t.Fatal("exact metadata field", k)
		}
		meta = bytes.Replace(meta, needle, append([]byte(`"`+k+`":`), raw...), 1)
	}
	batch.Put([]byte("meta"), meta)
	if e = db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
}
