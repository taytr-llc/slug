// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"

	"github.com/taytr-llc/slug/poolshare"
)

// SourceConfig exposes the pinned public source identity to trusted service
// composition. It authenticates the concrete journals under the usual gate;
// callers cannot use it to import balances or authorize settlement.
func (s *Store) SourceConfig(ctx context.Context) (poolshare.Config, error) {
	end, err := s.begin(ctx)
	if err != nil {
		return poolshare.Config{}, err
	}
	defer end()
	if err = s.verify(ctx); err != nil {
		return poolshare.Config{}, err
	}
	return s.state.Source, nil
}
