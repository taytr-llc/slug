// Copyright (c) 2026 The SLUG developers. ISC license.
// Package walletwithdraw retains reviewed candidate payments. It neither owns a
// wallet nor grants account authority; only the concrete original-action adapter
// supplies its fences, wallet reconciliation and exact node observations.
package walletwithdraw

import "errors"

const (
	Version           uint32 = 1
	MaxOperations            = 128
	MaxDraftBytes            = 16384
	MaxOperationBytes        = 32768
	MaxStateBytes            = 8388608
	maxMoney          int64  = 2100000000000000
	maxTime           int64  = 253402300799999
	maxGeneration     uint64 = 9007199254740991
	application              = "slug-candidate-withdrawal-journal"
	markerName               = "SLUG-WITHDRAWAL-JOURNAL.json"
	stateName                = "state.json"
)

var (
	ErrInvalid        = errors.New("invalid candidate withdrawal")
	ErrProfile        = errors.New("unsupported candidate withdrawal profile")
	ErrBusy           = errors.New("candidate withdrawal journal busy")
	ErrClosed         = errors.New("candidate withdrawal journal closed")
	ErrNotFound       = errors.New("candidate withdrawal not found")
	ErrConflict       = errors.New("candidate withdrawal conflict")
	ErrCapacity       = errors.New("candidate withdrawal capacity")
	ErrClock          = errors.New("candidate withdrawal clock unavailable")
	ErrCorrupt        = errors.New("candidate withdrawal journal corrupt")
	ErrStorage        = errors.New("candidate withdrawal storage failed")
	ErrUnauthorized   = errors.New("candidate withdrawal original authorization unavailable")
	ErrPoisoned       = errors.New("candidate withdrawal journal requires reopen")
	ErrOutcomeUnknown = errors.New("candidate withdrawal publication outcome unknown")
)

type Stage string

const (
	StageIntent      Stage = "intent"
	StagePrepared    Stage = "prepared"
	StageSigned      Stage = "signed"
	StageDispatching Stage = "dispatching"
	StageSubmitted   Stage = "submitted"
	StageAbandoned   Stage = "abandoned"
)

type DeliveryResult string

const (
	DeliveryPending      DeliveryResult = "pending"
	DeliveryAcknowledged DeliveryResult = "acknowledged"
	DeliveryUnknown      DeliveryResult = "unknown"
	DeliveryRejected     DeliveryResult = "rejected"
)

type ObservationState string

const (
	ObservationUnknown     ObservationState = "unknown"
	ObservationNotObserved ObservationState = "not_observed"
	ObservationMempool     ObservationState = "mempool"
	ObservationConfirmed   ObservationState = "confirmed"
)

