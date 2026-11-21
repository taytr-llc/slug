// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"

	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
)

// DailyConfigID correlates one explicitly selected full configuration. It
// authenticates no source, transport, account, funding or signing authority.
func DailyConfigID(c Config) (string, error) {
	if err := validateConfigShape(c, Version2); err != nil {
		return "", err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", ErrConfig
	}
	h := sha256.Sum256(append([]byte("SLUG pool share daily source\x00v2\x00"), raw...))
	return hex.EncodeToString(h[:]), nil
}

// CreateDaily creates only a fresh V2 root; it never adopts or rewrites V1.
// A nonnil owner alongside an error means cleanup failed and must be retained.
func CreateDaily(ctx context.Context, path string, expected Config, authority *registry.ManifestStore, node Node, sessions prospectservice.SessionAuthorizer) (*Pool, error) {
	return createSelected(ctx, path, expected, authority, node, sessions, Version2)
}

// OpenDaily verifies every retained proof and index before returning the same
// original source. ctx bounds this audit, not the surviving Pool lifetime.
func OpenDaily(ctx context.Context, path string, expected Config, authority *registry.ManifestStore, node Node, sessions prospectservice.SessionAuthorizer) (*Pool, error) {
	return openSelected(ctx, path, expected, authority, node, sessions, false, Version2)
}

func OpenDailyAudit(ctx context.Context, path string, expected Config, authority *registry.ManifestStore) (*Pool, error) {
	return openSelected(ctx, path, expected, authority, nil, nil, true, Version2)
}

func (p *Pool) originalJob(ctx context.Context, id string) (prospectjob.ManifestJob, error) {
	if p.config.Version != Version2 {
		return p.authority.GetManifestJob(id)
	}
	if ctx == nil {
		return prospectjob.ManifestJob{}, ErrInvalid
	}
	job, err := p.authority.TryGetManifestJob(ctx, id)
	if err != nil {
		return prospectjob.ManifestJob{}, p.sourceReadError(ctx, err)
	}
	return job, nil
}

func (p *Pool) sourceReadError(ctx context.Context, err error) error {
	if p.config.Version == Version2 {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if err == registry.ErrBusy || err == ErrBusy {
			return ErrBusy
		}
	}
	return ErrCorrupt
}

func (p *Pool) originalInfo(ctx context.Context) (registry.ManifestAuthorityInfo, error) {
	if p.config.Version != Version2 {
		return p.authority.Info()
	}
	if ctx == nil {
		return registry.ManifestAuthorityInfo{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return registry.ManifestAuthorityInfo{}, err
	}
	info, err := p.authority.TryInfo()
	if err != nil {
		return registry.ManifestAuthorityInfo{}, p.sourceReadError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return registry.ManifestAuthorityInfo{}, err
	}
	return info, nil
}

// No usable failed owner escapes. Only an actual Close failure retains the
// original closed/poisoned owner; no replacement or cleanup retry is fabricated.
func failedSelectedPool(p *Pool, err error) (*Pool, error) {
	if p.config.Version == Version2 {
		if closeErr := p.Close(); closeErr != nil {
			return p, errors.Join(err, closeErr)
		}
	} else {
		_ = p.db.Close()
	}
	return nil, err
}

func failedDailyFile(p *Pool, f *os.File, err error) (*Pool, error) {
	p.failedFile, p.poisoned, p.closing, p.closeErr = f, true, true, ErrPoisoned
	close(p.closeDone)
	return p, errors.Join(err, ErrPoisoned)
}

func syncDailyDirectories(ctx context.Context, p *Pool, paths ...string) (*Pool, error) {
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return failedSelectedPool(p, err)
		}
		f, err := os.Open(path)
		if err != nil {
			return failedSelectedPool(p, ErrPoisoned)
		}
		syncErr, closeErr := f.Sync(), f.Close()
		if closeErr != nil {
			return failedDailyFile(p, f, errors.Join(syncErr, closeErr))
		}
		if syncErr != nil {
			return failedSelectedPool(p, ErrPoisoned)
		}
	}
	if err := ctx.Err(); err != nil {
		return failedSelectedPool(p, err)
	}
	return p, nil
}
