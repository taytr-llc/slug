// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import "github.com/taytr-llc/slug/fightercondition"

// CutoffFence records a durable observation, not a signature or ownership proof.
type CutoffFence struct {
	Version        uint32 `json:"version"`
	StoreID        string `json:"store_id"`
	CutoffMS       int64  `json:"cutoff_ms"`
	ObservedAtMS   int64  `json:"observed_at_ms"`
	OperationCount uint32 `json:"operation_count"`
}
type CutoffRoster struct {
	Version        uint32     `json:"version"`
	StoreID        string     `json:"store_id"`
	CutoffMS       int64      `json:"cutoff_ms"`
	CommittedCount uint32     `json:"committed_count"`
	LastSequence   uint64     `json:"last_sequence"`
	HistoryDigest  string     `json:"history_digest"`
	Revision       uint64     `json:"revision"`
	Slots          [10]string `json:"slots"`
	AssignedAtMS   [10]int64  `json:"assigned_at_ms"`
}

// CutoffMember strips the private verified receipt and full manifest.
type CutoffMember struct {
	Slot              uint32                    `json:"slot"`
	SlotAssignedAtMS  int64                     `json:"slot_assigned_at_ms"`
	Assignment        Assignment                `json:"assignment"`
	Class             string                    `json:"class"`
	ConditionAtCutoff fightercondition.Snapshot `json:"condition_at_cutoff"`
}
type CutoffSelection struct {
	Version uint32         `json:"version"`
	Roster  CutoffRoster   `json:"roster"`
	Members []CutoffMember `json:"members"`
}
