// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestDailyPublicBindingOriginalOwner(t *testing.T) {
	h, _ := dailyFixture(t, 8)
	ctx := context.Background()
	before := dailyRows(t, h.pool)
	got, err := h.pool.DailyBinding(ctx)
	if err != nil || got != h.config || dailyRows(t, h.pool) != before || h.pool.operatorAdmission != nil {
		t.Fatal("binding changed or enabled original source", err)
	}
	h.pool.gate <- struct{}{}
	_, err = h.pool.DailyBinding(ctx)
	<-h.pool.gate
	if !errors.Is(err, ErrBusy) {
		t.Fatal("binding queued behind original operation", err)
	}
	h.pool.auditOnly = true
	_, err = h.pool.DailyBinding(ctx)
	h.pool.auditOnly = false
	if !errors.Is(err, ErrConfig) {
		t.Fatal("audit-only owner granted service binding", err)
	}
	// A valid-looking replacement metadata row cannot be adopted on failure.
	original := h.pool.meta
	changed := original
	changed.Bytes++
	raw, _ := json.Marshal(changed)
	if err = h.pool.db.Put([]byte("meta"), raw, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if got, err = h.pool.DailyBinding(ctx); !errors.Is(err, ErrCorrupt) || got != (Config{}) || h.pool.meta != original {
		t.Fatal("binding adopted changed metadata", err)
	}
	raw, _ = json.Marshal(original)
	if err = h.pool.db.Put([]byte("meta"), raw, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if err = h.pool.db.Put([]byte("z/unrelated-tail"), []byte("bad"), &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if got, err = h.pool.DailyBinding(ctx); !errors.Is(err, ErrCorrupt) || got != (Config{}) {
		t.Fatal("binding hid unrelated malformed namespace", err)
	}
	if err = h.pool.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = h.pool.DailyBinding(ctx); !errors.Is(err, ErrClosed) {
		t.Fatal("binding survived original close", err)
	}
}

func TestDailyPublicBindingCheapRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile refusal")
	}
	if got, err := (*Pool)(nil).DailyBinding(nil); !errors.Is(err, ErrProfile) || got != (Config{}) {
		t.Fatal("cheap profile reached owner", err)
	}
}
