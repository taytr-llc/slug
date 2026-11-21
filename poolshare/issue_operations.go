// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

func (p *Pool) issueResult(ctx context.Context, r issueRequest) (IssueResult, error) {
	out := IssueResult{Version: registry.IssueVersion1, State: r.State, IntentID: r.IntentID, JobID: r.JobID, ResolvedAtMS: r.ResolvedAtMS, Reason: r.Reason}
	if r.State == "work" {
		w, _, err := p.loadWorkContext(ctx, r.JobID)
		if err != nil {
			return IssueResult{}, err
		}
		out.Work = &MiningWork{w.Job, w.Template}
	}
	return out, nil
}
func (p *Pool) LookupIssue(ctx context.Context, token, requestID string) (IssueResult, error) {
	var out IssueResult
	err := p.withSession(ctx, token, func(ctx context.Context, key string, s *poolSession) error {
		if p.issueRecovery == nil {
			return ErrConfig
		}
		if !canonicalHash(requestID, true) {
			return ErrInvalid
		}
		if err := p.auditIssueLive(ctx); err != nil {
			return err
		}
		r, err := p.loadIssueRow(requestID)
		if err == ErrNotFound {
			out = IssueResult{Version: registry.IssueVersion1, State: "absent"}
			return nil
		}
		if err != nil {
			return err
		}
		if r.Intent.PublicKey != key || r.Intent.GymID != s.gym {
			return ErrUnauthorized
		}
		out, err = p.issueResult(ctx, r)
		return err
	})
	if err != nil {
		return IssueResult{}, err
	}
	return out, nil
}

