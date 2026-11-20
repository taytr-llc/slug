// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/taytr-llc/slug/wire/v2"
)

const MaxRecoveryCommits = 4096

// A capture retains only fixed 40-byte indices, never raw transactions or a
// signing capability. Store retains one slot; replacement revokes the old one.
type recoveryCommitIndex struct {
	txid             [32]byte
	height, position uint32
}
type RecoveryCommitCapture struct {
	store      *Store
	source     *PublicSource
	node       *OwnedChainSource
	head       HeadRef
	generation uint64
	gym        string
	entries    []recoveryCommitIndex
	closed     bool // guarded by store.mu
}
type RecoveryCommitCandidate struct {
	CommitTxID       string `json:"commit_txid"`
	BlockHash        string `json:"block_hash"`
	Height           uint32 `json:"height"`
	TransactionIndex uint32 `json:"transaction_index"`
	ProspectID       string `json:"prospect_id"`
	PriorStateID     string `json:"prior_state_id"`
	SenderGym        string `json:"sender_gym"`
	RecipientGym     string `json:"recipient_gym"`
}
type RecoveryCommitPage struct {
	Head       HeadRef                   `json:"head"`
	After      uint32                    `json:"after"`
	Next       uint32                    `json:"next"`
	Complete   bool                      `json:"complete"`
	Candidates []RecoveryCommitCandidate `json:"candidates"`
}

func (c *RecoveryCommitCapture) Head() HeadRef {
	if c == nil {
		return HeadRef{}
	}
	return c.head
}
func (c *RecoveryCommitCapture) Close() error {
	if c == nil || c.store == nil {
		return nil
	}
	c.store.mu.Lock()
	defer c.store.mu.Unlock()
	c.closed = true
	c.entries = nil
	if c.store.recoveryCapture == c {
		c.store.recoveryCapture = nil
	}
	return nil
}
func (c *RecoveryCommitCapture) checkLocked(s *Store) error {
	if c == nil || c.store != s || c.closed || s.recoveryCapture != c {
		return ErrStale
	}
	h := HeadLease{store: s, source: c.source, node: c.node, head: c.head, generation: c.generation}
	return h.checkLocked()
}

// recoveryLocator is only a bounded public framing read. It proves no funding
// ownership, gym authorization, escrow availability or signing eligibility.
func recoveryLocator(ctx context.Context, raw []byte) (*wire.MsgTx, string, string, error) {
	c, e := boundedTx(ctx, raw, 4096)
	if e != nil {
		return nil, "", "", e
	}
	if len(c.TxOut) != 3 || len(c.TxIn) < 1 || len(c.TxIn) > 16 {
		return nil, "", "", ErrInvalid
	}
	script := c.TxOut[1].PkScript
	if len(script) != 76 || script[0] != 0x6a || script[1] != 0x4a || string(script[2:11]) != "SLUGFCM1\x01" {
		return nil, "", "", ErrInvalid
	}
	prior, recipient := hex.EncodeToString(script[11:43]), hex.EncodeToString(script[43:])
	if !hexValue(prior, 32, true) || !gymKey(recipient) {
		return nil, "", "", ErrInvalid
	}
	return c, prior, recipient, nil
}

// Caller holds store.mu. Invalid/unknown locators in a complete reachable set
// are not candidates; a known predecessor missing portable evidence is a gap.
func (s *Store) recoveryCandidateLocked(ctx context.Context, commit TransactionWitness, gym string) (RecoveryCommitCandidate, bool, error) {
	var none RecoveryCommitCandidate
	if commit.Height > s.head.ChainHeight {
		return none, false, ErrCorrupt
	}
	if s.canonical[commit.Height] != commit.BlockHash {
		return none, false, ErrStale
	}
	if len(commit.Raw) > 8192 {
		return none, false, ErrCorrupt
	} // replay already enforced this cap
	raw, e := hex.DecodeString(commit.Raw)
	if e != nil {
		return none, false, ErrCorrupt
	}
	tx, priorID, recipient, e := recoveryLocator(ctx, raw)
	if errors.Is(e, ErrInvalid) {
		return none, false, nil
	}
	if e != nil {
		return none, false, e
	}
	if tx.TxHash().String() != commit.TxID {
		return none, false, ErrCorrupt
	}
	prior, ok := s.states[priorID]
	if !ok {
		return none, false, nil
	}
	if !s.portable[priorID] {
		return none, false, ErrUnavailable
	}
	if prior.Generation > 0 {
		var ev FinalStateEvidence
		if json.Unmarshal(prior.Evidence, &ev) != nil {
			return none, false, ErrCorrupt
		}
		if ev.SixtiethDescendantHeight >= commit.Height {
			return none, false, nil
		}
	}
	if prior.OwnerPublicKey != gym {
		return none, false, nil
	}
	return RecoveryCommitCandidate{commit.TxID, commit.BlockHash, commit.Height, commit.TransactionIndex, prior.ProspectID, priorID, prior.OwnerPublicKey, recipient}, true, nil
}

