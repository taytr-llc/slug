// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"

	"github.com/taytr-llc/slug/poolshare"
)

// SettlementSnapshot is an operator-only bounded export from the concrete
// accountant. It is not an import format or miner endpoint. Every call verifies
// the consumed acceptance prefix; returned values own their memory.
type SettlementSnapshot struct {
	Source  poolshare.Config `json:"source"`
	Records []Record         `json:"records"`
}

// WalkSettlement verifies the complete consumed source prefix and stored
// namespaces before visiting any records, then delivers ascending owned pages
// of at most MaxPage records. A nil visitor verifies only the source identity.
// This is an operator-only read from the concrete original accountant, not a
// receipt import or public transport. It observes no node and writes no state.
//
// The original store gate remains held, fixing observations across all pages;
// the visitor must not reenter or close this store. Callers stage their effects
// until successful return: cancellation, a visitor error or a later read error
// returns a zero Config, never an authenticated partial settlement snapshot.
func (s *Store) WalkSettlement(ctx context.Context, visit func([]Record) error) (poolshare.Config, error) {
	end, err := s.begin(ctx)
	if err != nil {
		return poolshare.Config{}, err
	}
	defer end()
	if err := s.verifyStream(ctx); err != nil {
		return poolshare.Config{}, err
	}
	if err := s.walkRecords(ctx, visit); err != nil {
		return poolshare.Config{}, err
	}
	return s.sourceConfig, nil
}

func (s *Store) SettlementSnapshot(ctx context.Context) (SettlementSnapshot, error) {
	end, err := s.begin(ctx)
	if err != nil {
		return SettlementSnapshot{}, err
	}
	defer end()
	records, err := s.verifyStored(ctx)
	if err != nil {
		return SettlementSnapshot{}, err
	}
	return SettlementSnapshot{Source: s.sourceConfig, Records: records}, ctx.Err()
}
