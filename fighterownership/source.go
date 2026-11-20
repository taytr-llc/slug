// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/registry"
)

// PublicSource borrows one exact concrete local publisher or remote public
// reader. Parsed DTOs cannot replace either original lifetime/authority.
type PublicSource struct {
	codec        ownershipCodec
	operator     *operatorauthority.Owner
	publisher    *registry.ManifestPublication
	reader       *PublicReader
	pin          Pin
	status       registry.PublicationStatus
	remoteStatus PublicReadStatus
}

func NewPublicSource(ctx context.Context, expected Pin, publisher *registry.ManifestPublication) (*PublicSource, error) {
	return newSelectedPublicSource(ctx, expected, publisher, ownershipCodec{}, nil)
}
func newSelectedPublicSource(ctx context.Context, expected Pin, publisher *registry.ManifestPublication, codec ownershipCodec, owner *operatorauthority.Owner) (*PublicSource, error) {
	if ctx == nil || publisher == nil || publisher.Pin() != expected {
		return nil, ErrSource
	}
	if _, e := codec.ID(expected); e != nil {
		return nil, e
	}
	status, e := publisher.Status(ctx)
	if e != nil {
		return nil, e
	}
	if codec.operator {
		if owner == nil {
			return nil, ErrSource
		}
		if e := publisher.CheckOperatorOwner(ctx, owner); e != nil {
			return nil, e
		}
	}
	return &PublicSource{codec: codec, operator: owner, publisher: publisher, pin: expected, status: status}, nil
}
func NewRemotePublicSource(ctx context.Context, expected Pin, reader *PublicReader) (*PublicSource, error) {
	return newSelectedRemotePublicSource(ctx, expected, reader, ownershipCodec{})
}
func newSelectedRemotePublicSource(ctx context.Context, expected Pin, reader *PublicReader, codec ownershipCodec) (*PublicSource, error) {
	if ctx == nil || reader == nil || reader.Pin() != expected {
		return nil, ErrSource
	}
	if _, e := codec.ID(expected); e != nil {
		return nil, e
	}
	status, e := reader.Status(ctx)
	if e != nil {
		return nil, e
	}
	if reader.codec != codec {
		return nil, ErrSource
	}
	return &PublicSource{codec: codec, reader: reader, pin: expected, status: status.Publication, remoteStatus: status}, nil
}
func (s *PublicSource) Check(ctx context.Context) error {
	if ctx == nil || s == nil {
		return ErrInvalid
	}
	if s.reader != nil {
		status, e := s.reader.Status(ctx)
		if e != nil {
			return e
		}
		if status != s.remoteStatus {
			return ErrStale
		}
		return nil
	}
	if s.publisher == nil {
		return ErrInvalid
	}
	status, e := s.publisher.Status(ctx)
	if e != nil {
		return e
	}
	if status != s.status {
		return ErrStale
	}
	return nil
}
func (s *PublicSource) Alive() error {
	if s == nil {
		return ErrClosed
	}
	if s.reader != nil {
		return s.reader.Alive()
	}
	if s.publisher == nil {
		return ErrClosed
	}
	return s.publisher.Alive()
}
func (s *PublicSource) sameOwner(other *PublicSource) bool {
	if s == nil || other == nil || s.pin != other.pin || s.codec != other.codec || s.operator != other.operator {
		return false
	}
	if s.reader != nil {
		return s.reader == other.reader && other.publisher == nil
	}
	return s.publisher != nil && s.publisher == other.publisher && other.reader == nil
}
func (s *PublicSource) admission(ctx context.Context, n uint32) (AdmissionStatement, error) {
	if s.reader != nil {
		return s.reader.Admission(ctx, n)
	}
	if s.publisher == nil {
		return AdmissionStatement{}, ErrSource
	}
	return s.publisher.Admission(ctx, n)
}
func (s *PublicSource) publication(ctx context.Context, n uint32) (PublicationEntry, error) {
	if s.reader != nil {
		return s.reader.Publication(ctx, n)
	}
	if s.publisher == nil {
		return PublicationEntry{}, ErrSource
	}
	return s.publisher.Publication(ctx, n)
}
func (s *PublicSource) cutoffIntent(ctx context.Context, id string) (CutoffIntent, error) {
	if s.reader != nil {
		return s.reader.CutoffIntent(ctx, id)
	}
	if s.publisher == nil {
		return CutoffIntent{}, ErrSource
	}
	return s.publisher.CutoffIntent(ctx, id)
}
