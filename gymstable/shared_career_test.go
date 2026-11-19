// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func TestSharedStableCareerRecentOriginalSessionAndCutoffFence(t *testing.T) {
	x := newSharedFixture(t, "BAB")
	a := x.gyms[0]
	ctx := context.Background()
	mustSync(t, a)
	id := a.records[0].Verified.ProspectID
	cutoffAssign(t, a, 0, 1, id)
	a.now++
	state, reader, failure := stableCareerOwner(t, a)
	if e := a.s.BindCareer(ctx, reader); e != nil {
		t.Fatal(e)
	}
	state.Events = []careerstate.Event{stableCareerEvent(state, id, a.now)}
	p, handle, e := a.s.PrepareCareerSharedRecent(ctx, a.session, 8)
	if e != nil || handle == nil || len(p.Items) != 1 || !reflect.DeepEqual(sharedSequences(p.Source.Items), []uint64{2}) || p.Source.NextAfter != 3 || p.Source.After != 0 || p.Items[0].Totals.XP != 1 || p.Source.Items[0].Condition.ProXP != 0 {
		t.Fatal("sparse career overlay", p, e)
	}
	changedView := a.session
	changedView.ViewID = strings.Repeat("c", 64)
	if value, e := a.s.ReadCareerInventory(ctx, changedView, handle); !errors.Is(e, ErrStaleView) || value.Version != 0 {
		t.Fatal("recent handle crossed view", e)
	}
	*failure = careerstate.ErrClosed
	if value, e := a.s.CareerSharedRecent(ctx, a.session, 8); !errors.Is(e, careerstate.ErrClosed) || value.Version != 0 {
		t.Fatal("unavailable career became XP0", e)
	}
	*failure = nil
	// The permanent award uses the same C1 exclusion and cannot be bypassed
	// merely because its original source position is sparse.
	if _, e := a.s.ReadEntrySelection(ctx, a.session, []string{id}); e == nil {
		t.Fatal("shared source bypassed professional-record exclusion")
	} else {
		var ineligible *CareerIneligibleError
		if !errors.As(e, &ineligible) || len(ineligible.ProspectIDs) != 1 || ineligible.ProspectIDs[0] != id {
			t.Fatal(e)
		}
	}
	cutoffRead(t, a, a.now, nil)
	if e = a.s.Close(); e != nil {
		t.Fatal(e)
	}
	a.s = nil
	if old, e := OpenShared(a.path, a.pin, a.source); e == nil {
		old.Close()
		t.Fatal("source-only writer reopened career-bound shared stable")
	}
	a.s, e = openSharedCareerWithClock(a.path, a.pin, a.source, func() time.Time { return time.UnixMilli(a.now) }, &state.Activation, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.s.AttachCareer(ctx, reader); e != nil {
		t.Fatal(e)
	}
	if value, e := a.s.ReadCareerInventory(ctx, a.session, handle); !errors.Is(e, ErrInvalid) || value.Version != 0 {
		t.Fatal("old handle crossed restart", e)
	}
	again, e := a.s.CareerSharedRecent(ctx, a.session, 8)
	if e != nil || again.Items[0].Totals != p.Items[0].Totals || again.CareerDigest != p.CareerDigest || again.Source.NextAfter != 3 {
		t.Fatal("career reopen changed sparse source/award", e)
	}
	it := a.s.db.NewIterator(util.BytesPrefix([]byte(careerconsumer.FencePrefix)), nil)
	if !it.Next() {
		it.Release()
		t.Fatal("missing cutoff receipt")
	}
	key := append([]byte{}, it.Key()...)
	it.Release()
	if e = a.s.db.Delete(key, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = a.s.Close(); e != nil {
		t.Fatal(e)
	}
	a.s = nil
	if reopened, e := OpenSharedCareer(a.path, a.pin, a.source, state.Activation); !errors.Is(e, ErrCorrupt) {
		if reopened != nil {
			reopened.Close()
		}
		t.Fatal("shared career accepted missing cutoff fence", e)
	}
}
