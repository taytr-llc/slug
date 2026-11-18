// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFrozenReferenceJournalGraph(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal profile")
	}
	var corpus struct {
		Cases []struct {
			Name   string
			Rows   [][2]string
			Accept bool
		}
	}
	raw, e := os.ReadFile("testdata/reference/fragments/store.json")
	if e != nil || json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("store corpus", e)
	}
	f := loadReference(t)
	var sourceTime int64
	for _, x := range f.Positive {
		if x.Kind == "Info" {
			var info Info
			json.Unmarshal(x.Value, &info)
			sourceTime = info.SourceCreatedAtMS
			break
		}
	}
	id, _ := ID(f.Pin)
	for _, x := range corpus.Cases {
		t.Run(x.Name, func(t *testing.T) {
			db, e := leveldb.OpenFile(filepath.Join(t.TempDir(), "db"), nil)
			if e != nil {
				t.Fatal(e)
			}
			s := newStore(db, time.Now)
			defer s.Close()
			s.sourceCreatedAtMS = sourceTime
			b := new(leveldb.Batch)
			seen := map[string]bool{}
			for i, row := range x.Rows {
				k, e := hex.DecodeString(row[0])
				if e != nil {
					t.Fatal(e)
				}
				if seen[string(k)] {
					// LevelDB unique keys cannot retain the model's duplicate physical row.
					// Preserve its multiplicity as an extra invalid physical key, never silently
					// coalesce the corrupt model into the valid three-row journal.
					k = append(append([]byte(nil), k...), byte(i))
				}
				seen[string(k)] = true
				b.Put(k, []byte(row[1]))
			}
			if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
				t.Fatal(e)
			}
			e = s.audit(context.Background(), marker{"slug-candidate-results", 1, id, f.Pin})
			if (e == nil) != x.Accept {
				t.Fatalf("audit accept=%v want=%v error=%v", e == nil, x.Accept, e)
			}
		})
	}
}
func TestFrozenIndexCapacityBound(t *testing.T) {
	var v struct {
		Index struct {
			Rows           []indexRow
			CanonicalBytes int `json:"canonical_bytes"`
			Cap            int
		} `json:"index_capacity"`
	}
	raw, e := os.ReadFile("testdata/reference/fragments/store.json")
	if e != nil || json.Unmarshal(raw, &v) != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(v.Index.Rows)
	if len(v.Index.Rows) != 256 || len(b) != v.Index.CanonicalBytes || v.Index.Cap != MaxIndexSize || !indexBound(v.Index.Rows) {
		t.Fatal("independent index capacity")
	}
	if indexBound(append(v.Index.Rows, v.Index.Rows[0])) {
		t.Fatal("257th row accepted")
	}
}
