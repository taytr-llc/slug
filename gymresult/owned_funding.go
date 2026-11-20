// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"github.com/syndtr/goleveldb/leveldb"
)

// fundingProjection applies the ownedValue pending batch to bounded journal facts.
// It is prepared before the final account lease, so the response describes the
// same inventory and reservations that become durable with its record.
func (s *OwnedStore) projectFunding(ctx context.Context, next ownedMetadata, batch *leveldb.Batch) (string, error) {
	if next.LastFundingTip == nil {
		return "unknown", nil
	}
	if !next.FundingComplete {
		return "scanning", nil
	}
	p := fundingProjection{pin: physicalOwnedPin(s.meta.Pin), coins: map[string]FundingCoin{}, reserved: map[string]bool{}}
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
