//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

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
type remotePoolFileStamp struct {
	dev, ino                             uint64
	uid                                  uint32
	mode                                 uint32
	size                                 int64
	links                                uint64
	mtimeSec, mtimeNS, ctimeSec, ctimeNS int64
}
type remotePoolDirectoryRef struct {
	file            *os.File
	name            string
	remotePoolStamp remotePoolFileStamp
}
type remotePoolPrivateInput struct {
	dirs      []remotePoolDirectoryRef
	file      *os.File
	name      string
	original  remotePoolFileStamp
	limit     int
	closeDone bool
	closeErr  error
}

func remotePoolStatFD(f *os.File) (remotePoolFileStamp, error) {
	if f == nil {
		return remotePoolFileStamp{}, errRemotePoolCredential
	}
	var st unix.Stat_t
	if unix.Fstat(int(f.Fd()), &st) != nil {
		return remotePoolFileStamp{}, errRemotePoolCredential
	}
	return remotePoolStamp(st), nil
}
func remotePoolStatAt(f *os.File, name string) (remotePoolFileStamp, error) {
	var st unix.Stat_t
	if f == nil || unix.Fstatat(int(f.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil {
		return remotePoolFileStamp{}, errRemotePoolCredential
	}
	return remotePoolStamp(st), nil
}
func remotePoolStamp(st unix.Stat_t) remotePoolFileStamp {
	return remotePoolFileStamp{uint64(st.Dev), uint64(st.Ino), st.Uid, uint32(st.Mode), st.Size, uint64(st.Nlink), int64(st.Mtim.Sec), int64(st.Mtim.Nsec), int64(st.Ctim.Sec), int64(st.Ctim.Nsec)}
}
func remotePoolDirectorySame(a, b remotePoolFileStamp) bool {
	return a.dev == b.dev && a.ino == b.ino && a.uid == b.uid && a.mode == b.mode && a.mode&unix.S_IFMT == unix.S_IFDIR
}
func remotePoolInputProtected(st remotePoolFileStamp, uid uint32, limit int) bool {
	return st.mode&unix.S_IFMT == unix.S_IFREG && st.uid == uid && st.links == 1 && st.size > 0 && st.size <= int64(limit) && (st.mode&07777 == 0400 || st.mode&07777 == 0600)
}
func remotePoolOpenAt(parent *os.File, name string, flags int) (*os.File, error) {
	fd, e := unix.Openat(int(parent.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, errRemotePoolCredential
	}
	return os.NewFile(uintptr(fd), "remote-pool-credential-input-selected"), nil
}
func selectRemotePoolPrivateInput(ctx context.Context, path string, limit int) (*remotePoolPrivateInput, error) {
	if ctx == nil || limit < 1 || limit > 1<<20 || len(path) > 4096 || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 || os.Getuid() != os.Geteuid() {
		return nil, errRemotePoolCredential
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || len(parts) > 65 {
		return nil, errRemotePoolCredential
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, errRemotePoolCredential
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	selected := &remotePoolPrivateInput{name: parts[len(parts)-1], limit: limit}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return selected, errRemotePoolCredential
	}
	root := os.NewFile(uintptr(fd), "remote-pool-credential-input-root")
	selected.dirs = append(selected.dirs, remotePoolDirectoryRef{file: root})
	st, e := remotePoolStatFD(root)
	if e != nil {
		return selected, e
	}
	selected.dirs[0].remotePoolStamp = st
	current := root
	for _, name := range parts[:len(parts)-1] {
		if e := ctx.Err(); e != nil {
			return selected, e
		}
		before, e := remotePoolStatAt(current, name)
		if e != nil || before.mode&unix.S_IFMT != unix.S_IFDIR {
			return selected, errRemotePoolCredential
		}
		next, e := remotePoolOpenAt(current, name, unix.O_RDONLY|unix.O_DIRECTORY)
		if e != nil {
			return selected, e
		}
		selected.dirs = append(selected.dirs, remotePoolDirectoryRef{file: next, name: name})
		after, e := remotePoolStatFD(next)
		if e != nil || !remotePoolDirectorySame(before, after) {
			return selected, errRemotePoolCredential
		}
		selected.dirs[len(selected.dirs)-1].remotePoolStamp = after
		current = next
	}
	parent := selected.dirs[len(selected.dirs)-1].remotePoolStamp
	uid := uint32(os.Geteuid())
	if parent.uid != uid || parent.mode&07777 != 0700 || remotePoolNoExtendedACL(current) != nil {
		return selected, errRemotePoolCredential
	}
	before, e := remotePoolStatAt(current, selected.name)
	if e != nil || !remotePoolInputProtected(before, uid, limit) {
		return selected, errRemotePoolCredential
	}
	selected.file, e = remotePoolOpenAt(current, selected.name, unix.O_RDONLY)
	if e != nil {
		return selected, e
	}
	after, e := remotePoolStatFD(selected.file)
	if e != nil || before != after || !remotePoolInputProtected(after, uid, limit) || remotePoolNoExtendedACL(selected.file) != nil {
		return selected, errRemotePoolCredential
	}
	selected.original = after
	if e := selected.check(ctx); e != nil {
		return selected, e
	}
	return selected, nil
}

// read returns one owned copy of the originally selected bytes. Callers clear
// it immediately after parsing/unlock. Path replacement never reloads a secret.
func (s *remotePoolPrivateInput) read(ctx context.Context) ([]byte, error) {
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	bounded := make([]byte, int(s.original.size)+1)
	n, e := s.file.ReadAt(bounded, 0)
	if n != int(s.original.size) || e != io.EOF {
		clear(bounded)
		return nil, errRemotePoolCredential
	}
	if e := s.check(ctx); e != nil {
		clear(bounded)
		return nil, e
	}
	return bounded[:n], nil
}

func (s *remotePoolPrivateInput) check(ctx context.Context) error {
	if ctx == nil || s == nil || s.closeDone || len(s.dirs) == 0 || s.file == nil {
		return errRemotePoolCredential
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	for i, d := range s.dirs {
		actual, e := remotePoolStatFD(d.file)
		if e != nil || !remotePoolDirectorySame(d.remotePoolStamp, actual) {
			return errRemotePoolCredentialChanged
		}
		if i > 0 {
			named, e := remotePoolStatAt(s.dirs[i-1].file, d.name)
			if e != nil || !remotePoolDirectorySame(d.remotePoolStamp, named) {
				return errRemotePoolCredentialChanged
			}
		}
	}
	parent := s.dirs[len(s.dirs)-1].file
	if remotePoolNoExtendedACL(parent) != nil || remotePoolNoExtendedACL(s.file) != nil || os.Getuid() != os.Geteuid() {
		return errRemotePoolCredentialChanged
	}
	actual, e := remotePoolStatFD(s.file)
	if e != nil || actual != s.original || !remotePoolInputProtected(actual, uint32(os.Geteuid()), s.limit) {
		return errRemotePoolCredentialChanged
	}
	named, e := remotePoolStatAt(parent, s.name)
	if e != nil || named != s.original {
		return errRemotePoolCredentialChanged
	}
	return ctx.Err()
}
func (s *remotePoolPrivateInput) close() error {
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
		s.closeErr = errRemotePoolCredentialCleanup
	}
	for i := len(s.dirs) - 1; i >= 0; i-- {
		if s.dirs[i].file.Close() != nil {
			s.closeErr = errRemotePoolCredentialCleanup
		}
	}
	return s.closeErr
}
