//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/wire/v2"
)

type dailyPoolCase struct {
	Name         string          `json:"name"`
	Valid        bool            `json:"valid"`
	Value        json.RawMessage `json:"value"`
	CanonicalHex string          `json:"canonical_hex"`
	ImmutableHex string          `json:"immutable_hex"`
	Reserve      uint64          `json:"reserve_bytes"`
}
type dailyPoolJournal struct {
	Marker poolOutboxMarker `json:"marker"`
	Rows   []struct {
		Key   string `json:"key_hex"`
		Value string `json:"value_hex"`
	} `json:"rows"`
}
type dailyPoolReference struct {
	Outbox struct {
		Configs     []dailyPoolCase `json:"config_cases"`
		Markers     []dailyPoolCase `json:"marker_cases"`
		Scans       []dailyPoolCase `json:"scan_cases"`
		Entries     []dailyPoolCase `json:"entry_cases"`
		Journals    []dailyPoolCase `json:"journal_cases"`
		Transitions []struct {
			Name   string           `json:"name"`
			Stored dailyPoolJournal `json:"stored"`
		} `json:"issue_transitions"`
	} `json:"outbox"`
	Transport struct {
		Identities []struct {
			Name   string     `json:"name"`
			Worker PoolConfig `json:"worker"`
		} `json:"wire_identities"`
	} `json:"transport"`
}

