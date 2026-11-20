// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"errors"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/seednode"
)

type OwnedCleanupError struct {
	Owner *OwnedStore
	cause error
}

func (e *OwnedCleanupError) Error() string { return "owned seed cleanup failed: " + e.cause.Error() }
func (e *OwnedCleanupError) Unwrap() error { return e.cause }
func ownedFailed(s *OwnedStore, e error) (*OwnedStore, error) {
	if cleanup := s.Close(); cleanup != nil {
		return nil, &OwnedCleanupError{s, errors.Join(e, cleanup)}
	}
	return nil, e
}
func ownedMapped(e error) error {
	if errors.Is(e, ErrOutcomeUnknown) || errors.Is(e, gymevent.ErrOutcomeUnknown) || errors.Is(e, seednode.ErrOutcomeUnknown) {
		return errors.Join(ErrOutcomeUnknown, e)
	}
	return mapped(e)
}
