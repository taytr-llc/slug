//go:build !slugtestpow && (darwin || linux)

// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type carrierFixture struct {
	identity Identity
	prep     Preparation
	pair     SignedPackage
	stages   map[string]Operation
}

func journalFixture(t *testing.T) carrierFixture {
	return journalFixtureNamed(t, "zero256/hosted/transfer/one/")
}
func journalFixtureNamed(t *testing.T, prefix string) carrierFixture {
	t.Helper()
	v := readWalletReference(t)
	c := carrierFixture{stages: make(map[string]Operation)}
	for _, r := range v.Positives {
		if !strings.HasPrefix(r.Name, prefix) {
			continue
		}
		var target any
		switch r.Kind {
		case "journal_marker":
			var m journalMarker
			if err := json.Unmarshal(r.Value, &m); err != nil {
				t.Fatal(err)
			}
			c.identity = Identity{JournalID: m.JournalID, Pin: m.Pin}
		case "preparation":
			target = &c.prep
		case "signed_package":
			target = &c.pair
		case "operation":
			var op Operation
			if err := json.Unmarshal(r.Value, &op); err != nil {
				t.Fatal(err)
			}
			c.stages[op.Stage] = op
		}
		if target != nil {
			if err := json.Unmarshal(r.Value, target); err != nil {
				t.Fatal(err)
			}
		}
	}
	if c.identity.JournalID == "" || c.pair.Commit.Raw == "" || len(c.stages) < 6 {
		t.Fatal("missing independent fixture")
	}
	return c
}

func TestCarrierJournalRestoredRecoveryHasOneDeliveryLeg(t *testing.T) {
	for _, kind := range []string{"recover_reveal", "recover_refund"} {
		t.Run(kind, func(t *testing.T) {
			c := journalFixtureNamed(t, "zero256/restored/"+kind+"/recovery/")
			path := filepath.Join(t.TempDir(), "journal")
			j, err := Create(context.Background(), path, c.identity)
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			j.now = func() int64 { return c.prep.PreparedAtMS - 1 }
			o, err := j.TryAcquire(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer o.Close()
			v := journalSigned(t, j, o, c)
			if v.SignAuthorization.Mode != "restored" || v.SignAuthorization.Generation != 0 {
				t.Fatal("restored owner acquired hosted approval")
			}
			a := approvalFor(&c.stages["spend_dispatching"].SpendDispatch.Authorization)
			if _, err = o.StageDispatch(v.RequestID, "commit", a); !errors.Is(err, ErrConflict) {
				t.Fatal("recovery dispatched a new commit")
			}
			p, err := o.StageDispatch(v.RequestID, "spend", a)
			journalPublish(t, p, err)
			d, err := o.TakeDelivery(v.RequestID, "spend")
			if err != nil {
				t.Fatal(err)
			}
			raw, id, attempt, err := d.Take()
			if err != nil || id != c.pair.Spend.TxID || hex.EncodeToString(raw) != c.pair.Spend.Raw || attempt != 1 {
				t.Fatal("recovery changed independent spend bytes")
			}
			p, err = o.StageObservation(v.RequestID, "spend", *c.stages["complete"].SpendObservation)
			v = journalPublish(t, p, err)
			if v.Stage != "complete" || v.CommitDispatch != nil || v.Preparation.RecoveryFunding == nil || v.Preparation.NewFunding != nil {
				t.Fatal("recovery invented funding or commit delivery")
			}
			if _, err = o.StageAbandon(v.RequestID); !errors.Is(err, ErrConflict) {
				t.Fatal("signed recovery abandoned")
			}
		})
	}
}

func fixtureOwner(t *testing.T) (*Journal, *Owner, carrierFixture, string) {
	t.Helper()
	c := journalFixture(t)
	path := filepath.Join(t.TempDir(), "journal")
	j, err := Create(context.Background(), path, c.identity)
	if err != nil {
		t.Fatal(err)
	}
	j.now = func() int64 { return c.prep.PreparedAtMS - 1 }
	o, err := j.TryAcquire(context.Background())
	if err != nil {
		_ = j.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Close(); _ = j.Close() })
	return j, o, c, path
}

