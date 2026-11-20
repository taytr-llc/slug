// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestCareerEvidenceOriginalCaptureOneProofAndOwnedValues(t *testing.T) {
	x, id, state, _, _ := historyFixture(t)
	state.Events = []careerstate.Event{historyFixtureEvent(x, state, id)}
	c, h, e := x.s.PrepareCareerHistory(t.Context(), x.session, id)
	if e != nil {
		t.Fatal(e)
	}
	// The current owner can acquire another award; the original selection is
	// deliberately retained, never replaced by a fresh capture during export.
	state.Events = nil
	proofs := 0
	inLease := false
	session := x.session
	session.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if cap != gymaccount.View {
			t.Fatal("public export requested secret capability")
		}
		inLease = true
		defer func() { inLease = false }()
		return x.session.Lease(ctx, cap, fn)
	}
	x.s.proofRead = func() {
		proofs++
		if inLease {
			t.Fatal("proof held account lease")
		}
		if !x.s.mu.TryLock() {
			t.Fatal("proof held stable lock")
		}
		x.s.mu.Unlock()
	}
	got, e := x.s.ReadCareerHistoryEvidence(t.Context(), session, h)
	if e != nil || proofs != 1 || got.Capture != c || len(got.Rows) != 1 || !reflect.DeepEqual(got.Admission, x.records[0]) {
		t.Fatal("original selected evidence", proofs, e)
	}
	before, _ := json.Marshal(got)
	got.Rows[0].Event.Method = "changed"
	got.Admission.Proof.MerkleBranch = append(got.Admission.Proof.MerkleBranch, "changed")
	again, e := x.s.ReadCareerHistoryEvidence(t.Context(), session, h)
	after, _ := json.Marshal(again)
	if e != nil || proofs != 2 || string(before) != string(after) {
		t.Fatal("returned values mutated trusted source/history", e)
	}
}

func TestCareerEvidenceRefusesForeignAndRevokedOriginalView(t *testing.T) {
	x, id, _, _, _ := historyFixture(t)
	_, h, e := x.s.PrepareCareerHistory(t.Context(), x.session, id)
	if e != nil {
		t.Fatal(e)
	}
	proofs := 0
	x.s.proofRead = func() { proofs++ }
	for _, bad := range []*CareerHistoryRead{nil, {}, {owner: x.s}} {
		got, e := x.s.ReadCareerHistoryEvidence(t.Context(), x.session, bad)
		if !errors.Is(e, ErrInvalid) || got.Rows != nil || got.Admission.Version != 0 {
			t.Fatal("invalid handle", e)
		}
	}
	stale := x.session
	stale.Generation++
	if got, e := x.s.ReadCareerHistoryEvidence(t.Context(), stale, h); !errors.Is(e, ErrStaleView) || got.Rows != nil {
		t.Fatal("replaced session", e)
	}
	if proofs != 0 {
		t.Fatal("precondition failures started proof")
	}
	revoked := false
	session := x.session
	session.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if revoked {
			return gymaccount.ErrUnauthorized
		}
		return x.session.Lease(ctx, cap, fn)
	}
	x.s.proofRead = func() { proofs++; revoked = true }
	got, e := x.s.ReadCareerHistoryEvidence(t.Context(), session, h)
	if !errors.Is(e, ErrUnauthorized) || got.Rows != nil || got.Admission.Version != 0 || proofs != 1 {
		t.Fatal("reset during source proof published stale evidence", e, proofs)
	}
}

func TestCareerEvidenceCloseJoinsProof(t *testing.T) {
	x, id, _, _, _ := historyFixture(t)
	_, h, e := x.s.PrepareCareerHistory(t.Context(), x.session, id)
	if e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	x.s.proofRead = func() { close(entered); <-release }
	done := make(chan error, 1)
	go func() {
		v, e := x.s.ReadCareerHistoryEvidence(t.Context(), x.session, h)
		if v.Rows != nil || v.Admission.Version != 0 {
			done <- errors.New("cancel published evidence")
			return
		}
		done <- e
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		<-done
		t.Fatal("proof boundary never entered")
	}
	closed := make(chan error, 1)
	go func() { closed <- x.s.Close() }()
	select {
	case <-closed:
		close(release)
		<-done
		t.Fatal("Close detached proof")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if e := <-done; !errors.Is(e, ErrCancelled) && !errors.Is(e, ErrClosed) {
		t.Fatal("joined cancellation", e)
	}
	if e := <-closed; e != nil {
		t.Fatal(e)
	}
}

func TestCareerEvidenceCheapProfileBeforeHandle(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	if got, e := (*Store)(nil).ReadCareerHistoryEvidence(nil, Session{}, nil); e == nil || got.Rows != nil || got.Admission.Version != 0 {
		t.Fatal("profile input refusal", e)
	}
}
