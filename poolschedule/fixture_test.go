// Copyright (c) 2026 The SLUG developers. ISC license.
// Mechanically adapted from poolsettle/fixture_test.go SHA256 61ca9dc011feea5793c11712fcde70e1a8b6981adff12d68eac6fe08aeb82006.
// The existing independent Set52 source remains pinned below; no new expected
// transaction bytes are derived from scheduler code. Settlement uses real time.
package poolschedule

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
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

const settlementTestToken = "public-settlement-test-session"

// All keys below are deliberately public scalar fixtures, never wallet or
// checkpoint keys. The source template is pinned to the independent Set52
// corpus; its payout is changed to scalar3 and a fresh issued job is actually
// mined. These are composition tests, not new expected-hash vectors.
func settlementFixtureKey(n byte) (*btcec.PrivateKey, string, string) {
	var scalar [32]byte
	scalar[31] = n
	key, pub := btcec.PrivKeyFromBytes(scalar[:])
	hash := address.Hash160(pub.SerializeCompressed())
	addr, _ := address.NewAddressWitnessPubKeyHash(hash, &chaincfg.MainNetParams)
	script := append([]byte{txscript.OP_0, 20}, hash...)
	return key, hex.EncodeToString(script), addr.EncodeAddress()
}

type settlementSessions struct{ key string }

func (s settlementSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	if token != settlementTestToken {
		return errors.New("bad fixture session")
	}
	return fn(s.key, ctx.Err)
}

type settlementSourceNode struct {
	template prospectwork.Template
	winner   prospectwork.ShareCheck
}

func (n *settlementSourceNode) ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error) {
	return n.template, nil
}
func (n *settlementSourceNode) Proposal(context.Context, prospect.Context, string) error { return nil }
func (n *settlementSourceNode) RecheckContext(context.Context, prospect.Context) error   { return nil }
func (n *settlementSourceNode) SubmitBlock(_ context.Context, _ prospect.Context, raw string) error {
	if n.winner.Candidate == nil || raw != n.winner.Candidate.BlockHex {
		return errors.New("unexpected fixture block")
	}
	return nil
}
func (n *settlementSourceNode) CanonicalBlock(_ context.Context, _ prospect.Context, p prospect.Proof, hash string) error {
	if n.winner.Candidate == nil || hash != n.winner.WorkHash || !reflect.DeepEqual(p, n.winner.Candidate.Proof) {
		return errors.New("unexpected fixture canonical proof")
	}
	return nil
}
func (n *settlementSourceNode) ObservePoolBlock(_ context.Context, c prospect.Context, p prospect.Proof, hash string) (prospectnode.PoolBlockObservation, error) {
	if err := n.CanonicalBlock(context.Background(), c, p, hash); err != nil {
		return prospectnode.PoolBlockObservation{}, err
	}
	return prospectnode.PoolBlockObservation{BlockHash: hash, BlockHeight: c.ParentHeight + 1, TipHash: strings.Repeat("a", 64), TipHeight: c.ParentHeight + 61, Canonical: true, Descendants: 60}, nil
}

type settlementBackend struct {
	funding                        string
	fee                            int64
	signCalls, broadcasts          int
	feeCalls                       int
	txs                            map[string][]byte
	confirmations                  uint32
	loseAck, failLookup, badSigner bool
	beforeBroadcast                func()
	beforeInspect                  func(context.Context)
	afterSign                      func()
	broadcastBytes                 [][]byte
}

