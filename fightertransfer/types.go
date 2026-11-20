// Copyright (c) 2026 The SLUG developers. ISC license.

// Package fightertransfer validates public supplied carrier bytes. It does not
// establish chain inclusion, source authority, effective ownership or finality.
// Those facts require the concrete public ownership replay owner. No API signs,
// chooses funds, opens wallets, or accepts a caller's verified/canonical flag.
package fightertransfer

import (
	"errors"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

const (
	Version               = 1
	IntentSize            = 129
	EnvelopeSize          = 202
	RedeemSize            = 240
	MaxInputs             = 16
	MaxRawSize            = wire.MaxBlockPayload
	MaxEvidenceSize       = 16 << 20
	MaxScriptSize         = 10000
	CommitAtoms     int64 = 500000
	ReturnAtoms     int64 = 490000
	LegFeeAtoms     int64 = 10000
	MinChangeAtoms  int64 = 1000
	maxMoney        int64 = 21000000 * 100000000
)

var (
	ErrInvalid     = errors.New("fighter transfer supplied evidence invalid")
	ErrUnavailable = errors.New("fighter transfer required evidence unavailable")
	ErrCapacity    = errors.New("fighter transfer supplied evidence capacity")
	ErrNotCarrier  = errors.New("transaction has no fighter transfer carrier discriminator")
)

type Kind string

const (
	Reveal Kind = "reveal"
	Refund Kind = "refund"
)

type Intent struct {
	PinID         [32]byte
	PriorStateID  [32]byte
	RecipientGym  [33]byte
	FundingDigest [32]byte
}
type Envelope struct {
	Intent    Intent
	Signature [64]byte // Strict big-endian r||s; s is low.
}

// Expected selects the cryptographic context to check. Supplying this struct is
// not proof that its predecessor or source is authoritative.
type Expected struct {
	PinID        [32]byte
	PriorStateID [32]byte
	SenderGym    [33]byte
}

// Evidence supplies complete transaction bytes. FundingTransactions contains
// distinct full parents, not asserted prevout amounts/scripts. Callers retain
// immutable input buffers throughout the call. All buffers count toward the
// aggregate cap, including repeated buffers. No raw buffer is retained.
type Evidence struct {
	CommitRaw           []byte
	SpendRaw            []byte
	FundingTransactions [][]byte
}

// Report describes verified relationships among the supplied bytes only. Raw
// actual TxIDs remain distinct from TransferID. It is not an ownership capability.
type Report struct {
	Kind         Kind
	Envelope     Envelope
	TransferID   [32]byte
	CommitTxID   chainhash.Hash
	SpendTxID    chainhash.Hash
	FundingKey   [33]byte
	ReturnScript [22]byte
	Funding      []wire.OutPoint
}
