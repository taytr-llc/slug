// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import "github.com/taytr-llc/slug/gymstable"

type CutoffEntry struct {
	Version        uint32   `json:"version"`
	StoreID        string   `json:"store_id"`
	Event          Event    `json:"event"`
	EntryID        string   `json:"entry_id"`
	CommittedCount uint32   `json:"committed_count"`
	LastSequence   uint64   `json:"last_sequence"`
	HistoryDigest  string   `json:"history_digest"`
	LatestReceipt  *Receipt `json:"latest_receipt"`
}
type CutoffSnapshot struct {
	Version uint32                   `json:"version"`
	Entry   CutoffEntry              `json:"entry"`
	Roster  gymstable.CutoffRoster   `json:"roster"`
	Members []gymstable.CutoffMember `json:"members"`
}
type CutoffFences struct {
	Version     uint32                `json:"version"`
	StableFence gymstable.CutoffFence `json:"stable_fence"`
	EntryFence  gymstable.CutoffFence `json:"entry_fence"`
}
type SnapshotSourceInfo struct {
	Version      uint32 `json:"version"`
	StoreID      string `json:"store_id"`
	Pin          Pin    `json:"pin"`
	ObservedAtMS int64  `json:"observed_at_ms"`
}
