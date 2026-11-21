// Copyright (c) 2026 The SLUG developers. ISC license.
// Package poolshare retains issued pool work and accepted-share evidence.
// It performs no balance accounting, payout or fighter admission.
package poolshare

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

const Version1 uint32 = 1
const Version2 uint32 = 2

var (
	ErrConfig         = errors.New("invalid pool configuration")
	ErrProfile        = errors.New("pool shares require production PoW")
	ErrClosed         = errors.New("pool journal is closed")
	ErrBusy           = errors.New("pool operation is busy")
	ErrPoisoned       = errors.New("pool write outcome uncertain; close and reopen")
	ErrCorrupt        = errors.New("pool journal is corrupt or its source changed")
	ErrCapacity       = errors.New("pool journal capacity reached")
	ErrClock          = errors.New("pool clock is invalid or moved backward")
	ErrExpired        = errors.New("ordinary pool share job expired")
	ErrUnauthorized   = errors.New("pool session is unauthorized")
	ErrOutcomeUnknown = errors.New("pool operation outcome is unknown; recover original work")
	ErrNotFound       = errors.New("pool evidence not found")
	ErrNode           = errors.New("pool node observation unavailable")
	ErrLowWork        = errors.New("work meets neither pool nor block target")
	ErrInvalid        = errors.New("invalid pool request")
)

// Config is trusted operator configuration. This candidate journal uses one
// dedicated manifest authority; all issuer jobs count toward MaxWorks, including
// issuance whose later proposal or acknowledgment failed. Capacities never evict
// evidence. MaxBytes reserves a full status slot for every retained submission;
// LevelDB/index/compaction overhead is additional.
type Config struct {
	Version        uint32                        `json:"version"`
	AuthorityPin   registry.ManifestAuthorityPin `json:"authority_pin"`
	Policy         prospectnode.Policy           `json:"policy"`
	PayoutAddress  string                        `json:"payout_address"`
	PoolTarget     string                        `json:"pool_target"`
	MaxWorks       uint64                        `json:"max_works"`
	MaxSubmissions uint64                        `json:"max_submissions"`
	MaxBytes       uint64                        `json:"max_bytes"`
}

// Node is an explicitly trusted adapter, normally *prospectnode.Client.
// CanonicalBlock receives only locally recomputed work, never claimant summaries.
type Node interface {
	ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error)
	Proposal(context.Context, prospect.Context, string) error
	RecheckContext(context.Context, prospect.Context) error
	SubmitBlock(context.Context, prospect.Context, string) error
	CanonicalBlock(context.Context, prospect.Context, prospect.Proof, string) error
}

// Work owns an unmined candidate. It remains subject to its original job's
// lifetime; possessing it authenticates no account or fighter admission.
type Work struct {
	Job        prospectjob.ManifestJob `json:"job"`
	PoolTarget string                  `json:"pool_target"`
	Candidate  prospectwork.Candidate  `json:"candidate"`
}

// Submission separates immutable share acceptance from block-delivery history.
// AcceptedSequence zero means no share has been accepted. Canonical-observed is
// a historical node observation, not current membership, finality or spendability.
// A late winning share belongs to WorkHash/BlockHeight's round; receipt ordering
// alone must not assign its future D79 reward. No field grants fighter birth.
type Submission struct {
	Version           uint32 `json:"version"`
	Sequence          uint64 `json:"sequence"`
	AcceptedSequence  uint64 `json:"accepted_sequence"`
	JobID             string `json:"job_id"`
	GymID             string `json:"gym_id"`
	Nonce             uint32 `json:"nonce"`
	PoolTarget        string `json:"pool_target"`
	Header            string `json:"header"`
	WorkHash          string `json:"work_hash"`
	DuplicateKey      string `json:"duplicate_key"`
	Pool              bool   `json:"pool"`
	Prospect          bool   `json:"prospect"`
	Block             bool   `json:"block"`
	BlockHeight       uint32 `json:"block_height"`
	ReceivedAtMS      int64  `json:"received_at_ms"`
	AcceptedAtMS      int64  `json:"accepted_at_ms"`
	Acceptance        string `json:"acceptance"`
	BlockState        string `json:"block_state"`
	BlockObservedAtMS int64  `json:"block_observed_at_ms"`
}

type savedWork struct {
	Job      prospectjob.ManifestJob `json:"job"`
	Template prospectwork.Template   `json:"template"`
}

type metadata struct {
	Application string `json:"application"`
	Version     uint32 `json:"version"`
	Config      Config `json:"config"`
	Works       uint64 `json:"works"`
	Submissions uint64 `json:"submissions"`
	Accepted    uint64 `json:"accepted"`
	Bytes       uint64 `json:"bytes"`
	LastClockMS int64  `json:"last_clock_ms"`
}

// Pool serializes operations without a waiting queue. Close refuses new work
// and drains the active operation. The caller retains ownership of the concrete
// issuer, node and session adapter and must keep them alive until Close returns.
// The dedicated issuer must not issue jobs outside this pool during its lifetime;
// its total job count bounds incomplete issuance as well as returned pool work.
// No method may be reentered by its adapters or session callbacks.
type Pool struct {
	operatorAdmission *OperatorAdmission // original gate; never detached or replaced
	auditOnly         bool
	db                *leveldb.DB
	meta              metadata
	config            Config
	authority         *registry.ManifestStore
	node              Node
	sessions          prospectservice.SessionAuthorizer
	gate              chan struct{}
	lifecycle         sync.Mutex
	closing           bool
	closeDone         chan struct{}
	closeErr          error
	failedFile        *os.File                    // V2 constructor close failure; retained, never retried
	nonceMemo         map[[32]byte]nonceMemoValue // V2 retained receipts only; original gate
	poisoned          bool
	clockSeen         int64
	now               func() time.Time
	writeBatch        func(*leveldb.Batch, *opt.WriteOptions) error
	checkpoint        func(string)
}