// CaptureRecoveryCommits samples one original ready head. RPC source checks
// happen outside store.mu/account leases; the exact local head is then checked
// again in the lock that captures every candidate index. No journal is touched.
func (s *Store) CaptureRecoveryCommits(ctx context.Context, h *HeadLease, gym string) (*RecoveryCommitCapture, error) {
	if ctx == nil || s == nil || h == nil || h.store != s || !gymKey(gym) {
		return nil, ErrInvalid
	}
	done, e := s.project(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if e = s.readLock(work); e != nil {
		return nil, e
	}
	stop := context.AfterFunc(s.life, cancel)
	defer stop()
	if e = h.checkLocked(); e != nil {
		s.mu.Unlock()
		return nil, e
	}
	source, node, observation := s.source, s.node, s.observation
	if h.head.ChainHeight != observation.Height || h.head.ChainHash != observation.Hash {
		s.mu.Unlock()
		return nil, ErrCorrupt
	}
	s.mu.Unlock()
	if e = source.Check(work); e != nil {
		return nil, e
	}
	if e = node.Check(work, observation); e != nil {
		return nil, e
	}
	if e = s.readLock(work); e != nil {
		return nil, e
	}
	defer s.mu.Unlock()
	if e = h.checkLocked(); e != nil {
		return nil, e
	}
	if s.source != source || s.node != node || s.observation != observation {
		return nil, ErrStale
	}
	if len(s.commits) > MaxRecoveryCommits {
		return nil, ErrCapacity
	}
	c := &RecoveryCommitCapture{store: s, source: source, node: node, head: h.head, generation: s.generation, gym: gym, entries: make([]recoveryCommitIndex, 0, len(s.commits))}
	n := 0
	for id, commit := range s.commits {
		if n%128 == 0 {
			if e = work.Err(); e != nil {
				return nil, e
			}
		}
		n++
		if id != commit.TxID {
			return nil, ErrCorrupt
		}
		_, keep, e := s.recoveryCandidateLocked(work, commit, gym)
		if e != nil {
			return nil, e
		}
		if !keep {
			continue
		}
		decoded, e := hex.DecodeString(id)
		if e != nil || len(decoded) != 32 {
			return nil, ErrCorrupt
		}
		row := recoveryCommitIndex{height: commit.Height, position: commit.TransactionIndex}
		copy(row.txid[:], decoded)
		c.entries = append(c.entries, row)
	}
	sort.Slice(c.entries, func(i, j int) bool {
		a, b := c.entries[i], c.entries[j]
		if a.height != b.height {
			return a.height < b.height
		}
		if a.position != b.position {
			return a.position < b.position
		}
		return bytes.Compare(a.txid[:], b.txid[:]) < 0
	})
	if e = work.Err(); e != nil {
		return nil, e
	}
	if s.recoveryCapture != nil {
		s.recoveryCapture.closed = true
		s.recoveryCapture.entries = nil
	}
	s.recoveryCapture = c
	return c, nil
}

// RecoveryCommitPage is a local projection of the exact opaque capture. It
// never probes escrow, derives keys, publishes an intent, signs or broadcasts.
func (s *Store) RecoveryCommitPage(ctx context.Context, c *RecoveryCommitCapture, after, limit uint32) (RecoveryCommitPage, error) {
	var none RecoveryCommitPage
	if limit < 1 || limit > 8 {
		return none, ErrInvalid
	}
	done, e := s.project(ctx)
	if e != nil {
		return none, e
	}
	defer done()
	if e = s.readLock(ctx); e != nil {
		return none, e
	}
	defer s.mu.Unlock()
	if e = c.checkLocked(s); e != nil {
		return none, e
	}
	if after > uint32(len(c.entries)) {
		return none, ErrInvalid
	}
	end := after + limit
	if end > uint32(len(c.entries)) {
		end = uint32(len(c.entries))
	}
	out := RecoveryCommitPage{Head: c.head, After: after, Next: end, Complete: end == uint32(len(c.entries)), Candidates: make([]RecoveryCommitCandidate, 0, end-after)}
	for _, index := range c.entries[after:end] {
		if e = ctx.Err(); e != nil {
			return none, e
		}
		id := hex.EncodeToString(index.txid[:])
		commit, ok := s.commits[id]
		if !ok || commit.Height != index.height || commit.TransactionIndex != index.position {
			return none, ErrStale
		}
		value, keep, e := s.recoveryCandidateLocked(ctx, commit, c.gym)
		if e != nil {
			return none, e
		}
		if !keep {
			return none, ErrStale
		}
		out.Candidates = append(out.Candidates, value)
	}
	return out, ctx.Err()
}
