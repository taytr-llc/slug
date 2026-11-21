// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"github.com/taytr-llc/slug/internal/poolcapacity"
)

func ValidateDailySetup(s Setup) error          { return validateSetupVersion(s, 2) }
func MarshalDailySetup(s Setup) ([]byte, error) { return marshalSetupVersion(s, 2) }
func ParseDailySetup(raw []byte) (Setup, error) { return parseSetupVersion(raw, 2) }
func DailySetupID(s Setup) (string, error)      { return setupIDVersion(s, 2) }
func ValidateDailyProvisionPolicy(p ProvisionPolicy) error {
	return validateProvisionPolicyVersion(p, 2)
}
func ParseDailyProvisionPolicy(raw []byte) (ProvisionPolicy, error) {
	return parseProvisionPolicyVersion(raw, 2)
}
func DailyProvisionPolicyID(p ProvisionPolicy) (string, error) { return provisionPolicyIDVersion(p, 2) }
func ComposeDailyProvisionSetup(p ProvisionPolicy, m ProvisionPublicMaterial, c ProvisionCertificate) (Setup, error) {
	return composeProvisionSetupVersion(p, m, c, 2)
}

// DailyCapacityReservation is the exact derived finite intake reservation. The
// bound includes the reserved unseen canonical row, not a caller-selected quota.
type DailyCapacityReservation struct {
	OrdinaryMaximum uint64                        `json:"ordinary_maximum"`
	Reservation     poolcapacity.DailyReservation `json:"reservation"`
}

func DailyCapacityEnvelope(s Setup) (DailyCapacityReservation, error) {
	if e := ValidateDailySetup(s); e != nil {
		return DailyCapacityReservation{}, e
	}
	n, r, e := poolcapacity.DailyEnvelope(poolcapacity.DailyInputs{SourceMax: s.Pool.MaxSubmissions, SourceBytes: s.Pool.MaxBytes, Rounds: s.RoundLimits.MaxRounds, RoundBytes: s.RoundLimits.MaxBytes, CreditRounds: uint64(s.Settlement.MaxRounds), Batches: uint64(s.Settlement.MaxBatches), Gyms: uint64(len(s.Settlement.Destinations))})
	if e != nil {
		return DailyCapacityReservation{}, ErrInvalid
	}
	return DailyCapacityReservation{OrdinaryMaximum: n, Reservation: r}, nil
}

func CreateDaily(ctx context.Context, root string, expected Setup, borrowed BorrowedOwners, private PrivateInputs) (*Runtime, error) {
	return constructVersion(ctx, root, expected, borrowed, private, true, true)
}
func OpenDaily(ctx context.Context, root string, expected Setup, borrowed BorrowedOwners, private PrivateInputs) (*Runtime, error) {
	return constructVersion(ctx, root, expected, borrowed, private, false, true)
}
func operatorIntentName(version uint32) string {
	if version == 2 {
		return "SLUG-POOL-OPERATOR-V2.json"
	}
	return "SLUG-POOL-OPERATOR-V1.json"
}
func operatorReadyName(version uint32) string {
	if version == 2 {
		return "SLUG-POOL-OPERATOR-READY-V2.json"
	}
	return "SLUG-POOL-OPERATOR-READY-V1.json"
}
func provisionIntentFile(version uint32) string {
	if version == 3 {
		return "SLUG-POOL-PROVISION-INTENT-ISSUE1.json"
	}
	if version == 2 {
		return "SLUG-POOL-PROVISION-INTENT-V2.json"
	}
	return provisionIntentName
}
func provisionReadyFile(version uint32) string {
	if version == 3 {
		return "SLUG-POOL-PROVISION-READY-ISSUE1.json"
	}
	if version == 2 {
		return "SLUG-POOL-PROVISION-READY-V2.json"
	}
	return provisionReadyName
}
func provisionBackupFile(version uint32) string {
	if version == 3 {
		return "SLUG-OPERATOR-POOL-WALLET-BACKUP-ISSUE1.json"
	}
	if version == 2 {
		return "SLUG-OPERATOR-POOL-WALLET-BACKUP-V2.json"
	}
	return provisionBackupName
}

// Provisioning checks finite liabilities before creating even the public intent.
// Full Setup composition and runtime attachment repeat the complete bindings.
func dailyProvisionCapacity(p ProvisionPolicy) error {
	_, _, e := poolcapacity.DailyEnvelope(poolcapacity.DailyInputs{SourceMax: p.Limits.MaxSubmissions, SourceBytes: p.Limits.PoolMaxBytes, Rounds: uint64(p.Limits.MaxRounds), RoundBytes: p.Limits.RoundMaxBytes, CreditRounds: uint64(p.Limits.MaxRounds), Batches: uint64(p.Limits.MaxBatches), Gyms: uint64(len(p.Enrollments))})
	if e != nil {
		return ErrProvisionInvalid
	}
	return nil
}
