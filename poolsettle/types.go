// Copyright (c) 2026 The SLUG developers. ISC license.
// Package poolsettle implements operator-only, daily pool settlement. It never
// imports caller receipts or discovers funds from a wallet's total balance.
package poolsettle

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolshare"
)

const Version1 uint32 = 1
const MinimumAtoms int64 = 100000000
const MaxMoney int64 = 21000000 * 100000000
const MaxInputs = 256
const MaxTransactionBytes = 100000

var (
	ErrInvalid  = errors.New("invalid pool settlement request")
	ErrConfig   = errors.New("invalid pool settlement configuration")
	ErrProfile  = errors.New("pool settlement requires production PoW")
	ErrClosed   = errors.New("pool settlement journal is closed")
	ErrBusy     = errors.New("pool settlement operation is busy")
	ErrPoisoned = errors.New("pool settlement write outcome uncertain; reopen before retry")
	ErrCorrupt  = errors.New("pool settlement journal is corrupt")
	ErrSource   = errors.New("pool settlement source changed or unavailable")
	ErrCapacity = errors.New("pool settlement capacity reached")
	ErrClock    = errors.New("pool settlement clock moved backward")
	ErrBackend  = errors.New("pool settlement funding or transaction observation unavailable")
)

// Destinations are trusted operator configuration, pinned for this journal.
// No request-supplied address, wallet coin selector or recovery key is accepted.
type Destination struct {
	GymID  string `json:"gym_id"`
	Script string `json:"script"`
}
type Config struct {
	Version      uint32        `json:"version"`
	ChangeScript string        `json:"change_script"`
	Destinations []Destination `json:"destinations"`
	MaxRounds    uint32        `json:"max_rounds"`
	MaxBatches   uint32        `json:"max_batches"`
}
type Fraction struct {
	Numerator   string `json:"numerator"`
	Denominator string `json:"denominator"`
}
type Claim struct {
	RoundHash   string   `json:"round_hash"`
	RoundHeight uint32   `json:"round_height"`
	GymID       string   `json:"gym_id"`
	Amount      Fraction `json:"amount"`
}

// Empty GymID denotes the pool's remaining 1% fee, never miner principal.
type Coin struct {
	TxID        string  `json:"txid"`
	Index       uint32  `json:"index"`
	Atoms       int64   `json:"atoms"`
	Script      string  `json:"script"`
	Coinbase    bool    `json:"coinbase"`
	BlockHash   string  `json:"block_hash"`
	BlockHeight uint32  `json:"block_height"`
	Claims      []Claim `json:"claims"`
}
type Credit struct {
	RoundHash   string `json:"round_hash"`
	RoundDigest string `json:"round_digest"`
	Coin        Coin   `json:"coin"`
}
type Payment struct {
	GymID  string `json:"gym_id"`
	Script string `json:"script"`
	Atoms  int64  `json:"atoms"`
}

// Funding states are ready, immature, awaiting-confirmation, funding-conflict.
// An RPC failure is an error, never fabricated chain evidence.
type Funding struct {
	TxID  string `json:"txid"`
	Index uint32 `json:"index"`
	State string `json:"state"`
}
type TransactionObservation struct {
	Raw           []byte
	Confirmations uint32
}

// Backend is trusted operator configuration. Implementations must pin chain
// identity and inspect exact UTXOs, canonical membership and next-block maturity.
// Sign receives a fully specified transaction and must neither broadcast nor
// change it. The store independently checks its inputs, outputs and signatures.
// ObserveTransaction returns empty Raw only for a definite unknown transaction;
// a nonempty Raw with zero confirmations is in the mempool.
type Backend interface {
	Inspect(context.Context, []Coin) ([]Funding, error)
	EstimateFee(context.Context, []Coin, []Payment, string) (int64, error)
	Sign(context.Context, []Coin, []byte) ([]byte, error)
	ObserveTransaction(context.Context, string) (TransactionObservation, error)
	Broadcast(context.Context, []byte) (string, error)
}
type Batch struct {
	Day         string    `json:"day"`
	CreatedAtMS int64     `json:"created_at_ms"`
	Inputs      []Coin    `json:"inputs"`
	Payments    []Payment `json:"payments"`
	FeeAtoms    int64     `json:"fee_atoms"`
	Change      *Coin     `json:"change"`
	TxID        string    `json:"txid"`
	Raw         string    `json:"raw"`
	State       string    `json:"state"` // prepared, broadcast, confirmed; historical observation
}
type Balance struct {
	GymID         string   `json:"gym_id"`
	Confirmed     Fraction `json:"confirmed"` // unpaid, including reserved
	Available     Fraction `json:"available"` // latest funding observation
	ReservedAtoms int64    `json:"reserved_atoms"`
	PaidAtoms     int64    `json:"paid_atoms"`
	Reason        string   `json:"reason"`
}
type Report struct {
	Day          string    `json:"day"`
	ObservedAtMS int64     `json:"observed_at_ms"`
	TxID         string    `json:"txid,omitempty"`
	State        string    `json:"state"`
	Reason       string    `json:"reason,omitempty"`
	Balances     []Balance `json:"balances"`
}
type state struct {
	Config      Config           `json:"config"`
	Source      poolshare.Config `json:"source"`
	Credits     []Credit         `json:"credits"`
	Batches     []Batch          `json:"batches"`
	LastClockMS int64            `json:"last_clock_ms"`
	Report      Report           `json:"report"`
}
type Store struct {
	db         *leveldb.DB
	state      state
	source     *poolround.Store
	backend    Backend
	now        func() time.Time
	gate       chan struct{}
	lifecycle  sync.Mutex
	closing    bool
	closeDone  chan struct{}
	closeErr   error
	poisoned   bool
	writeBatch func(*leveldb.Batch, *opt.WriteOptions) error
}
