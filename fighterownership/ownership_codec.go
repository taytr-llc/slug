// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"encoding/json"
	"github.com/taytr-llc/slug/registry"
	"math"
	"sort"
	"strings"
)

// Zero selects the unchanged fixture contract. Operator selection is retained
// explicitly by constructors; it is never inferred from untrusted Pin.Version.
type ownershipCodec struct {
	operator bool
	profile  registry.OperatorProfile
}

func (codec ownershipCodec) ID(p Pin) (string, error) {
	if !codec.operator {
		return ID(p)
	}
	if codec.profile.Validate() != nil || codec.profile.Pin() != p {
		return "", ErrSource
	}
	return codec.profile.ID(), nil
}
func (codec ownershipCodec) admissionID(p Pin, a AdmissionStatement) (string, error) {
	if !codec.operator {
		return registry.PublicAdmissionID(p, a)
	}
	if _, err := codec.ID(p); err != nil {
		return "", err
	}
	c, err := codec.profile.Codec()
	if err != nil {
		return "", ErrSource
	}
	return c.AdmissionID(a)
}
func (codec ownershipCodec) ValidateEffectiveState(p Pin, v EffectiveState) error {
	id, e := codec.ID(p)
	if e != nil || v.Version != 1 || v.PinID != id || !hexValue(v.ProspectID, 32, true) || !hexValue(v.StateID, 32, true) || !hexValue(v.BirthAdmissionID, 32, true) || !gymKey(v.OwnerPublicKey) || v.Generation > 4096 {
		return ErrInvalid
	}
	var stateID string
	if v.Generation == 0 {
		var evidence InitialStateEvidence
		if decode(v.Evidence, 4096, &evidence) != nil || evidence.Version != 1 || evidence.PinID != id || evidence.ProspectID != v.ProspectID || evidence.AdmissionID != v.BirthAdmissionID || evidence.OwnerPublicKey != v.OwnerPublicKey {
			return ErrInvalid
		}
		stateID, e = objectDigest("SLUG fighter ownership state", evidence)
	} else {
		var evidence FinalStateEvidence
		if decode(v.Evidence, 4096, &evidence) != nil || evidence.Version != 1 || evidence.PinID != id || evidence.ProspectID != v.ProspectID || evidence.OwnerPublicKey != v.OwnerPublicKey || !hexValue(evidence.PriorStateID, 32, true) || !hexValue(evidence.TransferID, 32, true) || !hexValue(evidence.CheckpointPublicationID, 32, true) || !hexValue(evidence.SixtiethDescendantHash, 32, true) || evidence.SixtiethDescendantHeight < 60 || evidence.SixtiethDescendantHeight > math.MaxInt32 {
			return ErrInvalid
		}
		stateID, e = objectDigest("SLUG fighter ownership state", evidence)
	}
	if e != nil || stateID != v.StateID {
		return ErrInvalid
	}
	return nil
}

func (codec ownershipCodec) ParseEffectiveState(p Pin, raw []byte) (EffectiveState, error) {
	var v EffectiveState
	if e := decode(raw, 4096, &v); e != nil {
		return v, e
	}
	return v, codec.ValidateEffectiveState(p, v)
}

func (codec ownershipCodec) MarshalEffectiveState(p Pin, v EffectiveState) ([]byte, error) {
	if e := codec.ValidateEffectiveState(p, v); e != nil {
		return nil, e
	}
	return json.Marshal(v)
}

func (codec ownershipCodec) InitialState(p Pin, a AdmissionStatement) (EffectiveState, error) {
	aid, e := codec.admissionID(p, a)
	if e != nil {
		return EffectiveState{}, ErrInvalid
	}
	pid, _ := codec.ID(p)
	ev := InitialStateEvidence{1, pid, a.Record.Verified.ProspectID, aid, a.Record.Authorization.PublicKey}
	sid, e := objectDigest("SLUG fighter ownership state", ev)
	if e != nil {
		return EffectiveState{}, e
	}
	raw, _ := json.Marshal(ev)
	v := EffectiveState{1, pid, ev.ProspectID, sid, ev.OwnerPublicKey, aid, 0, raw}
	return v, codec.ValidateEffectiveState(p, v)
}

func (codec ownershipCodec) ValidateOwnershipMap(p Pin, m OwnershipMap) error {
	id, e := codec.ID(p)
	if e != nil || m.Version != 1 || m.PinID != id || m.Items == nil || len(m.Items) > 1000 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	var previous uint32
	for _, x := range m.Items {
		if x.AdmissionSequence != previous+1 || !hexValue(x.ProspectID, 32, true) || !hexValue(x.StateID, 32, true) || seen[x.ProspectID] {
			return ErrInvalid
		}
		previous = x.AdmissionSequence
		seen[x.ProspectID] = true
	}
	return nil
}

func (codec ownershipCodec) OwnershipMapID(p Pin, m OwnershipMap) (string, error) {
	if e := codec.ValidateOwnershipMap(p, m); e != nil {
		return "", e
	}
	return objectDigest("SLUG fighter ownership map", m)
}

func (codec ownershipCodec) FinalizedPrefixID(p Pin, m OwnershipMap) (string, error) {
	if e := codec.ValidateOwnershipMap(p, m); e != nil {
		return "", e
	}
	return objectDigest("SLUG fighter ownership finalized prefix", m)
}

func (codec ownershipCodec) ParseOwnershipMap(p Pin, raw []byte) (OwnershipMap, error) {
	var m OwnershipMap
	if e := decode(raw, 262144, &m); e != nil {
		return m, e
	}
	return m, codec.ValidateOwnershipMap(p, m)
}

