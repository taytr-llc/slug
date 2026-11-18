// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymseed"
)

func mapped(e error) error {
	switch {
	case e == nil:
		return nil
	case errors.Is(e, gymseed.ErrOutcomeUnknown):
		return ErrOutcomeUnknown
	case errors.Is(e, gymseed.ErrSourceIncomplete), errors.Is(e, gymseed.ErrNotFound):
		return ErrSourceIncomplete
	case errors.Is(e, gymseed.ErrConflict):
		return ErrSourceConflict
	case errors.Is(e, gymseed.ErrUnauthorized):
		return ErrUnauthorized
	case errors.Is(e, gymseed.ErrForbidden):
		return ErrForbidden
	case errors.Is(e, gymseed.ErrStaleView):
		return ErrStaleView
	case errors.Is(e, gymseed.ErrClock):
		return ErrClock
	case errors.Is(e, gymseed.ErrCalendar):
		return ErrCalendar
	case errors.Is(e, gymseed.ErrCorrupt):
		return ErrCorrupt
	case errors.Is(e, gymseed.ErrCapacity):
		return ErrCapacity
	case errors.Is(e, gymseed.ErrInvalid):
		return ErrInvalid
	case errors.Is(e, gymseed.ErrCancelled), errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded):
		return ErrCancelled
	case errors.Is(e, gymseed.ErrUnavailable), errors.Is(e, gymseed.ErrClosed), errors.Is(e, gymseed.ErrPoisoned):
		return ErrUnavailable
	}
	return e
}
func (s *Store) checkView(v gymseed.CardView) error {
	if v.Version != 1 || v.StoreID != s.meta.Pin.SourceStoreID || !equal(v.Pin, s.meta.Pin.SourcePin) {
		return ErrForbidden
	}
	if v.CreatedAtMS != s.meta.SourceCreatedAtMS {
		return ErrSourceChanged
	}
	return nil
}
