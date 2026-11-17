// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This checks every retained state from the independent semantic scenarios
// against the actual journal audit and read arithmetic. It does not pretend to
// execute the model's injected proof/lease faults; those have separate real
// source, cancellation and process-kill tests.
func TestEventReferenceAllRetainedScenarioStates(t *testing.T) {
	c := readEventCorpus(t)
	captures := map[string]ParticipantCapture{}
	receipts := map[string]SnapshotReceipt{}
	addCapture := func(v ParticipantCapture) {
		if validateParticipantCapture(c.Pin.Value, v) != nil {
			t.Fatal("independent capture")
		}
		b, _ := json.Marshal(v)
		h := sha256.Sum256(b)
		captures[hex.EncodeToString(h[:])] = v
	}
	addReceipt := func(v SnapshotReceipt) {
		if validateSnapshotReceipt(c.Pin.Value, v) != nil {
			t.Fatal("independent receipt")
		}
		d, e := ReceiptDigest(c.Pin.Value, v)
		if e != nil {
			t.Fatal(e)
		}
		receipts[d] = v
		for i, p := range v.Input.Participants {
			addCapture(ParticipantCapture{1, p, v.Captures[i]})
		}
	}
	for _, v := range c.Records {
		addReceipt(v.Value)
	}
	for _, scenario := range c.Scenarios {
		for _, step := range scenario.Steps {
			var shape map[string]json.RawMessage
			if json.Unmarshal(step.Result, &shape) != nil {
				continue
			}
			if _, ok := shape["capture"]; ok {
				var v CaptureResult
				if json.Unmarshal(step.Result, &v) != nil {
					t.Fatal("capture result")
				}
				addCapture(v.Capture)
			}
			if _, ok := shape["input"]; ok {
				var v SnapshotReceipt
				if json.Unmarshal(step.Result, &v) != nil {
					t.Fatal("receipt")
				}
				addReceipt(v)
			}
		}
	}
	steps := 0
	for _, scenario := range c.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			db, e := leveldb.OpenFile(filepath.Join(t.TempDir(), "db"), nil)
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			for _, step := range scenario.Steps {
				steps++
				b := new(leveldb.Batch)
				it := db.NewIterator(nil, nil)
				for it.Next() {
					b.Delete(append([]byte(nil), it.Key()...))
				}
				if e = it.Error(); e != nil {
					t.Fatal(e)
				}
				it.Release()
				m := metadata{1, c.Pin.StoreID, c.Pin.Value, scenario.CreatedAtMS, step.State.ClockMS, step.State.EventCount}
				if e = put(b, []byte("meta"), m, MaxMetadataSize); e != nil {
					t.Fatal(e)
				}
				for _, row := range step.State.Events {
					st := row.Status
					r := eventRow{1, st.Event, st.Phase, st.CreatedAtMS, st.CompletedAtMS, st.SnapshotDigest}
					put(b, key('e', st.Event.EventID), r, MaxEventSize)
					for id, digest := range row.CaptureSHA {
						cap, ok := captures[digest]
						if !ok || cap.Snapshot.ParticipantID != id {
							t.Fatal("missing independently retained capture", digest)
						}
						put(b, key('c', st.Event.EventID, id), cap, MaxCaptureSize)
					}
					if row.ReceiptDigest != "" {
						v, ok := receipts[row.ReceiptDigest]
						if !ok {
							t.Fatal("missing independently retained receipt", row.ReceiptDigest)
						}
						put(b, key('s', st.Event.EventID), v, MaxReceiptSize)
					}
				}
				if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
					t.Fatal(e)
				}
				s := &Store{db: db}
				if e = s.audit(context.Background(), marker{"slug-candidate-events", 1, c.Pin.StoreID, c.Pin.Value}); e != nil {
					t.Fatalf("retained state step %d: %v", steps, e)
				}
				for _, row := range step.State.Events {
					actual, e := s.status(row.Status.Event, step.State.ClockMS)
					if e != nil || !equal(actual, row.Status) {
						t.Fatal("retained status mismatch", e)
					}
				}
			}
		})
	}
	if len(c.Scenarios) != 35 || steps != 91 {
		t.Fatal("state coverage", len(c.Scenarios), steps)
	}
}

