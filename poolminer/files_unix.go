//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"
)

type directoryLink struct {
	parent *os.File
	name   string
	child  *os.File
	dev    uint64
	ino    uint64
}
type credentialFiles struct {
	path      string
	chain     []*os.File
	links     []directoryLink
	root      *os.File
	db        *os.File
	marker    *os.File
	markerRaw []byte
	failed    []*os.File
	closeErr  error
}

func fdFile(fd int, name string, e error) (*os.File, error) {
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(fd), name), nil
}
func openDirectoryAt(parent *os.File, name string) (*os.File, error) {
	fd, e := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	return fdFile(fd, name, e)
}
func statFile(f *os.File) (unix.Stat_t, error) {
	var st unix.Stat_t
	if f == nil {
		return st, ErrStorage
	}
	e := unix.Fstat(int(f.Fd()), &st)
	return st, e
}
func sameInode(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid
}
func (f *credentialFiles) closeTemporary(file *os.File) error {
	if e := file.Close(); e != nil {
		f.failed = append(f.failed, file)
		f.closeErr = errors.Join(f.closeErr, e)
		return ErrStorage
	}
	return nil
}
func openCredentialFiles(path string, create bool) (f *credentialFiles, err error) {
	f = &credentialFiles{path: path}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 4096 || path == "/" {
		return f, ErrInvalid
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) > 64 {
		return f, ErrInvalid
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return f, ErrStorage
	}
	parent := os.NewFile(uintptr(fd), "/")
	f.chain = append(f.chain, parent)
	for i, name := range parts {
		if name == "" || name == "." || name == ".." {
			return f, ErrInvalid
		}
		if create && i == len(parts)-1 {
			if e = unix.Mkdirat(int(parent.Fd()), name, 0700); e != nil {
				return f, ErrStorage
			}
			if e = parent.Sync(); e != nil {
				return f, ErrStorage
			}
		}
		child, e := openDirectoryAt(parent, name)
		if e != nil {
			return f, ErrStorage
		}
		f.chain = append(f.chain, child)
		st, e := statFile(child)
		if e != nil {
			return f, ErrStorage
		}
		f.links = append(f.links, directoryLink{parent: parent, name: name, child: child, dev: uint64(st.Dev), ino: uint64(st.Ino)})
		parent = child
	}
	f.root = parent
	st, e := statFile(f.root)
	if e != nil || st.Mode&0777 != 0700 || st.Uid != uint32(os.Geteuid()) {
		return f, ErrStorage
	}
	if e = unix.Flock(int(f.root.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		if e == unix.EWOULDBLOCK || e == unix.EAGAIN {
			return f, ErrBusy
		}
		return f, ErrStorage
	}
	if create {
		return f, nil
	}
	if e = f.openEstablished(); e != nil {
		return f, e
	}
	return f, nil
}
func (f *credentialFiles) openEstablished() error {
	db, e := openDirectoryAt(f.root, "db")
	if e != nil {
		return ErrStorage
	}
	f.db = db
	st, e := statFile(db)
	if e != nil || st.Uid != uint32(os.Geteuid()) || st.Mode&0777 != 0700 {
		return ErrStorage
	}
	fd, e := unix.Openat(int(f.root.Fd()), markerName, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrStorage
	}
	f.marker = os.NewFile(uintptr(fd), markerName)
	ms, e := statFile(f.marker)
	if e != nil || ms.Mode&unix.S_IFMT != unix.S_IFREG || ms.Mode&0777 != 0600 || ms.Uid != uint32(os.Geteuid()) || ms.Nlink != 1 || ms.Size < 1 || ms.Size > maxMarkerBytes {
		return ErrStorage
	}
	raw := make([]byte, ms.Size)
	n, e := f.marker.ReadAt(raw, 0)
	if e != nil || n != len(raw) {
		return ErrStorage
	}
	f.markerRaw = raw
	return f.check()
}
func (f *credentialFiles) initialize(raw []byte) error {
	fd, e := unix.Openat(int(f.root.Fd()), markerName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrStorage
	}
	file := os.NewFile(uintptr(fd), markerName)
	n, writeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := f.closeTemporary(file)
	if n != len(raw) || writeErr != nil || syncErr != nil || closeErr != nil {
		return ErrStorage
	}
	if e = unix.Mkdirat(int(f.root.Fd()), "db", 0700); e != nil {
		return ErrStorage
	}
	if e = f.root.Sync(); e != nil {
		return ErrStorage
	}
	return f.openEstablished()
}
func (f *credentialFiles) check() error {
	if f == nil || f.closeErr != nil || f.root == nil {
		return ErrStorage
	}
	for _, l := range f.links {
		st, e := statFile(l.child)
		var named unix.Stat_t
		ne := unix.Fstatat(int(l.parent.Fd()), l.name, &named, unix.AT_SYMLINK_NOFOLLOW)
		if e != nil || ne != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || uint64(st.Dev) != l.dev || uint64(st.Ino) != l.ino || !sameInode(st, named) {
			return ErrStorage
		}
	}
	root, e := statFile(f.root)
	if e != nil || root.Mode&0777 != 0700 || root.Uid != uint32(os.Geteuid()) {
		return ErrStorage
	}
	for name, file := range map[string]*os.File{"db": f.db, markerName: f.marker} {
		st, e := statFile(file)
		var named unix.Stat_t
		ne := unix.Fstatat(int(f.root.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW)
		if e != nil || ne != nil || !sameInode(st, named) || st.Uid != uint32(os.Geteuid()) {
			return ErrStorage
		}
		if name == "db" {
			if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0777 != 0700 {
				return ErrStorage
			}
		} else {
			if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0777 != 0600 || st.Nlink != 1 || st.Size != int64(len(f.markerRaw)) {
				return ErrStorage
			}
		}
	}
	scan, e := openDirectoryAt(f.root, ".")
	if e != nil {
		return ErrStorage
	}
	names, readErr := scan.Readdirnames(4)
	closeErr := f.closeTemporary(scan)
	if (readErr != nil && readErr != io.EOF) || closeErr != nil || len(names) != 2 {
		return ErrStorage
	}
	seen := map[string]bool{}
	for _, name := range names {
		seen[name] = true
	}
	if !seen[markerName] || !seen["db"] {
		return ErrStorage
	}
	raw := make([]byte, len(f.markerRaw))
	n, e := f.marker.ReadAt(raw, 0)
	if e != nil || n != len(raw) || !bytes.Equal(raw, f.markerRaw) {
		return ErrStorage
	}
	return nil
}

// Inspect fixed LevelDB filenames before it opens background pathname-based
// storage. A live LevelDB may rename/delete its own files, so ongoing checks pin
// the directories/marker instead of racing its compaction directory listing.
var levelDBName = regexp.MustCompile(`^(CURRENT(\.bak|\.[0-9]+)?|LOCK|LOG(\.old)?|MANIFEST-[0-9]+|[0-9]+\.(log|ldb|sst|tmp))$`)

func (f *credentialFiles) auditDBBeforeOpen() error {
	scan, e := openDirectoryAt(f.db, ".")
	if e != nil {
		return ErrStorage
	}
	count := 0
	var failure error
	for {
		names, readErr := scan.Readdirnames(64)
		for _, name := range names {
			count++
			if count > 4096 || len(name) > 128 || !levelDBName.MatchString(name) {
				failure = ErrStorage
				break
			}
			var st unix.Stat_t
			if unix.Fstatat(int(f.db.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
				failure = ErrStorage
				break
			}
		}
		if failure != nil || readErr == io.EOF {
			break
		}
		if readErr != nil {
			failure = ErrStorage
			break
		}
	}
	return errors.Join(failure, f.closeTemporary(scan))
}
func (f *credentialFiles) sync() error {
	if f.check() != nil {
		return ErrStorage
	}
	var err error
	for _, file := range []*os.File{f.db, f.root, f.links[len(f.links)-1].parent} {
		err = errors.Join(err, file.Sync())
	}
	if err != nil {
		return ErrStorage
	}
	return f.check()
}
func (f *credentialFiles) close() error {
	if f == nil {
		return nil
	}
	if f.closeErr != nil {
		return f.closeErr
	}
	// Do not drop the root lock when closing a subordinate resource failed.
	for _, file := range []*os.File{f.marker, f.db} {
		if file != nil {
			if e := f.closeTemporary(file); e != nil {
				return e
			}
		}
	}
	f.marker = nil
	f.db = nil
	for i := len(f.chain) - 1; i >= 0; i-- {
		if e := f.closeTemporary(f.chain[i]); e != nil {
			return e
		}
	}
	f.chain = nil
	f.root = nil
	return nil
}
