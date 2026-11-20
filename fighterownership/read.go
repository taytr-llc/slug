// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
	"sort"
)

func (c *OwnedCapture) Head() HeadRef {
	if c == nil {
		return HeadRef{}
	}
	return c.head
}
func (c *OwnedCapture) GymPublicKey() string {
	if c == nil {
		return ""
	}
	return c.gym
}
func (s *Store) readLock(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if s == nil {
		return ErrClosed
	}
	if !s.mu.TryLock() {
		return ErrBusy
	}
	if e := s.state(); e != nil {
		s.mu.Unlock()
		return e
	}
	if !s.ready {
		s.mu.Unlock()
		return ErrUnavailable
	}
	return nil
}
func (s *Store) Head(ctx context.Context) (HeadRef, error) {
	if e := s.readLock(ctx); e != nil {
		return HeadRef{}, e
	}
	defer s.mu.Unlock()
	return s.head, ctx.Err()
}
func (s *Store) AcquireHead(ctx context.Context, expected HeadRef) (*HeadLease, error) {
	if e := s.readLock(ctx); e != nil {
		return nil, e
	}
	defer s.mu.Unlock()
	if expected != s.head {
		return nil, ErrStale
	}
	return &HeadLease{store: s, source: s.source, node: s.node, head: s.head, generation: s.generation}, nil
}
func (s *Store) CaptureOwned(ctx context.Context, gym string) (*OwnedCapture, error) {
	if !gymKey(gym) {
		return nil, ErrInvalid
	}
	if e := s.readLock(ctx); e != nil {
		return nil, e
	}
	defer s.mu.Unlock()
	c := &OwnedCapture{store: s, head: s.head, gym: gym, generation: s.generation, items: make([]MapItem, 0)}
	m, ok := s.maps[s.meta.CurrentMapID]
	if !ok {
		return nil, ErrCorrupt
	}
	for _, item := range m.Items {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		state, ok := s.states[item.StateID]
		if !ok {
			return nil, ErrCorrupt
		}
		if state.OwnerPublicKey == gym {
			c.items = append(c.items, item)
		}
	}
	return c, nil
}
func (s *Store) captureItem(c *OwnedCapture, m MapItem) (OwnedItem, error) {
	if c == nil || c.store != s || c.generation != s.generation {
		return OwnedItem{}, ErrStale
	}
	a, ok := s.admissions[m.AdmissionSequence]
	if !ok || a.Record.Verified.ProspectID != m.ProspectID {
		return OwnedItem{}, ErrCorrupt
	}
	state, ok := s.states[m.StateID]
	if !ok || state.ProspectID != m.ProspectID || state.OwnerPublicKey != c.gym {
		return OwnedItem{}, ErrCorrupt
	}
	return cloneItem(OwnedItem{a, state}), nil
}
func (s *Store) project(ctx context.Context) (func(), error) {
	if ctx == nil || s == nil {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	select {
	case s.projection <- struct{}{}:
		return func() { <-s.projection }, nil
	default:
		return nil, ErrBusy
	}
}
func (s *Store) OwnedPage(ctx context.Context, c *OwnedCapture, after, limit uint32) (OwnedPage, error) {
	if limit < 1 || limit > 8 {
		return OwnedPage{}, ErrInvalid
	}
	done, e := s.project(ctx)
	if e != nil {
		return OwnedPage{}, e
	}
	defer done()
	if e = s.readLock(ctx); e != nil {
		return OwnedPage{}, e
	}
	defer s.mu.Unlock()
	if c == nil || c.store != s || c.generation != s.generation || after > c.head.AdmissionSequence {
		return OwnedPage{}, ErrStale
	}
	out := OwnedPage{Version: 1, PinID: s.pinID, Head: c.head, GymPublicKey: c.gym, After: after, Next: after, Items: make([]OwnedItem, 0, limit)}
	for _, item := range c.items {
		if item.AdmissionSequence <= after {
			continue
		}
		if e = ctx.Err(); e != nil {
			return OwnedPage{}, e
		}
		v, e := s.captureItem(c, item)
		if e != nil {
			return OwnedPage{}, e
		}
		out.Items = append(out.Items, v)
		out.Next = item.AdmissionSequence
		if len(out.Items) == int(limit) {
			break
		}
	}
	out.Complete = true
	for _, item := range c.items {
		if item.AdmissionSequence > out.Next {
			out.Complete = false
			break
		}
	}
	if out.Complete {
		out.Next = c.head.AdmissionSequence
	}
	raw, e := json.Marshal(out)
	if e != nil || len(raw) > MaxPageBytes {
		return OwnedPage{}, ErrCapacity
	}
	return out, nil
}
func (s *Store) OwnedItem(ctx context.Context, c *OwnedCapture, id string) (OwnedItem, error) {
	if !hexValue(id, 32, true) {
		return OwnedItem{}, ErrInvalid
	}
	done, e := s.project(ctx)
	if e != nil {
		return OwnedItem{}, e
	}
	defer done()
	if e = s.readLock(ctx); e != nil {
		return OwnedItem{}, e
	}
	defer s.mu.Unlock()
	if c == nil || c.store != s || c.generation != s.generation {
		return OwnedItem{}, ErrStale
	}
	for _, m := range c.items {
		if e = ctx.Err(); e != nil {
			return OwnedItem{}, e
		}
		if m.ProspectID == id {
			return s.captureItem(c, m)
		}
	}
	return OwnedItem{}, ErrNotFound
}
func (s *Store) Cutoff(ctx context.Context, event string) (CutoffRecord, error) {
	if !hexValue(event, 32, true) {
		return CutoffRecord{}, ErrInvalid
	}
	if e := s.readLock(ctx); e != nil {
		return CutoffRecord{}, e
	}
	defer s.mu.Unlock()
	v, ok := s.cutoffs[event]
	if !ok {
		return CutoffRecord{}, ErrNotFound
	}
	return v, ctx.Err()
}
func (s *Store) ResolveCutoff(ctx context.Context, id string, ids []string) (CutoffOwners, error) {
	if !hexValue(id, 32, true) || len(ids) > 10 || !sort.StringsAreSorted(ids) {
		return CutoffOwners{}, ErrInvalid
	}
	for n, p := range ids {
		if !hexValue(p, 32, true) || n > 0 && p == ids[n-1] {
			return CutoffOwners{}, ErrInvalid
		}
	}
	done, e := s.project(ctx)
	if e != nil {
		return CutoffOwners{}, e
	}
	defer done()
	if e = s.readLock(ctx); e != nil {
		return CutoffOwners{}, e
	}
	defer s.mu.Unlock()
	event, ok := s.cutoffByID[id]
	if !ok {
		return CutoffOwners{}, ErrNotFound
	}
	cutoff, ok := s.cutoffs[event]
	if !ok {
		return CutoffOwners{}, ErrCorrupt
	}
	m, ok := s.maps[cutoff.OwnershipDigest]
	if !ok {
		return CutoffOwners{}, ErrUnavailable
	}
	out := CutoffOwners{Cutoff: cutoff, Head: cutoff.Head, Items: make([]OwnedItem, 0, len(ids))}
	for _, prospect := range ids {
		if e = ctx.Err(); e != nil {
			return CutoffOwners{}, e
		}
		found := false
		for _, item := range m.Items {
			if item.ProspectID != prospect {
				continue
			}
			state, ok := s.states[item.StateID]
			a, admitted := s.admissions[item.AdmissionSequence]
			if !ok || !admitted || a.Record.Verified.ProspectID != prospect {
				return CutoffOwners{}, ErrUnavailable
			}
			out.Items = append(out.Items, cloneItem(OwnedItem{a, state}))
			found = true
			break
		}
		if !found {
			return CutoffOwners{}, ErrNotFound
		}
	}
	return out, nil
}