func journalAllow(fn func(func() error) error) error { return fn(func() error { return nil }) }
func journalPublish(t *testing.T, p *PreparedCommit, err error) Operation {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	v, err := p.Publish(journalAllow)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func approvalFor(a *Authorization) AuthorizationInput {
	return AuthorizationInput{a.ActionID, a.ReviewID, a.ViewID, a.Generation}
}
func journalSigned(t *testing.T, j *Journal, o *Owner, c carrierFixture) Operation {
	t.Helper()
	p, err := o.StageIntent(c.prep.Request)
	journalPublish(t, p, err)
	j.now = func() int64 { return c.prep.PreparedAtMS }
	p, err = o.StagePrepared(c.prep.Request.RequestID, c.prep)
	journalPublish(t, p, err)
	j.now = func() int64 { return c.prep.PreparedAtMS + 100 }
	p, err = o.StageSigned(c.prep.Request.RequestID, c.pair, approvalFor(c.stages["signed"].SignAuthorization))
	v := journalPublish(t, p, err)
	clock := c.prep.PreparedAtMS + 101
	j.now = func() int64 { clock++; return clock }
	return v
}

func TestCarrierJournalRecoveryDeliveryAndCopies(t *testing.T) {
	j, o, c, path := fixtureOwner(t)
	v := journalSigned(t, j, o, c)
	id := v.RequestID
	r, err := o.Reconciliation(id)
	if err != nil || !j.OwnsReconciliation(r) || r.Kind() != "retained" || r.CommitTxID() != c.pair.Commit.TxID || r.SpendTxID() != c.pair.Spend.TxID {
		t.Fatalf("reconciliation: %v", err)
	}
	v.Preparation.NewFunding.Inputs[0].Atoms++
	got, err := o.Lookup(id)
	if err != nil || got.Preparation.NewFunding.Inputs[0].Atoms != c.prep.NewFunding.Inputs[0].Atoms {
		t.Fatal("getter aliases state")
	}
	if _, err = o.TakeDelivery(id, "commit"); !errors.Is(err, ErrConflict) {
		t.Fatal("signed granted delivery")
	}
	a := approvalFor(&c.stages["commit_dispatching"].CommitDispatch.Authorization)
	if _, err = o.StageDispatch(id, "spend", a); !errors.Is(err, ErrConflict) {
		t.Fatal("spend before commit")
	}
	p, err := o.StageDispatch(id, "commit", a)
	sent := journalPublish(t, p, err)
	d, err := o.TakeDelivery(id, "commit")
	if err != nil {
		t.Fatal(err)
	}
	raw, txid, attempt, err := d.Take()
	if err != nil || hex.EncodeToString(raw) != c.pair.Commit.Raw || txid != c.pair.Commit.TxID || attempt != 1 {
		t.Fatal("exact commit delivery")
	}
	if _, _, _, err = d.Take(); !errors.Is(err, ErrConflict) {
		t.Fatal("delivered twice")
	}
	p, err = o.StageDispatch(id, "commit", a)
	replay := journalPublish(t, p, err)
	if replay.Revision != sent.Revision {
		t.Fatal("same action made a new dispatch")
	}
	if _, err = o.TakeDelivery(id, "commit"); !errors.Is(err, ErrConflict) {
		t.Fatal("replay granted delivery")
	}
	changed := a
	changed.ReviewID = strings.Repeat("a", 64)
	if _, err = o.StageDispatch(id, "commit", changed); !errors.Is(err, ErrConflict) {
		t.Fatal("same action changed review")
	}
	if _, err = o.StageAbandon(id); !errors.Is(err, ErrConflict) {
		t.Fatal("signed abandonment")
	}
	if err = o.Close(); err != nil {
		t.Fatal(err)
	}
	if r.Check() == nil || j.OwnsDelivery(d) {
		t.Fatal("closed capability survived")
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j2, err := Open(context.Background(), path, c.identity)
	if err != nil {
		t.Fatal(err)
	}
	defer j2.Close()
	clock := c.prep.PreparedAtMS + 200
	j2.now = func() int64 { clock++; return clock }
	o2, err := j2.TryAcquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer o2.Close()
	if _, err = o2.TakeDelivery(id, "commit"); !errors.Is(err, ErrConflict) {
		t.Fatal("reopen silently dispatched")
	}
	a.ActionID = strings.Repeat("b", 64)
	p, err = o2.StageDispatch(id, "commit", a)
	journalPublish(t, p, err)
	d2, err := o2.TakeDelivery(id, "commit")
	if err != nil {
		t.Fatal(err)
	}
	raw2, txid2, attempt2, err := d2.Take()
	if err != nil || hex.EncodeToString(raw2) != hex.EncodeToString(raw) || txid2 != txid || attempt2 != 2 {
		t.Fatal("retry reselected or resigned")
	}
	p, err = o2.StageDeliveryResult(id, "commit", 2, "unknown", "outcome_unknown")
	journalPublish(t, p, err)
	if _, err = o2.StageDispatch(id, "spend", a); !errors.Is(err, ErrConflict) {
		t.Fatal("unknown delivery treated as observation")
	}
	p, err = o2.StageObservation(id, "commit", *c.stages["commit_observed"].CommitObservation)
	journalPublish(t, p, err)
	p, err = o2.StageDispatch(id, "spend", a)
	journalPublish(t, p, err)
	sd, err := o2.TakeDelivery(id, "spend")
	if err != nil {
		t.Fatal(err)
	}
	sraw, sid, _, err := sd.Take()
	if err != nil || hex.EncodeToString(sraw) != c.pair.Spend.Raw || sid != c.pair.Spend.TxID {
		t.Fatal("exact retained spend")
	}
	p, err = o2.StageObservation(id, "spend", *c.stages["complete"].SpendObservation)
	v = journalPublish(t, p, err)
	if v.Stage != "complete" {
		t.Fatal("not complete")
	}
	r2, err := o2.Reconciliation(id)
	if err != nil || r2.Kind() != "complete" || !j2.OwnsReconciliation(r2) || j.OwnsReconciliation(r2) {
		t.Fatal("completion provenance")
	}
	s, err := o2.Snapshot()
	if err != nil || s.ActiveRequestID != "" {
		t.Fatal("completion kept unresolved slot")
	}
	// Journal completion records delivery only. Public replay owns ownership.
	if _, err = o2.StageObservation(id, "spend", Observation{}); !errors.Is(err, ErrConflict) {
		t.Fatal("rewrote terminal delivery")
	}
}

func TestCarrierJournalOriginalFenceFailures(t *testing.T) {
	for _, kind := range []string{"missing", "deny", "swallowed", "double", "outer_failure"} {
		t.Run(kind, func(t *testing.T) {
			j, o, c, path := fixtureOwner(t)
			p, err := o.StageIntent(c.prep.Request)
			if err != nil {
				t.Fatal(err)
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
			_, err = p.Publish(f)
			possible := kind == "double" || kind == "outer_failure"
			if err == nil || errors.Is(err, ErrOutcomeUnknown) != possible {
				t.Fatalf("failure: %v", err)
			}
			_ = p.Close()
			_ = o.Close()
			_ = j.Close()
			j2, err := Open(context.Background(), path, c.identity)
			if err != nil {
				t.Fatal(err)
			}
			defer j2.Close()
			o2, err := j2.TryAcquire(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer o2.Close()
			s, err := o2.Snapshot()
			if err != nil || (s.Count == 1) != possible {
				t.Fatal("wrong recovered publication")
			}
		})
	}
}

func TestCarrierJournalPublicationUncertainty(t *testing.T) {
	for _, kind := range []string{"rename_before", "rename_after", "directory_sync"} {
		t.Run(kind, func(t *testing.T) {
			j, o, c, path := fixtureOwner(t)
			p, err := o.StageIntent(c.prep.Request)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "directory_sync" {
				j.syncFile = func(f *os.File) error {
					if f == j.chain.dir() {
						return ErrStorage
					}
					return f.Sync()
				}
			} else {
				j.rename = func(d *os.File, from, to string) error {
					if kind == "rename_after" {
						if e := renameAt(d, from, to); e != nil {
							return e
						}
					}
					return ErrStorage
				}
			}
			if _, err = p.Publish(journalAllow); !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatal("uncertain write not poisoned")
			}
			if _, err = o.Lookup(c.prep.Request.RequestID); !errors.Is(err, ErrPoisoned) {
				t.Fatal("poisoned owner read")
			}
			_ = p.Close()
			_ = o.Close()
			_ = j.Close()
			j2, err := Open(context.Background(), path, c.identity)
			if err != nil {
				t.Fatal(err)
			}
			defer j2.Close()
			o2, err := j2.TryAcquire(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer o2.Close()
			s, _ := o2.Snapshot()
			if (s.Count == 1) != (kind != "rename_before") {
				t.Fatal("recovery ignored visible state")
			}
			if _, err = o2.TakeDelivery(c.prep.Request.RequestID, "commit"); !errors.Is(err, ErrConflict) {
				t.Fatal("recovered delivery authority")
			}
		})
	}
}

func TestCarrierJournalOrphanStageAuditAndRefusal(t *testing.T) {
	for _, kind := range []string{"valid_successor", "foreign", "malformed", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			j, o, c, path := fixtureOwner(t)
			p, err := o.StageIntent(c.prep.Request)
			if err != nil {
				t.Fatal(err)
			}
			// Keep exact pre-publication bytes, then simulate a crash after staging.
			raw, err := os.ReadFile(filepath.Join(path, p.name))
			if err != nil {
				t.Fatal(err)
			}
			name := p.name
			_ = p.Close()
			_ = o.Close()
			_ = j.Close()
			switch kind {
			case "foreign":
				raw = []byte(strings.Replace(string(raw), c.identity.JournalID, strings.Repeat("c", 64), 1))
			case "malformed":
				raw = append(raw, '\n')
			case "oversized":
				raw = make([]byte, journalMaxBytes+1)
			}
			stagePath := filepath.Join(path, name)
			if err = os.WriteFile(stagePath, raw, 0600); err != nil {
				t.Fatal(err)
			}
			a, err := OpenAudit(context.Background(), path, c.identity)
			if kind != "valid_successor" {
				if err == nil {
					_ = a.Close()
					t.Fatal("invalid orphan accepted")
				}
				if _, e := os.Stat(stagePath); e != nil {
					t.Fatal("invalid evidence removed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = a.TryAcquire(context.Background()); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("audit grants writer")
			}
			if err = a.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(stagePath); err != nil {
				t.Fatal("audit removed stage")
			}
			j2, err := Open(context.Background(), path, c.identity)
			if err != nil {
				t.Fatal(err)
			}
			defer j2.Close()
			o2, err := j2.TryAcquire(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer o2.Close()
			s, _ := o2.Snapshot()
			if s.Count != 0 {
				t.Fatal("orphan successor adopted without approval")
			}
			if _, err = os.Stat(stagePath); !os.IsNotExist(err) {
				t.Fatal("valid abandoned stage not cleaned")
			}
		})
	}
}

func TestCarrierJournalIdentityAndFilesystemFences(t *testing.T) {
	for _, kind := range []string{"wrong_identity", "symlink", "hardlink", "missing", "replaced_inode"} {
		t.Run(kind, func(t *testing.T) {
			j, o, c, path := fixtureOwner(t)
			statePath := filepath.Join(path, journalStateName)
			if kind == "replaced_inode" {
				p, err := o.StageIntent(c.prep.Request)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(statePath)
				if err != nil {
					t.Fatal(err)
				}
				tmp := filepath.Join(path, "replacement")
				if os.WriteFile(tmp, raw, 0600) != nil || os.Rename(tmp, statePath) != nil {
					t.Fatal("replace")
				}
				if _, err = p.Publish(journalAllow); err == nil {
					t.Fatal("inode replacement ignored")
				}
				_ = p.Close()
				return
			}
			_ = o.Close()
			_ = j.Close()
			identity := c.identity
			switch kind {
			case "wrong_identity":
				identity.JournalID = strings.Repeat("d", 64)
			case "missing":
				if err := os.Remove(statePath); err != nil {
					t.Fatal(err)
				}
			case "symlink", "hardlink":
				other := filepath.Join(filepath.Dir(path), "original")
				if err := os.Rename(statePath, other); err != nil {
					t.Fatal(err)
				}
				var err error
				if kind == "symlink" {
					err = os.Symlink(other, statePath)
				} else {
					err = os.Link(other, statePath)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if x, err := Open(context.Background(), path, identity); err == nil {
				_ = x.Close()
				t.Fatal("unsafe state opened")
			}
		})
	}
}

func TestCarrierJournalCloseAndBrokenAsyncFence(t *testing.T) {
	t.Run("close_joins_owner", func(t *testing.T) {
		j, o, _, _ := fixtureOwner(t)
		done := make(chan error, 1)
		go func() { done <- j.Close() }()
		select {
		case <-o.ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("no cancellation")
		}
		select {
		case <-done:
			t.Fatal("closed before owner joined")
		default:
		}
		if _, err := o.Snapshot(); err == nil {
			t.Fatal("closed disclosure")
		}
		_ = o.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("no join")
		}
	})
	t.Run("retained_callback", func(t *testing.T) {
		j, o, c, _ := fixtureOwner(t)
		p, err := o.StageIntent(c.prep.Request)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		var saved func(func() error) error
		_, err = p.Publish(func(fn func(func() error) error) error { saved = fn; return nil })
		if !errors.Is(err, ErrUnauthorized) || saved(func() error { return nil }) == nil || len(j.state.Records) != 0 {
			t.Fatal("late callback published")
		}
	})
	t.Run("asynchronous_callback", func(t *testing.T) {
		j, o, c, _ := fixtureOwner(t)
		p, err := o.StageIntent(c.prep.Request)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		entered := make(chan struct{})
		release := make(chan struct{})
		returned := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			_, err := p.Publish(func(fn func(func() error) error) error {
				go func() { _ = fn(func() error { close(entered); <-release; return nil }) }()
				<-entered
				close(returned)
				return nil
			})
			done <- err
		}()
		<-returned
		select {
		case <-done:
			t.Fatal("did not join callback")
		default:
		}
		j.cancel()
		close(release)
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("broken fence accepted")
			}
		case <-time.After(time.Second):
			t.Fatal("join hung")
		}
		if len(j.state.Records) != 0 {
			t.Fatal("invalid async write")
		}
	})
}

func TestCarrierJournalUnsignedAbandonmentAndClock(t *testing.T) {
	j, o, c, _ := fixtureOwner(t)
	p, err := o.StageIntent(c.prep.Request)
	intent := journalPublish(t, p, err)
	j.now = func() int64 { return intent.UpdatedAtMS - 1 }
	if _, err = o.StageAbandon(intent.RequestID); !errors.Is(err, ErrClock) {
		t.Fatal("backward clock")
	}
	j.now = func() int64 { return c.prep.PreparedAtMS }
	p, err = o.StagePrepared(intent.RequestID, c.prep)
	journalPublish(t, p, err)
	if _, err = o.Reconciliation(intent.RequestID); !errors.Is(err, ErrConflict) {
		t.Fatal("prepared state releases exclusions")
	}
	if _, err = o.StageAbandon(intent.RequestID); !errors.Is(err, ErrClock) {
		t.Fatal("equal timestamp accepted for mutation")
	}
	j.now = func() int64 { return c.prep.PreparedAtMS + 1 }
	p, err = o.StageAbandon(intent.RequestID)
	journalPublish(t, p, err)
	r, err := o.Reconciliation(intent.RequestID)
	if err != nil || r.Kind() != "abandoned" || !j.OwnsReconciliation(r) {
		t.Fatal("unsigned abandonment reconciliation")
	}
	if _, err = o.TakeDelivery(intent.RequestID, "commit"); !errors.Is(err, ErrConflict) {
		t.Fatal("unsigned delivery")
	}
}
