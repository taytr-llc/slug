// Copyright (c) 2026 The SLUG developers. ISC license.
package recoveryfile

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

func (h *Handle) acquire(mode string) error {
	if h == nil {
		return ErrInvalid
	}
	if !h.mu.TryLock() {
		return ErrBusy
	}
	if h.closed {
		h.mu.Unlock()
		return ErrClosed
	}
	if h.used || h.pin.Mode != mode {
		h.mu.Unlock()
		return ErrInvalid
	}
	h.used = true
	return nil
}
func (h *Handle) parentMatches() error {
	// Revalidate the captured canonical route without following even an
	// intermediate replacement link. The publication itself still uses only
	// h.dir, never this newly observed pathname or descriptor.
	dir, _, e := walkDirectory(filepath.Dir(h.pin.Path), true)
	if e != nil {
		return errors.Join(ErrInvalid, e)
	}
	st, se := stampFile(dir)
	ce := closeOwned(se, dir)
	if ce != nil {
		return errors.Join(ErrInvalid, ce)
	}
	if !st.directory || st.identity != h.pin.Parent {
		return ErrInvalid
	}
	return nil
}

// callFence records the actual callback result so an omitted, repeated or
// error-swallowing adapter cannot turn a failed commit into success.
func callFence(ctx context.Context, fence Fence, work func(func() error) error) error {
	if ctx == nil || fence == nil || ctx.Err() != nil {
		return ErrInvalid
	}
	calls := 0
	var callbackErr error
	var callbackMu sync.Mutex
	sealed := false
	outer := fence(func(check func() error) error {
		callbackMu.Lock()
		defer callbackMu.Unlock()
		if sealed {
			return ErrInvalid
		}
		calls++
		if calls != 1 {
			callbackErr = errors.Join(callbackErr, ErrInvalid)
			return ErrInvalid
		}
		if e := checkContext(ctx, check); e != nil {
			callbackErr = e
			return e
		}
		callbackErr = work(check)
		return callbackErr
	})
	callbackMu.Lock()
	defer callbackMu.Unlock()
	sealed = true
	if calls != 1 {
		return errors.Join(ErrInvalid, callbackErr, outer)
	}
	return errors.Join(callbackErr, outer, ctx.Err())
}

