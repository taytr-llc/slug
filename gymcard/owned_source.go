// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"github.com/taytr-llc/slug/gymseed"
)

func (s *OwnedStore) checkView(v gymseed.OwnedCardView) error {
	if v.Version != 1 || v.StoreID != s.meta.Pin.SourceStoreID || !equal(v.Pin, s.meta.Pin.SourcePin) {
		return ErrForbidden
	}
	if v.CreatedAtMS != s.meta.SourceCreatedAtMS {
		return ErrSourceChanged
	}
	return nil
}
