// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/seednode"
)

func mapped(e error) error {
	switch {
	case e == nil:
		return nil
	case errors.Is(e, seednode.ErrOutcomeUnknown):
		return ErrOutcomeUnknown
	case errors.Is(e, seednode.ErrOffline):
		return ErrNodeOffline
	case errors.Is(e, seednode.ErrNotMature):
		return ErrNotMature
	case errors.Is(e, seednode.ErrCapacity):
		return ErrCapacity
	case errors.Is(e, seednode.ErrInvalid):
		return ErrInvalid
	case errors.Is(e, seednode.ErrUnavailable):
		return ErrUnavailable
	case errors.Is(e, gymbout.ErrOutcomeUnknown):
		return ErrOutcomeUnknown
	case errors.Is(e, gymbout.ErrSourceIncomplete), errors.Is(e, gymbout.ErrNotFound):
		return ErrSourceIncomplete
	case errors.Is(e, gymbout.ErrSourceChanged):
		return ErrSourceChanged
	case errors.Is(e, gymbout.ErrSourceConflict):
		return ErrSourceConflict
	case errors.Is(e, gymbout.ErrUnauthorized):
		return ErrUnauthorized
	case errors.Is(e, gymbout.ErrForbidden):
		return ErrForbidden
	case errors.Is(e, gymbout.ErrStaleView):
		return ErrStaleView
	case errors.Is(e, gymbout.ErrClock):
		return ErrClock
	case errors.Is(e, gymbout.ErrCalendar):
		return ErrCalendar
	case errors.Is(e, gymbout.ErrCorrupt):
		return ErrCorrupt
	case errors.Is(e, gymbout.ErrCapacity):
		return ErrCapacity
	case errors.Is(e, gymbout.ErrInvalid):
		return ErrInvalid
	case errors.Is(e, gymbout.ErrCancelled), errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded):
		return ErrCancelled
	case errors.Is(e, gymbout.ErrUnavailable), errors.Is(e, gymbout.ErrClosed), errors.Is(e, gymbout.ErrPoisoned):
		return ErrUnavailable
	}
	return e
}
func (s *Store) checkView(v gymbout.ResultView) error {
	if v.Version != 1 || v.StoreID != s.meta.Pin.SourceStoreID || !equal(v.Pin, s.meta.Pin.SourcePin) {
		return ErrForbidden
	}
	if v.CreatedAtMS != s.meta.SourceCreatedAtMS {
		return ErrSourceChanged
	}
	return nil
}
