// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEntryStrictCodecOwnsArraysAndRejectsAmbiguity(t *testing.T) {
	x := newEntryFixture(t, 0)
	r := x.request(t, "withdraw", 0)
	raw, e := MarshalRequest(r)
	if e != nil {
		t.Fatal(e)
	}
	bad := [][]byte{append(append([]byte{}, raw...), raw...), append([]byte{0xef, 0xbb, 0xbf}, raw...), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"\u0076ersion":1`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1e0`), 1), bytes.Replace(raw, []byte(`"prospect_ids":[]`), []byte(`"prospect_ids":null`), 1), bytes.Replace(raw, []byte(`"action":"withdraw"`), []byte(`"action":"withdraw","unknown":1`), 1), bytes.Replace(raw, []byte(`"expected_revision":0`), []byte(`"expected_revision":-0`), 1)}
	for i, b := range bad {
		if _, e := ParseRequest(b); e == nil {
			t.Fatal("ambiguous JSON accepted", i)
		}
	}
	replace := r
	replace.Action = "replace"
	replace.ProspectIDs = []string{strings.Repeat("1", 64)}
	copy, e := ValidateRequest(replace)
	if e != nil {
		t.Fatal(e)
	}
	replace.ProspectIDs[0] = strings.Repeat("2", 64)
	if copy.ProspectIDs[0] == replace.ProspectIDs[0] {
		t.Fatal("borrowed slice")
	}
	replace.ProspectIDs = make([]string, 6)
	if _, e = ValidateRequest(replace); e == nil {
		t.Fatal("six selected IDs")
	}
	// Structural parser accepts harmless transport whitespace and escaped keys.
	escaped := bytes.Replace(raw, []byte(`"version"`), []byte(`"\u0076ersion"`), 1)
	if _, e = ParseRequest(append([]byte(" \n"), escaped...)); e != nil {
		t.Fatal(e)
	}
}
func TestEntryCalendarBoundarySemantics(t *testing.T) {
	x := newEntryFixture(t, 0)
	for _, date := range []string{"2025-12-27", "2100-01-02", "2026-02-30", "2026-11-20", "2026-1-03", " 2026-01-03", "２０２６-01-03"} {
		if _, e := EventForDate(x.pin, date); !errors.Is(e, ErrCalendar) {
			t.Fatal(date, e)
		}
	}
	// These are semantic boundary checks; exact independently derived calendar
	// hashes/cutoffs are supplied by the separately frozen reference corpus.
	for _, date := range []string{"2026-03-07", "2026-03-14", "2026-10-31", "2026-11-07", "2099-12-26"} {
		ev, e := EventForDate(x.pin, date)
		if e != nil {
			t.Fatal(e)
		}
		before, e := upcomingDate(ev.CutoffMS - 1)
		if e != nil || before != date {
			t.Fatal(before, e)
		}
		next, e := upcomingDate(ev.CutoffMS)
		if date == "2099-12-26" {
			if !errors.Is(e, ErrCalendar) {
				t.Fatal(next, e)
			}
		} else if e != nil || next == date {
			t.Fatal("equality did not advance", next, e)
		}
	}
}
func TestEntryOpenRejectsGraphAndMarkerCorruption(t *testing.T) {
	for _, kind := range []string{"missing-operation", "missing-index", "unknown-key", "bad-event-revision", "bad-receipt-clock", "wrong-index", "missing-meta", "oversized-meta", "marker-symlink"} {
		t.Run(kind, func(t *testing.T) {
			x := newEntryFixture(t, 0)
			ctx := context.Background()
			r := x.request(t, "withdraw", 0)
			i, e := x.s.Prepare(ctx, x.session, r)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = x.s.Apply(ctx, x.session, i.OperationID); e != nil {
				t.Fatal(e)
			}
			if e = x.s.Close(); e != nil {
				t.Fatal(e)
			}
			x.s = nil
			if kind == "marker-symlink" {
				p := filepath.Join(x.path, markerName)
				if e = os.Rename(p, p+".old"); e != nil {
					t.Fatal(e)
				}
				if e = os.Symlink(p+".old", p); e != nil {
					t.Fatal(e)
				}
			} else {
				db, e := leveldb.OpenFile(filepath.Join(x.path, "db"), nil)
				if e != nil {
					t.Fatal(e)
				}
				b := new(leveldb.Batch)
				switch kind {
				case "missing-operation":
					b.Delete(key("o", i.OperationID))
				case "missing-index":
					b.Delete(seqKey(i.Sequence))
				case "unknown-key":
					b.Put([]byte("unknown"), []byte("{}"))
				case "bad-event-revision":
					raw, _ := db.Get(key("e", r.EventID), nil)
					var row eventRow
					json.Unmarshal(raw, &row)
					row.Revision++
					put(b, key("e", r.EventID), row, MaxEventSize)
				case "bad-receipt-clock":
					raw, _ := db.Get(key("o", i.OperationID), nil)
					var o Operation
					json.Unmarshal(raw, &o)
					o.Receipt.CommittedAtMS = 1
					put(b, key("o", i.OperationID), o, MaxOperationSize)
				case "wrong-index":
					b.Put(seqKey(i.Sequence), make([]byte, 32))
				case "missing-meta":
					b.Delete([]byte("meta"))
				case "oversized-meta":
					b.Put([]byte("meta"), make([]byte, MaxMetadataSize+1))
				}
				if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
					t.Fatal(e)
				}
				db.Close()
			}
			opened, e := Open(ctx, x.path, x.pin, x.stable)
			if opened != nil {
				opened.Close()
				t.Fatal("corruption returned handle")
			}
			if !errors.Is(e, ErrCorrupt) {
				t.Fatal(kind, e)
			}
		})
	}
}
func TestEntryCloseCancelsQueuedWorkAndBorrowsSource(t *testing.T) {
	x := newEntryFixture(t, 0)
	x.s.gate <- struct{}{}
	returned := make(chan error, 1)
	go func() { _, e := x.s.Pending(context.Background(), x.session); returned <- e }()
	closed := make(chan error, 1)
	go func() { closed <- x.s.Close() }()
	select {
	case e := <-returned:
		if !errors.Is(e, ErrCancelled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("pending did not cancel")
	}
	select {
	case <-closed:
		t.Fatal("Close did not join actual gate")
	default:
	}
	<-x.s.gate
	if e := <-closed; e != nil {
		t.Fatal(e)
	}
	if _, e := x.stable.EntrySourceInfo(context.Background()); e != nil {
		t.Fatal("entry closed borrowed source", e)
	}
}
func TestEntryRandomCollisionBound(t *testing.T) {
	x := newEntryFixture(t, 0)
	ctx := context.Background()
	r := x.request(t, "withdraw", 0)
	x.s.random = bytes.NewReader(make([]byte, 32))
	i, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.s.Abandon(ctx, x.session, i.OperationID); e != nil {
		t.Fatal(e)
	}
	reader := bytes.NewReader(make([]byte, 8*32+1))
	x.s.random = reader
	if _, e = x.s.Prepare(ctx, x.session, r); !errors.Is(e, ErrCapacity) {
		t.Fatal(e)
	}
	if reader.Len() != 1 {
		t.Fatal("unbounded collisions", reader.Len())
	}
	if x.s.meta.OperationCount != 1 {
		t.Fatal("failed prepare wrote")
	}
	x.reopen(t)
}

func TestEntryCreatePreparedEmptyDirectoryOnly(t *testing.T) {
	x := newEntryFixture(t, 0)
	base := t.TempDir()
	ctx := context.Background()
	for _, kind := range []string{"absent", "empty", "nonempty", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(base, kind)
			switch kind {
			case "empty", "nonempty":
				if e := os.Mkdir(path, 0700); e != nil {
					t.Fatal(e)
				}
				if kind == "nonempty" {
					if e := os.WriteFile(filepath.Join(path, "retained"), []byte("evidence"), 0600); e != nil {
						t.Fatal(e)
					}
				}
			case "symlink":
				target := filepath.Join(base, "target")
				os.Mkdir(target, 0700)
				if e := os.Symlink(target, path); e != nil {
					t.Fatal(e)
				}
			}
			s, e := Create(ctx, path, x.pin, x.stable)
			if kind == "absent" || kind == "empty" {
				if e != nil {
					t.Fatal(e)
				}
				s.Close()
			} else {
				if s != nil {
					s.Close()
					t.Fatal("unknown directory adopted")
				}
				if e == nil {
					t.Fatal("nonempty/link accepted")
				}
				if kind == "nonempty" {
					b, e := os.ReadFile(filepath.Join(path, "retained"))
					if e != nil || string(b) != "evidence" {
						t.Fatal("evidence removed")
					}
				}
			}
		})
	}
}
func TestEntryRealOperationCapacityKeepsTerminalReservation(t *testing.T) {
	x := newEntryFixture(t, 0)
	ctx := context.Background()
	r := x.request(t, "withdraw", 0)
	for n := 0; n < MaxOperations-1; n++ {
		i, e := x.s.Prepare(ctx, x.session, r)
		if e != nil {
			t.Fatalf("prepare %d: %v", n, e)
		}
		if _, e = x.s.Abandon(ctx, x.session, i.OperationID); e != nil {
			t.Fatal(e)
		}
	}
	i, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	if x.s.meta.OperationCount != MaxOperations {
		t.Fatal("test did not reach cap")
	}
	saved, e := x.s.Apply(ctx, x.session, i.OperationID)
	if e != nil || saved.Receipt.Revision != 1 {
		t.Fatal("reserved completion", e)
	}
	r.ExpectedRevision = 1
	if _, e = x.s.Prepare(ctx, x.session, r); !errors.Is(e, ErrCapacity) {
		t.Fatal("cap admitted new intent", e)
	}
	replay, e := x.s.Apply(ctx, x.session, i.OperationID)
	if e != nil || !replay.Replayed {
		t.Fatal("cap blocked historical replay", e)
	}
	x.reopen(t)
	if x.s.meta.OperationCount != MaxOperations {
		t.Fatal("count lost at reopen")
	}
}
func TestEntryRealEventCapacityPreservesExistingEvent(t *testing.T) {
	x := newEntryFixture(t, 0)
	ctx := context.Background()
	first := x.request(t, "withdraw", 0)
	date, _ := time.Parse("2006-01-02", first.Saturday)
	for n := 0; n < MaxEvents; n++ {
		d := date.AddDate(0, 0, 7*n).Format("2006-01-02")
		ev, e := EventForDate(x.pin, d)
		if e != nil {
			t.Fatal(e)
		}
		r := first
		r.Saturday = d
		r.EventID = ev.EventID
		i, e := x.s.Prepare(ctx, x.session, r)
		if e != nil {
			t.Fatal(n, e)
		}
		if _, e = x.s.Abandon(ctx, x.session, i.OperationID); e != nil {
			t.Fatal(e)
		}
	}
	lastDate := date.AddDate(0, 0, 7*MaxEvents).Format("2006-01-02")
	ev, _ := EventForDate(x.pin, lastDate)
	r := first
	r.Saturday = lastDate
	r.EventID = ev.EventID
	if _, e := x.s.Prepare(ctx, x.session, r); !errors.Is(e, ErrCapacity) {
		t.Fatal("event cap", e)
	}
	i, e := x.s.Prepare(ctx, x.session, first)
	if e != nil {
		t.Fatal("existing event blocked", e)
	}
	if _, e = x.s.Apply(ctx, x.session, i.OperationID); e != nil {
		t.Fatal(e)
	}
	x.reopen(t)
	var after string
	count := 0
	for {
		page, e := x.s.History(ctx, x.session, after, 8)
		if e != nil {
			t.Fatal(e)
		}
		count += len(page.Events)
		if !page.HasMore {
			break
		}
		after = page.NextEventID
	}
	if count != MaxEvents {
		t.Fatal("history lost rows", count)
	}
}

