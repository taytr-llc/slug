// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"github.com/syndtr/goleveldb/leveldb"
)

// fundingProjection applies the owned pending batch to bounded journal facts.
// It is prepared before the final account lease, so the response describes the
// same inventory and reservations that become durable with its record.
type fundingProjection struct {
	pin      Pin
	coins    map[string]FundingCoin
	reserved map[string]bool
	err      error
}

func (p *fundingProjection) Put(k, v []byte) {
	if p.err != nil || len(k) == 0 {
		return
	}
	switch k[0] {
	case 'c':
		var c FundingCoin
		if len(k) != 37 || decode(v, 1024, &c, true) != nil || validateFundingCoin(p.pin, c) != nil || string(k) != string(coinKey('c', c.Outpoint)) {
			p.err = ErrCorrupt
			return
		}
		p.coins[string(k)] = c
		if len(p.coins) > MaxFundingCoins {
			p.err = ErrCapacity
		}
	case 'u':
		var u reservation
		if len(k) != 37 || decode(v, 256, &u, true) != nil || u.Version != 1 || !hashID(u.ResultKey) || !hashID(u.TxID) {
			p.err = ErrCorrupt
			return
		}
		p.reserved[string(k)] = true
		if len(p.reserved) > MaxReservations {
			p.err = ErrCapacity
		}
	}
}
func (p *fundingProjection) Delete(k []byte) {
	if len(k) == 0 {
		return
	}
	if k[0] == 'c' {
		delete(p.coins, string(k))
	}
	// Reservations are permanent, including unknown or rejected dispatches.
	if k[0] == 'u' {
		p.err = ErrCorrupt
	}
}
func (s *Store) projectFunding(ctx context.Context, next metadata, batch *leveldb.Batch) (string, error) {
	if next.LastFundingTip == nil {
		return "unknown", nil
	}
	if !next.FundingComplete {
		return "scanning", nil
	}
	p := fundingProjection{pin: s.meta.Pin, coins: map[string]FundingCoin{}, reserved: map[string]bool{}}
	coins, e := s.funding(ctx)
	if e != nil {
		return "", e
	}
	for _, c := range coins {
		p.coins[string(coinKey('c', c.Outpoint))] = c
	}
	it := s.db.NewIterator(nil, nil)
	for ok := it.Seek([]byte{'u'}); ok && len(it.Key()) > 0 && it.Key()[0] == 'u'; ok = it.Next() {
		if ctx.Err() != nil {
			it.Release()
			return "", ErrCancelled
		}
		p.Put(it.Key(), it.Value())
		if p.err != nil {
			it.Release()
			return "", p.err
		}
	}
	e = it.Error()
	it.Release()
	if e != nil {
		return "", ErrCorrupt
	}
	if batch != nil && batch.Replay(&p) != nil {
		return "", ErrCorrupt
	}
	if p.err != nil {
		return "", p.err
	}
	for _, c := range p.coins {
		if !p.reserved[string(coinKey('u', c.Outpoint))] && mature(c, *next.LastFundingTip) && c.ValueSatoshis >= 1294 {
			return "available", nil
		}
	}
	return "waiting_for_funds", nil
}
