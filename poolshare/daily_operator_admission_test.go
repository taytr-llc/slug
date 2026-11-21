// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/poolcapacity"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type dailyAdmissionReference struct {
	Version   int `json:"version"`
	Envelopes []struct {
		Name           string                   `json:"name"`
		Input          poolcapacity.DailyInputs `json:"input"`
		Maximum        uint64                   `json:"maximum"`
		Classification string                   `json:"classification"`
	} `json:"envelopes"`
}

func dailyAdmissionInput(t *testing.T, name string) (poolcapacity.DailyInputs, uint64) {
	t.Helper()
	raw, err := os.ReadFile("../internal/poolcapacity/testdata/daily-reservation/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "2db086115f51b0c7e22d2fd7688ad5b46c56e238fa1346d709c9a19bebb7aa78" {
		t.Fatal("independent capacity reference changed")
	}
	var f dailyAdmissionReference
	if json.Unmarshal(raw, &f) != nil || f.Version != 1 || len(f.Envelopes) != 6 {
		t.Fatal("reference inventory")
	}
	for _, row := range f.Envelopes {
		if row.Name == name && row.Classification == "accept" {
			return row.Input, row.Maximum
		}
	}
	t.Fatal("missing frozen envelope", name)
	return poolcapacity.DailyInputs{}, 0
}

// Original normal-PoW fixture issuer/work. The node and session are controlled
// adapters; these tests do not claim real-node funding, wall expiry or payouts.
// Budget identities are the existing source gym and published scalar3's gym;
// published scalar2 is deliberately an authenticated but unconfigured account.
func dailyAdmissionFixture(t *testing.T, name string) (*poolHarness, DailyRecoveryBudget, uint64) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-PoW original owner; cheap refusal is separate")
	}
	in, maximum := dailyAdmissionInput(t, name)
	f := poolFixture(t)
	source := f.source(t, "coinbase-only")
	authority, _, pin := dailyFixtureAuthority(t)
	c := source.Job.Context
	h := &poolHarness{path: filepath.Join(t.TempDir(), "pool"), authority: authority, source: source,
		ordinary: f.caseByName(t, "coinbase-only-nonce-0"), winner: f.caseByName(t, "coinbase-only-nonce-4294967295"), now: source.Job.IssuedAtMS + 1000,
		sessions: &testPoolSessions{key: source.Job.PublicKey}, node: &testPoolNode{template: source.Template, canonicalErr: errors.New("not canonical")},
		config: Config{Version: Version2, AuthorityPin: pin, Policy: prospectnode.Policy{Version: 1, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}, PayoutAddress: source.PayoutAddress, PoolTarget: strings.Repeat("f", 64), MaxWorks: 8, MaxSubmissions: in.SourceMax, MaxBytes: in.SourceBytes}}
	h.node.want = &h.winner
	var err error
	h.pool, err = CreateDaily(context.Background(), h.path, h.config, h.authority, h.node, h.sessions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.pool.Close(); err != nil {
			t.Error("original pool cleanup", err)
		}
		if err := h.authority.Close(); err != nil {
			t.Error("original issuer cleanup", err)
		}
	})
	h.pool.now = func() time.Time { return time.UnixMilli(h.now) }
	w := savedWork{Job: source.Job, Template: source.Template}
	if _, err = h.pool.prepareContext(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if err = h.pool.saveWork(w, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	third, err := gymauth.GymID(c, "02f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f9")
	if err != nil || third == source.Job.GymID || in.Gyms != 2 {
		t.Fatal("public fixture membership", err)
	}
	gyms := []string{source.Job.GymID, third}
	sort.Strings(gyms)
	return h, DailyRecoveryBudget{Rounds: in.Rounds, RoundBytes: in.RoundBytes, CreditRounds: in.CreditRounds, Batches: in.Batches, GymIDs: gyms}, maximum
}

func dailyAdmissionAttach(t *testing.T, h *poolHarness, b DailyRecoveryBudget, maximum uint64) *OperatorAdmission {
	t.Helper()
	a, err := h.pool.AttachDailyOperatorAdmission(context.Background(), h.config, b)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := a.Status(context.Background()); err != nil || s.Maximum != maximum || !s.CanonicalRecovery || s.OverBound {
		t.Fatal("derived original envelope", s, err)
	}
	return a
}

func dailyAdmissionReopen(t *testing.T, h *poolHarness) {
	t.Helper()
	if err := h.pool.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	h.pool, err = OpenDaily(context.Background(), h.path, h.config, h.authority, h.node, h.sessions)
	if err != nil {
		t.Fatal(err)
	}
	h.pool.now = func() time.Time { return time.UnixMilli(h.now) }
}

func dailyAdmissionFull(t *testing.T) (*poolHarness, *OperatorAdmission, DailyRecoveryBudget, []Submission) {
	t.Helper()
	h, b, maximum := dailyAdmissionFixture(t, "kernel-actual-derived-limit")
	if maximum != 2 {
		t.Fatal("frozen derived full must be two")
	}
	a := dailyAdmissionAttach(t, h, b, maximum)
	if err := a.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	var rows []Submission
	for _, tc := range []testCase{h.ordinary, poolFixture(t).caseByName(t, "coinbase-only-nonce-1")} {
		if tc.Block {
			t.Fatal("ordinary fixture must not be a winner")
		}
		r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, tc.Nonce)
		if err != nil || r.AcceptedSequence != uint64(len(rows)+1) {
			t.Fatal(r, err)
		}
		assertFrozenSubmission(t, r, tc, h.source, h.config.PoolTarget)
		rows = append(rows, r)
	}
	return h, a, b, rows
}

