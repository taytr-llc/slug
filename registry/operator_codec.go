// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

// OperatorCodec verifies public statements against one independently supplied
// immutable operator profile. It neither signs nor establishes current state.
type OperatorCodec struct{ profile OperatorProfile }

func (p OperatorProfile) Codec() (OperatorCodec, error) {
	if err := p.Validate(); err != nil {
		return OperatorCodec{}, err
	}
	return OperatorCodec{profile: p}, nil
}

func (codec OperatorCodec) ValidateAdmission(a PublicAdmissionStatement) error {
	return (publicEvidenceCodec{operator: true}).ValidatePublicAdmission(codec.profile.pin, a)
}

func (codec OperatorCodec) AdmissionID(a PublicAdmissionStatement) (string, error) {
	return (publicEvidenceCodec{operator: true}).PublicAdmissionID(codec.profile.pin, a)
}

func (codec OperatorCodec) ParseAdmission(raw []byte) (PublicAdmissionStatement, error) {
	return (publicEvidenceCodec{operator: true}).ParsePublicAdmission(codec.profile.pin, raw)
}

func (codec OperatorCodec) MarshalAdmission(a PublicAdmissionStatement) ([]byte, error) {
	return (publicEvidenceCodec{operator: true}).MarshalPublicAdmission(codec.profile.pin, a)
}

func (codec OperatorCodec) ValidatePublication(e PublicPublicationEntry) error {
	return (publicEvidenceCodec{operator: true}).ValidatePublicPublication(codec.profile.pin, e)
}

func (codec OperatorCodec) PublicationID(e PublicPublicationEntry) (string, error) {
	return (publicEvidenceCodec{operator: true}).PublicPublicationID(codec.profile.pin, e)
}

func (codec OperatorCodec) ParsePublication(raw []byte) (PublicPublicationEntry, error) {
	return (publicEvidenceCodec{operator: true}).ParsePublicPublication(codec.profile.pin, raw)
}

func (codec OperatorCodec) MarshalPublication(e PublicPublicationEntry) ([]byte, error) {
	return (publicEvidenceCodec{operator: true}).MarshalPublicPublication(codec.profile.pin, e)
}

func (codec OperatorCodec) ValidateCutoffIntent(v PublicCutoffIntent) error {
	return (publicEvidenceCodec{operator: true}).ValidatePublicCutoffIntent(codec.profile.pin, v)
}

func (codec OperatorCodec) ValidateBoundary(b PublicSignedBoundary) error {
	return (publicEvidenceCodec{operator: true}).ValidatePublicBoundary(codec.profile.pin, b)
}

func (codec OperatorCodec) BoundaryID(b PublicSignedBoundary) (string, error) {
	return (publicEvidenceCodec{operator: true}).PublicBoundaryID(codec.profile.pin, b)
}

func (codec OperatorCodec) ParseBoundary(raw []byte) (PublicSignedBoundary, error) {
	return (publicEvidenceCodec{operator: true}).ParsePublicBoundary(codec.profile.pin, raw)
}

func (codec OperatorCodec) MarshalBoundary(b PublicSignedBoundary) ([]byte, error) {
	return (publicEvidenceCodec{operator: true}).MarshalPublicBoundary(codec.profile.pin, b)
}

func (codec OperatorCodec) ValidateHead(h PublicOwnershipHead) error {
	return (publicEvidenceCodec{operator: true}).ValidatePublicOwnershipHead(codec.profile.pin, h)
}

func (codec OperatorCodec) ValidateCutoff(c PublicCutoffRecord) error {
	return (publicEvidenceCodec{operator: true}).ValidatePublicCutoff(codec.profile.pin, c)
}

func (codec OperatorCodec) ParseCutoff(raw []byte) (PublicCutoffRecord, error) {
	return (publicEvidenceCodec{operator: true}).ParsePublicCutoff(codec.profile.pin, raw)
}

func (codec OperatorCodec) MarshalCutoff(c PublicCutoffRecord) ([]byte, error) {
	return (publicEvidenceCodec{operator: true}).MarshalPublicCutoff(codec.profile.pin, c)
}

func (codec OperatorCodec) CutoffID(v PublicCutoffRecord) (string, error) {
	return (publicEvidenceCodec{operator: true}).PublicCutoffID(codec.profile.pin, v)
}
