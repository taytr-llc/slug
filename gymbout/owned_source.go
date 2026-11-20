// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"github.com/taytr-llc/slug/gymcard"
)

func (s *OwnedStore) checkView(v gymcard.OwnedBoutView) error {
	if v.Version != 1 || v.StoreID != s.meta.Pin.SourceStoreID || !equal(v.Pin, s.meta.Pin.SourcePin) {
		return ErrForbidden
	}
	if v.CreatedAtMS != s.meta.SourceCreatedAtMS {
		return ErrSourceChanged
	}
	return nil
}
