//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
)

// The fixture verifies actual published normal-work admissions. Its finalized
// chain history is modeled, and its lease is a deterministic test authority;
// this does not substitute for the separate real-clock packaged-helper gate.
func TestCardPreparedReadsKeepOriginalAuthority(t *testing.T) {
	f := newCardFixture(t)
	x := f.x
	built, e := f.build(t)
	if e != nil {
		t.Fatal(e)
	}
	original, e := f.s.db.Get(key('r', built.Record.CardID), nil)
	if e != nil {
		t.Fatal(e)
	}
	reads := []struct {
		name string
		read func(context.Context) error
	}{
		{"status", func(ctx context.Context) error {
			v, e := f.s.Status(ctx, f.seed, x.s, x.entry[0], x.session[0], "2026-11-21")
			if e == nil && (!v.Exists || v.RecordDigest != recordDigest(built.Record)) {
				t.Fatal("status changed retained identity")
			}
			if e != nil && v != (Status{}) {
				t.Fatal("failed status disclosed result")
			}
			return e
		}},
		{"record", func(ctx context.Context) error {
			v, e := f.s.Read(ctx, f.seed, x.s, x.entry[0], x.session[0], "2026-11-21")
			if e == nil && !equal(v.Record, built.Record) {
				t.Fatal("read changed immutable bytes")
			}
			if e != nil && v.Version != 0 {
				t.Fatal("failed read disclosed result")
			}
			return e
		}},
		{"history", func(ctx context.Context) error {
			v, e := f.s.History(ctx, f.seed, x.s, x.entry[0], x.session[0], "", 8)
			if e == nil && (len(v.Cards) != 1 || v.Cards[0].RecordDigest != recordDigest(built.Record)) {
				t.Fatal("history changed retained identity")
			}
			if e != nil && v.Version != 0 {
				t.Fatal("failed history disclosed result")
			}
			return e
		}},
	}
	for _, row := range reads {
		t.Run(row.name, func(t *testing.T) {
			defer func() { x.revoke[0] = false; f.s.checkpoint = nil }()
			validated, observed := 0, 0
			f.s.checkpoint = func(stage string) {
				if stage == "record-validated" {
					validated++
				}
				if stage == "read-observed" {
					observed++
				}
			}
			x.revoke[0] = true
			if e := row.read(t.Context()); !errors.Is(e, ErrUnauthorized) || validated != 0 || observed != 0 {
				t.Fatal("preauthorization must precede retained validation", e, validated, observed)
			}
			x.revoke[0] = false
			f.s.checkpoint = func(stage string) {
				if stage == "record-validated" {
					x.revoke[0] = true
				}
				if stage == "read-observed" {
					observed++
				}
			}
			if e := row.read(t.Context()); !errors.Is(e, ErrUnauthorized) || observed != 0 {
				t.Fatal("revocation during preparation reached disclosure", e, observed)
			}
			x.revoke[0] = false
			ctx, cancel := context.WithCancel(t.Context())
			f.s.checkpoint = func(stage string) {
				if stage == "record-validated" {
					cancel()
				}
				if stage == "read-observed" {
					observed++
				}
			}
			e := row.read(ctx)
			cancel()
			if !errors.Is(e, ErrCancelled) || observed != 0 {
				t.Fatal("cancelled preparation reached disclosure", e, observed)
			}
			// A lost final lease after this read's durable observation remains
			// unknown, rather than being relabelled as a harmless cancellation.
			x.now++
			f.s.checkpoint = func(stage string) {
				if stage == "read-observed" {
					x.revoke[0] = true
				}
			}
			if e = row.read(t.Context()); !errors.Is(e, ErrOutcomeUnknown) || f.s.meta.ClockMS != x.now {
				t.Fatal("post-sync revocation lost uncertainty/clock", e, f.s.meta.ClockMS, x.now)
			}
			x.revoke[0] = false
			x.now++
			ctx, cancel = context.WithCancel(t.Context())
			f.s.checkpoint = func(stage string) {
				if stage == "read-observed" {
					cancel()
				}
			}
			e = row.read(ctx)
			cancel()
			if !errors.Is(e, ErrOutcomeUnknown) || f.s.meta.ClockMS != x.now {
				t.Fatal("post-sync cancellation lost uncertainty/clock", e)
			}
			f.s.checkpoint = nil
			// The fixture refuses every capability except View. Real held-account
			// authorization is covered by the concrete parent integration gate.
			if e = row.read(t.Context()); e != nil {
				t.Fatal("View-only read", e)
			}
			got, e := f.s.db.Get(key('r', built.Record.CardID), nil)
			if e != nil || !bytes.Equal(got, original) {
				t.Fatal("read changed saved card", e)
			}
		})
	}
	validated := 0
	f.s.checkpoint = func(stage string) {
		if stage == "record-validated" {
			validated++
		}
	}
	// Model a source recreated under the same deterministic Pin: its immutable
	// creation time must be checked before any saved row is validated/disclosed.
	originalCreated := f.s.meta.SourceCreatedAtMS
	f.s.meta.SourceCreatedAtMS++
	_, e = f.s.Status(t.Context(), f.seed, x.s, x.entry[0], x.session[0], "2026-11-21")
	f.s.meta.SourceCreatedAtMS = originalCreated
	f.s.checkpoint = nil
	if !errors.Is(e, ErrSourceChanged) || validated != 0 {
		t.Fatal("source incarnation checked too late", e, validated)
	}

	// A complete metadata read now has exactly the two source View traversals
	// (each entry begin/final lease), rather than two separate CardView cycles.
	base := x.session[0].Lease
	leases := 0
	x.session[0].Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		leases++
		return base(ctx, cap, fn)
	}
	for _, row := range reads {
		leases = 0
		if e = row.read(t.Context()); e != nil || leases != 4 {
			t.Fatal("duplicated source observation", row.name, leases, e)
		}
	}
	leases = 0
	absent, e := f.s.Status(t.Context(), f.seed, x.s, x.entry[0], x.session[0], "2026-11-28")
	if e != nil || leases != 4 || absent.Exists || absent.Usable || absent.Source.Status.Exists || absent.Source.Status.ScanHeight != -1 || absent.Source.Status.ScanTipHeight != -1 {
		t.Fatal("absent source observation", e, leases)
	}
	if _, e = ValidateStatus(f.p, absent); e != nil {
		t.Fatal("absent shape", e)
	}
}
