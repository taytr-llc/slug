//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

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
	f := newCardFixture(t)
	ctx := context.Background()
	state, reader := cardCareerOwner(t, f)
	if e := f.s.BindCareer(ctx, reader); e != nil {
		t.Fatal(e)
	}
	if e := f.s.Close(); e != nil {
		t.Fatal(e)
	}
	f.s = nil
	before := archiveHashes(t, f.path)
	s, e := OpenCareerAudit(ctx, f.path, f.p, f.seed, state.Activation)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Info(ctx); e != nil {
		t.Fatal(e)
	}
	archiveUnchanged(t, f.path, before, s.db, s.Close)
}
