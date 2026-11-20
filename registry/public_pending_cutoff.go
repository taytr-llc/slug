// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import "context"

// PendingCutoffPublication reads only this owner's exact retained cutoff
// publication intent. Nil means no applicable cutoff, not an empty queue.
// No new body or timestamp is staged and other pending kinds remain untouched.
func (p *ManifestPublication) PendingCutoffPublication(ctx context.Context) (*PreparedPublicPublication, error) {
	ctx, done, e := p.acquire(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	pending, e := p.readPending()
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if pending == nil || pending.Kind != "cutoff" {
		return nil, nil
	}
	return &PreparedPublicPublication{owner: p, intent: *pending}, nil
}

// CommitCutoffSigned completes this exact original pending reference with an
// already signed cutoff. The common commit checks its ID/signature, the exact
// original boundary intent, and the pending bytes/head in one sync CAS. This
// method cannot stage another publication or sign a cutoff/publication itself.
func (v *PreparedPublicPublication) CommitCutoffSigned(ctx context.Context, entry PublicPublicationEntry, cutoff PublicCutoffRecord) (PublicPublicationEntry, error) {
	if v != nil && v.owner != nil && v.owner.codec.operator {
		return PublicPublicationEntry{}, ErrPublicOwnership
	}
	if v == nil || v.owner == nil || v.intent.Kind != "cutoff" {
		return PublicPublicationEntry{}, ErrPublicOwnership
	}
	ctx, done, e := v.owner.acquire(ctx)
	if e != nil {
		return PublicPublicationEntry{}, e
	}
	defer done()
	return v.owner.commitSigned(ctx, v.intent, nil, entry, &cutoff)
}
