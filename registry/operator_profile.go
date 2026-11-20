// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"encoding/json"

	"github.com/taytr-llc/slug/internal/operatorauthority"
)

// OperatorProfile selects an independently supplied public source. It carries
// no signing key, store ownership, current frontier or ownership permission.
// Existing public-manifest constructors deliberately continue to select v1.
type OperatorProfile struct {
	pin PublicManifestPin
	id  string
}

// NewOperatorProfile validates the explicit v2 pin without accessing storage
// or selecting trust from a server response. The caller supplies its expected
// complete pin; parsing or validation cannot establish that expectation.
func NewOperatorProfile(pin PublicManifestPin) (OperatorProfile, error) {
	if err := validateOperatorPin(pin); err != nil {
		return OperatorProfile{}, err
	}
	id, err := PublicOwnershipDigest("SLUG fighter ownership pin", pin)
	if err != nil {
		return OperatorProfile{}, err
	}
	return OperatorProfile{pin: pin, id: id}, nil
}

func validateOperatorPin(pin PublicManifestPin) error {
	if _, err := validateManifestPin(pin.Source); err != nil {
		return ErrPublicOwnership
	}
	if pin.Version != 2 || !publicHex(pin.Source.AuthorityID, 32, true) ||
		!publicHex(pin.PublicationID, 32, true) ||
		pin.RuleID != PublicOwnershipRuleID() || pin.Limits != publicLimits ||
		pin.ScanOrigin != (PublicChainPoint{0, pin.Source.Genesis}) {
		return ErrPublicOwnership
	}
	keys := operatorauthority.Keys{
		Admission: pin.AdmissionKey, Publication: pin.PublicationKey,
		Cutoff: pin.CutoffKey, Checkpoint: pin.CheckpointKey,
	}
	if operatorauthority.ValidateKeys(keys) != nil {
		return ErrPublicOwnership
	}
	return nil
}

// Validate rejects the zero profile and preserves the complete immutable pin.
func (p OperatorProfile) Validate() error {
	if validateOperatorPin(p.pin) != nil {
		return ErrPublicOwnership
	}
	id, err := PublicOwnershipDigest("SLUG fighter ownership pin", p.pin)
	if err != nil || id != p.id {
		return ErrPublicOwnership
	}
	return nil
}

// Pin returns a public value copy. It does not make legacy v1 APIs accept v2.
func (p OperatorProfile) Pin() PublicManifestPin { return p.pin }

// ID returns the full pin commitment, or the empty string for a zero profile.
func (p OperatorProfile) ID() string { return p.id }

// ParseOperatorProfile accepts only the canonical v2 pin grammar. The result
// must still be compared with independently retained configuration before use.
func ParseOperatorProfile(raw []byte) (OperatorProfile, error) {
	var pin PublicManifestPin
	if err := publicDecode(raw, MaxPublicSmallSize, &pin); err != nil {
		return OperatorProfile{}, err
	}
	return NewOperatorProfile(pin)
}

func (p OperatorProfile) Marshal() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(p.pin)
}
