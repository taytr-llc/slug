package gymbout

import (
	"errors"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
)

type OwnedCleanupError struct {
	Owner *OwnedStore
	Cause error
}

func (e *OwnedCleanupError) Error() string { return "owned bout cleanup: " + e.Cause.Error() }
func (e *OwnedCleanupError) Unwrap() error { return e.Cause }
func ownedFailed(s *OwnedStore, e error) (*OwnedStore, error) {
	if c := s.Close(); c != nil {
		return nil, &OwnedCleanupError{s, errors.Join(e, c)}
	}
	return nil, e
}

func ownedMapped(e error) error {
	if errors.Is(e, ErrOutcomeUnknown) || errors.Is(e, gymcard.ErrOutcomeUnknown) || errors.Is(e, gymseed.ErrOutcomeUnknown) || errors.Is(e, gymevent.ErrOutcomeUnknown) || errors.Is(e, gymentry.ErrOutcomeUnknown) || errors.Is(e, gymstable.ErrOutcomeUnknown) || errors.Is(e, seednode.ErrOutcomeUnknown) {
		return errors.Join(ErrOutcomeUnknown, e)
	}
	return mapped(e)
}
