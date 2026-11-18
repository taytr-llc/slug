//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/seednode"
	"strings"
	"testing"
)

type modelAttestation struct {
	*modelResultChain
	cp    chaincfg.SignedCheckpoint
	calls int
	after func()
}

func (m *modelAttestation) AttestInclusion(context.Context, Anchor) (chaincfg.SignedCheckpoint, Tip, error) {
	m.calls++
	if m.after != nil {
		m.after()
	}
	return m.cp, m.tip, nil
}
func TestResultAttestationSourceCheckedBeforeAndAfterSigning(t *testing.T) {
	for _, phase := range []string{"before", "after", "rpc_after"} {
		t.Run(phase, func(t *testing.T) {
			f := newResultFixture(t)
			r := referenceRecord(t, "record-included59")
			installModeledResult(t, f, r)
			final := referenceRecord(t, "record-finalized60")
			m := &modelAttestation{modelResultChain: modelTip(r), cp: *final.Checkpoint}
			m.tip = final.Finality.ObservedTip
			m.hashes[m.tip.Height] = m.tip.Hash
			m.hashes[r.Inclusion.Anchor.Height] = r.Inclusion.Anchor.Hash
			change := func() { m.hashes[r.Statement.Statement.Source.Source.TargetHeight] = strings.Repeat("e", 64) }
			if phase == "before" {
				change()
			} else if phase == "after" {
				m.after = change
			} else {
				m.after = func() { m.err = seednode.ErrOffline }
			}
			c, done := fixtureCall(t, f)
			defer done()
			_, e := f.s.attest(c, m)
			want := ErrSourceConflict
			if phase == "rpc_after" {
				want = ErrNodeOffline
			}
			if !errors.Is(e, want) {
				t.Fatal("source/signature fence", phase, e)
			}
			expectCalls := 1
			if phase == "before" {
				expectCalls = 0
			}
			if m.calls != expectCalls {
				t.Fatal("signed before source check", m.calls)
			}
			got, ok, e := f.s.record(r.ResultKey)
			if e != nil || !ok || got.Checkpoint != nil || got.Finality != nil {
				t.Fatal("certificate escaped failed source fence", e)
			}
			conflicts := 1
			if phase == "rpc_after" {
				conflicts = 0
			}
			if len(got.Conflicts) != conflicts {
				t.Fatal("RPC failure became conflict", len(got.Conflicts))
			}
			auditCurrent(t, f)
		})
	}
}
func TestResultPublishTargetMismatchBeforeSigning(t *testing.T) {
	f, c, done, r := preparedFixture(t)
	defer done()
	m := fundedPublisher(t, f, r)
	m.hashes[r.Statement.Statement.Source.Source.TargetHeight] = strings.Repeat("e", 64)
	if _, e := f.s.publish(c, m); !errors.Is(e, ErrSourceConflict) || m.builds != 0 || m.sends != 0 {
		t.Fatal("signed/sent before fresh target", e, m.builds, m.sends)
	}
	auditCurrent(t, f)
}
