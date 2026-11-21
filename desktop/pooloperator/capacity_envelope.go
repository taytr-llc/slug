// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import "github.com/taytr-llc/slug/internal/poolcapacity"

// CapacityReservation is the original finite ordinary-intake reservation.
// Canonical recovery proves its separate whole-history bound before each append.
type CapacityReservation = poolcapacity.Reservation
type capacityInputs = poolcapacity.Inputs

func CapacityEnvelope(s Setup) (CapacityReservation, error) {
	if err := ValidateSetup(s); err != nil {
		return CapacityReservation{}, err
	}
	return capacityEnvelope(capacityInputs{
		SourceMax: s.Pool.MaxSubmissions, Rounds: s.RoundLimits.MaxRounds, RoundBytes: s.RoundLimits.MaxBytes,
		CreditRounds: uint64(s.Settlement.MaxRounds), Batches: uint64(s.Settlement.MaxBatches), Gyms: uint64(len(s.Settlement.Destinations)),
	})
}

func capacityEnvelope(in capacityInputs) (CapacityReservation, error) {
	bound, err := poolcapacity.Envelope(in)
	if err != nil {
		return CapacityReservation{}, ErrInvalid
	}
	return bound, nil
}
