// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
)

const (
	MaxWalletActivityRequestSize       = 1024
	MaxWalletActivitySmallResponseSize = 16384
	MaxWalletActivityResponseSize      = 98304
	WalletActivityScratchBytes         = 32 * 1024 * 1024
	WalletActivityMaxItems             = 256
)

var (
	ErrActivityUnavailable  = errors.New("wallet activity unavailable")
	ErrActivityNotFound     = errors.New("wallet activity capture not found")
	ErrActivityConflict     = errors.New("wallet activity operation conflict")
	ErrActivityReplyUnknown = errors.New("wallet activity admission reply unknown")
)

type WalletActivitySurfaceAPI interface {
	WalletActivitySnapshot(context.Context) (WalletActivitySnapshot, error)
	BeginWalletActivity(context.Context, string, string) (WalletActivityJob, error)
	WalletActivityJob(context.Context, string, string) (WalletActivityJob, error)
	WalletActivityPage(context.Context, string, string, uint32, uint32) (WalletActivityPage, error)
	WalletActivityDetail(context.Context, string, string, string) (WalletActivityDetail, error)
	CancelWalletActivity(context.Context, string, string) (WalletActivityJob, error)
}
type WalletActivityBinding struct {
	View            ManifestViewPin `json:"view"`
	SourceBindingID string          `json:"source_binding_id"`
	RootPublicKey   string          `json:"root_public_key"`
	GymPublicKey    string          `json:"gym_public_key"`
	GymID           string          `json:"gym_id"`
}
type WalletActivityCapture struct {
	Version      uint32 `json:"version"`
	CapturedAtMS int64  `json:"captured_at_ms"`
	WalletHeight uint32 `json:"wallet_height"`
	WalletTip    string `json:"wallet_tip"`
	NodeHeight   uint32 `json:"node_height"`
	NodeTip      string `json:"node_tip"`
	Count        uint32 `json:"count"`
	HasOlder     bool   `json:"has_older"`
}
type WalletActivityBlock struct {
	Height uint32 `json:"height"`
	Hash   string `json:"hash"`
	TimeMS int64  `json:"time_ms"`
}
type WalletActivityItem struct {
	TxID             string               `json:"txid"`
	RawSHA256        string               `json:"raw_sha256"`
	RecordedAtMS     int64                `json:"recorded_at_ms"`
	Coinbase         bool                 `json:"coinbase"`
	Block            *WalletActivityBlock `json:"block"`
	Confirmations    uint32               `json:"confirmations"`
	ReceivedAtoms    int64                `json:"received_atoms"`
	SpentAtoms       int64                `json:"spent_atoms"`
	ChangeAtoms      int64                `json:"change_atoms"`
	NetAtoms         int64                `json:"net_atoms"`
	FeeAtoms         *int64               `json:"fee_atoms"`
	InputCount       uint32               `json:"input_count"`
	OutputCount      uint32               `json:"output_count"`
	OwnedInputCount  uint32               `json:"owned_input_count"`
	OwnedOutputCount uint32               `json:"owned_output_count"`
}
type WalletActivityOutput struct {
	Index   uint32 `json:"index"`
	Atoms   int64  `json:"atoms"`
	Owned   bool   `json:"owned"`
	Change  bool   `json:"change"`
	Address string `json:"address"`
}
type WalletActivitySnapshot struct {
	Version      uint32                 `json:"version"`
	State        string                 `json:"state"`
	Error        string                 `json:"error"`
	Binding      *WalletActivityBinding `json:"binding"`
	Activity     string                 `json:"activity"`
	ActiveJobID  string                 `json:"active_job_id"`
	LastJobID    string                 `json:"last_job_id"`
	CaptureJobID string                 `json:"capture_job_id"`
}
type WalletActivityJob struct {
	Version uint32                 `json:"version"`
	JobID   string                 `json:"job_id"`
	State   string                 `json:"state"`
	Binding WalletActivityBinding  `json:"binding"`
	Error   string                 `json:"error"`
	Capture *WalletActivityCapture `json:"capture"`
}
type WalletActivityPage struct {
	Version uint32                `json:"version"`
	Binding WalletActivityBinding `json:"binding"`
	JobID   string                `json:"job_id"`
	Capture WalletActivityCapture `json:"capture"`
	After   uint32                `json:"after"`
	Limit   uint32                `json:"limit"`
	Next    uint32                `json:"next"`
	HasMore bool                  `json:"has_more"`
	Items   []WalletActivityItem  `json:"items"`
}
type WalletActivityDetail struct {
	Version uint32                 `json:"version"`
	Binding WalletActivityBinding  `json:"binding"`
	JobID   string                 `json:"job_id"`
	Capture WalletActivityCapture  `json:"capture"`
	Item    WalletActivityItem     `json:"item"`
	Outputs []WalletActivityOutput `json:"outputs"`
}
type WalletActivityRequest struct {
	ID      uint64 `json:"id"`
	Version uint32 `json:"version"`
	Command string `json:"command"`
	ViewID  string `json:"view_id"`
	JobID   string `json:"job_id"`
	After   uint32 `json:"after"`
	Limit   uint32 `json:"limit"`
	TxID    string `json:"txid"`
}
type WalletActivityResponse struct {
	ID       uint64                  `json:"id"`
	Version  uint32                  `json:"version"`
	OK       bool                    `json:"ok"`
	Snapshot *WalletActivitySnapshot `json:"snapshot"`
	Job      *WalletActivityJob      `json:"job"`
	Page     *WalletActivityPage     `json:"page"`
	Detail   *WalletActivityDetail   `json:"detail"`
	Error    string                  `json:"error"`
}

func BlankWalletActivitySnapshot() WalletActivitySnapshot {
	return WalletActivitySnapshot{Version: 1, State: "unavailable", Error: "activity_unavailable", Activity: "none"}
}
