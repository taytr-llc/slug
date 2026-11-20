// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import "errors"

const (
	journalApplication = "slug-fighter-carrier-journal"
	journalMarkerName  = "SLUG-FIGHTER-CARRIER-JOURNAL.json"
	journalStateName   = "state.json"
	journalMaxRecords  = 128
	journalMaxBytes    = 8 << 20
)

var (
	ErrBusy           = errors.New("fighter carrier journal busy")
	ErrClosed         = errors.New("fighter carrier journal closed")
	ErrNotFound       = errors.New("fighter carrier request not found")
	ErrConflict       = errors.New("fighter carrier conflict")
	ErrClock          = errors.New("fighter carrier clock unavailable")
	ErrCorrupt        = errors.New("fighter carrier journal corrupt")
	ErrStorage        = errors.New("fighter carrier journal storage failed")
	ErrUnauthorized   = errors.New("fighter carrier original authorization unavailable")
	ErrPoisoned       = errors.New("fighter carrier journal requires reopen")
	ErrOutcomeUnknown = errors.New("fighter carrier publication outcome unknown")
	ErrProfile        = errors.New("fighter carrier journal unsupported profile")
)

// Identity is supplied by the retained setup or independently selected restored
// owner. Open never adopts either identity field from a marker on disk.
type Identity struct {
	JournalID string `json:"JournalID"`
	Pin       Pin    `json:"Pin"`
}

type Snapshot struct {
	JournalID, BindingID             string
	Count                            uint32
	ClockHighWaterMS                 int64
	ActiveRequestID, LatestRequestID string
}

type journalMarker struct {
	Application string `json:"application"`
	Version     uint32 `json:"version"`
	JournalID   string `json:"journal_id"`
	Pin         Pin    `json:"pin"`
	BindingID   string `json:"binding_id"`
}

type journalState struct {
	Application      string      `json:"application"`
	Version          uint32      `json:"version"`
	JournalID        string      `json:"journal_id"`
	BindingID        string      `json:"binding_id"`
	ClockHighWaterMS int64       `json:"clock_high_water_ms"`
	Records          []Operation `json:"records"`
}

// Fence is the original native action's synchronous publication guard. A
// successful callback is not wallet authority, a signature or chain acceptance.
// PreparedCommit requires exactly one invocation and joins a broken asynchronous
// invocation before relinquishing its file owner.
type Fence func(func(check func() error) error) error
