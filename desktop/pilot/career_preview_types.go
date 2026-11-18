// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

const MaxCareerPreviewResultSize = 229376
const MaxCareerPreviewPairSize = 24576
const MaxCareerPreviewRecipeSize = 4096
const MaxCareerPreviewEngineSize = 196608
const careerPreviewAllowance = 262144
const careerPreviewEngineID = "slug-fighter-bout-candidate-v1"

type CareerPreviewControllerAPI interface {
	CareerControllerAPI
	CareerPreviewSnapshot() CareerPreviewSnapshot
	BeginCareerFightPreview(context.Context, string, string, string) (CareerPreviewJob, error)
	CareerFightPreviewJob(context.Context, string, string) (CareerPreviewJob, error)
	CancelCareerFightPreview(context.Context, string, string) (CareerPreviewJob, error)
}
type CareerPreviewFighterBinding struct {
	ProspectID         string               `json:"prospect_id"`
	SourceRecordDigest string               `json:"source_record_digest"`
	ManifestDigest     string               `json:"manifest_digest"`
	ConditionDigest    string               `json:"condition_digest"`
	Totals             fightercareer.Totals `json:"totals"`
}
type CareerPreviewRecipe struct {
	Version           uint32                         `json:"version"`
	EngineVersion     uint32                         `json:"engine_version"`
	EngineID          string                         `json:"engine_id"`
	Network           uint32                         `json:"network"`
	Genesis           string                         `json:"genesis"`
	StoreID           string                         `json:"store_id"`
	SourceAuthorityID string                         `json:"source_authority_id"`
	DescriptorID      string                         `json:"descriptor_id"`
	ActivationID      string                         `json:"activation_id"`
	CareerDigest      string                         `json:"career_digest"`
	AsOfMS            int64                          `json:"as_of_ms"`
	Rounds            uint32                         `json:"rounds"`
	Training          [2]uint32                      `json:"training"`
	Fighters          [2]CareerPreviewFighterBinding `json:"fighters"`
}
type CareerPreviewRequest struct {
	ID         uint64 `json:"id"`
	Version    uint32 `json:"version"`
	Command    string `json:"command"`
	ViewID     string `json:"view_id,omitempty"`
	FighterAID string `json:"fighter_a_id,omitempty"`
	FighterBID string `json:"fighter_b_id,omitempty"`
	JobID      string `json:"job_id,omitempty"`
}
type CareerPreviewResponse struct {
	ID       uint64                 `json:"id"`
	Version  uint32                 `json:"version"`
	OK       bool                   `json:"ok"`
	Snapshot *CareerPreviewSnapshot `json:"snapshot,omitempty"`
	Job      *CareerPreviewJob      `json:"job,omitempty"`
	Error    string                 `json:"error,omitempty"`
}
type CareerPreviewSnapshot struct {
	Version       uint32                  `json:"version"`
	State         string                  `json:"state"`
	Error         string                  `json:"error"`
	View          *ManifestViewPin        `json:"view"`
	StoreID       string                  `json:"store_id"`
	EngineVersion uint32                  `json:"engine_version"`
	EngineID      string                  `json:"engine_id"`
	Activation    *careerstate.Activation `json:"activation"`
	ActiveJobID   string                  `json:"active_job_id"`
	QueuedJobID   string                  `json:"queued_job_id"`
	LastJobID     string                  `json:"last_job_id"`
}
type CareerPreviewJob struct {
	Version       uint32               `json:"version"`
	JobID         string               `json:"job_id"`
	State         string               `json:"state"`
	View          ManifestViewPin      `json:"view"`
	StoreID       string               `json:"store_id"`
	EngineVersion uint32               `json:"engine_version"`
	EngineID      string               `json:"engine_id"`
	ActivationID  string               `json:"activation_id"`
	FighterAID    string               `json:"fighter_a_id"`
	FighterBID    string               `json:"fighter_b_id"`
	Error         string               `json:"error"`
	Result        *CareerPreviewResult `json:"result"`
}
type CareerPreviewResult struct {
	Version            uint32                      `json:"version"`
	Pair               gymstable.CareerPreviewPair `json:"pair"`
	Recipe             CareerPreviewRecipe         `json:"recipe"`
	Seed               string                      `json:"seed"`
	EngineResult       fighterbout.Result          `json:"engine_result"`
	EngineResultDigest string                      `json:"engine_result_digest"`
}
