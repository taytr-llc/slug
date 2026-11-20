// Copyright (c) 2026 The SLUG developers. ISC license.
package recoveryfile

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
)

// PublicEvidenceMaxSize is separate from the unchanged recovery-file MaxSize.
const PublicEvidenceMaxSize = 524288

// PublicEvidenceFile owns one create-only public export. Stage performs bounded
// bulk IO outside the account lease; Publish fences only the final exclusive
// link and its durability/identity checks. The caller must join Close before
// reporting success, and must never retry a possibly published operation.
// This helper validates storage ownership, not the public archive's contents.
type PublicEvidenceFile struct {
	h                 *Handle
	created, staged   bool
	publishAttempted  bool
	possiblyPublished bool
	initial, prepared fileStamp
}

// OpenPublicEvidence synchronously acquires the native-selected parent. It
// creates no temporary or destination file. The returned handle is never a
// recovery read/export capability and cannot enlarge the old Publish limit.
func OpenPublicEvidence(pin Pin, protected []string) (*PublicEvidenceFile, error) {
	if ValidatePin(pin, CreateMode) != nil {
		return nil, ErrInvalid
	}
	h, err := Open(pin, protected)
	if err != nil {
		return nil, err
	}
	h.temporary = ".slug-evidence-" + strings.TrimPrefix(h.temporary, ".slug-recovery-")
	return &PublicEvidenceFile{h: h}, nil
}

// cleanupLocked removes only the staging name still identifying our inode.
// Never remove a substituted inode or the published destination. Cleanup is
// outside the account fence and failures remain sticky even if Close retries it.
func (p *PublicEvidenceFile) cleanupLocked() error {
	if !p.created {
		return nil
	}
	h := p.h
	current, err := h.ops.inspect(h.dir, h.temporary)
	if os.IsNotExist(err) {
		p.created = false
		return nil
	}
	if err != nil || !current.regular || current.identity != p.initial.identity {
		return errors.Join(ErrCleanup, err)
	}
	if err := h.ops.remove(h.dir, h.temporary); err != nil {
		return errors.Join(ErrCleanup, err)
	}
	p.created = false
	if err := h.ops.syncDirectory(h.dir); err != nil {
		return errors.Join(ErrCleanup, err)
	}
	return nil
}

func (p *PublicEvidenceFile) rememberCleanup(err error) {
	if errors.Is(err, ErrCleanup) {
		p.h.closeErr = errors.Join(p.h.closeErr, err)
	}
}

// Stage accepts one bounded, owned copy of the caller's bytes. No authority
// fence is entered and no final destination is created. A failed stage is not
// reusable; Close still joins cleanup and reports any sticky cleanup failure.
func (p *PublicEvidenceFile) Stage(ctx context.Context, raw []byte) (resultErr error) {
	if p == nil || ctx == nil || len(raw) < 1 || len(raw) > PublicEvidenceMaxSize {
		return ErrInvalid
	}
	h := p.h
	if err := h.acquire(CreateMode); err != nil {
		return err
	}
	defer h.mu.Unlock()
	defer func() {
		if resultErr != nil {
			p.staged = false
			resultErr = errors.Join(resultErr, p.cleanupLocked())
			p.rememberCleanup(resultErr)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := h.parentMatches(); err != nil {
		return err
	}
	owned := append([]byte(nil), raw...)
	defer clear(owned)
	f, err := h.ops.create(h.dir, h.temporary)
	if err != nil {
		return err
	}
	if f == nil {
		return ErrInvalid
	}
	p.created = true
	open := true
	closeTemp := func() error {
		open = false
		err := h.ops.closeFile(f)
		if err != nil {
			// Join the actual descriptor even when a fault adapter reports a
			// close failure without performing the close. A real os.File.Close
			// has already retired its descriptor even when it returns an error.
			joined := f.Close()
			if errors.Is(joined, os.ErrClosed) {
				joined = nil
			}
			return errors.Join(ErrCleanup, err, joined)
		}
		return nil
	}
	defer func() {
		if open {
			resultErr = errors.Join(resultErr, closeTemp())
		}
	}()
	p.initial, err = stampFile(f)
	if err != nil || !p.initial.regular {
		return errors.Join(ErrInvalid, err)
	}
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := h.ops.write(f, owned)
	if err != nil {
		return err
	}
	if n != len(owned) {
		return io.ErrShortWrite
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := h.ops.syncFile(f); err != nil {
		return err
	}
	p.prepared, err = stampFile(f)
	if err != nil || !p.prepared.regular || p.prepared.identity != p.initial.identity || p.prepared.size != int64(len(owned)) {
		return errors.Join(ErrInvalid, err)
	}
	if err := closeTemp(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := h.ops.inspect(h.dir, h.temporary)
	if err != nil || current != p.prepared {
		return errors.Join(ErrInvalid, err)
	}
	p.staged = true
	return nil
}

// Publish is single-use even after definite refusal. Existing destinations are
// never replaced. Any uncertainty after a possible link preserves the final
// destination and returns ErrOutcomeUnknown. Staging cleanup is joined before
// this call returns, outside the original account fence.
func (p *PublicEvidenceFile) Publish(ctx context.Context, fence Fence) (resultErr error) {
	if p == nil || p.h == nil || ctx == nil || fence == nil {
		return ErrInvalid
	}
	h := p.h
	if !h.mu.TryLock() {
		return ErrBusy
	}
	defer h.mu.Unlock()
	if h.closed {
		return ErrClosed
	}
	if !p.staged || p.publishAttempted {
		return ErrInvalid
	}
	p.publishAttempted = true
	defer func() {
		resultErr = errors.Join(resultErr, p.cleanupLocked())
		p.rememberCleanup(resultErr)
		if resultErr != nil && p.possiblyPublished {
			resultErr = errors.Join(ErrOutcomeUnknown, resultErr)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	return callFence(ctx, fence, func(check func() error) error {
		if err := h.parentMatches(); err != nil {
			return err
		}
		if err := checkContext(ctx, check); err != nil {
			return err
		}
		current, err := h.ops.inspect(h.dir, h.temporary)
		if err != nil || current != p.prepared {
			return errors.Join(ErrInvalid, err)
		}
		if err := h.ops.link(h.dir, h.temporary, h.name); err != nil {
			if os.IsExist(err) {
				return ErrConflict
			}
			p.possiblyPublished = true
			return err
		}
		p.possiblyPublished = true
		published, err := h.ops.inspect(h.dir, h.name)
		if err != nil || published != p.prepared {
			return errors.Join(ErrInvalid, err)
		}
		if err := h.ops.syncDirectory(h.dir); err != nil {
			return err
		}
		if err := h.parentMatches(); err != nil {
			return err
		}
		return checkContext(ctx, check)
	})
}

// Close waits for Stage/Publish, cleans only owned staging storage and retires
// all descriptors. Callers must propagate cleanup failure and prevent further
// exports by that owner. A failure after possible publication remains uncertain.
func (p *PublicEvidenceFile) Close() error {
	if p == nil || p.h == nil {
		return nil
	}
	h := p.h
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return h.closeErr
	}
	h.closed = true
	h.closeErr = errors.Join(h.closeErr, p.cleanupLocked())
	if err := closeFiles(h.file, h.dir); err != nil {
		h.closeErr = errors.Join(h.closeErr, ErrCleanup, err)
	}
	if h.closeErr != nil && p.possiblyPublished {
		h.closeErr = errors.Join(ErrOutcomeUnknown, h.closeErr)
	}
	return h.closeErr
}
