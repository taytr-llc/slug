// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"sort"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

type dailyMutation struct {
	store  *Store
	view   *dailyView
	writes map[string][]byte
}

func newDailyMutation(s *Store, v *dailyView) *dailyMutation {
	return &dailyMutation{store: s, view: v, writes: make(map[string][]byte)}
}
func copyDailyReplay(r *settlementReplay) *settlementReplay {
	own := *r
	own.coins = dailyOwnedCoins(r.coins)
	own.accounting = balanceProjection{gross: make(map[string]*big.Rat), paid: make(map[string]int64), reserved: make(map[string]int64)}
	for k, v := range r.accounting.gross {
		own.accounting.gross[k] = new(big.Rat).Set(v)
	}
	for k, v := range r.accounting.paid {
		own.accounting.paid[k] = v
	}
	for k, v := range r.accounting.reserved {
		own.accounting.reserved[k] = v
	}
	return &own
}
func (m *dailyMutation) fork() *dailyMutation {
	v := *m.view
	v.replay = copyDailyReplay(v.replay)
	writes := make(map[string][]byte, len(m.writes))
	for k, v := range m.writes {
		writes[k] = v
	}
	return &dailyMutation{store: m.store, view: &v, writes: writes}
}
func (m *dailyMutation) add(key, raw []byte) error {
	if _, ok := m.writes[string(key)]; ok {
		return ErrCorrupt
	}
	exists, err := m.store.db.Has(key, nil)
	if err != nil {
		return ErrCorrupt
	}
	if exists {
		return ErrCorrupt
	}
	size := len(raw)
	if strings.HasPrefix(string(key), "bs/") {
		size = dailyReferenceLimit
	}
	amount := uint64(len(key) + size)
	if amount > dailyLogicalLimit-m.view.head.LogicalBytes {
		return ErrCapacity
	}
	m.view.head.LogicalBytes += amount
	m.writes[string(key)] = append([]byte(nil), raw...)
	return nil
}
func (m *dailyMutation) addValue(key []byte, value any, limit int) error {
	raw, err := dailyEncode(value, limit)
	if err != nil {
		return err
	}
	return m.add(key, raw)
}
func (m *dailyMutation) addReference(key []byte, kind string, seq uint64) error {
	return m.addValue(key, dailyReference{Kind: kind, Sequence: seq}, dailyReferenceLimit)
}
func (m *dailyMutation) stageCredits(ctx context.Context, credits []Credit) error {
	if uint64(len(credits)) > uint64(m.store.state.Config.MaxRounds)-m.view.head.Credits {
		return ErrCapacity
	}
	// Work on an owned candidate: a late malformed/capacity/canceled row cannot
	// leak earlier rows into a subsequently committed deferral report.
	next := m.fork()
	v := next.view
	v.replay.expected.Credits += uint64(len(credits))
	for _, c := range credits {
		if err := ctx.Err(); err != nil {
			return err
		}
		seq := v.head.Credits + 1
		if err := v.replay.addCredit(c, replayCreditCheck{Sequence: seq}); err != nil {
			return err
		}
		if len(v.replay.accounting.gross) > 256 {
			return ErrCapacity
		}
		if err := next.addValue(dailyNumber("c/", seq), c, dailyCreditLimit); err != nil {
			return err
		}
		if err := next.add([]byte("cr/"+c.RoundHash), dailySequence(seq)); err != nil {
			return err
		}
		if c.Coin.Atoms != 0 {
			if err := next.addReference(dailyOutpoint("o/", c.Coin.TxID, c.Coin.Index), "credit", seq); err != nil {
				return err
			}
		}
		v.head.Events++
		if err := next.addReference(dailyNumber("e/", v.head.Events), "credit", seq); err != nil {
			return err
		}
		v.head.Credits = seq
	}
	if err := v.replay.complete(); err != nil {
		return err
	}
	*m = *next
	return nil
}
func (m *dailyMutation) stageBatch(ctx context.Context, b Batch) error {
	if m.view.head.Batches >= uint64(m.store.state.Config.MaxBatches) {
		return ErrCapacity
	}
	next := m.fork()
	v := next.view
	seq := v.head.Batches + 1
	body, err := dailyBody(ctx, seq, b)
	if err != nil {
		return err
	}
	v.replay.expected.Batches++
	v.replay.clock = v.head.LastClockMS
	if err = v.replay.applyBatch(b, replayBatchCheck{Sequence: seq}); err != nil {
		return err
	}
	if err = next.addValue(dailyNumber("b/", seq), body, dailyBatchLimit); err != nil {
		return err
	}
	if err = next.addValue(dailyNumber("bs/", seq), dailyDelivery{State: b.State}, dailyReferenceLimit); err != nil {
		return err
	}
	for _, key := range [][]byte{[]byte("bt/" + b.TxID), []byte("bd/" + b.Day)} {
		if err = next.add(key, dailySequence(seq)); err != nil {
			return err
		}
	}
	for _, c := range b.Inputs {
		if err = next.add(dailyOutpoint("spent/", c.TxID, c.Index), dailySequence(seq)); err != nil {
			return err
		}
	}
	if b.Change != nil {
		if err = next.addReference(dailyOutpoint("o/", b.TxID, b.Change.Index), "change", seq); err != nil {
			return err
		}
	}
	v.head.Batches = seq
	v.head.Events++
	v.head.PendingBatch = seq
	v.head.LastDay = b.Day
	v.pending = &b
	if err = next.addReference(dailyNumber("e/", v.head.Events), "batch", seq); err != nil {
		return err
	}
	if err = v.replay.complete(); err != nil {
		return err
	}
	*m = *next
	return nil
}
func (m *dailyMutation) delivery(state string) error {
	v := m.view
	if v.pending == nil || state != "broadcast" && state != "confirmed" {
		return ErrCorrupt
	}
	if state == "confirmed" {
		for _, p := range v.pending.Payments {
			if v.replay.accounting.reserved[p.GymID] < p.Atoms || p.Atoms > math.MaxInt64-v.replay.accounting.paid[p.GymID] {
				return ErrCorrupt
			}
		}
		for _, p := range v.pending.Payments {
			v.replay.accounting.reserved[p.GymID] -= p.Atoms
			v.replay.accounting.paid[p.GymID] += p.Atoms
		}
	}
	raw, _ := json.Marshal(dailyDelivery{State: state})
	m.writes[string(dailyNumber("bs/", v.head.PendingBatch))] = raw
	v.replay.last.State = state
	if state == "confirmed" {
		v.head.PendingBatch = 0
		v.pending = nil
	} else {
		copy := *v.pending
		copy.State = state
		v.pending = &copy
	}
	return nil
}
func (m *dailyMutation) commit(ctx context.Context) error {
	s, v := m.store, m.view
	if s.auditOnly {
		return ErrInvalid
	}
	if s.poisoned {
		return ErrPoisoned
	}
	if v.head.LastClockMS < s.daily.head.LastClockMS || v.head.Revision == math.MaxUint64 {
		return ErrClock
	}
	if err := validateDailyReport(v); err != nil {
		return err
	}
	head := v.head
	head.Revision = s.daily.head.Revision + 1
	if !dailyValidHeader(head, s.daily.marker) {
		return ErrCorrupt
	}
	headRaw, err := dailyEncode(head, dailyHeadLimit)
	if err != nil {
		return err
	}
	reportRaw, err := dailyEncode(v.report, dailyReportLimit)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = s.checkDailyMarker(); err != nil {
		return err
	}
	// Detect a changed head before sync; full current namespace/source auditing
	// happened at admission under this same original operation gate.
	raw, err := s.db.Get([]byte("head"), nil)
	var old dailyHeader
	if err != nil || dailyDecode(raw, dailyHeadLimit, &old) != nil || old != s.daily.head {
		return ErrCorrupt
	}
	batch := new(leveldb.Batch)
	keys := make([]string, 0, len(m.writes))
	for k := range m.writes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		batch.Put([]byte(k), m.writes[k])
	}
	batch.Put([]byte("head"), headRaw)
	batch.Put([]byte("report"), reportRaw)
	if err = ctx.Err(); err != nil {
		return err
	}
	if s.writeBatch(batch, &opt.WriteOptions{Sync: true}) != nil {
		s.poisoned = true
		return ErrPoisoned
	}
	v.head = head
	s.daily.head = head
	s.daily.report = clone(v.report)
	// The capability reads construction-immutable Config/Source. Do not replace
	// the whole state struct while an admitted signer may check those fields.
	s.state.Report = clone(v.report)
	s.state.LastClockMS = head.LastClockMS
	m.writes = make(map[string][]byte)
	return nil
}
