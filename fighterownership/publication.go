// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/registry"
)

func (s *Store) mutation(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil || s == nil {
		return nil, nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	select {
	case s.gate <- struct{}{}:
	default:
		return nil, nil, ErrBusy
	}
	owned, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	done := func() { stop(); cancel(); <-s.gate }
	if !s.mu.TryLock() {
		done()
		return nil, nil, ErrBusy
	}
	e := s.state()
	if e == nil && s.readOnly {
		e = ErrUnavailable
	}
	if e == nil && s.replay != nil && !s.replay.closed.Load() {
		e = ErrBusy
	}
	s.mu.Unlock()
	if e != nil {
		done()
		return nil, nil, e
	}
	return owned, done, nil
}

// RecordChainObservation signs only an actual unchanged observation from the
// concrete node, then commits through the registry's exact durable intent.
func (s *Store) RecordChainObservation(ctx context.Context, source *PublicSource, node *OwnedChainSource) (*PublicSource, PublicationEntry, error) {
	if source == nil || source.publisher == nil || node == nil || source.pin != s.pin || node.pin != s.pin {
		return nil, PublicationEntry{}, ErrSource
	}
	ctx, done, e := s.mutation(ctx)
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	defer done()
	if e = source.Check(ctx); e != nil {
		return nil, PublicationEntry{}, e
	}
	observation, e := node.Observe(ctx)
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	previous, e := source.publisher.ChainHeadID(ctx)
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	prepared, e := source.publisher.PrepareChainPublication(ctx, registry.PublicChainReference{Height: observation.Height, Hash: observation.Hash, PreviousChainEntryID: previous})
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	entry := prepared.Entry()
	if e = source.Check(ctx); e != nil {
		return nil, PublicationEntry{}, e
	}
	if e = node.Check(ctx, observation); e != nil {
		return nil, PublicationEntry{}, e
	}
	entry.Signature, e = signObject("SLUG fighter ownership publication statement", entry)
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	if e = prepared.Check(ctx); e != nil {
		return nil, PublicationEntry{}, e
	}
	if e = node.Check(ctx, observation); e != nil {
		return nil, PublicationEntry{}, e
	}
	entry, e = prepared.CommitSigned(ctx, entry)
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	next, e := NewPublicSource(ctx, s.pin, source.publisher)
	return next, entry, e
}

// CaptureBoundary receives EventID/noon only from the concrete pinned event
// coordinator. Its signature attests the actual source/node/time bracket; the
// event consumer must separately compare its immutable calendar/event identity.
func (s *Store) CaptureBoundary(ctx context.Context, source *PublicSource, node *OwnedChainSource, eventID string, cutoffMS int64) (SignedBoundary, error) {
	if source == nil || source.publisher == nil || node == nil || source.pin != s.pin || node.pin != s.pin || !hexValue(eventID, 32, true) || cutoffMS < 0 || cutoffMS > MaxTimestampMS {
		return SignedBoundary{}, ErrInvalid
	}
	ctx, done, e := s.mutation(ctx)
	if e != nil {
		return SignedBoundary{}, e
	}
	defer done()
	if e = source.Check(ctx); e != nil {
		return SignedBoundary{}, e
	}
	intent, e := source.publisher.CutoffIntent(ctx, eventID)
	if e != nil && !errors.Is(e, registry.ErrNotFound) {
		return SignedBoundary{}, e
	}
	if e == nil {
		if intent.CutoffMS != cutoffMS {
			return SignedBoundary{}, ErrConflict
		}
		return s.persistBoundary(ctx, source, intent)
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	lease, e := source.publisher.BeginBoundary(bounded)
	if e != nil {
		return SignedBoundary{}, e
	}
	defer lease.Close()
	status := lease.Status()
	previous, e := source.publisher.PreviousCutoffID(bounded, cutoffMS)
	if e != nil {
		return SignedBoundary{}, e
	}
	s.mu.Lock()
	clockHigh := s.meta.ClockHighWaterMS
	clock := s.now
	finalitySequence := s.meta.FinalitySequence
	finalityHead := zeroID
	if finalitySequence > 0 {
		body, ok := s.frameBodies[finalitySequence]
		if !ok {
			s.mu.Unlock()
			return SignedBoundary{}, ErrUnavailable
		}
		finalityHead, e = objectDigest("SLUG fighter ownership finality body", body)
		if e != nil {
			s.mu.Unlock()
			return SignedBoundary{}, e
		}
	}
	s.mu.Unlock()
	start := clock().UnixMilli()
	if start < clockHigh || start < 0 || start >= cutoffMS || cutoffMS-start > 10000 {
		return SignedBoundary{}, ErrUnavailable
	}
	first, e := node.Observe(bounded)
	if e != nil {
		return SignedBoundary{}, e
	}
	firstEnd := clock().UnixMilli()
	if firstEnd < start || firstEnd >= cutoffMS {
		return SignedBoundary{}, ErrUnavailable
	}
	timer := time.NewTimer(time.Duration(cutoffMS-firstEnd) * time.Millisecond)
	select {
	case <-bounded.Done():
		timer.Stop()
		return SignedBoundary{}, bounded.Err()
	case <-timer.C:
	}
	secondStart := clock().UnixMilli()
	if secondStart < cutoffMS || secondStart < firstEnd {
		return SignedBoundary{}, ErrClock
	}
	second, e := node.Observe(bounded)
	if e != nil {
		return SignedBoundary{}, e
	}
	end := clock().UnixMilli()
	if end < secondStart || end > MaxTimestampMS || end-start > 10000 || first != second {
		return SignedBoundary{}, ErrUnavailable
	}
	intent = CutoffIntent{Version: 1, PinID: s.pinID, EventID: eventID, CutoffMS: cutoffMS,
		First:                     BoundaryObservation{RequestStartMS: start, ResponseEndMS: firstEnd, InstanceID: first.InstanceID, Revision: first.Revision, Height: first.Height, Hash: first.Hash},
		Second:                    BoundaryObservation{RequestStartMS: secondStart, ResponseEndMS: end, InstanceID: second.InstanceID, Revision: second.Revision, Height: second.Height, Hash: second.Hash},
		AcceptedAdmissionSequence: status.AcceptedSequence, PublicationSequence: status.PublicationSequence, PublicationHead: status.PublicationHead,
		FinalitySequence: finalitySequence, FinalityBodyHead: finalityHead, PriorCutoffID: previous}
	if e = registry.ValidatePublicCutoffIntent(s.pin, intent); e != nil {
		return SignedBoundary{}, e
	}
	if e = lease.CommitIntent(bounded, intent); e != nil {
		return SignedBoundary{}, e
	}
	if e = lease.Close(); e != nil {
		return SignedBoundary{}, e
	}
	return s.persistBoundary(ctx, source, intent)
}
func (s *Store) persistBoundary(ctx context.Context, source *PublicSource, intent CutoffIntent) (SignedBoundary, error) {
	if e := source.Alive(); e != nil {
		return SignedBoundary{}, e
	}
	if e := ctx.Err(); e != nil {
		return SignedBoundary{}, e
	}
	boundary := SignedBoundary{Version: 1, PinID: s.pinID, Intent: intent}
	var e error
	boundary.Signature, e = signObject("SLUG fighter ownership boundary statement", boundary)
	if e != nil {
		return SignedBoundary{}, e
	}
	if e = registry.ValidatePublicBoundary(s.pin, boundary); e != nil {
		return SignedBoundary{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e = s.state(); e != nil {
		return SignedBoundary{}, e
	}
	if old, ok := s.boundaries[intent.EventID]; ok {
		if old != boundary {
			return SignedBoundary{}, ErrConflict
		}
		return old, nil
	}
	if e = ctx.Err(); e != nil {
		return SignedBoundary{}, e
	}
	batch := new(leveldb.Batch)
	raw, _ := json.Marshal(boundary)
	batch.Put(idKey("b/", intent.EventID), raw)
	meta := s.meta
	if intent.Second.ResponseEndMS > meta.ClockHighWaterMS {
		meta.ClockHighWaterMS = intent.Second.ResponseEndMS
	}
	if e = s.commit(batch, meta); e != nil {
		return SignedBoundary{}, e
	}
	s.boundaries[intent.EventID] = boundary
	return boundary, nil
}