func TestDailyOperatorDerivedFullUnseenCanonicalReopen(t *testing.T) {
	h, a, b, ordinary := dailyAdmissionFull(t)
	ctx := context.Background()
	if s, err := a.Status(ctx); err != nil || s.Mode != "draining" || s.Submissions != 2 || s.RecoveryBlocks != 0 || s.OverBound {
		t.Fatal(s, err)
	}
	before := h.pool.meta
	issuer, err := h.authority.Info()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.pool.Issue(ctx, poolTestToken); err != ErrCapacity || h.pool.meta != before {
		t.Fatal("ordinary issuance after derived full", err)
	}
	// The frozen third ordinary nonce is tested structurally below; a winning
	// nonce cannot be admitted until the original node proves it canonical.
	h.now = h.source.Job.ExpiresAtMS + 5
	if r, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce); err != ErrNode || r != (Submission{}) {
		t.Fatal("noncanonical winner", r, err)
	}
	if _, err = h.pool.loadSubmission(h.source.Job.ID, h.winner.Nonce); err != ErrNotFound {
		t.Fatal("unseen nonce was retained before canonicality", err)
	}
	if h.pool.meta.Submissions != 2 || h.pool.meta.Accepted != 2 || h.node.broadcasts != 0 {
		t.Fatal("precanonical effects")
	}
	h.node.canonicalErr = nil
	got, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	assertFrozenSubmission(t, got, h.winner, h.source, h.config.PoolTarget)
	if got.Sequence != 3 || got.AcceptedSequence != 3 || got.Acceptance != "canonical-late-winner" || got.BlockState != "canonical-observed" || got.ReceivedAtMS != h.now || got.AcceptedAtMS != h.now || got.BlockObservedAtMS != h.now {
		t.Fatal("canonical receipt fields", got)
	}
	if after, err := h.authority.Info(); err != nil || !reflect.DeepEqual(after, issuer) {
		t.Fatal("receipt recovery changed issuer/birth", err)
	}
	if h.node.broadcasts != 0 {
		t.Fatal("unseen recovery broadcast")
	}
	dailyAdmissionReopen(t, h)
	if _, err = a.Status(ctx); err != ErrClosed {
		t.Fatal("old handle revived", err)
	}
	a = dailyAdmissionAttach(t, h, b, 2)
	if s, err := a.Status(ctx); err != nil || s.Submissions != 3 || s.Accepted != 3 || s.RecoveryBlocks != 1 || s.Mode != "draining" || s.OverBound {
		t.Fatal(s, err)
	}
	checks := h.node.canonicalChecks
	h.now++
	for i := 0; i < 2; i++ {
		r, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce)
		if err != nil || r != got || h.pool.meta.Submissions != 3 || h.pool.meta.Accepted != 3 || h.node.canonicalChecks != checks || h.node.broadcasts != 0 {
			t.Fatal("exact original retry", r, err)
		}
	}
	for _, r := range ordinary {
		if saved, err := h.pool.Lookup(ctx, poolTestToken, r.JobID, r.Nonce); err != nil || saved != r {
			t.Fatal("old ordinary receipt changed", err)
		}
	}
}

