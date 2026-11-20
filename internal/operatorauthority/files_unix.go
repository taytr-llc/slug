//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package operatorauthority

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Only seed-file identity includes times/size. Directory contents legitimately
// change, so parent identity is device/inode/owner/type/mode, not mtime/ctime.
type fileStamp struct {
	dev, ino                             uint64
	uid                                  uint32
	mode                                 uint32
	size                                 int64
	links                                uint64
	mtimeSec, mtimeNS, ctimeSec, ctimeNS int64
}
type directoryRef struct {
	file  *os.File
	name  string
	stamp fileStamp
}
type selectedSeed struct {
	dirs      []directoryRef
	file      *os.File
	name      string
	original  fileStamp
	closeDone bool
	closeErr  error
}

func statFD(f *os.File) (fileStamp, error) {
	if f == nil {
		return fileStamp{}, ErrUnavailable
	}
	var st unix.Stat_t
	if unix.Fstat(int(f.Fd()), &st) != nil {
		return fileStamp{}, ErrUnavailable
	}
	return stamp(st), nil
}
func statAt(f *os.File, name string) (fileStamp, error) {
	var st unix.Stat_t
	if f == nil || unix.Fstatat(int(f.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil {
		return fileStamp{}, ErrUnavailable
	}
	return stamp(st), nil
}
func stamp(st unix.Stat_t) fileStamp {
	return fileStamp{uint64(st.Dev), uint64(st.Ino), st.Uid, uint32(st.Mode), st.Size, uint64(st.Nlink), int64(st.Mtim.Sec), int64(st.Mtim.Nsec), int64(st.Ctim.Sec), int64(st.Ctim.Nsec)}
}
func directorySame(a, b fileStamp) bool {
	return a.dev == b.dev && a.ino == b.ino && a.uid == b.uid && a.mode == b.mode && a.mode&unix.S_IFMT == unix.S_IFDIR
}
func seedProtected(st fileStamp, uid uint32) bool {
	return st.mode&unix.S_IFMT == unix.S_IFREG && st.uid == uid && st.links == 1 && st.size == 32 && (st.mode&07777 == 0400 || st.mode&07777 == 0600)
}
func openAt(parent *os.File, name string, flags int) (*os.File, error) {
	fd, e := unix.Openat(int(parent.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, ErrUnavailable
	}
	return os.NewFile(uintptr(fd), "operator-authority-selected"), nil
}
func loadSeed(ctx context.Context, path string) (*selectedSeed, [32]byte, error) {
	var seed [32]byte
	if ctx == nil || len(path) > 4096 || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 || os.Getuid() != os.Geteuid() {
		return nil, seed, ErrInvalid
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || len(parts) > 65 {
		return nil, seed, ErrInvalid
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, seed, ErrInvalid
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, seed, e
	}
	selected := &selectedSeed{name: parts[len(parts)-1]}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return selected, seed, ErrUnavailable
	}
	root := os.NewFile(uintptr(fd), "operator-authority-root")
	selected.dirs = append(selected.dirs, directoryRef{file: root})
	st, e := statFD(root)
	if e != nil {
		return selected, seed, e
	}
	selected.dirs[0].stamp = st
	current := root
	for _, name := range parts[:len(parts)-1] {
		if e := ctx.Err(); e != nil {
			return selected, seed, e
		}
		before, e := statAt(current, name)
		if e != nil || before.mode&unix.S_IFMT != unix.S_IFDIR {
			return selected, seed, ErrUnavailable
		}
		next, e := openAt(current, name, unix.O_RDONLY|unix.O_DIRECTORY)
		if e != nil {
			return selected, seed, e
		}
		selected.dirs = append(selected.dirs, directoryRef{file: next, name: name})
		after, e := statFD(next)
		if e != nil || !directorySame(before, after) {
			return selected, seed, ErrUnavailable
		}
		selected.dirs[len(selected.dirs)-1].stamp = after
		current = next
	}
	parent := selected.dirs[len(selected.dirs)-1].stamp
	uid := uint32(os.Geteuid())
	if parent.uid != uid || parent.mode&07777 != 0700 || noExtendedACL(current) != nil {
		return selected, seed, ErrUnavailable
	}
	before, e := statAt(current, selected.name)
	if e != nil || !seedProtected(before, uid) {
		return selected, seed, ErrUnavailable
	}
	selected.file, e = openAt(current, selected.name, unix.O_RDONLY)
	if e != nil {
		return selected, seed, e
	}
	after, e := statFD(selected.file)
	if e != nil || before != after || !seedProtected(after, uid) || noExtendedACL(selected.file) != nil {
		return selected, seed, ErrUnavailable
	}
	selected.original = after
	var bounded [33]byte
	// Regular file and exact size were checked before reading. The 33rd byte
	// rejects concurrent growth/trailing newline without an unbounded allocation.
	n, re := io.ReadFull(selected.file, bounded[:])
	if n != 32 || re != io.ErrUnexpectedEOF {
		clear(bounded[:])
		return selected, seed, ErrUnavailable
	}
	copy(seed[:], bounded[:32])
	clear(bounded[:])
	if e := selected.check(ctx); e != nil {
		clear(seed[:])
		return selected, seed, e
	}
	return selected, seed, nil
}
func (s *selectedSeed) check(ctx context.Context) error {
	if ctx == nil || s == nil || s.closeDone || len(s.dirs) == 0 || s.file == nil {
		return ErrUnavailable
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	for i, d := range s.dirs {
		actual, e := statFD(d.file)
		if e != nil || !directorySame(d.stamp, actual) {
			return ErrIntegrity
		}
		if i > 0 {
			named, e := statAt(s.dirs[i-1].file, d.name)
			if e != nil || !directorySame(d.stamp, named) {
				return ErrIntegrity
			}
		}
	}
	parent := s.dirs[len(s.dirs)-1].file
	if noExtendedACL(parent) != nil || noExtendedACL(s.file) != nil || os.Getuid() != os.Geteuid() {
		return ErrIntegrity
	}
	actual, e := statFD(s.file)
	if e != nil || actual != s.original || !seedProtected(actual, uint32(os.Geteuid())) {
		return ErrIntegrity
	}
	named, e := statAt(parent, s.name)
	if e != nil || named != s.original {
		return ErrIntegrity
	}
	return ctx.Err()
}
func (s *selectedSeed) close() error {
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
		s.closeErr = ErrCleanup
	}
	for i := len(s.dirs) - 1; i >= 0; i-- {
		if s.dirs[i].file.Close() != nil {
			s.closeErr = ErrCleanup
		}
	}
	return s.closeErr
}
