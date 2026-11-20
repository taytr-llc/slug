// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"encoding/json"
	"math"
	"strconv"
)

// Selection is private: old public APIs always use the fixture profile.
type publicEvidenceCodec struct{ operator bool }

func (codec publicEvidenceCodec) validatePin(p PublicManifestPin) error {
	if codec.operator {
		return validateOperatorPin(p)
	}
	return ValidatePublicManifestPin(p)
}

func (codec publicEvidenceCodec) PublicManifestPinID(p PublicManifestPin) (string, error) {
	if e := codec.validatePin(p); e != nil {
		return "", e
	}
	return PublicOwnershipDigest("SLUG fighter ownership pin", p)
}

func (codec publicEvidenceCodec) ValidatePublicAdmission(p PublicManifestPin, a PublicAdmissionStatement) error {
	id, e := codec.PublicManifestPinID(p)
	if e != nil || a.Version != 1 || a.PinID != id || a.AdmissionSequence == 0 || a.AdmissionSequence > p.Limits.Admissions || uint64(a.AdmissionSequence) != a.Record.Sequence || !publicPrefix(a.AdmissionSequence-1, a.PreviousAdmissionID) || ValidatePublicManifestRecordShape(p.Source, a.Record) != nil {
		return ErrPublicOwnership
	}
	rd, e := PublicOwnershipDigest("SLUG fighter ownership admission record", a.Record)
	if e != nil || rd != a.RecordDigest {
		return ErrPublicOwnership
	}
	return publicSignature("SLUG fighter ownership admission statement", a, p.AdmissionKey, a.Signature)
}

func (codec publicEvidenceCodec) PublicAdmissionID(p PublicManifestPin, a PublicAdmissionStatement) (string, error) {
	if e := codec.ValidatePublicAdmission(p, a); e != nil {
		return "", e
	}
	return PublicOwnershipDigest("SLUG fighter ownership admission id", a)
}

func (codec publicEvidenceCodec) ParsePublicAdmission(p PublicManifestPin, raw []byte) (PublicAdmissionStatement, error) {
	var a PublicAdmissionStatement
	if e := publicDecode(raw, MaxPublicAdmissionSize, &a); e != nil {
		return a, e
	}
	return a, codec.ValidatePublicAdmission(p, a)
}

func (codec publicEvidenceCodec) MarshalPublicAdmission(p PublicManifestPin, a PublicAdmissionStatement) ([]byte, error) {
	if e := codec.ValidatePublicAdmission(p, a); e != nil {
		return nil, e
	}
	raw, e := json.Marshal(a)
	if len(raw) > MaxPublicAdmissionSize {
		return nil, ErrPublicOwnership
	}
	return raw, e
}

func (codec publicEvidenceCodec) ValidatePublicPublication(p PublicManifestPin, e PublicPublicationEntry) error {
	id, err := codec.PublicManifestPinID(p)
	if err != nil || e.Version != 1 || e.PinID != id || e.Sequence == 0 || e.Sequence > p.Limits.Publications || !publicPrefix(e.Sequence-1, e.PreviousID) || e.AttestedAtMS < 0 || e.AttestedAtMS > MaxPublicTimestampMS || validatePublicPayload(p, e) != nil {
		return ErrPublicOwnership
	}
	return publicSignature("SLUG fighter ownership publication statement", e, p.PublicationKey, e.Signature)
}

func (codec publicEvidenceCodec) PublicPublicationID(p PublicManifestPin, e PublicPublicationEntry) (string, error) {
	if err := codec.ValidatePublicPublication(p, e); err != nil {
		return "", err
	}
	return PublicOwnershipDigest("SLUG fighter ownership publication id", e)
}

func (codec publicEvidenceCodec) ParsePublicPublication(p PublicManifestPin, raw []byte) (PublicPublicationEntry, error) {
	var e PublicPublicationEntry
	if err := publicDecode(raw, MaxPublicSmallSize, &e); err != nil {
		return e, err
	}
	return e, codec.ValidatePublicPublication(p, e)
}

func (codec publicEvidenceCodec) MarshalPublicPublication(p PublicManifestPin, e PublicPublicationEntry) ([]byte, error) {
	if err := codec.ValidatePublicPublication(p, e); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(e)
	if len(raw) > MaxPublicSmallSize {
		return nil, ErrPublicOwnership
	}
	return raw, err
}

func (codec publicEvidenceCodec) ValidatePublicCutoffIntent(p PublicManifestPin, v PublicCutoffIntent) error {
	id, e := codec.PublicManifestPinID(p)
	if e != nil || v.Version != 1 || v.PinID != id || !publicHex(v.EventID, 32, true) || v.CutoffMS <= 0 || v.CutoffMS > MaxPublicTimestampMS || v.AcceptedAdmissionSequence > 1000 || v.PublicationSequence > 16384 || !publicPrefix(v.PublicationSequence, v.PublicationHead) || v.FinalitySequence > 4096 || !publicPrefix(v.FinalitySequence, v.FinalityBodyHead) || !publicHex(v.PriorCutoffID, 32, false) {
		return ErrPublicOwnership
	}
	for _, o := range []PublicBoundaryObservation{v.First, v.Second} {
		r, e := strconv.ParseUint(o.Revision, 10, 64)
		if e != nil || r == 0 || strconv.FormatUint(r, 10) != o.Revision || !publicHex(o.InstanceID, 32, true) || !publicHex(o.Hash, 32, false) || o.Height > math.MaxInt32 || o.RequestStartMS < 0 || o.ResponseEndMS < o.RequestStartMS || o.ResponseEndMS > MaxPublicTimestampMS || o.ResponseEndMS-o.RequestStartMS > 5000 {
			return ErrPublicOwnership
		}
	}
	a, b := v.First, v.Second
	if a.InstanceID != b.InstanceID || a.Revision != b.Revision || a.Height != b.Height || a.Hash != b.Hash || a.ResponseEndMS >= v.CutoffMS || b.RequestStartMS < v.CutoffMS || b.ResponseEndMS-a.RequestStartMS > 10000 {
		return ErrPublicOwnership
	}
	return nil
}

