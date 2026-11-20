// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"crypto/sha256"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func archiveHashes(t *testing.T, path string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	e := filepath.WalkDir(path, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(path, p)
		if e != nil {
			return e
		}
		out[rel] = sha256.Sum256(b)
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func archiveUnchanged(t *testing.T, path string, before map[string][32]byte, db *leveldb.DB, close func() error) {
	t.Helper()
	if e := db.Put([]byte("archive-write-probe"), []byte("must not persist"), nil); !errors.Is(e, leveldb.ErrReadOnly) {
		t.Fatal("archive admitted mutation", e)
	}
	if e := close(); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, archiveHashes(t, path)) {
		t.Fatal("audit changed physical archive bytes")
	}
}

func TestArchiveReadOnlyFullAudit(t *testing.T) {
	x := newSharedFixture(t, "AB")
	f := x.gyms[0]
	ctx := context.Background()
	state, reader, _ := stableCareerOwner(t, f)
	if e := f.s.BindCareer(ctx, reader); e != nil {
		t.Fatal(e)
	}
	// A stale observed source highwater is valid and must not be merged on audit.
	next := f.s.meta
	next.ObservedSourceCount = 0
	if e := f.s.commitLocked(new(leveldb.Batch), next); e != nil {
		t.Fatal(e)
	}
	if e := f.s.Close(); e != nil {
		t.Fatal(e)
	}
	f.s = nil
	before := archiveHashes(t, f.path)
	s, e := OpenSharedCareerAudit(f.path, f.pin, f.source, state.Activation)
	if e != nil {
		t.Fatal(e)
	}
	if s.meta.ObservedSourceCount != 0 {
		t.Fatal("archive merged source highwater")
	}
	if _, e = s.EntrySourceInfo(ctx); e != nil {
		t.Fatal(e)
	}
	archiveUnchanged(t, f.path, before, s.db, s.Close)
	db, e := leveldb.OpenFile(filepath.Join(f.path, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Delete([]byte("meta"), nil); e != nil {
		t.Fatal(e)
	}
	db.Close()
	if bad, e := OpenSharedCareerAudit(f.path, f.pin, f.source, state.Activation); bad != nil || e == nil {
		if bad != nil {
			bad.Close()
		}
		t.Fatal("missing metadata accepted", e)
	}
}
