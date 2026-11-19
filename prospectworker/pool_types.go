// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"github.com/taytr-llc/slug/poolshare"
)

const PoolVersion1 uint32 = 1

// PoolConfig is public, credential-free identity for a separate pool outbox.
// Admission uses the same source authority pin, but its own transport pin.
type PoolConfig struct {
	Version          uint32           `json:"version"`
	Source           poolshare.Config `json:"source"`
	PublicKey        string           `json:"public_key"`
	AuthorityOrigin  string           `json:"authority_origin"`
	TransportTrustID string           `json:"transport_trust_id"`
	AdmissionOrigin  string           `json:"admission_origin"`
	AdmissionTrustID string           `json:"admission_trust_id"`
	MaxRecords       uint64           `json:"max_records"`
	MaxBytes         uint64           `json:"max_bytes"`
}

// PoolScan retains exact issued work. Only NextNonce changes; exhausting uint32
// obtains another job instead of changing the pool's coinbase or extra nonce.
type PoolScan struct {
	Version   uint32               `json:"version"`
	Work      poolshare.MiningWork `json:"work"`
	NextNonce uint64               `json:"next_nonce"`
}

// PoolEntry synchronizes all independently qualifying outcomes before transport.
// Fighter is nil for pool-only work; a pool receipt never substitutes for it.
type PoolEntry struct {
	Version       uint32                `json:"version"`
	Sequence      uint64                `json:"sequence"`
	Work          poolshare.MiningWork  `json:"work"`
	Nonce         uint32                `json:"nonce"`
	WorkHash      string                `json:"work_hash"`
	DuplicateKey  string                `json:"duplicate_key"`
	PoolWin       bool                  `json:"pool_win"`
	PoolState     string                `json:"pool_state"`
	PoolReceipt   *poolshare.Submission `json:"pool_receipt,omitempty"`
	Fighter       *ManifestEntry        `json:"fighter,omitempty"`
	LastError     string                `json:"last_error,omitempty"`
	NextAttemptMS int64                 `json:"next_attempt_ms"`
	RetryCount    uint32                `json:"retry_count"`
}
