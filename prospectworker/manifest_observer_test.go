//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"reflect"
	"testing"
	"time"
)

func TestManifestObserverDurableOwnedAndLockFree(t *testing.T) {
	f, c := mwFixture(t)
	b, _ := mwBox(t, c)
	entry := mwFound(t, b, f)
	var w *ManifestWorker
	calls := 0
	a := &mwAuthority{c: c, f: f, submit: func() (prospectservice.ManifestAdmissionResult, error) { return mwResult(f), nil }}
	observer := func(ctx context.Context, r registry.ManifestRecord) error {
		calls++
		rows, e := b.List(0, 1)
		if e != nil || len(rows) != 1 || rows[0].AdmissionState != "accepted" || !reflect.DeepEqual(rows[0].Receipt.Record, f.Record) {
			t.Fatal("notification preceded durable accepted receipt", e)
		}
		_ = w.Status() // Neither worker nor outbox mutex may be held.
		r.Proof.MerkleBranch[0] = "mutated"
		return nil
	}
	var e error
	w, e = NewManifestWithObserver(c, a, &mwNode{f: f}, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		return f.Auth, nil
	}, b, observer)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.deliver(context.Background(), &entry); e != nil {
		t.Fatal(e)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	if _, e = w.deliver(context.Background(), &entry); e != nil || calls != 1 {
		t.Fatal("terminal replay notified", e, calls)
	}
	rows, e := b.List(0, 1)
	if e != nil || !reflect.DeepEqual(rows[0].Receipt.Record, f.Record) {
		t.Fatal("callback mutated retained evidence", e)
	}
}
func TestManifestObserverFailureAndCancellation(t *testing.T) {
	for _, mode := range []string{"error", "panic", "cancel-before", "cancel-during", "uncertain", "uncertain-committed", "bad-receipt"} {
		t.Run(mode, func(t *testing.T) {
			f, c := mwFixture(t)
			b, _ := mwBox(t, c)
			entry := mwFound(t, b, f)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			entered, release := make(chan struct{}), make(chan struct{})
			a := &mwAuthority{c: c, f: f, submit: func() (prospectservice.ManifestAdmissionResult, error) {
				r := mwResult(f)
				if mode == "cancel-before" {
					cancel()
				}
				if mode == "uncertain" {
					b.writeBatch = func(*leveldb.Batch, *opt.WriteOptions) error { return errors.New("private write detail") }
				}
				if mode == "uncertain-committed" {
					original := b.writeBatch
					b.writeBatch = func(batch *leveldb.Batch, opts *opt.WriteOptions) error {
						if e := original(batch, opts); e != nil {
							return e
						}
						return errors.New("private post-commit acknowledgement")
					}
				}

				if mode == "bad-receipt" {
					r.Record.StaticBinding.ManifestDigest = "wrong"
				}
				return r, nil
			}}
			w, e := NewManifestWithObserver(c, a, &mwNode{f: f}, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
				return f.Auth, nil
			}, b, func(ctx context.Context, r registry.ManifestRecord) error {
				calls++
				switch mode {
				case "error":
					return errors.New("private callback detail")
				case "panic":
					panic("private panic detail")
				case "cancel-during":
					close(entered)
					<-release
					if ctx.Err() == nil {
						t.Error("callback context not canceled")
					}
				}
				return nil
			})
			if e != nil {
				t.Fatal(e)
			}
			result := make(chan error, 1)
			go func() { _, e := w.deliver(ctx, &entry); result <- e }()
			if mode == "cancel-during" {
				<-entered
				cancel()
				select {
				case <-result:
					t.Fatal("callback was not drained")
				case <-time.After(20 * time.Millisecond):
				}
				close(release)
			}
			e = <-result
			switch mode {
			case "error", "panic":
				if !errors.Is(e, ErrManifestObserver) || e.Error() != ErrManifestObserver.Error() || w.Status().LastError != "observer_unavailable" {
					t.Fatal(e, w.Status())
				}
			case "uncertain", "uncertain-committed", "bad-receipt":
				if e == nil || calls != 0 {
					t.Fatal("invalid transition notified", e, calls)
				}
			default:
				if e != nil {
					t.Fatal(e)
				}
			}
			if mode == "cancel-before" && calls != 0 {
				t.Fatal("cancelled notification")
			}
			if mode != "uncertain" && mode != "uncertain-committed" && mode != "bad-receipt" {
				rows, e := b.List(0, 1)
				if e != nil || rows[0].AdmissionState != "accepted" || !reflect.DeepEqual(rows[0].Receipt.Record, f.Record) {
					t.Fatal("observer changed acceptance", e)
				}
			}
		})
	}
}

