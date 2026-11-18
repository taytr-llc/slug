// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
)

// WithResultView is a purpose-fixed original-session fence, not spend authority.
func (s *Store) WithResultView(ctx context.Context, session gymstable.Session, cap gymaccount.Capability, fn func(SnapshotSourceInfo, func() error) error) error {
	if ctx == nil || fn == nil || (cap != gymaccount.View && cap != gymaccount.ResultPublish) {
		return ErrInvalid
	}
	return s.withPublicationView(ctx, session, cap, fn)
}
