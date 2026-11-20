//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

type scheduleFiles struct {
	path      string
	markerRaw []byte
}

func openScheduleFiles(string, bool) (*scheduleFiles, error) { return nil, ErrProfile }
func (*scheduleFiles) initialize([]byte) error               { return ErrProfile }
func (*scheduleFiles) check() error                          { return ErrProfile }
func (*scheduleFiles) sync() error                           { return ErrProfile }
func (*scheduleFiles) close() error                          { return nil }

func (*scheduleFiles) auditDBBeforeOpen() error { return ErrProfile }
