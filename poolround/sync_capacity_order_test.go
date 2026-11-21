// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// The inherited public fixture already has one accepted winning share, while
// this round store has consumed nothing. Its minimum legal byte limit cannot
// retain even one nonempty round plus the required status reservation. Capacity
// must not hide corruption discovered by the final complete namespace audit.
func TestRoundSyncCapacityDoesNotMaskOldNamespaceCorruption(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		name := "clean-byte-capacity"
		if corrupt {
			name = "unknown-key-before-byte-capacity"
		}
		t.Run(name, func(t *testing.T) {
			h := newRoundHarness(t, false)
			ctx := context.Background()
			page, err := h.source.AccountingPage(ctx, 0, 0, 8)
			if err != nil || page.Through != 3 || len(page.Entries) != 3 || !page.Entries[2].Share.Block {
				t.Fatal("fixture lacks the original unsynced winner", err)
			}
			if err := h.store.Close(); err != nil {
				t.Fatal(err)
			}
			h.path = filepath.Join(t.TempDir(), "byte-capacity-rounds")
			h.limits = Limits{Version: Version1, MaxRounds: 1, MaxBytes: statusReserve}
			h.store, err = Create(h.path, h.limits, h.source, h.observer)
			if err != nil {
				t.Fatal(err)
			}
			if h.store.meta.Through != 0 || h.store.meta.Rounds != 0 || h.store.meta.Bytes != 0 {
				t.Fatal("constructor consumed fixture history")
			}
			if corrupt {
				if err := h.store.db.Put([]byte("unexpected"), []byte("public corruption fixture"), &opt.WriteOptions{Sync: true}); err != nil {
					t.Fatal(err)
				}
			}
			beforeRows := walkTestRows(t, h.store)
			beforeMeta := h.store.meta
			commits := 0
			h.store.writeBatch = func(*leveldb.Batch, *opt.WriteOptions) error {
				commits++
				return errors.New("unexpected commit during refused sync")
			}
			got, err := h.store.Sync(ctx)
			want := ErrCapacity
			if corrupt {
				want = ErrCorrupt
			}
			if err != want || got != (Summary{}) {
				t.Fatal("wrong refusal precedence or partial summary", got, err, "want", want)
			}
			if commits != 0 || h.store.poisoned || h.store.meta != beforeMeta || !reflect.DeepEqual(beforeRows, walkTestRows(t, h.store)) {
				t.Fatal("refused sync changed the original journal")
			}
			if h.observer.calls != 0 {
				t.Fatal("sync obtained a fresh node observation")
			}
		})
	}
}
