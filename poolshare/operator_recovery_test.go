// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/internal/poolcapacity"
	"github.com/taytr-llc/slug/prospectwork"
)

// These are leaf controls using the preexisting independently hashed normal-PoW
// fixture. The leaf fills its actual selected ordinary envelope; larger183-row
// headroom has independent arithmetic coverage, not an actual-run claim.
// The node/session adapters remain explicit controlled models.
func recoveryTestBudget() RecoveryBudget {
	return RecoveryBudget{Rounds: 4, RoundBytes: 8 << 20, CreditRounds: 4, Batches: 4, Gyms: 2}
}

func recoveryTestAttach(t *testing.T, h *poolHarness, maximum uint64, budget RecoveryBudget) *OperatorAdmission {
	t.Helper()
	a, err := h.pool.AttachOperatorAdmission(context.Background(), h.config, maximum)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := poolcapacity.Envelope(a.recoveryInputs(budget))
	if err != nil || ordinary.Submissions != maximum {
		t.Fatal("leaf must fill the actual selected envelope", ordinary, maximum, err)
	}
	if err = a.EnableCanonicalRecovery(context.Background(), budget); err != nil {
		t.Fatal(err)
	}
	return a
}

func recoveryTestFull(t *testing.T, physical uint64) (*poolHarness, *OperatorAdmission, []Submission) {
	t.Helper()
	h := setupPool(t, "", physical)
	a := recoveryTestAttach(t, h, 2, recoveryTestBudget())
	if err := a.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := poolFixture(t).caseByName(t, "coinbase-only-nonce-1")
	if h.ordinary.Block || second.Block || !h.winner.Block {
		t.Fatal("fixture must contain two ordinary rows and a separate winning nonce")
	}
	var accepted []Submission
	for _, tc := range []testCase{h.ordinary, second} {
		r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, tc.Nonce)
		if err != nil || r.AcceptedSequence != uint64(len(accepted)+1) {
			t.Fatal("ordinary fill", r, err)
		}
		assertFrozenSubmission(t, r, tc, h.source, h.config.PoolTarget)
		accepted = append(accepted, r)
	}
	if status, err := a.Status(context.Background()); err != nil || status.Mode != "draining" || status.Submissions != 2 {
		t.Fatal("ordinary ceiling not reached", status, err)
	}
	return h, a, accepted
}