func TestEventReferenceInitializationFilesystemStates(t *testing.T) {
	requireNormal(t)
	c := readEventCorpus(t)
	if len(c.Initialization) != 42 {
		t.Fatal("initialization coverage")
	}
	for _, v := range c.Initialization {
		t.Run(v.Name, func(t *testing.T) {
			dir := realTemp(t)
			path := filepath.Join(dir, "events")
			p := c.Pin.Value
			id := c.Pin.StoreID
			if v.State != "absent" {
				if e := os.Mkdir(path, 0700); e != nil {
					t.Fatal(e)
				}
			}
			if v.State == "symlink" {
				target := filepath.Join(dir, "target")
				os.Mkdir(target, 0700)
				os.Remove(path)
				if e := os.Symlink(target, path); e != nil {
					t.Fatal(e)
				}
			}
			if v.State != "absent" && v.State != "empty" && v.State != "symlink" && v.State != "missing_marker" {
				m := marker{"slug-candidate-events", 1, id, p}
				if v.State == "wrong_marker" {
					m.StoreID = strings.Repeat("f", 64)
				}
				b, _ := json.Marshal(m)
				if e := os.WriteFile(filepath.Join(path, markerName), append(b, '\n'), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if v.State != "absent" && v.State != "empty" && v.State != "symlink" && v.State != "marker_no_db" {
				if e := os.Mkdir(filepath.Join(path, "db"), 0700); e != nil {
					t.Fatal(e)
				}
				if v.State != "marker_empty_db" {
					db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
					if e != nil {
						t.Fatal(e)
					}
					b := new(leveldb.Batch)
					switch v.State {
					case "init_only":
						put(b, []byte("init"), initIntent{1, id, p}, MaxMetadataSize)
					case "initialized_empty", "initialized_nonempty", "mixed_init_meta", "missing_marker":
						m := metadata{1, id, p, v.OldCreated, v.OldCreated, 0}
						if v.State == "initialized_nonempty" {
							ev, e := EventForDate(p, "2026-11-21")
							if e != nil {
								t.Fatal(e)
							}
							m.EventCount = 1
							m.ClockMS = ev.CutoffMS + 1
							put(b, key('e', ev.EventID), eventRow{Version: 1, Event: ev, Phase: "awaiting_capture", CreatedAtMS: m.ClockMS}, MaxEventSize)
						}
						put(b, []byte("meta"), m, MaxMetadataSize)
						if v.State == "mixed_init_meta" {
							put(b, []byte("init"), initIntent{1, id, p}, MaxMetadataSize)
						}
					case "unknown_row":
						b.Put([]byte("unexpected"), []byte("{}"))
					case "partial_meta":
						b.Put([]byte("meta"), []byte(`{"version":1}`))
					}
					if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
						t.Fatal(e)
					}
					db.Close()
				}
			}
			clock := func() time.Time { return time.UnixMilli(v.PostPin) }
			var s *Store
			var e error
			switch v.Method {
			case "create":
				s, e = create(context.Background(), path, p, clock, nil)
			case "open":
				s, e = open(context.Background(), path, p, clock, false)
			case "resume_prepared":
				s, e = open(context.Background(), path, p, clock, true)
			default:
				t.Fatal("method")
			}
			if (e == nil) != v.Accepted {
				if s != nil {
					s.Close()
				}
				t.Fatal("initialization acceptance", e, v.Accepted)
			}
			if s != nil {
				info, e := s.Info(context.Background())
				s.Close()
				if e != nil || info.CreatedAtMS != v.ExpectedCreated {
					t.Fatal("creation time", info.CreatedAtMS, v.ExpectedCreated, e)
				}
			}
		})
	}
}
