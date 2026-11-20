// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymaccount

const IdentityVersion uint32 = 2

// IdentityConfig explicitly selects the fixture provider authority schema. The
// digest pins the verifier's complete validated trust configuration. It is not
// supplied by an untrusted login request.
type IdentityConfig struct {
	Version            uint32 `json:"version"`
	Accounts           Config `json:"accounts"`
	VerifierConfigHash string `json:"verifier_config_hash"`
	MaxLoginAttempts   uint32 `json:"max_login_attempts"`
}

// VerifiedLogin and VerifiedReset are trusted verifier output, never decoded
// directly from a request body. Their digest is set by that verifier, not taken
// from a token claim. No transport assertion or credential is retained here.
type VerifiedLogin struct {
	VerifierConfigHash                         string
	Issuer, Subject, Nonce                     string
	AuthenticatedAtMS, IssuedAtMS, ExpiresAtMS int64
	CredentialEpoch                            uint64
}
type VerifiedReset struct {
	VerifierConfigHash                    string
	Issuer, Subject, EventID, Stage       string
	FromEpoch, ToEpoch                    uint64
	VerifiedAtMS, IssuedAtMS, ExpiresAtMS int64
}

type LoginAttempt struct {
	Version     uint32 `json:"version"`
	ID          string `json:"id"`
	AuthorityID string `json:"authority_id"`
	Issuer      string `json:"issuer"`
	Nonce       string `json:"nonce"`
	Sequence    uint64 `json:"sequence"`
	IssuedAtMS  int64  `json:"issued_at_ms"`
	ExpiresAtMS int64  `json:"expires_at_ms"`
}

type ResetBarrier struct {
	Version          uint32 `json:"version"`
	EventID          string `json:"event_id"`
	AccountID        string `json:"account_id"`
	PrincipalID      string `json:"principal_id"`
	Generation       uint64 `json:"generation"`
	FromEpoch        uint64 `json:"from_epoch"`
	ToEpoch          uint64 `json:"to_epoch"`
	Status           string `json:"status"`
	VerifiedAtMS     int64  `json:"verified_at_ms"`
	BeganAtMS        int64  `json:"began_at_ms"`
	CompletedAtMS    int64  `json:"completed_at_ms"`
	HoldUntilMS      int64  `json:"hold_until_ms"`
	BeginSequence    uint64 `json:"begin_sequence"`
	CompleteSequence uint64 `json:"complete_sequence"`
}

type identityState struct {
	AccountID         string `json:"account_id"`
	CredentialEpoch   uint64 `json:"credential_epoch"`
	LastResetSequence uint64 `json:"last_reset_sequence"`
	PendingEventID    string `json:"pending_event_id"`
}
type loginAttemptRecord struct {
	Attempt      LoginAttempt `json:"attempt"`
	PrincipalID  string       `json:"principal_id"`
	SessionID    string       `json:"session_id"`
	Generation   uint64       `json:"generation"`
	ConsumedAtMS int64        `json:"consumed_at_ms"`
}
