// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

// Package gymprovider coordinates signed published-fixture provider evidence.
// It is not a public provider adapter or a source of account authority.
package gymprovider

import (
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
)

const Version1 uint32 = 1
const MaxRecordSize = 16384
const LifetimeMS int64 = 300000
const ProviderFixture = "published-fixture-v1"
const FixtureIssuer = "https://identity.slug.invalid"

var (
	ErrConfig         = errors.New("invalid provider coordinator configuration")
	ErrEvidence       = errors.New("provider evidence rejected")
	ErrNotFound       = errors.New("provider flow not found")
	ErrConflict       = errors.New("provider flow conflicts with retained binding")
	ErrExpired        = errors.New("provider flow expired")
	ErrCapacity       = errors.New("provider journal capacity exhausted")
	ErrUnavailable    = errors.New("provider coordinator unavailable")
	ErrConsumed       = errors.New("provider login flow consumed")
	ErrOutcomeUnknown = errors.New("provider operation outcome unknown")
	ErrCorrupt        = errors.New("invalid provider journal")
	ErrClosed         = errors.New("provider coordinator closed")
)

type Config struct {
	Version            uint32 `json:"version"`
	Provider           string `json:"provider"`
	Issuer             string `json:"issuer"`
	TenantID           string `json:"tenant_id"`
	ClientID           string `json:"client_id"`
	ConnectionID       string `json:"connection_id"`
	AuthorityID        string `json:"authority_id"`
	VerifierConfigHash string `json:"verifier_config_hash"`
	MaxLoginFlows      uint32 `json:"max_login_flows"`
	MaxResetFlows      uint32 `json:"max_reset_flows"`
}
type LoginFlow struct {
	Version       uint32 `json:"version"`
	CoordinatorID string `json:"coordinator_id"`
	AuthorityID   string `json:"authority_id"`
	ID            string `json:"id"`
	AttemptID     string `json:"attempt_id"`
	Issuer        string `json:"issuer"`
	Nonce         string `json:"nonce"`
	Sequence      uint64 `json:"sequence"`
	IssuedAtMS    int64  `json:"issued_at_ms"`
	ExpiresAtMS   int64  `json:"expires_at_ms"`
}
type ResetRequest struct {
	Version       uint32 `json:"version"`
	CoordinatorID string `json:"coordinator_id"`
	AuthorityID   string `json:"authority_id"`
	EventID       string `json:"event_id"`
	Nonce         string `json:"nonce"`
	IssuedAtMS    int64  `json:"issued_at_ms"`
	ExpiresAtMS   int64  `json:"expires_at_ms"`
}
type ResetResult struct {
	Version             uint32                  `json:"version"`
	CoordinatorID       string                  `json:"coordinator_id"`
	AuthorityID         string                  `json:"authority_id"`
	EventID             string                  `json:"event_id"`
	ProviderOperationID string                  `json:"provider_operation_id"`
	Phase               string                  `json:"phase"`
	CanCommit           bool                    `json:"can_commit"`
	Barrier             gymaccount.ResetBarrier `json:"barrier"`
}
type FlowStatus struct {
	Version        uint32 `json:"version"`
	CoordinatorID  string `json:"coordinator_id"`
	AuthorityID    string `json:"authority_id"`
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	Phase          string `json:"phase"`
	OutcomeUnknown bool   `json:"outcome_unknown"`
}