func TestOperatorCanonicalRecoveryUnseenAfterDrainAndReopen(t *testing.T) {
	h, a, ordinary := recoveryTestFull(t, 4)
	ctx := context.Background()
	issuerBefore, err := h.authority.Info()
	if err != nil {
		t.Fatal(err)
	}
	h.now = h.source.Job.ExpiresAtMS + 5
	h.node.canonicalErr = nil
	got, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil {
		t.Fatal("unseen canonical expired winner", err)
	}
	assertFrozenSubmission(t, got, h.winner, h.source, h.config.PoolTarget)
	if got.Sequence != 3 || got.AcceptedSequence != 3 || got.Acceptance != "canonical-late-winner" || got.BlockState != "canonical-observed" || got.ReceivedAtMS != h.now || got.AcceptedAtMS != h.now || got.BlockObservedAtMS != h.now {
		t.Fatal("canonical receipt/clock fields", got)
	}
	if h.node.broadcasts != 0 || h.node.canonicalChecks != 1 {
		t.Fatal("recovery broadcast or changed canonical proof", h.node.broadcasts, h.node.canonicalChecks)
	}
	if info, e := h.authority.Info(); e != nil || !reflect.DeepEqual(info, issuerBefore) {
		t.Fatal("pool-only recovery changed issuer/fighter evidence", e)
	}
	if _, err = h.pool.Issue(ctx, poolTestToken); err != ErrCapacity {
		t.Fatal("recovery reopened ordinary issuance", err)
	}
	h.reopen(t)
	if _, err = a.Status(ctx); err != ErrClosed {
		t.Fatal("old recovery controller survived reopen", err)
	}
	a = recoveryTestAttach(t, h, 2, recoveryTestBudget())
	if status, e := a.Status(ctx); e != nil || status.Mode != "draining" || status.Submissions != 3 || status.RecoveryBlocks != 1 || !status.CanonicalRecovery || status.OverBound {
		t.Fatal("reopened recovery accounting", status, e)
	}
	if err = a.with(ctx, func() error {
		blocks, gyms, e := h.pool.recoveryRows(ctx)
		if e == nil && (blocks != 1 || len(gyms) != 1 || !gyms[h.source.Job.GymID]) {
			t.Error("three original rows did not deduplicate the original claim gym")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	checks := h.node.canonicalChecks
	h.now++
	duplicate, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || duplicate != got || h.pool.meta.Submissions != 3 || h.pool.meta.Accepted != 3 || h.node.canonicalChecks != checks || h.node.broadcasts != 0 {
		t.Fatal("original identity not recovered exactly", duplicate, err)
	}
	page, err := h.pool.AccountingPage(ctx, 0, 0, 3)
	if err != nil || page.Through != 3 || len(page.Entries) != 3 {
		t.Fatal("accepted tail", err)
	}
	for i, r := range ordinary {
		r.BlockState, r.BlockObservedAtMS = "", 0
		if page.Entries[i].Share != r {
			t.Fatal("ordinary prefix changed")
		}
	}
	if page.Entries[2].Share.DuplicateKey != got.DuplicateKey || page.Entries[2].Proof == nil || page.Entries[2].Proof.MerkleBranch == nil || len(page.Entries[2].Proof.MerkleBranch) != 0 {
		t.Fatal("original empty-branch proof changed")
	}
}

func TestOperatorCanonicalRecoveryRefusesNoncanonicalAndWrongSession(t *testing.T) {
	t.Run("noncanonical", func(t *testing.T) {
		h, _, _ := recoveryTestFull(t, 4)
		h.now = h.source.Job.ExpiresAtMS + 1
		if r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); err != ErrNode || r != (Submission{}) {
			t.Fatal("noncanonical recovery accepted", r, err)
		}
		if h.pool.meta.Submissions != 2 || h.pool.meta.Accepted != 2 || h.node.broadcasts != 0 {
			t.Fatal("noncanonical failure had receipt/broadcast effects")
		}
		if _, err := h.pool.loadSubmission(h.source.Job.ID, h.winner.Nonce); err != ErrNotFound {
			t.Fatal("noncanonical row retained", err)
		}
	})
	t.Run("wrong-gym", func(t *testing.T) {
		h, _, _ := recoveryTestFull(t, 4)
		h.pool.sessions = newFenceSessions(h.source.Job.PublicKey)
		h.node.canonicalErr = nil
		if r, err := h.pool.Submit(context.Background(), "other-gym", h.source.Job.ID, h.winner.Nonce); !errors.Is(err, ErrUnauthorized) || r != (Submission{}) {
			t.Fatal("wrong gym recovered work", r, err)
		}
		if h.pool.meta.Submissions != 2 || h.node.canonicalChecks != 0 || h.node.broadcasts != 0 {
			t.Fatal("wrong gym reached node/commit")
		}
	})
	t.Run("ordinary", func(t *testing.T) {
		h := setupPool(t, "", 4)
		budget := recoveryTestBudget()
		budget.Batches = 2
		a := recoveryTestAttach(t, h, 1, budget)
		if err := a.Enable(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce); err != nil {
			t.Fatal(err)
		}
		h.now = h.source.Job.ExpiresAtMS + 1
		ordinary := poolFixture(t).caseByName(t, "coinbase-only-nonce-1")
		if r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, ordinary.Nonce); err != ErrCapacity || r != (Submission{}) {
			t.Fatal("ordinary late row used recovery allowance", r, err)
		}
		if h.pool.meta.Submissions != 1 || h.node.canonicalChecks != 0 || h.node.broadcasts != 0 {
			t.Fatal("ordinary recovery had effects")
		}
	})
}

func TestOperatorCanonicalRecoveryRevocationAndFinalClock(t *testing.T) {
	t.Run("revoked-during-canonical-check", func(t *testing.T) {
		h, _, _ := recoveryTestFull(t, 4)
		sessions := newFenceSessions(h.source.Job.PublicKey)
		h.pool.sessions = sessions
		h.node.canonicalErr = nil
		h.now = h.source.Job.ExpiresAtMS + 1
		h.node.onCanonical = func() {
			if sessions.held.Load() {
				t.Error("canonical node call holds account lease")
				return
			}
			if err := sessions.revoke(context.Background(), poolTestToken); err != nil {
				t.Error(err)
			}
		}
		if r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); !errors.Is(err, ErrUnauthorized) || r != (Submission{}) {
			t.Fatal("revoked operation accepted/disclosed", r, err)
		}
		if h.pool.meta.Submissions != 2 || h.pool.meta.Accepted != 2 || h.node.broadcasts != 0 {
			t.Fatal("revoked canonical response committed")
		}
	})
	t.Run("final-clock-crosses-expiry", func(t *testing.T) {
		h, _, _ := recoveryTestFull(t, 4)
		h.now = h.source.Job.ExpiresAtMS - 1
		h.node.canonicalErr = nil
		h.node.onCanonical = func() { h.now = h.source.Job.ExpiresAtMS + 7 }
		r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
		if err != nil || r.ReceivedAtMS != h.source.Job.ExpiresAtMS-1 || r.AcceptedAtMS != h.now || r.BlockObservedAtMS != h.now || r.Acceptance != "canonical-late-winner" {
			t.Fatal("reused earlier canonical acceptance time", r, err)
		}
		h.reopen(t)
		got, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
		if err != nil || got != r {
			t.Fatal("final timestamp failed restart audit", got, err)
		}
	})
}

