//go:build darwin || linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/taytr-llc/slugwallet/wallet"
	"golang.org/x/sys/unix"
)

func restoredSupported() bool { return true }

type restoredRoot struct {
	path      string
	dir, lock *os.File
	stamp     os.FileInfo
	closed    bool
	closeErr  error
}

// The namespace is retained by descriptor. Subordinate databases still audit
// their own concrete files; every path-based call is bracketed by check().
func restoredWalkDir(path string) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 4096 || len(strings.Split(path, string(filepath.Separator))) > 65 {
		return nil, ErrRestoredGymStorage
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrRestoredGymStorage
	}
	f := os.NewFile(uintptr(fd), "/")
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		fd, e = unix.Openat(int(f.Fd()), part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		ce := restoredClose(nil, f)
		if e != nil || ce != nil {
			cause := errors.Join(ErrRestoredGymStorage, ce)
			if e == nil {
				cause = restoredClose(cause, os.NewFile(uintptr(fd), part))
			}
			return nil, cause
		}
		f = os.NewFile(uintptr(fd), part)
	}
	return f, nil
}
func openRestoredRoot(path string, create bool) (*restoredRoot, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrRestoredGymStorage
	}
	if create {
		parent, e := restoredWalkDir(filepath.Dir(path))
		if e != nil {
			return nil, e
		}
		e = unix.Mkdirat(int(parent.Fd()), filepath.Base(path), 0700)
		if e != nil && !errors.Is(e, unix.EEXIST) {
			return nil, restoredClose(ErrRestoredGymStorage, parent)
		}
		if e = restoredClose(parent.Sync(), parent); e != nil {
			return nil, errors.Join(ErrRestoredGymStorage, e)
		}
	}
	dir, e := restoredWalkDir(path)
	if e != nil {
		return nil, e
	}
	r := &restoredRoot{path: path, dir: dir}
	r.stamp, e = dir.Stat()
	if e != nil || r.stamp.Mode().Perm() != 0700 {
		return nil, restoredClose(ErrRestoredGymStorage, dir)
	}
	if create {
		entries, readErr := dir.ReadDir(2)
		if readErr != nil && !errors.Is(readErr, io.EOF) || len(entries) > 1 || len(entries) == 1 && entries[0].Name() != "LOCK" {
			return nil, restoredClose(ErrRestoredGymStorage, dir)
		}
	}
	flags := unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC
	if create {
		flags |= unix.O_CREAT
	}
	fd, e := unix.Openat(int(dir.Fd()), "LOCK", flags, 0600)
	if e != nil {
		return nil, restoredClose(ErrRestoredGymStorage, dir)
	}
	r.lock = os.NewFile(uintptr(fd), "LOCK")
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Mode&0777 != 0600 || st.Size != 0 || unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		return nil, restoredClose(ErrRestoredGymStorage, r.lock, dir)
	}
	if e = errors.Join(r.lock.Sync(), dir.Sync(), r.check()); e != nil {
		return nil, restoredClose(errors.Join(ErrRestoredGymStorage, e), r)
	}
	return r, nil
}
func (r *restoredRoot) check() error {
	if r == nil || r.closed {
		return ErrRestoredGymStorage
	}
	d, e := restoredWalkDir(r.path)
	if e != nil {
		return e
	}
	st, se := d.Stat()
	ce := restoredClose(nil, d)
	if se != nil || ce != nil || !os.SameFile(st, r.stamp) {
		return errors.Join(ErrRestoredGymStorage, ce)
	}
	var retained, current unix.Stat_t
	if unix.Fstat(int(r.lock.Fd()), &retained) != nil || unix.Fstatat(int(r.dir.Fd()), "LOCK", &current, unix.AT_SYMLINK_NOFOLLOW) != nil || retained.Dev != current.Dev || retained.Ino != current.Ino || current.Nlink != 1 || current.Mode&unix.S_IFMT != unix.S_IFREG {
		return ErrRestoredGymStorage
	}
	return nil
}
func (r *restoredRoot) inventory() (map[string]bool, error) {
	if e := r.check(); e != nil {
		return nil, e
	}
	fd, e := unix.Openat(int(r.dir.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrRestoredGymStorage
	}
	f := os.NewFile(uintptr(fd), "root inventory")
	entries, e := f.ReadDir(8)
	ce := restoredClose(nil, f)
	if e != nil && !errors.Is(e, io.EOF) || ce != nil || len(entries) > 7 {
		return nil, errors.Join(ErrRestoredGymStorage, ce)
	}
	allowed := map[string]bool{"LOCK": false, restoredInitName: false, restoredActivateName: false, restoredReadyName: false, "wallet": true, "carrier": true, "ownership": true}
	out := map[string]bool{}
	for _, entry := range entries {
		directory, ok := allowed[entry.Name()]
		if !ok {
			return nil, ErrRestoredGymStorage
		}
		var st unix.Stat_t
		if unix.Fstatat(int(r.dir.Fd()), entry.Name(), &st, unix.AT_SYMLINK_NOFOLLOW) != nil {
			return nil, ErrRestoredGymStorage
		}
		if directory {
			if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0777 != 0700 {
				return nil, ErrRestoredGymStorage
			}
		} else if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Mode&0777 != 0600 {
			return nil, ErrRestoredGymStorage
		}
		out[entry.Name()] = true
	}
	return out, nil
}
func (r *restoredRoot) Close() error {
	if r == nil {
		return nil
	}
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	r.closeErr = restoredClose(nil, r.lock, r.dir)
	return r.closeErr
}

// readRestoredSelected never follows a file or ancestor symlink, and requires
// a single-link regular file whose descriptor metadata is stable around read.
// The returned bytes are caller-owned and cleared after parsing sensitive files.
func readRestoredSelected(ctx context.Context, path, root string, limit int) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || limit < 1 || limit > 65536 || !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 2048 || root != "" && (path == root || withinDir(path, root) || withinDir(root, path)) {
		return nil, ErrRestoredGymStorage
	}
	d, e := restoredWalkDir(filepath.Dir(path))
	if e != nil {
		return nil, e
	}
	fd, e := unix.Openat(int(d.Fd()), filepath.Base(path), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, restoredClose(ErrRestoredGymStorage, d)
	}
	f := os.NewFile(uintptr(fd), "selected input")
	var before, after unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Size < 1 || before.Size > int64(limit) {
		return nil, restoredClose(ErrRestoredGymStorage, f, d)
	}
	raw := make([]byte, int(before.Size))
	_, e = io.ReadFull(f, raw)
	var extra [1]byte
	n, tail := f.Read(extra[:])
	se := unix.Fstat(fd, &after)
	var named unix.Stat_t
	ne := unix.Fstatat(int(d.Fd()), filepath.Base(path), &named, unix.AT_SYMLINK_NOFOLLOW)
	ce := restoredClose(nil, f, d)
	if e != nil || n != 0 || tail != io.EOF || se != nil || ne != nil || ce != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || before.Mtim != after.Mtim || after.Nlink != 1 || named.Dev != after.Dev || named.Ino != after.Ino || named.Nlink != 1 || ctx.Err() != nil {
		clear(raw)
		return nil, errors.Join(ErrRestoredGymStorage, ce)
	}
	return raw, nil
}

