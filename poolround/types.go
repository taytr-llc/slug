// Copyright (c) 2026 The SLUG developers. ISC license.
// Package poolround retains fixed pool rounds and exact gross allocations.
// It does not round amounts, deduct fees, determine spendability or pay coins.
package poolround

import (
	"context"
	"errors"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
)

const Version1 uint32 = 1
const Version2 uint32 = 2
const MaxPage = 8

var (
	ErrProfile           = errors.New("pool rounds require production PoW")
	ErrInvalid           = errors.New("invalid pool round request")
	ErrConfig            = errors.New("invalid pool round configuration")
	ErrClosed            = errors.New("pool round journal is closed")
	ErrBusy              = errors.New("pool round operation is busy")
	ErrPoisoned          = errors.New("pool round write outcome uncertain; close and reopen")
	ErrCorrupt           = errors.New("pool round journal is corrupt")
	ErrSourceChanged     = errors.New("pool acceptance source changed")
	ErrSourceUnavailable = errors.New("pool acceptance source unavailable")
	ErrCapacity          = errors.New("pool round journal capacity reached")
	ErrNotFound          = errors.New("pool round not found")
	ErrLiveStale         = errors.New("live round verification is stale")
	ErrNode              = errors.New("fresh pool block observation unavailable")
)

type Limits struct {
	Version   uint32 `json:"version"`
	MaxRounds uint64 `json:"max_rounds"`
	MaxBytes  uint64 `json:"max_bytes"`
}

// Observer is trusted operator configuration, normally *prospectnode.Client.
// Inputs come only from the concrete pool source's locally verified work.
type Observer interface {
	ObservePoolBlock(context.Context, prospect.Context, prospect.Proof, string) (prospectnode.PoolBlockObservation, error)
}

// Allocation is an exact gross number of atomic units. No rounding, fee or
// carry-forward policy is hidden in Numerator/Denominator. The pinned source
// uses one share target, so Shares is the count of equally weighted shares.
type Allocation struct {
	GymID       string `json:"gym_id"`
	Shares      uint64 `json:"shares"`
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
}

type Round struct {
	Version       uint32                    `json:"version"`
	Sequence      uint64                    `json:"sequence"`
	BlockHash     string                    `json:"block_hash"`
	BlockHeight   uint32                    `json:"block_height"`
	FirstAccepted uint64                    `json:"first_accepted"`
	LastAccepted  uint64                    `json:"last_accepted"`
	ShareKeys     []string                  `json:"share_keys"`
	PoolTarget    string                    `json:"pool_target"`
	PayoutAddress string                    `json:"payout_address"`
	RewardAtoms   int64                     `json:"reward_atoms"`
	Allocations   []Allocation              `json:"allocations"`
	Winner        poolshare.AccountingEntry `json:"winner"`
}

// Record keeps the immutable allocation plan, latest successful observation
// and first sixty-descendant confirmation separately. Observations are historical
// evidence, never a current funding/spendability guarantee. Read failure does
// not erase an old observation or manufacture an orphan report.
type Record struct {
	Round        Round                              `json:"round"`
	Observation  *prospectnode.PoolBlockObservation `json:"observation"`
	Confirmation *prospectnode.PoolBlockObservation `json:"confirmation"`
}

func (r Record) Status() string {
	if r.Confirmation != nil {
		if r.Observation != nil && !r.Observation.Canonical {
			return "funding-conflict"
		}
		return "confirmed"
	}
	if r.Observation == nil {
		return "unobserved"
	}
	if !r.Observation.Canonical {
		return "orphaned"
	}
	return "provisional"
}

// ConfirmedEarnings remain locked through a later reported funding conflict.
// They do not authorize a payment or an advance from other funds.
func (r Record) ConfirmedEarnings() []Allocation {
	if r.Confirmation == nil {
		return nil
	}
	return append([]Allocation(nil), r.Round.Allocations...)
}

// ProvisionalEarnings disappear while an unconfirmed block is noncanonical.
// The original allocation plan and members remain unchanged in Round.
func (r Record) ProvisionalEarnings() []Allocation {
	if r.Confirmation != nil || r.Observation == nil || !r.Observation.Canonical {
		return nil
	}
	return append([]Allocation(nil), r.Round.Allocations...)
}

type Summary struct {
	Through       uint64 `json:"through"`
	Rounds        uint64 `json:"rounds"`
	PendingShares uint64 `json:"pending_shares"`
}

type metadata struct {
	Application string           `json:"application"`
	Version     uint32           `json:"version"`
	Limits      Limits           `json:"limits"`
	Source      poolshare.Config `json:"source"`
	Through     uint64           `json:"through"`
	Prefix      string           `json:"prefix"`
	Rounds      uint64           `json:"rounds"`
	Bytes       uint64           `json:"bytes"`
}

// Store is an operator-only in-process accountant. Keep its concrete pool and
// trusted observer alive until Close drains active work. No adapter callback
// may reenter this store. There is no public transport or caller receipt import.
type Store struct {
	auditOnly    bool
	db           *leveldb.DB
	meta         metadata
	limits       Limits
	source       *poolshare.Pool
	sourceConfig poolshare.Config
	node         Observer
	gate         chan struct{}
	lifecycle    sync.Mutex
	closing      bool
	closeDone    chan struct{}
	closeErr     error
	poisoned     bool
	live         *liveDirectory
	writeBatch   func(*leveldb.Batch, *opt.WriteOptions) error
}
