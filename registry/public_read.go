// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
)

func (p *ManifestPublication) readLock(ctx context.Context) error {
	if ctx == nil {
		return ErrPublicOwnership
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := p.Alive(); e != nil {
		return e
	}
	if !p.store.mu.TryLock() {
		return ErrBusy
	}
	if e := p.store.state(false); e != nil {
		p.store.mu.Unlock()
		return e
	}
	return nil
}
func (p *ManifestPublication) admissionLocked(n uint32) (PublicAdmissionStatement, error) {
	if n == 0 || n > p.store.public.meta.SignedAdmissionSequence {
		return PublicAdmissionStatement{}, ErrNotFound
	}
	raw, e := p.store.db.Get(publicCounterKey("ownership/admission/", n), nil)
	if e != nil {
		return PublicAdmissionStatement{}, ErrCorrupt
	}
	return ParsePublicAdmission(p.pin, raw)
}
func (p *ManifestPublication) publicationLocked(n uint32) (PublicPublicationEntry, error) {
	if n == 0 || n > p.store.public.meta.PublicationSequence {
		return PublicPublicationEntry{}, ErrNotFound
	}
	raw, e := p.store.db.Get(publicCounterKey("ownership/publication/", n), nil)
	if e != nil {
		return PublicPublicationEntry{}, ErrCorrupt
	}
	return ParsePublicPublication(p.pin, raw)
}
func (p *ManifestPublication) Admission(ctx context.Context, n uint32) (PublicAdmissionStatement, error) {
	if e := p.readLock(ctx); e != nil {
		return PublicAdmissionStatement{}, e
	}
	defer p.store.mu.Unlock()
	return p.admissionLocked(n)
}
func (p *ManifestPublication) Publication(ctx context.Context, n uint32) (PublicPublicationEntry, error) {
	if e := p.readLock(ctx); e != nil {
		return PublicPublicationEntry{}, e
	}
	defer p.store.mu.Unlock()
	return p.publicationLocked(n)
}
func (p *ManifestPublication) AdmissionPage(ctx context.Context, after, limit, through uint32) (PublicAdmissionPage, error) {
	if limit == 0 || limit > 8 || after > through {
		return PublicAdmissionPage{}, ErrPublicOwnership
	}
	if e := p.readLock(ctx); e != nil {
		return PublicAdmissionPage{}, e
	}
	defer p.store.mu.Unlock()
	if through > p.store.public.meta.SignedAdmissionSequence {
		return PublicAdmissionPage{}, ErrNotFound
	}
	head := publicZero
	if through > 0 {
		a, e := p.admissionLocked(through)
		if e != nil {
			return PublicAdmissionPage{}, e
		}
		head, e = PublicAdmissionID(p.pin, a)
		if e != nil {
			return PublicAdmissionPage{}, e
		}
	}
	page := PublicAdmissionPage{1, p.pinID, through, head, after, after, false, make([]PublicAdmissionStatement, 0, limit)}
	for n := after + 1; n <= through && uint32(len(page.Items)) < limit; n++ {
		if e := ctx.Err(); e != nil {
			return PublicAdmissionPage{}, e
		}
		a, e := p.admissionLocked(n)
		if e != nil {
			return PublicAdmissionPage{}, e
		}
		page.Items = append(page.Items, a)
		page.Next = n
	}
	page.Complete = page.Next == through
	raw, e := json.Marshal(page)
	if e != nil || len(raw) > 655360 {
		return PublicAdmissionPage{}, ErrPublicOwnership
	}
	return page, nil
}
func (p *ManifestPublication) PublicationPage(ctx context.Context, after, limit, through uint32) (PublicPublicationPage, error) {
	if limit == 0 || limit > 8 || after > through {
		return PublicPublicationPage{}, ErrPublicOwnership
	}
	if e := p.readLock(ctx); e != nil {
		return PublicPublicationPage{}, e
	}
	defer p.store.mu.Unlock()
	if through > p.store.public.meta.PublicationSequence {
		return PublicPublicationPage{}, ErrNotFound
	}
	head := publicZero
	if through > 0 {
		v, e := p.publicationLocked(through)
		if e != nil {
			return PublicPublicationPage{}, e
		}
		head, e = PublicPublicationID(p.pin, v)
		if e != nil {
			return PublicPublicationPage{}, e
		}
	}
	page := PublicPublicationPage{1, p.pinID, through, head, after, after, false, make([]PublicPublicationEntry, 0, limit)}
	for n := after + 1; n <= through && uint32(len(page.Items)) < limit; n++ {
		if e := ctx.Err(); e != nil {
			return PublicPublicationPage{}, e
		}
		v, e := p.publicationLocked(n)
		if e != nil {
			return PublicPublicationPage{}, e
		}
		page.Items = append(page.Items, v)
		page.Next = n
	}
	page.Complete = page.Next == through
	return page, nil
}
func (p *ManifestPublication) Cutoff(ctx context.Context, eventID string) (PublicCutoffRecord, error) {
	if !publicHex(eventID, 32, true) {
		return PublicCutoffRecord{}, ErrPublicOwnership
	}
	if e := p.readLock(ctx); e != nil {
		return PublicCutoffRecord{}, e
	}
	defer p.store.mu.Unlock()
	raw, e := p.store.db.Get(publicIDKey("ownership/cutoff/", eventID), nil)
	if errors.Is(e, leveldb.ErrNotFound) {
		return PublicCutoffRecord{}, ErrNotFound
	}
	if e != nil {
		return PublicCutoffRecord{}, e
	}
	return ParsePublicCutoff(p.pin, raw)
}
