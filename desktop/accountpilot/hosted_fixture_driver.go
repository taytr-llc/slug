// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymprovider"
)

func (d *hostedFixtureDriver) FreshLogin(ctx context.Context, flow gymprovider.LoginFlow, fixture string) (string, error) {
	if ctx == nil {
		return "", gymprovider.ErrConfig
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	now, e := d.enter(ctx)
	if e != nil {
		return "", e
	}
	defer d.mu.Unlock()
	at := hostedFixtureIndex(fixture)
	if at < 0 || !hostedFlowValid(flow, d.state.Config, d.state.CoordinatorID) {
		return "", gymprovider.ErrConflict
	}
	var row hostedAuth
	found := false
	for _, old := range d.state.Auth {
		if old.Flow.ID == flow.ID {
			if old.Flow != flow || old.Fixture != fixture {
				return "", gymprovider.ErrConflict
			}
			row, found = old, true
			break
		}
	}
	if !found {
		if len(d.state.Auth) >= 1024 {
			return "", gymprovider.ErrCapacity
		}
		if d.state.Epochs[at].PendingEventID != "" {
			return "", errHostedPending
		}
		if now >= flow.ExpiresAtMS || now/1000 < flow.IssuedAtMS/1000 {
			return "", gymprovider.ErrExpired
		}
		id, e := hostedRandomID(d.random)
		if e != nil {
			return "", e
		}
		for _, old := range d.state.Auth {
			if old.AuthID == id {
				return "", gymprovider.ErrConflict
			}
		}
		row = hostedAuth{Version: 1, Sequence: uint32(len(d.state.Auth) + 1), Fixture: fixture, Subject: fixture, AuthID: id, Flow: flow, CredentialEpoch: d.state.Epochs[at].CredentialEpoch, AuthenticatedAtMS: now / 1000 * 1000}
		next := hostedCloneState(d.state)
		next.Auth = append(next.Auth, row)
		if e = d.save(ctx, next, now); e != nil {
			return "", e
		}
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	claims := hostedLoginClaims{hostedClaims(d.state.Config, d.state.CoordinatorID, row.Subject, now), "slug-provider-fixture-login", row.AuthenticatedAtMS / 1000, row.Flow.Nonce, row.CredentialEpoch, row.Flow.ID, row.AuthID, row.Flow.ID, "fresh"}
	token, e := hostedSign(d.key, "JWT", claims)
	if e != nil {
		return "", e
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	return token, nil
}
func (d *hostedFixtureDriver) BeginResetEvidence(ctx context.Context, request gymprovider.ResetRequest, fixture string) (string, error) {
	if ctx == nil {
		return "", gymprovider.ErrConfig
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	now, e := d.enter(ctx)
	if e != nil {
		return "", e
	}
	defer d.mu.Unlock()
	at := hostedFixtureIndex(fixture)
	if at < 0 || !hostedRequestValid(request, d.state.Config, d.state.CoordinatorID) {
		return "", gymprovider.ErrConflict
	}
	var row hostedReset
	found := false
	for _, old := range d.state.Resets {
		if old.Request.EventID == request.EventID {
			if old.Request != request || old.Fixture != fixture {
				return "", gymprovider.ErrConflict
			}
			row, found = old, true
			break
		}
	}
	if !found {
		if len(d.state.Resets) >= 256 || d.state.Epochs[at].CredentialEpoch == math.MaxUint64 {
			return "", gymprovider.ErrCapacity
		}
		if d.state.Epochs[at].PendingEventID != "" {
			return "", errHostedPending
		}
		if now >= request.ExpiresAtMS || now/1000 < request.IssuedAtMS/1000 {
			return "", gymprovider.ErrExpired
		}
		account, e := d.accounts.GetIdentityAccount(ctx, gymprovider.FixtureIssuer, fixture)
		if e != nil {
			return "", e
		}
		id, e := hostedRandomID(d.random)
		if e != nil {
			return "", e
		}
		for _, old := range d.state.Resets {
			if old.OperationID == id {
				return "", gymprovider.ErrConflict
			}
		}
		epoch := d.state.Epochs[at].CredentialEpoch
		row = hostedReset{Version: 1, Sequence: uint32(len(d.state.Resets) + 1), Fixture: fixture, Subject: fixture, AccountID: account.ID, OperationID: id, Request: request, FromEpoch: epoch, ToEpoch: epoch + 1, VerifiedAtMS: now / 1000 * 1000, Phase: "verified"}
		next := hostedCloneState(d.state)
		next.Resets = append(next.Resets, row)
		next.Epochs[at].PendingEventID = request.EventID
		if e = d.save(ctx, next, now); e != nil {
			return "", e
		}
	}
	if row.Phase == "retired" {
		return "", gymprovider.ErrConflict
	}
	return d.resetEvidence(ctx, row, "begin", now)
}
func (d *hostedFixtureDriver) resetEvidence(ctx context.Context, row hostedReset, stage string, now int64) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	claims := hostedResetClaims{hostedClaims(d.state.Config, d.state.CoordinatorID, row.Subject, now), "slug-provider-fixture-reset", row.Request.EventID, stage, row.FromEpoch, row.ToEpoch, row.VerifiedAtMS / 1000, row.Request.Nonce, row.OperationID}
	token, e := hostedSign(d.key, "slug-reset+jwt", claims)
	if e != nil {
		return "", e
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	return token, nil
}
func (d *hostedFixtureDriver) findReset(request gymprovider.ResetRequest) (int, error) {
	if !hostedRequestValid(request, d.state.Config, d.state.CoordinatorID) {
		return -1, gymprovider.ErrConflict
	}
	for i, r := range d.state.Resets {
		if r.Request.EventID == request.EventID {
			if r.Request != request {
				return -1, gymprovider.ErrConflict
			}
			return i, nil
		}
	}
	return -1, gymprovider.ErrNotFound
}
func (d *hostedFixtureDriver) CompleteResetEvidence(ctx context.Context, request gymprovider.ResetRequest) (string, error) {
	if ctx == nil {
		return "", gymprovider.ErrConfig
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	now, e := d.enter(ctx)
	if e != nil {
		return "", e
	}
	defer d.mu.Unlock()
	i, e := d.findReset(request)
	if e != nil {
		return "", e
	}
	row := d.state.Resets[i]
	if row.Phase != "committed" && row.Phase != "completed" {
		return "", gymprovider.ErrConflict
	}
	return d.resetEvidence(ctx, row, "complete", now)
}
func (d *hostedFixtureDriver) CommitReset(ctx context.Context, request gymprovider.ResetRequest, ack gymprovider.ResetResult) error {
	return d.ack(ctx, request, ack, false)
}
func (d *hostedFixtureDriver) ObserveResetComplete(ctx context.Context, request gymprovider.ResetRequest, ack gymprovider.ResetResult) error {
	return d.ack(ctx, request, ack, true)
}
func (d *hostedFixtureDriver) ack(ctx context.Context, request gymprovider.ResetRequest, ack gymprovider.ResetResult, complete bool) error {
	if ctx == nil {
		return gymprovider.ErrConfig
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	now, e := d.enter(ctx)
	if e != nil {
		return e
	}
	defer d.mu.Unlock()
	i, e := d.findReset(request)
	if e != nil {
		return e
	}
	row := d.state.Resets[i]
	if !hostedAckValid(row, ack, complete) {
		return gymprovider.ErrConflict
	}
	old := row.BeginAck
	if complete {
		old = row.CompleteAck
	}
	if old != nil {
		if *old != ack {
			return gymprovider.ErrConflict
		}
		return nil
	}
	phase := "verified"
	if complete {
		phase = "committed"
	}
	if row.Phase != phase {
		return gymprovider.ErrConflict
	}
	actual, e := d.accounts.GetResetBarrier(ctx, gymprovider.FixtureIssuer, row.Subject, request.EventID)
	if e != nil {
		return e
	}
	if actual != ack.Barrier {
		return gymprovider.ErrConflict
	}
	// The acknowledgement must also be the exact coordinator stage. No caller
	// supplied CanCommit flag can mutate fixture credentials on its own.
	status, e := d.coordinator.Status(ctx, "reset", request.EventID)
	if e != nil {
		return e
	}
	want := "pending"
	if complete {
		want = "completed"
	}
	if status.Phase != want || status.OutcomeUnknown {
		return gymprovider.ErrConflict
	}
	next := hostedCloneState(d.state)
	r := &next.Resets[i]
	at := hostedFixtureIndex(r.Fixture)
	epoch := &next.Epochs[at]
	if epoch.PendingEventID != request.EventID {
		return gymprovider.ErrConflict
	}
	if complete {
		if epoch.CredentialEpoch != r.ToEpoch {
			return gymprovider.ErrConflict
		}
		r.Phase = "completed"
		r.CompleteAck = &ack
		epoch.PendingEventID = ""
	} else {
		if epoch.CredentialEpoch != r.FromEpoch {
			return gymprovider.ErrConflict
		}
		r.Phase = "committed"
		r.BeginAck = &ack
		epoch.CredentialEpoch = r.ToEpoch
	}
	return d.save(ctx, next, now)
}
func (d *hostedFixtureDriver) PendingReset(ctx context.Context, fixture string) (*FixturePendingReset, error) {
	if ctx == nil {
		return nil, gymprovider.ErrConfig
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, e := d.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer d.mu.Unlock()
	at := hostedFixtureIndex(fixture)
	if at < 0 {
		return nil, gymprovider.ErrConfig
	}
	event := d.state.Epochs[at].PendingEventID
	if event == "" {
		return nil, gymprovider.ErrNotFound
	}
	for _, r := range d.state.Resets {
		if r.Request.EventID == event && r.Fixture == fixture && (r.Phase == "verified" || r.Phase == "committed") {
			return &FixturePendingReset{r.Version, r.Fixture, r.AccountID, r.OperationID, r.Request, r.FromEpoch, r.ToEpoch, r.VerifiedAtMS, r.Phase}, nil
		}
	}
	return nil, gymprovider.ErrCorrupt
}
func (d *hostedFixtureDriver) RetireUnbegunReset(ctx context.Context, request gymprovider.ResetRequest) error {
	if ctx == nil {
		return gymprovider.ErrConfig
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	now, e := d.enter(ctx)
	if e != nil {
		return e
	}
	defer d.mu.Unlock()
	i, e := d.findReset(request)
	if e != nil {
		return e
	}
	r := d.state.Resets[i]
	if r.Phase != "verified" || now < r.Request.ExpiresAtMS {
		return gymprovider.ErrConflict
	}
	status, e := d.coordinator.Status(ctx, "reset", request.EventID)
	if e != nil {
		return e
	}
	if status.Phase != "issued" || status.OutcomeUnknown {
		return gymprovider.ErrConflict
	}
	_, e = d.accounts.GetResetBarrier(ctx, gymprovider.FixtureIssuer, r.Subject, request.EventID)
	if !errors.Is(e, gymaccount.ErrNotFound) {
		if e != nil {
			return e
		}
		return gymprovider.ErrConflict
	}
	next := hostedCloneState(d.state)
	at := hostedFixtureIndex(r.Fixture)
	if next.Epochs[at].PendingEventID != request.EventID {
		return gymprovider.ErrCorrupt
	}
	next.Epochs[at].PendingEventID = ""
	next.Resets[i].Phase = "retired"
	return d.save(ctx, next, now)
}