func dailyPoolOracle(t *testing.T) dailyPoolReference {
	t.Helper()
	raw, e := os.ReadFile("../testdata/public-daily/reference-v1.json")
	if e != nil || len(raw) != 3176583 {
		t.Fatal("daily reference input", e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3" {
		t.Fatal("daily reference changed")
	}
	var r dailyPoolReference
	if e = json.Unmarshal(raw, &r); e != nil {
		t.Fatal(e)
	}
	if len(r.Outbox.Configs) != 7 || len(r.Outbox.Markers) != 3 || len(r.Outbox.Scans) != 18 || len(r.Outbox.Entries) != 8 || len(r.Outbox.Journals) != 20 || len(r.Outbox.Transitions) != 26 || len(r.Transport.Identities) != 3 {
		t.Fatal("daily reference scope")
	}
	return r
}
func dailyPoolHex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func dailyPoolConfig(t *testing.T, r dailyPoolReference) PoolConfig {
	t.Helper()
	var c PoolConfig
	if e := json.Unmarshal(r.Outbox.Configs[0].Value, &c); e != nil {
		t.Fatal(e)
	}
	return c
}
func dailyPoolWork(t *testing.T, r dailyPoolReference) poolshare.MiningWork {
	t.Helper()
	for _, v := range r.Outbox.Scans {
		if v.Name == "modeled-all-three-miss-cursor-0" {
			var s PoolScan
			if e := json.Unmarshal(v.Value, &s); e != nil {
				t.Fatal(e)
			}
			return s.Work
		}
	}
	t.Fatal("missing original work")
	return poolshare.MiningWork{}
}
func TestDailyPoolIndependentBytesAndModes(t *testing.T) {
	r := dailyPoolOracle(t)
	c := dailyPoolConfig(t, r)
	if _, e := ValidateDailyPoolConfig(c); e != nil {
		t.Fatal(e)
	}
	if _, e := ValidatePoolConfig(c); e == nil {
		t.Fatal("legacy config admitted daily")
	}
	for _, v := range r.Outbox.Configs {
		t.Run("config-"+v.Name, func(t *testing.T) {
			var got PoolConfig
			if json.Unmarshal(v.Value, &got) != nil {
				t.Fatal("decode")
			}
			_, e := ValidateDailyPoolConfig(got)
			// These cases pin the original full reopen selection, not just syntax.
			if (e == nil && poolReopenConfig(got, c)) != v.Valid {
				t.Fatal("complete reopen binding", e)
			}
		})
	}
	for _, v := range r.Outbox.Scans {
		t.Run("scan-"+v.Name, func(t *testing.T) {
			var s PoolScan
			e := poolDecodeCanonical(dailyPoolHex(t, v.CanonicalHex), poolMaxScanSize, &s)
			var selected PoolConfig
			for _, id := range r.Transport.Identities {
				if strings.HasPrefix(v.Name, id.Name+"-") {
					selected = id.Worker
				}
			}
			if e == nil {
				e = validatePoolScan(s, selected)
			}
			if (e == nil) != v.Valid {
				t.Fatal("independent scan", e)
			}
		})
	}
	for _, v := range r.Outbox.Entries {
		t.Run("entry-"+v.Name, func(t *testing.T) {
			var e PoolEntry
			err := poolDecodeCanonical(dailyPoolHex(t, v.CanonicalHex), poolMaxEntrySize, &e)
			if err == nil {
				err = validatePoolEntry(e, c)
			}
			if (err == nil) != v.Valid {
				t.Fatal("independent entry", err)
			}
			if v.Valid {
				raw, _ := json.Marshal(poolImmutable(e))
				if !bytes.Equal(raw, dailyPoolHex(t, v.ImmutableHex)) || poolFutureReserve(e) != v.Reserve {
					t.Fatal("independent immutable/reserve")
				}
			}
		})
	}
}

func dailyPoolWriteJournal(t *testing.T, j dailyPoolJournal) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "box")
	if e := os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(j.Marker)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(path, poolMarkerName), raw, 0600); e != nil {
		t.Fatal(e)
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
	if e != nil {
		t.Fatal(e)
	}
	batch := new(leveldb.Batch)
	for _, v := range j.Rows {
		batch.Put(dailyPoolHex(t, v.Key), dailyPoolHex(t, v.Value))
	}
	we := db.Write(batch, &opt.WriteOptions{Sync: true})
	ce := db.Close()
	if e = errors.Join(we, ce); e != nil {
		t.Fatal(e)
	}
	return path
}
func TestDailyPoolIndependentJournalAuditAndReopen(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal profile journal")
	}
	r := dailyPoolOracle(t)
	c := dailyPoolConfig(t, r)
	for _, v := range r.Outbox.Journals {
		t.Run(v.Name, func(t *testing.T) {
			var j dailyPoolJournal
			if e := json.Unmarshal(v.Value, &j); e != nil {
				t.Fatal(e)
			}
			path := dailyPoolWriteJournal(t, j)
			b, e := OpenDailyPoolOutboxAudit(path, c)
			if b != nil {
				if ce := b.Close(); ce != nil {
					t.Fatal(ce)
				}
			}
			if (e == nil) != v.Valid {
				t.Fatal("independent full journal audit", e)
			}
			if old, e := OpenPoolOutbox(path, c); e == nil || old != nil {
				if old != nil {
					_ = old.Close()
				}
				t.Fatal("legacy adopted daily")
			}
		})
	}
	// Each conditional model's persisted after-state is checked by the same
	// actual cold audit. This is not a claim that a crash or network occurred.
	for _, v := range r.Outbox.Transitions {
		t.Run("stored-"+v.Name, func(t *testing.T) {
			path := dailyPoolWriteJournal(t, v.Stored)
			b, e := OpenDailyPoolOutboxAudit(path, c)
			if e != nil {
				t.Fatal("conditional persisted transition", e)
			}
			if e = b.Close(); e != nil {
				t.Fatal(e)
			}
		})
	}
	for _, v := range r.Outbox.Markers {
		t.Run("marker-"+v.Name, func(t *testing.T) {
			var j dailyPoolJournal
			if e := json.Unmarshal(r.Outbox.Journals[0].Value, &j); e != nil {
				t.Fatal(e)
			}
			path := dailyPoolWriteJournal(t, j)
			if e := os.WriteFile(filepath.Join(path, poolMarkerName), dailyPoolHex(t, v.CanonicalHex), 0600); e != nil {
				t.Fatal(e)
			}
			if e := readPoolMarker(path, c); (e == nil) != v.Valid {
				t.Fatal("marker", e)
			}
		})
	}
}

