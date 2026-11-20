//go:build !darwin && !linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import "os"

type fileStamp struct {
	Dev, Ino        uint64
	Size, Sec, Nsec int64
	Mode, UID       uint32
	Links           uint64
}
type dirChain struct{}

func supportedPlatform() bool                           { return false }
func stamp(*os.File) (fileStamp, error)                 { return fileStamp{}, ErrProfile }
func relativeStamp(*os.File, string) (fileStamp, error) { return fileStamp{}, ErrProfile }
func sameFile(a, b fileStamp) bool                      { return a == b }
func privateFile(fileStamp) bool                        { return false }
func createAt(*os.File, string) (*os.File, error)       { return nil, ErrProfile }
func readAt(*os.File, string, int) (*os.File, []byte, fileStamp, error) {
	return nil, nil, fileStamp{}, ErrProfile
}
func renameAt(*os.File, string, string) error      { return ErrProfile }
func removeAt(*os.File, string) error              { return ErrProfile }
func lockAt(*os.File, bool) (*os.File, error)      { return nil, ErrProfile }
func pinDirectory(string, bool) (*dirChain, error) { return nil, ErrProfile }
func (*dirChain) dir() *os.File                    { return nil }
func (*dirChain) check() error                     { return ErrProfile }
func (*dirChain) close() error                     { return nil }
