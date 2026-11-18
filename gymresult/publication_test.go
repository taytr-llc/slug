//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/seednode"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"strings"
	"testing"
)

// Modeled verified-network facts drive the actual source-authorized journal
// actions. They are not proof of node ownership, PoW or a real dispatch.
type modelPublisher struct {
	*modelResultChain
	publication           Publication
	builds, sends         int
	afterBuild, afterSend func()
	sendError             error
	sent                  [][]byte
	dynamicCarrier        bool
}

func (m *modelPublisher) Unspent(context.Context, Outpoint) (*seednode.UnspentOutput, error) {
	c := m.publication.Coin
	return &seednode.UnspentOutput{ValueSatoshis: c.ValueSatoshis, Script: c.Script, Coinbase: c.Coinbase, Confirmations: m.tip.Height - c.BlockHeight + 1, BestBlock: m.tip.Hash}, nil
}
func (m *modelPublisher) BuildPublication(_ context.Context, payload [73]byte, c FundingCoin) ([]byte, error) {
	m.builds++
	if m.dynamicCarrier {
		var scalar [32]byte
		scalar[31] = 2
		key, _ := btcec.PrivKeyFromBytes(scalar[:])
		defer key.Zero()
		h, _ := chainhash.NewHashFromStr(c.Outpoint.TxID)
		tx := wire.NewMsgTx(2)
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(h, c.Outpoint.Index), nil, nil))
		script := rawID(c.Script)
		code := append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, script[2:]...)
		code = append(code, txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
		tx.AddTxOut(wire.NewTxOut(0, append([]byte{txscript.OP_RETURN, 73}, payload[:]...)))
		tx.AddTxOut(wire.NewTxOut(c.ValueSatoshis-1000, script))
		fetcher := txscript.NewCannedPrevOutputFetcher(script, c.ValueSatoshis)
		w, e := txscript.WitnessSignature(tx, txscript.NewTxSigHashes(tx, fetcher), 0, c.ValueSatoshis, code, txscript.SigHashAll, key, true)
		if e != nil {
			return nil, e
		}
		tx.TxIn[0].Witness = w
		var b bytes.Buffer
		if e = tx.Serialize(&b); e != nil {
			return nil, e
		}
		m.publication.RawTx = hex.EncodeToString(b.Bytes())
		m.publication.TxID = tx.TxHash().String()
	}
	if m.afterBuild != nil {
		m.afterBuild()
	}
	return rawID(m.publication.RawTx), nil
}
func (m *modelPublisher) SubmitPublication(_ context.Context, _ [73]byte, _ FundingCoin, raw []byte) (string, error) {
	m.sends++
	m.sent = append(m.sent, append([]byte(nil), raw...))
	if m.afterSend != nil {
		m.afterSend()
	}
	return m.publication.TxID, m.sendError
}
func fundedPublisher(t *testing.T, f *resultFixture, r Record) *modelPublisher {
	t.Helper()
	pub := *referenceRecord(t, "record-built").Publication
	m := &modelPublisher{modelResultChain: modelTip(r), publication: pub}
	m.hashes[pub.Coin.BlockHeight] = pub.Coin.BlockHash
	next := f.s.meta
	next.LastFundingTip = &m.tip
	next.FundingComplete = true
	next.FundingCursor.Height = int32(m.tip.Height)
	next.FundingCursor.Hash = m.tip.Hash
	next.FundingCursor.CompleteToHeight = int32(m.tip.Height)
	next.FundingCursor.CompleteToHash = m.tip.Hash
	next.FundingCursor.ScanDigest = strings.Repeat("c", 64)
	b := new(leveldb.Batch)
	if e := put(b, coinKey('c', pub.Coin.Outpoint), pub.Coin, 1024); e != nil {
		t.Fatal(e)
	}
	if e := f.s.commitLocked(b, next); e != nil {
		t.Fatal(e)
	}
	return m
}
func auditCurrent(t *testing.T, f *resultFixture) {
	t.Helper()
	if e := f.s.audit(context.Background(), marker{"slug-candidate-results", 1, f.s.meta.StoreID, f.pin}); e != nil {
		t.Fatal("journal graph audit", e)
	}
}
func TestResultTargetChangesAfterBuildNeverRetainsUnreservedTx(t *testing.T) {
	f, c, done, r := preparedFixture(t)
	defer done()
	m := fundedPublisher(t, f, r)
	m.afterBuild = func() { m.hashes[r.Statement.Statement.Source.Source.TargetHeight] = strings.Repeat("e", 64) }
	_, e := f.s.publish(c, m)
	if !errors.Is(e, ErrSourceConflict) || m.builds != 1 || m.sends != 0 {
		t.Fatal("post-build source fence", e, m.builds, m.sends)
	}
	row, _, e := f.s.record(r.ResultKey)
	if e != nil || row.Publication != nil || len(row.Conflicts) != 1 {
		t.Fatal("tentative tx escaped", e)
	}
	if _, ok, e := f.s.raw(coinKey('u', m.publication.Coin.Outpoint), 256); e != nil || ok {
		t.Fatal("reservation without publication", e)
	}
	auditCurrent(t, f)
}
func TestResultDispatchRetryPreservesRawAndReservation(t *testing.T) {
	f, c, done, r := preparedFixture(t)
	defer done()
	m := fundedPublisher(t, f, r)
	m.sendError = seednode.ErrOutcomeUnknown
	_, e := f.s.publish(c, m)
	if !errors.Is(e, ErrOutcomeUnknown) || m.sends != 1 {
		t.Fatal("lost ACK", e)
	}
	first, _, e := f.s.record(r.ResultKey)
	if e != nil || first.Publication.State != "dispatch_unknown" || first.Publication.DispatchCount != 1 {
		t.Fatal(e)
	}
	auditCurrent(t, f)
	m.sendError = nil
	got, e := f.s.publish(c, m)
	if e != nil || m.builds != 1 || m.sends != 2 || got.Record.Publication.DispatchCount != 2 || got.Record.Publication.State != "submitted" || !bytes.Equal(m.sent[0], m.sent[1]) {
		t.Fatal("exact retry", e, m.builds, m.sends)
	}
	if got.Status.FundingState != "waiting_for_funds" {
		t.Fatal("reserved coin advertised available", got.Status.FundingState)
	}
	auditCurrent(t, f)
}