func TestDailyOperatorSelectionMembershipAndOwnedBudget(t *testing.T) {
	h, b, maximum := dailyAdmissionFixture(t, "kernel-actual-derived-limit")
	ctx := context.Background()
	before := dailyRows(t, h.pool)
	wrong := h.config
	wrong.Version = Version1
	if a, err := h.pool.AttachDailyOperatorAdmission(ctx, wrong, b); err != ErrConfig || a != nil {
		t.Fatal("wrong selection", err)
	}
	for _, gyms := range [][]string{nil, {""}, {strings.Repeat("0", 64)}, {b.GymIDs[0], b.GymIDs[0]}, {b.GymIDs[1], b.GymIDs[0]}, {strings.ToUpper(b.GymIDs[0])}} {
		bad := b
		bad.GymIDs = gyms
		if a, err := h.pool.AttachDailyOperatorAdmission(ctx, h.config, bad); err != ErrConfig || a != nil {
			t.Fatal("malformed membership", err)
		}
	}
	bad := b
	bad.Batches = 0
	if a, err := h.pool.AttachDailyOperatorAdmission(ctx, h.config, bad); err != ErrConfig || a != nil {
		t.Fatal("bad budget", err)
	}
	if dailyRows(t, h.pool) != before || h.pool.operatorAdmission != nil {
		t.Fatal("refused selection had effects")
	}
	a := dailyAdmissionAttach(t, h, b, maximum)
	b.GymIDs[0] = strings.Repeat("0", 64)
	if _, err := h.pool.AttachDailyOperatorAdmission(ctx, h.config, b); err != ErrConfig {
		t.Fatal("second attachment", err)
	}
	if err := a.EnableCanonicalRecovery(ctx, RecoveryBudget{}); err != ErrConfig {
		t.Fatal("legacy budget injected", err)
	}
	copyHandle := *a
	if _, err := copyHandle.Status(ctx); err != ErrInvalid {
		t.Fatal("copied handle acquired authority", err)
	}
	if _, err := h.pool.Issue(ctx, poolTestToken); err != ErrBusy {
		t.Fatal("attachment was not paused", err)
	}
	if err := a.Enable(ctx); err != nil {
		t.Fatal("caller mutated owned budget", err)
	}
	h.pool.sessions = newFenceSessions(h.source.Job.PublicKey)
	for _, full := range []bool{false, true} {
		if full {
			for _, n := range []uint32{0, 1} {
				if _, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, n); err != nil {
					t.Fatal(err)
				}
			}
		}
		prior, err := a.Status(ctx)
		if err != nil {
			t.Fatal(err)
		}
		rows, meta := dailyRows(t, h.pool), h.pool.meta
		issuer, err := h.authority.Info()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.pool.Issue(ctx, "other-gym"); err != ErrUnauthorized {
			t.Fatal("nonmember issue classification", err)
		}
		if _, err = h.pool.Submit(ctx, "other-gym", h.source.Job.ID, h.winner.Nonce); err != ErrUnauthorized {
			t.Fatal("original job-key binding", err)
		}
		after, e := a.Status(ctx)
		info, ie := h.authority.Info()
		if e != nil || ie != nil || after != prior || h.pool.meta != meta || dailyRows(t, h.pool) != rows || !reflect.DeepEqual(info, issuer) || h.pool.poisoned {
			t.Fatal("nonmember altered original intake/evidence", after, e, ie)
		}
	}
}