type dailyPoolAuthority struct {
	pwAuthority
	issue func(context.Context) (poolshare.MiningWork, error)
}

func (a *dailyPoolAuthority) ProtocolVersion() uint32 { return 2 }
func (a *dailyPoolAuthority) Issue(ctx context.Context) (poolshare.MiningWork, error) {
	return a.issue(ctx)
}
func dailyPoolWorker(t *testing.T, b *PoolOutbox, c PoolConfig, w poolshare.MiningWork) (*PoolWorker, *dailyPoolAuthority) {
	t.Helper()
	a := &dailyPoolAuthority{pwAuthority: pwAuthority{c: c, w: w}}
	a.issue = func(context.Context) (poolshare.MiningWork, error) { return w, nil }
	worker, e := NewDailyPool(c, a, &pwAdmission{c: c}, &pwNode{}, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		return gymauth.Authorization{}, errors.New("no fixture signing")
	}, b)
	if e != nil {
		t.Fatal(e)
	}
	return worker, a
}
func dailyPoolNewBox(t *testing.T, c PoolConfig) (*PoolOutbox, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "daily")
	b, e := CreateDailyPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		b.mu.Lock()
		b.running = false
		b.mu.Unlock()
		if e := b.Close(); e != nil {
			t.Error(e)
		}
	})
	return b, path
}
func TestDailyPoolIssueOriginalAttemptAndUncertainty(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal profile Issue")
	}
	r := dailyPoolOracle(t)
	c := dailyPoolConfig(t, r)
	work := dailyPoolWork(t, r)
	for _, tc := range []struct {
		name  string
		err   error
		clear bool
	}{
		{"busy", &poolservice.ClientError{Code: "busy"}, true},
		{"rate", &poolservice.ClientError{Code: "rate_limited"}, true},
		{"unknown-busy", &poolservice.ClientError{Code: "busy", OutcomeUnknown: true}, false},
		{"unknown", &poolservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}, false},
		{"capacity", &poolservice.ClientError{Code: "capacity"}, false},
		{"wrapped", fmt.Errorf("wrapped: %w", &poolservice.ClientError{Code: "busy"}), false},
		{"joined", errors.Join(&poolservice.ClientError{Code: "busy"}, errors.New("uncertain")), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, path := dailyPoolNewBox(t, c)
			w, a := dailyPoolWorker(t, b, c, work)
			b.running = true
			calls := 0
			a.issue = func(context.Context) (poolshare.MiningWork, error) {
				calls++
				if !b.mu.TryLock() {
					t.Error("mutex retained across network")
					return poolshare.MiningWork{}, errors.New("locked")
				}
				held := b.meta.IssuePending
				b.mu.Unlock()
				if !held {
					t.Error("dispatch before durable hold")
				}
				raw, e := b.db.Get([]byte("meta"), nil)
				var m poolOutboxMetadata
				if e != nil || decodePoolMetadata(raw, 2, &m) != nil || !m.IssuePending {
					t.Error("premark not durable")
				}
				return poolshare.MiningWork{}, tc.err
			}
			_, e := w.issueDaily(context.Background())
			if e == nil || calls != 1 || b.meta.IssuePending == tc.clear || b.issueAttempt != nil {
				t.Fatal("attempt result", e, calls, b.meta)
			}
			b.running = false
			if e = b.Close(); e != nil {
				t.Fatal(e)
			}
			reopened, e := OpenDailyPoolOutbox(path, c)
			if e != nil {
				t.Fatal(e)
			}
			defer func() {
				if e := reopened.Close(); e != nil {
					t.Error(e)
				}
			}()
			if reopened.meta.IssuePending == tc.clear {
				t.Fatal("hold changed across original reopen")
			}
			if !tc.clear {
				rw, ra := dailyPoolWorker(t, reopened, c, work)
				ra.issue = func(context.Context) (poolshare.MiningWork, error) {
					t.Error("replacement Issue after reopen")
					return work, nil
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				rw.wait = func(context.Context, time.Duration) error {
					if s := rw.Status(); s.State != "paused" || s.LastError != "issue_unknown" {
						t.Error("not held", s)
					}
					cancel()
					return ctx.Err()
				}
				if e = rw.Run(ctx); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
	for _, kind := range []string{"valid-expired", "cancelled", "foreign-attempt", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			b, _ := dailyPoolNewBox(t, c)
			w, a := dailyPoolWorker(t, b, c, work)
			b.running = true
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a.issue = func(context.Context) (poolshare.MiningWork, error) {
				switch kind {
				case "cancelled":
					cancel()
					return poolshare.MiningWork{}, &poolservice.ClientError{Code: "busy"}
				case "foreign-attempt":
					b.mu.Lock()
					b.issueAttempt = &poolDailyIssueAttempt{w, ctx}
					b.mu.Unlock()
				case "malformed":
					return poolshare.MiningWork{}, nil
				}
				return work, nil
			}
			s, e := w.issueDaily(ctx)
			if kind == "valid-expired" {
				if e != nil || s == nil || b.meta.IssuePending || b.cachedScan == nil || !reflect.DeepEqual(s.Work, work) {
					t.Fatal("known expired response was not saved", e)
				}
				raw, _ := b.db.Get([]byte("scan"), nil)
				var saved PoolScan
				if poolDecodeCanonical(raw, poolMaxScanSize, &saved) != nil || !reflect.DeepEqual(saved.Work, work) {
					t.Fatal("exact known work not synchronized")
				}
			} else if e == nil || !b.meta.IssuePending || b.cachedScan != nil {
				t.Fatal("invalid completion cleared hold", e)
			}
		})
	}
}

