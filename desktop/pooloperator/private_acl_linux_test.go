//go:build linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// Only an original owned disposable public-fixture descriptor receives the
// actual kernel ACL. No setfacl, privilege, host path or external process is used.
func opLinuxTestACL(t *testing.T, path, attribute string, directory bool) {
	t.Helper()
	if attribute != "system.posix_acl_access" && attribute != "system.posix_acl_default" {
		t.Fatal("unexpected test attribute")
	}
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		t.Fatal(e)
	}
	file := os.NewFile(uintptr(fd), "public-synthetic-linux-acl")
	installed := false
	t.Cleanup(func() {
		if installed {
			if e := unix.Fremovexattr(int(file.Fd()), attribute); e != nil {
				t.Error("exact synthetic ACL removal", e)
			}
		}
		if e := file.Close(); e != nil {
			t.Error("original ACL descriptor close", e)
		}
	})
	before, e := statFD(file)
	mode, permission := uint32(0600), uint16(6)
	if directory {
		mode, permission = 0700, 7
	}
	if e != nil || before.uid != uint32(os.Geteuid()) || before.mode&07777 != mode || (before.mode&unix.S_IFMT == unix.S_IFDIR) != directory || noExtendedACL(file) != nil {
		t.Fatal("original test owner/mode/ACL", e)
	}
	// POSIX ACL xattr v2: the named user is masked to no effective permission.
	// Group/other remain zero, preserving owner-only mode while the ACL exists.
	type entry struct {
		tag, perm uint16
		id        uint32
	}
	entries := []entry{{1, permission, ^uint32(0)}, {2, 4, uint32(os.Geteuid()) + 1}, {4, 0, ^uint32(0)}, {16, 0, ^uint32(0)}, {32, 0, ^uint32(0)}}
	raw := make([]byte, 4+8*len(entries))
	binary.LittleEndian.PutUint32(raw, 2)
	for i, item := range entries {
		at := 4 + 8*i
		binary.LittleEndian.PutUint16(raw[at:], item.tag)
		binary.LittleEndian.PutUint16(raw[at+2:], item.perm)
		binary.LittleEndian.PutUint32(raw[at+4:], item.id)
	}
	if e = unix.Fsetxattr(int(file.Fd()), attribute, raw, 0); e != nil {
		// Unsupported/denied ACL mutation is an unavailable Linux gate, never skip.
		t.Fatal("actual synthetic ACL could not be installed", e)
	}
	installed = true
	after, e := statFD(file)
	if e != nil || after.uid != before.uid || after.mode&07777 != mode || after.dev != before.dev || after.ino != before.ino {
		t.Fatal("ACL fixture changed owner/mode/object", e)
	}
	if n, e := unix.Fgetxattr(int(file.Fd()), attribute, nil); e != nil || n != len(raw) {
		t.Fatal("actual ACL absent or unexpected size", n, e)
	}
	if !errors.Is(noExtendedACL(file), ErrUnavailable) {
		t.Fatal("actual access/default ACL was accepted")
	}
}

func TestOperatorPrivateInputLinuxACLRefusal(t *testing.T) {
	for _, tc := range []struct {
		name      string
		attribute string
		parent    bool
		after     bool
	}{
		{"file-access-before", "system.posix_acl_access", false, false},
		{"parent-access-before", "system.posix_acl_access", true, false},
		{"parent-default-before", "system.posix_acl_default", true, false},
		{"file-access-after", "system.posix_acl_access", false, true},
		{"parent-default-after", "system.posix_acl_default", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Create the file before a parent default ACL so inheritance/mode changes
			// cannot explain refusal of the original selected object.
			path := opSetupTestPrivateFile(t)
			target := path
			if tc.parent {
				target = filepath.Dir(path)
			}
			var selected *privateInput
			if tc.after {
				selected = opSetupTestSelect(t, path, len(opSetupTestPrivateBytes))
			}
			opLinuxTestACL(t, target, tc.attribute, tc.parent)
			if selected == nil {
				opSetupTestPrivateRefused(t, context.Background(), path, len(opSetupTestPrivateBytes))
			} else if raw, e := selected.read(context.Background()); !errors.Is(e, ErrIntegrity) || raw != nil {
				t.Fatal("post-selection ACL disclosed original input", e)
			}
		})
	}
}
