// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import "context"

const (
	MaxWithdrawalRequestSize  = 2048
	MaxWithdrawalResponseSize = 16384
	WithdrawalMetadataBytes   = 65536
	WithdrawalWorkBytes       = 32 * 1024 * 1024
)

type WithdrawalSurfaceAPI interface {
	HandleWithdrawal(context.Context, WithdrawalRequest) WithdrawalResponse
}

type WithdrawalBinding struct {
	View                ManifestViewPin `json:"view"`
	SourceBindingID     string          `json:"source_binding_id"`
	JournalID           string          `json:"journal_id"`
	WithdrawalBindingID string          `json:"withdrawal_binding_id"`
}
type WithdrawalIntent struct {
	Destination       string `json:"destination"`
	RecipientScript   string `json:"recipient_script"`
	AmountAtoms       int64  `json:"amount_atoms"`
	FeeRateAtomsPerKB int64  `json:"fee_rate_atoms_per_kb"`
	FeeLimitAtoms     int64  `json:"fee_limit_atoms"`
}
type WithdrawalDraftView struct {
	Version              uint32 `json:"version"`
	DraftDigest          string `json:"draft_digest"`
	UnsignedTxID         string `json:"unsigned_txid"`
	Destination          string `json:"destination"`
	RecipientScript      string `json:"recipient_script"`
	AmountAtoms          int64  `json:"amount_atoms"`
	FeeAtoms             int64  `json:"fee_atoms"`
	TotalDebitAtoms      int64  `json:"total_debit_atoms"`
	InputCount           uint32 `json:"input_count"`
	InputAtoms           int64  `json:"input_atoms"`
	RecipientOutputIndex uint32 `json:"recipient_output_index"`
	ChangeAtoms          int64  `json:"change_atoms"`
	ChangeOutputIndex    int32  `json:"change_output_index"`
	PreparedAtMS         int64  `json:"prepared_at_ms"`
	ObservedTip          string `json:"observed_tip"`
	ObservedHeight       uint32 `json:"observed_height"`
}
type WithdrawalReview struct {
	Version      uint32              `json:"version"`
	Binding      WithdrawalBinding   `json:"binding"`
	OperationID  string              `json:"operation_id"`
	ActionID     string              `json:"action_id"`
	ReviewID     string              `json:"review_id"`
	Kind         string              `json:"kind"`
	Draft        WithdrawalDraftView `json:"draft"`
	TxID         string              `json:"txid"`
	ReviewedAtMS int64               `json:"reviewed_at_ms"`
	ExpiresAtMS  int64               `json:"expires_at_ms"`
}
type WithdrawalAuthorization struct {
	ActionID       string `json:"action_id"`
	ReviewID       string `json:"review_id"`
	ViewID         string `json:"view_id"`
	Generation     uint64 `json:"generation"`
	AuthorizedAtMS int64  `json:"authorized_at_ms"`
}
type WithdrawalDispatch struct {
	Attempt       uint32                  `json:"attempt"`
	Authorization WithdrawalAuthorization `json:"authorization"`
	IntentAtMS    int64                   `json:"intent_at_ms"`
	Result        string                  `json:"result"`
	ResultAtMS    int64                   `json:"result_at_ms"`
	Error         string                  `json:"error"`
}
type WithdrawalObservation struct {
	Version       uint32 `json:"version"`
	TxID          string `json:"txid"`
	ObservedAtMS  int64  `json:"observed_at_ms"`
	State         string `json:"state"`
	NodeTip       string `json:"node_tip"`
	NodeHeight    uint32 `json:"node_height"`
	Confirmations uint32 `json:"confirmations"`
	BlockHash     string `json:"block_hash"`
	Error         string `json:"error"`
}
type WithdrawalOperation struct {
	Version             uint32                 `json:"version"`
	OperationID         string                 `json:"operation_id"`
	JournalID           string                 `json:"journal_id"`
	WithdrawalBindingID string                 `json:"withdrawal_binding_id"`
	Sequence            uint32                 `json:"sequence"`
	Revision            uint32                 `json:"revision"`
	CreatedAtMS         int64                  `json:"created_at_ms"`
	UpdatedAtMS         int64                  `json:"updated_at_ms"`
	PreparedAtMS        int64                  `json:"prepared_at_ms"`
	Stage               string                 `json:"stage"`
	Intent              WithdrawalIntent       `json:"intent"`
	IntentDigest        string                 `json:"intent_digest"`
	Draft               *WithdrawalDraftView   `json:"draft"`
	TxID                string                 `json:"txid"`
	SignedSHA256        string                 `json:"signed_sha256"`
	Dispatch            *WithdrawalDispatch    `json:"dispatch"`
	Observation         *WithdrawalObservation `json:"observation"`
}
type WithdrawalAction struct {
	Version     uint32               `json:"version"`
	Binding     WithdrawalBinding    `json:"binding"`
	OperationID string               `json:"operation_id"`
	ActionID    string               `json:"action_id"`
	Kind        string               `json:"kind"`
	State       string               `json:"state"`
	ReviewID    string               `json:"review_id"`
	Error       string               `json:"error"`
	Operation   *WithdrawalOperation `json:"operation"`
}
type WithdrawalSnapshot struct {
	Version               uint32             `json:"version"`
	State                 string             `json:"state"`
	Error                 string             `json:"error"`
	Binding               *WithdrawalBinding `json:"binding"`
	ActiveActionID        string             `json:"active_action_id"`
	LastActionID          string             `json:"last_action_id"`
	UnresolvedOperationID string             `json:"unresolved_operation_id"`
	LatestOperationID     string             `json:"latest_operation_id"`
	CanPrepare            bool               `json:"can_prepare"`
	CanReview             bool               `json:"can_review"`
}
type WithdrawalRequest struct {
	ID          uint64            `json:"id"`
	Version     uint32            `json:"version"`
	Command     string            `json:"command"`
	ViewID      string            `json:"view_id"`
	OperationID string            `json:"operation_id"`
	ActionID    string            `json:"action_id"`
	ReviewID    string            `json:"review_id"`
	ReviewKind  string            `json:"review_kind"`
	DraftDigest string            `json:"draft_digest"`
	TxID        string            `json:"txid"`
	Intent      *WithdrawalIntent `json:"intent"`
}
type WithdrawalResponse struct {
	ID        uint64               `json:"id"`
	Version   uint32               `json:"version"`
	OK        bool                 `json:"ok"`
	Snapshot  *WithdrawalSnapshot  `json:"snapshot"`
	Operation *WithdrawalOperation `json:"operation"`
	Review    *WithdrawalReview    `json:"review"`
	Action    *WithdrawalAction    `json:"action"`
	Error     string               `json:"error"`
}

func BlankWithdrawalSnapshot() WithdrawalSnapshot {
	return WithdrawalSnapshot{Version: 1, State: "unavailable", Error: "withdrawal_unavailable"}
}
func WithdrawalFailure(id uint64, code string) WithdrawalResponse {
	if id == 0 {
		id = 1
	}
	return WithdrawalResponse{ID: id, Version: 1, Error: code}
}