func TestDailyPoolIssueSyncFailuresKeepOriginalEvidence(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal profile sync")
	}
	r := dailyPoolOracle(t)
	c := dailyPoolConfig(t, r)
	work := dailyPoolWork(t, r)
	for _, phase := range []string{"premark", "scan", "clear"} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-after-%t", phase, after), func(t *testing.T) {
				b, path := dailyPoolNewBox(t, c)
				w, a := dailyPoolWorker(t, b, c, work)
				b.running = true
				writes, calls := 0, 0
				original := b.writeBatch
				b.writeBatch = func(batch *leveldb.Batch, o *opt.WriteOptions) error {
					writes++
					if !o.Sync {
						t.Error("non-sync Issue commit")
					}
					target := 1
					if phase != "premark" {
						target = 2
					}
					if writes == target {
						if after {
							if e := original(batch, o); e != nil {
								return e
							}
						}
						return errors.New("controlled sync failure")
					}
					return original(batch, o)
				}
				a.issue = func(context.Context) (poolshare.MiningWork, error) {
					calls++
					if phase == "clear" {
						return poolshare.MiningWork{}, &poolservice.ClientError{Code: "busy"}
					}
					return work, nil
				}
				if _, e := w.issueDaily(context.Background()); !errors.Is(e, ErrPoisoned) || !b.poisoned {
					t.Fatal("write outcome not sticky", e)
				}
				wantCalls := 1
				if phase == "premark" {
					wantCalls = 0
				}
				if calls != wantCalls {
					t.Fatal("network crossed uncertain premark", calls)
				}
				if _, e := w.issueDaily(context.Background()); !errors.Is(e, ErrPoisoned) || calls != wantCalls {
					t.Fatal("poisoned owner reused", e)
				}
				b.running = false
				if e := b.Close(); e != nil {
					t.Fatal(e)
				}
				reopened, e := OpenDailyPoolOutbox(path, c)
				if e != nil {
					t.Fatal(e)
				}
				defer func() {
					if e := reopened.Close(); e != nil {
						t.Error(e)
					}
				}()
				expectedHold := !(phase == "premark" && !after || phase != "premark" && after)
				if reopened.meta.IssuePending != expectedHold || (reopened.cachedScan != nil) != (phase == "scan" && after) {
					t.Fatal("exact stored outcome not retained", reopened.meta)
				}
			})
		}
	}
}

