// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import "github.com/taytr-llc/slug/registry"

// IssueResult is owned public evidence from the original selected source. A
// parsed result alone is not issuance, session or payment authority.
type IssueResult struct {
	Version      uint32      `json:"version"`
	State        string      `json:"state"`
	IntentID     string      `json:"intent_id"`
	JobID        string      `json:"job_id"`
	ResolvedAtMS int64       `json:"resolved_at_ms"`
	Reason       string      `json:"reason"`
	Work         *MiningWork `json:"work"`
}

type issueRequest struct {
	Version      uint32               `json:"version"`
	Intent       registry.IssueIntent `json:"intent"`
	IntentID     string               `json:"intent_id"`
	State        string               `json:"state"`
	JobID        string               `json:"job_id"`
	ResolvedAtMS int64                `json:"resolved_at_ms"`
	Reason       string               `json:"reason"`
}

type issueState struct {
	profile                   registry.IssueProfile
	requests, bytes, reserved uint64
}

type issueMetadata struct {
	Application     string                `json:"application"`
	Version         uint32                `json:"version"`
	Base            metadata              `json:"base"`
	IssueProfile    registry.IssueProfile `json:"issue_profile"`
	Requests        uint64                `json:"requests"`
	RequestBytes    uint64                `json:"request_bytes"`
	RequestReserved uint64                `json:"request_reserved"`
}

type issueMarker struct {
	Application  string                `json:"application"`
	Version      uint32                `json:"version"`
	Base         marker                `json:"base"`
	IssueProfile registry.IssueProfile `json:"issue_profile"`
}

const issueApplication = "slug-pool-issue-recovery"
