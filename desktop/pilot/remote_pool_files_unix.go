//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var remotePoolComponentNames = [...]string{"outbox"}

type remotePoolFiles struct {
	path                                  string
	setup                                 RemotePoolConfig
	create, established, closed, poisoned bool
	closeErr                              error
	failedClosers                         []*os.File
	dirs                                  []remotePoolDirectoryRef
	children                              map[string]remotePoolDirectoryRef
	intent, ready                         *remotePoolPrivateInput
}

func remotePoolComponent(name string) bool {
	for _, n := range remotePoolComponentNames {
		if n == name {
			return true
		}
	}
	return false
}
func (f *remotePoolFiles) root() *os.File { return f.dirs[len(f.dirs)-1].file }

// The returned partial owner must be closed even on failure. It retains every
// descriptor/lock until cleanup actually completes; no ambiguous fd is retried.
func openRemotePoolFiles(path string, s RemotePoolConfig, create bool) (f *remotePoolFiles, err error) {
	if ValidateRemotePoolConfig(s) != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 4096 || strings.IndexByte(path, 0) >= 0 || os.Getuid() != os.Geteuid() {
		return nil, ErrRemotePoolConfig
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || len(parts) > 65 {
		return nil, ErrRemotePoolConfig
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, ErrRemotePoolConfig
		}
	}
	f = &remotePoolFiles{path: path, setup: s, create: create, children: map[string]remotePoolDirectoryRef{}}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return f, ErrRemotePoolUnavailable
	}
	root := os.NewFile(uintptr(fd), "remote-pool-root")
	f.dirs = append(f.dirs, remotePoolDirectoryRef{file: root})
	stamp, e := remotePoolStatFD(root)
	if e != nil {
		return f, e
	}
	f.dirs[0].remotePoolStamp = stamp
	current := root
	for i, name := range parts {
		var st unix.Stat_t
		e = unix.Fstatat(int(current.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
		if errors.Is(e, unix.ENOENT) && i == len(parts)-1 && create {
			if unix.Mkdirat(int(current.Fd()), name, 0700) != nil || current.Sync() != nil {
				return f, ErrRemotePoolUnavailable
			}
		} else if e != nil {
			return f, ErrRemotePoolUnavailable
		}
		before, e := remotePoolStatAt(current, name)
		if e != nil || before.mode&unix.S_IFMT != unix.S_IFDIR {
			return f, ErrRemotePoolChanged
		}
		next, e := remotePoolOpenAt(current, name, unix.O_RDONLY|unix.O_DIRECTORY)
		if e != nil {
			return f, e
		}
		f.dirs = append(f.dirs, remotePoolDirectoryRef{file: next, name: name})
		after, e := remotePoolStatFD(next)
		if e != nil || !remotePoolDirectorySame(before, after) {
			return f, ErrRemotePoolChanged
		}
		f.dirs[len(f.dirs)-1].remotePoolStamp = after
		current = next
	}
	st := f.dirs[len(f.dirs)-1].remotePoolStamp
	if st.uid != uint32(os.Geteuid()) || st.mode&07777 != 0700 || remotePoolNoExtendedACL(current) != nil {
		return f, ErrRemotePoolChanged
	}
	if unix.Flock(int(current.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return f, ErrRemotePoolUnavailable
	}
	names, e := f.names()
	if e != nil {
		return f, e
	}
	_, hasIntent := names[remotePoolIntentName]
	_, hasReady := names[remotePoolReadyName]
	if !create && (!hasReady || !hasIntent) {
		return f, ErrRemotePoolChanged
	}
	if !hasIntent {
		if !create || len(names) != 0 {
			return f, ErrRemotePoolChanged
		}
		raw, _, e := remotePoolMarkerBytes(s)
		if e != nil {
			return f, e
		}
		if e = f.writeExclusive(remotePoolIntentName, raw); e != nil {
			return f, e
		}
	}
	f.intent, e = selectRemotePoolPrivateInput(context.Background(), filepath.Join(path, remotePoolIntentName), RemotePoolMaxConfigBytes+512)
	if e != nil {
		return f, e
	}
	got, e := f.intent.read(context.Background())
	if e != nil {
		return f, e
	}
	want, _, _ := remotePoolMarkerBytes(s)
	if !bytes.Equal(got, want) {
		return f, ErrRemotePoolChanged
	}
	f.established = hasReady
	if hasReady {
		f.ready, e = selectRemotePoolPrivateInput(context.Background(), filepath.Join(path, remotePoolReadyName), 1024)
		if e != nil {
			return f, e
		}
		got, e = f.ready.read(context.Background())
		if e != nil {
			return f, e
		}
		_, want, e = remotePoolMarkerBytes(s)
		if e != nil || !bytes.Equal(got, want) {
			return f, ErrRemotePoolChanged
		}
		for _, name := range remotePoolComponentNames {
			if e = f.adopt(name); e != nil {
				return f, e
			}
		}
	}
	return f, f.check()
}

func (f *remotePoolFiles) names() (map[string]bool, error) {
	d, e := remotePoolOpenAt(f.root(), ".", unix.O_RDONLY|unix.O_DIRECTORY)
	if e != nil {
		return nil, e
	}
	entries, e := d.ReadDir(4)
	ce := f.closeTemporary(d)
	if ce != nil {
		return nil, ce
	}
	if e != nil && !errors.Is(e, io.EOF) || ce != nil || len(entries) > 3 {
		return nil, ErrRemotePoolChanged
	}
	names := map[string]bool{}
	for _, entry := range entries {
		n := entry.Name()
		if n != remotePoolIntentName && n != remotePoolReadyName && !remotePoolComponent(n) || entry.Type()&os.ModeSymlink != 0 {
			return nil, ErrRemotePoolChanged
		}
		names[n] = true
	}
	return names, nil
}

func (f *remotePoolFiles) writeExclusive(name string, raw []byte) error {
	fd, e := unix.Openat(int(f.root().Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		f.poisoned = true
		return ErrRemotePoolUnavailable
	}
	w := os.NewFile(uintptr(fd), "remote-pool-marker")
	n, we := w.Write(raw)
	se, ce := w.Sync(), f.closeTemporary(w)
	if ce != nil {
		return ce
	}
	if we != nil || n != len(raw) || se != nil || ce != nil || f.root().Sync() != nil {
		f.poisoned = true
		return ErrRemotePoolChanged
	}
	return nil
}

// Keep the exact failed owner reachable. Close errors can have an ambiguous
// descriptor outcome, so cleanup never retries the raw descriptor or releases
// the attachment root lock after an earlier cleanup failure.
func (f *remotePoolFiles) closeTemporary(file *os.File) error {
	if file.Close() == nil {
		return nil
	}
	f.failedClosers = append(f.failedClosers, file)
	f.poisoned = true
	f.closeErr = ErrRemotePoolCleanup
	return ErrRemotePoolCleanup
}

func (f *remotePoolFiles) check() error {
	if f == nil || f.closed || f.poisoned || len(f.dirs) == 0 || f.intent == nil {
		return ErrRemotePoolChanged
	}
	for i, d := range f.dirs {
		got, e := remotePoolStatFD(d.file)
		if e != nil || !remotePoolDirectorySame(got, d.remotePoolStamp) {
			return ErrRemotePoolChanged
		}
		if i > 0 {
			named, e := remotePoolStatAt(f.dirs[i-1].file, d.name)
			if e != nil || !remotePoolDirectorySame(named, d.remotePoolStamp) {
				return ErrRemotePoolChanged
			}
		}
	}
	if remotePoolNoExtendedACL(f.root()) != nil || f.intent.check(context.Background()) != nil {
		return ErrRemotePoolChanged
	}
	if f.established && (f.ready == nil || f.ready.check(context.Background()) != nil) {
		return ErrRemotePoolChanged
	}
	names, e := f.names()
	if e != nil {
		return e
	}
	if !names[remotePoolIntentName] || names[remotePoolReadyName] != f.established {
		return ErrRemotePoolChanged
	}
	for name, d := range f.children {
		named, e := remotePoolStatAt(f.root(), name)
		if e != nil || !remotePoolDirectorySame(named, d.remotePoolStamp) {
			return ErrRemotePoolChanged
		}
		got, e := remotePoolStatFD(d.file)
		if e != nil || !remotePoolDirectorySame(got, d.remotePoolStamp) || remotePoolNoExtendedACL(d.file) != nil {
			return ErrRemotePoolChanged
		}
	}
	return nil
}

func (f *remotePoolFiles) child(name string) (string, bool, error) {
	if !remotePoolComponent(name) {
		return "", false, ErrRemotePoolConfig
	}
	if e := f.check(); e != nil {
		return "", false, e
	}
	var st unix.Stat_t
	e := unix.Fstatat(int(f.root().Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(e, unix.ENOENT) && f.create && !f.established {
		return filepath.Join(f.path, name), true, nil
	}
	if e != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", false, ErrRemotePoolChanged
	}
	return filepath.Join(f.path, name), false, nil
}

func (f *remotePoolFiles) adopt(name string) error {
	if !remotePoolComponent(name) || f.closed || f.poisoned {
		return ErrRemotePoolConfig
	}
	if _, ok := f.children[name]; ok {
		return f.check()
	}
	before, e := remotePoolStatAt(f.root(), name)
	if e != nil || before.mode&unix.S_IFMT != unix.S_IFDIR || before.uid != uint32(os.Geteuid()) || before.mode&07777 != 0700 {
		return ErrRemotePoolChanged
	}
	d, e := remotePoolOpenAt(f.root(), name, unix.O_RDONLY|unix.O_DIRECTORY)
	if e != nil {
		return e
	}
	f.children[name] = remotePoolDirectoryRef{file: d, name: name, remotePoolStamp: before}
	got, e := remotePoolStatFD(d)
	if e != nil || !remotePoolDirectorySame(got, before) || remotePoolNoExtendedACL(d) != nil {
		return ErrRemotePoolChanged
	}
	return nil
}

func (f *remotePoolFiles) markReady() error {
	if f.established {
		return f.check()
	}
	if e := f.check(); e != nil {
		return e
	}
	if len(f.children) != len(remotePoolComponentNames) {
		return ErrRemotePoolChanged
	}
	_, raw, e := remotePoolMarkerBytes(f.setup)
	if e != nil {
		return e
	}
	if e = f.writeExclusive(remotePoolReadyName, raw); e != nil {
		return e
	}
	f.ready, e = selectRemotePoolPrivateInput(context.Background(), filepath.Join(f.path, remotePoolReadyName), 1024)
	if e != nil {
		f.poisoned = true
		return e
	}
	got, e := f.ready.read(context.Background())
	if e != nil || !bytes.Equal(got, raw) {
		f.poisoned = true
		return ErrRemotePoolChanged
	}
	f.established = true
	return f.check()
}

func (f *remotePoolFiles) close() error {
	if f == nil {
		return nil
	}
	if f.closed || f.closeErr != nil {
		return f.closeErr
	}
	f.closed = true
	if f.ready != nil && f.ready.close() != nil {
		f.closeErr = ErrRemotePoolCleanup
		return f.closeErr
	}
	if f.intent != nil && f.intent.close() != nil {
		f.closeErr = ErrRemotePoolCleanup
		return f.closeErr
	}
	for _, d := range f.children {
		if f.closeTemporary(d.file) != nil {
			return f.closeErr
		}
	}
	// Ancestors close first; the deepest directory owns the lock and closes last.
	for i := 0; i < len(f.dirs); i++ {
		if f.closeTemporary(f.dirs[i].file) != nil {
			return f.closeErr
		}
	}
	return f.closeErr
}
