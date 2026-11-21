// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"encoding/json"
	"errors"

	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectwork"
)

func ValidateDailyPoolConfig(c PoolConfig) (PoolConfig, error) {
	fail := errors.New("invalid daily pool worker configuration")
	if c.Version != PoolVersion2 || c.MaxRecords == 0 || c.MaxRecords > 4096 || c.MaxBytes < 64<<20 || c.MaxBytes > 1<<30 || !manifestHashShape(c.TransportTrustID) {
		return PoolConfig{}, fail
	}
	if _, e := poolshare.DailyConfigID(c.Source); e != nil {
		return PoolConfig{}, fail
	}
	m := poolManifestConfig(c)
	if valid, e := ValidateManifestConfig(m); e != nil || valid != m {
		return PoolConfig{}, fail
	}
	// Validate the other public origin through the same lower-level grammar.
	// No dummy legacy pool client or invented settlement identity is involved.
	m.AuthorityOrigin = c.AuthorityOrigin
	if valid, e := ValidateManifestConfig(m); e != nil || valid != m {
		return PoolConfig{}, fail
	}
	return c, nil
}

func validateSelectedPoolConfig(c PoolConfig, daily bool) (PoolConfig, error) {
	if daily {
		return ValidateDailyPoolConfig(c)
	}
	return ValidatePoolConfig(c)
}
func NewDailyPool(c PoolConfig, a poolservice.Authority, admission ManifestAuthority, node Node, signer Signer, outbox *PoolOutbox) (*PoolWorker, error) {
	return newPool(c, a, admission, node, signer, outbox, nil, nil, true)
}
func CreateDailyPoolOutbox(path string, c PoolConfig) (*PoolOutbox, error) {
	return createPoolOutbox(path, c, true)
}
func OpenDailyPoolOutbox(path string, c PoolConfig) (*PoolOutbox, error) {
	return openSelectedPoolOutbox(path, c, false, true)
}
func OpenDailyPoolOutboxAudit(path string, c PoolConfig) (*PoolOutbox, error) {
	return openSelectedPoolOutbox(path, c, true, true)
}

func poolOutboxConstructorFailure(b *PoolOutbox, cause error, daily bool) (*PoolOutbox, error) {
	if !daily {
		// Preserve the legacy constructor's cleanup/return behavior.
		_ = b.db.Close()
		return nil, cause
	}
	if e := b.Close(); e != nil {
		// A failed original closer remains reachable and sticky. The parent
		// attachment must retain this owner rather than release its root.
		return b, errors.Join(cause, e)
	}
	return nil, cause
}
func poolReopenConfig(saved, selected PoolConfig) bool {
	if selected.Version == PoolVersion2 {
		return saved == selected
	}
	return samePoolIdentity(saved, selected) && selected.MaxRecords >= saved.MaxRecords && selected.MaxBytes >= saved.MaxBytes
}
func preparePoolWork(c PoolConfig, w poolshare.MiningWork) (*prospectwork.ManifestWork, error) {
	if c.Version == PoolVersion2 {
		return poolservice.PrepareDailyWork(c.Source, c.PublicKey, w)
	}
	return poolservice.PrepareWork(c.Source, c.PublicKey, w)
}
func poolSubmissionMatches(c PoolConfig, w poolshare.MiningWork, nonce uint32, check prospectwork.ShareCheck, s poolshare.Submission) bool {
	if c.Version == PoolVersion2 {
		return poolservice.SubmissionMatchesDaily(c.Source, w, nonce, check, s)
	}
	return poolservice.SubmissionMatches(c.Source, w, nonce, check, s)
}

// V1 never encodes issue_pending. V2 requires its exact final field even false.
type poolDailyMetadata struct {
	poolOutboxMetadata
	IssuePending bool `json:"issue_pending"`
}

func encodePoolMetadata(m poolOutboxMetadata) ([]byte, error) {
	if m.Version == PoolVersion2 {
		return json.Marshal(poolDailyMetadata{m, m.IssuePending})
	}
	if m.Version != PoolVersion1 || m.IssuePending {
		return nil, ErrCorrupt
	}
	return json.Marshal(m)
}
func decodePoolMetadata(raw []byte, version uint32, out *poolOutboxMetadata) error {
	if version == PoolVersion2 {
		var m poolDailyMetadata
		if e := poolDecodeCanonical(raw, maxMetadataSize, &m); e != nil {
			return e
		}
		*out = m.poolOutboxMetadata
		out.IssuePending = m.IssuePending
		return nil
	}
	return poolDecodeCanonical(raw, maxMetadataSize, out)
}

type poolPreparedScan struct {
	scan        PoolScan
	raw         []byte
	reservation uint64
}

func preparePoolScan(s *PoolScan, c PoolConfig) (*poolPreparedScan, error) {
	if s == nil {
		return nil, nil
	}
	if validatePoolScan(*s, c) != nil {
		return nil, ErrCorrupt
	}
	owned, e := clonePoolScan(*s)
	if e != nil {
		return nil, e
	}
	payload := owned
	payload.NextNonce = 0
	raw, e := json.Marshal(payload)
	if e != nil || len(raw) > poolMaxScanSize {
		return nil, ErrCapacity
	}
	reservation := poolReservationEntryBytes(owned, c)
	if reservation == poolInvalidReserve {
		return nil, ErrCapacity
	}
	return &poolPreparedScan{owned, raw, reservation}, nil
}
