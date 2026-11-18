//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoutAuditedIndexRejectsChangedMissingAndUnexpectedRows(t *testing.T) {
	for _, kind := range []string{"changed", "missing", "unindexed"} {
		t.Run(kind, func(t *testing.T) {
			f := newBoutFixture(t)
			ctx := context.Background()
			r, e := f.run(ctx)
			if e != nil {
				t.Fatal(e)
			}
			key := key('r', r.Record.BoutID)
			raw, e := f.s.db.Get(key, nil)
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "changed":
				raw = append(raw, ' ')
				e = f.s.db.Put(key, raw, &opt.WriteOptions{Sync: true})
			case "missing":
				e = f.s.db.Delete(key, &opt.WriteOptions{Sync: true})
			case "unindexed":
				f.s.index = nil
			}
			if e != nil {
				t.Fatal(e)
			}
			calls := 0
			f.s.checkpoint = func(stage string) {
				if stage == "before-engine" {
					calls++
				}
			}
			if _, e = f.status(ctx); !errors.Is(e, ErrCorrupt) {
				t.Fatal("index drift disclosed", e)
			}
			if _, e = f.run(ctx); !errors.Is(e, ErrCorrupt) || calls != 0 {
				t.Fatal("corrupt replay simulated", e, calls)
			}
		})
	}
}
func TestBoutOpenAuditsResultAndCanonicalGraph(t *testing.T) {
	for _, kind := range []string{"engine", "extra-key", "count", "noncanonical"} {
		t.Run(kind, func(t *testing.T) {
			f := newBoutFixture(t)
			ctx := context.Background()
			r, e := f.run(ctx)
			if e != nil {
				t.Fatal(e)
			}
			f.s.Close()
			db, e := leveldb.OpenFile(filepath.Join(f.dir, "bout", "db"), nil)
			if e != nil {
				t.Fatal(e)
			}
			b := new(leveldb.Batch)
			switch kind {
			case "engine":
				r.Record.Result.DrawCounts.Combat++
				r.Record.ResultDigest = resultDigest(r.Record.Result)
				raw, _ := json.Marshal(r.Record)
				b.Put(key('r', r.Record.BoutID), raw)
			case "extra-key":
				b.Put([]byte("unexpected"), []byte("0"))
			case "count":
				m := f.s.meta
				m.BoutCount = 0
				put(b, []byte("meta"), m, MaxMetadataSize)
			case "noncanonical":
				raw, _ := json.MarshalIndent(r.Record, "", " ")
				b.Put(key('r', r.Record.BoutID), raw)
			}
			if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
				t.Fatal(e)
			}
			db.Close()
			if _, e = open(ctx, filepath.Join(f.dir, "bout"), f.p, f.card, f.clock, false); !errors.Is(e, ErrCorrupt) {
				t.Fatal("corrupt journal accepted", e)
			}
		})
	}
}
func TestBoutNoEffectsForInvalidProfileAndReadyMissing(t *testing.T) {
	f := newBoutFixture(t)
	ctx := context.Background()
	if _, e := f.s.Run(nil, f.card, f.seed, f.event, f.entry, f.session, f.date(), f.id()); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := f.s.Status(ctx, nil, f.seed, f.event, f.entry, f.session, f.date(), f.id()); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := f.s.Status(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date(), strings.Repeat("f", 64)); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	f.s.Close()
	if e := os.Rename(filepath.Join(f.dir, "bout", "db"), filepath.Join(f.dir, "preserved-db")); e != nil {
		t.Fatal(e)
	}
	if _, e := open(ctx, filepath.Join(f.dir, "bout"), f.p, f.card, f.clock, false); e == nil {
		t.Fatal("missing ready DB recreated")
	}
	if _, e := os.Stat(filepath.Join(f.dir, "bout", "db")); !os.IsNotExist(e) {
		t.Fatal("new ready DB appeared")
	}
}
