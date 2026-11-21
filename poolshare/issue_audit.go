// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

func issueKey(prefix, id string) []byte {
	b, _ := hex.DecodeString(id)
	return append([]byte(prefix), b...)
}
func issueTemplateID(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func issuePendingReserve(rowBytes, templateBytes int) uint64 {
	final := uint64(40 + registry.MaxIssueRowSize + 40 + 32 + templateBytes + prospectjob.MaxManifestJobSize + 20)
	now := uint64(40 + rowBytes + 40 + templateBytes)
	if final > now {
		return final - now
	}
	return 0
}
func (p *Pool) decodeIssueRow(raw []byte, id string) (issueRequest, error) {
	var r issueRequest
	if decode(raw, registry.MaxIssueRowSize, &r) != nil || p.issueRecovery == nil || r.Version != registry.IssueVersion1 || r.Intent.RequestID != id || !canonicalHash(id, true) {
		return issueRequest{}, ErrCorrupt
	}
	pid, err := registry.IssueProfileID(p.issueRecovery.profile)
	iid, ie := registry.IssueIntentID(r.Intent)
	if err != nil || ie != nil || r.Intent.ProfileID != pid || iid != r.IntentID || r.ResolvedAtMS > p.meta.LastClockMS || r.ResolvedAtMS > registry.MaxPublicTimestampMS {
		return issueRequest{}, ErrCorrupt
	}
	switch r.State {
	case "pending":
		if r.JobID != "" || r.ResolvedAtMS != 0 || r.Reason != "" {
			return issueRequest{}, ErrCorrupt
		}
	case "work":
		if !canonicalHash(r.JobID, true) || r.ResolvedAtMS <= 0 || r.Reason != "" {
			return issueRequest{}, ErrCorrupt
		}
	case "issued_unused":
		if !canonicalHash(r.JobID, true) || r.ResolvedAtMS <= 0 || r.Reason != "expired_before_work" {
			return issueRequest{}, ErrCorrupt
		}
	case "not_issued":
		if r.JobID != "" || r.ResolvedAtMS <= 0 || (r.Reason != "parent_changed" && r.Reason != "intake_drained") {
			return issueRequest{}, ErrCorrupt
		}
	default:
		return issueRequest{}, ErrCorrupt
	}
	return r, nil
}
func (p *Pool) loadIssueRow(id string) (issueRequest, error) {
	if !canonicalHash(id, true) {
		return issueRequest{}, ErrInvalid
	}
	raw, err := p.db.Get(issueKey("issue/r/", id), nil)
	if errors.Is(err, leveldb.ErrNotFound) {
		return issueRequest{}, ErrNotFound
	}
	if err != nil {
		return issueRequest{}, ErrCorrupt
	}
	return p.decodeIssueRow(raw, id)
}
func (p *Pool) issueTemplate(r issueRequest) (prospectwork.Template, []byte, error) {
	var t prospectwork.Template
	if r.State == "work" {
		raw, err := p.db.Get(workKey(r.JobID), nil)
		var w savedWork
		if err != nil || decode(raw, maxWorkSize, &w) != nil {
			return t, nil, ErrCorrupt
		}
		t = w.Template
	} else {
		raw, err := p.db.Get(issueKey("issue/t/", r.Intent.RequestID), nil)
		if err != nil || decode(raw, prospectwork.MaxTemplateSize, &t) != nil {
			return t, nil, ErrCorrupt
		}
	}
	raw, err := json.Marshal(t)
	if err != nil || len(raw) > prospectwork.MaxTemplateSize || !p.samePolicy(t.Context) || !templateBounds(t) || prospectwork.ValidateTemplate(t) != nil || issueTemplateID(raw) != r.Intent.TemplateSHA256 {
		return prospectwork.Template{}, nil, ErrCorrupt
	}
	gym, ge := gymauth.GymID(t.Context, r.Intent.PublicKey)
	if ge != nil || gym != r.Intent.GymID {
		return prospectwork.Template{}, nil, ErrCorrupt
	}
	return t, raw, nil
}

func (p *Pool) auditIssueRows(ctx context.Context) error {
	if p.issueRecovery == nil {
		return ErrConfig
	}
	info, total, err := p.authority.AuditIssueBinding(ctx, p.config.AuthorityPin, p.issueRecovery.profile)
	if err != nil {
		return p.sourceReadError(ctx, err)
	}
	if (!p.auditOnly && info.ReadOnly) || info.Jobs > total || total > p.issueRecovery.requests {
		return ErrCorrupt
	}
	var requests, templates, inverses, works, matched, issued, size, reserve uint64
	it := p.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		k, v := it.Key(), it.Value()
		if !bytes.HasPrefix(k, []byte("issue/")) {
			continue
		}
		if len(k) != 40 {
			return ErrCorrupt
		}
		size += uint64(len(k) + len(v))
		if size > p.config.MaxBytes {
			return ErrCorrupt
		}
		switch string(k[:8]) {
		case "issue/r/":
			id := hex.EncodeToString(k[8:])
			r, err := p.decodeIssueRow(v, id)
			if err != nil {
				return err
			}
			requests++
			if requests > p.issueRecovery.profile.MaxRequests {
				return ErrCorrupt
			}
			t, tr, err := p.issueTemplate(r)
			if err != nil {
				return err
			}
			ir, err := p.authority.TryIssueRequest(ctx, id)
			if err == registry.ErrIssueRequestNotFound {
				if r.State != "pending" {
					return ErrCorrupt
				}
				reserve += issuePendingReserve(len(v), len(tr))
				continue
			}
			if err != nil {
				return p.sourceReadError(ctx, err)
			}
			if ir.IntentID != r.IntentID || ir.ProfileID != r.Intent.ProfileID || ir.GymID != r.Intent.GymID || ir.PublicKey != r.Intent.PublicKey {
				return ErrCorrupt
			}
			matched++
			if ir.State == "issued" {
				issued++
				job, err := p.originalJob(ctx, ir.JobID)
				if err != nil {
					return err
				}
				if job.Context != t.Context || job.PublicKey != r.Intent.PublicKey || job.GymID != r.Intent.GymID {
					return ErrCorrupt
				}
				if _, err := p.prepareContext(ctx, savedWork{job, t}); err != nil {
					return err
				}
				switch r.State {
				case "pending":
					reserve += issuePendingReserve(len(v), len(tr))
				case "work":
					if r.JobID != job.ID || r.ResolvedAtMS < job.IssuedAtMS || r.ResolvedAtMS >= job.ExpiresAtMS {
						return ErrCorrupt
					}
					w, _, err := p.loadWorkContext(ctx, job.ID)
					if err != nil {
						return err
					}
					wr, _ := json.Marshal(w.Template)
					if !bytes.Equal(wr, tr) {
						return ErrCorrupt
					}
					works++
					if present, err := p.db.Has(issueKey("issue/t/", id), nil); err != nil || present {
						return ErrCorrupt
					}
				case "issued_unused":
					if r.JobID != job.ID || r.ResolvedAtMS < job.ExpiresAtMS {
						return ErrCorrupt
					}
					if exists, err := p.db.Has(workKey(job.ID), nil); err != nil || exists {
						return ErrCorrupt
					}
				default:
					return ErrCorrupt
				}
			} else if ir.State == "sealed" {
				if r.State == "pending" {
					reserve += issuePendingReserve(len(v), len(tr))
				} else if r.State != "not_issued" || r.Reason != ir.Reason || r.ResolvedAtMS < ir.SealedAtMS {
					return ErrCorrupt
				}
			} else {
				return ErrCorrupt
			}
			if r.State == "work" || r.State == "issued_unused" {
				inverse, err := p.db.Get(issueKey("issue/j/", r.JobID), nil)
				want, _ := hex.DecodeString(id)
				if err != nil || !bytes.Equal(inverse, want) {
					return ErrCorrupt
				}
			}
		case "issue/t/":
			r, err := p.loadIssueRow(hex.EncodeToString(k[8:]))
			if err != nil || r.State == "work" {
				return ErrCorrupt
			}
			templates++
		case "issue/j/":
			if len(v) != 32 {
				return ErrCorrupt
			}
			r, err := p.loadIssueRow(hex.EncodeToString(v))
			if err != nil || (r.State != "work" && r.State != "issued_unused") || r.JobID != hex.EncodeToString(k[8:]) {
				return ErrCorrupt
			}
			inverses++
		default:
			return ErrCorrupt
		}
	}
	if err := it.Error(); err != nil {
		return ErrCorrupt
	}
	// Inverse count additionally excludes unreferenced valid work. The registry
	// matched census excludes unknown sealed requests that job counts miss.
	if requests != p.issueRecovery.requests || size != p.issueRecovery.bytes || reserve != p.issueRecovery.reserved || works != p.meta.Works || templates != requests-works || inverses > issued || matched != total || issued != info.Jobs || !issueOccupancyFits(p.meta.Bytes, size, reserve, p.config.MaxBytes) {
		return ErrCorrupt
	}
	// Every inverse was checked above; every completed request checked its one
	// inverse. Pending issuer jobs deliberately do not yet have source inverses.
	return ctx.Err()
}