func TestDailyOperatorSourceHoldbackBeforeIssuer(t *testing.T) {
	h, b, maximum := dailyAdmissionFixture(t, "source-byte-derived-limit")
	a := dailyAdmissionAttach(t, h, b, maximum)
	if maximum != 1 {
		t.Fatal("independent byte-derived ceiling")
	}
	if err := a.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.pool.now = time.Now
	refused := false
	for i := 0; i < 7; i++ {
		issuer, err := h.authority.Info()
		if err != nil {
			t.Fatal(err)
		}
		rows, meta, calls := dailyRows(t, h.pool), h.pool.meta, h.node.currents
		_, err = h.pool.Issue(context.Background(), poolTestToken)
		if err == nil {
			continue
		}
		if err != ErrCapacity {
			t.Fatal("unexpected issuance failure", err)
		}
		after, e := h.authority.Info()
		if e != nil || !reflect.DeepEqual(after, issuer) || dailyRows(t, h.pool) != rows || h.pool.meta != meta || h.node.currents != calls {
			t.Fatal("holdback after issuer/node/clock effect", e)
		}
		if meta.Works >= h.config.MaxWorks || issuer.Jobs >= h.config.MaxWorks || meta.Submissions != 0 || h.config.MaxBytes-meta.Bytes >= maxWorkSize+2*maxSubmissionSize {
			t.Fatal("not a genuine byte holdback", meta, issuer.Jobs)
		}
		refused = true
		break
	}
	if !refused {
		t.Fatal("valid original work did not reach selected holdback")
	}
	if s, err := a.Status(context.Background()); err != nil || s.Mode != "draining" || s.OverBound {
		t.Fatal(s, err)
	}
	// The reserved slot remains usable; exact late proof does not issue or
	// broadcast anything even though ordinary work bytes closed issuance.
	h.node.canonicalErr = nil
	r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || r.Sequence != 1 || r.AcceptedSequence != 1 || h.node.broadcasts != 0 {
		t.Fatal("reserved receipt lost", r, err)
	}
}

