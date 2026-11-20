//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

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
	f := newResultFixture(t)
	ctx := context.Background()
	activateFixture(t, f)
	if e := f.s.Close(); e != nil {
		t.Fatal(e)
	}
	f.s = nil
	path := filepath.Join(f.dir, "result")
	before := archiveHashes(t, path)
	s, e := OpenCareerAudit(ctx, path, f.pin, f.bout)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Info(ctx); e != nil {
		t.Fatal(e)
	}
	archiveUnchanged(t, path, before, s.db, s.Close)
}
