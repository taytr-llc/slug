// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymstable"
)

func FighterEvidenceErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "stale_view", "unauthorized", "forbidden", "busy", "history_not_found", "history_unavailable", "evidence_not_found", "evidence_capacity", "evidence_operation_conflict", "evidence_unavailable", "evidence_profile", "outcome_unknown")
}
func evidenceTerminalError(s string) bool {
	return stableOne(s, "evidence_invalid", "evidence_source_changed", "evidence_unavailable", "evidence_destination_conflict", "evidence_storage_failed", "evidence_export_uncertain")
}
func evidenceRequestValid(r FighterEvidenceRequest) bool {
	if r.Version != 1 || r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	switch r.Command {
	case "fighter_evidence_snapshot":
		return stableOptionalID(r.ViewID) && r.HistoryJobID == "" && r.HistoryID == "" && r.JobID == "" && r.Selection == nil
	case "fighter_evidence_begin":
		return canonicalManifestID(r.ViewID) && canonicalManifestID(r.HistoryJobID) && canonicalManifestID(r.HistoryID) && canonicalManifestID(r.JobID) && r.Selection != nil && recoveryfile.ValidatePin(*r.Selection, recoveryfile.CreateMode) == nil
	case "fighter_evidence_job", "fighter_evidence_cancel":
		return canonicalManifestID(r.ViewID) && canonicalManifestID(r.JobID) && r.HistoryJobID == "" && r.HistoryID == "" && r.Selection == nil
	}
	return false
}
func ValidateFighterEvidenceSnapshot(s FighterEvidenceSnapshot) error {
	if s.Version != 1 || !stableOptionalID(s.StoreID) || !stableOptionalID(s.ActiveJobID) || !stableOptionalID(s.LastJobID) {
		return gymstable.ErrInvalid
	}
	if s.State == "unavailable" {
		if s.Error == "evidence_unavailable" && s.View == nil && s.StoreID == "" && s.ActiveJobID == "" && s.LastJobID == "" {
			return nil
		}
		return gymstable.ErrInvalid
	}
	if s.View == nil || !stableViewValid(*s.View) || !canonicalManifestID(s.StoreID) || s.Error != "" || s.ActiveJobID != "" && s.ActiveJobID == s.LastJobID {
		return gymstable.ErrInvalid
	}
	if (s.State == "idle" && s.ActiveJobID == "" || s.State == "busy" && s.ActiveJobID != "") && historySize(s, 4096) {
		return nil
	}
	return gymstable.ErrInvalid
}
func ValidateFighterEvidenceJob(j FighterEvidenceJob) error {
	if j.Version != 1 || !stableViewValid(j.View) {
		return gymstable.ErrInvalid
	}
	for _, s := range []string{j.JobID, j.StoreID, j.HistoryJobID, j.HistoryID, j.ProspectID} {
		if !canonicalManifestID(s) {
			return gymstable.ErrInvalid
		}
	}
	switch j.State {
	case "running":
		if j.Error != "" || j.Summary != nil {
			return gymstable.ErrInvalid
		}
	case "cancelled":
		if j.Error != "evidence_cancelled" || j.Summary != nil {
			return gymstable.ErrInvalid
		}
	case "failed":
		if !evidenceTerminalError(j.Error) || j.Summary != nil {
			return gymstable.ErrInvalid
		}
	case "done":
		s := j.Summary
		if s == nil || j.Error != "" || s.Version != 1 || !canonicalManifestID(s.FileSHA256) || s.FileBytes == 0 || s.FileBytes > 524288 || s.ProspectID != j.ProspectID || s.HistoryID != j.HistoryID || !canonicalManifestID(s.SourceRecordDigest) || s.EventCount > 256 || !stableTime(s.CapturedAsOfMS) || !historySize(s, 2048) {
			return gymstable.ErrInvalid
		}
	default:
		return gymstable.ErrInvalid
	}
	if !historySize(j, MaxFighterEvidenceJobSize) {
		return gymstable.ErrInvalid
	}
	return nil
}
func evidenceResponseValid(r FighterEvidenceResponse) bool {
	if r.Version != 1 || r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	if !r.OK {
		return r.Job == nil && r.Snapshot == nil && FighterEvidenceErrorCode(r.Error)
	}
	if r.Error != "" {
		return false
	}
	if r.Job != nil {
		return r.Snapshot == nil && ValidateFighterEvidenceJob(*r.Job) == nil
	}
	return r.Snapshot != nil && ValidateFighterEvidenceSnapshot(*r.Snapshot) == nil
}
func ParseFighterEvidenceRequest(raw []byte) (FighterEvidenceRequest, error) {
	var r FighterEvidenceRequest
	if _, e := fighterEvidenceDecode(raw, MaxFighterEvidenceRequestSize-1, 4, &r); e != nil || !evidenceRequestValid(r) {
		return FighterEvidenceRequest{}, gymstable.ErrInvalid
	}
	return r, nil
}
func MarshalFighterEvidenceRequest(r FighterEvidenceRequest) ([]byte, error) {
	if !evidenceRequestValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) >= MaxFighterEvidenceRequestSize {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}
func ParseFighterEvidenceResponse(raw []byte) (FighterEvidenceResponse, error) {
	var r FighterEvidenceResponse
	if _, e := fighterEvidenceDecode(raw, MaxFighterEvidenceResponseSize-1, 6, &r); e != nil || !evidenceResponseValid(r) {
		return FighterEvidenceResponse{}, gymstable.ErrInvalid
	}
	return r, nil
}
func MarshalFighterEvidenceResponse(r FighterEvidenceResponse) ([]byte, error) {
	if !evidenceResponseValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) >= MaxFighterEvidenceResponseSize {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}
