// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/wire/v2"
)

// DailyBinding reaudits the original writable daily source for construction of
// its public service. It returns public configuration only; it neither enables
// intake nor replaces an original source, node, issuer or session owner.
func (p *Pool) DailyBinding(ctx context.Context) (Config, error) {
	if wire.TestPoW {
		return Config{}, ErrProfile
	}
	if ctx == nil {
		return Config{}, ErrInvalid
	}
	if p == nil || p.db == nil || p.gate == nil {
		return Config{}, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	p.lifecycle.Lock()
	if p.closing {
		p.lifecycle.Unlock()
		return Config{}, ErrClosed
	}
	select {
	case p.gate <- struct{}{}:
	default:
		p.lifecycle.Unlock()
		return Config{}, ErrBusy
	}
	p.lifecycle.Unlock()
	defer func() { <-p.gate }()
	if p.poisoned {
		return Config{}, ErrPoisoned
	}
	if p.auditOnly || p.config.Version != Version2 {
		return Config{}, ErrConfig
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := p.db.Get([]byte("meta"), nil)
	var candidate metadata
	if err != nil || decode(raw, maxMetadataSize, &candidate) != nil || candidate != p.meta || candidate.Application != application || candidate.Version != Version2 || candidate.Config != p.config || candidate.Works > p.config.MaxWorks || candidate.Submissions > p.config.MaxSubmissions || candidate.Accepted > candidate.Submissions || candidate.Bytes > p.config.MaxBytes || candidate.LastClockMS < 0 {
		return Config{}, ErrCorrupt
	}
	info, err := p.originalInfo(ctx)
	if err != nil {
		return Config{}, err
	}
	if info.Pin != p.config.AuthorityPin || info.ReadOnly || info.Jobs < candidate.Works {
		return Config{}, ErrCorrupt
	}
	if err = p.auditRowsContext(ctx); err != nil {
		if err == ErrNotFound || err == ErrInvalid {
			err = ErrCorrupt
		}
		return Config{}, err
	}
	if err = ctx.Err(); err != nil {
		return Config{}, err
	}
	return p.config, nil
}
