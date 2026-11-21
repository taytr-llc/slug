//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

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
	"sync"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

// Real original issuer/source/round/ledger owners, with controlled session and
// node observations and the published scalar3 signer. This fixture creates no
// READY runtime, listeners or wallet and makes no actual-node/custody claim.
type intakeBusyHarness struct {
	control                    *Runtime
	issuer                     *registry.ManifestStore
	pool                       *poolshare.Pool
	rounds                     *poolround.Store
	ledger                     *poolsettle.Store
	node                       *opLiveNode
	backend                    *intakeBusyBackend
	source                     poolshare.Config
	config                     poolsettle.Config
	ledgerPath, publicKey, gym string
}

type intakeBusyBackend struct {
	inspections, quotes, signs, sends, observations int
	raw                                             []byte
}

func (b *intakeBusyBackend) Inspect(ctx context.Context, coins []poolsettle.Coin) ([]poolsettle.Funding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.inspections++
	out := make([]poolsettle.Funding, len(coins))
	for i, c := range coins {
		out[i] = poolsettle.Funding{TxID: c.TxID, Index: c.Index, State: "ready"}
	}
	return out, nil
}
func (b *intakeBusyBackend) EstimateFee(ctx context.Context, _ []poolsettle.Coin, _ []poolsettle.Payment, _ string) (int64, error) {
	b.quotes++
	return 1000, ctx.Err()
}
func (b *intakeBusyBackend) Sign(ctx context.Context, coins []poolsettle.Coin, raw []byte) ([]byte, error) {
	b.signs++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tx := wire.NewMsgTx(2)
	if err := tx.Deserialize(bytes.NewReader(raw)); err != nil {
		return nil, err
	}
	var scalar [32]byte
	scalar[31] = 3
	key, _ := btcec.PrivKeyFromBytes(scalar[:])
	defer key.Zero()
	script, e := hex.DecodeString(poolsettlefixture.PoolScript())
	if e != nil {
		return nil, e
	}
	code := append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, script[2:]...)
	code = append(code, txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for _, c := range coins {
		if c.Script != poolsettlefixture.PoolScript() {
			return nil, errors.New("fixture input script")
		}
		hash, e := chainhash.NewHashFromStr(c.TxID)
		if e != nil {
			return nil, e
		}
		fetcher.AddPrevOut(*wire.NewOutPoint(hash, c.Index), wire.NewTxOut(c.Atoms, script))
	}
	hashes := txscript.NewTxSigHashes(tx, fetcher)
	for i, c := range coins {
		w, e := txscript.WitnessSignature(tx, hashes, i, c.Atoms, code, txscript.SigHashAll, key, true)
		if e != nil {
			return nil, e
		}
		tx.TxIn[i].Witness = w
	}
	var out bytes.Buffer
	if e = tx.Serialize(&out); e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}
func (b *intakeBusyBackend) ObserveTransaction(ctx context.Context, _ string) (poolsettle.TransactionObservation, error) {
	b.observations++
	return poolsettle.TransactionObservation{}, ctx.Err()
}
func (b *intakeBusyBackend) Broadcast(ctx context.Context, raw []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	b.sends++
	b.raw = append([]byte(nil), raw...)
	return "", errors.New("controlled lost acknowledgement")
}

