//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"testing"
)

func TestResultProofPublicationErrorClassification(t *testing.T) {
	for _, mode := range []string{"definite_prelink", "opaque_unknown", "postlink_revocation", "postlink_close"} {
		t.Run(mode, func(t *testing.T) {
			f, c, done, r := preparedFixture(t)
			defer done()
			_, st, e := f.s.read(c, gymaccount.View, true)
			if e != nil {
				t.Fatal(e)
			}
			foreign := errors.New("test foreign error")
			if mode == "opaque_unknown" {
				foreign = errors.New("outcome_unknown")
			}
			summary := ProofSummary{RecordDigest: recordDigest(r)}
			e = f.s.publishProof(c, r.Statement.Statement.Source, st.Source, summary, func(check func() error) error {
				if e := check(); e != nil {
					return e
				}
				if mode == "definite_prelink" || mode == "opaque_unknown" {
					return foreign
				}
				if mode == "postlink_revocation" {
					f.revoked.Store(true)
				} else {
					f.s.cancel()
				}
				return nil
			})
			if mode == "definite_prelink" || mode == "opaque_unknown" {
				if e != foreign {
					t.Fatal("foreign refusal replaced", e)
				}
			} else if !errors.Is(e, ErrOutcomeUnknown) {
				t.Fatal("possible link reported definite", e)
			}
		})
	}
}