// Publish copies the bounded bytes before entering the original authority
// fence. Only the exclusive, descriptor-relative local commit is fenced.
func (h *Handle) Publish(ctx context.Context, raw []byte, fence Fence) (resultErr error) {
	if ctx == nil || len(raw) < 1 || len(raw) > MaxSize || fence == nil {
		return ErrInvalid
	}
	if e := h.acquire(CreateMode); e != nil {
		return e
	}
	defer h.mu.Unlock()
	defer func() {
		if errors.Is(resultErr, ErrCleanup) {
			h.closeErr = errors.Join(h.closeErr, resultErr)
		}
	}()
	if e := h.parentMatches(); e != nil {
		return e
	}
	owned := append([]byte(nil), raw...)
	defer clear(owned)
	linked, possiblyLinked := false, false
	resultErr = callFence(ctx, fence, func(check func() error) (err error) {
		if e := h.parentMatches(); e != nil {
			return e
		}
		if e := checkContext(ctx, check); e != nil {
			return e
		}
		f, e := h.ops.create(h.dir, h.temporary)
		if e != nil {
			return e
		}
		if f == nil {
			return ErrInvalid
		}
		open := true
		created := true
		initial, se := stampFile(f)
		defer func() {
			if open {
				err = errors.Join(err, failedClose(f, h.ops.closeFile(f)))
				open = false
			}
			if created {
				current, e := h.ops.inspect(h.dir, h.temporary)
				if e != nil || current.identity != initial.identity || !current.regular {
					err = errors.Join(err, ErrCleanup, e)
				} else if e = h.ops.remove(h.dir, h.temporary); e != nil {
					err = errors.Join(err, ErrCleanup, e)
				}
			}
		}()
		if se != nil || !initial.regular {
			return ErrInvalid
		}
		if e = f.Chmod(0600); e != nil {
			return e
		}
		n, e := h.ops.write(f, owned)
		if e != nil {
			return e
		}
		if n != len(owned) {
			return io.ErrShortWrite
		}
		if e = h.ops.syncFile(f); e != nil {
			return e
		}
		prepared, e := stampFile(f)
		if e != nil || !prepared.regular || prepared.identity != initial.identity || prepared.size != int64(len(owned)) {
			return ErrInvalid
		}
		e = failedClose(f, h.ops.closeFile(f))
		open = false
		if e != nil {
			return e
		}
		if e = checkContext(ctx, check); e != nil {
			return e
		}
		if e = h.parentMatches(); e != nil {
			return e
		}
		current, e := h.ops.inspect(h.dir, h.temporary)
		if e != nil || current != prepared {
			return ErrInvalid
		}
		e = h.ops.link(h.dir, h.temporary, h.name)
		if e != nil {
			if os.IsExist(e) {
				return ErrConflict
			}
			possiblyLinked = true
			return e
		}
		linked = true
		// A successful syscall alone is not proof that a same-user namespace
		// mutation left the owned inode at the published name.
		published, e := h.ops.inspect(h.dir, h.name)
		if e != nil || published != prepared {
			return ErrInvalid
		}
		current, e = h.ops.inspect(h.dir, h.temporary)
		if e != nil || current.identity != prepared.identity || !current.regular {
			return ErrCleanup
		}
		if e = h.ops.remove(h.dir, h.temporary); e != nil {
			return errors.Join(ErrCleanup, e)
		}
		created = false
		if e = h.ops.syncDirectory(h.dir); e != nil {
			return e
		}
		if e = h.parentMatches(); e != nil {
			return e
		}
		return checkContext(ctx, check)
	})
	if resultErr != nil && (linked || possiblyLinked) {
		resultErr = errors.Join(ErrOutcomeUnknown, resultErr)
	}
	return resultErr
}

// Read only discloses owned bytes if the original authority fence succeeds.
// The native-held file identity and before/after metadata must still match.
func (h *Handle) Read(ctx context.Context, fence Fence) (out []byte, resultErr error) {
	if ctx == nil || fence == nil {
		return nil, ErrInvalid
	}
	if e := h.acquire(ReadMode); e != nil {
		return nil, e
	}
	defer h.mu.Unlock()
	defer func() {
		if errors.Is(resultErr, ErrCleanup) {
			h.closeErr = errors.Join(h.closeErr, resultErr)
		}
	}()
	if e := h.parentMatches(); e != nil {
		return nil, e
	}
	buf := make([]byte, int(h.pin.File.Size)+1)
	defer func() {
		if resultErr != nil {
			clear(buf)
			out = nil
		}
	}()
	resultErr = callFence(ctx, fence, func(check func() error) error {
		if e := h.parentMatches(); e != nil {
			return e
		}
		before, e := stampFile(h.file)
		if e != nil || !before.regular || before.fileIdentity() != *h.pin.File {
			return ErrInvalid
		}
		named, e := h.ops.inspect(h.dir, h.name)
		if e != nil || named != before {
			return ErrInvalid
		}
		offset := 0
		for offset < len(buf) {
			if e = checkContext(ctx, check); e != nil {
				return e
			}
			n, re := h.ops.read(h.file, buf[offset:])
			if n < 0 || n > len(buf)-offset {
				return ErrInvalid
			}
			offset += n
			if re == io.EOF {
				break
			}
			if re != nil {
				return re
			}
			if n == 0 {
				return io.ErrNoProgress
			}
		}
		if offset != int(h.pin.File.Size) {
			return ErrInvalid
		}
		after, e := stampFile(h.file)
		if e != nil || after != before {
			return ErrInvalid
		}
		named, e = h.ops.inspect(h.dir, h.name)
		if e != nil || named != before {
			return ErrInvalid
		}
		if e = h.parentMatches(); e != nil {
			return e
		}
		if e = checkContext(ctx, check); e != nil {
			return e
		}
		out = buf[:offset:offset]
		return nil
	})
	return out, resultErr
}
