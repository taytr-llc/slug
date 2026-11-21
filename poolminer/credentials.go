// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

import (
	"context"
	"crypto/subtle"
	"errors"
	"sort"
	"sync/atomic"
)

func sameHash(a, b string) bool {
	return len(a) == 64 && len(b) == 64 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func (s *Store) enrolled(gym string) (Enrollment, bool) {
	for _, e := range s.state.Config.Enrollments {
		if e.GymID == gym {
			return e, true
		}
	}
	return Enrollment{}, false
}
func credentialIndex(cs []Credential, gym, a string) int {
	for i, c := range cs {
		if c.GymID == gym && c.Audience == a {
			return i
		}
	}
	return -1
}

// Rotate is a local operator action. Token bytes exist only in this call and
// transient digest input; no plaintext is returned, stored, or included in errors.
func (s *Store) Rotate(ctx context.Context, gym, a string, expected uint32, token string, expires int64) (Credential, error) {
	if !identifier(gym) || !audience(a) || !identifier(token) || !validTime(expires) || expected > maxHistory {
		return Credential{}, ErrInvalid
	}
	end, e := s.begin(ctx)
	if e != nil {
		return Credential{}, e
	}
	defer end()
	if _, ok := s.enrolled(gym); !ok {
		return Credential{}, ErrUnauthorized
	}
	hashes, e := tokenHashesVersion(s.configID, a, token, s.version())
	if e != nil {
		return Credential{}, e
	}
	now, e := s.clock(ctx)
	if e != nil {
		return Credential{}, e
	}
	i := credentialIndex(s.state.Credentials, gym, a)
	if i >= 0 {
		old := s.state.Credentials[i]
		if old.Generation == expected+1 && sameHash(old.TokenHash, hashes.TokenHash) && sameHash(old.ReuseHash, hashes.ReuseHash) && old.ExpiresAtMS == expires && old.RevokedAtMS == 0 {
			return old, s.disclose(ctx)
		}
		if old.Generation != expected {
			return Credential{}, ErrConflict
		}
	} else if expected != 0 {
		return Credential{}, ErrConflict
	}
	if expires <= now || expires-now > MaxLifetimeMS {
		return Credential{}, ErrInvalid
	}
	if expected >= maxHistory || len(s.state.History) >= maxHistory || s.state.Revision == ^uint32(0) {
		return Credential{}, ErrCapacity
	}
	for _, h := range s.state.History {
		if sameHash(h.ReuseHash, hashes.ReuseHash) || sameHash(h.TokenHash, hashes.TokenHash) {
			return Credential{}, ErrConflict
		}
	}
	c := Credential{GymID: gym, Audience: a, Generation: expected + 1, TokenHash: hashes.TokenHash, ReuseHash: hashes.ReuseHash, IssuedAtMS: now, ExpiresAtMS: expires}
	n := cloneState(s.state)
	n.Revision++
	n.History = append(n.History, hashes)
	if i < 0 {
		n.Credentials = append(n.Credentials, c)
	} else {
		n.Credentials[i] = c
	}
	sort.Slice(n.Credentials, func(i, j int) bool {
		return n.Credentials[i].GymID+":"+n.Credentials[i].Audience < n.Credentials[j].GymID+":"+n.Credentials[j].Audience
	})
	if e = s.commit(ctx, n); e != nil {
		return Credential{}, e
	}
	return c, nil
}
func (s *Store) Revoke(ctx context.Context, gym, a string, expected uint32) (Credential, error) {
	if !identifier(gym) || !audience(a) || expected == 0 || expected > maxHistory {
		return Credential{}, ErrInvalid
	}
	end, e := s.begin(ctx)
	if e != nil {
		return Credential{}, e
	}
	defer end()
	i := credentialIndex(s.state.Credentials, gym, a)
	if i < 0 || s.state.Credentials[i].Generation != expected {
		return Credential{}, ErrConflict
	}
	now, e := s.clock(ctx)
	if e != nil {
		return Credential{}, e
	}
	c := s.state.Credentials[i]
	if c.RevokedAtMS != 0 {
		return c, s.disclose(ctx)
	}
	c.RevokedAtMS = now
	n := cloneState(s.state)
	n.Revision++
	n.Credentials[i] = c
	if e = s.commit(ctx, n); e != nil {
		return Credential{}, e
	}
	return c, nil
}
func (s *Store) disclose(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	return s.alive()
}

// WithSession implements the original SessionAuthorizer fence. The temporary
// recheck is active only during fn and never reacquires the operation gate.
func (a *Sessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	if a == nil || a.owner == nil || !audience(a.audience) || fn == nil || !identifier(token) {
		return ErrUnauthorized
	}
	s := a.owner
	end, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	hashes, e := tokenHashesVersion(s.configID, a.audience, token, s.version())
	if e != nil {
		return ErrUnauthorized
	}
	var saved *Credential
	for i := range s.state.Credentials {
		c := &s.state.Credentials[i]
		if c.Audience == a.audience && sameHash(c.TokenHash, hashes.TokenHash) && sameHash(c.ReuseHash, hashes.ReuseHash) {
			copy := *c
			saved = &copy
		}
	}
	if saved == nil || saved.RevokedAtMS != 0 {
		return ErrUnauthorized
	}
	en, ok := s.enrolled(saved.GymID)
	if !ok {
		return ErrUnauthorized
	}
	var active atomic.Bool
	active.Store(true)
	defer func() { s.checkMu.Lock(); active.Store(false); s.checkMu.Unlock() }()
	check := func() error {
		s.checkMu.Lock()
		defer s.checkMu.Unlock()
		if !active.Load() {
			return ErrUnauthorized
		}
		if e := s.disclose(ctx); e != nil {
			return e
		}
		now, e := s.clock(ctx)
		if e != nil {
			return e
		}
		if now >= saved.ExpiresAtMS {
			return ErrUnauthorized
		}
		return s.disclose(ctx)
	}
	if e = check(); e != nil {
		return e
	}
	operationErr := fn(en.PublicKey, check)
	afterErr := check()
	return errors.Join(operationErr, afterErr)
}
