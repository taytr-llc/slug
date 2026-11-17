// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEntryTrustedSaveReplaceWithdrawReplay(t *testing.T) {
	x := newEntryFixture(t, 3)
	ctx := context.Background()
	r := x.request(t, "replace", 0)
	i, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	again, e := x.s.Prepare(ctx, x.session, r)
	if e != nil || !equal(i, again) {
		t.Fatal("prepare replay", e)
	}
	first, e := x.s.Apply(ctx, x.session, i.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	if first.Replayed || first.Receipt.Revision != 1 || len(first.Receipt.Members) != 3 {
		t.Fatal(first)
	}
	raw, e := MarshalReceipt(first.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ParseReceipt(raw); e != nil {
		t.Fatal(e)
	}
	x.reopen(t)
	retry, e := x.s.Apply(ctx, x.session, i.OperationID)
	if e != nil || !retry.Replayed || !equal(retry.Receipt, first.Receipt) {
		t.Fatal("durable replay", e)
	}
	// A distinct explicit save of identical members is a new revision.
	r.ExpectedRevision = 1
	i2, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	second, e := x.s.Apply(ctx, x.session, i2.OperationID)
	if e != nil || second.Receipt.Revision != 2 {
		t.Fatal("identical selection revision", e)
	}
	// Check retains a common verified as-of, while the entry clock is newer.
	check, e := x.s.Check(ctx, x.session, r.Saturday)
	if e != nil || len(check.Members) != 3 {
		t.Fatal("check", e)
	}
	if _, e = MarshalCheckResult(check); e != nil {
		t.Fatal("check codec", e)
	}
	// A roster move warns without silently replacing/withdrawing saved members.
	op, e := x.stable.PrepareSlot(ctx, x.session, gymstable.SlotRequest{ExpectedRevision: 3, Slot: 1, ProspectID: ""})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.stable.ApplySlot(ctx, x.session, op.OperationID); e != nil {
		t.Fatal(e)
	}
	status, e := x.s.Status(ctx, x.session, r.Saturday)
	if e != nil || !status.RosterChanged || status.Revision != 2 {
		t.Fatal("roster warning", status, e)
	}
	withdraw := x.request(t, "withdraw", 2)
	wi, e := x.s.Prepare(ctx, x.session, withdraw)
	if e != nil {
		t.Fatal(e)
	}
	wr, e := x.s.Apply(ctx, x.session, wi.OperationID)
	if e != nil || wr.Receipt.Revision != 3 || len(wr.Receipt.Members) != 0 || wr.Receipt.RosterRevision != 0 {
		t.Fatal("withdraw", e)
	}
	x.reopen(t)
	history, e := x.s.History(ctx, x.session, "", 8)
	if e != nil || len(history.Events) != 1 || history.Events[0].Revision != 3 {
		t.Fatal(history, e)
	}
	// A new view can inspect original history, but cannot Apply its intent.
	newer := entrySession(x.pin.StablePin, 1, strings.Repeat("b", 64))
	if _, e = x.s.Apply(ctx, newer, i.OperationID); !errors.Is(e, ErrStaleView) {
		t.Fatal("new view applied old intent", e)
	}
	got, e := x.s.Receipt(ctx, newer, i.OperationID)
	if e != nil || !equal(got, first.Receipt) {
		t.Fatal("historical receipt", e)
	}
}
func TestEntryFinalCutoffPrepareAndApply(t *testing.T) {
	for _, phase := range []string{"prepare", "apply"} {
		t.Run(phase, func(t *testing.T) {
			x := newEntryFixture(t, 0)
			ctx := context.Background()
			r := x.request(t, "withdraw", 0)
			ev, _ := EventForDate(x.pin, r.Saturday)
			x.now = ev.CutoffMS - 1
			var i Intent
			var e error
			if phase == "apply" {
				i, e = x.s.Prepare(ctx, x.session, r)
				if e != nil {
					t.Fatal(e)
				}
			}
			x.s.checkpoint = func(stage string) {
				if stage == "before_"+phase+"_guard" {
					x.now = ev.CutoffMS
				}
			}
			if phase == "prepare" {
				_, e = x.s.Prepare(ctx, x.session, r)
			} else {
				_, e = x.s.Apply(ctx, x.session, i.OperationID)
			}
			if !errors.Is(e, ErrClosed) {
				t.Fatal("equality did not close", e)
			}
			x.s.checkpoint = nil
			x.reopen(t)
			status, e := x.s.Status(ctx, x.session, r.Saturday)
			if e != nil || status.Revision != 0 || status.Phase != "closed_pending_lock" {
				t.Fatal(status, e)
			}
			if phase == "prepare" {
				if status.Exists || x.s.meta.OperationCount != 0 {
					t.Fatal("rejected prepare allocated")
				}
			} else {
				if !status.Exists || status.ClosedObservedAtMS != ev.CutoffMS {
					t.Fatal(status)
				}
				again, e := x.s.Prepare(ctx, x.session, r)
				if e != nil || again.OperationID != i.OperationID {
					t.Fatal("late pending recovery", e)
				}
				if _, e = x.s.Abandon(ctx, x.session, i.OperationID); e != nil {
					t.Fatal(e)
				}
			}
			x.now--
			if _, e = x.s.Pending(ctx, x.session); !errors.Is(e, ErrClock) {
				t.Fatal("rollback accepted", e)
			}
		})
	}
}
func TestEntryUnknownWritesAndPostCommitLeaseFailure(t *testing.T) {
	for _, phase := range []string{"prepare", "apply", "abandon"} {
		t.Run(phase, func(t *testing.T) {
			x := newEntryFixture(t, 0)
			ctx := context.Background()
			r := x.request(t, "withdraw", 0)
			var i Intent
			var e error
			if phase != "prepare" {
				i, e = x.s.Prepare(ctx, x.session, r)
				if e != nil {
					t.Fatal(e)
				}
			}
			real := x.s.writeBatch
			x.s.checkpoint = func(stage string) {
				if stage == "before_"+phase {
					x.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
						if e := real(b, o); e != nil {
							return e
						}
						return errors.New("lost sync ACK")
					}
				}
			}
			switch phase {
			case "prepare":
				_, e = x.s.Prepare(ctx, x.session, r)
			case "apply":
				_, e = x.s.Apply(ctx, x.session, i.OperationID)
			case "abandon":
				_, e = x.s.Abandon(ctx, x.session, i.OperationID)
			}
			if !errors.Is(e, ErrOutcomeUnknown) || !x.s.poisoned {
				t.Fatal("unknown not poisoned", e)
			}
			if _, e = x.s.Pending(ctx, x.session); !errors.Is(e, ErrPoisoned) {
				t.Fatal("poison reused", e)
			}
			x.reopen(t)
			if phase == "prepare" {
				p, e := x.s.Pending(ctx, x.session)
				if e != nil || p.Operation == nil {
					t.Fatal(e)
				}
			} else {
				o, e := x.s.Operation(ctx, x.session, i.OperationID)
				if e != nil || o.Operation.State == "prepared" {
					t.Fatal(e)
				}
			}
		})
	}
	t.Run("outer lease rejects committed callback", func(t *testing.T) {
		x := newEntryFixture(t, 0)
		ctx := context.Background()
		r := x.request(t, "withdraw", 0)
		i, e := x.s.Prepare(ctx, x.session, r)
		if e != nil {
			t.Fatal(e)
		}
		after := false
		x.s.checkpoint = func(stage string) {
			if stage == "after_apply" {
				after = true
			}
		}
		session := x.session
		original := session.Lease
		session.Lease = func(ctx context.Context, c gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
			e := original(ctx, c, fn)
			if e == nil && after {
				return gymaccount.ErrUnauthorized
			}
			return e
		}
		if _, e = x.s.Apply(ctx, session, i.OperationID); !errors.Is(e, ErrOutcomeUnknown) {
			t.Fatal(e)
		}
		x.reopen(t)
		o, e := x.s.Operation(ctx, x.session, i.OperationID)
		if e != nil || o.Operation.State != "committed" {
			t.Fatal(e)
		}
	})
}
func TestEntryMetadataBoundsHistoryAndCancellation(t *testing.T) {
	x := newEntryFixture(t, 0)
	ctx := context.Background()
	r := x.request(t, "withdraw", 0)
	status, e := x.s.Status(ctx, x.session, r.Saturday)
	if e != nil || status.Exists {
		t.Fatal(e)
	}
	check, e := x.s.Check(ctx, x.session, r.Saturday)
	if e != nil || len(check.Members) != 0 || x.s.meta.EventCount != 0 {
		t.Fatal(e)
	}
	if _, e = x.s.History(ctx, x.session, "", 0); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e = x.s.History(ctx, x.session, strings.Repeat("f", 64), 1); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	i, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.s.Receipt(ctx, x.session, i.OperationID); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	other := entrySession(x.pin.StablePin, 2, strings.Repeat("b", 64))
	if _, e = x.s.Abandon(ctx, other, i.OperationID); e != nil {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = x.s.Prepare(cancelled, x.session, r); !errors.Is(e, ErrCancelled) {
		t.Fatal(e)
	}
	x.s.gate <- struct{}{}
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	if _, e = x.s.Pending(short, x.session); !errors.Is(e, ErrCancelled) {
		t.Fatal(e)
	}
	<-x.s.gate
	x.reopen(t)
	p, e := x.s.Pending(ctx, x.session)
	if e != nil || p.Operation != nil {
		t.Fatal(e)
	}
}

func TestEntryFinalRetirementFromVerifiedDNA(t *testing.T) {
	x := newEntryFixture(t, 1)
	ctx := context.Background()
	item := x.items[0]
	retirement := item.Assignment.BornAtMS + int64(item.Manifest.DNA.PrimeWeeks+item.Manifest.DNA.DeclineWeeks)*604800000
	x.now = retirement - 1
	r := x.request(t, "replace", 0)
	i, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	x.s.checkpoint = func(stage string) {
		if stage == "before_apply_guard" {
			x.now = retirement
		}
	}
	if _, e = x.s.Apply(ctx, x.session, i.OperationID); !errors.Is(e, ErrRetired) {
		t.Fatal("final projection did not retire", e)
	}
	if x.s.meta.ClockMS != retirement {
		t.Fatal("retirement rejection lost observation")
	}
	x.s.checkpoint = nil
	x.reopen(t)
	status, e := x.s.Status(ctx, x.session, r.Saturday)
	if e != nil || status.Revision != 0 {
		t.Fatal(status, e)
	}
}
func TestEntryReopenRejectsSourceCheckpointRegression(t *testing.T) {
	x := newEntryFixture(t, 1)
	ctx := context.Background()
	if e := x.s.Close(); e != nil {
		t.Fatal(e)
	}
	x.s = nil
	replacement, e := gymstable.Create(filepath.Join(t.TempDir(), "replacement"), x.pin.StablePin, x.source)
	if e != nil {
		t.Fatal(e)
	}
	defer replacement.Close()
	opened, e := Open(ctx, x.path, x.pin, replacement)
	if opened != nil {
		opened.Close()
		t.Fatal("regressed source adopted")
	}
	if !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
}

func TestEntryCheckCapturedClockAndRevisionFence(t *testing.T) {
	for _, mode := range []string{"rollback-after-proof", "roster-after-proof"} {
		t.Run(mode, func(t *testing.T) {
			x := newEntryFixture(t, 1)
			ctx := context.Background()
			r := x.request(t, "replace", 0)
			i, e := x.s.Prepare(ctx, x.session, r)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = x.s.Apply(ctx, x.session, i.OperationID); e != nil {
				t.Fatal(e)
			}
			hookRan := false
			x.s.checkpoint = func(stage string) {
				if stage != "after_check_selection" {
					return
				}
				hookRan = true
				if mode == "rollback-after-proof" {
					prior := x.s.meta.ClockMS
					x.s.now = func() time.Time { return time.UnixMilli(prior) }
				} else {
					i, e := x.stable.PrepareSlot(ctx, x.session, gymstable.SlotRequest{ExpectedRevision: 1, Slot: 1})
					if e != nil {
						t.Fatal(e)
					}
					if _, e = x.stable.ApplySlot(ctx, x.session, i.OperationID); e != nil {
						t.Fatal(e)
					}
				}
			}
			got, e := x.s.Check(ctx, x.session, r.Saturday)
			want := ErrClock
			if mode == "roster-after-proof" {
				want = ErrConflict
			}
			if !hookRan || !errors.Is(e, want) || got.Version != 0 {
				t.Fatal("mixed observation disclosed", mode, e)
			}
			x.s.checkpoint = nil
			x.s.now = time.Now
			status, e := x.s.Status(ctx, x.session, r.Saturday)
			if e != nil || status.Revision != 1 {
				t.Fatal("saved history changed", e)
			}
		})
	}
}
