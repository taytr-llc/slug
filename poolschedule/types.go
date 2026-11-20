// Copyright (c) 2026 The SLUG developers. ISC license.
// Package poolschedule schedules one original concrete settlement journal. Its
// cursor records attempts, never a second financial ledger or payment authority.
package poolschedule

import (
	"context"
	"errors"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/poolsettle"
)

const Version1 uint32 = 1
const MaxTimeMS int64 = 253402300799999
const markerName = "SLUG-POOL-SCHEDULE.json"
const maxMarkerBytes = 2048
const maxStateBytes = 4096

var stateKey = []byte("state")
var (
	ErrInvalid     = errors.New("invalid payout schedule")
	ErrProfile     = errors.New("payout schedule requires production PoW")
	ErrBusy        = errors.New("payout schedule busy")
	ErrClosed      = errors.New("payout schedule closed")
	ErrClock       = errors.New("payout schedule clock refused")
	ErrCapacity    = errors.New("payout schedule capacity reached")
	ErrStorage     = errors.New("payout schedule storage unavailable")
	ErrPoisoned    = errors.New("payout schedule outcome unknown")
	ErrUnavailable = errors.New("payout schedule original settlement unavailable")
)

type Config struct {
	Version             uint32 `json:"version"`
	SchedulerID         string `json:"scheduler_id"`
	RuntimeID           string `json:"runtime_id"`
	SettlementBindingID string `json:"settlement_binding_id"`
	DailyUTCMinute      uint32 `json:"daily_utc_minute"`
	RetryMS             uint32 `json:"retry_ms"`
}
type Attempt struct {
	Sequence     uint32 `json:"sequence"`
	Kind         string `json:"kind"`
	AdmittedDay  string `json:"admitted_day"`
	StartedAtMS  int64  `json:"started_at_ms"`
	FinishedAtMS int64  `json:"finished_at_ms"`
	Result       string `json:"result"`
	ReportState  string `json:"report_state"`
	ReportReason string `json:"report_reason"`
	ErrorCode    string `json:"error_code"`
}
type State struct {
	Version     uint32   `json:"version"`
	Config      Config   `json:"config"`
	Revision    uint32   `json:"revision"`
	LastClockMS int64    `json:"last_clock_ms"`
	DailyCursor string   `json:"daily_cursor"`
	NextWakeMS  int64    `json:"next_wake_ms"`
	Attempt     *Attempt `json:"attempt"`
}

// With a nonnil error, State is only the last successfully audited snapshot;
// an uncertain write must be reconciled through the original settlement owner.
type Status struct {
	State    State `json:"state"`
	Running  bool  `json:"running"`
	Poisoned bool  `json:"poisoned"`
}

type Scheduler struct {
	mu        sync.Mutex
	store     *poolsettle.Store
	files     *scheduleFiles
	db        *leveldb.DB
	state     State
	running   bool
	closing   bool
	closed    bool
	poisoned  bool
	runCancel context.CancelFunc
	runDone   chan struct{}
	closeErr  error
	dbClosed  bool
	claimed   bool
}

// CleanupError retains a constructor's exact partially opened owner if its
// cleanup did not succeed. No replacement owner may assume its process claim.
// Close is sticky for an actual resource-close error; it never retries an OS
// descriptor whose first Close failed.
type CleanupError struct {
	owner *Scheduler
	cause error
}

func (e *CleanupError) Error() string { return "payout schedule cleanup incomplete" }
func (e *CleanupError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}
func (e *CleanupError) Close(ctx context.Context) error {
	if e == nil || e.owner == nil {
		return ErrClosed
	}
	return e.owner.Close(ctx)
}

func cloneState(s State) State {
	if s.Attempt != nil {
		a := *s.Attempt
		s.Attempt = &a
	}
	return s
}
