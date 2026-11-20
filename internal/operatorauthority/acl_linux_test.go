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
	// POSIX ACL xattr v2: owner rw, an extra named user read, group none,
	// mask none, other none. Mode can stay owner-only, but the ACL is present.
	type entry struct {
		tag, perm uint16
		id        uint32
	}
	entries := []entry{{1, 6, ^uint32(0)}, {2, 4, uint32(os.Geteuid()) + 1}, {4, 0, ^uint32(0)}, {16, 0, ^uint32(0)}, {32, 0, ^uint32(0)}}
	raw := make([]byte, 4+8*len(entries))
	binary.LittleEndian.PutUint32(raw, 2)
	for i, e := range entries {
		at := 4 + 8*i
		binary.LittleEndian.PutUint16(raw[at:], e.tag)
		binary.LittleEndian.PutUint16(raw[at+2:], e.perm)
		binary.LittleEndian.PutUint32(raw[at+4:], e.id)
	}
	file, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer file.Close()
	if e = unix.Fsetxattr(int(file.Fd()), "system.posix_acl_access", raw, 0); e != nil {
		t.Fatal("could not set bounded synthetic ACL", e)
	}
}
