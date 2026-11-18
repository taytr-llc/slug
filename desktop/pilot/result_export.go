// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/seednode"
	"hash"
	"io"
	"os"
	"path/filepath"
)

// Only an unpublished 0600 temporary file receives the bounded proof stream.
// Large writes and file Sync occur before the final original-account lease.
type resultProofFile struct {
	root               *os.Root
	file               *os.File
	parent, path, name string
	info               os.FileInfo
	digest             hash.Hash
	count              uint32
	closed, linked     bool
}

func (c *ManifestController) newResultProofFile(path string) (*resultProofFile, error) {
	path, e := c.manifestExportPath(path, false)
	if e != nil {
		return nil, gymresult.ErrInvalid
	}
	parent := filepath.Dir(path)
	info, e := os.Stat(parent)
	if e != nil || !info.IsDir() {
		return nil, gymresult.ErrInvalid
	}
	root, e := os.OpenRoot(parent)
	if e != nil {
		return nil, e
	}
	fail := func(err error) (*resultProofFile, error) { _ = root.Close(); return nil, err }
	dir, e := root.Open(".")
	if e != nil {
		return fail(e)
	}
	pinned, e := dir.Stat()
	ce := dir.Close()
	if e != nil || ce != nil || !os.SameFile(info, pinned) {
		return fail(gymresult.ErrInvalid)
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return fail(e)
	}
	name := ".result-proof-" + hex.EncodeToString(nonce[:])
	file, e := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return fail(e)
	}
	return &resultProofFile{root: root, file: file, parent: parent, path: path, name: name, info: info, digest: sha256.New()}, nil
}
func (f *resultProofFile) Write(b []byte) (int, error) {
	if f.closed || uint64(f.count)+uint64(len(b)) > gymresult.MaxProofSize {
		return 0, gymresult.ErrCapacity
	}
	n, e := f.file.Write(b)
	if n > 0 {
		f.count += uint32(n)
		_, _ = f.digest.Write(b[:n])
	}
	if e == nil && n != len(b) {
		e = io.ErrShortWrite
	}
	return n, e
}
func (f *resultProofFile) close() error {
	var e error
	if !f.closed {
		e = f.file.Close()
		f.closed = true
	}
	e = errors.Join(e, f.root.Remove(f.name))
	return errors.Join(e, f.root.Close())
}
func (f *resultProofFile) prepare(ctx context.Context, s gymresult.ProofSummary) (func(func() error) error, error) {
	if f.closed || s.Bytes != f.count || s.SHA256 != hex.EncodeToString(f.digest.Sum(nil)) || ctx.Err() != nil {
		return nil, gymresult.ErrInvalid
	}
	e := f.file.Sync()
	ce := f.file.Close()
	f.closed = true
	if e != nil || ce != nil {
		return nil, errors.Join(e, ce)
	}
	return func(guard func() error) error {
		now, e := os.Stat(f.parent)
		if e != nil || !os.SameFile(f.info, now) || ctx.Err() != nil || guard == nil {
			return gymresult.ErrCancelled
		}
		if e = guard(); e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = f.root.Link(f.name, filepath.Base(f.path)); e != nil {
			return e
		}
		f.linked = true
		dir, e := f.root.Open(".")
		if e != nil {
			return gymresult.ErrOutcomeUnknown
		}
		e = dir.Sync()
		ce := dir.Close()
		if e != nil || ce != nil || ctx.Err() != nil {
			return gymresult.ErrOutcomeUnknown
		}
		return nil
	}, nil
}
func (s *StableController) exportResultProof(ctx context.Context, date, bid, path string, p *seednode.ResultPublisher) (out gymresult.ProofSummary, err error) {
	f, e := s.ManifestController.newResultProofFile(path)
	if e != nil {
		return out, e
	}
	defer func() {
		ce := f.close()
		if ce != nil {
			if f.linked {
				err = gymresult.ErrOutcomeUnknown
			} else {
				err = errors.Join(err, ce)
			}
		}
	}()
	out, err = s.resultStore.ExportProof(ctx, s.boutStore, s.cardStore, s.seedStore, s.eventStore, s.entryStore, s.session(), date, bid, p, f, func(v gymresult.ProofSummary) (func(func() error) error, error) { return f.prepare(ctx, v) })
	if err != nil && f.linked {
		err = gymresult.ErrOutcomeUnknown
	}
	return
}
