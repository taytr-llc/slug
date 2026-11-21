// Copyright (c) 2026 The SLUG developers. ISC license.
// Package poolminer owns explicit operator-issued mining credentials. It grants
// no account, wallet, transfer, provider-login or MFA authority.
package poolminer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
)

const Version1 uint32 = 1
const Version2 uint32 = 2
const PoolAudience = "pool"
const AdmissionAudience = "admission"
const MaxTimeMS int64 = 253402300799999
const MaxLifetimeMS int64 = 30 * 24 * 60 * 60 * 1000
const maxHistory = 4096
const markerName = "SLUG-POOL-MINERS.json"
const maxMarkerBytes = 65536
const maxStateBytes = 1048576

var stateKey = []byte("state")
var (
	ErrInvalid        = errors.New("invalid miner credential configuration or request")
	ErrProfile        = errors.New("miner credentials require production PoW")
	ErrUnauthorized   = errors.New("miner credential unavailable")
	ErrConflict       = errors.New("miner credential generation conflict")
	ErrBusy           = errors.New("miner credential operation busy")
	ErrClosed         = errors.New("miner credentials closed")
	ErrClock          = errors.New("miner credential clock refused")
	ErrCapacity       = errors.New("miner credential capacity reached")
	ErrStorage        = errors.New("miner credential storage unavailable")
	ErrPoisoned       = errors.New("miner credential owner poisoned")
	ErrOutcomeUnknown = errors.New("miner credential write outcome unknown")
)

type Enrollment struct {
	PublicKey   string `json:"public_key"`
	GymID       string `json:"gym_id"`
	PayeeScript string `json:"payee_script"`
}
type Config struct {
	Version            uint32       `json:"version"`
	CredentialID       string       `json:"credential_id"`
	RuntimeID          string       `json:"runtime_id"`
	OperatorProfileID  string       `json:"operator_profile_id"`
	SourceID           string       `json:"source_id"`
	SettlementConfigID string       `json:"settlement_config_id"`
	Enrollments        []Enrollment `json:"enrollments"`
}

// Binding is independently selected configuration, not a store/authority handle.
type Binding struct {
	Profile    registry.OperatorProfile
	Source     poolshare.Config
	Settlement poolsettle.Config
}
type Credential struct {
	GymID       string `json:"gym_id"`
	Audience    string `json:"audience"`
	Generation  uint32 `json:"generation"`
	TokenHash   string `json:"token_hash"`
	ReuseHash   string `json:"reuse_hash"`
	IssuedAtMS  int64  `json:"issued_at_ms"`
	ExpiresAtMS int64  `json:"expires_at_ms"`
	RevokedAtMS int64  `json:"revoked_at_ms"`
}
type TokenUse struct {
	TokenHash string `json:"token_hash"`
	ReuseHash string `json:"reuse_hash"`
}
type State struct {
	Version     uint32       `json:"version"`
	Config      Config       `json:"config"`
	Revision    uint32       `json:"revision"`
	ClockMS     int64        `json:"clock_ms"`
	Credentials []Credential `json:"credentials"`
	History     []TokenUse   `json:"history"`
}

// Store retains its original files until all callbacks/rechecks join. Callbacks
// must be bounded and may not reenter the store (contention returns ErrBusy).
type Store struct {
	daily      bool // selected only by the explicit constructor
	gate       chan struct{}
	closeGate  chan struct{}
	checkMu    sync.Mutex
	closing    atomic.Bool
	closed     atomic.Bool
	poisoned   atomic.Bool
	files      *credentialFiles
	db         *leveldb.DB
	state      State
	configID   string
	now        func() time.Time
	write      func([]byte) error
	checkpoint func(string)
	dbClosed   bool
	closeErr   error
}

// Sessions is a concrete audience projection of one original Store. Its zero
// value refuses. It does not accept a caller-selected audience at WithSession.
type Sessions struct {
	owner    *Store
	audience string
}

func (s *Store) PoolSessions() *Sessions { return &Sessions{owner: s, audience: PoolAudience} }
func (s *Store) AdmissionSessions() *Sessions {
	return &Sessions{owner: s, audience: AdmissionAudience}
}

type CleanupError struct {
	owner *Store
	cause error
}

func (e *CleanupError) Error() string { return "miner credential cleanup incomplete" }
func (e *CleanupError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}
func (e *CleanupError) Close(ctx context.Context) error {
	if e == nil || e.owner == nil {
		return ErrClosed
	}
	return e.owner.Close(ctx)
}
func cloneConfig(c Config) Config { c.Enrollments = append([]Enrollment{}, c.Enrollments...); return c }
func cloneState(s State) State {
	s.Config = cloneConfig(s.Config)
	s.Credentials = append([]Credential{}, s.Credentials...)
	s.History = append([]TokenUse{}, s.History...)
	return s
}
func normalWrite(db *leveldb.DB) func([]byte) error {
	return func(raw []byte) error { return db.Put(stateKey, raw, &opt.WriteOptions{Sync: true}) }
}