func TestDailyOperatorAuditPrecedenceAndOriginalMetadata(t *testing.T) {
	for _, name := range []string{"meta", "nonce", "sequence", "accepted", "work", "gym", "unknown-tail", "missing-row"} {
		t.Run(name, func(t *testing.T) {
			h, b, _ := dailyAdmissionFixture(t, "kernel-actual-derived-limit")
			r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
			if err != nil {
				t.Fatal(err)
			}
			original := h.pool.meta
			put := func(k, v []byte) {
				t.Helper()
				if err := h.pool.db.Put(k, v, &opt.WriteOptions{Sync: true}); err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "meta":
				changed := original
				changed.Bytes++
				raw, _ := json.Marshal(changed)
				put([]byte("meta"), raw)
			case "nonce":
				put(nonceKey(r.JobID, r.Nonce), []byte(strings.Repeat("f", 64)))
			case "sequence":
				put(seqKey('q', 1), []byte(strings.Repeat("f", 64)))
			case "accepted":
				put(seqKey('a', 1), []byte(strings.Repeat("f", 64)))
			case "work":
				w := savedWork{Job: h.source.Job, Template: h.source.Template}
				w.Job.AuthorityID = strings.Repeat("f", 64)
				raw, _ := json.Marshal(w)
				put(workKey(w.Job.ID), raw)
			case "gym":
				r.GymID = strings.Repeat("A", 64)
				raw, _ := json.Marshal(r)
				put(recordKey(r.DuplicateKey), raw)
			case "unknown-tail":
				put([]byte("z/unknown"), []byte("not authority"))
			case "missing-row":
				if err := h.pool.db.Delete(recordKey(r.DuplicateKey), &opt.WriteOptions{Sync: true}); err != nil {
					t.Fatal(err)
				}
			}
			// Also select a valid retained nonmember: later corruption wins over
			// membership and the now-zero ordinary ceiling's capacity meaning.
			b.GymIDs = []string{strings.Repeat("e", 64), strings.Repeat("f", 64)}
			b.Batches = 1
			if a, err := h.pool.AttachDailyOperatorAdmission(context.Background(), h.config, b); err != ErrCorrupt || a != nil {
				t.Fatal("bad source masked by capacity/membership", a, err)
			}
			if h.pool.meta != original || h.pool.operatorAdmission != nil {
				t.Fatal("candidate metadata adopted on audit failure")
			}
		})
	}
	t.Run("valid-nonmember", func(t *testing.T) {
		h, b, _ := dailyAdmissionFixture(t, "kernel-actual-derived-limit")
		b.GymIDs = []string{strings.Repeat("e", 64), strings.Repeat("f", 64)}
		rows := dailyRows(t, h.pool)
		if a, err := h.pool.AttachDailyOperatorAdmission(context.Background(), h.config, b); err != ErrConfig || a != nil {
			t.Fatal("valid retained nonmember", a, err)
		}
		if dailyRows(t, h.pool) != rows {
			t.Fatal("nonmember audit mutated evidence")
		}
	})
	t.Run("late-corruption-before-physical-capacity", func(t *testing.T) {
		h, _, _, _ := dailyAdmissionFull(t)
		if err := h.pool.db.Put([]byte("z/late"), []byte("corrupt"), &opt.WriteOptions{Sync: true}); err != nil {
			t.Fatal(err)
		}
		h.node.canonicalErr = nil
		if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); err != ErrCorrupt {
			t.Fatal("late corrupt row hidden by drain", err)
		}
		if h.node.canonicalChecks != 0 || h.node.broadcasts != 0 || h.pool.meta.Submissions != 2 {
			t.Fatal("corrupt census reached node/append")
		}
	})
}

func TestDailyOperatorUncertainCommitAndExactSavedRecovery(t *testing.T) {
	for _, afterSync := range []bool{false, true} {
		name := "before-sync"
		if afterSync {
			name = "after-sync"
		}
		t.Run(name, func(t *testing.T) {
			h, _, b, _ := dailyAdmissionFull(t)
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
						t.Error("receipt and acceptance not one synchronized batch")
					}
					if afterSync {
						if err := write(batch, options); err != nil {
							return err
						}
					}
					return errors.New("synthetic sync uncertainty")
				}
				return write(batch, options)
			}
			if r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); err != ErrPoisoned || r != (Submission{}) || !fired {
				t.Fatal("unknown write disclosed success", r, err)
			}
			dailyAdmissionReopen(t, h)
			dailyAdmissionAttach(t, h, b, 2)
			prior, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
			if afterSync {
				if err != nil || prior.Sequence != 3 || prior.AcceptedSequence != 3 {
					t.Fatal(prior, err)
				}
			} else if err != ErrNotFound {
				t.Fatal(err)
			}
			got, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
			if err != nil || got.Sequence != 3 || got.AcceptedSequence != 3 || h.pool.meta.Submissions != 3 || h.pool.meta.Accepted != 3 || h.node.broadcasts != 0 || (afterSync && got != prior) {
				t.Fatal("reopen duplicated original obligation", got, err)
			}
		})
	}
}