func TestEntryOpenRejectsHistoricalRosterRegression(t *testing.T) {
	x := newEntryFixture(t, 1)
	ctx := context.Background()
	r := x.request(t, "replace", 0)
	first, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.s.Apply(ctx, x.session, first.OperationID); e != nil {
		t.Fatal(e)
	}
	r.ExpectedRevision = 1
	second, e := x.s.Prepare(ctx, x.session, r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.s.Apply(ctx, x.session, second.OperationID); e != nil {
		t.Fatal(e)
	}
	if e = x.s.Close(); e != nil {
		t.Fatal(e)
	}
	x.s = nil
	db, e := leveldb.OpenFile(filepath.Join(x.path, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := db.Get(key("o", second.OperationID), nil)
	if e != nil {
		t.Fatal(e)
	}
	var op Operation
	if e = json.Unmarshal(raw, &op); e != nil {
		t.Fatal(e)
	}
	// Keep the operation/receipt locally consistent but regress the actual
	// historical replace revision. Withdraw's zero sentinel is not this case.
	op.Intent.Request.ExpectedRosterRevision = 0
	op.Receipt.RosterRevision = 0
	if e = validateOperation(op); e != nil {
		t.Fatal("mutation failed to isolate graph invariant", e)
	}
	batch := new(leveldb.Batch)
	if e = put(batch, key("o", second.OperationID), op, MaxOperationSize); e != nil {
		t.Fatal(e)
	}
	if e = db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	db.Close()
	opened, e := Open(ctx, x.path, x.pin, x.stable)
	if opened != nil {
		opened.Close()
	}
	if !errors.Is(e, ErrCorrupt) {
		t.Fatal("historical replace roster revision regressed", e)
	}
}

func TestEntryHistoryCancellationIsNotCorruption(t *testing.T) {
	x := newEntryFixture(t, 0)
	r := x.request(t, "withdraw", 0)
	if _, e := x.s.Prepare(context.Background(), x.session, r); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	x.s.checkpoint = func(stage string) {
		if stage == "history_row" {
			cancel()
		}
	}
	if _, e := x.s.History(ctx, x.session, "", 8); !errors.Is(e, ErrCancelled) {
		t.Fatal("intentional cancellation reported corruption", e)
	}
	x.s.checkpoint = nil
	x.reopen(t)
	p, e := x.s.History(context.Background(), x.session, "", 8)
	if e != nil || len(p.Events) != 1 {
		t.Fatal("cancel damaged history", e)
	}
}