func TestManifestObserverRunStopsWithoutResubmission(t *testing.T) {
	f, c := mwFixture(t)
	b, _ := mwBox(t, c)
	mwFound(t, b, f)
	submits, observed := 0, 0
	a := &mwAuthority{c: c, f: f, submit: func() (prospectservice.ManifestAdmissionResult, error) { submits++; return mwResult(f), nil }}
	w, e := NewManifestWithObserver(c, a, &mwNode{f: f}, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		return f.Auth, nil
	}, b, func(context.Context, registry.ManifestRecord) error {
		observed++
		return errors.New("private callback failure")
	})
	if e != nil {
		t.Fatal(e)
	}
	w.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS + 100) }
	if e = w.Run(context.Background()); !errors.Is(e, ErrManifestObserver) || observed != 1 || submits != 1 || w.Status().LastError != "observer_unavailable" {
		t.Fatal(e, observed, submits, w.Status())
	}
	rows, e := b.List(0, 1)
	if e != nil || rows[0].AdmissionState != "accepted" {
		t.Fatal("failed observer discarded durable receipt", e)
	}
	// A restarted worker sees the terminal receipt but never emits a startup event.
	ctx, cancel := context.WithCancel(context.Background())
	a.issue = cancel
	restarted, e := NewManifestWithObserver(c, a, &mwNode{f: f}, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		t.Fatal("unexpected signature")
		return f.Auth, nil
	}, b, func(context.Context, registry.ManifestRecord) error { t.Fatal("terminal startup replay"); return nil })
	if e != nil {
		t.Fatal(e)
	}
	restarted.now = w.now
	if e = restarted.Run(ctx); e != nil || submits != 1 {
		t.Fatal(e, submits)
	}
}

// This is a typed authority-response fault fixture, not packaged-helper fault
// injection. Both responses refer to the unchanged independently frozen receipt.
func TestManifestObserverHistoricalAcknowledgmentAfterUnknown(t *testing.T) {
	f, c := mwFixture(t)
	box, path := mwBox(t, c)
	entry := mwFound(t, box, f)
	submissions, signatures, broadcasts, notifications := 0, 0, 0, 0
	authority := &mwAuthority{c: c, f: f, submit: func() (prospectservice.ManifestAdmissionResult, error) {
		submissions++
		if submissions == 1 {
			return prospectservice.ManifestAdmissionResult{}, &prospectservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
		}
		result := mwResult(f)
		result.Inserted = false
		result.CurrentNodeChecked = false
		result.ContextSource = "historical-receipt"
		return result, nil
	}}
	node := &mwNode{f: f, call: func(method string) {
		if method == "broadcast" {
			broadcasts++
		}
	}}
	signer := func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		signatures++
		return f.Auth, nil
	}
	observer := func(ctx context.Context, r registry.ManifestRecord) error {
		notifications++
		stored, e := box.List(0, 1)
		if e != nil || len(stored) != 1 || stored[0].AdmissionState != "accepted" || stored[0].Receipt == nil || stored[0].Receipt.Inserted || stored[0].Receipt.CurrentNodeChecked || stored[0].Receipt.ContextSource != "historical-receipt" {
			t.Fatal("historical notification preceded exact durable save", e)
		}
		if !reflect.DeepEqual(r, f.Record) || r.Birth != f.Record.Birth || !reflect.DeepEqual(stored[0].Receipt.Record, r) {
			t.Fatal("historical notification changed original receipt or birth")
		}
		return nil
	}
	worker, e := NewManifestWithObserver(c, authority, node, signer, box, observer)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = worker.deliver(context.Background(), &entry); e != nil {
		t.Fatal(e)
	}
	if notifications != 0 || entry.AdmissionState != "pending" || entry.Receipt != nil || entry.LastError != "outcome_unknown" {
		t.Fatal("uncertain acknowledgment notified or discarded pending evidence", notifications, entry.AdmissionState, entry.LastError)
	}
	if e = box.Close(); e != nil {
		t.Fatal(e)
	}
	box, e = OpenManifestOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	pending, e := box.List(0, 1)
	if e != nil || len(pending) != 1 || pending[0].Receipt != nil || pending[0].Authorization == nil || !reflect.DeepEqual(*pending[0].Authorization, f.Auth) {
		t.Fatal("unknown outcome did not retain identical authorization", e)
	}
	entry = pending[0]
	worker, e = NewManifestWithObserver(c, authority, node, signer, box, observer)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = worker.deliver(context.Background(), &entry); e != nil {
		t.Fatal(e)
	}
	if notifications != 1 || submissions != 2 || signatures != 1 || broadcasts != 1 {
		t.Fatal("historical acknowledgment repeated work or omitted notification", notifications, submissions, signatures, broadcasts)
	}
	if _, e = worker.deliver(context.Background(), &entry); e != nil || notifications != 1 || submissions != 2 {
		t.Fatal("terminal historical receipt notified twice", e, notifications, submissions)
	}
}
