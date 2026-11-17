// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymprovider

import (
	"context"
	"errors"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
)

func (s *Coordinator) IssueLogin(ctx context.Context) (LoginFlow, error) {
	if e := s.lock(ctx); e != nil {
		return LoginFlow{}, e
	}
	defer s.unlock()
	if _, e := s.observe(ctx); e != nil {
		return LoginFlow{}, e
	}
	if s.meta.LoginFlows >= s.meta.Marker.Config.MaxLoginFlows {
		return LoginFlow{}, ErrCapacity
	}
	id, e := s.unique("login:")
	if e != nil {
		return LoginFlow{}, e
	}
	r := loginRecord{Flow: LoginFlow{Version: 1, CoordinatorID: s.meta.Marker.CoordinatorID, AuthorityID: s.meta.Marker.Config.AuthorityID, ID: id, Issuer: s.meta.Marker.Config.Issuer}, Phase: "reserved"}
	next := s.meta
	next.LoginFlows++
	b := new(leveldb.Batch)
	if put(b, "login:"+id, r) != nil {
		return LoginFlow{}, ErrCorrupt
	}
	if e = s.commit(ctx, b, next); e != nil {
		return LoginFlow{}, e
	}
	s.point("reserve-login")
	attempt, e := s.authority.IssueLogin(ctx, r.Flow.Issuer)
	if e != nil {
		return LoginFlow{}, safeError(e)
	}
	s.point("authority-issue-login")
	r.Flow.AttemptID = attempt.ID
	r.Flow.Nonce = attempt.Nonce
	r.Flow.Sequence = attempt.Sequence
	r.Flow.IssuedAtMS = attempt.IssuedAtMS
	r.Flow.ExpiresAtMS = attempt.ExpiresAtMS
	r.Phase = "issued"
	if attempt.AuthorityID != r.Flow.AuthorityID || attempt.Issuer != r.Flow.Issuer || attempt.Version != 2 || s.validLogin(r) != nil {
		return LoginFlow{}, ErrCorrupt
	}
	b = new(leveldb.Batch)
	if put(b, "login:"+id, r) != nil {
		return LoginFlow{}, ErrCorrupt
	}
	next = s.meta
	if attempt.IssuedAtMS > next.ClockMS {
		next.ClockMS = attempt.IssuedAtMS
	}
	if e = s.commit(ctx, b, next); e != nil {
		return LoginFlow{}, e
	}
	return r.Flow, nil
}
func semanticLogin(v gymidentity.CoordinatedLogin) loginSemantic {
	return loginSemantic{v.Login.Issuer, v.Login.Subject, v.OriginalAuthID, v.OriginalFlowID, v.Mode, v.Login.CredentialEpoch, v.Login.AuthenticatedAtMS}
}
func sameOrigin(o originRecord, p loginSemantic) bool {
	return o.Semantic.Issuer == p.Issuer && o.Semantic.Subject == p.Subject && o.Semantic.OriginalAuthID == p.OriginalAuthID && o.Semantic.OriginalFlowID == p.OriginalFlowID && o.Semantic.Epoch == p.Epoch && o.Semantic.AuthenticatedAtMS == p.AuthenticatedAtMS
}
func (s *Coordinator) abandonToken(token string) {
	if !validID(token) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.authority.Logout(ctx, token)
}

