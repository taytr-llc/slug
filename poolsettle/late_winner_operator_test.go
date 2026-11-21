//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/internal/poolcapacity"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// This fixture uses real normal-PoW checks and the original concrete journals.
// Node canonical/funding observations are controlled leaves, not actual-node
// acceptance. The separate actual runner proves real expiry and chain evidence.
type lateWinnerNode struct {
	template   prospectwork.Template
	winners    map[string]prospectwork.ShareCheck
	canonical  map[string]bool
	tip        uint32
	broadcasts int
}

func (n *lateWinnerNode) ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error) {
	return n.template, nil
}
func (n *lateWinnerNode) Proposal(context.Context, prospect.Context, string) error { return nil }
func (n *lateWinnerNode) RecheckContext(_ context.Context, c prospect.Context) error {
	if c.ParentHash != n.template.Context.ParentHash || c.ParentHeight != n.template.Context.ParentHeight {
		return errors.New("fixture parent changed")
	}
	return nil
}
func (n *lateWinnerNode) SubmitBlock(_ context.Context, _ prospect.Context, raw string) error {
	n.broadcasts++
	for _, w := range n.winners {
		if w.Candidate != nil && w.Candidate.BlockHex == raw {
			return nil
		}
	}
	return errors.New("fixture unexpected block")
}
func (n *lateWinnerNode) CanonicalBlock(_ context.Context, _ prospect.Context, proof prospect.Proof, hash string) error {
	w, ok := n.winners[hash]
	if !ok || !n.canonical[hash] || w.Candidate == nil || !reflect.DeepEqual(w.Candidate.Proof, proof) {
		return errors.New("fixture block is not canonical")
	}
	return nil
}
func (n *lateWinnerNode) ObservePoolBlock(ctx context.Context, c prospect.Context, proof prospect.Proof, hash string) (prospectnode.PoolBlockObservation, error) {
	if e := n.CanonicalBlock(ctx, c, proof, hash); e != nil {
		return prospectnode.PoolBlockObservation{}, e
	}
	height := c.ParentHeight + 1
	tipHash := strings.Repeat("a", 64)
	if n.tip == height {
		// A zero-descendant canonical block is itself the observed tip.
		tipHash = hash
	}
	return prospectnode.PoolBlockObservation{BlockHash: hash, BlockHeight: height, TipHash: tipHash, TipHeight: n.tip, Canonical: true, Descendants: n.tip - height}, nil
}

type lateWinnerSessions map[string]string