func (b *settlementBackend) Inspect(ctx context.Context, coins []poolsettle.Coin) ([]poolsettle.Funding, error) {
	if b.beforeInspect != nil {
		b.beforeInspect(ctx)
	}
	result := make([]poolsettle.Funding, len(coins))
	for i, c := range coins {
		result[i] = poolsettle.Funding{TxID: c.TxID, Index: c.Index, State: b.funding}
	}
	return result, nil
}
func (b *settlementBackend) EstimateFee(context.Context, []poolsettle.Coin, []poolsettle.Payment, string) (int64, error) {
	b.feeCalls++
	return b.fee, nil
}
func (b *settlementBackend) Sign(_ context.Context, coins []poolsettle.Coin, raw []byte) ([]byte, error) {
	b.signCalls++
	tx := wire.NewMsgTx(2)
	if err := tx.Deserialize(bytes.NewReader(raw)); err != nil {
		return nil, err
	}
	if b.badSigner {
		tx.TxOut[0].PkScript = append([]byte(nil), tx.TxOut[len(tx.TxOut)-1].PkScript...)
	}
	key, scriptHex, _ := settlementFixtureKey(3)
	script, _ := hex.DecodeString(scriptHex)
	code := append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, script[2:]...)
	code = append(code, txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for _, c := range coins {
		if c.Script != scriptHex {
			return nil, errors.New("unexpected signing script")
		}
		h, err := chainhash.NewHashFromStr(c.TxID)
		if err != nil {
			return nil, err
		}
		fetcher.AddPrevOut(*wire.NewOutPoint(h, c.Index), wire.NewTxOut(c.Atoms, script))
	}
	hashes := txscript.NewTxSigHashes(tx, fetcher)
	for i, c := range coins {
		w, err := txscript.WitnessSignature(tx, hashes, i, c.Atoms, code, txscript.SigHashAll, key, true)
		if err != nil {
			return nil, err
		}
		tx.TxIn[i].Witness = w
	}
	var out bytes.Buffer
	if err := tx.Serialize(&out); err != nil {
		return nil, err
	}
	if b.afterSign != nil {
		b.afterSign()
	}
	return out.Bytes(), nil
}
func (b *settlementBackend) ObserveTransaction(_ context.Context, id string) (poolsettle.TransactionObservation, error) {
	if b.failLookup {
		return poolsettle.TransactionObservation{}, errors.New("fixture RPC unavailable")
	}
	return poolsettle.TransactionObservation{Raw: append([]byte(nil), b.txs[id]...), Confirmations: b.confirmations}, nil
}
func (b *settlementBackend) Broadcast(_ context.Context, raw []byte) (string, error) {
	if b.beforeBroadcast != nil {
		b.beforeBroadcast()
	}
	tx := wire.NewMsgTx(2)
	if err := tx.Deserialize(bytes.NewReader(raw)); err != nil {
		return "", err
	}
	id := tx.TxHash().String()
	b.broadcasts++
	b.broadcastBytes = append(b.broadcastBytes, append([]byte(nil), raw...))
	b.txs[id] = append([]byte(nil), raw...)
	if b.loseAck {
		b.loseAck = false
		return "", errors.New("fixture lost send acknowledgment")
	}
	return id, nil
}

type settlementHarness struct {
	store     *poolsettle.Store
	issuer    *registry.ManifestStore
	source    *poolround.Store
	pool      *poolshare.Pool
	node      *settlementSourceNode
	backend   *settlementBackend
	config    poolsettle.Config
	path, gym string
	clock     time.Time
}

func newSettlementHarness(t *testing.T) *settlementHarness {
	return newSettlementHarnessWithLimit(t, 4)
}

