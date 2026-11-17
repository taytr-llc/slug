// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymprovider

import (
	"math"
	"strings"

	"github.com/taytr-llc/slug/gymaccount"
)

func (s *Coordinator) validLogin(r loginRecord) error {
	f := r.Flow
	c := s.meta.Marker.Config
	if f.Version != 1 || f.CoordinatorID != s.meta.Marker.CoordinatorID || f.AuthorityID != c.AuthorityID || f.Issuer != c.Issuer || !validID(f.ID) {
		return ErrCorrupt
	}
	if r.Phase == "reserved" || r.Phase == "unavailable" {
		if f.AttemptID != "" || f.Nonce != "" || f.Sequence != 0 || f.IssuedAtMS != 0 || f.ExpiresAtMS != 0 || r.Semantic != nil || r.OriginKey != "" || r.AccountID != "" || r.Generation != 0 {
			return ErrCorrupt
		}
		return nil
	}
	if !validID(f.AttemptID) || !validID(f.Nonce) || f.Sequence == 0 || f.IssuedAtMS <= 0 || f.IssuedAtMS > math.MaxInt64-LifetimeMS || f.ExpiresAtMS != f.IssuedAtMS+LifetimeMS {
		return ErrCorrupt
	}
	if r.Phase == "issued" {
		if r.Semantic != nil || r.OriginKey != "" || r.AccountID != "" || r.Generation != 0 {
			return ErrCorrupt
		}
		return nil
	}
	if r.Semantic == nil || !validID(r.OriginKey) {
		return ErrCorrupt
	}
	p := r.Semantic
	if p.Issuer != c.Issuer || !validID(p.OriginalAuthID) || !validID(p.OriginalFlowID) || p.Epoch == 0 || p.AuthenticatedAtMS <= 0 || p.AuthenticatedAtMS >= f.ExpiresAtMS {
		return ErrCorrupt
	}
	if _, e := gymaccount.PrincipalID(p.Issuer, p.Subject); e != nil {
		return ErrCorrupt
	}
	if p.Mode == "fresh" {
		if p.OriginalFlowID != f.ID || p.AuthenticatedAtMS/1000 < f.IssuedAtMS/1000 {
			return ErrCorrupt
		}
	} else if p.Mode != "reuse" {
		return ErrCorrupt
	}
	if r.OriginKey != s.index("original authentication", p.Issuer, p.Subject, p.OriginalAuthID) {
		return ErrCorrupt
	}
	switch r.Phase {
	case "submitting", "consumed_or_unknown":
		if r.AccountID != "" || r.Generation != 0 {
			return ErrCorrupt
		}
	case "completed":
		if !validID(r.AccountID) || r.Generation == 0 {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	return nil
}
func (s *Coordinator) validReset(r resetRecord) error {
	q := r.Request
	c := s.meta.Marker.Config
	if q.Version != 1 || q.CoordinatorID != s.meta.Marker.CoordinatorID || q.AuthorityID != c.AuthorityID || !validID(q.EventID) || !validID(q.Nonce) || q.IssuedAtMS <= 0 || q.IssuedAtMS > math.MaxInt64-LifetimeMS || q.ExpiresAtMS != q.IssuedAtMS+LifetimeMS {
		return ErrCorrupt
	}
	if r.Phase == "issued" {
		if r.Semantic != nil || r.OperationKey != "" || r.Barrier != nil {
			return ErrCorrupt
		}
		return nil
	}
	if r.Semantic == nil || !validID(r.OperationKey) {
		return ErrCorrupt
	}
	p := r.Semantic
	if p.Issuer != c.Issuer || !validID(p.ProviderOperationID) || p.FromEpoch == 0 || p.FromEpoch == math.MaxUint64 || p.ToEpoch != p.FromEpoch+1 || p.VerifiedAtMS <= 0 || p.VerifiedAtMS >= q.ExpiresAtMS || p.VerifiedAtMS/1000 < q.IssuedAtMS/1000 {
		return ErrCorrupt
	}
	if _, e := gymaccount.PrincipalID(p.Issuer, p.Subject); e != nil {
		return ErrCorrupt
	}
	if r.OperationKey != s.index("reset operation", p.Issuer, p.Subject, p.ProviderOperationID) {
		return ErrCorrupt
	}
	switch r.Phase {
	case "begin_prepared":
		if r.Barrier != nil {
			return ErrCorrupt
		}
	case "pending", "complete_prepared":
		if r.Barrier == nil || r.Barrier.Status != "pending" {
			return ErrCorrupt
		}
	case "completed":
		if r.Barrier == nil || r.Barrier.Status != "completed" {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	if r.Barrier != nil && s.validBarrier(r, *r.Barrier) != nil {
		return ErrCorrupt
	}
	return nil
}
func (s *Coordinator) audit() error {
	if s.meta.ClockMS < 0 || s.meta.ClockMS > math.MaxInt64-LifetimeMS || s.meta.LoginFlows > s.meta.Marker.Config.MaxLoginFlows || s.meta.ResetFlows > s.meta.Marker.Config.MaxResetFlows {
		return ErrCorrupt
	}
	logins := map[string]loginRecord{}
	resets := map[string]resetRecord{}
	origins := map[string]originRecord{}
	operations := map[string]string{}
	attempts := map[string]bool{}
	maxRows := 1 + 2*int(s.meta.Marker.Config.MaxLoginFlows) + 2*int(s.meta.Marker.Config.MaxResetFlows)
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	count := 0
	for it.Next() {
		count++
		if count > maxRows || len(it.Value()) > MaxRecordSize {
			return ErrCorrupt
		}
		key := string(it.Key())
		switch {
		case key == "meta":
		case strings.HasPrefix(key, "login:"):
			id := strings.TrimPrefix(key, "login:")
			var r loginRecord
			if !validID(id) || canonical(it.Value(), &r) != nil || r.Flow.ID != id || s.validLogin(r) != nil {
				return ErrCorrupt
			}
			if r.Flow.IssuedAtMS > s.meta.ClockMS || r.Semantic != nil && r.Semantic.AuthenticatedAtMS > s.meta.ClockMS {
				return ErrCorrupt
			}
			logins[id] = r
			if r.Flow.AttemptID != "" {
				if attempts[r.Flow.AttemptID] {
					return ErrCorrupt
				}
				attempts[r.Flow.AttemptID] = true
			}
		case strings.HasPrefix(key, "reset:"):
			id := strings.TrimPrefix(key, "reset:")
			var r resetRecord
			if !validID(id) || canonical(it.Value(), &r) != nil || r.Request.EventID != id || s.validReset(r) != nil {
				return ErrCorrupt
			}
			if r.Request.IssuedAtMS > s.meta.ClockMS || r.Semantic != nil && r.Semantic.VerifiedAtMS > s.meta.ClockMS || r.Barrier != nil && (r.Barrier.BeganAtMS > s.meta.ClockMS || r.Barrier.CompletedAtMS > s.meta.ClockMS) {
				return ErrCorrupt
			}
			resets[id] = r
		case strings.HasPrefix(key, "origin:"):
			id := strings.TrimPrefix(key, "origin:")
			var r originRecord
			if !validID(id) || canonical(it.Value(), &r) != nil || r.Semantic.Mode != "fresh" || !validID(r.AttemptID) || r.Sequence == 0 || (r.State != "prepared" && r.State != "accepted") {
				return ErrCorrupt
			}
			origins[id] = r
		case strings.HasPrefix(key, "operation:"):
			id := strings.TrimPrefix(key, "operation:")
			var event string
			if !validID(id) || canonical(it.Value(), &event) != nil || !validID(event) {
				return ErrCorrupt
			}
			operations[id] = event
		default:
			return ErrCorrupt
		}
	}
	if it.Error() != nil || uint32(len(logins)) != s.meta.LoginFlows || uint32(len(resets)) != s.meta.ResetFlows {
		return ErrCorrupt
	}
	for id, o := range origins {
		r, ok := logins[o.Semantic.OriginalFlowID]
		if !ok || r.Semantic == nil || r.OriginKey != id || *r.Semantic != o.Semantic || r.Flow.AttemptID != o.AttemptID || r.Flow.Sequence != o.Sequence {
			return ErrCorrupt
		}
		if o.State == "accepted" && r.Phase != "completed" || o.State == "prepared" && (r.Phase != "submitting" && r.Phase != "consumed_or_unknown") {
			return ErrCorrupt
		}
	}
	for _, r := range logins {
		if r.Semantic == nil {
			continue
		}
		o, ok := origins[r.OriginKey]
		if !ok || !sameOrigin(o, *r.Semantic) {
			return ErrCorrupt
		}
		if r.Semantic.Mode == "reuse" && o.State != "accepted" {
			return ErrCorrupt
		}
	}
	for id, event := range operations {
		r, ok := resets[event]
		if !ok || r.OperationKey != id || r.Semantic == nil {
			return ErrCorrupt
		}
	}
	for id, r := range resets {
		if r.Semantic != nil && operations[r.OperationKey] != id {
			return ErrCorrupt
		}
	}
	return nil
}
