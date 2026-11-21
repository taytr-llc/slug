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

// SchedulerBinding audits the original writable settlement owner and returns
// owned copies of its immutable configuration. It is a logical binding only,
// not a unique database identity, replacement-store permission or payment
// capability. The operator runtime retains the original path and Store pointer.
// Read-only audit handles cannot be attached to a mutation scheduler.
func (s *Store) SchedulerBinding(ctx context.Context) (Config, poolshare.Config, error) {
	if s != nil && s.daily != nil {
		return Config{}, poolshare.Config{}, ErrConfig
	}
	if s != nil && s.auditOnly {
		return Config{}, poolshare.Config{}, ErrInvalid
	}
	end, err := s.begin(ctx)
	if err != nil {
		return Config{}, poolshare.Config{}, err
	}
	defer end()
	if err = s.verify(ctx); err != nil {
		return Config{}, poolshare.Config{}, err
	}
	return clone(s.state.Config), s.state.Source, nil
}

// DailySchedulerBinding selects the original writable Config2 owner. It audits
// the full current source/indexed ledger and returns owned configuration only.
func (s *Store) DailySchedulerBinding(ctx context.Context) (Config, poolshare.Config, error) {
	if s == nil || s.daily == nil || s.auditOnly {
		return Config{}, poolshare.Config{}, ErrConfig
	}
	end, err := s.begin(ctx)
	if err != nil {
		return Config{}, poolshare.Config{}, err
	}
	defer end()
	if err = s.verifyDaily(ctx); err != nil {
		return Config{}, poolshare.Config{}, err
	}
	c := s.state.Config
	c.Destinations = append([]Destination(nil), c.Destinations...)
	return c, s.state.Source, ctx.Err()
}
