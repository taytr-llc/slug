//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/taytr-llc/slugwallet/wallet"
	"golang.org/x/sys/unix"
)

type provisionFiles struct {
	version uint32
	base    *operatorFiles // reuse retained descriptor and sticky close leaves only
	path    string
	inputs  map[string]*privateInput
	changed bool
}

func openProvisionFiles(path string, create bool) (*provisionFiles, error) {
	return openProvisionFilesVersion(path, create, 1)
}
func openProvisionFilesVersion(path string, create bool, version uint32) (f *provisionFiles, err error) {
	if (version != 1 && version != 2) || !commandPath(path) || os.Getuid() != os.Geteuid() {
		return nil, ErrProvisionInvalid
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || len(parts) > 65 {
		return nil, ErrProvisionInvalid
	}
	f = &provisionFiles{version: version, base: &operatorFiles{path: path, children: map[string]directoryRef{}}, path: path, inputs: map[string]*privateInput{}}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return f, ErrUnavailable
	}
	current := os.NewFile(uintptr(fd), "provision-root")
	f.base.dirs = append(f.base.dirs, directoryRef{file: current})
	st, e := statFD(current)
	if e != nil {
		return f, e
	}
	f.base.dirs[0].stamp = st
	for i, name := range parts {
		if name == "" || name == "." || name == ".." {
			return f, ErrProvisionInvalid
		}
		var st unix.Stat_t
		e = unix.Fstatat(int(current.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
		last := i == len(parts)-1
		if last && create {
			if !errors.Is(e, unix.ENOENT) {
				return f, ErrProvisionIncomplete
			}
			if unix.Mkdirat(int(current.Fd()), name, 0700) != nil {
				return f, ErrUnavailable
			}
			f.changed = true
			if current.Sync() != nil {
				return f, ErrUnavailable
			}
		} else if e != nil {
			return f, ErrUnavailable
		}
		before, e := statAt(current, name)
		if e != nil || before.mode&unix.S_IFMT != unix.S_IFDIR {
			return f, ErrIntegrity
		}
		next, e := openAt(current, name, unix.O_RDONLY|unix.O_DIRECTORY)
		if e != nil {
			return f, e
		}
		f.base.dirs = append(f.base.dirs, directoryRef{file: next, name: name, stamp: before})
		after, e := statFD(next)
		if e != nil || !directorySame(before, after) {
			return f, ErrIntegrity
		}
		current = next
	}
	st, e = statFD(current)
	if e != nil || st.uid != uint32(os.Geteuid()) || st.mode&07777 != 0700 || noExtendedACL(current) != nil {
		return f, ErrIntegrity
	}
	if unix.Flock(int(current.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return f, ErrUnavailable
	}
	if e = f.inventory(); e != nil {
		return f, e
	}
	if create {
		names, e := f.names("")
		if e != nil || len(names) != 0 {
			return f, ErrIntegrity
		}
	}
	return f, nil
}
func provisionAllowed(dir, name string) bool { return provisionAllowedVersion(dir, name, 1) }
func provisionAllowedVersion(dir, name string, version uint32) bool {
	switch dir {
	case "":
		return name == provisionIntentFile(version) || name == provisionReadyFile(version) || name == "offline" || name == "online" || name == "public" || name == "wallet"
	case "offline":
		return name == "material-v1.bin" || name == provisionBackupFile(version)
	case "online":
		return name == "admission.seed" || name == "publication.seed" || name == "cutoff.seed"
	case "public":
		return name == "setup.json"
	case "wallet":
		return name == wallet.WalletDBName
	}
	return false
}
func (f *provisionFiles) directory(name string) *os.File {
	if name == "" {
		return f.base.root()
	}
	return f.base.children[name].file
}
func (f *provisionFiles) names(dir string) (map[string]bool, error) {
	base := f.directory(dir)
	if base == nil {
		return nil, ErrProvisionIncomplete
	}
	d, e := openAt(base, ".", unix.O_RDONLY|unix.O_DIRECTORY)
	if e != nil {
		return nil, e
	}
	rows, e := d.ReadDir(8)
	ce := f.base.closeTemporary(d)
	if ce != nil {
		return nil, ce
	}
	if e != nil && !errors.Is(e, io.EOF) || len(rows) > 6 {
		return nil, ErrIntegrity
	}
	found := map[string]bool{}
	for _, row := range rows {
		name := row.Name()
		if !provisionAllowedVersion(dir, name, f.version) || row.Type()&os.ModeSymlink != 0 {
			return nil, ErrIntegrity
		}
		isDir := dir == "" && (name == "offline" || name == "online" || name == "public" || name == "wallet")
		if row.IsDir() != isDir {
			return nil, ErrIntegrity
		}
		found[name] = true
	}
	return found, nil
}
func (f *provisionFiles) child(name string, create bool) (bool, error) {
	if name != "offline" && name != "online" && name != "public" && name != "wallet" {
		return false, ErrProvisionInvalid
	}
	if _, ok := f.base.children[name]; ok {
		return true, f.check(context.Background())
	}
	var s unix.Stat_t
	e := unix.Fstatat(int(f.base.root().Fd()), name, &s, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(e, unix.ENOENT) {
		if !create {
			return false, nil
		}
		if name == "wallet" {
			return false, ErrProvisionInvalid
		}
		if unix.Mkdirat(int(f.base.root().Fd()), name, 0700) != nil {
			return false, ErrUnavailable
		}
		f.changed = true
		if f.base.root().Sync() != nil {
			return false, ErrUnavailable
		}
	} else if e != nil {
		return false, ErrUnavailable
	}
	before, e := statAt(f.base.root(), name)
	if e != nil || before.mode&unix.S_IFMT != unix.S_IFDIR || before.uid != uint32(os.Geteuid()) || before.mode&07777 != 0700 {
		return false, ErrIntegrity
	}
	d, e := openAt(f.base.root(), name, unix.O_RDONLY|unix.O_DIRECTORY)
	if e != nil {
		return false, e
	}
	f.base.children[name] = directoryRef{file: d, name: name, stamp: before}
	after, e := statFD(d)
	if e != nil || !directorySame(before, after) || noExtendedACL(d) != nil {
		return false, ErrIntegrity
	}
	return true, nil
}
func (f *provisionFiles) inventory() error {
	names, e := f.names("")
	if e != nil {
		return e
	}
	for _, name := range []string{"offline", "online", "public", "wallet"} {
		if names[name] {
			if _, e = f.child(name, false); e != nil {
				return e
			}
			if _, e = f.names(name); e != nil {
				return e
			}
		}
	}
	return f.check(context.Background())
}
func (f *provisionFiles) check(ctx context.Context) error {
	if ctx == nil || f == nil || f.base == nil || f.base.closed || f.base.poisoned || f.base.closeErr != nil {
		return ErrIntegrity
	}
	for i, d := range f.base.dirs {
		got, e := statFD(d.file)
		if e != nil || !directorySame(got, d.stamp) {
			return ErrIntegrity
		}
		if i > 0 {
			named, e := statAt(f.base.dirs[i-1].file, d.name)
			if e != nil || !directorySame(named, d.stamp) {
				return ErrIntegrity
			}
		}
	}
	if len(f.base.dirs) == 0 || noExtendedACL(f.base.root()) != nil {
		return ErrIntegrity
	}
	for _, d := range f.base.children {
		got, e := statFD(d.file)
		if e != nil || !directorySame(got, d.stamp) || noExtendedACL(d.file) != nil {
			return ErrIntegrity
		}
		named, e := statAt(f.base.root(), d.name)
		if e != nil || !directorySame(named, d.stamp) {
			return ErrIntegrity
		}
	}
	for _, in := range f.inputs {
		if e := in.check(ctx); e != nil {
			return e
		}
	}
	return ctx.Err()
}
func (f *provisionFiles) read(ctx context.Context, dir, name string, limit int) ([]byte, bool, error) {
	if !provisionAllowedVersion(dir, name, f.version) || name == "offline" || name == "online" || name == "public" || name == "wallet" || dir == "wallet" {
		return nil, false, ErrProvisionInvalid
	}
	if e := f.check(ctx); e != nil {
		return nil, false, e
	}
	parent := f.directory(dir)
	if parent == nil {
		return nil, false, nil
	}
	key := filepath.Join(dir, name)
	in := f.inputs[key]
	if in == nil {
		var st unix.Stat_t
		e := unix.Fstatat(int(parent.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
		if errors.Is(e, unix.ENOENT) {
			return nil, false, nil
		}
		if e != nil {
			return nil, false, ErrUnavailable
		}
		in, e = selectPrivateInput(ctx, filepath.Join(f.path, dir, name), limit)
		if in != nil {
			f.inputs[key] = in
		}
		if e != nil {
			return nil, true, e
		}
	}
	raw, e := in.read(ctx)
	if e != nil {
		return nil, true, e
	}
	if e = f.check(ctx); e != nil {
		clear(raw)
		return nil, true, e
	}
	return raw, true, nil
}
func (f *provisionFiles) ensure(ctx context.Context, dir, name string, raw []byte, limit int, create bool) error {
	got, exists, e := f.read(ctx, dir, name, limit)
	if e != nil {
		return e
	}
	if exists {
		defer clear(got)
		if !bytes.Equal(got, raw) {
			return ErrProvisionIncomplete
		}
		return nil
	}
	if !create || len(raw) < 1 || len(raw) > limit {
		return ErrProvisionIncomplete
	}
	parent := f.directory(dir)
	if parent == nil {
		return ErrProvisionIncomplete
	}
	if e = f.check(ctx); e != nil {
		return e
	}
	fd, e := unix.Openat(int(parent.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrUnavailable
	}
	f.changed = true
	file := os.NewFile(uintptr(fd), "provision-selected-output")
	n, we := file.Write(raw)
	se := file.Sync()
	ce := f.base.closeTemporary(file)
	if we != nil || n != len(raw) || se != nil || ce != nil || parent.Sync() != nil {
		f.base.poisoned = true
		return errors.Join(ErrIntegrity, ce)
	}
	got, exists, e = f.read(ctx, dir, name, limit)
	defer clear(got)
	if e != nil || !exists || !bytes.Equal(raw, got) {
		return errors.Join(ErrIntegrity, e)
	}
	return nil
}
func (f *provisionFiles) canGenerate() bool {
	names, e := f.names("")
	if e != nil || names[provisionReadyFile(f.version)] || names["online"] || names["public"] || names["wallet"] {
		return false
	}
	if f.directory("offline") == nil {
		return true
	}
	names, e = f.names("offline")
	return e == nil && len(names) == 0
}
func (f *provisionFiles) close() error {
	if f == nil {
		return nil
	}
	if f.base.closeErr != nil {
		return f.base.closeErr
	}
	for _, in := range f.inputs {
		if e := in.close(); e != nil {
			f.base.closeErr = ErrCleanup
			return ErrCleanup
		}
	}
	return f.base.close()
}
