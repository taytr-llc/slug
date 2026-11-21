// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"github.com/taytr-llc/slug/poolshare"
	"os"
)

const (
	dailyApplication           = "slug-pool-settlement-daily"
	dailyMarkerName            = "SLUG-POOL-SETTLEMENT.json"
	dailyHeadLimit             = 128 << 10
	dailyReportLimit           = 8 << 20
	dailyCreditLimit           = 128 << 10
	dailyBatchLimit            = 2 << 20
	dailyReferenceLimit        = 256
	dailyLogicalLimit   uint64 = 8 << 30
	dailyPageLimit             = 8
)

type dailyMarker struct {
	Application string           `json:"application"`
	Version     uint32           `json:"version"`
	Config      Config           `json:"config"`
	Source      poolshare.Config `json:"source"`
	ConfigID    string           `json:"config_id"`
	SourceID    string           `json:"source_id"`
}

type dailyHeader struct {
	Application  string `json:"application"`
	Version      uint32 `json:"version"`
	ConfigID     string `json:"config_id"`
	SourceID     string `json:"source_id"`
	Revision     uint64 `json:"revision"`
	Credits      uint64 `json:"credits"`
	Batches      uint64 `json:"batches"`
	Events       uint64 `json:"events"`
	LogicalBytes uint64 `json:"logical_bytes"`
	PendingBatch uint64 `json:"pending_batch"`
	LastDay      string `json:"last_day"`
	LastClockMS  int64  `json:"last_clock_ms"`
}

type dailyInput struct {
	TxID  string `json:"txid"`
	Index uint32 `json:"index"`
}

type dailyChange struct {
	Index        uint32 `json:"index"`
	Atoms        int64  `json:"atoms"`
	Script       string `json:"script"`
	ClaimCount   uint64 `json:"claim_count"`
	ClaimsDigest string `json:"claims_digest"`
}

// dailyBatchBody stores original signed bytes once. Complete input/change
// claims are reconstructed from audited preceding events, never caller DTOs.
type dailyBatchBody struct {
	Version     uint32       `json:"version"`
	Sequence    uint64       `json:"sequence"`
	Day         string       `json:"day"`
	CreatedAtMS int64        `json:"created_at_ms"`
	Inputs      []dailyInput `json:"inputs"`
	Payments    []Payment    `json:"payments"`
	FeeAtoms    int64        `json:"fee_atoms"`
	Change      *dailyChange `json:"change"`
	TxID        string       `json:"txid"`
	Raw         string       `json:"raw"`
}

type dailyDelivery struct {
	State string `json:"state"`
}
type dailyReference struct {
	Kind     string `json:"kind"`
	Sequence uint64 `json:"sequence"`
}

// dailyState holds identity and the last successfully committed small header.
// It is not an authenticated frontier cache. Every operation must audit the
// original current DB and source before publishing financial state.
type dailyState struct {
	failedFiles []*os.File
	closeErr    error
	path        string
	marker      dailyMarker
	head        dailyHeader
	report      Report
}