func TestOperatorCanonicalRecoveryUncertainCommitReopen(t *testing.T) {
	for _, afterSync := range []bool{false, true} {
		name := "before-sync"
		if afterSync {
			name = "after-sync"
		}
		t.Run(name, func(t *testing.T) {
			h, _, _ := recoveryTestFull(t, 4)
			h.now = h.source.Job.ExpiresAtMS + 1
			h.node.canonicalErr = nil
			write := h.pool.writeBatch
			fired := false
			h.pool.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
				d := new(submissionBatchDetector)
				if err := batch.Replay(d); err != nil {
					return err
				}
				if d.found && !fired {
					fired = true
					if !d.accepted || options == nil || !options.Sync {
						t.Error("canonical receipt not atomically accepted/synchronized")
					}
					if afterSync {
						if err := write(batch, options); err != nil {
							return err
						}
					}
					return errors.New("synthetic canonical commit uncertainty")
				}
				return write(batch, options)
			}
			if r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); !errors.Is(err, ErrPoisoned) || r != (Submission{}) || !fired {
				t.Fatal("unknown write disclosed success", r, err)
			}
			if h.node.broadcasts != 0 {
				t.Fatal("unknown write broadcast block")
			}
			h.reopen(t)
			recoveryTestAttach(t, h, 2, recoveryTestBudget())
			before, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
			if afterSync {
				if err != nil || before.Sequence != 3 || before.AcceptedSequence != 3 {
					t.Fatal("synced receipt lost", before, err)
				}
			} else if err != ErrNotFound {
				t.Fatal("failed write created receipt", before, err)
			}
			got, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
			if err != nil || got.Sequence != 3 || got.AcceptedSequence != 3 || h.pool.meta.Submissions != 3 || h.pool.meta.Accepted != 3 || h.node.broadcasts != 0 || (afterSync && got != before) {
				t.Fatal("recovery duplicated original row", got, err)
			}
		})
	}
}

