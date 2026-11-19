//go:build darwin || linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package recoveryfile

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func supportedPlatform() bool { return true }
func fromStat(s unix.Stat_t) fileStamp {
	n := new(big.Int).Mul(big.NewInt(s.Mtim.Sec), big.NewInt(1000000000))
	n.Add(n, big.NewInt(s.Mtim.Nsec))
	return fileStamp{identity: Identity{fmt.Sprintf("%016x", uint64(s.Dev)), fmt.Sprintf("%016x", uint64(s.Ino))}, size: s.Size, modified: n.String(), regular: s.Mode&unix.S_IFMT == unix.S_IFREG, directory: s.Mode&unix.S_IFMT == unix.S_IFDIR}
}
func stampFile(f *os.File) (fileStamp, error) {
	var st unix.Stat_t
	if e := unix.Fstat(int(f.Fd()), &st); e != nil {
		return fileStamp{}, e
	}
	return fromStat(st), nil
}
func inspectRelative(dir *os.File, name string) (fileStamp, error) {
	var st unix.Stat_t
	if e := unix.Fstatat(int(dir.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW); e != nil {
		return fileStamp{}, e
	}
	return fromStat(st), nil
}
func openRelative(dir *os.File, name string, flags int, mode uint32) (*os.File, error) {
	fd, e := unix.Openat(int(dir.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC, mode)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(fd), name), nil
}
func openReadRelative(dir *os.File, name string) (*os.File, error) {
	return openRelative(dir, name, unix.O_RDONLY|unix.O_NONBLOCK, 0)
}
func createRelative(dir *os.File, name string) (*os.File, error) {
	return openRelative(dir, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0600)
}
func linkRelative(dir *os.File, old, name string) error {
	return unix.Linkat(int(dir.Fd()), old, int(dir.Fd()), name, 0)
}
func removeRelative(dir *os.File, name string) error { return unix.Unlinkat(int(dir.Fd()), name, 0) }

var managedMarkers = []string{"SLUG-PILOT.json", "SLUG-PILOT-READY.json", "SLUG-PILOT-CHAINS.json", "SLUG-ACCOUNTS.json", "SLUG-MANIFEST-PILOT.json", "SLUG-MANIFEST-PILOT-READY.json", "SLUG-MANIFEST-PILOT-CHAINS.json", "SLUG-MANIFEST-ACCOUNTS.json", "SLUG-POOL-ACCOUNTS.json", "SLUG-POOL-RUNTIME.json"}

func managedDirectory(dir *os.File, base string) bool {
	switch base {
	case "LocalPilot", "LocalWalletPilot", "LocalAccountPilot", "LocalManifestPilot", "LocalPoolPilot":
		return true
	}
	for _, name := range managedMarkers {
		if _, e := inspectRelative(dir, name); !os.IsNotExist(e) {
			return true
		}
	}
	return false
}
func walkDirectory(path string, checkManaged bool) (*os.File, []Identity, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(strings.Split(path, string(filepath.Separator))) > 65 {
		return nil, nil, ErrInvalid
	}
	fd, e := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, nil, e
	}
	current := os.NewFile(uintptr(fd), string(filepath.Separator))
	var ancestors []Identity
	inspect := func(base string) error {
		st, e := stampFile(current)
		if e != nil || !st.directory {
			return ErrInvalid
		}
		if checkManaged && managedDirectory(current, base) {
			return ErrInvalid
		}
		ancestors = append(ancestors, st.identity)
		return nil
	}
	if e = inspect(string(filepath.Separator)); e != nil {
		_ = current.Close()
		return nil, nil, e
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		next, e := openRelative(current, part, unix.O_RDONLY|unix.O_DIRECTORY, 0)
		ce := current.Close()
		if e != nil || ce != nil {
			if next != nil {
				_ = next.Close()
			}
			if e != nil {
				return nil, nil, e
			}
			return nil, nil, ce
		}
		current = next
		if e = inspect(part); e != nil {
			_ = current.Close()
			return nil, nil, e
		}
	}
	return current, ancestors, nil
}