func (codec publicEvidenceCodec) ValidatePublicBoundary(p PublicManifestPin, b PublicSignedBoundary) error {
	id, e := codec.PublicManifestPinID(p)
	if e != nil || b.Version != 1 || b.PinID != id || codec.ValidatePublicCutoffIntent(p, b.Intent) != nil {
		return ErrPublicOwnership
	}
	return publicSignature("SLUG fighter ownership boundary statement", b, p.CutoffKey, b.Signature)
}

func (codec publicEvidenceCodec) PublicBoundaryID(p PublicManifestPin, b PublicSignedBoundary) (string, error) {
	if e := codec.ValidatePublicBoundary(p, b); e != nil {
		return "", e
	}
	return PublicOwnershipDigest("SLUG fighter ownership boundary id", b)
}

func (codec publicEvidenceCodec) ParsePublicBoundary(p PublicManifestPin, raw []byte) (PublicSignedBoundary, error) {
	var b PublicSignedBoundary
	if e := publicDecode(raw, MaxPublicSmallSize, &b); e != nil {
		return b, e
	}
	return b, codec.ValidatePublicBoundary(p, b)
}

func (codec publicEvidenceCodec) MarshalPublicBoundary(p PublicManifestPin, b PublicSignedBoundary) ([]byte, error) {
	if e := codec.ValidatePublicBoundary(p, b); e != nil {
		return nil, e
	}
	return json.Marshal(b)
}

func (codec publicEvidenceCodec) ValidatePublicOwnershipHead(p PublicManifestPin, h PublicOwnershipHead) error {
	id, e := codec.PublicManifestPinID(p)
	if e != nil || h.Version != 1 || h.PinID != id || h.AdmissionSequence > 1000 || h.PublicationSequence > 16384 || !publicPrefix(h.AdmissionSequence, h.AdmissionHead) || !publicPrefix(h.PublicationSequence, h.PublicationHead) || h.ChainHeight > math.MaxInt32 || !publicHex(h.ChainHash, 32, true) || !publicHex(h.ReplayDigest, 32, true) || !publicHex(h.FinalizedPrefixID, 32, true) || h.ConflictID != "" && !publicHex(h.ConflictID, 32, true) {
		return ErrPublicOwnership
	}
	return nil
}

func (codec publicEvidenceCodec) ValidatePublicCutoff(p PublicManifestPin, c PublicCutoffRecord) error {
	id, e := codec.PublicManifestPinID(p)
	if e != nil || c.Version != 1 || c.PinID != id || codec.ValidatePublicBoundary(p, c.Boundary) != nil || codec.ValidatePublicOwnershipHead(p, c.Head) != nil || !publicHex(c.OwnershipDigest, 32, true) {
		return ErrPublicOwnership
	}
	v := c.Boundary.Intent
	if c.EventID != v.EventID || c.CutoffMS != v.CutoffMS || c.AdmissionSequence != v.AcceptedAdmissionSequence || c.AdmissionSequence != c.Head.AdmissionSequence || c.AdmissionHead != c.Head.AdmissionHead || c.Head.PublicationSequence != v.PublicationSequence || c.Head.PublicationHead != v.PublicationHead || c.Head.ChainHeight != v.First.Height || c.Head.ChainHash != v.First.Hash {
		return ErrPublicOwnership
	}
	return publicSignature("SLUG fighter ownership cutoff statement", c, p.CutoffKey, c.Signature)
}

func (codec publicEvidenceCodec) ParsePublicCutoff(p PublicManifestPin, raw []byte) (PublicCutoffRecord, error) {
	var c PublicCutoffRecord
	if e := publicDecode(raw, MaxPublicSmallSize, &c); e != nil {
		return c, e
	}
	return c, codec.ValidatePublicCutoff(p, c)
}

func (codec publicEvidenceCodec) MarshalPublicCutoff(p PublicManifestPin, c PublicCutoffRecord) ([]byte, error) {
	if e := codec.ValidatePublicCutoff(p, c); e != nil {
		return nil, e
	}
	raw, e := json.Marshal(c)
	if len(raw) > MaxPublicSmallSize {
		return nil, ErrPublicOwnership
	}
	return raw, e
}

func (codec publicEvidenceCodec) PublicCutoffID(p PublicManifestPin, v PublicCutoffRecord) (string, error) {
	if e := codec.ValidatePublicCutoff(p, v); e != nil {
		return "", e
	}
	return PublicOwnershipDigest("SLUG fighter ownership cutoff id", v)
}
