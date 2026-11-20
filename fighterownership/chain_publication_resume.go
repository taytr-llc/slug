// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"

	"github.com/taytr-llc/slug/registry"
)

// ResumeChainPublication completes only the original publisher's retained
// unsigned chain intent. The old point must still belong to a freshly observed
// canonical chain from the same concrete node. This is a dated publication,
// never current Head readiness, a noon bracket, or a verified ownership map.
// false means no chain was resumed; other pending kinds are not consumed.
func (s *Store) ResumeChainPublication(ctx context.Context, source *PublicSource, node *OwnedChainSource) (*PublicSource, bool, error) {
	if s == nil || source == nil || source.publisher == nil || node == nil || source.pin != s.pin || node.pin != s.pin {
		return nil, false, ErrInvalid
	}
	ctx, done, e := s.mutation(ctx)
	if e != nil {
		return nil, false, e
	}
	defer done()
	return s.resumeChainPublication(ctx, source, node)
}

// Caller holds the core mutation permit. Registry permits are short and never
// held across node I/O or signing; Check/CommitSigned retain exact CAS ownership.
func (s *Store) resumeChainPublication(ctx context.Context, source *PublicSource, node *OwnedChainSource) (*PublicSource, bool, error) {
	if e := source.Check(ctx); e != nil {
		return nil, false, e
	}
	prepared, e := source.publisher.PendingChainPublication(ctx)
	if e != nil {
		return nil, false, e
	}
	if prepared == nil {
		return source, false, nil
	}
	entry := prepared.Entry()
	var reference registry.PublicChainReference
	if entry.Kind != "chain" || json.Unmarshal(entry.Payload, &reference) != nil {
		return nil, false, ErrCorrupt
	}
	if reference.Height >= s.pin.Limits.Headers {
		return nil, false, ErrCapacity
	}
	previous, e := source.publisher.ChainHeadID(ctx)
	if e != nil {
		return nil, false, e
	}
	if reference.PreviousChainEntryID != previous {
		return nil, false, ErrConflict
	}
	observation, e := node.Observe(ctx)
	if e != nil {
		return nil, false, e
	}
	if reference.Height > observation.Height {
		return nil, false, ErrUnavailable
	}
	header, e := node.HeaderAt(ctx, reference.Height)
	if e != nil {
		return nil, false, e
	}
	if header.Height != reference.Height || header.Hash != reference.Hash {
		return nil, false, ErrStale
	}
	if e = node.Check(ctx, observation); e != nil {
		return nil, false, e
	}
	if e = source.Check(ctx); e != nil {
		return nil, false, e
	}
	if e = prepared.Check(ctx); e != nil {
		return nil, false, e
	}
	entry.Signature, e = signObject("SLUG fighter ownership publication statement", entry)
	if e != nil {
		return nil, false, e
	}
	if e = node.Check(ctx, observation); e != nil {
		return nil, false, e
	}
	if e = source.Check(ctx); e != nil {
		return nil, false, e
	}
	if _, e = prepared.CommitSigned(ctx, entry); e != nil {
		return nil, false, e
	}
	next, e := NewPublicSource(ctx, s.pin, source.publisher)
	return next, true, e
}
