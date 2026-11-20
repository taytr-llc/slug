// Copyright (c) 2026 The SLUG developers. ISC license.

// Package fightercarrier contains the bounded private carrier journal codecs.
// Parsed data is never a wallet, original session, or ownership capability.
package fightercarrier

import "errors"

const (
	MaxOperations               = 128
	MaxRequestBytes             = 1024
	MaxReviewBytes              = 4096
	MaxPreparationBytes         = 16384
	MaxWalletRecordBytes        = 24576
	MaxSignedPackageBytes       = 12288
	MaxOperationBytes           = 49152
	MaxStateBytes               = 8388608
	MaxTimeMS             int64 = 253402300799999
	MaxMoney              int64 = 2100000000000000
)

var (
	ErrInvalid  = errors.New("fighter carrier invalid data")
	ErrCapacity = errors.New("fighter carrier capacity")
)

type Pin struct {
	Version                uint32 `json:"version"`
	OwnerMode              string `json:"owner_mode"`
	ManagerSchema          uint32 `json:"manager_schema"`
	Network                uint32 `json:"network"`
	Genesis                string `json:"genesis"`
	WalletProfile          string `json:"wallet_profile"`
	RootPublicKey          string `json:"root_public_key"`
	GymPublicKey           string `json:"gym_public_key"`
	GymID                  string `json:"gym_id"`
	WalletOwnerID          string `json:"wallet_owner_id"`
	OwnershipPinID         string `json:"ownership_pin_id"`
	AccountAuthorityID     string `json:"account_authority_id"`
	AccountID              string `json:"account_id"`
	AllocationBindingID    string `json:"allocation_binding_id"`
	OriginalManifestSHA256 string `json:"original_manifest_sha256"`
}

type Request struct {
	Version        uint32 `json:"version"`
	RequestID      string `json:"request_id"`
	Kind           string `json:"kind"`
	OwnershipPinID string `json:"ownership_pin_id"`
	PriorStateID   string `json:"prior_state_id"`
	RecipientGym   string `json:"recipient_gym"`
	CommitTxID     string `json:"commit_txid"`
}

type OwnedInput struct {
	TxID        string `json:"txid"`
	OutputIndex uint32 `json:"output_index"`
	Atoms       int64  `json:"atoms"`
	Script      string `json:"script"`
	Branch      uint32 `json:"branch"`
	Index       uint32 `json:"index"`
	BlockHash   string `json:"block_hash"`
	BlockHeight int32  `json:"block_height"`
	Coinbase    bool   `json:"coinbase"`
}

type ReturnPath struct {
	Address string `json:"address"`
	Script  string `json:"script"`
	Branch  uint32 `json:"branch"`
	Index   uint32 `json:"index"`
}

type NewFunding struct {
	Inputs            []OwnedInput `json:"inputs"`
	TotalInputAtoms   int64        `json:"total_input_atoms"`
	FundingDigest     string       `json:"funding_digest"`
	AllocationID      string       `json:"allocation_id"`
	ReservationDigest string       `json:"reservation_digest"`
	ReturnPath        ReturnPath   `json:"return_path"`
	CommitChangeAtoms int64        `json:"commit_change_atoms"`
}

type RestoredAllocation struct {
	StartIndex     uint32   `json:"start_index"`
	EndIndex       uint32   `json:"end_index"`
	SelectedIndex  uint32   `json:"selected_index"`
	InvalidIndices []uint32 `json:"invalid_indices"`
	CursorsBefore  []uint32 `json:"cursors_before"`
	CursorsAfter   []uint32 `json:"cursors_after"`
}

type RecoveryFunding struct {
	CommitRaw        string     `json:"commit_raw"`
	CommitTxID       string     `json:"commit_txid"`
	CommitBlockHash  string     `json:"commit_block_hash"`
	CommitHeight     int32      `json:"commit_height"`
	CommitPosition   uint32     `json:"commit_position"`
	FundingDigest    string     `json:"funding_digest"`
	TransferID       string     `json:"transfer_id"`
	FundingPublicKey string     `json:"funding_public_key"`
	FundingBranch    uint32     `json:"funding_branch"`
	FundingIndex     uint32     `json:"funding_index"`
	ReturnPath       ReturnPath `json:"return_path"`
	GymAuthorization string     `json:"gym_authorization"`
}

type Review struct {
	Version            uint32 `json:"version"`
	RequestID          string `json:"request_id"`
	Kind               string `json:"kind"`
	BindingID          string `json:"binding_id"`
	OwnershipPinID     string `json:"ownership_pin_id"`
	PriorStateID       string `json:"prior_state_id"`
	SenderGym          string `json:"sender_gym"`
	RecipientGym       string `json:"recipient_gym"`
	TransferID         string `json:"transfer_id"`
	FundingDigest      string `json:"funding_digest"`
	InputDetailsDigest string `json:"input_details_digest"`
	InputCount         uint32 `json:"input_count"`
	TotalInputAtoms    int64  `json:"total_input_atoms"`
	ReturnAddress      string `json:"return_address"`
	ReturnScript       string `json:"return_script"`
	ReturnBranch       uint32 `json:"return_branch"`
	ReturnIndex        uint32 `json:"return_index"`
	CommitChangeAtoms  int64  `json:"commit_change_atoms"`
	EscrowAtoms        int64  `json:"escrow_atoms"`
	CommitFeeAtoms     int64  `json:"commit_fee_atoms"`
	SpendFeeAtoms      int64  `json:"spend_fee_atoms"`
	TotalFeeAtoms      int64  `json:"total_fee_atoms"`
	ReturnedAtoms      int64  `json:"returned_atoms"`
	CommitTxID         string `json:"commit_txid"`
}