// ResumeIssue only advances the exact retained request. An uncertain return
// never selects a second request, job, template, session, or authority owner.
func (p *Pool) ResumeIssue(ctx context.Context, token, requestID string) (IssueResult, error) {
	var out IssueResult
	err := p.withSession(ctx, token, func(ctx context.Context, key string, s *poolSession) error {
		if p.issueRecovery == nil {
			return ErrConfig
		}
		if !canonicalHash(requestID, true) {
			return ErrInvalid
		}
		if err := p.auditIssueLive(ctx); err != nil {
			return err
		}
		r, err := p.loadIssueRow(requestID)
		if err != nil && err != ErrNotFound {
			return err
		}
		if err == nil && (r.Intent.PublicKey != key || r.Intent.GymID != s.gym) {
			return ErrUnauthorized
		}
		if err == nil && r.State != "pending" {
			out, err = p.issueResult(ctx, r)
			return err
		}
		if err := p.dailyRequestMember(s.gym); err != nil {
			return err
		}
		if err == ErrNotFound {
			if err := p.operatorIntake(); err != nil {
				return err
			}
			if p.issueRecovery.requests >= p.issueRecovery.profile.MaxRequests || p.meta.Submissions >= p.config.MaxSubmissions {
				return ErrCapacity
			}
			t, tr, err := p.resolveIssueTemplate(ctx)
			if err != nil {
				return err
			}
			pid, _ := registry.IssueProfileID(p.issueRecovery.profile)
			i := registry.IssueIntent{Version: registry.IssueVersion1, ProfileID: pid, RequestID: requestID, GymID: s.gym, PublicKey: key, TemplateSHA256: issueTemplateID(tr)}
			iid, err := registry.IssueIntentID(i)
			if err != nil {
				return ErrConfig
			}
			r = issueRequest{Version: registry.IssueVersion1, Intent: i, IntentID: iid, State: "pending"}
			raw, _ := json.Marshal(r)
			if len(raw) > registry.MaxIssueRowSize {
				return ErrCapacity
			}
			reserve := issuePendingReserve(len(raw), len(tr))
			growth := uint64(40+len(raw)+40+len(tr)) + reserve
			if growth > p.freeSourceBytes() || maxSubmissionSize > p.freeSourceBytes()-growth {
				return ErrCapacity
			}
			if err := p.dailyWorkHeadroom(growth); err != nil {
				return err
			}
			if err := p.node.RecheckContext(ctx, t.Context); err != nil {
				return ErrNode
			}
			err = s.do(func(check func() error) error {
				ms, err := p.observeClock(check)
				if err != nil {
					return err
				}
				next := p.meta
				next.LastClockMS = ms
				selected := *p.issueRecovery
				selected.requests++
				selected.bytes += uint64(40 + len(raw) + 40 + len(tr))
				selected.reserved += reserve
				b := new(leveldb.Batch)
				b.Put(issueKey("issue/r/", requestID), raw)
				b.Put(issueKey("issue/t/", requestID), tr)
				if err = check(); err != nil {
					return err
				}
				if err = p.commitIssue(b, next, &selected); err != nil {
					return err
				}
				s.effects = true
				p.mark("issue-pending")
				return nil
			})
			if err != nil {
				return err
			}
		}
		t, _, err := p.issueTemplate(r)
		if err != nil {
			return err
		}
		issuer, err := p.authority.TryIssueRequest(ctx, requestID)
		var job prospectjob.ManifestJob
		if err == registry.ErrIssueRequestNotFound {
			reason, err := p.unissuedReason(ctx, t)
			if err != nil {
				return err
			}
			if reason != "" {
				err = s.do(func(check func() error) error {
					var e error
					issuer, job, e = p.authority.SealIssueRequest(ctx, r.Intent, reason, check)
					if e == nil {
						s.effects = true
					}
					return p.issueRegistryError(ctx, e)
				})
			} else {
				prepared, e := p.authority.PrepareIssueRequest(ctx, r.Intent, t.Context)
				if e != nil {
					return p.issueRegistryError(ctx, e)
				}
				if err = p.node.RecheckContext(ctx, t.Context); err != nil {
					return ErrNode
				}
				err = s.do(func(check func() error) error {
					var e error
					issuer, job, e = p.authority.CommitIssueRequest(prepared, check)
					if e == nil {
						s.effects = true
					}
					return p.issueRegistryError(ctx, e)
				})
			}
			if err != nil {
				return err
			}
		} else if err != nil {
			return p.issueRegistryError(ctx, err)
		}
		if issuer.IntentID != r.IntentID || issuer.ProfileID != r.Intent.ProfileID || issuer.GymID != r.Intent.GymID || issuer.PublicKey != r.Intent.PublicKey {
			return ErrCorrupt
		}
		if issuer.State == "sealed" {
			r, err = p.finishIssue(s, r, issuer, prospectjob.ManifestJob{}, t, "not_issued")
			if err != nil {
				return err
			}
			out, err = p.issueResult(ctx, r)
			return err
		}
		if issuer.State != "issued" {
			return ErrCorrupt
		}
		if job.ID == "" {
			job, err = p.originalJob(ctx, issuer.JobID)
			if err != nil {
				return err
			}
		}
		if job.ID != issuer.JobID || job.Context != t.Context || job.PublicKey != key || job.GymID != s.gym {
			return ErrCorrupt
		}
		w := savedWork{job, t}
		prepared, err := p.prepareContext(ctx, w)
		if err != nil {
			return err
		}
		ms, err := p.sessionClock(s)
		if err != nil {
			return err
		}
		if ms < job.IssuedAtMS {
			return ErrClock
		}
		if ms >= job.ExpiresAtMS {
			r, err = p.finishIssue(s, r, issuer, job, t, "issued_unused")
			if err != nil {
				return err
			}
			out, err = p.issueResult(ctx, r)
			return err
		}
		if err = p.node.Proposal(ctx, job.Context, prepared.Candidate().BlockHex); err != nil {
			return ErrNode
		}
		if err = p.node.RecheckContext(ctx, job.Context); err != nil {
			return ErrNode
		}
		if _, err = p.prepareContext(ctx, w); err != nil {
			return err
		}
		r, err = p.finishIssue(s, r, issuer, job, t, "work")
		if err != nil {
			return err
		}
		out, err = p.issueResult(ctx, r)
		return err
	})
	if err != nil {
		return IssueResult{}, err
	}
	return out, nil
}
func (p *Pool) resolveIssueTemplate(ctx context.Context) (prospectwork.Template, []byte, error) {
	t, err := p.node.ResolveTemplate(ctx, p.config.Policy)
	if err != nil || !p.samePolicy(t.Context) || !templateBounds(t) {
		return prospectwork.Template{}, nil, ErrNode
	}
	raw, err := json.Marshal(t)
	var owned prospectwork.Template
	if err != nil || len(raw) > prospectwork.MaxTemplateSize || decode(raw, prospectwork.MaxTemplateSize, &owned) != nil || prospectwork.ValidateTemplate(owned) != nil {
		return prospectwork.Template{}, nil, ErrNode
	}
	return owned, raw, nil
}
func (p *Pool) unissuedReason(ctx context.Context, t prospectwork.Template) (string, error) {
	if a := p.operatorAdmission; a != nil && a.daily != nil {
		_, over, err := a.dailyCensus(ctx)
		if err != nil {
			return "", err
		}
		if over || a.draining || p.meta.Submissions >= a.maximum {
			a.draining = true
			return "intake_drained", nil
		}
		if !a.enabled {
			return "", ErrBusy
		}
	}
	// Failed Recheck alone is never positive evidence of a changed parent.
	fresh, _, err := p.resolveIssueTemplate(ctx)
	if err != nil {
		return "", err
	}
	if err = p.node.RecheckContext(ctx, fresh.Context); err != nil {
		return "", ErrNode
	}
	if fresh.Context.ParentHash != t.Context.ParentHash {
		return "parent_changed", nil
	}
	if err = p.node.RecheckContext(ctx, t.Context); err != nil {
		return "", ErrNode
	}
	return "", nil
}
func (p *Pool) issueRegistryError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if err == registry.ErrBusy {
		return ErrBusy
	}
	if err == registry.ErrPublicCapacity {
		return ErrCapacity
	}
	if errors.Is(err, registry.ErrPoisoned) {
		return ErrOutcomeUnknown
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (p *Pool) finishIssue(s *poolSession, old issueRequest, issuer registry.IssuerRequest, job prospectjob.ManifestJob, t prospectwork.Template, state string) (issueRequest, error) {
	var result issueRequest
	// Work and original template preparation occurred outside the account lease.
	tr, _ := json.Marshal(t)
	before, _ := json.Marshal(old)
	var wr []byte
	if state == "work" {
		var err error
		wr, err = json.Marshal(savedWork{job, t})
		if err != nil || len(wr) > maxWorkSize {
			return result, ErrCorrupt
		}
	}
	err := s.do(func(check func() error) error {
		original, err := p.loadIssueRow(old.Intent.RequestID)
		if err != nil || original != old || old.State != "pending" {
			return ErrCorrupt
		}
		ms, err := p.observeClock(check)
		if err != nil {
			return err
		}
		if state == "not_issued" {
			if issuer.State != "sealed" || ms < issuer.SealedAtMS {
				return ErrCorrupt
			}
		} else if issuer.State != "issued" || ms < job.IssuedAtMS {
			return ErrClock
		}
		if state == "work" && ms >= job.ExpiresAtMS {
			state = "issued_unused"
			wr = nil
		}
		if state == "issued_unused" && ms < job.ExpiresAtMS {
			return ErrClock
		}
		nextRow := old
		nextRow.State = state
		nextRow.ResolvedAtMS = ms
		if state == "not_issued" {
			nextRow.Reason = issuer.Reason
		} else {
			nextRow.JobID = job.ID
			if state == "issued_unused" {
				nextRow.Reason = "expired_before_work"
			}
		}
		raw, _ := json.Marshal(nextRow)
		if len(raw) > registry.MaxIssueRowSize {
			return ErrCorrupt
		}
		next := p.meta
		next.LastClockMS = ms
		selected := *p.issueRecovery
		reservation := issuePendingReserve(len(before), len(tr))
		if selected.reserved < reservation || selected.bytes < uint64(len(before)) {
			return ErrCorrupt
		}
		selected.reserved -= reservation
		selected.bytes = selected.bytes - uint64(len(before)) + uint64(len(raw))
		b := new(leveldb.Batch)
		b.Put(issueKey("issue/r/", old.Intent.RequestID), raw)
		if state != "not_issued" {
			for _, k := range [][]byte{workKey(job.ID), issueKey("issue/j/", job.ID)} {
				present, err := p.db.Has(k, nil)
				if err != nil || present {
					return ErrCorrupt
				}
			}
			id, _ := hex.DecodeString(old.Intent.RequestID)
			b.Put(issueKey("issue/j/", job.ID), id)
			selected.bytes += 40 + 32
		}
		if state == "work" {
			if selected.bytes < uint64(40+len(tr)) || next.Works >= p.config.MaxWorks {
				return ErrCorrupt
			}
			selected.bytes -= uint64(40 + len(tr))
			b.Delete(issueKey("issue/t/", old.Intent.RequestID))
			b.Put(workKey(job.ID), wr)
			next.Works++
			next.Bytes += uint64(len(wr))
		}
		if !issueOccupancyFits(next.Bytes, selected.bytes, selected.reserved, p.config.MaxBytes) {
			return ErrCorrupt
		}
		if err = check(); err != nil {
			return err
		}
		if err = p.commitIssue(b, next, &selected); err != nil {
			return err
		}
		s.effects = true
		result = nextRow
		p.mark("issue-resolved")
		return nil
	})
	if err != nil {
		return issueRequest{}, err
	}
	return result, nil
}
