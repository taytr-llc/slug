//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"errors"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// OwnedFixtureCounters contain public fixture-phase evidence only. This type
// and both methods below are absent from ordinary helper/app binaries.
type OwnedFixtureCounters struct {
	Records, Jobs    uint64
	Accepted, Signed uint32
}

func (r *PoolRuntime) OwnedFixtureCountersForTest(ctx context.Context, expected OwnedSourceConfig) (OwnedFixtureCounters, error) {
	if r == nil || ctx == nil || ValidateOwnedGymSource(expected) != nil {
		return OwnedFixtureCounters{}, ErrPoolRuntime
	}
	work, done, e := r.begin(ctx)
	if e != nil {
		return OwnedFixtureCounters{}, e
	}
	defer done()
	if r.owned == nil || r.owned.config != expected || r.issuer == nil {
		return OwnedFixtureCounters{}, ErrPoolRuntime
	}
	before, e := r.issuer.Info()
	if e != nil {
		return OwnedFixtureCounters{}, e
	}
	p, e := r.owned.publisher.Status(work)
	if e != nil {
		return OwnedFixtureCounters{}, e
	}
	after, e := r.issuer.Info()
	if e != nil {
		return OwnedFixtureCounters{}, e
	}
	if before.Records != after.Records || before.Jobs != after.Jobs || before.Pin != after.Pin || after.Pin != expected.Ownership.Source {
		return OwnedFixtureCounters{}, fighterownership.ErrStale
	}
	return OwnedFixtureCounters{after.Records, after.Jobs, p.AcceptedSequence, p.SignedAdmissionSequence}, nil
}

// PublishOwnedCheckpointForTest receives public signed bytes, not a signer or
// a private-key callback. The original retained source validates the signature;
// the concrete node must corroborate the actual inclusion header and unchanged
// observation. It creates no transfer, source assignment or finality flag.
func (r *PoolRuntime) PublishOwnedCheckpointForTest(ctx context.Context, expected OwnedSourceConfig, checkpoint chaincfg.SignedCheckpoint) (registry.PublicPublicationEntry, error) {
	if r == nil || ctx == nil || ValidateOwnedGymSource(expected) != nil || checkpoint.Height < 0 || len(checkpoint.Signature) != 64 || uint32(checkpoint.Network) != expected.Ownership.Source.Network || checkpoint.GenesisHash.String() != expected.Ownership.Source.Genesis {
		return registry.PublicPublicationEntry{}, ErrPoolRuntime
	}
	checkpoint.Signature = append([]byte(nil), checkpoint.Signature...)
	work, done, e := r.begin(ctx)
	if e != nil {
		return registry.PublicPublicationEntry{}, e
	}
	defer done()
	o := r.owned
	if o == nil || o.config != expected || o.store == nil || o.node == nil || o.publisher == nil {
		return registry.PublicPublicationEntry{}, ErrPoolRuntime
	}
	observed, e := o.node.Observe(work)
	if e != nil {
		return registry.PublicPublicationEntry{}, e
	}
	if uint32(checkpoint.Height) > observed.Height {
		return registry.PublicPublicationEntry{}, fighterownership.ErrUnavailable
	}
	h, e := o.node.HeaderAt(work, uint32(checkpoint.Height))
	if e != nil {
		return registry.PublicPublicationEntry{}, e
	}
	var header wire.BlockHeader
	if h.Hash != checkpoint.Hash.String() || header.Deserialize(bytes.NewReader(h.Raw)) != nil {
		return registry.PublicPublicationEntry{}, fighterownership.ErrConflict
	}
	hash, e := prospect.TryHashHeader(work, header)
	if e != nil {
		return registry.PublicPublicationEntry{}, e
	}
	if hash != checkpoint.Hash {
		return registry.PublicPublicationEntry{}, fighterownership.ErrConflict
	}
	if e = o.node.Check(work, observed); e != nil {
		return registry.PublicPublicationEntry{}, e
	}
	entry, e := o.publisher.PublishCheckpoint(work, checkpoint)
	if e != nil {
		return registry.PublicPublicationEntry{}, e
	}
	if e = o.node.Check(work, observed); e != nil {
		return registry.PublicPublicationEntry{}, errors.Join(fighterownership.ErrOutcomeUnknown, e)
	}
	return entry, nil
}