func TestOperatorCanonicalRecoveryPhysicalAndFinancialRefusal(t *testing.T) {
	t.Run("physical-full", func(t *testing.T) {
		h, _, _ := recoveryTestFull(t, 2)
		h.node.canonicalErr = nil
		h.now = h.source.Job.ExpiresAtMS + 1
		if r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); err != ErrCapacity || r != (Submission{}) {
			t.Fatal("physical bound bypassed", r, err)
		}
		if h.pool.meta.Submissions != 2 || h.node.canonicalChecks != 0 || h.node.broadcasts != 0 {
			t.Fatal("physical bound changed evidence")
		}
	})
	t.Run("unchanged-one-batch-budget", func(t *testing.T) {
		h := setupPool(t, "", 4)
		budget := recoveryTestBudget()
		budget.Batches = 1
		a := recoveryTestAttach(t, h, 1, budget)
		if err := a.Enable(context.Background()); err != nil {
			t.Fatal(err)
		}
		// Existing pending block row already consumes the only root budget.
		h.now = h.source.Job.ExpiresAtMS
		if r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); err != nil || !r.Block || r.AcceptedSequence != 0 {
			t.Fatal("pending retained block", r, err)
		}
		var count uint64
		err := a.with(context.Background(), func() error {
			_, n, e := a.recoveryBound(context.Background(), h.source.Job.GymID)
			count = n
			return e
		})
		if err != ErrCapacity || count != 1 {
			t.Fatal("second potential block enlarged batch budget", count, err)
		}
		// The refusal counts a possible second alias row separately; it does
		// not claim a fabricated nonce/proof is a real canonical alias.
		h.node.canonicalErr = nil
		r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
		if err != nil || r.Sequence != 1 || r.AcceptedSequence != 1 {
			t.Fatal("budget blocked exact saved recovery", r, err)
		}
	})
}

func TestOperatorCanonicalRecoveryAliasesRetainDistinctRows(t *testing.T) {
	h, _, _ := recoveryTestFull(t, 4)
	ctx := context.Background()
	h.now = h.source.Job.ExpiresAtMS + 1
	h.node.canonicalErr = nil
	first, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	_, prepared, err := h.pool.loadWork(h.source.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Find at most one additional normal-PoW alias in a fixed32-nonce window.
	// This establishes a structural same-coinbase/different-header relation;
	// it creates no expected hash/signature vector from the product.
	var alias prospectwork.ShareCheck
	for nonce := uint32(2); nonce < 34; nonce++ {
		checked, e := prepared.CheckNonce(ctx, nonce)
		if e != nil {
			t.Fatal(e)
		}
		if checked.Block {
			alias = checked
			break
		}
	}
	if !alias.Block || alias.Candidate == nil || alias.Candidate.Proof.Coinbase != h.winner.Candidate.Proof.Coinbase || alias.Header == first.Header || alias.DuplicateKey == first.DuplicateKey {
		t.Fatal("bounded search did not produce a distinct same-funding alias")
	}
	// The concrete node adapter now observes the alternate header canonical.
	// The prior accepted round identity must remain fixed, never overwritten.
	want := testCase{Candidate: alias.Candidate, WorkHash: alias.WorkHash}
	h.node.want = &want
	h.now++
	second, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, alias.Nonce)
	if err != nil || second.Sequence != 4 || second.AcceptedSequence != 4 || second.DuplicateKey == first.DuplicateKey || second.JobID != first.JobID {
		t.Fatal("alias overwritten or lost", second, err)
	}
	h.reopen(t)
	a := recoveryTestAttach(t, h, 2, recoveryTestBudget())
	if status, e := a.Status(ctx); e != nil || status.RecoveryBlocks != 2 || status.Submissions != 4 || status.OverBound {
		t.Fatal("alias rows collapsed after restart", status, e)
	}
	for _, original := range []Submission{first, second} {
		got, e := h.pool.Lookup(ctx, poolTestToken, h.source.Job.ID, original.Nonce)
		if e != nil || got != original {
			t.Fatal("original alias evidence changed", got, e)
		}
	}
	if h.node.broadcasts != 0 || h.node.canonicalChecks != 2 {
		t.Fatal("canonical alias recovery retransmitted or reaccepted")
	}
}
