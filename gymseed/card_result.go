// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *Store) WithCardPreparedResultView(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, capability gymaccount.Capability, prepare func(Info) ([]string, func(CardView, func() error) error, error)) error {
	if capability != gymaccount.View && capability != gymaccount.ResultPublish {
		return ErrInvalid
	}
	return s.withCardPreparedPurpose(ctx, events, entries, session, capability, true, prepare)
}
