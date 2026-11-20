// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/seednode"
)

// publicationBinding takes expected pins only from the original configured
// league and actual runtime owners. The immutable files corroborate this tuple
// inside seednode; they never supply its authority or process identity.
func (l *OwnedLeague) publicationBinding(ctx context.Context) (seednode.OwnershipPoolOwnerBinding, error) {
	if l == nil || ctx == nil || ctx.Err() != nil || l.life.Err() != nil || l.ready == nil {
		return seednode.OwnershipPoolOwnerBinding{}, gymseed.ErrUnavailable
	}
	r := l.runtime
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.life.Err() != nil || r.owned == nil || r.owned.store != l.source || r.owned.config != l.config.Source || r.source != l.config.PoolSource || r.nodes == nil || r.nodes.primary == nil || r.nodes.peer == nil || r.accounts == nil || r.issuer == nil {
		return seednode.OwnershipPoolOwnerBinding{}, gymseed.ErrUnavailable
	}
	intent := ownedLeagueIntent{Format: "slug-owned-league", Version: 1, Config: l.Config()}
	return seednode.OwnershipPoolOwnerBinding{Source: l.Config().EventPin, OwnershipPin: l.config.Source.Ownership, OwnershipStoreID: l.config.Source.Store.StoreID, EventStoreID: l.config.EventStoreID, EventCreatedAtMS: l.ready.Prepared.Stages[4].CreatedAtMS, Pool: l.config.PoolSource, PoolBindingID: l.config.SourceBindingID, Accounts: r.accounts, Issuer: r.issuer, Ownership: l.source, Events: l.events, Home: r.config.DataDir, Primary: seednode.OwnedProcess{Cmd: r.nodes.primary.cmd, Done: r.nodes.primary.done}, Peer: seednode.OwnedProcess{Cmd: r.nodes.peer.cmd, Done: r.nodes.peer.done}, Life: r.life, RuntimeMarkerSHA256: poolCanonicalHash(r.marker), LeagueIntentSHA256: poolCanonicalHash(intent), LeagueReadySHA256: poolCanonicalHash(*l.ready)}, nil
}
func (l *OwnedLeague) seedPublisher(ctx context.Context) (*seednode.Publisher, error) {
	binding, e := l.publicationBinding(ctx)
	if e != nil {
		return nil, e
	}
	return seednode.AttachOwnershipPool(ctx, binding)
}
func (l *OwnedLeague) resultPublisher(ctx context.Context) (*seednode.ResultPublisher, error) {
	binding, e := l.publicationBinding(ctx)
	if e != nil {
		return nil, e
	}
	return seednode.AttachOwnershipPoolResult(ctx, binding)
}
