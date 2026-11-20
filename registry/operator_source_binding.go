// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"context"
	"github.com/taytr-llc/slug/internal/operatorauthority"
)

// CheckOperatorOwner discloses no signer and attaches nothing. The concrete
// core checks this exact pointer before admitting local signing work.
func (p *ManifestPublication) CheckOperatorOwner(ctx context.Context, owner *operatorauthority.Owner) error {
	if p == nil || !p.codec.operator || owner == nil || p.operator != owner {
		return ErrPublicOwnership
	}
	if e := p.Alive(); e != nil {
		return e
	}
	return owner.Check(ctx, p.binding)
}
