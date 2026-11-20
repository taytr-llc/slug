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
type remoteLaunchStamp struct {
	dev, ino                             uint64
	uid                                  uint32
	mode                                 uint32
	size                                 int64
	links                                uint64
	mtimeSec, mtimeNS, ctimeSec, ctimeNS int64
}
type remoteLaunchDirectory struct {
	file                  *os.File
	name                  string
	remoteLaunchStatStamp remoteLaunchStamp
}
type remoteLaunchInput struct {
	dirs      []remoteLaunchDirectory
	file      *os.File
	name      string
	original  remoteLaunchStamp
	limit     int
	closeDone bool
	closeErr  error
}

func remoteLaunchStatFD(f *os.File) (remoteLaunchStamp, error) {
	if f == nil {
		return remoteLaunchStamp{}, errRemoteLaunchUnavailable
	}
	var st unix.Stat_t
	if unix.Fstat(int(f.Fd()), &st) != nil {
		return remoteLaunchStamp{}, errRemoteLaunchUnavailable
	}
	return remoteLaunchStatStamp(st), nil
}
func remoteLaunchStatAt(f *os.File, name string) (remoteLaunchStamp, error) {
	var st unix.Stat_t
	if f == nil || unix.Fstatat(int(f.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil {
		return remoteLaunchStamp{}, errRemoteLaunchUnavailable
	}
	return remoteLaunchStatStamp(st), nil
}
func remoteLaunchStatStamp(st unix.Stat_t) remoteLaunchStamp {
	return remoteLaunchStamp{uint64(st.Dev), uint64(st.Ino), st.Uid, uint32(st.Mode), st.Size, uint64(st.Nlink), int64(st.Mtim.Sec), int64(st.Mtim.Nsec), int64(st.Ctim.Sec), int64(st.Ctim.Nsec)}
}
func remoteLaunchDirectorySame(a, b remoteLaunchStamp) bool {
	return a.dev == b.dev && a.ino == b.ino && a.uid == b.uid && a.mode == b.mode && a.mode&unix.S_IFMT == unix.S_IFDIR
}
func remoteLaunchInputProtected(st remoteLaunchStamp, uid uint32, limit int) bool {
	return st.mode&unix.S_IFMT == unix.S_IFREG && st.uid == uid && st.links == 1 && st.size > 0 && st.size <= int64(limit) && (st.mode&07777 == 0400 || st.mode&07777 == 0600)
}
func remoteLaunchOpenAt(parent *os.File, name string, flags int) (*os.File, error) {
	fd, e := unix.Openat(int(parent.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, errRemoteLaunchUnavailable
	}
	return os.NewFile(uintptr(fd), "remote-launch-input-selected"), nil
}
func selectRemoteLaunchInput(ctx context.Context, path string, limit int) (*remoteLaunchInput, error) {
	if ctx == nil || limit < 1 || limit > 1<<20 || len(path) > 4096 || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 || os.Getuid() != os.Geteuid() {
		return nil, errRemoteLaunchInvalid
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || len(parts) > 65 {
		return nil, errRemoteLaunchInvalid
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, errRemoteLaunchInvalid
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	selected := &remoteLaunchInput{name: parts[len(parts)-1], limit: limit}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return selected, errRemoteLaunchUnavailable
	}
	root := os.NewFile(uintptr(fd), "remote-launch-input-root")
	selected.dirs = append(selected.dirs, remoteLaunchDirectory{file: root})
	st, e := remoteLaunchStatFD(root)
	if e != nil {
		return selected, e
	}
	selected.dirs[0].remoteLaunchStatStamp = st
	current := root
	for _, name := range parts[:len(parts)-1] {
		if e := ctx.Err(); e != nil {
			return selected, e
		}
		before, e := remoteLaunchStatAt(current, name)
		if e != nil || before.mode&unix.S_IFMT != unix.S_IFDIR {
			return selected, errRemoteLaunchUnavailable
		}
		next, e := remoteLaunchOpenAt(current, name, unix.O_RDONLY|unix.O_DIRECTORY)
		if e != nil {
			return selected, e
		}
		selected.dirs = append(selected.dirs, remoteLaunchDirectory{file: next, name: name})
		after, e := remoteLaunchStatFD(next)
		if e != nil || !remoteLaunchDirectorySame(before, after) {
			return selected, errRemoteLaunchUnavailable
		}
		selected.dirs[len(selected.dirs)-1].remoteLaunchStatStamp = after
		current = next
	}
	parent := selected.dirs[len(selected.dirs)-1].remoteLaunchStatStamp
	uid := uint32(os.Geteuid())
	if parent.uid != uid || parent.mode&07777 != 0700 || remoteLaunchNoExtendedACL(current) != nil {
		return selected, errRemoteLaunchUnavailable
	}
	before, e := remoteLaunchStatAt(current, selected.name)
	if e != nil || !remoteLaunchInputProtected(before, uid, limit) {
		return selected, errRemoteLaunchUnavailable
	}
	selected.file, e = remoteLaunchOpenAt(current, selected.name, unix.O_RDONLY)
	if e != nil {
		return selected, e
	}
	after, e := remoteLaunchStatFD(selected.file)
	if e != nil || before != after || !remoteLaunchInputProtected(after, uid, limit) || remoteLaunchNoExtendedACL(selected.file) != nil {
		return selected, errRemoteLaunchUnavailable
	}
	selected.original = after
	if e := selected.check(ctx); e != nil {
		return selected, e
	}
	return selected, nil
}

// read returns one owned copy of the originally selected bytes. Callers clear
// it immediately after parsing/unlock. Path replacement never reloads a secret.
func (s *remoteLaunchInput) read(ctx context.Context) ([]byte, error) {
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	bounded := make([]byte, int(s.original.size)+1)
	n, e := s.file.ReadAt(bounded, 0)
	if n != int(s.original.size) || e != io.EOF {
		clear(bounded)
		return nil, errRemoteLaunchUnavailable
	}
	if e := s.check(ctx); e != nil {
		clear(bounded)
		return nil, e
	}
	return bounded[:n], nil
}

func (s *remoteLaunchInput) check(ctx context.Context) error {
	if ctx == nil || s == nil || s.closeDone || len(s.dirs) == 0 || s.file == nil {
		return errRemoteLaunchUnavailable
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	for i, d := range s.dirs {
		actual, e := remoteLaunchStatFD(d.file)
		if e != nil || !remoteLaunchDirectorySame(d.remoteLaunchStatStamp, actual) {
			return errRemoteLaunchIntegrity
		}
		if i > 0 {
			named, e := remoteLaunchStatAt(s.dirs[i-1].file, d.name)
			if e != nil || !remoteLaunchDirectorySame(d.remoteLaunchStatStamp, named) {
				return errRemoteLaunchIntegrity
			}
		}
	}
	parent := s.dirs[len(s.dirs)-1].file
	if remoteLaunchNoExtendedACL(parent) != nil || remoteLaunchNoExtendedACL(s.file) != nil || os.Getuid() != os.Geteuid() {
		return errRemoteLaunchIntegrity
	}
	actual, e := remoteLaunchStatFD(s.file)
	if e != nil || actual != s.original || !remoteLaunchInputProtected(actual, uint32(os.Geteuid()), s.limit) {
		return errRemoteLaunchIntegrity
	}
	named, e := remoteLaunchStatAt(parent, s.name)
	if e != nil || named != s.original {
		return errRemoteLaunchIntegrity
	}
	return ctx.Err()
}
func (s *remoteLaunchInput) close() error {
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
		s.closeErr = errRemoteLaunchCleanup
	}
	for i := len(s.dirs) - 1; i >= 0; i-- {
		if s.dirs[i].file.Close() != nil {
			s.closeErr = errRemoteLaunchCleanup
		}
	}
	return s.closeErr
}
