// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"reflect"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/poolround"
)

type dailyView struct {
	head    dailyHeader
	report  Report
	replay  *settlementReplay
	pending *Batch
}
type dailyReader struct {
	ctx  context.Context
	db   *leveldb.Snapshot
	keys map[string]bool
}

func (r *dailyReader) get(key []byte, limit int, out any) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if r.keys[string(key)] {
		return ErrCorrupt
	}
	raw, err := r.db.Get(key, nil)
	if err != nil {
		return ErrCorrupt
	}
	if dailyDecode(raw, limit, out) != nil {
		return ErrCorrupt
	}
	r.keys[string(key)] = true
	return nil
}
func (r *dailyReader) exact(key, want []byte) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if r.keys[string(key)] {
		return ErrCorrupt
	}
	raw, err := r.db.Get(key, nil)
	if err != nil || !bytes.Equal(raw, want) {
		return ErrCorrupt
	}
	r.keys[string(key)] = true
	return nil
}
func (r *dailyReader) reference(key []byte, kind string, seq uint64) error {
	raw, _ := json.Marshal(dailyReference{Kind: kind, Sequence: seq})
	return r.exact(key, raw)
}
func (s *Store) auditDaily(ctx context.Context) (*dailyView, error) {
	if s.daily == nil || ctx == nil {
		return nil, ErrInvalid
	}
	if err := s.checkDailyMarker(); err != nil {
		return nil, err
	}
	snapshot, err := s.db.GetSnapshot()
	if err != nil {
		return nil, ErrCorrupt
	}
	defer snapshot.Release()
	v, err := s.auditDailyRows(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	if _, err = s.dailySource(ctx, snapshot, v.head, false); err != nil {
		return nil, err
	}
	return v, ctx.Err()
}

// auditDailyRows is conditional storage replay only. The public owner path
// always follows it with dailySource before any disclosure or mutation.
func (s *Store) auditDailyRows(ctx context.Context, snapshot *leveldb.Snapshot) (*dailyView, error) {
	var err error
	r := dailyReader{ctx: ctx, db: snapshot, keys: make(map[string]bool)}
	v := &dailyView{}
	if err = r.get([]byte("head"), dailyHeadLimit, &v.head); err != nil {
		return nil, err
	}
	if !dailyValidHeader(v.head, s.daily.marker) || v.head != s.daily.head {
		return nil, ErrCorrupt
	}
	if err = r.get([]byte("report"), dailyReportLimit, &v.report); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(v.report, s.daily.report) {
		return nil, ErrCorrupt
	}
	v.replay, err = newDailySettlementReplay(s.state.Config, v.head.LastClockMS, replayCounts{Credits: v.head.Credits, Batches: v.head.Batches})
	if err != nil {
		return nil, err
	}
	gyms := make(map[string]bool)
	for _, d := range s.state.Config.Destinations {
		gyms[d.GymID] = true
	}
	for event := uint64(1); event <= v.head.Events; event++ {
		var ref dailyReference
		if err = r.get(dailyNumber("e/", event), dailyReferenceLimit, &ref); err != nil {
			return nil, err
		}
		switch ref.Kind {
		case "credit":
			if ref.Sequence != v.replay.seen.Credits+1 {
				return nil, ErrCorrupt
			}
			var c Credit
			if err = r.get(dailyNumber("c/", ref.Sequence), dailyCreditLimit, &c); err != nil {
				return nil, err
			}
			if err = r.exact([]byte("cr/"+c.RoundHash), dailySequence(ref.Sequence)); err != nil {
				return nil, err
			}
			if c.Coin.Atoms != 0 {
				if err = r.reference(dailyOutpoint("o/", c.Coin.TxID, c.Coin.Index), "credit", ref.Sequence); err != nil {
					return nil, err
				}
			}
			for _, claim := range c.Coin.Claims {
				if claim.GymID != "" {
					gyms[claim.GymID] = true
				}
			}
			if len(gyms) > 256 {
				return nil, ErrCorrupt
			}
			if err = v.replay.addCredit(c, replayCreditCheck{Sequence: ref.Sequence}); err != nil {
				return nil, err
			}
		case "batch":
			var body dailyBatchBody
			var delivery dailyDelivery
			if ref.Sequence != v.replay.seen.Batches+1 {
				return nil, ErrCorrupt
			}
			if err = r.get(dailyNumber("b/", ref.Sequence), dailyBatchLimit, &body); err != nil {
				return nil, err
			}
			if body.Sequence != ref.Sequence {
				return nil, ErrCorrupt
			}
			if err = r.get(dailyNumber("bs/", ref.Sequence), dailyReferenceLimit, &delivery); err != nil {
				return nil, err
			}
			b, e := dailyReconstruct(ctx, v.replay, body, delivery)
			if e != nil {
				return nil, e
			}
			for _, key := range [][]byte{[]byte("bt/" + b.TxID), []byte("bd/" + b.Day)} {
				if err = r.exact(key, dailySequence(ref.Sequence)); err != nil {
					return nil, err
				}
			}
			for _, c := range b.Inputs {
				if err = r.exact(dailyOutpoint("spent/", c.TxID, c.Index), dailySequence(ref.Sequence)); err != nil {
					return nil, err
				}
			}
			if b.Change != nil {
				if err = r.reference(dailyOutpoint("o/", b.TxID, b.Change.Index), "change", ref.Sequence); err != nil {
					return nil, err
				}
			}
			if err = v.replay.applyBatch(b, replayBatchCheck{Sequence: ref.Sequence}); err != nil {
				return nil, err
			}
			if b.State != "confirmed" {
				v.pending = &b
			}
		default:
			return nil, ErrCorrupt
		}
	}
	if err = v.replay.complete(); err != nil {
		return nil, err
	}
	if v.head.LastDay != v.replay.last.Day || (v.pending == nil) != (v.head.PendingBatch == 0) {
		return nil, ErrCorrupt
	}
	if err = validateDailyReport(v); err != nil {
		return nil, err
	}
	// The complete namespace is checked, including keys no event or index ever
	// requested. Mutable delivery values reserve their maximum bytes from birth.
	logical := uint64(dailyHeadLimit + dailyReportLimit)
	count := 0
	it := snapshot.NewIterator(nil, nil)
	for it.Next() {
		if err = ctx.Err(); err != nil {
			break
		}
		key := it.Key()
		count++
		if !r.keys[string(key)] {
			err = ErrCorrupt
			break
		}
		if bytes.Equal(key, []byte("head")) || bytes.Equal(key, []byte("report")) {
			continue
		}
		n := len(it.Value())
		if bytes.HasPrefix(key, []byte("bs/")) {
			n = dailyReferenceLimit
		}
		add := uint64(len(key) + n)
		if add > dailyLogicalLimit-logical {
			err = ErrCorrupt
			break
		}
		logical += add
	}
	iteratorErr := it.Error()
	it.Release()
	if err != nil {
		return nil, err
	}
	if iteratorErr != nil || count != len(r.keys) || logical != v.head.LogicalBytes {
		return nil, ErrCorrupt
	}
	return v, ctx.Err()
}
func (s *Store) verifyDaily(ctx context.Context) error { _, err := s.auditDaily(ctx); return err }

// dailySource never mutates state. A full original walk must check every old
// credit before its owned additional records can be staged by a caller.
func (s *Store) dailySource(ctx context.Context, db *leveldb.Snapshot, head dailyHeader, importNew bool) ([]Credit, error) {
	checked := make(map[uint64]bool)
	additional := []Credit{}
	config, err := s.source.WalkSettlement(ctx, func(page []poolround.Record) error {
		for _, r := range page {
			if err := ctx.Err(); err != nil {
				return err
			}
			if r.Confirmation == nil {
				continue
			}
			c, e := deriveCredit(r)
			if e != nil || c.Coin.Script != s.state.Config.ChangeScript {
				return ErrSource
			}
			index, e := db.Get([]byte("cr/"+c.RoundHash), nil)
			if e == leveldb.ErrNotFound {
				if importNew {
					additional = append(additional, c)
				}
				continue
			}
			if e != nil || len(index) != 8 {
				return ErrSource
			}
			seq := dailyReadSequence(index)
			if seq == 0 || seq > head.Credits || checked[seq] {
				return ErrSource
			}
			raw, e := db.Get(dailyNumber("c/", seq), nil)
			var old Credit
			if e != nil || dailyDecode(raw, dailyCreditLimit, &old) != nil || !reflect.DeepEqual(c, old) {
				return ErrSource
			}
			checked[seq] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if config != s.state.Source || uint64(len(checked)) != head.Credits {
		return nil, ErrSource
	}
	return additional, ctx.Err()
}
func validateDailyReport(v *dailyView) error {
	r, h := v.report, v.head
	if h.LastClockMS == 0 {
		if h.Credits != 0 || h.Batches != 0 || !reflect.DeepEqual(r, Report{}) {
			return ErrCorrupt
		}
		return nil
	}
	if r.ObservedAtMS <= 0 || r.ObservedAtMS > h.LastClockMS || !knownReason(r.Reason) {
		return ErrCorrupt
	}
	d, e := time.Parse("2006-01-02", r.Day)
	if e != nil || d.Format("2006-01-02") != r.Day || r.Day > time.UnixMilli(r.ObservedAtMS).UTC().Format("2006-01-02") || r.Day < v.replay.last.Day || r.TxID != v.replay.last.TxID {
		return ErrCorrupt
	}
	if r.State != "pending" && r.State != "complete" && r.State != "deferred" {
		return ErrCorrupt
	}
	want, e := v.replay.balances(nil, "", nil)
	if e != nil || len(want) != len(r.Balances) || len(want) > 256 {
		return ErrCorrupt
	}
	for i, b := range r.Balances {
		w := want[i]
		if b.GymID != w.GymID || b.Confirmed != w.Confirmed || b.PaidAtoms != w.PaidAtoms || b.ReservedAtoms != w.ReservedAtoms || !knownReason(b.Reason) {
			return ErrCorrupt
		}
		a, e := rat(b.Available)
		if e != nil {
			return ErrCorrupt
		}
		c, e := rat(b.Confirmed)
		if e != nil || a.Cmp(new(big.Rat).Sub(c, new(big.Rat).SetInt64(b.ReservedAtoms))) > 0 || c.Sign() == 0 && b.Reason != "" {
			return ErrCorrupt
		}
	}
	return nil
}