type Preparation struct {
	Version             uint32              `json:"version"`
	BindingID           string              `json:"binding_id"`
	Request             Request             `json:"request"`
	PreparedAtMS        int64               `json:"prepared_at_ms"`
	TipHash             string              `json:"tip_hash"`
	TipHeight           int32               `json:"tip_height"`
	OwnershipHeadDigest string              `json:"ownership_head_digest"`
	NewFunding          *NewFunding         `json:"new_funding"`
	RestoredAllocation  *RestoredAllocation `json:"restored_allocation"`
	RecoveryFunding     *RecoveryFunding    `json:"recovery_funding"`
	Review              Review              `json:"review"`
	ReviewDigest        string              `json:"review_digest"`
}

type AuthorizationRequest struct {
	Version        uint32 `json:"version"`
	BindingID      string `json:"binding_id"`
	RequestID      string `json:"request_id"`
	ReviewDigest   string `json:"review_digest"`
	OwnershipPinID string `json:"ownership_pin_id"`
	PriorStateID   string `json:"prior_state_id"`
	SenderGym      string `json:"sender_gym"`
	RecipientGym   string `json:"recipient_gym"`
	FundingDigest  string `json:"funding_digest"`
}

type RawTransaction struct {
	Raw       string `json:"raw"`
	TxID      string `json:"txid"`
	RawSHA256 string `json:"raw_sha256"`
}

type SignedPackage struct {
	Version          uint32         `json:"version"`
	ReviewDigest     string         `json:"review_digest"`
	TransferID       string         `json:"transfer_id"`
	GymAuthorization string         `json:"gym_authorization"`
	Commit           RawTransaction `json:"commit"`
	Spend            RawTransaction `json:"spend"`
	SpendKind        string         `json:"spend_kind"`
}

type Authorization struct {
	Mode           string `json:"mode"`
	ActionID       string `json:"action_id"`
	ReviewID       string `json:"review_id"`
	ViewID         string `json:"view_id"`
	Generation     uint64 `json:"generation"`
	AuthorizedAtMS int64  `json:"authorized_at_ms"`
}

type Dispatch struct {
	Attempt       uint32        `json:"attempt"`
	Authorization Authorization `json:"authorization"`
	IntentAtMS    int64         `json:"intent_at_ms"`
	Result        string        `json:"result"`
	ResultAtMS    int64         `json:"result_at_ms"`
	Error         string        `json:"error"`
}

type Observation struct {
	ObservedAtMS     int64  `json:"observed_at_ms"`
	State            string `json:"state"`
	ExpectedTxID     string `json:"expected_txid"`
	ObservedTxID     string `json:"observed_txid"`
	NodeTip          string `json:"node_tip"`
	NodeHeight       int32  `json:"node_height"`
	BlockHash        string `json:"block_hash"`
	BlockHeight      int32  `json:"block_height"`
	TransactionIndex uint32 `json:"transaction_index"`
	Confirmations    uint32 `json:"confirmations"`
	TransferID       string `json:"transfer_id"`
	RawSHA256        string `json:"raw_sha256"`
	Error            string `json:"error"`
}

type Operation struct {
	Version             uint32         `json:"version"`
	Sequence            uint32         `json:"sequence"`
	RequestID           string         `json:"request_id"`
	Revision            uint32         `json:"revision"`
	CreatedAtMS         int64          `json:"created_at_ms"`
	UpdatedAtMS         int64          `json:"updated_at_ms"`
	Stage               string         `json:"stage"`
	Request             Request        `json:"request"`
	RequestDigest       string         `json:"request_digest"`
	Preparation         *Preparation   `json:"preparation"`
	PreparationDigest   string         `json:"preparation_digest"`
	SignedPackage       *SignedPackage `json:"signed_package"`
	SignedPackageDigest string         `json:"signed_package_digest"`
	SignAuthorization   *Authorization `json:"sign_authorization"`
	CommitDispatch      *Dispatch      `json:"commit_dispatch"`
	SpendDispatch       *Dispatch      `json:"spend_dispatch"`
	CommitObservation   *Observation   `json:"commit_observation"`
	SpendObservation    *Observation   `json:"spend_observation"`
}

type WalletRecord struct {
	Version             uint32      `json:"version"`
	Sequence            uint32      `json:"sequence"`
	RequestID           string      `json:"request_id"`
	BindingID           string      `json:"binding_id"`
	State               string      `json:"state"`
	Preparation         Preparation `json:"preparation"`
	PreparationDigest   string      `json:"preparation_digest"`
	SignedPackageDigest string      `json:"signed_package_digest"`
	CommitTxID          string      `json:"commit_txid"`
	SpendTxID           string      `json:"spend_txid"`
}
