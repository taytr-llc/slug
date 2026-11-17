// Copyright (c) 2026 The SLUG developers. ISC license.

// Package seednode provides purpose-limited publication on one concrete owned
// candidate fixture chain. It is not a public RPC or production signing API.
package seednode

import (
	"errors"
	"os/exec"

	"github.com/taytr-llc/slug/gymevent"
)

const (
	MaxStepBlocks               = 8
	MaxTransactionSize          = 1024
	MaxBlockSize                = 4000000
	FeeSatoshis           int64 = 1000
	MinimumChangeSatoshis int64 = 294
)

var (
	ErrInvalid        = errors.New("invalid_request")
	ErrOffline        = errors.New("seed_node_offline")
	ErrUnavailable    = errors.New("seed_unavailable")
	ErrCapacity       = errors.New("seed_capacity")
	ErrNotMature      = errors.New("seed_not_mature")
	ErrOutcomeUnknown = errors.New("outcome_unknown")
	ErrProfile        = errors.New("seed publisher requires normal PoW")
)

type Tip struct {
	Height uint32 `json:"height"`
	Hash   string `json:"hash"`
}

type Outpoint struct {
	TxID  string `json:"txid"`
	Index uint32 `json:"index"`
}

type FundingCoin struct {
	Outpoint      Outpoint `json:"outpoint"`
	ValueSatoshis int64    `json:"value_satoshis"`
	Script        string   `json:"script"`
	BlockHeight   uint32   `json:"block_height"`
	BlockHash     string   `json:"block_hash"`
	Coinbase      bool     `json:"coinbase"`
}

// Anchor is a verified local observation, not a self-contained output proof.
// Reverification obtains the full public block identified by Hash.
type Anchor struct {
	Height       uint32   `json:"height"`
	Hash         string   `json:"hash"`
	Header       string   `json:"header"`
	TxID         string   `json:"txid"`
	RawTxDigest  string   `json:"raw_tx_digest"`
	RawTxSize    uint32   `json:"raw_tx_size"`
	TxCount      uint32   `json:"tx_count"`
	TxIndex      uint32   `json:"tx_index"`
	OutputIndex  uint32   `json:"output_index"`
	Payload      string   `json:"payload"`
	MerkleBranch []string `json:"merkle_branch"`
}

type HeaderRef struct {
	Height uint32
	Hash   string
	Header string
}

type UnspentOutput struct {
	ValueSatoshis int64
	Script        string
	Confirmations uint32
	Coinbase      bool
	BestBlock     string
}

// OwnedProcess is supplied only by the existing process owner. Done closes
// after Wait, avoiding an unsynchronized read of exec.Cmd.ProcessState.
type OwnedProcess struct {
	Cmd  *exec.Cmd
	Done <-chan struct{}
}

type OwnerBinding struct {
	Source  gymevent.Pin
	Home    string
	Primary OwnedProcess
	Peer    OwnedProcess
}

type ChainIdentity struct {
	SourceStoreID              string
	PublisherAccountID         string
	PublisherSourceAuthorityID string
	PublisherChainID           string
	Network                    uint32
	Genesis                    string
}