func TestDailyPoolHeldReopenStillReconcilesOriginalPending(t *testing.T) {
	r := dailyPoolOracle(t)
	c, work := dailyPoolConfig(t, r), dailyPoolWork(t, r)
	var journal dailyPoolJournal
	for _, v := range r.Outbox.Journals {
		if v.Name == "held-pending" {
			if e := json.Unmarshal(v.Value, &journal); e != nil {
				t.Fatal(e)
			}
		}
	}
	var accepted PoolEntry
	for _, v := range r.Outbox.Entries {
		if v.Name == "pool-only-accepted" {
			if e := json.Unmarshal(v.Value, &accepted); e != nil {
				t.Fatal(e)
			}
		}
	}
	if accepted.PoolReceipt == nil {
		t.Fatal("missing independent receipt")
	}
	path := dailyPoolWriteJournal(t, journal)
	b, e := OpenDailyPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := b.Close(); e != nil {
			t.Error(e)
		}
	}()
	w, a := dailyPoolWorker(t, b, c, work)
	a.issue = func(context.Context) (poolshare.MiningWork, error) {
		t.Error("held recovery issued replacement work")
		return work, nil
	}
	a.lookup = func() (poolshare.Submission, error) { return *accepted.PoolReceipt, nil }
	a.submit = func() (poolshare.Submission, error) {
		t.Error("known accepted receipt resubmitted")
		return poolshare.Submission{}, errors.New("unexpected submit")
	}
	w.now = func() time.Time { return time.UnixMilli(work.Job.IssuedAtMS + 10000) }
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	w.wait = func(context.Context, time.Duration) error {
		if s := w.Status(); s.State != "paused" || s.LastError != "issue_unknown" || s.Pending != 0 {
			t.Error("held recovery status", s)
		}
		cancel()
		return ctx.Err()
	}
	if e = w.Run(ctx); e != nil {
		t.Fatal(e)
	}
	rows, e := b.List(0, 1)
	if e != nil || len(rows) != 1 || rows[0].PoolReceipt == nil || *rows[0].PoolReceipt != *accepted.PoolReceipt || rows[0].PoolState != "accepted" || !b.meta.IssuePending || w.Status().Attempts != 0 || a.lookups != 1 || a.submits != 0 {
		t.Fatal("held pending identity/recovery changed", e)
	}
}

