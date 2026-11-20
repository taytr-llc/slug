//go:build darwin || linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func openInput(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
