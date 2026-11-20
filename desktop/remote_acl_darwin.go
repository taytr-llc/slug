//go:build darwin && cgo

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

/*
#include <sys/types.h>
#include <sys/acl.h>
#include <errno.h>

// fgetxattr on com.apple.system.Security is denied even for no-ACL files.
// Query the actual descriptor ACL API instead. The caller separately validates
// that this original descriptor is live and has the retained expected identity.
// NULL+ENOENT is absence on that descriptor, never a path-existence fallback.
// Any returned ACL (including an empty explicit ACL) is conservatively refused.
static int slug_native_remote_no_extended_acl(int fd) {
    errno = 0;
    acl_t acl = acl_get_fd_np(fd, ACL_TYPE_EXTENDED);
    if (acl == NULL) {
        return errno == ENOENT ? 0 : -1;
    }
    (void)acl_free(acl);
    return -1;
}
*/
import "C"

import "os"

func nativeRemoteNoExtendedACL(f *os.File) error {
	if f == nil || C.slug_native_remote_no_extended_acl(C.int(f.Fd())) != 0 {
		return errNativeRemoteUnavailable
	}
	return nil
}
