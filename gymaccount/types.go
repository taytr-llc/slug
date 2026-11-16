// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

// Package gymaccount implements a private two-public-fixture account authority.
// It is not a production login provider, wallet custodian, or ownership registry.
package gymaccount

import (
	"context"
	"errors"
)

const (
	SessionLifetimeMS     int64 = 3600000
	MiningLifetimeMS      int64 = 300000
	FreshAuthenticationMS int64 = 300000
	RecoveryHoldMS        int64 = 86400000
	MaxRecordSize               = 8192
)

type Capability string

const (
	View                Capability = "view"
	Enrollment          Capability = "enrollment"
	Mining              Capability = "mining"
	RecoveryExport      Capability = "recovery-export"
	Withdrawal          Capability = "withdrawal"
	FighterTransfer     Capability = "fighter-transfer"
	RecoveryAuthority   Capability = "recovery-authority"
	MarketplacePurchase Capability = "marketplace-purchase"
	MarketplaceListing  Capability = "marketplace-listing"
)

var (
	ErrClosed       = errors.New("account store closed")
	ErrPoisoned     = errors.New("account write outcome uncertain; reopen required")
	ErrCorrupt      = errors.New("account store corrupt or incompatible")
	ErrUnauthorized = errors.New("account authorization unavailable")
	ErrForbidden    = errors.New("account capability unavailable")
	ErrExpired      = errors.New("account authorization expired")
	ErrHeld         = errors.New("account action temporarily held after recovery")
	ErrConflict     = errors.New("account identity conflict")
	ErrNotFound     = errors.New("account record unavailable")
	ErrCapacity     = errors.New("account store capacity reached")
	ErrClock        = errors.New("account authority clock unavailable")
)

type Identity struct {
	Network       uint32 `json:"network"`
	Genesis       string `json:"genesis"`
	Profile       string `json:"profile"`
	RootPublicKey string `json:"root_public_key"`
	GymPublicKey  string `json:"gym_public_key"`
	GymID         string `json:"gym_id"`
}

type FixturePrincipal struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
	Fixture string `json:"fixture"`
}

type Config struct {
	Version       uint32             `json:"version"`
	Principals    []FixturePrincipal `json:"principals"`
	MaxSessions   uint32             `json:"max_sessions"`
	MaxChallenges uint32             `json:"max_challenges"`
	MaxEvents     uint32             `json:"max_events"`
}

// Principal is trusted adapter output, never an HTTP request body. A service
// must verify the provider assertion before calling Login or ApplyReset.
type Principal struct {
	Issuer            string
	Subject           string
	AuthenticatedAtMS int64
	RecoveryEventID   string
	RecoveryVerified  bool
}

type Provider interface {
	Verify(context.Context, string) (Principal, error)
}

type Account struct {
	Version          uint32    `json:"version"`
	ID               string    `json:"id"`
	PrincipalID      string    `json:"principal_id"`
	Generation       uint64    `json:"generation"`
	Enrollment       *Identity `json:"enrollment,omitempty"`
	RecoveryRequired bool      `json:"recovery_required"`
	HoldUntilMS      int64     `json:"hold_until_ms"`
}

// Session is private helper state. Token is deliberately absent from JSON and
// formatted output; transport code must explicitly use Token as a credential.
type Session struct {
	Token       string  `json:"-"`
	Kind        string  `json:"kind"`
	Account     Account `json:"account"`
	ExpiresAtMS int64   `json:"expires_at_ms"`
}

func (Session) String() string     { return "gymaccount.Session{credential redacted}" }
func (s Session) GoString() string { return s.String() }

type EnrollmentResult struct {
	Version     uint32   `json:"version"`
	AccountID   string   `json:"account_id"`
	Generation  uint64   `json:"generation"`
	ChallengeID string   `json:"challenge_id"`
	Action      string   `json:"action"`
	Identity    Identity `json:"identity"`
}

type ResetResult struct {
	Version     uint32 `json:"version"`
	AccountID   string `json:"account_id"`
	Generation  uint64 `json:"generation"`
	EventID     string `json:"event_id"` // Domain-separated digest, never provider assertion.
	AtMS        int64  `json:"at_ms"`
	HoldUntilMS int64  `json:"hold_until_ms"`
}

type Event struct {
	Version    uint32 `json:"version"`
	Sequence   uint64 `json:"sequence"`
	Kind       string `json:"kind"`
	AccountID  string `json:"account_id"`
	Generation uint64 `json:"generation"`
	AtMS       int64  `json:"at_ms"`
	Reference  string `json:"reference"`
}
