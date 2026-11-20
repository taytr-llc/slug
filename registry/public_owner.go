// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"context"
	"sync"
	"sync/atomic"
)

// ManifestPublication is a concrete borrowed source. It cannot be constructed
// from caller receipt bytes and never owns or independently closes the DB.
type ManifestPublication struct {
	store     *ManifestStore
	pin       PublicManifestPin
	pinID     string
	life      context.Context
	cancel    context.CancelFunc
	gate      chan struct{}
	closed    atomic.Bool
	closeOnce sync.Once
}

func (p *ManifestPublication) Pin() PublicManifestPin {
	if p == nil {
		return PublicManifestPin{}
	}
	return p.pin
}
func (p *ManifestPublication) Alive() error {
	if p == nil || p.store == nil || p.closed.Load() || p.life == nil || p.life.Err() != nil {
		return ErrClosed
	}
	return nil
}
func (p *ManifestPublication) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		if p.cancel != nil {
			p.cancel()
		}
		if p.gate != nil {
			p.gate <- struct{}{}
			<-p.gate
		}
	})
	return nil
}
func (p *ManifestPublication) Status(ctx context.Context) (PublicationStatus, error) {
	if ctx == nil {
		return PublicationStatus{}, ErrPublicOwnership
	}
	if e := ctx.Err(); e != nil {
		return PublicationStatus{}, e
	}
	if e := p.Alive(); e != nil {
		return PublicationStatus{}, e
	}
	s := p.store
	if !s.mu.TryLock() {
		return PublicationStatus{}, ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(false); e != nil {
		return PublicationStatus{}, e
	}
	if s.public == nil || s.public.pin != p.pin {
		return PublicationStatus{}, ErrPublicOwnership
	}
	m := s.public.meta
	return PublicationStatus{1, p.pinID, m.AcceptedSequence, m.SignedAdmissionSequence, m.AdmissionHead, m.PublicationSequence, m.PublicationHead, m.AcceptedSequence != m.SignedAdmissionSequence}, nil
}

type publicManifestMetadata struct {
	Version                 uint32 `json:"version"`
	PinID                   string `json:"pin_id"`
	AcceptedSequence        uint32 `json:"accepted_sequence"`
	SignedAdmissionSequence uint32 `json:"signed_admission_sequence"`
	AdmissionHead           string `json:"admission_head"`
	PublicationSequence     uint32 `json:"publication_sequence"`
	PublicationHead         string `json:"publication_head"`
	ChainEntryID            string `json:"chain_entry_id"`
	CheckpointCount         uint32 `json:"checkpoint_count"`
	CutoffCount             uint32 `json:"cutoff_count"`
	LogicalBytes            uint32 `json:"logical_bytes"`
	ClockHighWaterMS        int64  `json:"clock_high_water_ms"`
	PendingPublicationID    string `json:"pending_publication_id"`
}
type publicManifestState struct {
	closing   bool
	boundary  bool
	pin       PublicManifestPin
	pinID     string
	meta      publicManifestMetadata
	publisher *ManifestPublication
}

func (s *ManifestStore) closePublic() {
	s.mu.Lock()
	s.public.closing = true
	p := s.public.publisher
	s.mu.Unlock()
	if p != nil {
		_ = p.Close()
	}
}
