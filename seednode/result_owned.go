// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"context"
	"encoding/hex"
)

// ResultIdentity binds the separate result purpose to the same owned chain.
// Its fixed public fixture funding key is never caller-selectable.
type ResultIdentity struct {
	Chain            ChainIdentity
	Purpose          string
	FundingPublicKey string
	FundingScript    string
}

// ResultPublisher borrows the existing owner/RPC mechanics, not the seed
// transaction or checkpoint policy. It never starts or stops either node.
type ResultPublisher struct {
	owner *Publisher
}

func AttachOwnedResult(ctx context.Context, binding OwnerBinding) (*ResultPublisher, error) {
	owner, err := AttachOwned(ctx, binding)
	if err != nil {
		return nil, err
	}
	return &ResultPublisher{owner: owner}, nil
}

func (p *ResultPublisher) Identity() ResultIdentity {
	if p == nil || p.owner == nil {
		return ResultIdentity{}
	}
	key, script, _ := resultFundingKey()
	return ResultIdentity{Chain: p.owner.Identity(), Purpose: "candidate_result_v1",
		FundingPublicKey: hex.EncodeToString(key.PubKey().SerializeCompressed()),
		FundingScript:    hex.EncodeToString(script)}
}

func (p *ResultPublisher) Close() error {
	if p == nil || p.owner == nil {
		return nil
	}
	return p.owner.Close()
}

func (p *ResultPublisher) begin(ctx context.Context) (context.Context, func(), error) {
	if p == nil || p.owner == nil {
		return nil, nil, ErrOffline
	}
	c, end, err := p.owner.begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err = p.finish(c); err != nil {
		end()
		return nil, nil, err
	}
	return c, end, nil
}

func (p *ResultPublisher) finish(ctx context.Context) error {
	if ctx.Err() != nil {
		return ErrOffline
	}
	if err := p.owner.checkOwner(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ErrOffline
	}
	return nil
}

func (p *ResultPublisher) Observe(ctx context.Context) (Tip, error) {
	c, end, err := p.begin(ctx)
	if err != nil {
		return Tip{}, err
	}
	defer end()
	return p.owner.observe(c)
}

func (p *ResultPublisher) CanonicalHash(ctx context.Context, height uint32) (string, error) {
	c, end, err := p.begin(ctx)
	if err != nil {
		return "", err
	}
	defer end()
	hash, err := p.owner.canonical(c, height)
	if err != nil {
		return "", err
	}
	if err = p.finish(c); err != nil {
		return "", err
	}
	return hash, nil
}
