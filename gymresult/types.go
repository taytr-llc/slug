// Copyright (c) 2026 The SLUG developers. ISC license.
// Package gymresult retains fixture-only candidate publication evidence.
package gymresult

import (
	"errors"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/seednode"
)

const (
	Version1               uint32 = 1
	MaxResults                    = 256
	MaxFundingCoins               = 128
	MaxReservations               = 256
	MaxScanBlocks                 = 8
	MaxScanRestarts               = 16
	MaxPriorInclusions            = 16
	MaxConflicts                  = 8
	MaxPinSize                    = 32 * 1024
	MaxInfoSize                   = 36 * 1024
	MaxMarkerSize                 = 36 * 1024
	MaxMetadataSize               = 64 * 1024
	MaxStatementSize              = 24 * 1024
	MaxSignedStatementSize        = 25 * 1024
	MaxPublicationSize            = 4 * 1024
	MaxRawTxSize                  = 1024
	MaxAnchorSize                 = 2 * 1024
	MaxInclusionSize              = 3 * 1024
	MaxPriorInclusionSize         = 4 * 1024
	MaxScanRestartSize            = 1024
	MaxConflictSize               = 2 * 1024
	MaxRecordSize                 = 192 * 1024
	MaxStatusSize                 = 16 * 1024
	MaxActionResultSize           = 224 * 1024
	MaxReadResultSize             = 224 * 1024
	MaxResponseSize               = 256 * 1024
	MaxIndexSize                  = 96 * 1024
	MaxProofSize                  = 24 * 1024 * 1024
	MaxProofHeaders               = 512
	MaxProofBlocks                = 3
	MaxProofSummarySize           = 2 * 1024
	MaxSafeInteger         uint64 = 9007199254740991
	MaxTimestampMS         int64  = 8640000000000000
)

var (
	ErrInvalid          = errors.New("invalid_request")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrStaleView        = errors.New("stale_view")
	ErrBusy             = errors.New("busy")
	ErrOutcomeUnknown   = errors.New("outcome_unknown")
	ErrUnavailable      = errors.New("result_unavailable")
	ErrCorrupt          = errors.New("result_corrupt")
	ErrCapacity         = errors.New("result_capacity")
	ErrClock            = errors.New("result_clock")
	ErrCalendar         = errors.New("result_calendar")
	ErrNotFound         = errors.New("result_not_found")
	ErrNotPrepared      = errors.New("result_not_prepared")
	ErrSourceIncomplete = errors.New("result_source_incomplete")
	ErrSourceChanged    = errors.New("result_source_changed")
	ErrSourceConflict   = errors.New("result_source_conflict")
	ErrNodeOffline      = errors.New("result_node_offline")
	ErrWaitingForFunds  = errors.New("result_waiting_for_funds")
	ErrScanIncomplete   = errors.New("result_scan_incomplete")
	ErrNotMature        = errors.New("result_not_mature")
	ErrConflict         = errors.New("result_conflict")
	ErrCancelled        = errors.New("result_cancelled")
	ErrProofUnavailable = errors.New("result_proof_unavailable")
	ErrClosed           = errors.New("result store closed")
	ErrPoisoned         = errors.New("result write uncertain; reopen required")
	ErrProfile          = errors.New("result requires production PoW")
)

