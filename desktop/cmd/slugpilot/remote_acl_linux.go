//go:build linux

// Copyright (c) 2026 The SLUG developers. ISC license.
// Presence is refused; no ACL values are read or logged.
package main

import (
	"golang.org/x/sys/unix"
	"os"
)

func remoteLaunchNoExtendedACL(f *os.File) error {
	if f == nil {
		return errRemoteLaunchUnavailable
	}
	for _, name := range [...]string{"system.posix_acl_access", "system.posix_acl_default"} {
		_, e := unix.Fgetxattr(int(f.Fd()), name, nil)
		if e != unix.ENODATA {
			return errRemoteLaunchUnavailable
		}
	}
	return nil
}