type Pin struct {
	Version             uint32 `json:"version"`
	Network             uint32 `json:"network"`
	Genesis             string `json:"genesis"`
	AccountAuthorityID  string `json:"account_authority_id"`
	AccountID           string `json:"account_id"`
	Fixture             string `json:"fixture"`
	WalletProfile       string `json:"wallet_profile"`
	RootPublicKey       string `json:"root_public_key"`
	GymPublicKey        string `json:"gym_public_key"`
	GymID               string `json:"gym_id"`
	PoolSourceBindingID string `json:"pool_source_binding_id"`
}
type Identity struct {
	JournalID string
	Pin       Pin
}
type Request struct {
	Version     uint32 `json:"version"`
	OperationID string `json:"operation_id"`
	Recipient   string `json:"recipient"`
	AmountAtoms int64  `json:"amount_atoms"`
}
type Intent struct {
	Version           uint32 `json:"version"`
	OperationID       string `json:"operation_id"`
	BindingID         string `json:"binding_id"`
	CreatedAtMS       int64  `json:"created_at_ms"`
	Destination       string `json:"destination"`
	RecipientScript   string `json:"recipient_script"`
	RecipientAtoms    int64  `json:"recipient_atoms"`
	FeeRateAtomsPerKB int64  `json:"fee_rate_atoms_per_kb"`
	MaxFeeAtoms       int64  `json:"max_fee_atoms"`
	MinConfirmations  uint32 `json:"min_confirmations"`
	InputPurpose      uint32 `json:"input_purpose"`
	ChangePurpose     uint32 `json:"change_purpose"`
	Account           uint32 `json:"account"`
}
type Input struct {
	TxID        string `json:"txid"`
	OutputIndex uint32 `json:"output_index"`
	Atoms       int64  `json:"atoms"`
	Script      string `json:"script"`
	Branch      uint32 `json:"branch"`
	Index       uint32 `json:"index"`
	BlockHash   string `json:"block_hash"`
	BlockHeight uint32 `json:"block_height"`
	Coinbase    bool   `json:"coinbase"`
}
type Change struct {
	Address     string `json:"address"`
	Script      string `json:"script"`
	Index       uint32 `json:"index"`
	OutputIndex int32  `json:"output_index"`
	Atoms       int64  `json:"atoms"`
}
type Draft struct {
	Version              uint32  `json:"version"`
	BindingID            string  `json:"binding_id"`
	OperationID          string  `json:"operation_id"`
	Recipient            string  `json:"recipient"`
	RecipientScript      string  `json:"recipient_script"`
	AmountAtoms          int64   `json:"amount_atoms"`
	RecipientOutputIndex uint32  `json:"recipient_output_index"`
	FeeRateAtomsPerKB    int64   `json:"fee_rate_atoms_per_kb"`
	FeeCeilingAtoms      int64   `json:"fee_ceiling_atoms"`
	MinConfirmations     uint32  `json:"min_confirmations"`
	TipHash              string  `json:"tip_hash"`
	TipHeight            uint32  `json:"tip_height"`
	TxVersion            int32   `json:"tx_version"`
	InputSequence        uint32  `json:"input_sequence"`
	LockTime             uint32  `json:"lock_time"`
	SighashType          uint32  `json:"sighash_type"`
	Inputs               []Input `json:"inputs"`
	TotalInputAtoms      int64   `json:"total_input_atoms"`
	Change               Change  `json:"change"`
	FeeAtoms             int64   `json:"fee_atoms"`
	UnsignedTxHex        string  `json:"unsigned_tx_hex"`
}
type Signed struct {
	Raw       string `json:"raw"`
	TxID      string `json:"txid"`
	RawSHA256 string `json:"raw_sha256"`
}
type Authorization struct {
	ActionID       string `json:"action_id"`
	ReviewID       string `json:"review_id"`
	ViewID         string `json:"view_id"`
	Generation     uint64 `json:"generation"`
	AuthorizedAtMS int64  `json:"authorized_at_ms"`
}
type AuthorizationInput struct {
	ActionID, ReviewID, ViewID string
	Generation                 uint64
}
type Dispatch struct {
	Attempt       uint32         `json:"attempt"`
	Authorization Authorization  `json:"authorization"`
	IntentAtMS    int64          `json:"intent_at_ms"`
	Result        DeliveryResult `json:"result"`
	ResultAtMS    int64          `json:"result_at_ms"`
	Error         string         `json:"error"`
}
type Observation struct {
	Version       uint32           `json:"version"`
	TxID          string           `json:"txid"`
	ObservedAtMS  int64            `json:"observed_at_ms"`
	State         ObservationState `json:"state"`
	NodeTip       string           `json:"node_tip"`
	NodeHeight    uint32           `json:"node_height"`
	Confirmations uint32           `json:"confirmations"`
	BlockHash     string           `json:"block_hash"`
	Error         string           `json:"error"`
}
type ObservationInput struct {
	State         ObservationState
	NodeTip       string
	NodeHeight    uint32
	Confirmations uint32
	BlockHash     string
	Error         string
}
type Operation struct {
	Version           uint32         `json:"version"`
	Sequence          uint32         `json:"sequence"`
	OperationID       string         `json:"operation_id"`
	Revision          uint32         `json:"revision"`
	UpdatedAtMS       int64          `json:"updated_at_ms"`
	PreparedAtMS      int64          `json:"prepared_at_ms"`
	Stage             Stage          `json:"stage"`
	Intent            Intent         `json:"intent"`
	IntentDigest      string         `json:"intent_digest"`
	Draft             *Draft         `json:"draft"`
	DraftDigest       string         `json:"draft_digest"`
	Signed            *Signed        `json:"signed"`
	SignAuthorization *Authorization `json:"sign_authorization"`
	Dispatch          *Dispatch      `json:"dispatch"`
	Observation       *Observation   `json:"observation"`
}
type Snapshot struct {
	JournalID, BindingID                     string
	Count                                    uint32
	ClockHighWaterMS                         int64
	UnresolvedOperationID, LatestOperationID string
}
type marker struct {
	Application string `json:"application"`
	Version     uint32 `json:"version"`
	JournalID   string `json:"journal_id"`
	Pin         Pin    `json:"pin"`
	BindingID   string `json:"binding_id"`
}
type state struct {
	Application      string      `json:"application"`
	Version          uint32      `json:"version"`
	JournalID        string      `json:"journal_id"`
	BindingID        string      `json:"binding_id"`
	ClockHighWaterMS int64       `json:"clock_high_water_ms"`
	Records          []Operation `json:"records"`
}
type Fence func(func(check func() error) error) error

func ownedOperation(v Operation) Operation {
	if v.Draft != nil {
		x := *v.Draft
		x.Inputs = append([]Input(nil), x.Inputs...)
		v.Draft = &x
	}
	if v.Signed != nil {
		x := *v.Signed
		v.Signed = &x
	}
	if v.SignAuthorization != nil {
		x := *v.SignAuthorization
		v.SignAuthorization = &x
	}
	if v.Dispatch != nil {
		x := *v.Dispatch
		v.Dispatch = &x
	}
	if v.Observation != nil {
		x := *v.Observation
		v.Observation = &x
	}
	return v
}
