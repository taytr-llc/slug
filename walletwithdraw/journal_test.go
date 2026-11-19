//go:build !slugtestpow && (darwin || linux)

// Copyright (c) 2026 The SLUG developers. ISC license.
package walletwithdraw

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func allow(fn func(func() error) error) error { return fn(func() error { return nil }) }
func fixtureJournal(t *testing.T) (*Journal, *Owner, journalCase, string) {
	t.Helper()
	c := journalReference(t)[0]
	path := filepath.Join(t.TempDir(), "journal")
	j, e := Create(context.Background(), path, Identity{c.Identity.JournalID, c.Identity.Pin})
	if e != nil {
		t.Fatal(e)
	}
	j.now = func() int64 { return 1795000000100 }
	o, e := j.TryAcquire(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = o.Close(); _ = j.Close() })
	return j, o, c, path
}
func requestFor(c journalCase) Request {
	v := c.Phases[0].Operation.Intent
	return Request{1, v.OperationID, v.Destination, v.RecipientAtoms}
}
func commit(t *testing.T, p *PreparedCommit, e error) Operation {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	v, e := p.Publish(allow)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func phase(c journalCase, name string) Operation {
	for _, p := range c.Phases {
		if p.Name == name {
			return ownedOperation(p.Operation)
		}
	}
	panic(name)
}
func staged(t *testing.T, o *Owner, c journalCase, stage Stage) Operation {
	t.Helper()
	p, e := o.StageIntent(requestFor(c))
	v := commit(t, p, e)
	if stage == StageIntent {
		return v
	}
	d := *phase(c, "prepared").Draft
	p, e = o.StagePrepared(v.OperationID, d)
	v = commit(t, p, e)
	if stage == StagePrepared {
		return v
	}
	s := phase(c, "signed")
	raw, _ := hex.DecodeString(s.Signed.Raw)
	a := s.SignAuthorization
	p, e = o.StageSigned(v.OperationID, raw, AuthorizationInput{a.ActionID, a.ReviewID, a.ViewID, a.Generation})
	v = commit(t, p, e)
	return v
}
func TestWithdrawalJournalRecoveryDeliveryAndCopies(t *testing.T) {
	j, o, c, path := fixtureJournal(t)
	v := staged(t, o, c, StageSigned)
	id := v.OperationID
	r, e := o.Reconciliation(id)
	if e != nil || r.Kind() != "retained" || r.TxID() != v.Signed.TxID || r.Check() != nil {
		t.Fatalf("reconciliation %v", e)
	}
	v.Draft.Inputs[0].Atoms++
	got, e := o.Lookup(id)
	if e != nil || got.Draft.Inputs[0].Atoms == v.Draft.Inputs[0].Atoms {
		t.Fatal("aliased getter")
	}
	if _, e = o.TakeDelivery(id); !errors.Is(e, ErrConflict) {
		t.Fatal("signed grants delivery")
	}
	a := AuthorizationInput{strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64), 1}
	p, e := o.StageDispatch(id, a)
	sent := commit(t, p, e)
	d, e := o.TakeDelivery(id)
	if e != nil {
		t.Fatal(e)
	}
	raw, txid, attempt, e := d.Take()
	if e != nil || hex.EncodeToString(raw) != sent.Signed.Raw || txid != sent.Signed.TxID || attempt != 1 {
		t.Fatal("delivery")
	}
	if _, _, _, e = d.Take(); !errors.Is(e, ErrConflict) {
		t.Fatal("twice")
	}
	if _, e = o.TakeDelivery(id); !errors.Is(e, ErrConflict) {
		t.Fatal("ticket twice")
	}
	p, e = o.StageDispatch(id, a)
	replay := commit(t, p, e)
	if replay.Revision != sent.Revision {
		t.Fatal("replay mutated")
	}
	if _, e = o.TakeDelivery(id); !errors.Is(e, ErrConflict) {
		t.Fatal("replay grants ticket")
	}
	if e = o.Close(); e != nil {
		t.Fatal(e)
	}
	if r.Check() == nil {
		t.Fatal("closed reconciliation")
	}
	if e = j.Close(); e != nil {
		t.Fatal(e)
	}
	j2, e := Open(context.Background(), path, Identity{c.Identity.JournalID, c.Identity.Pin})
	if e != nil {
		t.Fatal(e)
	}
	defer j2.Close()
	j2.now = func() int64 { return 1795000000200 }
	o2, e := j2.TryAcquire(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer o2.Close()
	if _, e = o2.TakeDelivery(id); !errors.Is(e, ErrConflict) {
		t.Fatal("reopened pending sent automatically")
	}
	p, e = o2.StageObservation(id, ObservationInput{State: ObservationMempool, NodeTip: strings.Repeat("4", 64), NodeHeight: 109})
	v = commit(t, p, e)
	if v.Stage != StageSubmitted {
		t.Fatal("not submitted")
	}
	p, e = o2.StageObservation(id, ObservationInput{State: ObservationNotObserved, NodeTip: strings.Repeat("4", 64), NodeHeight: 109})
	v = commit(t, p, e)
	if v.Stage != StageSubmitted || v.Signed == nil {
		t.Fatal("missing released signed")
	}
	if _, e = o2.StageAbandon(id); !errors.Is(e, ErrConflict) {
		t.Fatal("signed abandoned")
	}
	newRequest := requestFor(c)
	newRequest.OperationID = strings.Repeat("5", 64)
	p, e = o2.StageIntent(newRequest)
	nv := commit(t, p, e)
	draft := *phase(c, "prepared").Draft
	draft.OperationID = nv.OperationID
	if _, e = o2.StagePrepared(nv.OperationID, draft); e == nil {
		t.Fatal("reused permanently signed input")
	}
}
func TestWithdrawalObservedSignedWithoutDispatch(t *testing.T) {
	_, o, c, _ := fixtureJournal(t)
	v := staged(t, o, c, StageSigned)
	p, e := o.StageObservation(v.OperationID, ObservationInput{State: ObservationMempool, NodeTip: strings.Repeat("6", 64), NodeHeight: 109})
	v = commit(t, p, e)
	if v.Stage != StageSubmitted || v.Dispatch != nil {
		t.Fatal("observation invented dispatch")
	}
	if _, e = o.TakeDelivery(v.OperationID); e == nil {
		t.Fatal("observation grants dispatch")
	}
}
func TestWithdrawalOriginalFenceFailures(t *testing.T) {
	for _, kind := range []string{"missing", "deny", "swallowed", "double", "outer_failure"} {
		t.Run(kind, func(t *testing.T) {
			j, o, c, path := fixtureJournal(t)
			p, e := o.StageIntent(requestFor(c))
			if e != nil {
				t.Fatal(e)
			}
			f := func(fn func(func() error) error) error {
				switch kind {
				case "missing":
					return nil
				case "deny":
					return ErrUnauthorized
				case "swallowed":
					_ = fn(func() error { return ErrUnauthorized })
					return nil
				case "double":
					_ = fn(func() error { return nil })
					_ = fn(func() error { return nil })
					return nil
				default:
					if e := fn(func() error { return nil }); e != nil {
						return e
					}
					return ErrUnauthorized
				}
			}
			_, e = p.Publish(f)
			possible := kind == "double" || kind == "outer_failure"
			if e == nil || errors.Is(e, ErrOutcomeUnknown) != possible {
				t.Fatalf("failure %v", e)
			}
			_ = p.Close()
			_ = o.Close()
			_ = j.Close()
			j2, e := Open(context.Background(), path, Identity{c.Identity.JournalID, c.Identity.Pin})
			if e != nil {
				t.Fatal(e)
			}
			defer j2.Close()
			o2, e := j2.TryAcquire(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			defer o2.Close()
			s, _ := o2.Snapshot()
			if (s.Count == 1) != possible {
				t.Fatalf("committed %d", s.Count)
			}
		})
	}
}
func TestWithdrawalCommitIOUncertainty(t *testing.T) {
	for _, kind := range []string{"rename", "directory_sync"} {
		t.Run(kind, func(t *testing.T) {
			j, o, c, path := fixtureJournal(t)
			p, e := o.StageIntent(requestFor(c))
			if e != nil {
				t.Fatal(e)
			}
			if kind == "rename" {
				j.rename = func(*os.File, string, string) error { return errors.New("injected") }
			} else {
				j.syncFile = func(f *os.File) error {
					if f == j.chain.dir() {
						return errors.New("injected")
					}
					return f.Sync()
				}
			}
			_, e = p.Publish(allow)
			if !errors.Is(e, ErrOutcomeUnknown) {
				t.Fatalf("not unknown: %v", e)
			}
			_ = p.Close()
			_ = o.Close()
			_ = j.Close()
			j2, e := Open(context.Background(), path, Identity{c.Identity.JournalID, c.Identity.Pin})
			if e != nil {
				t.Fatal(e)
			}
			defer j2.Close()
			o2, e := j2.TryAcquire(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			defer o2.Close()
			s, _ := o2.Snapshot()
			if (s.Count == 1) != (kind == "directory_sync") {
				t.Fatal("wrong recovered state")
			}
		})
	}
}
func TestWithdrawalClockCountersAndCapacity(t *testing.T) {
	j, o, c, _ := fixtureJournal(t)
	r := requestFor(c)
	p, e := o.StageIntent(r)
	v := commit(t, p, e)
	j.now = func() int64 { return v.UpdatedAtMS - 1 }
	if _, e = o.StageAbandon(v.OperationID); !errors.Is(e, ErrClock) {
		t.Fatal("backward clock")
	}
	p, e = o.StageIntent(r)
	again := commit(t, p, e)
	if !same(v, again) {
		t.Fatal("clock replay changed state")
	}
	j.now = func() int64 { return v.UpdatedAtMS }
	p, e = o.StageAbandon(r.OperationID)
	commit(t, p, e)
	for n := 2; n <= MaxOperations; n++ {
		r.OperationID = fmt.Sprintf("%064x", n)
		p, e = o.StageIntent(r)
		op := commit(t, p, e)
		if n < MaxOperations {
			p, e = o.StageAbandon(op.OperationID)
			commit(t, p, e)
		}
	}
	lastID := r.OperationID
	d := *phase(c, "prepared").Draft
	d.OperationID = lastID
	p, e = o.StagePrepared(lastID, d)
	commit(t, p, e)
	signed := phase(c, "signed")
	signedRaw, _ := hex.DecodeString(signed.Signed.Raw)
	a := signed.SignAuthorization
	p, e = o.StageSigned(lastID, signedRaw, AuthorizationInput{a.ActionID, a.ReviewID, a.ViewID, a.Generation})
	commit(t, p, e)
	p, e = o.StageObservation(lastID, ObservationInput{State: ObservationMempool, NodeTip: strings.Repeat("7", 64), NodeHeight: 109})
	commit(t, p, e)
	r.OperationID = fmt.Sprintf("%064x", MaxOperations+1)
	if _, e = o.StageIntent(r); !errors.Is(e, ErrCapacity) {
		t.Fatal("capacity")
	}
	items, e := o.List(112, 16)
	if e != nil || len(items) != 16 {
		t.Fatal("recovery page")
	}
	p, e = o.StageObservation(items[15].OperationID, ObservationInput{State: ObservationUnknown, Error: "node_unavailable"})
	commit(t, p, e)
}
func TestWithdrawalCloseCancelsAndJoinsOwner(t *testing.T) {
	j, o, _, _ := fixtureJournal(t)
	done := make(chan error, 1)
	go func() { done <- j.Close() }()
	select {
	case <-o.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("no cancellation")
	}
	select {
	case <-done:
		t.Fatal("closed before owner join")
	default:
	}
	if _, e := o.Snapshot(); e == nil {
		t.Fatal("closed disclosure")
	}
	_ = o.Close()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("no join")
	}
}
func TestWithdrawalStorageIdentityAndMissingState(t *testing.T) {
	j, o, c, path := fixtureJournal(t)
	p, e := o.StageIntent(requestFor(c))
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(path, stateName))
	if e != nil {
		t.Fatal(e)
	}
	other := filepath.Join(path, "replacement")
	if os.WriteFile(other, b, 0600) != nil || os.Rename(other, filepath.Join(path, stateName)) != nil {
		t.Fatal("replace fixture")
	}
	if _, e = p.Publish(allow); e == nil {
		t.Fatal("substituted inode published")
	}
	_ = p.Close()
	_ = o.Close()
	_ = j.Close()
	if os.Remove(filepath.Join(path, stateName)) != nil {
		t.Fatal("remove")
	}
	if x, e := Open(context.Background(), path, Identity{c.Identity.JournalID, c.Identity.Pin}); e == nil {
		_ = x.Close()
		t.Fatal("missing state opened")
	}
	if x, e := Create(context.Background(), path, Identity{c.Identity.JournalID, c.Identity.Pin}); e == nil {
		_ = x.Close()
		t.Fatal("missing state recreated")
	}
}
func TestWithdrawalExactInitializationResume(t *testing.T) {
	c := journalReference(t)[0]
	id := Identity{c.Identity.JournalID, c.Identity.Pin}
	path := filepath.Join(t.TempDir(), "journal")
	if os.Mkdir(path, 0700) != nil {
		t.Fatal("mkdir")
	}
	empty := state{application, 1, id.JournalID, c.BindingID, 0, []Operation{}}
	raw, _ := json.Marshal(empty)
	if os.WriteFile(filepath.Join(path, stateName), raw, 0600) != nil {
		t.Fatal("initial state")
	}
	j, e := Create(context.Background(), path, id)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	if _, e = Open(context.Background(), path, id); !errors.Is(e, ErrBusy) {
		t.Fatalf("second owner %v", e)
	}
}

func TestWithdrawalRetainedFenceCallbackCannotPublish(t *testing.T) {
	j, o, c, _ := fixtureJournal(t)
	p, e := o.StageIntent(requestFor(c))
	if e != nil {
		t.Fatal(e)
	}
	var saved func(func() error) error
	_, e = p.Publish(func(fn func(func() error) error) error { saved = fn; return nil })
	if !errors.Is(e, ErrUnauthorized) {
		t.Fatal(e)
	}
	if e = saved(func() error { return nil }); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("late callback authorized")
	}
	if len(j.state.Records) != 0 {
		t.Fatal("late mutation")
	}
	_ = p.Close()
}
func TestWithdrawalAsyncBrokenFenceJoinsBeforeRefusal(t *testing.T) {
	j, o, c, _ := fixtureJournal(t)
	p, e := o.StageIntent(requestFor(c))
	if e != nil {
		t.Fatal(e)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	outerReturned := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, e := p.Publish(func(fn func(func() error) error) error {
			go func() { _ = fn(func() error { close(entered); <-release; return nil }) }()
			<-entered
			close(outerReturned)
			return nil
		})
		done <- e
	}()
	<-outerReturned
	select {
	case <-done:
		t.Fatal("did not join callback")
	default:
	}
	j.cancel()
	close(release)
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("async callback accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("join hung")
	}
	if len(j.state.Records) != 0 {
		t.Fatal("invalid async write")
	}
	_ = p.Close()
}
func TestWithdrawalHandleExactJournalProvenance(t *testing.T) {
	j, o, c, _ := fixtureJournal(t)
	v := staged(t, o, c, StageSigned)
	r, e := o.Reconciliation(v.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	j2, _, _, _ := fixtureJournal(t)
	if !j.OwnsReconciliation(r) || j2.OwnsReconciliation(r) {
		t.Fatal("reconciliation provenance")
	}
	a := AuthorizationInput{strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), 1}
	p, e := o.StageDispatch(v.OperationID, a)
	commit(t, p, e)
	d, e := o.TakeDelivery(v.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	if !j.OwnsDelivery(d) || j2.OwnsDelivery(d) {
		t.Fatal("delivery provenance")
	}
	if _, _, _, e = d.Take(); e != nil {
		t.Fatal(e)
	}
	if !j.OwnsDelivery(d) {
		t.Fatal("provenance changed on consumption")
	}
	if _, _, _, e = d.Take(); e == nil {
		t.Fatal("provenance revived ticket")
	}
	_ = o.Close()
	if j.OwnsDelivery(d) || j.OwnsReconciliation(r) {
		t.Fatal("closed owner provenance")
	}
}
