//go:build linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package operatorauthority

import (
	"encoding/binary"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func addSyntheticACL(t *testing.T, path string) {
	t.Helper()
	file, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	installed := false
	t.Cleanup(func() {
		if installed {
			if e := unix.Fremovexattr(int(file.Fd()), "system.posix_acl_access"); e != nil {
				t.Error("synthetic ACL cleanup", e)
			}
		}
		if e := file.Close(); e != nil {
			t.Error("synthetic ACL descriptor close", e)
		}
	})
	before, e := statFD(file)
	if e != nil || before.uid != uint32(os.Geteuid()) || noExtendedACL(file) != nil {
		t.Fatal("original synthetic ACL fixture", e)
	}
	permission, mode := uint16(6), uint32(0600)
	if before.mode&unix.S_IFMT == unix.S_IFDIR {
		permission, mode = 7, 0700
	}
	if before.mode&07777 != mode {
		t.Fatal("unexpected original fixture mode")
	}
	// Preserve owner rw for files and rwx for directories. Otherwise the test
	// could refuse a parent for missing traversal permission instead of its ACL,
	// and Go's later TempDir cleanup could not reach the fixture files.
	// The additional named user is masked to none; group/other remain none.
	type entry struct {
		tag, perm uint16
		id        uint32
	}
	entries := []entry{{1, permission, ^uint32(0)}, {2, 4, uint32(os.Geteuid()) + 1}, {4, 0, ^uint32(0)}, {16, 0, ^uint32(0)}, {32, 0, ^uint32(0)}}
	raw := make([]byte, 4+8*len(entries))
	binary.LittleEndian.PutUint32(raw, 2)
	for i, e := range entries {
		at := 4 + 8*i
		binary.LittleEndian.PutUint16(raw[at:], e.tag)
		binary.LittleEndian.PutUint16(raw[at+2:], e.perm)
		binary.LittleEndian.PutUint32(raw[at+4:], e.id)
	}
	if e = unix.Fsetxattr(int(file.Fd()), "system.posix_acl_access", raw, 0); e != nil {
		t.Fatal("could not set bounded synthetic ACL", e)
	}
	installed = true
	after, e := statFD(file)
	if e != nil || after.dev != before.dev || after.ino != before.ino || after.uid != before.uid || after.mode != before.mode {
		t.Fatal("ACL fixture changed original identity or mode", e)
	}
	if n, e := unix.Fgetxattr(int(file.Fd()), "system.posix_acl_access", nil); e != nil || n != len(raw) || noExtendedACL(file) != ErrUnavailable {
		t.Fatal("actual extended ACL was absent or accepted", n, e)
	}
}
