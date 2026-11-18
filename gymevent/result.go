// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *Store) WithResultView(ctx context.Context, entries *gymentry.Store, session gymstable.Session, cap gymaccount.Capability, fn func(SeedSourceInfo, func() error) error) error {
	if ctx == nil || entries == nil || fn == nil || (cap != gymaccount.View && cap != gymaccount.ResultPublish) {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	return s.publicationView(work, entries, session, cap, true, fn)
}
