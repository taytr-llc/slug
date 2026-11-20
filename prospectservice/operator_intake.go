// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectservice

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

var ErrIntakeDraining = errors.New("operator admission intake is draining")
var ErrIntakeUnavailable = errors.New("operator admission intake unavailable")

type operatorIntakeState struct{ enabled, draining bool }

// NewPausedFencedManifestAdmissionServer is the explicit operator route. New
// admission begins paused; exact historical receipt recovery remains available.
// The old constructor and its session/byte contracts are unchanged.
func NewPausedFencedManifestAdmissionServer(config ManifestServerConfig, policy prospectnode.Policy, node ManifestNode, store *registry.ManifestStore, sessions SessionAuthorizer) (*ManifestServer, error) {
	s, err := NewFencedManifestAdmissionServer(config, policy, node, store, sessions)
	if err != nil {
		return nil, err
	}
	s.operatorIntake = &operatorIntakeState{}
	return s, nil
}

// withOperatorIntake participates in Run's original handler join and serializes
// with the entire admitted proof/session/commit operation. It takes no account
// lease. A close racing the wait is checked again after acquiring active.
func (s *ManifestServer) withOperatorIntake(ctx context.Context, fn func(*operatorIntakeState) error) error {
	if s == nil || ctx == nil || s.operatorIntake == nil || s.active == nil {
		return ErrIntakeUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.beginHandler() {
		return ErrIntakeUnavailable
	}
	defer s.handlers.Done()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	select {
	case s.active <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.active }()
	s.lifecycle.Lock()
	closed := s.closing
	s.lifecycle.Unlock()
	if closed || s.poisoned.Load() {
		return ErrIntakeUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(s.operatorIntake)
}

func (s *ManifestServer) EnableOperatorIntake(ctx context.Context) error {
	return s.withOperatorIntake(ctx, func(i *operatorIntakeState) error {
		if i.draining {
			return ErrIntakeDraining
		}
		i.enabled = true
		return nil
	})
}

// DrainOperatorIntake joins the current admission, then permanently refuses new
// records for this server lifetime. It does not disable historical receipts or
// close the issuer, node, credentials, listener, or unrelated financial owners.
func (s *ManifestServer) DrainOperatorIntake(ctx context.Context) error {
	return s.withOperatorIntake(ctx, func(i *operatorIntakeState) error { i.draining = true; return nil })
}

// OperatorIntakeMode is a local observation, not proof of remaining capacity.
func (s *ManifestServer) OperatorIntakeMode(ctx context.Context) (string, error) {
	var mode string
	err := s.withOperatorIntake(ctx, func(i *operatorIntakeState) error {
		mode = "paused"
		if i.enabled {
			mode = "enabled"
		}
		if i.draining {
			mode = "draining"
		}
		return nil
	})
	return mode, err
}
