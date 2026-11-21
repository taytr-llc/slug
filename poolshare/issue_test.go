//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"bytes"
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
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type issueTestFamily struct {
	Name      string
	Profile   registry.IssueProfile
	ProfileID string `json:"profile_id"`
	RequestID string `json:"request_id"`
	Work      MiningWork
	Setup     struct {
		Authority registry.PublicManifestPin
		Pool      Config
	}
}

func issueSourceReference(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("../testdata/issue-recovery/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if len(raw) != 6134980 || hex.EncodeToString(h[:]) != "4bbe4eb9556ed5a894cee6ff565b0dcca805f27fea2d77cfe3a56242dc3dac91" {
		t.Fatal("independent issue corpus changed")
	}
	var x map[string]json.RawMessage
	if json.Unmarshal(raw, &x) != nil {
		t.Fatal("reference JSON")
	}
	return x
}

type issueTestNode struct {
	testPoolNode
	resolveCalls int
	failResolve  int
	proposalErr  error
	onProposal   func()
}

func (n *issueTestNode) ResolveTemplate(ctx context.Context, p prospectnode.Policy) (prospectwork.Template, error) {
	n.resolveCalls++
	if n.failResolve == n.resolveCalls {
		return prospectwork.Template{}, errors.New("controlled unavailable")
	}
	return n.testPoolNode.ResolveTemplate(ctx, p)
}
func (n *issueTestNode) Proposal(ctx context.Context, c prospect.Context, block string) error {
	n.proposals++
	if n.onProposal != nil {
		n.onProposal()
	}
	return n.proposalErr
}

type issueTestOwner struct {
	p                *Pool
	a                *registry.ManifestStore
	owner            *operatorauthority.Owner
	profile          registry.OperatorProfile
	f                issueTestFamily
	path, issuerPath string
	n                *issueTestNode
	s                *testPoolSessions
}

