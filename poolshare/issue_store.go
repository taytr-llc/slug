// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
)

func validateIssueConfig(ctx context.Context, c Config, i registry.IssueProfile, a *registry.ManifestStore, n Node, s prospectservice.SessionAuthorizer, audit bool) error {
	if err := validateConfigSelected(ctx, c, a, n, s, audit, Version2); err != nil {
		return err
	}
	id, err := DailyConfigID(c)
	if err != nil || registry.ValidateIssueProfile(i) != nil || i.SourceConfigID != id || i.MaxRequests != c.MaxWorks {
		return ErrConfig
	}
	info, _, err := a.AuditIssueBinding(ctx, c.AuthorityPin, i)
	if err != nil {
		return err
	}
	if info.ReadOnly && !audit {
		return ErrConfig
	}
	return ctx.Err()
}
func CreateRecoverableDaily(ctx context.Context, path string, c Config, i registry.IssueProfile, a *registry.ManifestStore, n Node, s prospectservice.SessionAuthorizer) (*Pool, error) {
	if err := validateIssueConfig(ctx, c, i, a, n, s, false); err != nil {
		return nil, err
	}
	p := newPool(c, a, n, s)
	p.issueRecovery = &issueState{profile: i}
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, ErrInvalid
	}
	raw, err := json.Marshal(issueMarker{issueApplication, registry.IssueVersion1, marker{application, Version2, c}, i})
	if err != nil || len(raw) > 2*maxMetadataSize {
		return nil, ErrConfig
	}
	f, err := os.OpenFile(filepath.Join(path, markerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	count, we := f.Write(raw)
	if we == nil && count != len(raw) {
		we = io.ErrShortWrite
	}
	se, ce := f.Sync(), f.Close()
	if ce != nil {
		return failedDailyFile(p, f, errors.Join(we, se, ce))
	}
	if err = errors.Join(we, se); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(path, "db")
	if err = os.Mkdir(dbPath, 0700); err != nil {
		return nil, err
	}
	db, err := leveldb.OpenFile(dbPath, &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
	if err != nil {
		return nil, err
	}
	p.db, p.writeBatch = db, db.Write
	p.meta = metadata{Application: application, Version: Version2, Config: c}
	if err = ctx.Err(); err == nil {
		err = p.commit(new(leveldb.Batch), p.meta)
	}
	if err != nil {
		return failedSelectedPool(p, err)
	}
	return syncDailyDirectories(ctx, p, dbPath, path, filepath.Dir(filepath.Clean(path)))
}
func OpenRecoverableDaily(ctx context.Context, path string, c Config, i registry.IssueProfile, a *registry.ManifestStore, n Node, s prospectservice.SessionAuthorizer) (*Pool, error) {
	return openIssuePool(ctx, path, c, i, a, n, s, false)
}
func OpenRecoverableDailyAudit(ctx context.Context, path string, c Config, i registry.IssueProfile, a *registry.ManifestStore) (*Pool, error) {
	return openIssuePool(ctx, path, c, i, a, nil, nil, true)
}
func openIssuePool(ctx context.Context, path string, c Config, i registry.IssueProfile, a *registry.ManifestStore, n Node, s prospectservice.SessionAuthorizer, audit bool) (*Pool, error) {
	if err := validateIssueConfig(ctx, c, i, a, n, s, audit); err != nil {
		return nil, err
	}
	p := newPool(c, a, n, s)
	p.issueRecovery = &issueState{profile: i}
	if !directory(path) || !directory(filepath.Join(path, "db")) {
		return nil, ErrCorrupt
	}
	name := filepath.Join(path, markerName)
	before, err := os.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Size() > 2*maxMetadataSize {
		return nil, ErrCorrupt
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	opened, se := f.Stat()
	if se != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		ce := f.Close()
		if ce != nil {
			return failedDailyFile(p, f, errors.Join(ErrCorrupt, se, ce))
		}
		return nil, ErrCorrupt
	}
	raw, re := io.ReadAll(io.LimitReader(f, 2*maxMetadataSize+1))
	ce := f.Close()
	if ce != nil {
		return failedDailyFile(p, f, errors.Join(re, ce))
	}
	if re != nil {
		return nil, re
	}
	var m issueMarker
	if decode(raw, 2*maxMetadataSize, &m) != nil || m.Application != issueApplication || m.Version != registry.IssueVersion1 || m.Base != (marker{application, Version2, c}) || m.IssueProfile != i {
		return nil, ErrCorrupt
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, ReadOnly: audit, Strict: opt.StrictAll})
	if err != nil {
		return nil, err
	}
	p.db, p.writeBatch, p.auditOnly = db, db.Write, audit
	raw, err = db.Get([]byte("meta"), nil)
	var meta issueMetadata
	if err != nil || decode(raw, 2*maxMetadataSize, &meta) != nil || meta.Application != issueApplication || meta.Version != registry.IssueVersion1 || meta.IssueProfile != i {
		return failedSelectedPool(p, ErrCorrupt)
	}
	p.issueRecovery = &issueState{i, meta.Requests, meta.RequestBytes, meta.RequestReserved}
	p.meta = meta.Base
	if err = p.auditContext(ctx); err != nil {
		return failedSelectedPool(p, err)
	}
	return p, nil
}

func issueOccupancyFits(base, used, reserved, limit uint64) bool {
	return base <= limit && used <= limit-base && reserved <= limit-base-used
}
func (p *Pool) freeSourceBytes() uint64 {
	if p.issueRecovery == nil {
		if p.meta.Bytes > p.config.MaxBytes {
			return 0
		}
		return p.config.MaxBytes - p.meta.Bytes
	}
	if !issueOccupancyFits(p.meta.Bytes, p.issueRecovery.bytes, p.issueRecovery.reserved, p.config.MaxBytes) {
		return 0
	}
	return p.config.MaxBytes - p.meta.Bytes - p.issueRecovery.bytes - p.issueRecovery.reserved
}
func (p *Pool) decodeStoredMetadata(raw []byte, out *metadata) error {
	if p.issueRecovery == nil {
		return decode(raw, maxMetadataSize, out)
	}
	var m issueMetadata
	if decode(raw, 2*maxMetadataSize, &m) != nil || m.Application != issueApplication || m.Version != registry.IssueVersion1 || m.IssueProfile != p.issueRecovery.profile || m.Requests != p.issueRecovery.requests || m.RequestBytes != p.issueRecovery.bytes || m.RequestReserved != p.issueRecovery.reserved || m.Requests > m.IssueProfile.MaxRequests || m.Base.Works > m.Requests || m.Base.LastClockMS > registry.MaxPublicTimestampMS || !issueOccupancyFits(m.Base.Bytes, m.RequestBytes, m.RequestReserved, p.config.MaxBytes) {
		return ErrCorrupt
	}
	inner, _ := json.Marshal(m.Base)
	if len(inner) > maxMetadataSize {
		return ErrCorrupt
	}
	*out = m.Base
	return nil
}

// This selection check is specific to constructing a service. Base2 financial
// consumers continue to use DailyBinding on the same original Pool owner.
func (p *Pool) CheckDailyServiceSelection(ctx context.Context, expected *registry.IssueProfile) error {
	return p.operatorGate(ctx, func(work context.Context) error {
		if p.auditOnly || p.config.Version != Version2 {
			return ErrConfig
		}
		if expected == nil {
			if p.issueRecovery != nil {
				return ErrConfig
			}
			return work.Err()
		}
		if p.issueRecovery == nil || p.issueRecovery.profile != *expected {
			return ErrConfig
		}
		return p.auditIssueLive(work)
	})
}
func (p *Pool) RecoverableDailyBinding(ctx context.Context) (Config, registry.IssueProfile, error) {
	var c Config
	var i registry.IssueProfile
	err := p.operatorGate(ctx, func(work context.Context) error {
		if p.auditOnly || p.issueRecovery == nil || p.config.Version != Version2 {
			return ErrConfig
		}
		if err := p.auditIssueLive(work); err != nil {
			return err
		}
		c, i = p.config, p.issueRecovery.profile
		return nil
	})
	if err != nil {
		return Config{}, registry.IssueProfile{}, err
	}
	return c, i, nil
}
func (p *Pool) auditIssueLive(ctx context.Context) error {
	raw, err := p.db.Get([]byte("meta"), nil)
	var m metadata
	if err != nil || p.decodeStoredMetadata(raw, &m) != nil || m != p.meta {
		return ErrCorrupt
	}
	return p.auditRowsContext(ctx)
}
