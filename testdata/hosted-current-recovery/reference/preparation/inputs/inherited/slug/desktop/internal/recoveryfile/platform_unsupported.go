//go:build !darwin && !linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package recoveryfile

import "os"

func supportedPlatform() bool                                  { return false }
func stampFile(*os.File) (fileStamp, error)                    { return fileStamp{}, ErrUnsupported }
func inspectRelative(*os.File, string) (fileStamp, error)      { return fileStamp{}, ErrUnsupported }
func openReadRelative(*os.File, string) (*os.File, error)      { return nil, ErrUnsupported }
func createRelative(*os.File, string) (*os.File, error)        { return nil, ErrUnsupported }
func linkRelative(*os.File, string, string) error              { return ErrUnsupported }
func removeRelative(*os.File, string) error                    { return ErrUnsupported }
func walkDirectory(string, bool) (*os.File, []Identity, error) { return nil, nil, ErrUnsupported }
