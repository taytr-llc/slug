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
