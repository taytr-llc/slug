//go:build !darwin && !linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import "context"

type restoredRoot struct{}

func restoredSupported() bool                              { return false }
func openRestoredRoot(string, bool) (*restoredRoot, error) { return nil, ErrRestoredGymStorage }
func (*restoredRoot) check() error                         { return ErrRestoredGymStorage }
func (*restoredRoot) inventory() (map[string]bool, error)  { return nil, ErrRestoredGymStorage }
func (*restoredRoot) Close() error                         { return ErrRestoredGymStorage }
func readRestoredSelected(context.Context, string, string, int) ([]byte, error) {
	return nil, ErrRestoredGymStorage
}

func (*restoredRoot) walletStatus(bool) (bool, error) { return false, ErrRestoredGymStorage }