func TestResultAcknowledgedBuiltThenHoldIsDefinite(t *testing.T) {
	f, c, done, r := preparedFixture(t)
	defer done()
	m := fundedPublisher(t, f, r)
	f.s.checkpoint = func(stage string) {
		if stage == "transaction-retained" {
			f.held.Store(true)
		}
	}
	_, e := f.s.publish(c, m)
	if !errors.Is(e, ErrForbidden) || m.sends != 0 {
		t.Fatal("acknowledged build tainted definite refusal", e, m.sends)
	}
	row, ok, e := f.s.record(r.ResultKey)
	if e != nil || !ok || row.Publication == nil || row.Publication.State != "built" || row.Publication.DispatchCount != 0 {
		t.Fatal("known built state lost", e)
	}
	auditCurrent(t, f)
}
func TestResultSyncFaultsDoNotGuessDispatch(t *testing.T) {
	for _, point := range []string{"reservation", "armed", "ack"} {
		t.Run(point, func(t *testing.T) {
			f, c, done, r := preparedFixture(t)
			defer func() { done() }()
			m := fundedPublisher(t, f, r)
			fail := func() {
				f.s.writeBatch = func(*leveldb.Batch, *opt.WriteOptions) error { return errors.New("test uncertain sync") }
			}
			if point == "reservation" {
				fail()
			}
			f.s.checkpoint = func(stage string) {
				if point == "armed" && stage == "transaction-retained" || point == "ack" && stage == "dispatch-acknowledged" {
					fail()
				}
			}
			_, e := f.s.publish(c, m)
			if !errors.Is(e, ErrOutcomeUnknown) || !f.s.poisoned {
				t.Fatal("sync error", e)
			}
			want := 0
			if point == "ack" {
				want = 1
			}
			if m.sends != want {
				t.Fatal("unexpected send", m.sends, want)
			}
			f.s.writeBatch = f.s.db.Write
			// Full Open, rather than an in-memory guess, reconciles the possible write.
			done()
			f.s.Close()
			var err error
			f.s, err = open(context.Background(), filepath.Join(f.dir, "result"), f.pin, f.bout, f.clock, false)
			if err != nil {
				t.Fatal("reopen uncertain journal", err)
			}
			c, done = fixtureCall(t, f)
			auditCurrent(t, f)
		})
	}
}
func TestResultArmedEffectRevocationUnknown(t *testing.T) {
	for _, which := range []string{"hold", "logout_or_reset", "cancel", "close"} {
		t.Run(which, func(t *testing.T) {
			f, c, done, r := preparedFixture(t)
			m := fundedPublisher(t, f, r)
			defer func() { done() }()
			ctx, cancel := context.WithCancel(c.ctx)
			defer cancel()
			c.ctx = ctx
			var closed chan error
			f.s.checkpoint = func(stage string) {
				if stage != "dispatch-armed" {
					return
				}
				switch which {
				case "hold":
					f.held.Store(true)
				case "logout_or_reset":
					f.revoked.Store(true)
				case "cancel":
					cancel()
				case "close":
					closed = make(chan error, 1)
					go func() { closed <- f.s.Close() }()
					<-f.s.life.Done()
				}
			}
			_, e := f.s.publish(c, m)
			if !errors.Is(e, ErrOutcomeUnknown) {
				t.Fatal("armed exit must remain unknown", e)
			}
			if which == "cancel" || which == "close" {
				if m.sends != 0 {
					t.Fatal("cancelled before transport sent")
				}
			} else if m.sends != 1 {
				t.Fatal("armed dispatch did not retain honest possible send", m.sends)
			}
			if closed != nil {
				done()
				done = func() {}
				if e = <-closed; e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
func TestResultDisputedStructuralEvidenceDoesNotReplaceSource(t *testing.T) {
	f, c, done, r := preparedFixture(t)
	done()
	var evidence SignedStatement
	raw, _ := json.Marshal(r.Statement)
	json.Unmarshal(raw, &evidence)
	s := &evidence.Statement.Source
	cond := &s.Source.Corners[0].Member.ConditionAtCutoff
	cond.AgedAttributesTenths.Power++
	cond.LeagueAttributesTenths.Power++
	cond.RatingHundredths += 10
	cb, _ := json.Marshal(s.Source)
	s.SourceDigest = hash("SLUG candidate booked bout source\x00v1\x00", lp(cb))
	s.RecordDigest = strings.Repeat("f", 64)
	evidence.Signature = hex.EncodeToString(ed25519.Sign(fixtureKey(false), statementMessage(evidence.Statement)))
	if validateSignedStatement(f.pin, evidence) == nil {
		t.Fatal("disputed condition became local source")
	}
	got, e := f.s.ObserveEquivocation(context.Background(), f.bout, f.card, f.seed, f.event, f.entry, f.session, c.date, c.id, evidence)
	if e != nil || got.Record.Equivocation == nil || !equal(got.Record.Statement, r.Statement) || got.Status.Phase != "conflict" {
		t.Fatal("evidence rejected or replaced local source", e)
	}
	if validateRecord(f.pin, got.Record) != nil {
		t.Fatal("retained evidence fails record validation")
	}
	bad := evidence
	bad.Signature = strings.Repeat("0", 128)
	if _, e = f.s.ObserveEquivocation(context.Background(), f.bout, f.card, f.seed, f.event, f.entry, f.session, c.date, c.id, bad); !errors.Is(e, ErrInvalid) {
		t.Fatal("bad evidence signature", e)
	}
	auditCurrent(t, f)
}
