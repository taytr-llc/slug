// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
)

func fixturePin(t testing.TB) Pin {
	t.Helper()
	b, e := os.ReadFile("../gymstable/testdata/reference/stable_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var v struct{ Pins []struct{ Pin gymstable.Pin } }
	if json.Unmarshal(b, &v) != nil || len(v.Pins) != 2 {
		t.Fatal("fixture pins")
	}
	p := Pin{Version: 1, CalendarID: gymentry.CalendarID(), RulesID: RulesID(), Participants: []gymentry.Pin{}}
	for _, x := range v.Pins {
		a := x.Pin
		ns, _ := gymentry.NamespaceID(a.Identity.Network, a.Identity.Genesis, a.AccountAuthorityID)
		id, _ := gymstable.ID(a)
		p.Participants = append(p.Participants, gymentry.Pin{Version: 1, NamespaceID: ns, CalendarID: gymentry.CalendarID(), RulesID: gymentry.RulesID(), StableStoreID: id, StablePin: a})
	}
	sort.Slice(p.Participants, func(i, j int) bool {
		return p.Participants[i].StablePin.Identity.GymID < p.Participants[j].StablePin.Identity.GymID
	})
	if _, e := ValidatePin(p); e != nil {
		t.Fatal(e)
	}
	return p
}
func requireNormal(t *testing.T) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal authority profile")
	}
}
func TestEventInitializeSamplesAfterDurablePinAndPreservesTime(t *testing.T) {
	requireNormal(t)
	p := fixturePin(t)
	ev, e := EventForDate(p, "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	now := ev.CutoffMS - 1
	path := filepath.Join(realTemp(t), "events")
	called := false
	s, e := create(context.Background(), path, p, func() time.Time { return time.UnixMilli(now) }, func(stage string) {
		if stage == "init-synced" {
			called = true
			now = ev.CutoffMS
		}
	})
	if e != nil {
		t.Fatal(e)
	}
	info, e := s.Info(context.Background())
	if e != nil || !called || info.CreatedAtMS != ev.CutoffMS {
		t.Fatal(info, e)
	}
	s.Close()
	now += 1000
	s, e = open(context.Background(), path, p, func() time.Time { return time.UnixMilli(now) }, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	again, e := s.Info(context.Background())
	if e != nil || again.CreatedAtMS != info.CreatedAtMS || again.EventCount != 0 {
		t.Fatal(again, e)
	}
}
func TestEventInitializationResumeAndRefusal(t *testing.T) {
	requireNormal(t)
	p := fixturePin(t)
	now := int64(1795000000000)
	for _, state := range []string{"marker-only", "empty-db", "init-only", "mixed", "unknown", "initialized"} {
		t.Run(state, func(t *testing.T) {
			path := filepath.Join(realTemp(t), "events")
			id, _ := ID(p)
			os.Mkdir(path, 0700)
			b, _ := json.Marshal(marker{"slug-candidate-events", 1, id, p})
			if e := os.WriteFile(filepath.Join(path, markerName), append(b, '\n'), 0600); e != nil {
				t.Fatal(e)
			}
			if state != "marker-only" {
				os.Mkdir(filepath.Join(path, "db"), 0700)
			}
			if state != "marker-only" && state != "empty-db" {
				db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
				if e != nil {
					t.Fatal(e)
				}
				batch := new(leveldb.Batch)
				if state == "init-only" || state == "mixed" {
					put(batch, []byte("init"), initIntent{1, id, p}, MaxMetadataSize)
				}
				if state == "initialized" || state == "mixed" {
					put(batch, []byte("meta"), metadata{1, id, p, now, now, 0}, MaxMetadataSize)
				}
				if state == "unknown" {
					batch.Put([]byte("unexpected"), []byte("retained"))
				}
				if e = db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
					t.Fatal(e)
				}
				db.Close()
			}
			if state != "initialized" {
				if s, e := Open(context.Background(), path, p); e == nil {
					s.Close()
					t.Fatal("ordinary Open adopted partial")
				}
			}
			s, e := open(context.Background(), path, p, func() time.Time { return time.UnixMilli(now + 100) }, true)
			if state == "mixed" || state == "unknown" {
				if e == nil {
					s.Close()
					t.Fatal("partial adopted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			i, e := s.Info(context.Background())
			expected := now + 100
			if state == "initialized" {
				expected = now
			}
			if e != nil || i.CreatedAtMS != expected {
				t.Fatal(i, e)
			}
		})
	}
}
func TestEventInitLostAcknowledgementPreservesCommittedTime(t *testing.T) {
	requireNormal(t)
	p := fixturePin(t)
	path := filepath.Join(realTemp(t), "events")
	now := int64(1795000000000)
	ctx, cancel := context.WithCancel(context.Background())
	_, e := create(ctx, path, p, func() time.Time { return time.UnixMilli(now) }, func(stage string) {
		if stage == "initialized" {
			cancel()
		}
	})
	if !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal(e)
	}
	s, e := open(context.Background(), path, p, func() time.Time { return time.UnixMilli(now + 500) }, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	i, e := s.Info(context.Background())
	if e != nil || i.CreatedAtMS != now {
		t.Fatal(i, e)
	}
}
func TestEventRawAuditRefusesUnknownAndOversizedRows(t *testing.T) {
	requireNormal(t)
	p := fixturePin(t)
	for _, kind := range []string{"unknown", "init", "event-key", "event-size", "capture-key", "capture-size", "receipt-size", "metadata-count"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(realTemp(t), "events")
			s, e := Create(context.Background(), path, p)
			if e != nil {
				t.Fatal(e)
			}
			m := s.meta
			s.Close()
			db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
			if e != nil {
				t.Fatal(e)
			}
			batch := new(leveldb.Batch)
			switch kind {
			case "unknown":
				batch.Put([]byte("no"), []byte("{}"))
			case "init":
				put(batch, []byte("init"), initIntent{1, m.StoreID, p}, MaxMetadataSize)
			case "event-key":
				batch.Put([]byte("e"), []byte("{}"))
			case "event-size":
				batch.Put(makeKey('e', 33), make([]byte, MaxEventSize+1))
			case "capture-key":
				batch.Put(makeKey('c', 33), []byte("{}"))
			case "capture-size":
				batch.Put(makeKey('c', 65), make([]byte, MaxCaptureSize+1))
			case "receipt-size":
				batch.Put(makeKey('s', 33), make([]byte, MaxReceiptSize+1))
			case "metadata-count":
				m.EventCount = 1
				put(batch, []byte("meta"), m, MaxMetadataSize)
			}
			if e = db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
				t.Fatal(e)
			}
			db.Close()
			if s, e = Open(context.Background(), path, p); !errors.Is(e, ErrCorrupt) {
				if s != nil {
					s.Close()
				}
				t.Fatal(e)
			}
		})
	}
}
func makeKey(prefix byte, n int) []byte { k := make([]byte, n); k[0] = prefix; return k }
func TestEventProfileBeforeFilesystem(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile")
	}
	path := filepath.Join(realTemp(t), "must-not-exist")
	for _, fn := range []func(context.Context, string, Pin) (*Store, error){Create, Open, ResumePrepared} {
		if _, e := fn(context.Background(), path, Pin{}); !errors.Is(e, ErrProfile) {
			t.Fatal(e)
		}
	}
	if _, e := os.Lstat(path); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("touched filesystem", e)
	}
}
func TestEventPureCalendarFollowingCentury(t *testing.T) {
	p := fixturePin(t)
	ev, e := EventForDate(p, "2099-12-26")
	if e != nil {
		t.Fatal(e)
	}
	if time.UnixMilli(ev.IncidentDeadlineMS).UTC().Format("2006-01-02T15:04Z") != "2100-01-02T20:00Z" {
		t.Fatal(ev)
	}
	if _, e = EventForDate(p, "2100-01-02"); !errors.Is(e, ErrCalendar) {
		t.Fatal(e)
	}
}

func realTemp(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