func (s lateWinnerSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	key, ok := s[token]
	if !ok {
		return errors.New("fixture session refused")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	return fn(key, ctx.Err)
}

type lateWinnerOwners struct {
	root         string
	profile      registry.OperatorProfile
	issueProfile *registry.IssueProfile
	issueRequest uint64
	binding      operatorauthority.Binding
	seeds        operatorauthority.OnlineSeedFiles
	owner        *operatorauthority.Owner
	issuer       *registry.ManifestStore
	pool         *poolshare.Pool
	rounds       *poolround.Store
	settlement   *Store
	intake       *poolshare.OperatorAdmission
	node         *lateWinnerNode
	sessions     lateWinnerSessions
	poolConfig   poolshare.Config
	roundLimits  poolround.Limits
	settleConfig Config
	backend      *recoveryBackend
	clock        time.Time
}

func (h *lateWinnerOwners) close() error {
	// Dependency order is intentional; keep any failed original owner reachable.
	if h.settlement != nil {
		if e := h.settlement.Close(); e != nil {
			return e
		}
		h.settlement = nil
	}
	if h.rounds != nil {
		if e := h.rounds.Close(); e != nil {
			return e
		}
		h.rounds = nil
	}
	if h.pool != nil {
		if e := h.pool.Close(); e != nil {
			return e
		}
		h.pool = nil
	}
	if h.issuer != nil {
		if e := h.issuer.Close(); e != nil {
			return e
		}
		h.issuer = nil
	}
	if h.owner != nil {
		if e := h.owner.Close(); e != nil {
			return e
		}
		h.owner = nil
	}
	return nil
}
func (h *lateWinnerOwners) attach(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	in := poolcapacity.Inputs{SourceMax: h.poolConfig.MaxSubmissions, Rounds: h.roundLimits.MaxRounds, RoundBytes: h.roundLimits.MaxBytes, CreditRounds: uint64(h.settleConfig.MaxRounds), Batches: uint64(h.settleConfig.MaxBatches), Gyms: uint64(len(h.settleConfig.Destinations))}
	reservation, e := poolcapacity.Envelope(in)
	if e != nil || reservation.Submissions != 2 {
		t.Fatal("actual ordinary envelope", reservation, e)
	}
	h.intake, e = h.pool.AttachOperatorAdmission(ctx, h.poolConfig, reservation.Submissions)
	if e != nil {
		t.Fatal(e)
	}
	e = h.intake.EnableCanonicalRecovery(ctx, poolshare.RecoveryBudget{Rounds: in.Rounds, RoundBytes: in.RoundBytes, CreditRounds: in.CreditRounds, Batches: in.Batches, Gyms: in.Gyms})
	if e != nil {
		t.Fatal(e)
	}
	s, e := h.intake.Status(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if s.Mode != "draining" {
		if e = h.intake.Enable(ctx); e != nil {
			t.Fatal(e)
		}
	}
}
func (h *lateWinnerOwners) reopen(t *testing.T) {
	t.Helper()
	old := h.intake
	if e := h.close(); e != nil {
		t.Fatal(e)
	}
	if _, e := old.Status(context.Background()); e != poolshare.ErrClosed {
		t.Fatal("old admission owner survived", e)
	}
	var e error
	h.owner, e = operatorauthority.OpenOnline(context.Background(), h.binding, h.seeds)
	if e != nil {
		t.Fatal(e)
	}
	h.issuer, e = registry.OpenOperatorManifestAuthority(filepath.Join(h.root, "issuer"), h.profile, h.owner)
	if e != nil {
		t.Fatal(e)
	}
	h.pool, e = poolshare.Open(filepath.Join(h.root, "pool"), h.poolConfig, h.issuer, h.node, h.sessions)
	if e != nil {
		t.Fatal(e)
	}
	h.rounds, e = poolround.Open(filepath.Join(h.root, "rounds"), h.roundLimits, h.pool, h.node)
	if e != nil {
		t.Fatal(e)
	}
	h.settlement, e = Open(filepath.Join(h.root, "settlement"), h.settleConfig, h.rounds, h.backend)
	if e != nil {
		t.Fatal(e)
	}
	h.settlement.now = func() time.Time { return h.clock }
	h.attach(t)
}

func lateWinnerFixture(t *testing.T) *lateWinnerOwners {
	t.Helper()
	h := &lateWinnerOwners{clock: time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)}
	t.Cleanup(func() {
		if e := h.close(); e != nil {
			t.Error(e)
		}
	})
	var e error
	h.root, e = filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(h.root, 0700); e != nil {
		t.Fatal(e)
	}
	read := func(path, hash string) []byte {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		got := sha256.Sum256(b)
		if hex.EncodeToString(got[:]) != hash {
			t.Fatal("independent fixture changed", path)
		}
		return b
	}
	var authority struct {
		Positives []struct {
			Family, Name string
			Value        json.RawMessage
		}
	}
	if e = json.Unmarshal(read("../registry/testdata/operator-reference/authority-v2.json", "7328ea2102c9ac993281bd732590c3e734eda26a3d180c2fa98bbd37da02c6b4"), &authority); e != nil {
		t.Fatal(e)
	}
	var pin registry.PublicManifestPin
	for _, c := range authority.Positives {
		if c.Family == "pin" && c.Name == "v1/operator_v2_distinct" {
			if e = json.Unmarshal(c.Value, &pin); e != nil {
				t.Fatal(e)
			}
		}
	}
	h.profile, e = registry.NewOperatorProfile(pin)
	if e != nil {
		t.Fatal(e)
	}
	genesis, e := chainhash.NewHashFromStr(pin.Source.Genesis)
	if e != nil {
		t.Fatal(e)
	}
	h.binding = operatorauthority.Binding{Version: 2, PinID: h.profile.ID(), Network: wire.BitcoinNet(pin.Source.Network), Genesis: *genesis, Keys: operatorauthority.Keys{Admission: pin.AdmissionKey, Publication: pin.PublicationKey, Cutoff: pin.CutoffKey, Checkpoint: pin.CheckpointKey}}
	paths := map[string]string{}
	for _, role := range []string{"admission", "publication", "cutoff"} {
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		paths[role] = filepath.Join(h.root, role)
		if e = os.WriteFile(paths[role], seed[:], 0600); e != nil {
			t.Fatal(e)
		}
	}
	h.seeds = operatorauthority.OnlineSeedFiles{Admission: paths["admission"], Publication: paths["publication"], Cutoff: paths["cutoff"]}
	h.owner, e = operatorauthority.OpenOnline(context.Background(), h.binding, h.seeds)
	if e != nil {
		t.Fatal(e)
	}
	d, e := fighterdescriptor.ResolveCandidate(pin.Source.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	h.issuer, e = registry.CreateOperatorManifestAuthority(filepath.Join(h.root, "issuer"), d, h.profile, h.owner)
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Sources []struct {
			Name     string
			Template prospectwork.Template
		}
	}
	if e = json.Unmarshal(read("../prospectwork/testdata/pool_share_reference/pool_share_v1.json", "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87"), &corpus); e != nil {
		t.Fatal(e)
	}
	h.node = &lateWinnerNode{winners: map[string]prospectwork.ShareCheck{}, canonical: map[string]bool{}}
	for _, s := range corpus.Sources {
		if s.Name == "coinbase-only" {
			h.node.template = s.Template
		}
	}
	c := h.node.template.Context
	if c.Version == 0 {
		t.Fatal("template absent")
	}
	ka, _, _ := settlementFixtureKey(1)
	defer ka.Zero()
	kb, _, _ := settlementFixtureKey(2)
	defer kb.Zero()
	h.sessions = lateWinnerSessions{"gym-a": hex.EncodeToString(ka.PubKey().SerializeCompressed()), "gym-b": hex.EncodeToString(kb.PubKey().SerializeCompressed())}
	_, poolScript, payout := settlementFixtureKey(3)
	h.poolConfig = poolshare.Config{Version: 1, AuthorityPin: pin.Source, Policy: prospectnode.Policy{Version: c.Version, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}, PayoutAddress: payout, PoolTarget: strings.Repeat("f", 64), MaxWorks: 4, MaxSubmissions: 4, MaxBytes: 8 << 20}
	h.roundLimits = poolround.Limits{Version: 1, MaxRounds: 4, MaxBytes: 8 << 20}
	ga, e := gymauth.GymID(c, h.sessions["gym-a"])
	if e != nil {
		t.Fatal(e)
	}
	gb, e := gymauth.GymID(c, h.sessions["gym-b"])
	if e != nil {
		t.Fatal(e)
	}
	_, payA, _ := settlementFixtureKey(4)
	_, payB, _ := settlementFixtureKey(5)
	h.settleConfig = Config{Version: 1, ChangeScript: poolScript, MaxRounds: 4, MaxBatches: 4, Destinations: []Destination{{GymID: ga, Script: payA}, {GymID: gb, Script: payB}}}
	if h.settleConfig.Destinations[0].GymID > h.settleConfig.Destinations[1].GymID {
		h.settleConfig.Destinations[0], h.settleConfig.Destinations[1] = h.settleConfig.Destinations[1], h.settleConfig.Destinations[0]
	}
	h.pool, e = poolshare.Create(filepath.Join(h.root, "pool"), h.poolConfig, h.issuer, h.node, h.sessions)
	if e != nil {
		t.Fatal(e)
	}
	h.rounds, e = poolround.Create(filepath.Join(h.root, "rounds"), h.roundLimits, h.pool, h.node)
	if e != nil {
		t.Fatal(e)
	}
	h.backend = &recoveryBackend{Backend: &settlementBackend{funding: "ready", fee: 1000, txs: map[string][]byte{}}}
	h.settlement, e = Create(filepath.Join(h.root, "settlement"), h.settleConfig, h.rounds, h.backend)
	if e != nil {
		t.Fatal(e)
	}
	h.settlement.now = func() time.Time { return h.clock }
	// These exact two original gym/payee selections remain unchanged thereafter.
	h.attach(t)
	return h
}

func lateWinnerSearch(t *testing.T, h *lateWinnerOwners, work poolshare.Work, ordinary bool) (prospectwork.ShareCheck, prospectwork.ShareCheck) {
	t.Helper()
	prepared, e := prospectwork.PrepareManifestWork(work.Job, h.node.template, h.poolConfig.PayoutAddress, 0, h.poolConfig.PoolTarget)
	if e != nil {
		t.Fatal(e)
	}
	var winner, share prospectwork.ShareCheck
	for nonce := uint32(0); nonce < 64; nonce++ {
		c, e := prepared.CheckNonce(context.Background(), nonce)
		if e != nil {
			t.Fatal(e)
		}
		if c.Block && winner.Candidate == nil {
			winner = c
		}
		if c.Pool && !c.Block && share.WorkHash == "" {
			share = c
		}
		if winner.Candidate != nil && (!ordinary || share.WorkHash != "") {
			break
		}
	}
	if winner.Candidate == nil || ordinary && share.WorkHash == "" {
		t.Fatal("bounded normal-PoW search exhausted")
	}
	h.node.winners[winner.WorkHash] = winner
	return winner, share
}

func TestOperatorV2UnseenCanonicalRecoveryPreservesPendingPayoutAcrossOwners(t *testing.T) {
	h := lateWinnerFixture(t)
	ctx := context.Background()
	a, e := h.pool.Issue(ctx, "gym-a")
	if e != nil {
		t.Fatal(e)
	}
	wa, _ := lateWinnerSearch(t, h, a, false)
	h.node.canonical[wa.WorkHash] = true
	h.node.tip = a.Job.Context.ParentHeight + 61
	ra, e := h.pool.Submit(ctx, "gym-a", a.Job.ID, wa.Nonce)
	if e != nil || ra.AcceptedSequence != 1 {
		t.Fatal(ra, e)
	}
	if _, e = h.rounds.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	if r, e := h.rounds.Refresh(ctx, wa.WorkHash); e != nil || r.Confirmation == nil {
		t.Fatal(r, e)
	}
	base := h.backend.Backend.(*settlementBackend)
	base.loseAck = true
	if r, e := h.settlement.RunDaily(ctx); e != nil || r.Reason != "broadcast-unknown" || len(h.settlement.state.Batches) != 1 {
		t.Fatal(r, e)
	}
	original := clone(h.settlement.state.Batches[0])
	if original.FeeAtoms != 1000 || original.Change == nil || original.Change.Atoms != 9999000 || len(original.Payments) != 1 || original.Payments[0].Atoms != 990000000 || original.Payments[0].GymID != a.Job.GymID {
		t.Fatal("independent first-payout arithmetic", original)
	}
	assertOriginal := func() {
		t.Helper()
		if len(h.settlement.state.Batches) != 1 {
			t.Fatal("new signed batch")
		}
		got := clone(h.settlement.state.Batches[0])
		got.State = original.State
		if !reflect.DeepEqual(got, original) || base.signCalls != 1 || h.backend.quotes != 1 {
			t.Fatal("reserved payment identity changed")
		}
	}
	h.node.template.Context.ParentHash = strings.Repeat("a", 64)
	h.node.template.Context.ParentHeight = h.node.tip
	h.node.template.Block.PreviousHash = h.node.template.Context.ParentHash
	h.node.template.Block.Height = int64(h.node.tip) + 1
	b, e := h.pool.Issue(ctx, "gym-b")
	if e != nil {
		t.Fatal(e)
	}
	wb, ordinary := lateWinnerSearch(t, h, b, true)
	if r, e := h.pool.Submit(ctx, "gym-b", b.Job.ID, ordinary.Nonce); e != nil || r.AcceptedSequence != 2 {
		t.Fatal(r, e)
	}
	if _, e = h.pool.Lookup(ctx, "gym-b", b.Job.ID, wb.Nonce); e != poolshare.ErrNotFound {
		t.Fatal("winner was already retained", e)
	}
	if _, e = h.pool.Issue(ctx, "gym-a"); e != poolshare.ErrCapacity {
		t.Fatal("ordinary intake is not full", e)
	}
	h.node.canonical[wb.WorkHash] = true
	h.node.tip = b.Job.Context.ParentHeight + 61
	h.reopen(t)
	assertOriginal()
	beforeBroadcasts := h.node.broadcasts
	rb, e := h.pool.Submit(ctx, "gym-b", b.Job.ID, wb.Nonce)
	if e != nil || rb.Sequence != 3 || rb.AcceptedSequence != 3 || rb.BlockState != "canonical-observed" || rb.BlockObservedAtMS == 0 {
		t.Fatal("unseen canonical recovery", rb, e)
	}
	if h.node.broadcasts != beforeBroadcasts {
		t.Fatal("canonical recovery speculatively broadcast")
	}
	h.reopen(t)
	assertOriginal()
	retry, e := h.pool.Submit(ctx, "gym-b", b.Job.ID, wb.Nonce)
	if e != nil || retry != rb {
		t.Fatal("duplicate created another receipt", retry, e)
	}
	if _, e = h.rounds.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	round, e := h.rounds.Refresh(ctx, wb.WorkHash)
	if e != nil || round.Confirmation == nil || round.Round.FirstAccepted != 2 || round.Round.LastAccepted != 3 {
		t.Fatal("fixed late-winner round", round, e)
	}
	if len(h.settlement.state.Credits) != 1 {
		t.Fatal("receipt bypassed settlement import")
	}
	assertOriginal()
	delete(base.txs, original.TxID)
	if r, e := h.settlement.RecoverPending(ctx); e != nil || r.State != "pending" {
		t.Fatal(r, e)
	}
	if len(base.broadcastBytes) != 2 || hex.EncodeToString(base.broadcastBytes[1]) != original.Raw {
		t.Fatal("restart changed original rebroadcast")
	}
	assertOriginal()
	base.confirmations = 1
	if _, e = h.settlement.RecoverPending(ctx); e != nil {
		t.Fatal(e)
	}
	h.backend.inspect = func(_ context.Context, coins []Coin) ([]Funding, error) {
		out := make([]Funding, len(coins))
		for i, c := range coins {
			state := "ready"
			if c.Coinbase && c.BlockHash == wb.WorkHash {
				state = "immature"
			}
			out[i] = Funding{TxID: c.TxID, Index: c.Index, State: state}
		}
		return out, nil
	}
	if r, e := h.settlement.RunDaily(ctx); e != nil || r.Reason != "daily-batch-complete" {
		t.Fatal(r, e)
	}
	assertCredits := func() {
		t.Helper()
		if len(h.settlement.state.Credits) != 2 {
			t.Fatal("not exactly one additional credit")
		}
		count := 0
		for _, c := range h.settlement.state.Credits {
			if c.RoundHash == wb.WorkHash {
				count++
				if c.Coin.TxID == original.Inputs[0].TxID {
					t.Fatal("new reward aliases reserved funding")
				}
				var atoms Fraction
				for _, claim := range c.Coin.Claims {
					if claim.GymID == b.Job.GymID {
						atoms = claim.Amount
					}
				}
				if atoms != (Fraction{"990000000", "1"}) {
					t.Fatal("new reward credited twice or split incorrectly", atoms)
				}
			}
		}
		if count != 1 {
			t.Fatal("duplicate round credit")
		}
		assertOriginal()
	}
	assertCredits()
	h.reopen(t)
	if _, e = h.settlement.RunDaily(ctx); e != nil {
		t.Fatal(e)
	}
	assertCredits()
	if info, e := h.issuer.Info(); e != nil || info.Records != 0 {
		t.Fatal("pool credit created a fighter", info, e)
	}
}