// FinishLogin never replays a session token. An unknown/consumed login requires
// another explicit authentication flow; no bearer secret lives in this journal.
func (s *Coordinator) FinishLogin(ctx context.Context, id, token string) (gymaccount.Session, error) {
	if !validID(id) {
		return gymaccount.Session{}, ErrEvidence
	}
	if e := s.lock(ctx); e != nil {
		return gymaccount.Session{}, e
	}
	defer s.unlock()
	var r loginRecord
	if e := s.load("login:"+id, &r); e != nil {
		return gymaccount.Session{}, e
	}
	if s.validLogin(r) != nil || r.Flow.ID != id {
		return gymaccount.Session{}, ErrCorrupt
	}
	now, e := s.observe(ctx)
	if e != nil {
		return gymaccount.Session{}, e
	}
	switch r.Phase {
	case "reserved", "unavailable":
		return gymaccount.Session{}, ErrUnavailable
	case "completed":
		return gymaccount.Session{}, ErrConsumed
	case "consumed_or_unknown":
		return gymaccount.Session{}, ErrOutcomeUnknown
	}
	v, e := s.verifier.VerifyCoordinatedLogin(ctx, token, s.binding(), id, r.Flow.Nonce)
	// The verifier reads its clock too. Persist an observation after it even
	// on rejection, so crossing an expiry second cannot leave highwater behind.
	observed, clockErr := s.observe(ctx)
	if clockErr != nil {
		return gymaccount.Session{}, clockErr
	}
	now = observed

	if e != nil {
		return gymaccount.Session{}, evidenceError(e)
	}
	if now >= r.Flow.ExpiresAtMS {
		return gymaccount.Session{}, ErrExpired
	}
	p := semanticLogin(v)
	key := s.index("original authentication", p.Issuer, p.Subject, p.OriginalAuthID)
	if r.Semantic != nil && (r.OriginKey != key || *r.Semantic != p) {
		return gymaccount.Session{}, ErrConflict
	}
	var origin originRecord
	e = s.load("origin:"+key, &origin)
	if p.Mode == "fresh" {
		if p.OriginalFlowID != id || p.AuthenticatedAtMS/1000 < r.Flow.IssuedAtMS/1000 {
			return gymaccount.Session{}, ErrEvidence
		}
		if errors.Is(e, ErrNotFound) {
			origin = originRecord{Semantic: p, AttemptID: r.Flow.AttemptID, Sequence: r.Flow.Sequence, State: "prepared"}
		} else if e != nil {
			return gymaccount.Session{}, e
		} else if !sameOrigin(origin, p) || origin.AttemptID != r.Flow.AttemptID || origin.Sequence != r.Flow.Sequence {
			return gymaccount.Session{}, ErrConflict
		}
	} else {
		if e != nil || origin.State != "accepted" || !sameOrigin(origin, p) {
			return gymaccount.Session{}, ErrEvidence
		}
	}
	attempt, e := s.authority.GetLoginAttempt(ctx, r.Flow.AttemptID)
	if e != nil {
		if r.Phase == "submitting" && (errors.Is(e, gymaccount.ErrUnauthorized) || errors.Is(e, gymaccount.ErrExpired)) {
			r.Phase = "consumed_or_unknown"
			b := new(leveldb.Batch)
			if put(b, "login:"+id, r) != nil {
				return gymaccount.Session{}, ErrCorrupt
			}
			if ce := s.commit(ctx, b, s.meta); ce != nil {
				return gymaccount.Session{}, ce
			}
			return gymaccount.Session{}, ErrOutcomeUnknown
		}
		return gymaccount.Session{}, safeError(e)
	}
	if attempt.ID != r.Flow.AttemptID || attempt.AuthorityID != r.Flow.AuthorityID || attempt.Nonce != r.Flow.Nonce || attempt.Sequence != r.Flow.Sequence || attempt.Issuer != r.Flow.Issuer || attempt.IssuedAtMS != r.Flow.IssuedAtMS || attempt.ExpiresAtMS != r.Flow.ExpiresAtMS {
		return gymaccount.Session{}, ErrCorrupt
	}
	if r.Phase == "issued" {
		r.Phase = "submitting"
		r.Semantic = &p
		r.OriginKey = key
		if s.validLogin(r) != nil {
			return gymaccount.Session{}, ErrEvidence
		}
		next := s.meta
		if p.AuthenticatedAtMS > next.ClockMS {
			next.ClockMS = p.AuthenticatedAtMS
		}
		b := new(leveldb.Batch)
		if put(b, "login:"+id, r) != nil {
			return gymaccount.Session{}, ErrCorrupt
		}
		if p.Mode == "fresh" && put(b, "origin:"+key, origin) != nil {
			return gymaccount.Session{}, ErrCorrupt
		}
		if e = s.commit(ctx, b, next); e != nil {
			return gymaccount.Session{}, e
		}
	}
	s.point("submitting-login")
	result, e := s.authority.LoginWithAssertion(ctx, r.Flow.AttemptID, v.Login)
	if e != nil {
		return gymaccount.Session{}, safeError(e)
	}
	s.point("authority-login")
	principal, e := gymaccount.PrincipalID(p.Issuer, p.Subject)
	if e != nil || !validID(result.Token) || result.Kind != "login" || result.Account.Version != 1 || !validID(result.Account.ID) || result.Account.PrincipalID != principal || result.Account.Generation == 0 {
		s.abandonToken(result.Token)
		return gymaccount.Session{}, ErrCorrupt
	}
	r.Phase = "completed"
	r.AccountID = result.Account.ID
	r.Generation = result.Account.Generation
	b := new(leveldb.Batch)
	if put(b, "login:"+id, r) != nil {
		s.abandonToken(result.Token)
		return gymaccount.Session{}, ErrCorrupt
	}
	if p.Mode == "fresh" {
		origin.State = "accepted"
		if put(b, "origin:"+key, origin) != nil {
			s.abandonToken(result.Token)
			return gymaccount.Session{}, ErrCorrupt
		}
	}
	if e = s.commit(ctx, b, s.meta); e != nil {
		s.abandonToken(result.Token)
		return gymaccount.Session{}, e
	}
	return result, nil
}
func evidenceError(e error) error {
	if errors.Is(e, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return ErrEvidence
}

func (s *Coordinator) Status(ctx context.Context, kind, id string) (FlowStatus, error) {
	if !validID(id) || (kind != "login" && kind != "reset") {
		return FlowStatus{}, ErrEvidence
	}
	if e := s.lock(ctx); e != nil {
		return FlowStatus{}, e
	}
	defer s.unlock()
	if _, e := s.observe(ctx); e != nil {
		return FlowStatus{}, e
	}
	result := FlowStatus{Version: 1, CoordinatorID: s.meta.Marker.CoordinatorID, AuthorityID: s.meta.Marker.Config.AuthorityID, Kind: kind, ID: id}
	if kind == "login" {
		var r loginRecord
		if e := s.load("login:"+id, &r); e != nil {
			return FlowStatus{}, e
		}
		if s.validLogin(r) != nil || r.Flow.ID != id {
			return FlowStatus{}, ErrCorrupt
		}
		result.Phase = r.Phase
		if result.Phase == "reserved" {
			result.Phase = "unavailable"
		}
		result.OutcomeUnknown = r.Phase == "reserved" || r.Phase == "submitting" || r.Phase == "consumed_or_unknown"
	} else {
		var r resetRecord
		if e := s.load("reset:"+id, &r); e != nil {
			return FlowStatus{}, e
		}
		if s.validReset(r) != nil || r.Request.EventID != id {
			return FlowStatus{}, ErrCorrupt
		}
		result.Phase = r.Phase
		result.OutcomeUnknown = r.Phase == "begin_prepared" || r.Phase == "complete_prepared"
	}
	return result, nil
}
