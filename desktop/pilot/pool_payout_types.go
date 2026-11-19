// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/poolsettle"
)

const (
	MaxPoolPayoutRequestSize  = 1024
	MaxPoolPayoutResponseSize = 12288
	PoolPayoutCaptureBytes    = 131072
	PoolPayoutScratchBytes    = 2 * 1024 * 1024
)

var (
	ErrPayoutUnavailable  = errors.New("payout history unavailable")
	ErrPayoutNotFound     = errors.New("payout capture not found")
	ErrPayoutConflict     = errors.New("payout operation conflict")
	ErrPayoutReplyUnknown = errors.New("payout admission reply unknown")
)

type PoolPayoutSurfaceAPI interface {
	PoolPayoutSnapshot(context.Context) (PoolPayoutSnapshot, error)
	BeginPoolPayoutHistory(context.Context, string, string) (PoolPayoutJob, error)
	PoolPayoutJob(context.Context, string, string) (PoolPayoutJob, error)
	PoolPayoutPage(context.Context, string, string, uint32, uint32) (PoolPayoutPage, error)
	CancelPoolPayoutHistory(context.Context, string, string) (PoolPayoutJob, error)
	ReadPoolPayoutTransaction(context.Context, string, string, string, uint32) (PoolPayoutObservation, error)
}
type PoolPayoutBinding struct {
	View            ManifestViewPin `json:"view"`
	SourceBindingID string          `json:"source_binding_id"`
	RecipientScript string          `json:"recipient_script"`
}
type PoolPayoutCapture struct {
	Version                uint32 `json:"version"`
	Count                  uint32 `json:"count"`
	PaidAtoms              int64  `json:"paid_atoms"`
	ReservedAtoms          int64  `json:"reserved_atoms"`
	SettlementObservedAtMS int64  `json:"settlement_observed_at_ms"`
	CapturedAtMS           int64  `json:"captured_at_ms"`
}
type PoolPayoutRequest struct {
	ID          uint64 `json:"id"`
	Version     uint32 `json:"version"`
	Command     string `json:"command"`
	ViewID      string `json:"view_id"`
	JobID       string `json:"job_id"`
	After       uint32 `json:"after"`
	Limit       uint32 `json:"limit"`
	TxID        string `json:"txid"`
	OutputIndex uint32 `json:"output_index"`
}
type PoolPayoutResponse struct {
	ID          uint64                 `json:"id"`
	Version     uint32                 `json:"version"`
	OK          bool                   `json:"ok"`
	Snapshot    *PoolPayoutSnapshot    `json:"snapshot"`
	Job         *PoolPayoutJob         `json:"job"`
	Page        *PoolPayoutPage        `json:"page"`
	Observation *PoolPayoutObservation `json:"observation"`
	Error       string                 `json:"error"`
}
type PoolPayoutSnapshot struct {
	Version      uint32             `json:"version"`
	State        string             `json:"state"`
	Error        string             `json:"error"`
	Binding      *PoolPayoutBinding `json:"binding"`
	Activity     string             `json:"activity"`
	ActiveJobID  string             `json:"active_job_id"`
	LastJobID    string             `json:"last_job_id"`
	CaptureJobID string             `json:"capture_job_id"`
}
type PoolPayoutJob struct {
	Version uint32             `json:"version"`
	JobID   string             `json:"job_id"`
	State   string             `json:"state"`
	Binding PoolPayoutBinding  `json:"binding"`
	Error   string             `json:"error"`
	Capture *PoolPayoutCapture `json:"capture"`
}
type PoolPayoutPage struct {
	Version uint32                  `json:"version"`
	Binding PoolPayoutBinding       `json:"binding"`
	JobID   string                  `json:"job_id"`
	Capture PoolPayoutCapture       `json:"capture"`
	After   uint32                  `json:"after"`
	Limit   uint32                  `json:"limit"`
	Next    uint32                  `json:"next"`
	HasMore bool                    `json:"has_more"`
	Items   []poolsettle.PayoutItem `json:"items"`
}
type PoolPayoutObservation struct {
	Version       uint32                `json:"version"`
	Binding       PoolPayoutBinding     `json:"binding"`
	JobID         string                `json:"job_id"`
	Capture       PoolPayoutCapture     `json:"capture"`
	Payment       poolsettle.PayoutItem `json:"payment"`
	ObservedAtMS  int64                 `json:"observed_at_ms"`
	State         string                `json:"state"`
	Error         string                `json:"error"`
	WalletSynced  bool                  `json:"wallet_synced"`
	WalletHeight  uint32                `json:"wallet_height"`
	WalletTip     string                `json:"wallet_tip"`
	Confirmations uint32                `json:"confirmations"`
	BlockHash     string                `json:"block_hash"`
}

func BlankPoolPayoutSnapshot() PoolPayoutSnapshot {
	return PoolPayoutSnapshot{Version: 1, State: "unavailable", Error: "payout_unavailable", Activity: "none"}
}
