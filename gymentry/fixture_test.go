// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type entryFixture struct {
	s       *Store
	source  *registry.ManifestStore
	stable  *gymstable.Store
	pin     Pin
	session gymstable.Session
	path    string
	now     int64
	items   []gymstable.Item
}

func newEntryFixture(t *testing.T, count int) *entryFixture {
	return newNamedEntryFixture(t, "zero256", count)
}
func newNamedEntryFixture(t *testing.T, name string, count int) *entryFixture {
	t.Helper()
	if wire.TestPoW {
		t.Skip("trusted published normal-PoW source")
	}
	raw, e := os.ReadFile("../gymstable/testdata/reference/stable_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		Pins []struct {
			Name string
			Pin  gymstable.Pin
		}
		Cases []struct {
			Fixture string
			Record  registry.ManifestRecord
		}
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	var pin gymstable.Pin
	var records []registry.ManifestRecord
	for _, p := range f.Pins {
		if p.Name == name {
			pin = p.Pin
		}
	}
	for _, c := range f.Cases {
		if c.Fixture == name {
			records = append(records, c.Record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Sequence < records[j].Sequence })
	records = records[:count]
	dir := os.Getenv("SLUG_ENTRY_CRASH_ROOT")
	if dir == "" {
		dir = t.TempDir()
	}
	sourcePath := filepath.Join(dir, "source")
	sourceRows(t, sourcePath, pin, records)
	source, e := registry.OpenManifestAuthority(sourcePath, pin.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	stable, e := gymstable.Create(filepath.Join(dir, "stable"), pin, source)
	if e != nil {
		source.Close()
		t.Fatal(e)
	}
	x := &entryFixture{source: source, stable: stable, path: filepath.Join(dir, "entry")}
	ns, _ := NamespaceID(pin.Identity.Network, pin.Identity.Genesis, pin.AccountAuthorityID)
	sid, _ := gymstable.ID(pin)
	x.pin = Pin{Version1, ns, CalendarID(), RulesID(), sid, pin}
	x.session = entrySession(pin, 1, strings.Repeat("a", 64))
	if count > 0 {
		synced, e := stable.Sync(context.Background(), x.session)
		if e != nil {
			t.Fatal(e)
		}
		x.items = synced.Items
		for i, item := range x.items {
			intent, e := stable.PrepareSlot(context.Background(), x.session, gymstable.SlotRequest{ExpectedRevision: uint64(i), Slot: uint32(i + 1), ProspectID: item.Assignment.ProspectID})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = stable.ApplySlot(context.Background(), x.session, intent.OperationID); e != nil {
				t.Fatal(e)
			}
		}
	}
	x.now = time.Now().UnixMilli()
	x.s, e = create(context.Background(), x.path, x.pin, stable, func() time.Time { return time.UnixMilli(max(x.now, time.Now().UnixMilli())) }, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if x.s != nil {
			x.s.Close()
		}
		stable.Close()
		source.Close()
	})
	return x
}
func entrySession(p gymstable.Pin, generation uint64, view string) gymstable.Session {
	return gymstable.Session{Generation: generation, ViewID: view, Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		identity := p.Identity
		return fn(gymaccount.Account{ID: p.AccountID, Generation: generation, Enrollment: &identity}, func() error { return ctx.Err() })
	}}
}
func (x *entryFixture) request(t *testing.T, action string, revision uint64) Request {
	t.Helper()
	date, e := upcomingDate(x.now)
	if e != nil {
		t.Fatal(e)
	}
	ev, e := EventForDate(x.pin, date)
	if e != nil {
		t.Fatal(e)
	}
	r := Request{Version: Version1, EventID: ev.EventID, Saturday: date, Action: action, ExpectedRevision: revision, ProspectIDs: []string{}}
	if action == "replace" {
		r.ExpectedRosterRevision = uint64(len(x.items))
		for _, i := range x.items {
			r.ProspectIDs = append(r.ProspectIDs, i.Assignment.ProspectID)
		}
		sort.Strings(r.ProspectIDs)
	}
	return r
}
func (x *entryFixture) reopen(t *testing.T) {
	t.Helper()
	if e := x.s.Close(); e != nil {
		t.Fatal(e)
	}
	var e error
	x.s, e = Open(context.Background(), x.path, x.pin, x.stable)
	if e != nil {
		t.Fatal(e)
	}
	x.s.now = func() time.Time { return time.UnixMilli(max(x.now, time.Now().UnixMilli())) }
}

func sourceRows(t *testing.T, path string, pin gymstable.Pin, records []registry.ManifestRecord) {
	t.Helper()
	if e := os.MkdirAll(filepath.Join(path, "db"), 0700); e != nil {
		t.Fatal(e)
	}
	descriptor, e := fighterdescriptor.ResolveCandidate(pin.Source.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	marker := struct {
		Application  string                       `json:"application"`
		Version      uint32                       `json:"version"`
		Network      uint32                       `json:"network"`
		Genesis      string                       `json:"genesis"`
		AuthorityID  string                       `json:"authority_id"`
		DescriptorID string                       `json:"descriptor_id"`
		Descriptor   fighterdescriptor.Descriptor `json:"descriptor"`
	}{"slug-prospect-registry", 5, pin.Source.Network, pin.Source.Genesis, pin.Source.AuthorityID, pin.Source.DescriptorID, descriptor}
	raw, _ := json.Marshal(marker)
	if e = os.WriteFile(filepath.Join(path, "SLUG-REGISTRY.json"), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	b := new(leveldb.Batch)
	jobs := map[string]bool{}
	var clock int64
	for _, r := range records {
		raw, _ = json.Marshal(r)
		b.Put([]byte("record:"+r.Verified.ProspectID), raw)
		k := append([]byte("sequence:"), make([]byte, 8)...)
		binary.BigEndian.PutUint64(k[len("sequence:"):], r.Sequence)
		b.Put(k, []byte(r.Verified.ProspectID))
		raw, _ = json.Marshal(r.Job)
		b.Put([]byte("job:"+r.Job.ID), raw)
		jobs[r.Job.ID] = true
		clock = max(clock, r.Birth.AcceptedAtMS)
	}
	// Structural fixture assembly uses the immutable independent records as input.
	// No new proof/receipt expectation is produced by the implementation under test.
	meta := struct {
		Version      uint32                       `json:"version"`
		Network      uint32                       `json:"network"`
		Genesis      string                       `json:"genesis"`
		AuthorityID  string                       `json:"authority_id"`
		DescriptorID string                       `json:"descriptor_id"`
		Descriptor   fighterdescriptor.Descriptor `json:"descriptor"`
		LastSequence uint64                       `json:"last_sequence"`
		JobCount     uint64                       `json:"job_count"`
		LastClockMS  int64                        `json:"last_clock_ms"`
	}{5, pin.Source.Network, pin.Source.Genesis, pin.Source.AuthorityID, pin.Source.DescriptorID, descriptor, uint64(len(records)), uint64(len(jobs)), clock}
	raw, _ = json.Marshal(meta)
	b.Put([]byte("meta"), raw)
	if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
}
