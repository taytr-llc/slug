// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import "github.com/taytr-llc/slug/registry"

// OperatorCodec verifies public projections under an independently retained
// operator profile. It grants neither signing authority nor current ownership.
type OperatorCodec struct{ profile registry.OperatorProfile }

func NewOperatorCodec(profile registry.OperatorProfile) (OperatorCodec, error) {
	if profile.Validate() != nil {
		return OperatorCodec{}, ErrSource
	}
	return OperatorCodec{profile: profile}, nil
}
func (codec OperatorCodec) ValidateEffectiveState(v EffectiveState) error {
	return (ownershipCodec{operator: true, profile: codec.profile}).ValidateEffectiveState(codec.profile.Pin(), v)
}

func (codec OperatorCodec) ParseEffectiveState(raw []byte) (EffectiveState, error) {
	return (ownershipCodec{operator: true, profile: codec.profile}).ParseEffectiveState(codec.profile.Pin(), raw)
}

func (codec OperatorCodec) MarshalEffectiveState(v EffectiveState) ([]byte, error) {
	return (ownershipCodec{operator: true, profile: codec.profile}).MarshalEffectiveState(codec.profile.Pin(), v)
}

func (codec OperatorCodec) InitialState(a AdmissionStatement) (EffectiveState, error) {
	return (ownershipCodec{operator: true, profile: codec.profile}).InitialState(codec.profile.Pin(), a)
}

func (codec OperatorCodec) ValidateOwnershipMap(m OwnershipMap) error {
	return (ownershipCodec{operator: true, profile: codec.profile}).ValidateOwnershipMap(codec.profile.Pin(), m)
}

func (codec OperatorCodec) OwnershipMapID(m OwnershipMap) (string, error) {
	return (ownershipCodec{operator: true, profile: codec.profile}).OwnershipMapID(codec.profile.Pin(), m)
}

func (codec OperatorCodec) FinalizedPrefixID(m OwnershipMap) (string, error) {
	return (ownershipCodec{operator: true, profile: codec.profile}).FinalizedPrefixID(codec.profile.Pin(), m)
}

func (codec OperatorCodec) ParseOwnershipMap(raw []byte) (OwnershipMap, error) {
	return (ownershipCodec{operator: true, profile: codec.profile}).ParseOwnershipMap(codec.profile.Pin(), raw)
}

func (codec OperatorCodec) ValidateHead(h HeadRef) error {
	return (ownershipCodec{operator: true, profile: codec.profile}).ValidateHead(codec.profile.Pin(), h)
}

func (codec OperatorCodec) HeadID(h HeadRef) (string, error) {
	return (ownershipCodec{operator: true, profile: codec.profile}).HeadID(codec.profile.Pin(), h)
}

func (codec OperatorCodec) ValidateFinalityFrame(v FinalityFrame) error {
	return (ownershipCodec{operator: true, profile: codec.profile}).ValidateFinalityFrame(codec.profile.Pin(), v)
}

func (codec OperatorCodec) FinalityFrameID(v FinalityFrame) (string, error) {
	return (ownershipCodec{operator: true, profile: codec.profile}).FinalityFrameID(codec.profile.Pin(), v)
}