func TestDailyOperatorDrainOnlyPendingAndPostCeilingHold(t *testing.T) {
	t.Run("selected-zero", func(t *testing.T) {
		h, b, max := dailyAdmissionFixture(t, "drain-only")
		a := dailyAdmissionAttach(t, h, b, max)
		if max != 0 || a.Enable(context.Background()) != ErrCapacity {
			t.Fatal("zero ceiling revived")
		}
		h.node.canonicalErr = nil
		h.now = h.source.Job.ExpiresAtMS + 1
		if r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); err != nil || r.Sequence != 1 || r.AcceptedSequence != 1 {
			t.Fatal("zero ordinary envelope lost reserved winner", r, err)
		}
	})
	t.Run("saved-pending", func(t *testing.T) {
		h, b, max := dailyAdmissionFixture(t, "kernel-actual-derived-limit")
		a := dailyAdmissionAttach(t, h, b, max)
		if err := a.Enable(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, 0); err != nil {
			t.Fatal(err)
		}
		h.now = h.source.Job.ExpiresAtMS + 1
		pending, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
		if err != nil || pending.AcceptedSequence != 0 || pending.Sequence != 2 {
			t.Fatal(pending, err)
		}
		dailyAdmissionReopen(t, h)
		a = dailyAdmissionAttach(t, h, b, 2)
		h.node.canonicalErr = nil
		got, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
		if err != nil || got.Sequence != pending.Sequence || got.AcceptedSequence != 2 || h.pool.meta.Submissions != 2 {
			t.Fatal("saved pending displaced", got, err)
		}
		if s, err := a.Status(context.Background()); err != nil || s.RecoveryBlocks != 1 || s.OverBound {
			t.Fatal(s, err)
		}
	})
	t.Run("unqualified-current-parent-row", func(t *testing.T) {
		h, b, _ := dailyAdmissionFixture(t, "kernel-actual-derived-limit")
		// Three valid unattached rows: the third is an accepted block with the
		// legacy current-parent classification, not a post-ceiling canonical grant.
		for _, n := range []uint32{0, 1, h.winner.Nonce} {
			if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, n); err != nil {
				t.Fatal(err)
			}
		}
		a, err := h.pool.AttachDailyOperatorAdmission(context.Background(), h.config, b)
		if err != nil {
			t.Fatal(err)
		}
		if s, err := a.Status(context.Background()); err != nil || !s.OverBound || s.Mode != "draining" || s.RecoveryBlocks != 1 {
			t.Fatal(s, err)
		}
		if err := a.Enable(context.Background()); err != ErrCapacity {
			t.Fatal("unqualified history enlarged reservation", err)
		}
		// Existing identity remains recoverable; no new grant is inferred.
		if _, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, 0); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDailyOperatorOriginalCancellationCloseAndErrorClass(t *testing.T) {
	h, b, max := dailyAdmissionFixture(t, "kernel-actual-derived-limit")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := dailyRows(t, h.pool)
	if a, err := h.pool.AttachDailyOperatorAdmission(ctx, h.config, b); !errors.Is(err, context.Canceled) || a != nil {
		t.Fatal(a, err)
	}
	if dailyRows(t, h.pool) != before || h.pool.operatorAdmission != nil {
		t.Fatal("canceled selection effects")
	}
	a := dailyAdmissionAttach(t, h, b, max)
	if err := a.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	mixed := errors.Join(registry.ErrPublicCapacity, registry.ErrCorrupt)
	if err := h.pool.withSession(context.Background(), poolTestToken, func(context.Context, string, *poolSession) error { return mixed }); err != mixed {
		t.Fatal(err)
	}
	if s, err := a.Status(context.Background()); err != nil || s.Mode != "enabled" {
		t.Fatal("mixed error became capacity", s, err)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	var submitErr error
	h.node.onCurrent = func() { close(entered); <-release }
	defer func() { once.Do(func() { close(release) }); <-done }()
	go func() {
		defer close(done)
		_, submitErr = h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, 0)
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("original writer not entered")
	}
	bounded, end := context.WithTimeout(context.Background(), 20*time.Millisecond)
	_, err := a.Status(bounded)
	end()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("status did not honor original bounded gate", err)
	}
	closed := make(chan error, 1)
	go func() { closed <- h.pool.Close() }()
	once.Do(func() { close(release) })
	<-done
	if submitErr != nil {
		t.Fatal(submitErr)
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close failed to join original writer")
	}
	if _, err := a.Status(context.Background()); err != ErrClosed {
		t.Fatal("closed original handle revived", err)
	}
}

func TestDailyOperatorCheapRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	if a, err := (*Pool)(nil).AttachDailyOperatorAdmission(nil, Config{}, DailyRecoveryBudget{}); a != nil || err != ErrProfile {
		t.Fatal(a, err)
	}
}

func TestDailyOperatorAliasesCountWholeBudget(t *testing.T) {
	h, a, b, _ := dailyAdmissionFull(t)
	ctx := context.Background()
	h.now = h.source.Job.ExpiresAtMS + 1
	h.node.canonicalErr = nil
	first, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	_, prepared, err := h.pool.loadWorkContext(ctx, h.source.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	rows := []Submission{first}
	found := 0
	for nonce := uint32(2); nonce < 66 && found < 3; nonce++ {
		check, err := prepared.CheckNonce(ctx, nonce)
		if err != nil {
			t.Fatal(err)
		}
		if !check.Block {
			continue
		}
		found++
		if check.Candidate == nil || check.Candidate.Proof.Coinbase != h.winner.Candidate.Proof.Coinbase {
			t.Fatal("same-funding alias relation")
		}
		want := testCase{Candidate: check.Candidate, WorkHash: check.WorkHash}
		h.node.want = &want
		calls := h.node.canonicalChecks
		got, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, nonce)
		if found == 3 {
			if err != ErrCapacity || got != (Submission{}) || h.node.canonicalChecks != calls || h.pool.meta.Submissions != 5 {
				t.Fatal("fourth possible block exceeded original six-batch budget", got, err)
			}
			break
		}
		if err != nil || got.Sequence != uint64(3+found) || got.AcceptedSequence != got.Sequence {
			t.Fatal("alias merged or lost", got, err)
		}
		rows = append(rows, got)
	}
	if found != 3 {
		t.Fatal("bounded normal-PoW alias search did not reach budget control")
	}
	if s, err := a.Status(ctx); err != nil || s.Submissions != 5 || s.RecoveryBlocks != 3 || s.OverBound {
		t.Fatal("whole alias census", s, err)
	}
	dailyAdmissionReopen(t, h)
	dailyAdmissionAttach(t, h, b, 2)
	for _, row := range rows {
		if got, err := h.pool.Lookup(ctx, poolTestToken, row.JobID, row.Nonce); err != nil || got != row {
			t.Fatal("retained alias changed", got, err)
		}
	}
	if h.node.broadcasts != 0 {
		t.Fatal("canonical recovery broadcast alias")
	}
}

