// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"github.com/taytr-llc/slug/gymbout"
)

func (s *OwnedStore) checkView(v gymbout.OwnedResultView) error {
	if v.Version != 1 || v.StoreID != s.meta.Pin.SourceStoreID || !equal(v.Pin, s.meta.Pin.SourcePin) {
		return ErrForbidden
	}
	if v.CreatedAtMS != s.meta.SourceCreatedAtMS {
		return ErrSourceChanged
	}
	return nil
}