func newSettlementHarnessWithLimit(t *testing.T, maxRounds uint32) *settlementHarness {
	t.Helper()
	if wire.TestPoW {
		t.Skip("actual normal-PoW source; refusal tested separately")
	}
	raw, err := os.ReadFile("../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("independent template fixture changed")
	}
	var corpus struct {
		Sources []struct {
			Name     string                `json:"name"`
			Template prospectwork.Template `json:"template"`
		} `json:"sources"`
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	var template prospectwork.Template
	for _, s := range corpus.Sources {
		if s.Name == "coinbase-only" {
			template = s.Template
		}
	}
	if template.Context.Version == 0 {
		t.Fatal("missing pinned fixture template")
	}
	descriptor, err := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := registry.CreateManifestAuthority(filepath.Join(t.TempDir(), "issuer"), template.Context.Network, template.Context.Genesis, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = issuer.Close() })
	info, err := issuer.Info()
	if err != nil {
		t.Fatal(err)
	}
	gymKey, _, _ := settlementFixtureKey(1)
	_, poolScript, poolAddress := settlementFixtureKey(3)
	_, payScript, _ := settlementFixtureKey(4)
	node := &settlementSourceNode{template: template}
	c := template.Context
	config := poolshare.Config{Version: 1, AuthorityPin: info.Pin, Policy: prospectnode.Policy{Version: 1, Network: c.Network, Genesis: c.Genesis, DNAVersion: c.DNAVersion, ProspectTarget: c.ProspectTarget}, PayoutAddress: poolAddress, PoolTarget: strings.Repeat("f", 64), MaxWorks: 4, MaxSubmissions: 4, MaxBytes: 8 << 20}
	pool, err := poolshare.Create(filepath.Join(t.TempDir(), "pool"), config, issuer, node, settlementSessions{hex.EncodeToString(gymKey.PubKey().SerializeCompressed())})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	ctx := context.Background()
	work, err := pool.Issue(ctx, settlementTestToken)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := prospectwork.PrepareManifestWork(work.Job, template, poolAddress, 0, config.PoolTarget)
	if err != nil {
		t.Fatal(err)
	}
	for nonce := uint32(0); nonce < 64; nonce++ {
		check, e := prepared.CheckNonce(ctx, nonce)
		if e != nil {
			t.Fatal(e)
		}
		if check.Block {
			node.winner = check
			break
		}
	}
	if node.winner.Candidate == nil {
		t.Fatal("fixture nonce-search bound exhausted")
	}
	accepted, err := pool.Submit(ctx, settlementTestToken, work.Job.ID, node.winner.Nonce)
	if err != nil || accepted.AcceptedSequence != 1 {
		t.Fatalf("accept actual pool work: %+v %v", accepted, err)
	}
	rounds, err := poolround.Create(filepath.Join(t.TempDir(), "rounds"), poolround.Limits{Version: 1, MaxRounds: 4, MaxBytes: 8 << 20}, pool, node)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rounds.Close() })
	if _, err = rounds.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	record, err := rounds.Refresh(ctx, node.winner.WorkHash)
	if err != nil || record.Confirmation == nil {
		t.Fatalf("confirmed source: %+v %v", record, err)
	}
	h := &settlementHarness{source: rounds, pool: pool, node: node, backend: &settlementBackend{funding: "ready", fee: 1000, txs: make(map[string][]byte)}, config: poolsettle.Config{Version: 1, ChangeScript: poolScript, Destinations: []poolsettle.Destination{{GymID: work.Job.GymID, Script: payScript}}, MaxRounds: maxRounds, MaxBatches: 4}, path: filepath.Join(t.TempDir(), "settlement"), gym: work.Job.GymID, clock: time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)}
	h.issuer = issuer
	h.store, err = poolsettle.Create(h.path, h.config, h.source, h.backend)
	if err != nil {
		t.Fatal(err)
	}
	// Use the real production settlement clock; scheduler timing never changes it.
	t.Cleanup(func() { _ = h.store.Close() })
	return h
}

// addConfirmedRound extends the controlled source's canonical-parent assertion
// and actually mines/accepts a second manifest-bound winner. It does not mint a
// settlement credit directly or bypass either concrete source journal.
func (h *settlementHarness) addConfirmedRound(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	h.node.template.Context.ParentHash = h.node.winner.WorkHash
	h.node.template.Context.ParentHeight++
	h.node.template.Block.PreviousHash = h.node.winner.WorkHash
	h.node.template.Block.Height++
	work, err := h.pool.Issue(ctx, settlementTestToken)
	if err != nil {
		t.Fatal(err)
	}
	_, _, payout := settlementFixtureKey(3)
	prepared, err := prospectwork.PrepareManifestWork(work.Job, h.node.template, payout, 0, strings.Repeat("f", 64))
	if err != nil {
		t.Fatal(err)
	}
	h.node.winner = prospectwork.ShareCheck{}
	for nonce := uint32(0); nonce < 64; nonce++ {
		check, err := prepared.CheckNonce(ctx, nonce)
		if err != nil {
			t.Fatal(err)
		}
		if check.Block {
			h.node.winner = check
			break
		}
	}
	if h.node.winner.Candidate == nil {
		t.Fatal("second fixture nonce-search bound exhausted")
	}
	receipt, err := h.pool.Submit(ctx, settlementTestToken, work.Job.ID, h.node.winner.Nonce)
	if err != nil || receipt.AcceptedSequence != 2 {
		t.Fatalf("second accepted round: %+v %v", receipt, err)
	}
	if _, err = h.source.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	round, err := h.source.Refresh(ctx, h.node.winner.WorkHash)
	if err != nil || round.Confirmation == nil {
		t.Fatalf("second confirmed round: %+v %v", round, err)
	}
}

func (h *settlementHarness) reopen(t *testing.T) {
	t.Helper()
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	h.store, err = poolsettle.Open(h.path, h.config, h.source, h.backend)
	if err != nil {
		t.Fatal(err)
	}
	// Use the real production settlement clock; scheduler timing never changes it.
}
