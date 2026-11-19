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
