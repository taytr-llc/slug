// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *OwnedStore) WithCardPreparedResultView(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, capability gymaccount.Capability, prepare func(OwnedInfo) ([]string, func(OwnedCardView, func() error) error, error)) error {
	if capability != gymaccount.View && capability != gymaccount.ResultPublish {
		return ErrInvalid
	}
	return s.withCardPreparedPurpose(ctx, events, entries, session, capability, true, prepare)
}