func (codec ownershipCodec) ValidateHead(p Pin, h HeadRef) error {
	id, e := codec.ID(p)
	if e != nil || h.Version != 1 || h.PinID != id || h.AdmissionSequence > 1000 || h.PublicationSequence > 16384 || !validPrefix(h.AdmissionSequence, h.AdmissionHead) || !validPrefix(h.PublicationSequence, h.PublicationHead) || h.ChainHeight > math.MaxInt32 || !hexValue(h.ChainHash, 32, true) || !hexValue(h.ReplayDigest, 32, true) || !hexValue(h.FinalizedPrefixID, 32, true) || h.ConflictID != "" && !hexValue(h.ConflictID, 32, true) {
		return ErrInvalid
	}
	return nil
}

func (codec ownershipCodec) HeadID(p Pin, h HeadRef) (string, error) {
	if e := codec.ValidateHead(p, h); e != nil {
		return "", e
	}
	return objectDigest("SLUG fighter ownership head", h)
}

func (codec ownershipCodec) validateWitness(p Pin, v CarrierWitness) error {
	id, e := codec.ID(p)
	if e != nil || v.Version != 1 || v.PinID != id || !hexValue(v.TransferID, 32, true) || !hexValue(v.PriorStateID, 32, true) || !hexValue(v.Envelope, 202, true) || (v.Kind != "reveal" && v.Kind != "refund") || !validTransactionWitness(v.Commit, 4096) || !validTransactionWitness(v.Spend, 12000) || len(v.FundingPrevouts) < 1 || len(v.FundingPrevouts) > 16 {
		return ErrInvalid
	}
	prev := ""
	for _, f := range v.FundingPrevouts {
		if !hexValue(f.TxID, 32, true) || f.Atoms < 1 || f.Atoms > 21000000*100000000 || !hexValue(f.Script, 22, true) || !strings.HasPrefix(f.Script, "0014") {
			return ErrInvalid
		}
		k := fundingOrder(f.TxID, f.Index)
		if k <= prev {
			return ErrInvalid
		}
		prev = k
	}
	return nil
}

func (codec ownershipCodec) validateFrameBody(p Pin, v FinalityFrameBody) error {
	id, e := codec.ID(p)
	if e != nil || v.Version != 1 || v.PinID != id || v.Sequence < 1 || v.Sequence > 4096 || !validPrefix(v.Sequence-1, v.PreviousBodyID) || v.AdmissionSequence > 1000 || v.PublicationSequence > 16384 || !validPrefix(v.AdmissionSequence, v.AdmissionHead) || !validPrefix(v.PublicationSequence, v.PublicationHead) || v.ChainHeight > math.MaxInt32 || !hexValue(v.ChainHash, 32, true) || !hexValue(v.OwnershipMapID, 32, true) || !hexValue(v.FinalizedPrefixID, 32, true) {
		return ErrInvalid
	}
	return nil
}

func (codec ownershipCodec) ValidateFinalityFrame(p Pin, v FinalityFrame) error {
	if e := codec.validateFrameBody(p, v.FinalityFrameBody); e != nil {
		return e
	}
	return verifyObject("SLUG fighter ownership finality statement", p.PublicationKey, v, v.Signature)
}

func (codec ownershipCodec) FinalityFrameID(p Pin, v FinalityFrame) (string, error) {
	if e := codec.ValidateFinalityFrame(p, v); e != nil {
		return "", e
	}
	return objectDigest("SLUG fighter ownership finality id", v)
}

func (codec ownershipCodec) validateFinalityWitness(p Pin, v FinalityWitness) error {
	id, e := codec.ID(p)
	if e != nil || v.Version != 1 || v.PinID != id || !hexValue(v.StateID, 32, true) || !validFrontier(v.Frontier) || !hexValue(v.CheckpointPublicationID, 32, true) || v.SixtiethDescendantHeight < 60 || v.SixtiethDescendantHeight > math.MaxInt32 || !hexValue(v.SixtiethDescendantHash, 32, true) {
		return ErrInvalid
	}
	return nil
}

func (codec ownershipCodec) validateConflict(p Pin, v ConflictRecord) error {
	id, e := codec.ID(p)
	if e != nil || v.Version != 1 || v.PinID != id || (v.Kind != "publication_equivocation" && v.Kind != "checkpoint_conflict" && v.Kind != "finalized_branch_conflict") || len(v.EvidenceIDs) < 2 || len(v.EvidenceIDs) > 16 || !sort.StringsAreSorted(v.EvidenceIDs) {
		return ErrInvalid
	}
	if v.Kind != "publication_equivocation" && (!hexValue(v.ProspectID, 32, true) || !hexValue(v.RetainedStateID, 32, true)) {
		return ErrInvalid
	}
	if (v.ProspectID != "" && !hexValue(v.ProspectID, 32, true)) || (v.RetainedStateID != "" && !hexValue(v.RetainedStateID, 32, true)) {
		return ErrInvalid
	}
	for _, f := range []*FrontierEvidence{v.RetainedFrontier, v.ContradictoryFrontier} {
		if f != nil && !validFrontier(*f) {
			return ErrInvalid
		}
	}
	for n, x := range v.EvidenceIDs {
		if !hexValue(x, 32, true) || n > 0 && v.EvidenceIDs[n-1] == x {
			return ErrInvalid
		}
	}
	return nil
}
