//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"testing"

	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
)

type capacityUnusedNode struct{ t *testing.T }

func (n capacityUnusedNode) used() { n.t.Error("intake control consulted node") }
func (n capacityUnusedNode) Resolve(context.Context, prospectnode.Policy) (prospect.Context, error) {
	n.used()
	return prospect.Context{}, ErrUnavailable
}
func (n capacityUnusedNode) ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error) {
	n.used()
	return prospectwork.Template{}, ErrUnavailable
}
func (n capacityUnusedNode) Proposal(context.Context, prospect.Context, string) error {
	n.used()
	return ErrUnavailable
}
func (n capacityUnusedNode) RecheckContext(context.Context, prospect.Context) error {
	n.used()
	return ErrUnavailable
}
func (n capacityUnusedNode) SubmitBlock(context.Context, prospect.Context, string) error {
	n.used()
	return ErrUnavailable
}
func (n capacityUnusedNode) CanonicalBlock(context.Context, prospect.Context, prospect.Proof, string) error {
	n.used()
	return ErrUnavailable
}
func (n capacityUnusedNode) RecheckAdmission(context.Context, prospect.Context, prospect.Proof, prospect.Verified) (string, error) {
	n.used()
	return "", ErrUnavailable
}

// Real original pool/issuer/session/controllers; no listener, mining, replay or
// funding is faked. The booleans exercise only the private startup coordination
// state machine, and explicitly do not grant any ownership Head.
func TestOperatorCapacityOriginalStartupAndPermanentDrain(t *testing.T) {
	r, _ := opActionTestRuntime(t)
	ctx := context.Background()
	path, fresh, err := r.files.child("pool")
	if err != nil || !fresh {
		t.Fatal(err)
	}
	node := capacityUnusedNode{t}
	r.pool, err = poolshare.Create(path, r.setup.Pool, r.issuer, node, r.miners.PoolSessions())
	if err != nil {
		t.Fatal(err)
	}
	if err = r.files.adopt("pool"); err != nil {
		t.Fatal(err)
	}
	r.reservation, err = CapacityEnvelope(r.setup)
	if err != nil {
		t.Fatal(err)
	}
	r.intake, err = r.pool.AttachOperatorAdmission(ctx, r.setup.Pool, r.reservation.Submissions)
	if err != nil {
		t.Fatal(err)
	}
	r.admissionServer, err = prospectservice.NewPausedFencedManifestAdmissionServer(prospectservice.ManifestServerConfig{Version: 3, Listen: "127.0.0.1:0", AuthorityPin: r.setup.Pool.AuthorityPin, Clients: []prospectservice.Credential{}, MaxJobs: r.setup.Pool.MaxWorks, MaxRecords: uint64(r.setup.Authority.Limits.Admissions)}, r.setup.Pool.Policy, node, r.issuer, r.miners.AdmissionSessions())
	if err != nil {
		t.Fatal(err)
	}
	if err = r.updateIntake(ctx, false, false, ""); err != nil || r.status.Capacity.Mode != "paused" {
		t.Fatal(r.status.Capacity, err)
	}
	if err = r.updateIntake(ctx, true, false, ""); err != nil || r.status.Capacity.Mode != "paused" {
		t.Fatal("public alone enabled intake", err)
	}
	if err = r.updateIntake(ctx, false, true, ""); err != nil || r.status.Capacity.Mode != "enabled" {
		t.Fatal("both original startup observations failed", err)
	}
	if err = r.updateIntake(ctx, false, false, "round_capacity"); err != nil || r.status.Capacity.Mode != "draining" {
		t.Fatal(err)
	}
	if err = r.updateIntake(ctx, true, true, ""); err != nil || r.status.Capacity.Mode != "draining" || r.status.Capacity.Reason != "round_capacity" {
		t.Fatal("drain re-enabled or changed cause", err)
	}
	if mode, err := r.admissionServer.OperatorIntakeMode(ctx); err != nil || mode != "draining" {
		t.Fatal("fighter admission did not drain", mode, err)
	}
	if _, err = r.ownership.Head(ctx); err == nil {
		t.Fatal("intake control fabricated current ownership")
	}
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	before := r.status.Capacity
	if err = r.updateIntake(stopped, true, true, ""); err == nil || r.status.Capacity != before {
		t.Fatal("canceled transition changed observation", err)
	}
}
