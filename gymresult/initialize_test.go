//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"os"
	"path/filepath"
	"testing"
)

func TestFrozenInitializationModels(t *testing.T) {
	f := newResultFixture(t)
	var models struct {
		Cases []struct {
			Name, Existing, ParentPhase string
			Accept, PreserveCreatedAt   bool
		} `json:"initialization"`
	}
	raw, e := os.ReadFile("testdata/reference/fragments/store.json")
	if e != nil || json.Unmarshal(raw, &models) != nil {
		t.Fatal(e)
	}
	// JSON uses explicit snake case fields, independent of Go field-name matching.
	var source struct {
		Initialization []struct {
			Name, Existing    string
			ParentPhase       string `json:"parent_phase"`
			Accept            bool
			PreserveCreatedAt bool `json:"preserve_created_at"`
		}
	}
	if json.Unmarshal(raw, &source) != nil {
		t.Fatal("models")
	}
	for _, x := range source.Initialization {
		t.Run(x.Name, func(t *testing.T) {
			path := filepath.Join(f.dir, "init-"+x.Name)
			id, _ := ID(f.pin)
			mk := marker{"slug-candidate-results", 1, id, f.pin}
			m := f.s.meta
			m.ClockMS = max(m.ClockMS, recordMaxTime(referenceRecord(t, "record-finalized60")))
			oldCreated := m.CreatedAtMS
			if x.Existing != "absent" {
				if e := os.Mkdir(path, 0700); e != nil {
					t.Fatal(e)
				}
			}
			if x.Existing == "symlink" {
				os.Remove(path)
				if e = os.Symlink(filepath.Join(f.dir, "result"), path); e != nil {
					t.Fatal(e)
				}
			} else if x.Existing == "unknown" {
				if e = os.WriteFile(filepath.Join(path, "unknown"), []byte("x"), 0600); e != nil {
					t.Fatal(e)
				}
			} else if x.Existing != "absent" && x.Existing != "empty" {
				if x.Existing == "wrong-pin" {
					mk.Pin.RulesID = hash("wrong rules")
				}
				b, _ := json.Marshal(mk)
				if e = os.WriteFile(filepath.Join(path, markerName), append(b, '\n'), 0600); e != nil {
					t.Fatal(e)
				}
				if x.Existing != "marker" && x.Existing != "wrong-pin" {
					db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
					if e != nil {
						t.Fatal(e)
					}
					batch := new(leveldb.Batch)
					if x.Existing == "init" || x.Existing == "wrong-source-time" {
						src := m.SourceCreatedAtMS
						if x.Existing == "wrong-source-time" {
							src++
						}
						put(batch, []byte("init"), initIntent{1, id, f.pin, src}, MaxMetadataSize)
					} else {
						if x.Existing == "populated" {
							rec := referenceRecord(t, "record-finalized60")
							m.CreatedAtMS = min(m.CreatedAtMS, rec.PreparedAtMS)
							m.ResultCount = 1
							put(batch, key('r', rec.ResultKey), rec, MaxRecordSize)
							put(batch, coinKey('u', rec.Publication.Coin.Outpoint), reservation{1, rec.ResultKey, rec.Publication.TxID}, 256)
						}
						put(batch, []byte("meta"), m, MaxMetadataSize)
					}
					if e = db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
						t.Fatal(e)
					}
					db.Close()
				}
			}
			got, e := open(context.Background(), path, f.pin, f.bout, f.clock, x.ParentPhase == "prepared")
			if (e == nil) != x.Accept {
				if got != nil {
					got.Close()
				}
				t.Fatalf("accept=%v want=%v error=%v", e == nil, x.Accept, e)
			}
			if got != nil {
				if x.PreserveCreatedAt && got.meta.CreatedAtMS != oldCreated {
					t.Fatal("creation time replaced")
				}
				got.Close()
			}
		})
	}
}