func newIntakeBusyHarness(t *testing.T) *intakeBusyHarness {
	t.Helper()
	original, _ := opActionTestRuntime(t)
	ctx := context.Background()
	root := opSetupTestDir(t)
	h := &intakeBusyHarness{issuer: original.issuer, publicKey: original.setup.Miners.Enrollments[0].PublicKey, gym: original.setup.Miners.Enrollments[0].GymID, backend: &intakeBusyBackend{}}
	// This cleanup is registered after the original authority's cleanup, so all
	// borrowing daily stores join before that original issuer/key owner closes.
	t.Cleanup(func() {
		if h.ledger != nil {
			if e := h.ledger.Close(); e != nil {
				t.Error(e)
				return
			}
			h.ledger = nil
		}
		if h.rounds != nil {
			if e := h.rounds.Close(); e != nil {
				t.Error(e)
				return
			}
			h.rounds = nil
		}
		if h.pool != nil {
			if e := h.pool.Close(); e != nil {
				t.Error(e)
				return
			}
			h.pool = nil
		}
	})
	raw, e := os.ReadFile("../../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("independent template changed")
	}
	var f struct {
		Sources []struct {
			Name     string
			Template prospectwork.Template
		}
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	h.node = &opLiveNode{winners: map[string]prospectwork.ShareCheck{}, canonical: map[string]bool{}}
	for _, s := range f.Sources {
		if s.Name == "coinbase-only" {
			h.node.template = s.Template
		}
	}
	if h.node.template.Context.Version == 0 {
		t.Fatal("missing template")
	}
	h.source = original.setup.Pool
	h.source.Version = 2
	h.source.PayoutAddress = poolsettlefixture.PoolAddress()
	h.source.MaxWorks = 8
	h.source.MaxSubmissions = 8
	// Frozen daily-capacity reference kernel-actual-derived-limit: ordinary2
	// reserves N3/R3 and five possible batches; Batches4 would allow only1.
	h.source.MaxBytes = 64 << 20
	h.config = original.setup.Settlement
	h.config.Version = 2
	h.config.ChangeScript = poolsettlefixture.PoolScript()
	h.config.MaxRounds = 8
	h.config.MaxBatches = 6
	h.ledgerPath = filepath.Join(root, "ledger")
	h.pool, e = poolshare.CreateDaily(ctx, filepath.Join(root, "pool"), h.source, h.issuer, h.node, opLiveSessions(h.publicKey))
	if e != nil {
		t.Fatal(e)
	}
	h.rounds, e = poolround.CreateDaily(ctx, filepath.Join(root, "rounds"), poolround.Limits{Version: 2, MaxRounds: 8, MaxBytes: 32 << 20}, h.pool, h.node)
	if e != nil {
		t.Fatal(e)
	}
	h.ledger, e = poolsettle.CreateDaily(ctx, h.ledgerPath, h.config, h.rounds, h.backend)
	if e != nil {
		t.Fatal(e)
	}
	gyms := make([]string, len(h.config.Destinations))
	for i, d := range h.config.Destinations {
		gyms[i] = d.GymID
	}
	intake, e := h.pool.AttachDailyOperatorAdmission(ctx, h.source, poolshare.DailyRecoveryBudget{Rounds: 8, RoundBytes: 32 << 20, CreditRounds: 8, Batches: 6, GymIDs: gyms})
	if e != nil {
		t.Fatal(e)
	}
	server, e := prospectservice.NewPausedFencedManifestAdmissionServer(prospectservice.ManifestServerConfig{Version: 3, Listen: "127.0.0.1:0", AuthorityPin: h.source.AuthorityPin, Clients: []prospectservice.Credential{}, MaxJobs: h.source.MaxWorks, MaxRecords: uint64(original.setup.Authority.Limits.Admissions)}, h.source.Policy, capacityUnusedNode{t}, h.issuer, original.miners.AdmissionSessions())
	if e != nil {
		t.Fatal(e)
	}
	h.control = &Runtime{intake: intake, admissionServer: server}
	status, e := intake.Status(ctx)
	if e != nil || status.Mode != "paused" || status.Maximum != 2 {
		t.Fatal("original paused derived envelope", status, e)
	}
	return h
}

// Hold the ORIGINAL registry gate in its public commit's final recheck. The
// bounded channel fence is test-only; a production lease recheck must never
// perform external I/O. Refusing the callback occurs before clock/job writes.
func (h *intakeBusyHarness) holdIssuer(t *testing.T) func() {
	t.Helper()
	before, e := h.issuer.TryInfo()
	if e != nil {
		t.Fatal(e)
	}
	prepared, e := h.issuer.PrepareManifestIssue(h.node.template.Context, h.publicKey)
	if e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	refused := errors.New("controlled issuer fence refused without commit")
	go func() {
		_, e := h.issuer.CommitManifestIssue(prepared, func() error { close(entered); <-release; return refused })
		done <- e
	}()
	var once sync.Once
	joined := false
	join := func() {
		if joined {
			return
		}
		once.Do(func() { close(release) })
		select {
		case err := <-done:
			joined = true
			if !errors.Is(err, refused) {
				t.Error("unexpected issuer result", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("original issuer did not join")
			return
		}
		// The resumed intake now shares this gate. Use the original blocking
		// metadata read after our commit has joined, outside any source/account
		// lease, so legitimate resumed auditing cannot make this check flaky.
		after, e := h.issuer.Info()
		if e != nil || !reflect.DeepEqual(before, after) {
			t.Error("refused fence mutated issuer", e)
		}
	}
	t.Cleanup(join)
	select {
	case <-entered:
	case err := <-done:
		joined = true
		t.Fatal("issuer never acquired gate", err)
	case <-time.After(10 * time.Second):
		t.Fatal("issuer did not reach fence")
	}
	if _, e = h.issuer.TryInfo(); e != registry.ErrBusy {
		t.Fatal("not original issuer contention", e)
	}
	if _, e = h.control.intake.Status(context.Background()); e != poolshare.ErrBusy {
		t.Fatal("daily census lost exact source Busy", e)
	}
	return join
}

// Read every logical ledger row only after the exact writable owner closes,
// then reopen the same ledger against the same concrete source/backend.
func (h *intakeBusyHarness) ledgerSnapshot(t *testing.T) map[string][]byte {
	t.Helper()
	if e := h.ledger.Close(); e != nil {
		t.Fatal(e)
	}
	h.ledger = nil
	db, e := leveldb.OpenFile(filepath.Join(h.ledgerPath, "db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true, Strict: opt.StrictAll})
	if e != nil {
		t.Fatal(e)
	}
	out := map[string][]byte{}
	it := db.NewIterator(nil, nil)
	for it.Next() {
		out[string(it.Key())] = append([]byte(nil), it.Value()...)
	}
	e = it.Error()
	it.Release()
	ce := db.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	h.ledger, e = poolsettle.OpenDaily(context.Background(), h.ledgerPath, h.config, h.rounds, h.backend)
	if e != nil {
		t.Fatal(e)
	}
	return out
}

func (h *intakeBusyHarness) reserveA(t *testing.T) poolsettle.Report {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e := h.control.updateIntake(ctx, true, true, ""); e != nil {
		t.Fatal(e)
	}
	work, e := h.pool.IssueMiningWork(ctx, "published-live-round-session")
	if e != nil {
		t.Fatal(e)
	}
	prepared, e := prospectwork.PrepareManifestWork(work.Job, work.Template, h.source.PayoutAddress, 0, h.source.PoolTarget)
	if e != nil {
		t.Fatal(e)
	}
	var winner prospectwork.ShareCheck
	for nonce := uint32(0); nonce < 64; nonce++ {
		check, e := prepared.CheckNonce(ctx, nonce)
		if e != nil {
			t.Fatal(e)
		}
		if check.Block {
			winner = check
			break
		}
	}
	if winner.Candidate == nil {
		t.Fatal("bounded normal-PoW search exhausted")
	}
	h.node.winners[winner.WorkHash] = winner
	h.node.canonical[winner.WorkHash] = true
	h.node.tip = work.Job.Context.ParentHeight + 61
	receipt, e := h.pool.Submit(ctx, "published-live-round-session", work.Job.ID, winner.Nonce)
	if e != nil || receipt.AcceptedSequence != 1 || !receipt.Block || receipt.GymID != h.gym {
		t.Fatal(receipt, e)
	}
	if _, e = h.rounds.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	row, e := h.rounds.Refresh(ctx, winner.WorkHash)
	if e != nil || row.Confirmation == nil {
		t.Fatal(row, e)
	}
	report, e := h.ledger.RunDaily(ctx)
	if e != nil || report.State != "pending" || report.Reason != "broadcast-unknown" || report.TxID == "" {
		t.Fatal(report, e)
	}
	if h.backend.signs != 1 || h.backend.sends != 1 || h.backend.quotes != 1 || len(h.backend.raw) == 0 {
		t.Fatal("missing original signed reserved payout")
	}
	found := false
	for _, balance := range report.Balances {
		if balance.GymID == h.gym {
			found = true
			if balance.ReservedAtoms != 990000000 || balance.PaidAtoms != 0 || balance.Confirmed != (poolsettle.Fraction{Numerator: "990000000", Denominator: "1"}) {
				t.Fatal("original one-reward 99/1 reservation", balance)
			}
		}
	}
	if !found {
		t.Fatal("original gym balance missing")
	}
	return report
}

func TestDailyIntakeOriginalIssuerBusyPreservesDrainAndPayout(t *testing.T) {
	h := newIntakeBusyHarness(t)
	beforeReport := h.reserveA(t)
	before := h.ledgerSnapshot(t)
	bodyKey := string(append([]byte("b/"), 0, 0, 0, 0, 0, 0, 0, 1))
	if len(before[bodyKey]) == 0 {
		t.Fatal("missing actual immutable batch body")
	}
	var body struct{ Raw, TxID string }
	if json.Unmarshal(before[bodyKey], &body) != nil || body.Raw != hex.EncodeToString(h.backend.raw) || body.TxID != beforeReport.TxID {
		t.Fatal("original signed body differs from reservation")
	}
	counts := [5]int{h.backend.inspections, h.backend.quotes, h.backend.signs, h.backend.sends, h.backend.observations}
	signed := append([]byte(nil), h.backend.raw...)
	release := h.holdIssuer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.control.updateIntake(ctx, false, false, "public_capacity") }()
	joined := false
	t.Cleanup(func() {
		if !joined {
			cancel()
			release()
			select {
			case <-done:
				joined = true
			case <-time.After(10 * time.Second):
				t.Error("intake transition failed to join")
			}
		}
	})
	// The prior implementation returned exact Busy immediately. Keep the actual
	// source gate occupied across several retry intervals, with no other caller
	// responsible for remembering this capacity event.
	select {
	case err := <-done:
		joined = true
		t.Fatal("capacity cause escaped instead of waiting", err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case err := <-done:
		joined = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("same intake invocation failed to complete")
	}
	status, e := h.control.intake.Status(context.Background())
	if e != nil || status.Mode != "draining" || status.Submissions != 1 {
		t.Fatal(status, e)
	}
	if h.control.status.Capacity.Reason != "public_capacity" {
		t.Fatal("lost original capacity cause", h.control.status.Capacity)
	}
	if mode, e := h.control.admissionServer.OperatorIntakeMode(context.Background()); e != nil || mode != "draining" {
		t.Fatal(mode, e)
	}
	if e = h.control.updateIntake(context.Background(), true, true, ""); e != nil || h.control.status.Capacity.Reason != "public_capacity" || h.control.status.Capacity.Mode != "draining" {
		t.Fatal("later readiness undid permanent drain", e)
	}
	after := h.ledgerSnapshot(t)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("intake contention/drain changed original ledger rows")
	}
	afterReport, e := h.ledger.View(context.Background())
	if e != nil || !reflect.DeepEqual(beforeReport, afterReport) {
		t.Fatal("reserved payout view changed", e)
	}
	if counts != ([5]int{h.backend.inspections, h.backend.quotes, h.backend.signs, h.backend.sends, h.backend.observations}) || !bytes.Equal(signed, h.backend.raw) {
		t.Fatal("intake retry performed financial effects")
	}
}

func TestDailyIntakeOriginalIssuerCancellationNeverEnables(t *testing.T) {
	h := newIntakeBusyHarness(t)
	for _, deadline := range []bool{false, true} {
		name := "parent-cancel"
		if deadline {
			name = "parent-deadline"
		}
		t.Run(name, func(t *testing.T) {
			release := h.holdIssuer(t)
			ctx, cancel := context.WithCancel(context.Background())
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- h.control.updateIntake(ctx, true, true, "") }()
			joined := false
			t.Cleanup(func() {
				if !joined {
					cancel()
					release()
					select {
					case <-done:
						joined = true
					case <-time.After(10 * time.Second):
						t.Error("canceled intake did not join")
					}
				}
			})
			if !deadline {
				select {
				case err := <-done:
					joined = true
					t.Fatal("busy escaped before parent cancellation", err)
				case <-time.After(100 * time.Millisecond):
				}
				cancel()
			}
			want := error(context.Canceled)
			if deadline {
				want = context.DeadlineExceeded
			}
			select {
			case err := <-done:
				joined = true
				if err != want {
					t.Fatal("wrong parent cause", err, want)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("canceled transition failed to join")
			}
			// The issuer is STILL held: no source release is needed to cancel a retry.
			if mode, e := h.control.admissionServer.OperatorIntakeMode(context.Background()); e != nil || mode != "paused" {
				t.Fatal("cancellation enabled admission", mode, e)
			}
			release()
			s, e := h.control.intake.Status(context.Background())
			if e != nil || s.Mode != "paused" || s.Submissions != 0 {
				t.Fatal("cancellation enabled or changed pool", s, e)
			}
		})
	}
	if h.backend.inspections+h.backend.quotes+h.backend.signs+h.backend.sends+h.backend.observations != 0 {
		t.Fatal("canceled intake touched finance")
	}
}
