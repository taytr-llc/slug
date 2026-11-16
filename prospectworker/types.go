// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

// Package prospectworker runs a bounded automatic miner with a durable public
// outbox. It never reads wallet keys and never enables incompatible test PoW.
package prospectworker

import (
	"context"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
)

const Version1 uint32 = 1

// Config pins public mining identity and bounds all retained public evidence.
// Capacity is operational storage, not a limit on prospect eligibility.
type Config struct {
	Version         uint32              `json:"version"`
	Policy          prospectnode.Policy `json:"policy"`
	PublicKey       string              `json:"public_key"`
	PayoutAddress   string              `json:"payout_address"`
	AuthorityOrigin string              `json:"authority_origin"`
	MaxRecords      uint64              `json:"max_records"`
	MaxBytes        uint64              `json:"max_bytes"`
}

type Authority interface {
	ProtocolVersion() uint32
	AuthorityOrigin() string
	IssueJob(context.Context, string) (prospectjob.Job, error)
	Submit(context.Context, prospectjob.Job, prospect.Proof, gymauth.Authorization) (prospectservice.AdmissionResult, error)
}

type Node interface {
	ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error)
	Proposal(context.Context, prospect.Context, string) error
	RecheckContext(context.Context, prospect.Context) error
	SubmitBlock(context.Context, prospect.Context, string) error
	CanonicalBlock(context.Context, prospect.Context, prospect.Proof, string) error
}

// Signer is a cooperative, in-memory wallet adapter. It must honor cancellation.
// The worker validates and synchronizes its public output before submission.
type Signer func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error)

// Entry holds immutable work and separate block/admission delivery outcomes.
// A transport error never changes the job or replaces a saved authorization.
type Entry struct {
	Version        uint32                           `json:"version"`
	Sequence       uint64                           `json:"sequence"`
	WorkHash       string                           `json:"work_hash"`
	Job            prospectjob.Job                  `json:"job"`
	Candidate      prospectwork.Candidate           `json:"candidate"`
	BlockWin       bool                             `json:"block_win"`
	ProspectWin    bool                             `json:"prospect_win"`
	Authorization  *gymauth.Authorization           `json:"authorization,omitempty"`
	Receipt        *prospectservice.AdmissionResult `json:"receipt,omitempty"`
	BlockState     string                           `json:"block_state"`
	AdmissionState string                           `json:"admission_state"`
	LastError      string                           `json:"last_error,omitempty"`
	NextAttemptMS  int64                            `json:"next_attempt_ms"`
	RetryCount     uint32                           `json:"retry_count"`
}

// Scan is synchronized before hashing. After a crash the same unfinished nonce
// is retried; a found record and advancement are committed atomically.
type Scan struct {
	Version    uint32                 `json:"version"`
	Job        prospectjob.Job        `json:"job"`
	Template   prospectwork.Template  `json:"template"`
	Candidate  prospectwork.Candidate `json:"candidate"`
	ExtraNonce uint64                 `json:"extra_nonce"`
	NextNonce  uint64                 `json:"next_nonce"`
}

type Status struct {
	State           string  `json:"state"`
	JobID           string  `json:"job_id,omitempty"`
	ParentHash      string  `json:"parent_hash,omitempty"`
	Attempts        uint64  `json:"attempts"`
	HashesPerSecond float64 `json:"hashes_per_second"`
	Pending         uint64  `json:"pending"`
	Records         uint64  `json:"records"`
	Bytes           uint64  `json:"bytes"`
	LastError       string  `json:"last_error,omitempty"`
	RecoveryError   string  `json:"recovery_error,omitempty"`
	NextRecoveryMS  int64   `json:"next_recovery_ms,omitempty"`
}
