// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func (g *Gym) PoolSnapshot() (pilot.PoolSnapshot, error) {
	if g.poolHelper == nil {
		return pilot.PoolSnapshot{}, bridge.ErrUnavailable
	}
	r, e := g.poolHelper.PoolRequest(context.Background(), pilot.PoolRequest{Command: "snapshot"})
	if e != nil {
		return pilot.PoolSnapshot{}, e
	}
	if e = g.observeEvidenceAccount(g.stableHelper, r.Snapshot.Account.State, r.Snapshot.View); e != nil {
		return pilot.PoolSnapshot{}, e
	}
	return *r.Snapshot, nil
}
func (g *Gym) RefreshPoolBalance() error {
	if g.poolHelper == nil {
		return bridge.ErrUnavailable
	}
	_, e := g.poolHelper.PoolRequest(context.Background(), pilot.PoolRequest{Command: "refresh_pool_balance"})
	return e
}

// Public receipt selection shares native dialogs, never a snapshot grammar.
func (g *Gym) publicManifestTransport() manifestTransport {
	if g.poolHelper != nil {
		return poolPortraitTransport{g.poolHelper}
	}
	return g.manifestHelper
}

type poolPortraitTransport struct{ b *bridge.PoolBridge }

func (p poolPortraitTransport) request(ctx context.Context, r pilot.PoolRequest) (pilot.ManifestResponse, error) {
	v, e := p.b.PoolRequest(ctx, r)
	return pilot.ManifestResponse{ID: v.ID, OK: v.OK, Portrait: v.Portrait, ManifestReceipt: v.ManifestReceipt, Error: v.Error}, e
}
func (p poolPortraitTransport) Request(ctx context.Context, c string, n uint64) (pilot.ManifestResponse, error) {
	if c != "manifest_portrait" && c != "manifest_receipt" {
		return pilot.ManifestResponse{}, bridge.ErrUnavailable
	}
	return p.request(ctx, pilot.PoolRequest{Command: c, Sequence: n})
}
func (p poolPortraitTransport) RequestFixture(context.Context, string) (pilot.ManifestResponse, error) {
	return pilot.ManifestResponse{}, bridge.ErrUnavailable
}
func (p poolPortraitTransport) RequestPath(context.Context, string, string) (pilot.ManifestResponse, error) {
	return pilot.ManifestResponse{}, bridge.ErrUnavailable
}
func (p poolPortraitTransport) ExportManifest(ctx context.Context, n uint64, v, id, path string) (pilot.ManifestResponse, error) {
	return p.request(ctx, pilot.PoolRequest{Command: "export_manifest_receipt", Sequence: n, ViewID: v, ProspectID: id, Path: path})
}
func (p poolPortraitTransport) ReleaseManifest(ctx context.Context, v, id string) (pilot.ManifestResponse, error) {
	return p.request(ctx, pilot.PoolRequest{Command: "release_manifest_receipt", ViewID: v, ProspectID: id})
}
func (p poolPortraitTransport) Close() error { return bridge.ErrUnavailable }