func newIssueTestOwner(t *testing.T) *issueTestOwner {
	t.Helper()
	x := issueSourceReference(t)
	var f struct {
		Families []issueTestFamily `json:"matching_families"`
	}
	if json.Unmarshal(x["fixtures"], &f) != nil || len(f.Families) == 0 {
		t.Fatal("family")
	}
	v := f.Families[0]
	profile, err := registry.NewOperatorProfile(v.Setup.Authority)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, role := range []string{"admission", "publication", "cutoff"} {
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		paths[role] = filepath.Join(root, role)
		if err = os.WriteFile(paths[role], seed[:], 0600); err != nil {
			t.Fatal(err)
		}
	}
	pin := profile.Pin()
	genesis, err := chainhash.NewHashFromStr(pin.Source.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	binding := operatorauthority.Binding{Version: 2, PinID: profile.ID(), Network: wire.BitcoinNet(pin.Source.Network), Genesis: *genesis, Keys: operatorauthority.Keys{Admission: pin.AdmissionKey, Publication: pin.PublicationKey, Cutoff: pin.CutoffKey, Checkpoint: pin.CheckpointKey}}
	owner, err := operatorauthority.OpenOnline(context.Background(), binding, operatorauthority.OnlineSeedFiles{Admission: paths["admission"], Publication: paths["publication"], Cutoff: paths["cutoff"]})
	if err != nil {
		t.Fatal(err)
	}
	h := &issueTestOwner{owner: owner, profile: profile, f: v, path: filepath.Join(root, "pool"), issuerPath: filepath.Join(root, "issuer"), n: &issueTestNode{testPoolNode: testPoolNode{template: v.Work.Template}}, s: &testPoolSessions{key: v.Work.Job.PublicKey}}
	t.Cleanup(func() {
		if h.p != nil {
			if err := h.p.Close(); err != nil {
				t.Error(err)
			}
		}
		if h.a != nil {
			if err := h.a.Close(); err != nil {
				t.Error(err)
			}
		}
		if err := h.owner.Close(); err != nil {
			t.Error(err)
		}
	})
	d, err := fighterdescriptor.ResolveCandidate(pin.Source.DescriptorID)
	if err != nil {
		t.Fatal(err)
	}
	h.a, err = registry.CreateRecoverableOperatorManifestAuthority(context.Background(), h.issuerPath, d, profile, v.Profile, owner)
	if err != nil {
		t.Fatal(err)
	}
	h.p, err = CreateRecoverableDaily(context.Background(), h.path, v.Setup.Pool, v.Profile, h.a, h.n, h.s)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func (h *issueTestOwner) reopen(t *testing.T) {
	t.Helper()
	if err := h.p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := h.a.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	h.a, err = registry.OpenRecoverableOperatorManifestAuthority(context.Background(), h.issuerPath, h.profile, h.f.Profile, h.owner)
	if err != nil {
		t.Fatal(err)
	}
	h.p, err = OpenRecoverableDaily(context.Background(), h.path, h.f.Setup.Pool, h.f.Profile, h.a, h.n, h.s)
	if err != nil {
		t.Fatal(err)
	}
}
func (h *issueTestOwner) counts(t *testing.T, requests, jobs, works uint64) {
	t.Helper()
	info, n, err := h.a.AuditIssueBinding(context.Background(), h.f.Setup.Pool.AuthorityPin, h.f.Profile)
	if err != nil || info.Jobs != jobs || n != requests || h.p.issueRecovery.requests != requests || h.p.meta.Works != works {
		t.Fatal("exact source/issuer counters", info, n, h.p.meta, err)
	}
}

func TestRecoverableSourceProposalGapExactReopen(t *testing.T) {
	h := newIssueTestOwner(t)
	ctx := context.Background()
	id := h.f.RequestID
	h.n.proposalErr = errors.New("lost proposal response")
	if _, err := h.p.ResumeIssue(ctx, poolTestToken, id); err != ErrOutcomeUnknown {
		t.Fatal("issued gap", err)
	}
	h.counts(t, 1, 1, 0)
	pending, err := h.p.LookupIssue(ctx, poolTestToken, id)
	if err != nil || pending.State != "pending" || pending.Work != nil || pending.JobID != "" {
		t.Fatal(pending, err)
	}
	ir, err := h.a.TryIssueRequest(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	original, err := h.a.TryGetManifestJob(ctx, ir.JobID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := h.p.db.Get(issueKey("issue/t/", id), nil)
	if err != nil {
		t.Fatal(err)
	}
	h.reopen(t)
	h.n.proposalErr = nil
	out, err := h.p.ResumeIssue(ctx, poolTestToken, id)
	if err != nil || out.State != "work" || out.Work == nil || out.Work.Job != original {
		t.Fatal("original recovered", out, err)
	}
	h.counts(t, 1, 1, 1)
	after, _ := json.Marshal(out.Work.Template)
	if !bytes.Equal(before, after) {
		t.Fatal("template changed")
	}
	calls := h.n.proposals
	h.reopen(t)
	repeated, err := h.p.ResumeIssue(ctx, poolTestToken, id)
	if err != nil || !reflect.DeepEqual(repeated, out) || h.n.proposals != calls {
		t.Fatal("completed replay touched node", err)
	}
	if _, err := h.p.Issue(ctx, poolTestToken); err != ErrConfig {
		t.Fatal("old issue selected owner", err)
	}
	if err := h.p.CheckDailyServiceSelection(ctx, nil); err != ErrConfig {
		t.Fatal("old service selection", err)
	}
	if _, err := h.p.DailyBinding(ctx); err != nil {
		t.Fatal("base financial binding", err)
	}
	h.s.key = "02f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f9" // Valid original-session key for another gym must never recover data
	if _, err := h.p.LookupIssue(ctx, poolTestToken, id); err == nil {
		t.Fatal("foreign identity disclosure")
	}
}
func TestRecoverableSourceExpiredUnusedAndSealedAbsence(t *testing.T) {
	t.Run("issued-unused", func(t *testing.T) {
		h := newIssueTestOwner(t)
		h.n.proposalErr = errors.New("proposal unavailable")
		ctx := context.Background()
		id := h.f.RequestID
		if _, err := h.p.ResumeIssue(ctx, poolTestToken, id); err != ErrOutcomeUnknown {
			t.Fatal(err)
		}
		ir, err := h.a.TryIssueRequest(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		job, err := h.a.TryGetManifestJob(ctx, ir.JobID)
		if err != nil {
			t.Fatal(err)
		}
		h.reopen(t)
		h.p.now = func() time.Time { return time.UnixMilli(job.ExpiresAtMS) }
		calls := h.n.proposals
		out, err := h.p.ResumeIssue(ctx, poolTestToken, id)
		if err != nil || out.State != "issued_unused" || out.Work != nil || out.JobID != job.ID || out.ResolvedAtMS < job.ExpiresAtMS || h.n.proposals != calls {
			t.Fatal(out, err)
		}
		h.counts(t, 1, 1, 0)
		h.reopen(t)
		got, err := h.p.ResumeIssue(ctx, poolTestToken, id)
		if err != nil || !reflect.DeepEqual(got, out) {
			t.Fatal("terminal replay", err)
		}
	})
	t.Run("pending-absent-seal", func(t *testing.T) {
		h := newIssueTestOwner(t)
		ctx := context.Background()
		h.n.failResolve = 2
		if _, err := h.p.ResumeIssue(ctx, poolTestToken, h.f.RequestID); err != ErrOutcomeUnknown {
			t.Fatal(err)
		}
		info, requests, err := h.a.AuditIssueBinding(ctx, h.f.Setup.Pool.AuthorityPin, h.f.Profile)
		if err != nil || info.Jobs != 0 || requests != 0 || h.p.issueRecovery.requests != 1 {
			t.Fatal("preissuer gap", info, requests, err)
		}
		h.reopen(t)
		gym := h.f.Work.Job.GymID
		other, err := gymauth.GymID(h.f.Work.Job.Context, "02f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f9")
		if err != nil {
			t.Fatal(err)
		}
		gyms := []string{gym, other}
		sort.Strings(gyms)
		a, err := h.p.AttachDailyOperatorAdmission(ctx, h.f.Setup.Pool, DailyRecoveryBudget{Rounds: 8, RoundBytes: 64 << 20, CreditRounds: 8, Batches: 6, GymIDs: gyms})
		if err != nil {
			t.Fatal(err)
		}
		if err = a.Drain(ctx); err != nil {
			t.Fatal(err)
		}
		out, err := h.p.ResumeIssue(ctx, poolTestToken, h.f.RequestID)
		if err != nil || out.State != "not_issued" || out.Reason != "intake_drained" || out.Work != nil || out.JobID != "" {
			t.Fatal(out, err)
		}
		h.counts(t, 1, 0, 0)
		h.reopen(t)
		again, err := h.p.ResumeIssue(ctx, poolTestToken, h.f.RequestID)
		if err != nil || !reflect.DeepEqual(again, out) {
			t.Fatal("sealed replay", err)
		}
	})
}
func TestRecoverableSourceUncertainCommitAndNamespaceCensus(t *testing.T) {
	h := newIssueTestOwner(t)
	ctx := context.Background()
	original := h.p.writeBatch
	writes := 0
	h.p.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
		writes++
		probe := new(issueTestBatch)
		if err := b.Replay(probe); err != nil {
			return err
		}
		if err := original(b, o); err != nil {
			return err
		}
		if probe.work {
			return errors.New("lost final write acknowledgment")
		}
		return nil
	}
	// Fail only the exact batch containing the final work, irrespective of clock granularity.
	if _, err := h.p.ResumeIssue(ctx, poolTestToken, h.f.RequestID); err != ErrPoisoned {
		t.Fatal("uncertain source write", err, writes)
	}
	h.reopen(t)
	out, err := h.p.ResumeIssue(ctx, poolTestToken, h.f.RequestID)
	if err != nil || out.State != "work" {
		t.Fatal(out, err)
	}
	h.counts(t, 1, 1, 1)
	// Valid but unrelated sealed issuer evidence cannot hide behind equal totals.
	r, err := h.p.loadIssueRow(h.f.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	unrelated := r.Intent
	unrelated.RequestID = strings.Repeat("9", 64)
	if _, _, err = h.a.SealIssueRequest(ctx, unrelated, "parent_changed", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = h.p.DailyBinding(ctx); err != ErrCorrupt {
		t.Fatal("unmatched issuer seal accepted", err)
	}
}
func TestRecoverableSourceCancellationAndReservedBytes(t *testing.T) {
	h := newIssueTestOwner(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.p.ResumeIssue(ctx, poolTestToken, h.f.RequestID); err != context.Canceled {
		t.Fatal(err)
	}
	h.counts(t, 0, 0, 0)
	h.n.failResolve = 2
	if _, err := h.p.ResumeIssue(context.Background(), poolTestToken, h.f.RequestID); err != ErrOutcomeUnknown {
		t.Fatal(err)
	}
	if h.p.issueRecovery.reserved == 0 || h.p.issueRecovery.bytes == 0 || h.p.freeSourceBytes() != h.p.config.MaxBytes-h.p.meta.Bytes-h.p.issueRecovery.bytes-h.p.issueRecovery.reserved {
		t.Fatal("pending reservation missing")
	}
	before := h.p.issueRecovery.reserved
	h.n.failResolve = 0
	h.n.onProposal = func() { h.s.revoked = true }
	if _, err := h.p.ResumeIssue(context.Background(), poolTestToken, h.f.RequestID); err == nil {
		t.Fatal("revoked commit")
	}
	if h.p.meta.Works != 0 || h.p.issueRecovery.reserved != before {
		t.Fatal("revocation released reservation")
	}
	h.s.revoked = false
	h.n.onProposal = nil
	h.reopen(t)
	if _, err := h.p.ResumeIssue(context.Background(), poolTestToken, h.f.RequestID); err != nil {
		t.Fatal(err)
	}
	if h.p.issueRecovery.reserved != 0 {
		t.Fatal("completed reservation not converted")
	}
}

type issueTestBatch struct{ work bool }

func (p *issueTestBatch) Put(k, v []byte) {
	if len(k) == 65 && k[0] == 'w' {
		p.work = true
	}
}
func (p *issueTestBatch) Delete(k []byte) {}
