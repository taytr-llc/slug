// Copyright (c) 2026 The SLUG developers. ISC license.
package btcjson

// GetMempoolEntrySnapshotResult is the supported SLUG getmempoolentry
// projection. It does not invent unavailable ancestry or priority statistics.
// Fee retains the inherited RPC coin-unit convention, not integer authority.
type GetMempoolEntrySnapshotResult struct {
	Size    int32    `json:"size"`
	Vsize   int32    `json:"vsize"`
	Weight  int64    `json:"weight"`
	Height  int64    `json:"height"`
	Wtxid   string   `json:"wtxid"`
	Time    int64    `json:"time"`
	Fee     float64  `json:"fee"`
	Depends []string `json:"depends"`
}
