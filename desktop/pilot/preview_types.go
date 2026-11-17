// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymstable"
)

const MaxPreviewResultSize = 229376
const MaxPreviewPairSize = 24576
const MaxPreviewRecipeSize = 4096
const MaxPreviewEngineSize = 196608
const previewAllowance = 262144
const previewEngineID = "slug-fighter-bout-candidate-v1"

type PreviewControllerAPI interface {
	StableControllerAPI
	PreviewSnapshot() PreviewSnapshot
	BeginFightPreview(context.Context, string, string, string) (PreviewJob, error)
	FightPreviewJob(context.Context, string, string) (PreviewJob, error)
	CancelFightPreview(context.Context, string, string) (PreviewJob, error)
}
type PreviewFighterBinding struct {
	ProspectID         string `json:"prospect_id"`
	SourceRecordDigest string `json:"source_record_digest"`
	ManifestDigest     string `json:"manifest_digest"`
	ConditionDigest    string `json:"condition_digest"`
}
type PreviewRecipe struct {
	Version           uint32                   `json:"version"`
	EngineVersion     uint32                   `json:"engine_version"`
	EngineID          string                   `json:"engine_id"`
	Network           uint32                   `json:"network"`
	Genesis           string                   `json:"genesis"`
	StoreID           string                   `json:"store_id"`
	SourceAuthorityID string                   `json:"source_authority_id"`
	DescriptorID      string                   `json:"descriptor_id"`
	AsOfMS            int64                    `json:"as_of_ms"`
	Rounds            uint32                   `json:"rounds"`
	Training          [2]uint32                `json:"training"`
	Fighters          [2]PreviewFighterBinding `json:"fighters"`
}
type PreviewRequest struct {
	ID         uint64 `json:"id"`
	Version    uint32 `json:"version"`
	Command    string `json:"command"`
	ViewID     string `json:"view_id,omitempty"`
	FighterAID string `json:"fighter_a_id,omitempty"`
	FighterBID string `json:"fighter_b_id,omitempty"`
	JobID      string `json:"job_id,omitempty"`
}
type PreviewResponse struct {
	ID       uint64           `json:"id"`
	Version  uint32           `json:"version"`
	OK       bool             `json:"ok"`
	Snapshot *PreviewSnapshot `json:"snapshot,omitempty"`
	Job      *PreviewJob      `json:"job,omitempty"`
	Error    string           `json:"error,omitempty"`
}
type PreviewSnapshot struct {
	Version       uint32           `json:"version"`
	State         string           `json:"state"`
	Error         string           `json:"error"`
	View          *ManifestViewPin `json:"view"`
	StoreID       string           `json:"store_id"`
	EngineVersion uint32           `json:"engine_version"`
	EngineID      string           `json:"engine_id"`
	ActiveJobID   string           `json:"active_job_id"`
	QueuedJobID   string           `json:"queued_job_id"`
	LastJobID     string           `json:"last_job_id"`
}
type PreviewJob struct {
	Version       uint32          `json:"version"`
	JobID         string          `json:"job_id"`
	State         string          `json:"state"`
	View          ManifestViewPin `json:"view"`
	StoreID       string          `json:"store_id"`
	EngineVersion uint32          `json:"engine_version"`
	EngineID      string          `json:"engine_id"`
	FighterAID    string          `json:"fighter_a_id"`
	FighterBID    string          `json:"fighter_b_id"`
	Error         string          `json:"error"`
	Result        *PreviewResult  `json:"result"`
}
type PreviewResult struct {
	Version            uint32                `json:"version"`
	Pair               gymstable.PreviewPair `json:"pair"`
	Recipe             PreviewRecipe         `json:"recipe"`
	Seed               string                `json:"seed"`
	EngineResult       fighterbout.Result    `json:"engine_result"`
	EngineResultDigest string                `json:"engine_result_digest"`
}
