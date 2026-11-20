// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymevent"
)

const (
	OwnedMaxPinSize             = 64 * 1024
	OwnedMaxMetadataSize        = 96 * 1024
	OwnedMaxRecordSize          = 224 * 1024
	OwnedMaxResponseSize        = 272 * 1024
	OwnedMaxStatementSize       = 36 * 1024
	OwnedMaxSignedStatementSize = 37 * 1024
)

type OwnedPin struct {
	Version                    uint32           `json:"version"`
	SourceStoreID              string           `json:"source_store_id"`
	SourcePin                  gymbout.OwnedPin `json:"source_pin"`
	PublisherGymID             string           `json:"publisher_gym_id"`
	PublisherAccountID         string           `json:"publisher_account_id"`
	PublisherSourceAuthorityID string           `json:"publisher_source_authority_id"`
	PublisherChainID           string           `json:"publisher_chain_id"`
	Network                    uint32           `json:"network"`
	Genesis                    string           `json:"genesis"`
	FloorHeight                uint32           `json:"floor_height"`
	FloorHash                  string           `json:"floor_hash"`
	PublisherPublicKey         string           `json:"publisher_public_key"`
	CheckpointPublicKey        string           `json:"checkpoint_public_key"`
	FundingPublicKey           string           `json:"funding_public_key"`
	FundingScript              string           `json:"funding_script"`
	RulesID                    string           `json:"rules_id"`
}

type OwnedInfo struct {
	Version             uint32   `json:"version"`
	StoreID             string   `json:"store_id"`
	Pin                 OwnedPin `json:"pin"`
	SourceCreatedAtMS   int64    `json:"source_created_at_ms"`
	CreatedAtMS         int64    `json:"created_at_ms"`
	ResultCount         uint32   `json:"result_count"`
	FundingHeight       int32    `json:"funding_height"`
	FundingHash         string   `json:"funding_hash"`
	FundingComplete     bool     `json:"funding_complete"`
	FundingGeneration   uint32   `json:"funding_generation"`
	FundingRestartCount uint32   `json:"funding_restart_count"`
}

type OwnedStatement struct {
	Version             uint32                    `json:"version"`
	ResultKey           string                    `json:"result_key"`
	StoreID             string                    `json:"store_id"`
	SourceStoreID       string                    `json:"source_store_id"`
	Source              gymbout.OwnedResultSource `json:"source"`
	PublisherChainID    string                    `json:"publisher_chain_id"`
	Network             uint32                    `json:"network"`
	Genesis             string                    `json:"genesis"`
	RulesID             string                    `json:"rules_id"`
	PublisherPublicKey  string                    `json:"publisher_public_key"`
	CheckpointPublicKey string                    `json:"checkpoint_public_key"`
}

type OwnedSignedStatement struct {
	Version   uint32         `json:"version"`
	Statement OwnedStatement `json:"statement"`
	Signature string         `json:"signature"`
}

type OwnedConflict struct {
	Kind           string                     `json:"kind"`
	ObservedAtMS   int64                      `json:"observed_at_ms"`
	ExpectedHeight uint32                     `json:"expected_height"`
	ExpectedHash   string                     `json:"expected_hash"`
	ObservedHash   string                     `json:"observed_hash"`
	ObservedTip    *Tip                       `json:"observed_tip"`
	Checkpoint     *chaincfg.SignedCheckpoint `json:"checkpoint"`
}

type OwnedRecord struct {
	Version          uint32                     `json:"version"`
	StoreID          string                     `json:"store_id"`
	ResultKey        string                     `json:"result_key"`
	Statement        OwnedSignedStatement       `json:"statement"`
	PreparedAtMS     int64                      `json:"prepared_at_ms"`
	Publication      *Publication               `json:"publication"`
	Cursor           Cursor                     `json:"cursor"`
	ScanRestarts     []ScanRestart              `json:"scan_restarts"`
	Inclusion        *Inclusion                 `json:"inclusion"`
	PriorInclusions  []PriorInclusion           `json:"prior_inclusions"`
	Checkpoint       *chaincfg.SignedCheckpoint `json:"checkpoint"`
	Finality         *Finality                  `json:"finality"`
	Equivocation     *OwnedSignedStatement      `json:"equivocation"`
	Conflicts        []OwnedConflict            `json:"conflicts"`
	ConflictOverflow bool                       `json:"conflict_overflow"`
	Observation      Observation                `json:"observation"`
}

type OwnedStatus struct {
	Version             uint32              `json:"version"`
	StoreID             string              `json:"store_id"`
	ResultKey           string              `json:"result_key"`
	CardID              string              `json:"card_id"`
	BoutID              string              `json:"bout_id"`
	Event               gymevent.Event      `json:"event"`
	Exists              bool                `json:"exists"`
	Phase               string              `json:"phase"`
	RecordDigest        string              `json:"record_digest"`
	SourceRecordDigest  string              `json:"source_record_digest"`
	InputID             string              `json:"input_id"`
	ResultDigest        string              `json:"result_digest"`
	PreparedAtMS        int64               `json:"prepared_at_ms"`
	PublicationState    string              `json:"publication_state"`
	TxID                string              `json:"txid"`
	ScanHeight          int32               `json:"scan_height"`
	ScanTipHeight       int32               `json:"scan_tip_height"`
	ScanGeneration      uint32              `json:"scan_generation"`
	InclusionHeight     uint32              `json:"inclusion_height"`
	InclusionHash       string              `json:"inclusion_hash"`
	InclusionGeneration uint32              `json:"inclusion_generation"`
	PriorInclusionCount uint32              `json:"prior_inclusion_count"`
	Descendants         uint32              `json:"descendants"`
	CheckpointPresent   bool                `json:"checkpoint_present"`
	FinalizedAtMS       int64               `json:"finalized_at_ms"`
	ConflictCount       uint32              `json:"conflict_count"`
	ConflictOverflow    bool                `json:"conflict_overflow"`
	ObservedAtMS        int64               `json:"observed_at_ms"`
	ChainObservedAtMS   int64               `json:"chain_observed_at_ms"`
	NodeState           string              `json:"node_state"`
	FundingState        string              `json:"funding_state"`
	CurrentFinality     bool                `json:"current_finality"`
	Source              gymbout.OwnedStatus `json:"source"`
}

type OwnedActionResult struct {
	Version uint32      `json:"version"`
	Record  OwnedRecord `json:"record"`
	Status  OwnedStatus `json:"status"`
}

type OwnedReadResult struct {
	Version uint32      `json:"version"`
	Record  OwnedRecord `json:"record"`
	Status  OwnedStatus `json:"status"`
}