func TestDailyOperatorPhysicalFullAuditAndLegacyParity(t *testing.T) {
	t.Run("physical-full", func(t *testing.T) {
		h, b, _ := dailyAdmissionFixture(t, "kernel-actual-derived-limit")
		h.node.want = nil // controlled node accepts each locally checked original block
		for nonce := uint32(0); nonce < 8; nonce++ {
			if r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, nonce); err != nil || r.Sequence != uint64(nonce)+1 {
				t.Fatal("valid physical prefix", r, err)
			}
		}
		a, err := h.pool.AttachDailyOperatorAdmission(context.Background(), h.config, b)
		if err != nil {
			t.Fatal(err)
		}
		if s, err := a.Status(context.Background()); err != nil || s.Submissions != 8 || !s.OverBound {
			t.Fatal(s, err)
		}
		before := h.pool.meta
		if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); err != ErrCapacity {
			t.Fatal(err)
		}
		if h.pool.meta != before {
			t.Fatal("physical full changed metadata")
		}
		if err := h.pool.db.Put([]byte("z/late-corrupt"), []byte("x"), &opt.WriteOptions{Sync: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); err != ErrCorrupt {
			t.Fatal("physical capacity masked corruption", err)
		}
	})
	t.Run("legacy-unchanged", func(t *testing.T) {
		if wire.TestPoW {
			t.Skip("normal legacy fixture")
		}
		h := setupPool(t, "", 8)
		if a, err := h.pool.AttachDailyOperatorAdmission(context.Background(), h.config, DailyRecoveryBudget{}); a != nil || err != ErrConfig {
			t.Fatal("legacy adopted by daily", a, err)
		}
		a := recoveryTestAttach(t, h, 2, recoveryTestBudget())
		if a.daily != nil {
			t.Fatal("legacy mode changed")
		}
		if err := a.Enable(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, 0); err != nil {
			t.Fatal(err)
		}
		if s, err := a.Status(context.Background()); err != nil || s.Maximum != 2 || s.Submissions != 1 || s.Mode != "enabled" {
			t.Fatal(s, err)
		}
	})
}

func TestDailyOperatorFinalSessionClockAndCapacityError(t *testing.T) {
	t.Run("cancel-before-canonical-commit", func(t *testing.T) {
		h, _, _, _ := dailyAdmissionFull(t)
		h.node.canonicalErr = nil
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h.node.onCanonical = cancel
		before := h.pool.meta
		if r, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce); !errors.Is(err, context.Canceled) || r != (Submission{}) {
			t.Fatal("canceled proof reached commit", r, err)
		}
		if h.pool.meta != before || h.node.broadcasts != 0 {
			t.Fatal("canceled original session effects")
		}
	})
	t.Run("final-clock-classification", func(t *testing.T) {
		h, _, b, _ := dailyAdmissionFull(t)
		h.node.canonicalErr = nil
		h.now = h.source.Job.ExpiresAtMS - 1
		h.node.onCanonical = func() { h.now = h.source.Job.ExpiresAtMS + 7 }
		r, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
		if err != nil || r.ReceivedAtMS != h.source.Job.ExpiresAtMS-1 || r.AcceptedAtMS != h.now || r.Acceptance != "canonical-late-winner" {
			t.Fatal(r, err)
		}
		dailyAdmissionReopen(t, h)
		dailyAdmissionAttach(t, h, b, 2)
		if got, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); err != nil || got != r {
			t.Fatal("final original timestamp changed", got, err)
		}
	})
	t.Run("none-fits-after-valid-audit", func(t *testing.T) {
		h, b, _ := dailyAdmissionFixture(t, "kernel-actual-derived-limit")
		if err := h.pool.Close(); err != nil {
			t.Fatal(err)
		}
		config := h.config
		config.MaxBytes = 4096
		p, err := CreateDaily(context.Background(), filepath.Join(t.TempDir(), "empty-small"), config, h.authority, h.node, h.sessions)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := p.Close(); err != nil {
				t.Error("original pool cleanup", err)
			}
		}()
		before := dailyRows(t, p)
		if a, err := p.AttachDailyOperatorAdmission(context.Background(), config, b); a != nil || err != ErrCapacity {
			t.Fatal("insufficient valid reservation", a, err)
		}
		if p.operatorAdmission != nil || dailyRows(t, p) != before {
			t.Fatal("failed envelope published state")
		}
		if err := p.db.Put([]byte("z/corrupt"), []byte("x"), &opt.WriteOptions{Sync: true}); err != nil {
			t.Fatal(err)
		}
		if a, err := p.AttachDailyOperatorAdmission(context.Background(), config, b); a != nil || err != ErrCorrupt {
			t.Fatal("envelope capacity masked malformed empty source", a, err)
		}
	})
}
