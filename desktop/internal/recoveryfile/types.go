// Copyright (c) 2026 The SLUG developers. ISC license.
// Package recoveryfile pins native-selected file objects across the private
// native/helper handoff. A Pin identifies storage, never account authority.
package recoveryfile

import (
	"context"
	"errors"
	"os"
	"sync"
)

const (
	CreateMode = "create"
	ReadMode   = "read"
	MaxSize    = 65536
	MaxPath    = 2048
)

var (
	ErrInvalid        = errors.New("invalid recovery file selection")
	ErrConflict       = errors.New("recovery destination already exists")
	ErrClosed         = errors.New("recovery file selection closed")
	ErrBusy           = errors.New("recovery file operation busy")
	ErrUnsupported    = errors.New("pinned recovery files unsupported")
	ErrOutcomeUnknown = errors.New("recovery file publication outcome unknown")
	ErrCleanup        = errors.New("recovery file cleanup failed")
)

type Identity struct {
	Device string `json:"device"`
	Inode  string `json:"inode"`
}
type FileIdentity struct {
	Device     string `json:"device"`
	Inode      string `json:"inode"`
	Size       int64  `json:"size"`
	ModifiedNS string `json:"modified_ns"`
}
type Pin struct {
	Version uint32        `json:"version"`
	Mode    string        `json:"mode"`
	Path    string        `json:"path"`
	Parent  Identity      `json:"parent"`
	File    *FileIdentity `json:"file"`
}

// Fence must invoke commit exactly once inside the caller's original account,
// view and capability lease. Wallet/network work precedes this local commit.
type Fence func(commit func(check func() error) error) error

type Selection struct {
	mu        sync.Mutex
	pin       Pin
	dir, file *os.File
	closed    bool
	closeErr  error
}

type Handle struct {
	mu              sync.Mutex
	pin             Pin
	dir, file       *os.File
	name, temporary string
	closed, used    bool
	closeErr        error
	ops             fileOperations
}

type fileStamp struct {
	identity           Identity
	size               int64
	modified           string
	regular, directory bool
}

func (s fileStamp) fileIdentity() FileIdentity {
	return FileIdentity{s.identity.Device, s.identity.Inode, s.size, s.modified}
}

type fileOperations struct {
	create        func(*os.File, string) (*os.File, error)
	write         func(*os.File, []byte) (int, error)
	syncFile      func(*os.File) error
	closeFile     func(*os.File) error
	link          func(*os.File, string, string) error
	remove        func(*os.File, string) error
	syncDirectory func(*os.File) error
	inspect       func(*os.File, string) (fileStamp, error)
	read          func(*os.File, []byte) (int, error)
}

func defaultOperations() fileOperations {
	return fileOperations{create: createRelative, write: func(f *os.File, b []byte) (int, error) { return f.Write(b) }, syncFile: func(f *os.File) error { return f.Sync() }, closeFile: func(f *os.File) error { return f.Close() }, link: linkRelative, remove: removeRelative, syncDirectory: func(f *os.File) error { return f.Sync() }, inspect: inspectRelative, read: func(f *os.File, b []byte) (int, error) { return f.Read(b) }}
}

func ownedPin(p Pin) Pin {
	if p.File != nil {
		f := *p.File
		p.File = &f
	}
	return p
}
func (s *Selection) Pin() Pin {
	if s == nil {
		return Pin{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Pin{}
	}
	return ownedPin(s.pin)
}
func (s *Selection) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	s.closeErr = closeFiles(s.file, s.dir)
	return s.closeErr
}
func (h *Handle) Close() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return h.closeErr
	}
	h.closed = true
	h.closeErr = errors.Join(h.closeErr, closeFiles(h.file, h.dir))
	return h.closeErr
}
func closeFiles(files ...*os.File) error {
	var es []error
	for _, f := range files {
		if f != nil {
			es = append(es, f.Close())
		}
	}
	return errors.Join(es...)
}
func checkContext(ctx context.Context, check func() error) error {
	if ctx == nil || check == nil {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := check(); e != nil {
		return e
	}
	return ctx.Err()
}
