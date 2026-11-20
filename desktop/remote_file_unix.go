//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Only selected-file identity includes times/size. Directory contents legitimately
// change, so parent identity is device/inode/owner/type/mode, not mtime/ctime.
type nativeRemoteStamp struct {
	dev, ino                             uint64
	uid                                  uint32
	mode                                 uint32
	size                                 int64
	links                                uint64
	mtimeSec, mtimeNS, ctimeSec, ctimeNS int64
}
type nativeRemoteDirectory struct {
	file                  *os.File
	name                  string
	nativeRemoteStatStamp nativeRemoteStamp
}
type nativeRemoteInput struct {
	dirs      []nativeRemoteDirectory
	file      *os.File
	name      string
	original  nativeRemoteStamp
	limit     int
	closeDone bool
	closeErr  error
}

func nativeRemoteStatFD(f *os.File) (nativeRemoteStamp, error) {
	if f == nil {
		return nativeRemoteStamp{}, errNativeRemoteUnavailable
	}
	var st unix.Stat_t
	if unix.Fstat(int(f.Fd()), &st) != nil {
		return nativeRemoteStamp{}, errNativeRemoteUnavailable
	}
	return nativeRemoteStatStamp(st), nil
}
func nativeRemoteStatAt(f *os.File, name string) (nativeRemoteStamp, error) {
	var st unix.Stat_t
	if f == nil || unix.Fstatat(int(f.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil {
		return nativeRemoteStamp{}, errNativeRemoteUnavailable
	}
	return nativeRemoteStatStamp(st), nil
}
func nativeRemoteStatStamp(st unix.Stat_t) nativeRemoteStamp {
	return nativeRemoteStamp{uint64(st.Dev), uint64(st.Ino), st.Uid, uint32(st.Mode), st.Size, uint64(st.Nlink), int64(st.Mtim.Sec), int64(st.Mtim.Nsec), int64(st.Ctim.Sec), int64(st.Ctim.Nsec)}
}
func nativeRemoteDirectorySame(a, b nativeRemoteStamp) bool {
	return a.dev == b.dev && a.ino == b.ino && a.uid == b.uid && a.mode == b.mode && a.mode&unix.S_IFMT == unix.S_IFDIR
}
func nativeRemoteInputProtected(st nativeRemoteStamp, uid uint32, limit int) bool {
	return st.mode&unix.S_IFMT == unix.S_IFREG && st.uid == uid && st.links == 1 && st.size > 0 && st.size <= int64(limit) && (st.mode&07777 == 0400 || st.mode&07777 == 0600)
}
func nativeRemoteOpenAt(parent *os.File, name string, flags int) (*os.File, error) {
	fd, e := unix.Openat(int(parent.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, errNativeRemoteUnavailable
	}
	return os.NewFile(uintptr(fd), "native-remote-input-selected"), nil
}
func selectNativeRemoteInput(ctx context.Context, path string, limit int) (*nativeRemoteInput, error) {
	if ctx == nil || limit < 1 || limit > 1<<20 || len(path) > 4096 || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 || os.Getuid() != os.Geteuid() {
		return nil, errNativeRemoteInvalid
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || len(parts) > 65 {
		return nil, errNativeRemoteInvalid
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, errNativeRemoteInvalid
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	selected := &nativeRemoteInput{name: parts[len(parts)-1], limit: limit}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return selected, errNativeRemoteUnavailable
	}
	root := os.NewFile(uintptr(fd), "native-remote-input-root")
	selected.dirs = append(selected.dirs, nativeRemoteDirectory{file: root})
	st, e := nativeRemoteStatFD(root)
	if e != nil {
		return selected, e
	}
	selected.dirs[0].nativeRemoteStatStamp = st
	current := root
	for _, name := range parts[:len(parts)-1] {
		if e := ctx.Err(); e != nil {
			return selected, e
		}
		before, e := nativeRemoteStatAt(current, name)
		if e != nil || before.mode&unix.S_IFMT != unix.S_IFDIR {
			return selected, errNativeRemoteUnavailable
		}
		next, e := nativeRemoteOpenAt(current, name, unix.O_RDONLY|unix.O_DIRECTORY)
		if e != nil {
			return selected, e
		}
		selected.dirs = append(selected.dirs, nativeRemoteDirectory{file: next, name: name})
		after, e := nativeRemoteStatFD(next)
		if e != nil || !nativeRemoteDirectorySame(before, after) {
			return selected, errNativeRemoteUnavailable
		}
		selected.dirs[len(selected.dirs)-1].nativeRemoteStatStamp = after
		current = next
	}
	parent := selected.dirs[len(selected.dirs)-1].nativeRemoteStatStamp
	uid := uint32(os.Geteuid())
	if parent.uid != uid || parent.mode&07777 != 0700 || nativeRemoteNoExtendedACL(current) != nil {
		return selected, errNativeRemoteUnavailable
	}
	before, e := nativeRemoteStatAt(current, selected.name)
	if e != nil || !nativeRemoteInputProtected(before, uid, limit) {
		return selected, errNativeRemoteUnavailable
	}
	selected.file, e = nativeRemoteOpenAt(current, selected.name, unix.O_RDONLY)
	if e != nil {
		return selected, e
	}
	after, e := nativeRemoteStatFD(selected.file)
	if e != nil || before != after || !nativeRemoteInputProtected(after, uid, limit) || nativeRemoteNoExtendedACL(selected.file) != nil {
		return selected, errNativeRemoteUnavailable
	}
	selected.original = after
	if e := selected.check(ctx); e != nil {
		return selected, e
	}
	return selected, nil
}

// read returns one owned copy of the originally selected bytes. Callers clear
// it immediately after parsing/unlock. Path replacement never reloads a secret.
func (s *nativeRemoteInput) read(ctx context.Context) ([]byte, error) {
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	bounded := make([]byte, int(s.original.size)+1)
	n, e := s.file.ReadAt(bounded, 0)
	if n != int(s.original.size) || e != io.EOF {
		clear(bounded)
		return nil, errNativeRemoteUnavailable
	}
	if e := s.check(ctx); e != nil {
		clear(bounded)
		return nil, e
	}
	return bounded[:n], nil
}

func (s *nativeRemoteInput) check(ctx context.Context) error {
	if ctx == nil || s == nil || s.closeDone || len(s.dirs) == 0 || s.file == nil {
		return errNativeRemoteUnavailable
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	for i, d := range s.dirs {
		actual, e := nativeRemoteStatFD(d.file)
		if e != nil || !nativeRemoteDirectorySame(d.nativeRemoteStatStamp, actual) {
			return errNativeRemoteIntegrity
		}
		if i > 0 {
			named, e := nativeRemoteStatAt(s.dirs[i-1].file, d.name)
			if e != nil || !nativeRemoteDirectorySame(d.nativeRemoteStatStamp, named) {
				return errNativeRemoteIntegrity
			}
		}
	}
	parent := s.dirs[len(s.dirs)-1].file
	if nativeRemoteNoExtendedACL(parent) != nil || nativeRemoteNoExtendedACL(s.file) != nil || os.Getuid() != os.Geteuid() {
		return errNativeRemoteIntegrity
	}
	actual, e := nativeRemoteStatFD(s.file)
	if e != nil || actual != s.original || !nativeRemoteInputProtected(actual, uint32(os.Geteuid()), s.limit) {
		return errNativeRemoteIntegrity
	}
	named, e := nativeRemoteStatAt(parent, s.name)
	if e != nil || named != s.original {
		return errNativeRemoteIntegrity
	}
	return ctx.Err()
}
func (s *nativeRemoteInput) close() error {
	if s == nil {
		return nil
	}
	if s.closeDone {
		return s.closeErr
	}
	s.closeDone = true
	// Attempt every owned close once, preserving static failure. Never retry a
	// raw descriptor number after an ambiguous OS close and possible fd reuse.
	if s.file != nil && s.file.Close() != nil {
		s.closeErr = errNativeRemoteCleanup
	}
	for i := len(s.dirs) - 1; i >= 0; i-- {
		if s.dirs[i].file.Close() != nil {
			s.closeErr = errNativeRemoteCleanup
		}
	}
	return s.closeErr
}
