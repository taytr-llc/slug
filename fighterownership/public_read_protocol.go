// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/registry"
)

const publicReadPath = "/slug/ownership/v1"

// PublicReadStatus is a selected source's declared frontier, not verified
// ownership or proof that no other public history exists.
type PublicReadStatus struct {
	Version                   uint32                     `json:"version"`
	PinID                     string                     `json:"pin_id"`
	Publication               registry.PublicationStatus `json:"publication"`
	FinalitySequence          uint32                     `json:"finality_sequence"`
	PublishedFinalitySequence uint32                     `json:"published_finality_sequence"`
	FinalityBodyHead          string                     `json:"finality_body_head"`
	FinalityHead              string                     `json:"finality_head"`
}

type publicReadRequest struct {
	Version  uint32 `json:"version"`
	PinID    string `json:"pin_id"`
	ID       uint64 `json:"id"`
	Method   string `json:"method"`
	Sequence uint32 `json:"sequence"`
	EventID  string `json:"event_id"`
}
type publicReadResponse struct {
	Version uint32          `json:"version"`
	PinID   string          `json:"pin_id"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result"`
	Error   string          `json:"error"`
}

func publicReadLimit(method string) int {
	switch method {
	case "status":
		return 2048
	case "admission":
		return 69632
	case "publication", "finality_frame", "cutoff":
		return 8192
	case "cutoff_intent":
		return 4096
	default:
		return 0
	}
}
func (r publicReadRequest) validate(pinID string) error {
	return r.validateSelected(pinID, 1)
}
func (r publicReadRequest) validateSelected(pinID string, version uint32) error {
	if r.Version != version || r.PinID != pinID || r.ID == 0 || publicReadLimit(r.Method) == 0 {
		return ErrInvalid
	}
	switch r.Method {
	case "status":
		if r.Sequence != 0 || r.EventID != "" {
			return ErrInvalid
		}
	case "admission", "publication", "finality_frame":
		limit := uint32(16384)
		if r.Method == "admission" {
			limit = 1000
		}
		if r.Method == "finality_frame" {
			limit = 4096
		}
		if r.Sequence == 0 || r.Sequence > limit || r.EventID != "" {
			return ErrInvalid
		}
	case "cutoff_intent", "cutoff":
		if r.Sequence != 0 || !hexValue(r.EventID, 32, true) {
			return ErrInvalid
		}
	}
	return nil
}
func validatePublicReadStatus(pin Pin, v PublicReadStatus) error {
	return (ownershipCodec{}).validatePublicReadStatus(pin, v)
}
func (codec ownershipCodec) validatePublicReadStatus(pin Pin, v PublicReadStatus) error {
	id, e := codec.ID(pin)
	p := v.Publication
	if e != nil || v.Version != codec.wireVersion() || v.PinID != id || p.Version != 1 || p.PinID != id ||
		p.AcceptedSequence > pin.Limits.Admissions || p.SignedAdmissionSequence > p.AcceptedSequence ||
		p.PublicationSequence > pin.Limits.Publications || p.SignedAdmissionSequence > p.PublicationSequence ||
		!validPrefix(p.SignedAdmissionSequence, p.AdmissionHead) || !validPrefix(p.PublicationSequence, p.PublicationHead) ||
		p.PublicEvidencePending != (p.AcceptedSequence != p.SignedAdmissionSequence) ||
		v.FinalitySequence > 4096 || v.PublishedFinalitySequence > v.FinalitySequence ||
		!validPrefix(v.FinalitySequence, v.FinalityBodyHead) || !validPrefix(v.PublishedFinalitySequence, v.FinalityHead) {
		return ErrInvalid
	}
	return nil
}
func publicReadError(e error) string {
	switch {
	case e == nil:
		return ""
	case errors.Is(e, ErrBusy), errors.Is(e, registry.ErrBusy):
		return "busy"
	case errors.Is(e, ErrClosed), errors.Is(e, registry.ErrClosed):
		return "closed"
	case errors.Is(e, registry.ErrNotFound):
		return "not_found"
	case errors.Is(e, ErrInvalid):
		return "invalid"
	default:
		return "unavailable"
	}
}
func parsePublicReadError(code string) error {
	switch code {
	case "busy":
		return ErrBusy
	case "closed":
		return ErrClosed
	case "not_found":
		return registry.ErrNotFound
	case "invalid":
		return ErrInvalid
	default:
		return ErrUnavailable
	}
}
