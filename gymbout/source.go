// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymcard"
)

func mapped(e error) error {
	switch {
	case e == nil:
		return nil
	case errors.Is(e, gymcard.ErrOutcomeUnknown):
		return ErrOutcomeUnknown
	case errors.Is(e, gymcard.ErrSourceIncomplete), errors.Is(e, gymcard.ErrNotFound):
		return ErrSourceIncomplete
	case errors.Is(e, gymcard.ErrSourceChanged):
		return ErrSourceChanged
	case errors.Is(e, gymcard.ErrSourceConflict):
		return ErrSourceConflict
	case errors.Is(e, gymcard.ErrUnauthorized):
		return ErrUnauthorized
	case errors.Is(e, gymcard.ErrForbidden):
		return ErrForbidden
	case errors.Is(e, gymcard.ErrStaleView):
		return ErrStaleView
	case errors.Is(e, gymcard.ErrClock):
		return ErrClock
	case errors.Is(e, gymcard.ErrCalendar):
		return ErrCalendar
	case errors.Is(e, gymcard.ErrCorrupt):
		return ErrCorrupt
	case errors.Is(e, gymcard.ErrCapacity):
		return ErrCapacity
	case errors.Is(e, gymcard.ErrInvalid):
		return ErrInvalid
	case errors.Is(e, gymcard.ErrCancelled), errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded):
		return ErrCancelled
	case errors.Is(e, gymcard.ErrUnavailable), errors.Is(e, gymcard.ErrClosed), errors.Is(e, gymcard.ErrPoisoned):
		return ErrUnavailable
	}
	return e
}
func (s *Store) checkView(v gymcard.BoutView) error {
	if v.Version != 1 || v.StoreID != s.meta.Pin.SourceStoreID || !equal(v.Pin, s.meta.Pin.SourcePin) {
		return ErrForbidden
	}
	if v.CreatedAtMS != s.meta.SourceCreatedAtMS {
		return ErrSourceChanged
	}
	return nil
}