// walletStatus audits the fixed one-file child before any Loader effect.
func (r *restoredRoot) walletStatus(create bool) (exists bool, resultErr error) {
	if e := r.check(); e != nil {
		return false, e
	}
	if create {
		e := unix.Mkdirat(int(r.dir.Fd()), "wallet", 0700)
		if e != nil && !errors.Is(e, unix.EEXIST) {
			return false, ErrRestoredGymStorage
		}
		if r.dir.Sync() != nil {
			return false, ErrRestoredGymStorage
		}
	}
	fd, e := unix.Openat(int(r.dir.Fd()), "wallet", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(e, unix.ENOENT) && !create {
		return false, nil
	}
	if e != nil {
		return false, ErrRestoredGymStorage
	}
	d := os.NewFile(uintptr(fd), "wallet inventory")
	defer func() { resultErr = restoredClose(resultErr, d) }()
	st, e := d.Stat()
	if e != nil || st.Mode().Perm() != 0700 {
		return false, ErrRestoredGymStorage
	}
	entries, e := d.ReadDir(2)
	if e != nil && !errors.Is(e, io.EOF) || len(entries) > 1 {
		return false, ErrRestoredGymStorage
	}
	if len(entries) == 0 {
		return false, nil
	}
	if entries[0].Name() != wallet.WalletDBName {
		return false, ErrRestoredGymStorage
	}
	var info unix.Stat_t
	if unix.Fstatat(fd, wallet.WalletDBName, &info, unix.AT_SYMLINK_NOFOLLOW) != nil || info.Mode&unix.S_IFMT != unix.S_IFREG || info.Nlink != 1 || info.Mode&0777 != 0600 || info.Size == 0 {
		return false, ErrRestoredGymStorage
	}
	return true, r.check()
}
