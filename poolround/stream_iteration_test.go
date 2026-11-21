// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// This fixture exercises only the private, second-pass record decoder. These
// deliberately modeled records have NOT passed verifyStream, do not authorize
// accounting, and must never be supplied through WalkSettlement as a source.
func TestRoundWalkIterationOwnedOutputPageBoundary(t *testing.T) {
	cfg, entries, _ := modeledReference(t)
	rounds, err := derive(cfg, entries[:3])
	if err != nil || len(rounds) != 1 {
		t.Fatal(err)
	}
	db, err := leveldb.OpenFile(filepath.Join(t.TempDir(), "iteration-only"), &opt.Options{ErrorIfExist: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	s := &Store{db: db, meta: metadata{Rounds: 9}}
	batch := new(leveldb.Batch)
	want := make([]Record, 0, 9)
	for n := uint64(1); n <= 9; n++ {
		round := rounds[0]
		round.Sequence, round.BlockHash = n, fmt.Sprintf("%064x", n+1000)
		row := Record{Round: round}
		raw, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		batch.Put(roundKey(round.BlockHash), raw)
		batch.Put(indexKey(n), []byte(round.BlockHash))
		want = append(want, row)
	}
	if err := db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	before := walkTestRows(t, s)
	var pages [][]Record
	err = s.walkRecords(context.Background(), func(page []Record) error {
		pages = append(pages, page) // retain prior page to expose accidental reuse
		return nil
	})
	if err != nil || len(pages) != 2 || len(pages[0]) != 8 || len(pages[1]) != 1 {
		t.Fatal("wrong eight-record boundary", err)
	}
	got := append(append([]Record(nil), pages[0]...), pages[1]...)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("decoder reordered records or reused a prior page")
	}
	pages[0][0].Round.ShareKeys[0] = "changed"
	pages[0][0].Round.Winner.Proof.Header = "changed"
	if pages[1][0].Round.ShareKeys[0] == "changed" || pages[1][0].Round.Winner.Proof.Header == "changed" {
		t.Fatal("separate decoded pages alias nested values")
	}
	if !reflect.DeepEqual(before, walkTestRows(t, s)) {
		t.Fatal("decoder wrote modeled journal")
	}
}
