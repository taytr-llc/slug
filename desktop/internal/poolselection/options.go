// Copyright (c) 2026 The SLUG developers. ISC license.
// Package poolselection shares the native/helper command-line selection policy.
package poolselection

import (
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"path/filepath"
	"strings"
)

var ErrSelection = errors.New("candidate name version requires an explicit compatible pool selection")

// Options is pure. Explicit name v2 cannot inherit a packaged mode default or
// an environment/default data path. Both flag entry points call it before I/O.
func Options(pool, poolExplicit bool, version string, versionExplicit bool, data string, dataExplicit bool) (pilot.PoolOptions, error) {
	if version != "1" && version != "2" {
		return pilot.PoolOptions{}, ErrSelection
	}
	if versionExplicit && !pool {
		return pilot.PoolOptions{}, ErrSelection
	}
	if version == "2" {
		if !versionExplicit || !pool || !poolExplicit || !dataExplicit || !filepath.IsAbs(data) || filepath.Clean(data) != data || strings.ContainsAny(data, "\x00\r\n\t") {
			return pilot.PoolOptions{}, ErrSelection
		}
		id, e := fighterdescriptor.ID(fighterdescriptor.CandidateNameV2Descriptor())
		if e != nil {
			return pilot.PoolOptions{}, ErrSelection
		}
		return pilot.ResolvePoolOptions(pilot.PoolOptions{CandidateDescriptorID: id})
	}
	return pilot.ResolvePoolOptions(pilot.PoolOptions{})
}

// HostedOptions requires an explicit canonical namespace even for name v1.
// Callers select this only for an explicit true --candidate-hosted-pool flag,
// after refusing every explicitly supplied legacy mode flag.
func HostedOptions(version string, versionExplicit bool, data string, dataExplicit bool) (pilot.PoolOptions, error) {
	if !dataExplicit || !filepath.IsAbs(data) || filepath.Clean(data) != data || strings.ContainsAny(data, "\x00\r\n\t") {
		return pilot.PoolOptions{}, ErrSelection
	}
	return Options(true, true, version, versionExplicit, data, true)
}

// HostedRecoveryOptions shares only immutable name selection with Set71. Mode
// and namespace selection remain explicit at both entry points.
func HostedRecoveryOptions(version string, versionExplicit bool, data string, dataExplicit bool) (pilot.PoolOptions, error) {
	return HostedOptions(version, versionExplicit, data, dataExplicit)
}
