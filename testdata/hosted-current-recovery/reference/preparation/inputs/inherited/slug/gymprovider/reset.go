// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymprovider

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
)

func (s *Coordinator) IssueReset(ctx context.Context) (ResetRequest, error) {
	if e := s.lock(ctx); e != nil {
		return ResetRequest{}, e
	}
	defer s.unlock()
	now, e := s.observe(ctx)
	if e != nil {
		return ResetRequest{}, e
	}
	if s.meta.ResetFlows >= s.meta.Marker.Config.MaxResetFlows {
		return ResetRequest{}, ErrCapacity
	}
	id, e := s.unique("reset:")
	if e != nil {
		return ResetRequest{}, e
	}
	nonce, e := randomID(s.random)
	if e != nil {
		return ResetRequest{}, e
	}
	r := resetRecord{Request: ResetRequest{1, s.meta.Marker.CoordinatorID, s.meta.Marker.Config.AuthorityID, id, nonce, now, now + LifetimeMS}, Phase: "issued"}
	next := s.meta
	next.ResetFlows++
	b := new(leveldb.Batch)
	if put(b, "reset:"+id, r) != nil {
		return ResetRequest{}, ErrCorrupt
	}
	if e = s.commit(ctx, b, next); e != nil {
		return ResetRequest{}, e
	}
	return r.Request, nil
}
func resetSemanticFor(v gymidentity.CoordinatedReset) resetSemantic {
	p := v.Reset
	return resetSemantic{p.Issuer, p.Subject, v.ProviderOperationID, p.FromEpoch, p.ToEpoch, p.VerifiedAtMS}
}
func barrierEvent(principal, event string) string {
	h := sha256.New()
	h.Write([]byte("SLUG account recovery event\x00v1\x00"))
	raw, _ := hex.DecodeString(principal)
	h.Write(raw)
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(event)))
	h.Write(n[:])
	h.Write([]byte(event))
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Coordinator) validBarrier(r resetRecord, b gymaccount.ResetBarrier) error {
	if r.Semantic == nil {
		return ErrCorrupt
	}
	p := r.Semantic
	principal, e := gymaccount.PrincipalID(p.Issuer, p.Subject)
	if e != nil || b.Version != 2 || !validID(b.AccountID) || b.PrincipalID != principal || b.EventID != barrierEvent(principal, r.Request.EventID) || b.Generation < 2 || b.FromEpoch != p.FromEpoch || b.ToEpoch != p.ToEpoch || b.VerifiedAtMS != p.VerifiedAtMS || b.BeganAtMS < b.VerifiedAtMS || b.BeginSequence == 0 {
		return ErrCorrupt
	}
	switch b.Status {
	case "pending":
		if b.CompletedAtMS != 0 || b.CompleteSequence != 0 || b.HoldUntilMS != 0 {
			return ErrCorrupt
		}
	case "completed":
		if b.CompletedAtMS < b.BeganAtMS || b.CompletedAtMS > math.MaxInt64-gymaccount.RecoveryHoldMS || b.HoldUntilMS != b.CompletedAtMS+gymaccount.RecoveryHoldMS || b.CompleteSequence <= b.BeginSequence {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	if r.Barrier != nil {
		old := *r.Barrier
		copy := b
		copy.Status = old.Status
		copy.CompletedAtMS = old.CompletedAtMS
		copy.CompleteSequence = old.CompleteSequence
		copy.HoldUntilMS = old.HoldUntilMS
		if copy != old || old.Status == "completed" && b != old {
			return ErrCorrupt
		}
	}
	return nil
}
func (s *Coordinator) BeginReset(ctx context.Context, id, token string) (ResetResult, error) {
	return s.reset(ctx, id, token, false)
}
func (s *Coordinator) CompleteReset(ctx context.Context, id, token string) (ResetResult, error) {
	return s.reset(ctx, id, token, true)
}
func (s *Coordinator) reset(ctx context.Context, id, token string, complete bool) (ResetResult, error) {
	if !validID(id) {
		return ResetResult{}, ErrEvidence
	}
	if e := s.lock(ctx); e != nil {
		return ResetResult{}, e
	}
	defer s.unlock()
	var r resetRecord
	if e := s.load("reset:"+id, &r); e != nil {
		return ResetResult{}, e
	}
	if s.validReset(r) != nil || r.Request.EventID != id {
		return ResetResult{}, ErrCorrupt
	}
	now, e := s.observe(ctx)
	if e != nil {
		return ResetResult{}, e
	}
	v, e := s.verifier.VerifyCoordinatedReset(ctx, token, s.binding(), id, r.Request.Nonce)
	// The verifier reads its clock too. Persist an observation after it even
	// on rejection, so crossing an expiry second cannot leave highwater behind.
	observed, clockErr := s.observe(ctx)
	if clockErr != nil {
		return ResetResult{}, clockErr
	}
	now = observed

	if e != nil {
		return ResetResult{}, evidenceError(e)
	}
	stage := "begin"
	if complete {
		stage = "complete"
	}
	if v.Reset.Stage != stage {
		return ResetResult{}, ErrEvidence
	}
	p := resetSemanticFor(v)
	key := s.index("reset operation", p.Issuer, p.Subject, p.ProviderOperationID)
	if r.Semantic != nil && (*r.Semantic != p || r.OperationKey != key) {
		return ResetResult{}, ErrConflict
	}
	if complete && (r.Phase == "issued" || r.Phase == "begin_prepared") {
		return ResetResult{}, ErrConflict
	}
	if r.Phase == "issued" {
		if now >= r.Request.ExpiresAtMS || p.VerifiedAtMS >= r.Request.ExpiresAtMS || p.VerifiedAtMS/1000 < r.Request.IssuedAtMS/1000 || now-p.VerifiedAtMS > LifetimeMS {
			return ResetResult{}, ErrExpired
		}
		var old string
		e = s.load("operation:"+key, &old)
		if e == nil && old != id {
			return ResetResult{}, ErrConflict
		}
		if e != nil && !errors.Is(e, ErrNotFound) {
			return ResetResult{}, e
		}
		if e == nil {
			return ResetResult{}, ErrCorrupt
		}
		r.Semantic = &p
		r.OperationKey = key
		r.Phase = "begin_prepared"
		if s.validReset(r) != nil {
			return ResetResult{}, ErrEvidence
		}
		next := s.meta
		if p.VerifiedAtMS > next.ClockMS {
			next.ClockMS = p.VerifiedAtMS
		}
		b := new(leveldb.Batch)
		if put(b, "reset:"+id, r) != nil || put(b, "operation:"+key, id) != nil {
			return ResetResult{}, ErrCorrupt
		}
		if e = s.commit(ctx, b, next); e != nil {
			return ResetResult{}, e
		}
	}
	if complete && r.Phase == "pending" {
		r.Phase = "complete_prepared"
		b := new(leveldb.Batch)
		if put(b, "reset:"+id, r) != nil {
			return ResetResult{}, ErrCorrupt
		}
		if e = s.commit(ctx, b, s.meta); e != nil {
			return ResetResult{}, e
		}
	}
	var barrier gymaccount.ResetBarrier
	if complete {
		s.point("complete-prepared")
		barrier, _, e = s.authority.CompleteReset(ctx, v.Reset)
		if e == nil {
			s.point("authority-complete")
		}
	} else {
		s.point("begin-prepared")
		barrier, _, e = s.authority.BeginReset(ctx, v.Reset)
		if e == nil {
			s.point("authority-begin")
		}
	}
	if e != nil {
		return ResetResult{}, safeError(e)
	}
	if s.validBarrier(r, barrier) != nil {
		return ResetResult{}, ErrCorrupt
	}
	if complete && barrier.Status != "completed" {
		return ResetResult{}, ErrCorrupt
	}
	// A retained completion assertion cannot be turned back into a permit by a
	// repeated begin, even if its authority completion write was not committed.
	canCommit := !complete && barrier.Status == "pending" && r.Phase != "complete_prepared"
	r.Barrier = &barrier
	if barrier.Status == "completed" {
		r.Phase = "completed"
	} else if r.Phase != "complete_prepared" {
		r.Phase = "pending"
	}
	b := new(leveldb.Batch)
	if put(b, "reset:"+id, r) != nil {
		return ResetResult{}, ErrCorrupt
	}
	next := s.meta
	if barrier.BeganAtMS > next.ClockMS {
		next.ClockMS = barrier.BeganAtMS
	}
	if barrier.CompletedAtMS > next.ClockMS {
		next.ClockMS = barrier.CompletedAtMS
	}
	if e = s.commit(ctx, b, next); e != nil {
		return ResetResult{}, e
	}
	return ResetResult{1, s.meta.Marker.CoordinatorID, s.meta.Marker.Config.AuthorityID, id, p.ProviderOperationID, barrier.Status, canCommit, barrier}, nil
}
