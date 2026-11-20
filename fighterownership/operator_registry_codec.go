// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import "github.com/taytr-llc/slug/registry"

func (codec ownershipCodec) PublicAdmissionID(p Pin, a AdmissionStatement) (string, error) {
	if !codec.operator {
		return registry.PublicAdmissionID(p, a)
	}
	if _, e := codec.ID(p); e != nil {
		return "", e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return "", e
	}
	return c.AdmissionID(a)
}

func (codec ownershipCodec) PublicPublicationID(p Pin, v PublicationEntry) (string, error) {
	if !codec.operator {
		return registry.PublicPublicationID(p, v)
	}
	if _, e := codec.ID(p); e != nil {
		return "", e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return "", e
	}
	return c.PublicationID(v)
}

func (codec ownershipCodec) PublicBoundaryID(p Pin, v SignedBoundary) (string, error) {
	if !codec.operator {
		return registry.PublicBoundaryID(p, v)
	}
	if _, e := codec.ID(p); e != nil {
		return "", e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return "", e
	}
	return c.BoundaryID(v)
}

func (codec ownershipCodec) PublicCutoffID(p Pin, v CutoffRecord) (string, error) {
	if !codec.operator {
		return registry.PublicCutoffID(p, v)
	}
	if _, e := codec.ID(p); e != nil {
		return "", e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return "", e
	}
	return c.CutoffID(v)
}

func (codec ownershipCodec) ValidatePublicAdmission(p Pin, v AdmissionStatement) error {
	if !codec.operator {
		return registry.ValidatePublicAdmission(p, v)
	}
	if _, e := codec.ID(p); e != nil {
		return e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return e
	}
	return c.ValidateAdmission(v)
}

func (codec ownershipCodec) ValidatePublicPublication(p Pin, v PublicationEntry) error {
	if !codec.operator {
		return registry.ValidatePublicPublication(p, v)
	}
	if _, e := codec.ID(p); e != nil {
		return e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return e
	}
	return c.ValidatePublication(v)
}

func (codec ownershipCodec) ValidatePublicBoundary(p Pin, v SignedBoundary) error {
	if !codec.operator {
		return registry.ValidatePublicBoundary(p, v)
	}
	if _, e := codec.ID(p); e != nil {
		return e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return e
	}
	return c.ValidateBoundary(v)
}

func (codec ownershipCodec) ValidatePublicCutoffIntent(p Pin, v CutoffIntent) error {
	if !codec.operator {
		return registry.ValidatePublicCutoffIntent(p, v)
	}
	if _, e := codec.ID(p); e != nil {
		return e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return e
	}
	return c.ValidateCutoffIntent(v)
}

func (codec ownershipCodec) ValidatePublicCutoff(p Pin, v CutoffRecord) error {
	if !codec.operator {
		return registry.ValidatePublicCutoff(p, v)
	}
	if _, e := codec.ID(p); e != nil {
		return e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return e
	}
	return c.ValidateCutoff(v)
}

func (codec ownershipCodec) ParsePublicAdmission(p Pin, raw []byte) (AdmissionStatement, error) {
	if !codec.operator {
		return registry.ParsePublicAdmission(p, raw)
	}
	if _, e := codec.ID(p); e != nil {
		return AdmissionStatement{}, e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return AdmissionStatement{}, e
	}
	return c.ParseAdmission(raw)
}

func (codec ownershipCodec) ParsePublicPublication(p Pin, raw []byte) (PublicationEntry, error) {
	if !codec.operator {
		return registry.ParsePublicPublication(p, raw)
	}
	if _, e := codec.ID(p); e != nil {
		return PublicationEntry{}, e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return PublicationEntry{}, e
	}
	return c.ParsePublication(raw)
}

func (codec ownershipCodec) ParsePublicBoundary(p Pin, raw []byte) (SignedBoundary, error) {
	if !codec.operator {
		return registry.ParsePublicBoundary(p, raw)
	}
	if _, e := codec.ID(p); e != nil {
		return SignedBoundary{}, e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return SignedBoundary{}, e
	}
	return c.ParseBoundary(raw)
}

func (codec ownershipCodec) ParsePublicCutoff(p Pin, raw []byte) (CutoffRecord, error) {
	if !codec.operator {
		return registry.ParsePublicCutoff(p, raw)
	}
	if _, e := codec.ID(p); e != nil {
		return CutoffRecord{}, e
	}
	c, e := codec.profile.Codec()
	if e != nil {
		return CutoffRecord{}, e
	}
	return c.ParseCutoff(raw)
}

func (codec ownershipCodec) wireVersion() uint32 {
	if codec.operator {
		return 2
	}
	return 1
}
func (codec ownershipCodec) readPath() string {
	if codec.operator {
		return "/slug/ownership/v2"
	}
	return publicReadPath
}
func (codec ownershipCodec) markerName() string {
	if codec.operator {
		return "SLUG-FIGHTER-OWNERSHIP-V2.json"
	}
	return markerName
}