type Tip = seednode.Tip
type Outpoint = seednode.Outpoint
type FundingCoin = seednode.FundingCoin
type Anchor = seednode.Anchor
type Cursor = gymseed.Cursor
type ScanRestart = gymseed.ScanRestart
type Pin struct {
	Version                    uint32      `json:"version"`
	SourceStoreID              string      `json:"source_store_id"`
	SourcePin                  gymbout.Pin `json:"source_pin"`
	PublisherGymID             string      `json:"publisher_gym_id"`
	PublisherAccountID         string      `json:"publisher_account_id"`
	PublisherSourceAuthorityID string      `json:"publisher_source_authority_id"`
	PublisherChainID           string      `json:"publisher_chain_id"`
	Network                    uint32      `json:"network"`
	Genesis                    string      `json:"genesis"`
	FloorHeight                uint32      `json:"floor_height"`
	FloorHash                  string      `json:"floor_hash"`
	PublisherPublicKey         string      `json:"publisher_public_key"`
	CheckpointPublicKey        string      `json:"checkpoint_public_key"`
	FundingPublicKey           string      `json:"funding_public_key"`
	FundingScript              string      `json:"funding_script"`
	RulesID                    string      `json:"rules_id"`
}
type Info struct {
	Version             uint32 `json:"version"`
	StoreID             string `json:"store_id"`
	Pin                 Pin    `json:"pin"`
	SourceCreatedAtMS   int64  `json:"source_created_at_ms"`
	CreatedAtMS         int64  `json:"created_at_ms"`
	ResultCount         uint32 `json:"result_count"`
	FundingHeight       int32  `json:"funding_height"`
	FundingHash         string `json:"funding_hash"`
	FundingComplete     bool   `json:"funding_complete"`
	FundingGeneration   uint32 `json:"funding_generation"`
	FundingRestartCount uint32 `json:"funding_restart_count"`
}
type Statement struct {
	Version             uint32               `json:"version"`
	ResultKey           string               `json:"result_key"`
	StoreID             string               `json:"store_id"`
	SourceStoreID       string               `json:"source_store_id"`
	Source              gymbout.ResultSource `json:"source"`
	PublisherChainID    string               `json:"publisher_chain_id"`
	Network             uint32               `json:"network"`
	Genesis             string               `json:"genesis"`
	RulesID             string               `json:"rules_id"`
	PublisherPublicKey  string               `json:"publisher_public_key"`
	CheckpointPublicKey string               `json:"checkpoint_public_key"`
}
type SignedStatement struct {
	Version   uint32    `json:"version"`
	Statement Statement `json:"statement"`
	Signature string    `json:"signature"`
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
type Inclusion struct {
	Version      uint32 `json:"version"`
	Generation   uint32 `json:"generation"`
	Anchor       Anchor `json:"anchor"`
	ObservedAtMS int64  `json:"observed_at_ms"`
	ObservedTip  Tip    `json:"observed_tip"`
}
type PriorInclusion struct {
	Version         uint32    `json:"version"`
	Inclusion       Inclusion `json:"inclusion"`
	OrphanedAtMS    int64     `json:"orphaned_at_ms"`
	ObservedTip     Tip       `json:"observed_tip"`
	ReplacementHash string    `json:"replacement_hash"`
}
type Finality struct {
	Checkpoint   chaincfg.SignedCheckpoint `json:"checkpoint"`
	Inclusion    Inclusion                 `json:"inclusion"`
	ObservedAtMS int64                     `json:"observed_at_ms"`
	ObservedTip  Tip                       `json:"observed_tip"`
	Descendants  uint32                    `json:"descendants"`
}
type Conflict struct {
	Kind           string                     `json:"kind"`
	ObservedAtMS   int64                      `json:"observed_at_ms"`
	ExpectedHeight uint32                     `json:"expected_height"`
	ExpectedHash   string                     `json:"expected_hash"`
	ObservedHash   string                     `json:"observed_hash"`
	ObservedTip    *Tip                       `json:"observed_tip"`
	Checkpoint     *chaincfg.SignedCheckpoint `json:"checkpoint"`
}
type Observation struct {
	ObservedAtMS      int64  `json:"observed_at_ms"`
	ChainObservedAtMS int64  `json:"chain_observed_at_ms"`
	Tip               *Tip   `json:"tip"`
	NodeState         string `json:"node_state"`
	Descendants       uint32 `json:"descendants"`
	CurrentFinality   bool   `json:"current_finality"`
}
type Record struct {
	Version          uint32                     `json:"version"`
	StoreID          string                     `json:"store_id"`
	ResultKey        string                     `json:"result_key"`
	Statement        SignedStatement            `json:"statement"`
	PreparedAtMS     int64                      `json:"prepared_at_ms"`
	Publication      *Publication               `json:"publication"`
	Cursor           Cursor                     `json:"cursor"`
	ScanRestarts     []ScanRestart              `json:"scan_restarts"`
	Inclusion        *Inclusion                 `json:"inclusion"`
	PriorInclusions  []PriorInclusion           `json:"prior_inclusions"`
	Checkpoint       *chaincfg.SignedCheckpoint `json:"checkpoint"`
	Finality         *Finality                  `json:"finality"`
	Equivocation     *SignedStatement           `json:"equivocation"`
	Conflicts        []Conflict                 `json:"conflicts"`
	ConflictOverflow bool                       `json:"conflict_overflow"`
	Observation      Observation                `json:"observation"`
}
type Status struct {
	Version             uint32         `json:"version"`
	StoreID             string         `json:"store_id"`
	ResultKey           string         `json:"result_key"`
	CardID              string         `json:"card_id"`
	BoutID              string         `json:"bout_id"`
	Event               gymevent.Event `json:"event"`
	Exists              bool           `json:"exists"`
	Phase               string         `json:"phase"`
	RecordDigest        string         `json:"record_digest"`
	SourceRecordDigest  string         `json:"source_record_digest"`
	InputID             string         `json:"input_id"`
	ResultDigest        string         `json:"result_digest"`
	PreparedAtMS        int64          `json:"prepared_at_ms"`
	PublicationState    string         `json:"publication_state"`
	TxID                string         `json:"txid"`
	ScanHeight          int32          `json:"scan_height"`
	ScanTipHeight       int32          `json:"scan_tip_height"`
	ScanGeneration      uint32         `json:"scan_generation"`
	InclusionHeight     uint32         `json:"inclusion_height"`
	InclusionHash       string         `json:"inclusion_hash"`
	InclusionGeneration uint32         `json:"inclusion_generation"`
	PriorInclusionCount uint32         `json:"prior_inclusion_count"`
	Descendants         uint32         `json:"descendants"`
	CheckpointPresent   bool           `json:"checkpoint_present"`
	FinalizedAtMS       int64          `json:"finalized_at_ms"`
	ConflictCount       uint32         `json:"conflict_count"`
	ConflictOverflow    bool           `json:"conflict_overflow"`
	ObservedAtMS        int64          `json:"observed_at_ms"`
	ChainObservedAtMS   int64          `json:"chain_observed_at_ms"`
	NodeState           string         `json:"node_state"`
	FundingState        string         `json:"funding_state"`
	CurrentFinality     bool           `json:"current_finality"`
	Source              gymbout.Status `json:"source"`
}
type ActionResult struct {
	Version uint32 `json:"version"`
	Record  Record `json:"record"`
	Status  Status `json:"status"`
}
type ReadResult struct {
	Version uint32 `json:"version"`
	Record  Record `json:"record"`
	Status  Status `json:"status"`
}
type ProofHeader struct {
	Height uint32 `json:"height"`
	Hash   string `json:"hash"`
	Header string `json:"header"` // exactly80 bytes lower-case hex
}
type ProofBlock struct {
	Height   uint32 `json:"height"`
	Hash     string `json:"hash"`
	RawBlock string `json:"raw_block"` // full wire block, <=4,000,000 bytes, hex
}
type PublicProof struct {
	Version           uint32         `json:"version"`
	Scope             string         `json:"scope"` // exactly candidate_result_proof_v1
	Pin               Pin            `json:"pin"`
	Result            Record         `json:"result"`
	Status            Status         `json:"status"`
	Bout              gymbout.Record `json:"bout"`
	Card              gymcard.Record `json:"card"`
	ObservedTip       Tip            `json:"observed_tip"`
	ResultFinalityTip *Tip           `json:"result_finality_tip"`
	SourceFinalityTip Tip            `json:"source_finality_tip"`
	Headers           []ProofHeader  `json:"headers"`
	Blocks            []ProofBlock   `json:"blocks"`
}
type ProofSummary struct {
	Version            uint32 `json:"version"`
	ResultKey          string `json:"result_key"`
	CardID             string `json:"card_id"`
	BoutID             string `json:"bout_id"`
	RecordDigest       string `json:"record_digest"`
	SourceRecordDigest string `json:"source_record_digest"`
	InputID            string `json:"input_id"`
	ResultDigest       string `json:"result_digest"`
	TxID               string `json:"txid"`
	SHA256             string `json:"sha256"`
	Bytes              uint32 `json:"bytes"`
	HeaderCount        uint32 `json:"header_count"`
	BlockCount         uint32 `json:"block_count"`
	FinalizedAtMS      int64  `json:"finalized_at_ms"`
	ObservedTip        Tip    `json:"observed_tip"`
}