// Both lifetimes enter through the actual exclusive Run owner. The first
// unknown Issue has no recoverable request identity; the separate retained
// pending submission does, and must still reconcile after original reopen.
func TestDailyPoolRunUnknownIssuePersistsAcrossOriginalReopen(t *testing.T) {
	r := dailyPoolOracle(t)
	c, work := dailyPoolConfig(t, r), dailyPoolWork(t, r)
	var journal dailyPoolJournal
	var accepted PoolEntry
	for _, v := range r.Outbox.Journals {
		if v.Name == "pending-entry" {
			if e := json.Unmarshal(v.Value, &journal); e != nil {
				t.Fatal(e)
			}
		}
	}
	for _, v := range r.Outbox.Entries {
		if v.Name == "pool-only-accepted" {
			if e := json.Unmarshal(v.Value, &accepted); e != nil {
				t.Fatal(e)
			}
		}
	}
	if accepted.PoolReceipt == nil {
		t.Fatal("missing accepted reference")
	}
	path := dailyPoolWriteJournal(t, journal)
	b, e := OpenDailyPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := b.Close(); e != nil {
			t.Error(e)
		}
	}()
	issues, submits := 0, 0
	w, a := dailyPoolWorker(t, b, c, work)
	a.issue = func(context.Context) (poolshare.MiningWork, error) {
		issues++
		return poolshare.MiningWork{}, &poolservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	a.lookup = func() (poolshare.Submission, error) {
		return poolshare.Submission{}, &poolservice.ClientError{Code: "not_found"}
	}
	a.submit = func() (poolshare.Submission, error) {
		submits++
		return poolshare.Submission{}, &poolservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
	}
	w.now = func() time.Time { return time.UnixMilli(work.Job.IssuedAtMS + 100) }
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	w.wait = func(context.Context, time.Duration) error {
		s := w.Status()
		if s.State != "paused" || s.LastError != "issue_unknown" || s.Pending != 1 {
			t.Error("first Run did not retain uncertainty", s)
		}
		cancel()
		return ctx.Err()
	}
	if e = w.Run(ctx); e != nil {
		t.Fatal(e)
	}
	if issues != 1 || submits != 1 || w.Status().Attempts != 0 || !b.meta.IssuePending || b.cachedScan != nil || b.running {
		t.Fatal("first Run outcome changed")
	}
	first, e := b.List(0, 1)
	if e != nil || len(first) != 1 || first[0].PoolState != "pending" {
		t.Fatal("original pending row", e)
	}
	if e = b.Close(); e != nil {
		t.Fatal(e)
	}
	b, e = OpenDailyPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	w, a = dailyPoolWorker(t, b, c, work)
	a.issue = func(context.Context) (poolshare.MiningWork, error) { issues++; return work, nil }
	a.lookup = func() (poolshare.Submission, error) { return *accepted.PoolReceipt, nil }
	a.submit = func() (poolshare.Submission, error) { submits++; return *accepted.PoolReceipt, nil }
	w.now = func() time.Time { return time.UnixMilli(work.Job.IssuedAtMS + 10000) }
	ctx2, cancel2 := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel2()
	w.wait = func(context.Context, time.Duration) error {
		s := w.Status()
		if s.State != "paused" || s.LastError != "issue_unknown" || s.Pending != 0 {
			t.Error("reopened Run did not preserve hold/recovery", s)
		}
		cancel2()
		return ctx2.Err()
	}
	if e = w.Run(ctx2); e != nil {
		t.Fatal(e)
	}
	last, e := b.List(0, 1)
	if e != nil || len(last) != 1 || last[0].PoolState != "accepted" || last[0].PoolReceipt == nil || *last[0].PoolReceipt != *accepted.PoolReceipt || !reflect.DeepEqual(poolImmutable(first[0]), poolImmutable(last[0])) || issues != 1 || submits != 1 || a.lookups != 1 || w.Status().Attempts != 0 || !b.meta.IssuePending || b.cachedScan != nil || b.running {
		t.Fatal("Run restart replaced Issue, hashed, or changed pending identity", e)
	}
}

func TestDailyPoolConstructorFailureRetainsOriginalCleanupOwner(t *testing.T) {
	r := dailyPoolOracle(t)
	construction := errors.New("controlled constructor audit refusal")
	cleanup := errors.New("controlled original close acknowledgement failure")
	b, e := CreateDailyPoolOutbox(filepath.Join(t.TempDir(), "daily"), dailyPoolConfig(t, r))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := b.Close(); e != nil && !errors.Is(e, cleanup) {
			t.Error(e)
		}
	})
	calls := 0
	b.closeDB = func() error {
		calls++
		// The actual fixture DB is joined; its synthetic acknowledgement fails.
		return errors.Join(b.db.Close(), cleanup)
	}
	retained, e := poolOutboxConstructorFailure(b, construction, true)
	if retained != b || !errors.Is(e, construction) || !errors.Is(e, cleanup) || !b.closed || calls != 1 {
		t.Fatal("constructor dropped the failed original owner", e)
	}
	if e = retained.Close(); !errors.Is(e, cleanup) || calls != 1 {
		t.Fatal("failed cleanup was retried or declared complete", e)
	}
}
