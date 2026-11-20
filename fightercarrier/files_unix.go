//go:build darwin || linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type fileStamp struct {
	Dev, Ino        uint64
	Size, Sec, Nsec int64
	Mode, UID       uint32
	Links           uint64
}
type dirChain struct {
	dirs   []*os.File
	names  []string
	stamps []fileStamp
}

func supportedPlatform() bool { return true }
func stamp(f *os.File) (fileStamp, error) {
	var s unix.Stat_t
	e := unix.Fstat(int(f.Fd()), &s)
	return statValue(s), e
}
func statValue(s unix.Stat_t) fileStamp {
	return fileStamp{uint64(s.Dev), uint64(s.Ino), s.Size, s.Mtim.Sec, s.Mtim.Nsec, uint32(s.Mode), s.Uid, uint64(s.Nlink)}
}
func relativeStamp(d *os.File, n string) (fileStamp, error) {
	var s unix.Stat_t
	e := unix.Fstatat(int(d.Fd()), n, &s, unix.AT_SYMLINK_NOFOLLOW)
	return statValue(s), e
}
func sameFile(a, b fileStamp) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.UID == b.UID
}
func privateFile(s fileStamp) bool {
	return s.Mode&unix.S_IFMT == unix.S_IFREG && s.Mode&0777 == 0600 && s.UID == uint32(os.Getuid()) && s.Links == 1
}
func openAt(d *os.File, n string, flags int) (*os.File, error) {
	fd, e := unix.Openat(int(d.Fd()), n, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(fd), n), nil
}
func createAt(d *os.File, n string) (*os.File, error) {
	return openAt(d, n, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL)
}
func readAt(d *os.File, n string, limit int) (*os.File, []byte, fileStamp, error) {
	f, e := openAt(d, n, unix.O_RDONLY)
	if e != nil {
		return nil, nil, fileStamp{}, e
	}
	s, e := stamp(f)
	if e != nil || !privateFile(s) || s.Size < 1 || s.Size > int64(limit) {
		f.Close()
		return nil, nil, fileStamp{}, ErrCorrupt
	}
	b, e := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	after, se := stamp(f)
	at, ae := relativeStamp(d, n)
	if e != nil || se != nil || ae != nil || after != s || at != s || len(b) != int(s.Size) {
		f.Close()
		return nil, nil, fileStamp{}, ErrCorrupt
	}
	return f, b, s, nil
}
func renameAt(d *os.File, old, n string) error {
	return unix.Renameat(int(d.Fd()), old, int(d.Fd()), n)
}
func removeAt(d *os.File, n string) error { return unix.Unlinkat(int(d.Fd()), n, 0) }
func lockAt(d *os.File, create bool) (*os.File, error) {
	flags := unix.O_RDWR
	if create {
		flags |= unix.O_CREAT
	}
	f, e := openAt(d, "LOCK", flags)
	if e != nil {
		return nil, e
	}
	s, e := stamp(f)
	if e != nil || !privateFile(s) {
		f.Close()
		return nil, ErrCorrupt
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, ErrBusy
	}
	return f, nil
}
func pinDirectory(path string, create bool) (result *dirChain, err error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrInvalid
	}
	if fi, e := os.Lstat(path); e == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, ErrInvalid
	} else if e != nil && !os.IsNotExist(e) {
		return nil, ErrStorage
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(path))
	if e != nil {
		return nil, ErrStorage
	}
	path = filepath.Join(parent, filepath.Base(path))
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) > 64 {
		return nil, ErrInvalid
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return nil, ErrStorage
	}
	c := &dirChain{dirs: []*os.File{os.NewFile(uintptr(fd), "/")}}
	defer func() {
		if err != nil {
			_ = c.close()
		}
	}()
	st, e := stamp(c.dirs[0])
	if e != nil {
		return nil, ErrStorage
	}
	c.stamps = append(c.stamps, st)
	for i, n := range parts {
		p := c.dirs[len(c.dirs)-1]
		f, e := openAt(p, n, unix.O_RDONLY|unix.O_DIRECTORY)
		if e != nil && create && i == len(parts)-1 && errors.Is(e, unix.ENOENT) {
			if e = unix.Mkdirat(int(p.Fd()), n, 0700); e == nil {
				e = p.Sync()
			}
			if e != nil {
				return nil, ErrStorage
			}
			f, e = openAt(p, n, unix.O_RDONLY|unix.O_DIRECTORY)
		}
		if e != nil {
			return nil, ErrStorage
		}
		c.dirs = append(c.dirs, f)
		c.names = append(c.names, n)
		st, e := stamp(f)
		if e != nil {
			return nil, ErrStorage
		}
		c.stamps = append(c.stamps, st)
	}
	last := c.stamps[len(c.stamps)-1]
	if last.Mode&unix.S_IFMT != unix.S_IFDIR || last.Mode&0777 != 0700 || last.UID != uint32(os.Getuid()) {
		return nil, ErrCorrupt
	}
	if e = c.check(); e != nil {
		return nil, e
	}
	return c, nil
}
func (c *dirChain) dir() *os.File { return c.dirs[len(c.dirs)-1] }
func (c *dirChain) check() error {
	for i, f := range c.dirs {
		s, e := stamp(f)
		if e != nil || !sameFile(s, c.stamps[i]) {
			return ErrStorage
		}
		if i > 0 {
			s, e = relativeStamp(c.dirs[i-1], c.names[i-1])
			if e != nil || !sameFile(s, c.stamps[i]) {
				return ErrStorage
			}
		}
	}
	return nil
}
func (c *dirChain) close() error {
	var err error
	for i := len(c.dirs) - 1; i >= 0; i-- {
		err = errors.Join(err, c.dirs[i].Close())
	}
	return err
}
