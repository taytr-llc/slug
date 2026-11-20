// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectservice

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

// NewFencedManifestAdmissionServer selects staged proof/node work followed by
// an atomic original-session commit. Only admission/recovery is exposed; the
// separately owned pool remains the issuer. Existing session servers retain
// their original account-before-registry lease behavior.
// The owner keeps one admission writer for this issuer; its pool writes jobs
// only. This preserves the server's serialized MaxRecords capacity check.
func NewFencedManifestAdmissionServer(config ManifestServerConfig, policy prospectnode.Policy, node ManifestNode, store *registry.ManifestStore, sessions SessionAuthorizer) (*ManifestServer, error) {
	if sessions == nil {
		return nil, errors.New("account session authorizer is required")
	}
	v := reflect.ValueOf(sessions)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Interface, reflect.Slice, reflect.Chan:
		if v.IsNil() {
			return nil, errors.New("account session authorizer is required")
		}
	}
	server, err := NewManifestAdmissionServer(config, policy, node, store, sessions)
	if err != nil {
		return nil, err
	}
	server.fencedAdmission = true
	return server, nil
}

func fencedAdmissionFailure(server *ManifestServer, err error) *responseError {
	// The exact prewrite sentinel can close only this attached intake. Mixed
	// integrity/session/unknown errors retain their original failure semantics.
	if err == registry.ErrPublicCapacity && server.operatorIntake != nil {
		server.operatorIntake.draining = true
		return server.storeFailure(errCapacity)
	}
	if errors.Is(err, mininglease.ErrOutcomeUnknown) {
		return &responseError{"outcome_unknown", 503, true}
	}
	if errors.Is(err, mininglease.ErrUnauthorized) {
		return apiError("unauthorized", 401)
	}
	// A prepared token can lose a race to another issuer operation. Recover by
	// preparing the same proof again; this is never a permanent proof rejection.
	if errors.Is(err, registry.ErrBusy) || errors.Is(err, registry.ErrConflict) {
		return apiError("busy", 503)
	}
	return server.storeFailure(err)
}

func (server *ManifestServer) admitFenced(ctx context.Context, token string, credential *clientCredential, body []byte) (any, *responseError) {
	proof, authorization, err := ParseSubmission(body)
	if err != nil || proof.Version != prospect.Version2 || authorization.Version != gymauth.Version2 {
		return nil, apiError("invalid_request", 400)
	}
	if !strings.EqualFold(authorization.PublicKey, credential.publicKey) {
		return nil, apiError("forbidden", 403)
	}
	lease, err := mininglease.Capture(ctx, server.sessions, token)
	if err != nil || lease.PublicKey() != credential.publicKey {
		return nil, apiError("unauthorized", 401)
	}
	// Info and preparation can wait for an issuer reader and perform Argon.
	// Neither runs inside the captured account lease.
	info, err := server.store.Info()
	if err != nil {
		return nil, server.storeFailure(err)
	}
	prepared, err := server.store.PrepareManifestAdmission(proof, authorization)
	if err != nil {
		return nil, fencedAdmissionFailure(server, err)
	}
	if err := lease.Check(); err != nil {
		return nil, apiError("unauthorized", 401)
	}
	observation := registry.Eligibility{}
	if !prepared.Historical() {
		if intake := server.operatorIntake; intake != nil {
			if intake.draining {
				return nil, apiError("capacity", 503)
			}
			if !intake.enabled {
				return nil, apiError("busy", 503)
			}
		}
		if info.Records >= server.config.MaxRecords {
			if server.operatorIntake != nil {
				server.operatorIntake.draining = true
			}
			return nil, server.storeFailure(errCapacity)
		}
		initial := prepared.Context()
		if !server.samePolicy(initial) {
			return nil, server.storeFailure(errPolicy)
		}
		verified := prepared.Verified()
		source, err := server.node.RecheckAdmission(ctx, initial, prepared.Proof(), verified)
		if err != nil || ctx.Err() != nil {
			return nil, server.storeFailure(errNode)
		}
		observation = registry.Eligibility{RuleVersion: registry.EligibilityRuleVersion2, Source: source}
		switch source {
		case "current-parent":
			observation.Hash, observation.Height = initial.ParentHash, initial.ParentHeight
		case "canonical-winning-block":
			observation.Hash, observation.Height = verified.WorkHash, initial.ParentHeight+1
		default:
			return nil, server.storeFailure(errNode)
		}
	}
	var record registry.ManifestRecord
	var inserted bool
	err = lease.Do(func(check func() error) error {
		var commitErr error
		record, inserted, commitErr = server.store.CommitManifestAdmission(prepared, observation, check)
		return commitErr
	})
	if err != nil {
		return nil, fencedAdmissionFailure(server, err)
	}
	// A success racing logout/reset must never publish under a replacement
	// session. The exact durable receipt remains recoverable by the same proof.
	if lease.Check() != nil {
		return nil, &responseError{"outcome_unknown", 503, true}
	}
	source := "historical-receipt"
	if inserted {
		source = "trusted-node-snapshot"
	}
	return ManifestAdmissionResult{Version: Version3, ContextSource: source, CurrentNodeChecked: inserted, Authentication: "gym-key-control", BirthRule: "job-issuance", Inserted: inserted, Record: record}, nil
}
