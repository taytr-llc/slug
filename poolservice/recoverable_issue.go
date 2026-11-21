// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/registry"
	"time"
)

// ValidateRecoverableIssueResponse validates public content, not authentication
// or durable issuance. It never manufactures original-client provenance.
func ValidateRecoverableIssueResponse(s RecoverableDailySelection, requestID string, r IssueResponse, lookup bool) error {
	if !issueResponseBounds(r) || validateRecoverableSelection(s) != nil || !hex64(requestID, true) {
		return errConfig
	}
	profile, e := registry.IssueProfileID(s.IssueProfile)
	if e != nil {
		return errConfig
	}
	gym, e := gymauth.GymID(identity(s.Source), s.PublicKey)
	if e != nil {
		return errConfig
	}
	if r.Version != Version3 || r.BindingID != s.IssueProfile.SourceConfigID || r.SettlementConfigID != s.SettlementConfigID || r.RecipientScript != s.RecipientScript || r.IssueProfileID != profile || r.RequestID != requestID || r.GymID != gym || r.PublicKey != s.PublicKey {
		return errConfig
	}
	v := r.Result
	if v.Version != 1 || v.ResolvedAtMS < 0 || v.ResolvedAtMS > 253402300799999 {
		return errConfig
	}
	switch v.State {
	case "absent":
		if !lookup || v.IntentID != "" || v.JobID != "" || v.ResolvedAtMS != 0 || v.Reason != "" || v.Work != nil {
			return errConfig
		}
	case "pending":
		if !lookup || !hex64(v.IntentID, true) || v.JobID != "" || v.ResolvedAtMS != 0 || v.Reason != "" || v.Work != nil {
			return errConfig
		}
	case "work":
		if v.Work == nil || !hex64(v.IntentID, true) || !hex64(v.JobID, true) || v.JobID != v.Work.Job.ID || v.Reason != "" || v.ResolvedAtMS < v.Work.Job.IssuedAtMS || v.ResolvedAtMS >= v.Work.Job.ExpiresAtMS {
			return errConfig
		}
		if _, e = PrepareDailyWork(s.Source, s.PublicKey, *v.Work); e != nil {
			return errConfig
		}
		raw, e := json.Marshal(v.Work.Template)
		if e != nil {
			return errConfig
		}
		sum := sha256.Sum256(raw)
		intent, e := registry.IssueIntentID(registry.IssueIntent{Version: 1, ProfileID: profile, RequestID: requestID, GymID: gym, PublicKey: s.PublicKey, TemplateSHA256: hex.EncodeToString(sum[:])})
		if e != nil || v.IntentID != intent {
			return errConfig
		}
	case "issued_unused":
		if !hex64(v.IntentID, true) || !hex64(v.JobID, true) || v.ResolvedAtMS == 0 || v.Reason != "expired_before_work" || v.Work != nil {
			return errConfig
		}
	case "not_issued":
		if !hex64(v.IntentID, true) || v.JobID != "" || v.ResolvedAtMS == 0 || (v.Reason != "parent_changed" && v.Reason != "intake_drained") || v.Work != nil {
			return errConfig
		}
	default:
		return errConfig
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > MaxWorkResponseSize {
		return errConfig
	}
	return nil
}
func (c *Client) issueResponse(ctx context.Context, requestID string, lookup bool) (IssueResponse, error) {
	if c == nil || c.recoverable == nil || !hex64(requestID, true) {
		return IssueResponse{}, &ClientError{Code: "invalid_request"}
	}
	if e := c.ready(ctx); e != nil {
		return IssueResponse{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	path := "issue"
	if lookup {
		path = "issue-lookup"
	}
	raw, e := c.post(ctx, path, issueRequest{requestID}, !lookup, MaxWorkResponseSize)
	if e != nil {
		return IssueResponse{}, e
	}
	var result IssueResponse
	if decodeRecoverableIssue(raw, &result) != nil || ValidateRecoverableIssueResponse(c.recoverable.selection, requestID, result, lookup) != nil || ctx.Err() != nil {
		return IssueResponse{}, failure(!lookup)
	}
	canonical, e := json.Marshal(result)
	if e != nil {
		return IssueResponse{}, failure(!lookup)
	}
	result.provenance = &issueResponseProvenance{c, requestID, issueResponseDigest(canonical), len(canonical)}
	return result, nil
}
func (c *Client) ResumeIssue(ctx context.Context, requestID string) (IssueResponse, error) {
	return c.issueResponse(ctx, requestID, false)
}
func (c *Client) LookupIssue(ctx context.Context, requestID string) (IssueResponse, error) {
	return c.issueResponse(ctx, requestID, true)
}
func (c *Client) CheckIssueResponse(requestID string, r IssueResponse) error {
	p := r.provenance
	if !issueResponseBounds(r) || c == nil || c.recoverable == nil || p == nil || p.client != c || p.request != requestID || r.RequestID != requestID {
		return errConfig
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) != p.size || len(raw) > MaxWorkResponseSize || issueResponseDigest(raw) != p.digest {
		return errConfig
	}
	return nil
}

// Bound caller-owned mutations before any complete canonical encoding. Content
// checks above and the immutable receipt digest remain independently required.
func issueResponseBounds(r IssueResponse) bool {
	for _, s := range []string{r.BindingID, r.SettlementConfigID, r.IssueProfileID, r.RequestID, r.GymID, r.Result.IntentID, r.Result.JobID} {
		if len(s) > 64 {
			return false
		}
	}
	if len(r.PublicKey) > 66 || len(r.RecipientScript) > 44 || len(r.Result.State) > 16 || len(r.Result.Reason) > 32 {
		return false
	}
	if w := r.Result.Work; w != nil {
		if prospectjob.ValidateManifest(w.Job) != nil || w.Template.Context != w.Job.Context || !workBounds(*w) {
			return false
		}
	}
	return true
}

func issueResponseDigest(raw []byte) [32]byte {
	h := sha256.New()
	h.Write([]byte("SLUG pool issue recovery client response\x00v1\x00"))
	h.Write(raw)
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}
