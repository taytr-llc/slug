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

	"golang.org/x/sys/unix"
)

const intentName = "SLUG-POOL-OPERATOR-V1.json"
const readyName = "SLUG-POOL-OPERATOR-READY-V1.json"

var componentNames = [...]string{"issuer", "pool", "rounds", "settlement", "scheduler", "ownership", "miners"}

type operatorFiles struct {
	path                                  string
	setup                                 Setup
	issue                                 *RecoverableDailySetup
	create, established, closed, poisoned bool
	closeErr                              error
	failedClosers                         []*os.File
	dirs                                  []directoryRef
	children                              map[string]directoryRef
	intent, ready                         *privateInput
}

func component(name string) bool {
	for _, n := range componentNames {
		if n == name {
			return true
		}
	}
	return false
}
func (f *operatorFiles) root() *os.File { return f.dirs[len(f.dirs)-1].file }

// The returned partial owner must be closed even on failure. It retains every
// descriptor/lock until cleanup actually completes; no ambiguous fd is retried.
func openOperatorFiles(path string, s Setup, create bool) (*operatorFiles, error) {
	return openOperatorFilesVersion(path, s, create, 1)
}
func openOperatorFilesVersion(path string, s Setup, create bool, version uint32) (f *operatorFiles, err error) {
	return openOperatorFilesSelection(path, s, create, version, nil)
}
func openOperatorFilesSelection(path string, s Setup, create bool, version uint32, issue *RecoverableDailySetup) (f *operatorFiles, err error) {
	if issue != nil {
		if version != 2 || ValidateRecoverableDailySetup(*issue) != nil {
			return nil, ErrInvalid
		}
		base, e := MarshalDailySetup(s)
		want, x := MarshalDailySetup(issue.Base)
		if e != nil || x != nil || !bytes.Equal(base, want) {
			return nil, ErrInvalid
		}
	}
	if validateSetupVersion(s, version) != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 4096 || strings.IndexByte(path, 0) >= 0 || os.Getuid() != os.Geteuid() {
		return nil, ErrInvalid
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || len(parts) > 65 {
		return nil, ErrInvalid
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, ErrInvalid
		}
	}
	f = &operatorFiles{path: path, setup: s, issue: issue, create: create, children: map[string]directoryRef{}}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return f, ErrUnavailable
	}
	root := os.NewFile(uintptr(fd), "pool-operator-root")
	f.dirs = append(f.dirs, directoryRef{file: root})
	stamp, e := statFD(root)
	if e != nil {
		return f, e
	}
	f.dirs[0].stamp = stamp
	current := root
	for i, name := range parts {
		var st unix.Stat_t
		e = unix.Fstatat(int(current.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
		if errors.Is(e, unix.ENOENT) && i == len(parts)-1 && create {
			if unix.Mkdirat(int(current.Fd()), name, 0700) != nil || current.Sync() != nil {
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
		f.dirs = append(f.dirs, directoryRef{file: next, name: name})
		after, e := statFD(next)
		if e != nil || !directorySame(before, after) {
			return f, ErrIntegrity
		}
		f.dirs[len(f.dirs)-1].stamp = after
		current = next
	}
	st := f.dirs[len(f.dirs)-1].stamp
	if st.uid != uint32(os.Geteuid()) || st.mode&07777 != 0700 || noExtendedACL(current) != nil {
		return f, ErrIntegrity
	}
	if unix.Flock(int(current.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return f, ErrUnavailable
	}
	names, e := f.names()
	if e != nil {
		return f, e
	}
	_, hasIntent := names[f.intentFilename()]
	_, hasReady := names[f.readyFilename()]
	if create && hasReady || !create && (!hasReady || !hasIntent) {
		return f, ErrIntegrity
	}
	if !hasIntent {
		if !create || len(names) != 0 {
			return f, ErrIntegrity
		}
		raw, e := f.setupBytes()
		if e != nil {
			return f, e
		}
		if e = f.writeExclusive(f.intentFilename(), append(raw, '\n')); e != nil {
			return f, e
		}
	}
	f.intent, e = selectPrivateInput(context.Background(), filepath.Join(path, f.intentFilename()), f.setupLimit()+1)
	if e != nil {
		return f, e
	}
	got, e := f.intent.read(context.Background())
	if e != nil {
		return f, e
	}
	want, _ := f.setupBytes()
	want = append(want, '\n')
	if !bytes.Equal(got, want) {
		return f, ErrIntegrity
	}
	f.established = hasReady
	if hasReady {
		f.ready, e = selectPrivateInput(context.Background(), filepath.Join(path, f.readyFilename()), 1024)
		if e != nil {
			return f, e
		}
		got, e = f.ready.read(context.Background())
		if e != nil {
			return f, e
		}
		want, e = f.readyBytes()
		if e != nil || !bytes.Equal(got, want) {
			return f, ErrIntegrity
		}
		for _, name := range componentNames {
			if e = f.adopt(name); e != nil {
				return f, e
			}
		}
	}
	return f, f.check()
}

func (f *operatorFiles) names() (map[string]bool, error) {
	d, e := openAt(f.root(), ".", unix.O_RDONLY|unix.O_DIRECTORY)
	if e != nil {
		return nil, e
	}
	entries, e := d.ReadDir(10)
	ce := f.closeTemporary(d)
	if ce != nil {
		return nil, ce
	}
	if e != nil && !errors.Is(e, io.EOF) || ce != nil || len(entries) > 9 {
		return nil, ErrIntegrity
	}
	names := map[string]bool{}
	for _, entry := range entries {
		n := entry.Name()
		if n != f.intentFilename() && n != f.readyFilename() && !component(n) || entry.Type()&os.ModeSymlink != 0 {
			return nil, ErrIntegrity
		}
		names[n] = true
	}
	return names, nil
}

func (f *operatorFiles) writeExclusive(name string, raw []byte) error {
	fd, e := unix.Openat(int(f.root().Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrUnavailable
	}
	w := os.NewFile(uintptr(fd), "pool-operator-marker")
	n, we := w.Write(raw)
	se, ce := w.Sync(), f.closeTemporary(w)
	if ce != nil {
		return ce
	}
	if we != nil || n != len(raw) || se != nil || ce != nil || f.root().Sync() != nil {
		f.poisoned = true
		return ErrIntegrity
	}
	return nil
}

// Keep the exact failed owner reachable. Close errors can have an ambiguous
// descriptor outcome, so cleanup never retries the raw descriptor or releases
// the operator root lock after an earlier cleanup failure.
func (f *operatorFiles) closeTemporary(file *os.File) error {
	if file.Close() == nil {
		return nil
	}
	f.failedClosers = append(f.failedClosers, file)
	f.poisoned = true
	f.closeErr = ErrCleanup
	return ErrCleanup
}

func (f *operatorFiles) check() error {
	if f == nil || f.closed || f.poisoned || len(f.dirs) == 0 || f.intent == nil {
		return ErrIntegrity
	}
	for i, d := range f.dirs {
		got, e := statFD(d.file)
		if e != nil || !directorySame(got, d.stamp) {
			return ErrIntegrity
		}
		if i > 0 {
			named, e := statAt(f.dirs[i-1].file, d.name)
			if e != nil || !directorySame(named, d.stamp) {
				return ErrIntegrity
			}
		}
	}
	if noExtendedACL(f.root()) != nil || f.intent.check(context.Background()) != nil {
		return ErrIntegrity
	}
	if f.established && (f.ready == nil || f.ready.check(context.Background()) != nil) {
		return ErrIntegrity
	}
	names, e := f.names()
	if e != nil {
		return e
	}
	if !names[f.intentFilename()] || names[f.readyFilename()] != f.established {
		return ErrIntegrity
	}
	for name, d := range f.children {
		named, e := statAt(f.root(), name)
		if e != nil || !directorySame(named, d.stamp) {
			return ErrIntegrity
		}
		got, e := statFD(d.file)
		if e != nil || !directorySame(got, d.stamp) || noExtendedACL(d.file) != nil {
			return ErrIntegrity
		}
	}
	return nil
}

func (f *operatorFiles) child(name string) (string, bool, error) {
	if !component(name) {
		return "", false, ErrInvalid
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
		return "", false, ErrIntegrity
	}
	return filepath.Join(f.path, name), false, nil
}

func (f *operatorFiles) adopt(name string) error {
	if !component(name) || f.closed || f.poisoned {
		return ErrInvalid
	}
	if _, ok := f.children[name]; ok {
		return f.check()
	}
	before, e := statAt(f.root(), name)
	if e != nil || before.mode&unix.S_IFMT != unix.S_IFDIR || before.uid != uint32(os.Geteuid()) || before.mode&07777 != 0700 {
		return ErrIntegrity
	}
	d, e := openAt(f.root(), name, unix.O_RDONLY|unix.O_DIRECTORY)
	if e != nil {
		return e
	}
	f.children[name] = directoryRef{file: d, name: name, stamp: before}
	got, e := statFD(d)
	if e != nil || !directorySame(got, before) || noExtendedACL(d) != nil {
		return ErrIntegrity
	}
	return nil
}

func (f *operatorFiles) markReady() error {
	if f.established {
		return f.check()
	}
	if e := f.check(); e != nil {
		return e
	}
	if len(f.children) != len(componentNames) {
		return ErrIntegrity
	}
	raw, e := f.readyBytes()
	if e != nil {
		return e
	}
	if e = f.writeExclusive(f.readyFilename(), raw); e != nil {
		return e
	}
	f.ready, e = selectPrivateInput(context.Background(), filepath.Join(f.path, f.readyFilename()), 1024)
	if e != nil {
		f.poisoned = true
		return e
	}
	got, e := f.ready.read(context.Background())
	if e != nil || !bytes.Equal(got, raw) {
		f.poisoned = true
		return ErrIntegrity
	}
	f.established = true
	return f.check()
}

func (f *operatorFiles) close() error {
	if f == nil {
		return nil
	}
	if f.closed || f.closeErr != nil {
		return f.closeErr
	}
	f.closed = true
	if f.ready != nil && f.ready.close() != nil {
		f.closeErr = ErrCleanup
		return f.closeErr
	}
	if f.intent != nil && f.intent.close() != nil {
		f.closeErr = ErrCleanup
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

func (f *operatorFiles) intentFilename() string {
	if f.issue != nil {
		return "SLUG-POOL-OPERATOR-ISSUE1.json"
	}
	return operatorIntentName(f.setup.Version)
}
func (f *operatorFiles) readyFilename() string {
	if f.issue != nil {
		return "SLUG-POOL-OPERATOR-READY-ISSUE1.json"
	}
	return operatorReadyName(f.setup.Version)
}
func (f *operatorFiles) setupBytes() ([]byte, error) {
	if f.issue != nil {
		return MarshalRecoverableDailySetup(*f.issue)
	}
	return marshalSetupVersion(f.setup, f.setup.Version)
}
func (f *operatorFiles) readyBytes() ([]byte, error) {
	if f.issue != nil {
		return recoverableReadyBytes(*f.issue)
	}
	return readyBytesVersion(f.setup, f.setup.Version)
}
func (f *operatorFiles) setupLimit() int {
	if f.issue != nil {
		return MaxRecoverableSetupBytes
	}
	return MaxSetupBytes
}
