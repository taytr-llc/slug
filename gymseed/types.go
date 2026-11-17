// Copyright (c) 2026 The SLUG developers. ISC license.
// Package gymseed retains candidate publication and conditional seed evidence.
// It is not a production beacon, portable chain proof or result authority.
package gymseed

import (
	"errors"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/seednode"
)

const (
	Version1                 uint32 = 1
	MaxEvents                       = 256
	MaxPageSize                     = 8
	MaxScanBlocks                   = 8
	MaxScanRestarts                 = 16
	MaxFundingCoins                 = 128
	MaxConflicts                    = 8
	MaxPinSize                      = 16384
	MaxInfoSize                     = 20480
	MaxMarkerSize                   = 20480
	MaxMetadataSize                 = 40960
	MaxDeclarationSize              = 4096
	MaxSignedDeclarationSize        = 5120
	MaxPublicationSize              = 4096
	MaxRawTxSize                    = 1024
	MaxScanRestartSize              = 1024
	MaxConflictSize                 = 8192
	MaxRecordSize                   = 229376
	MaxStatusSize                   = 8192
	MaxResponseSize                 = 262144
	MaxSafeInteger           uint64 = 9007199254740991
	MaxTimestampMS           int64  = 8640000000000000
)

var (
	ErrInvalid           = errors.New("invalid_request")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrStaleView         = errors.New("stale_view")
	ErrBusy              = errors.New("busy")
	ErrOutcomeUnknown    = errors.New("outcome_unknown")
	ErrUnavailable       = errors.New("seed_unavailable")
	ErrCorrupt           = errors.New("seed_corrupt")
	ErrCapacity          = errors.New("seed_capacity")
	ErrClock             = errors.New("seed_clock")
	ErrCalendar          = errors.New("seed_calendar")
	ErrCreatedTooLate    = errors.New("seed_created_too_late")
	ErrSourceIncomplete  = errors.New("seed_source_incomplete")
	ErrNotFound          = errors.New("seed_not_found")
	ErrNotPrepared       = errors.New("seed_not_prepared")
	ErrNodeOffline       = errors.New("seed_node_offline")
	ErrWaitingForFunds   = errors.New("seed_waiting_for_funds")
	ErrScanIncomplete    = errors.New("seed_scan_incomplete")
	ErrConflict          = errors.New("seed_conflict")
	ErrTargetUnavailable = errors.New("seed_target_unavailable")
	ErrNotMature         = errors.New("seed_not_mature")
	ErrCancelled         = errors.New("seed_cancelled")
	ErrClosed            = errors.New("seed store closed")
	ErrPoisoned          = errors.New("seed write uncertain; reopen required")
	ErrProfile           = errors.New("seed requires production PoW")
)

type Tip = seednode.Tip
type Outpoint = seednode.Outpoint
type FundingCoin = seednode.FundingCoin
type Anchor = seednode.Anchor

type Pin struct {
	Version                    uint32       `json:"version"`
	SourceStoreID              string       `json:"source_store_id"`
	SourcePin                  gymevent.Pin `json:"source_pin"`
	PublisherGymID             string       `json:"publisher_gym_id"`
	PublisherAccountID         string       `json:"publisher_account_id"`
	PublisherSourceAuthorityID string       `json:"publisher_source_authority_id"`
	PublisherChainID           string       `json:"publisher_chain_id"`
	Network                    uint32       `json:"network"`
	Genesis                    string       `json:"genesis"`
	FloorHeight                uint32       `json:"floor_height"`
	FloorHash                  string       `json:"floor_hash"`
	PublisherPublicKey         string       `json:"publisher_public_key"`
	CheckpointPublicKey        string       `json:"checkpoint_public_key"`
	FundingPublicKey           string       `json:"funding_public_key"`
	FundingScript              string       `json:"funding_script"`
	RulesID                    string       `json:"rules_id"`
}

type Info struct {
	Version             uint32 `json:"version"`
	StoreID             string `json:"store_id"`
	Pin                 Pin    `json:"pin"`
	CreatedAtMS         int64  `json:"created_at_ms"`
	EventCount          uint32 `json:"event_count"`
	FundingHeight       int32  `json:"funding_height"`
	FundingHash         string `json:"funding_hash"`
	FundingComplete     bool   `json:"funding_complete"`
	FundingGeneration   uint32 `json:"funding_generation"`
	FundingRestartCount uint32 `json:"funding_restart_count"`
}

type Declaration struct {
	Version             uint32         `json:"version"`
	EventKey            string         `json:"event_key"`
	StoreID             string         `json:"store_id"`
	SourceStoreID       string         `json:"source_store_id"`
	Event               gymevent.Event `json:"event"`
	SnapshotDigest      string         `json:"snapshot_digest"`
	ReceiptDigest       string         `json:"receipt_digest"`
	PublisherChainID    string         `json:"publisher_chain_id"`
	Network             uint32         `json:"network"`
	Genesis             string         `json:"genesis"`
	FloorHeight         uint32         `json:"floor_height"`
	FloorHash           string         `json:"floor_hash"`
	RulesID             string         `json:"rules_id"`
	PublisherPublicKey  string         `json:"publisher_public_key"`
	CheckpointPublicKey string         `json:"checkpoint_public_key"`
}

type SignedDeclaration struct {
	Version     uint32      `json:"version"`
	Declaration Declaration `json:"declaration"`
	Signature   string      `json:"signature"`
}

type Publication struct {
	Version           uint32      `json:"version"`
	RawTx             string      `json:"raw_tx"`
	TxID              string      `json:"txid"`
	Coin              FundingCoin `json:"coin"`
	BuiltAtMS         int64       `json:"built_at_ms"`
	DispatchCount     uint32      `json:"dispatch_count"`
	FirstDispatchAtMS int64       `json:"first_dispatch_at_ms"`
	LastDispatchAtMS  int64       `json:"last_dispatch_at_ms"`
	State             string      `json:"state"`
}

type Cursor struct {
	Generation       uint32 `json:"generation"`
	Height           int32  `json:"height"`
	Hash             string `json:"hash"`
	ScanDigest       string `json:"scan_digest"`
	CompleteToHeight int32  `json:"complete_to_height"`
	CompleteToHash   string `json:"complete_to_hash"`
}

type ScanRestart struct {
	Generation      uint32 `json:"generation"`
	Previous        Cursor `json:"previous"`
	ObservedAtMS    int64  `json:"observed_at_ms"`
	ObservedTip     Tip    `json:"observed_tip"`
	ReplacementHash string `json:"replacement_hash"`
}

type Target struct {
	Height       uint32 `json:"height"`
	Hash         string `json:"hash"`
	Header       string `json:"header"`
	ObservedAtMS int64  `json:"observed_at_ms"`
	ObservedTip  Tip    `json:"observed_tip"`
}

type Finality struct {
	Checkpoint        chaincfg.SignedCheckpoint `json:"checkpoint"`
	ObservedAtMS      int64                     `json:"observed_at_ms"`
	ObservedTip       Tip                       `json:"observed_tip"`
	TargetDescendants uint32                    `json:"target_descendants"`
}

type Conflict struct {
	Kind           string                     `json:"kind"`
	ObservedAtMS   int64                      `json:"observed_at_ms"`
	Declaration    *SignedDeclaration         `json:"declaration"`
	Checkpoint     *chaincfg.SignedCheckpoint `json:"checkpoint"`
	ExpectedHeight uint32                     `json:"expected_height"`
	ExpectedHash   string                     `json:"expected_hash"`
	ObservedHash   string                     `json:"observed_hash"`
	ObservedTip    *Tip                       `json:"observed_tip"`
}

type Observation struct {
	ObservedAtMS      int64  `json:"observed_at_ms"`
	ChainObservedAtMS int64  `json:"chain_observed_at_ms"`
	Tip               *Tip   `json:"tip"`
	NodeState         string `json:"node_state"`
	TargetDescendants uint32 `json:"target_descendants"`
	CurrentFinality   bool   `json:"current_finality"`
}

type Record struct {
	Version          uint32                     `json:"version"`
	StoreID          string                     `json:"store_id"`
	EventKey         string                     `json:"event_key"`
	SourceReceipt    gymevent.SnapshotReceipt   `json:"source_receipt"`
	Declaration      SignedDeclaration          `json:"declaration"`
	PreparedAtMS     int64                      `json:"prepared_at_ms"`
	Publication      *Publication               `json:"publication"`
	Cursor           Cursor                     `json:"cursor"`
	ScanRestarts     []ScanRestart              `json:"scan_restarts"`
	Anchor           *Anchor                    `json:"anchor"`
	Target           *Target                    `json:"target"`
	Checkpoint       *chaincfg.SignedCheckpoint `json:"checkpoint"`
	Finality         *Finality                  `json:"finality"`
	Conflicts        []Conflict                 `json:"conflicts"`
	ConflictOverflow bool                       `json:"conflict_overflow"`
	IncidentAtMS     int64                      `json:"incident_at_ms"`
	Observation      Observation                `json:"observation"`
}

type Status struct {
	Version           uint32         `json:"version"`
	StoreID           string         `json:"store_id"`
	EventKey          string         `json:"event_key"`
	Event             gymevent.Event `json:"event"`
	Exists            bool           `json:"exists"`
	Phase             string         `json:"phase"`
	PublicationState  string         `json:"publication_state"`
	ScanHeight        int32          `json:"scan_height"`
	ScanTipHeight     int32          `json:"scan_tip_height"`
	AnchorHeight      uint32         `json:"anchor_height"`
	AnchorHash        string         `json:"anchor_hash"`
	TargetHeight      uint32         `json:"target_height"`
	TargetHash        string         `json:"target_hash"`
	TargetDescendants uint32         `json:"target_descendants"`
	CheckpointPresent bool           `json:"checkpoint_present"`
	FinalizedAtMS     int64          `json:"finalized_at_ms"`
	ConflictCount     uint32         `json:"conflict_count"`
	ConflictOverflow  bool           `json:"conflict_overflow"`
	IncidentAtMS      int64          `json:"incident_at_ms"`
	PreparedAtMS      int64          `json:"prepared_at_ms"`
	ObservedAtMS      int64          `json:"observed_at_ms"`
	ChainObservedAtMS int64          `json:"chain_observed_at_ms"`
	NodeState         string         `json:"node_state"`
	FundingState      string         `json:"funding_state"`
	CurrentFinality   bool           `json:"current_finality"`
	EscalationDue     bool           `json:"escalation_due"`
	ScanGeneration    uint32         `json:"scan_generation"`
	FundingGeneration uint32         `json:"funding_generation"`
}

type HistoryPage struct {
	Version       uint32   `json:"version"`
	StoreID       string   `json:"store_id"`
	AfterEventKey string   `json:"after_event_key"`
	NextEventKey  string   `json:"next_event_key"`
	HasMore       bool     `json:"has_more"`
	ObservedAtMS  int64    `json:"observed_at_ms"`
	Events        []Status `json:"events"`
}
