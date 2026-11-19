// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"os"
	"path/filepath"
	"strings"
)

// PoolOptions selects one immutable candidate source. The zero value preserves
// the original candidate descriptor. No stored value selects an option.
type PoolOptions struct{ CandidateDescriptorID string }

// ResolvePoolOptions accepts only the two frozen, compiled candidate tuples.
// It performs no filesystem access or source discovery.
func ResolvePoolOptions(options PoolOptions) (PoolOptions, error) {
	v1, e := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if e != nil {
		return PoolOptions{}, ErrPoolRuntime
	}
	v2, e := fighterdescriptor.ID(fighterdescriptor.CandidateNameV2Descriptor())
	if e != nil || v1 == v2 {
		return PoolOptions{}, ErrPoolRuntime
	}
	if options.CandidateDescriptorID == "" {
		options.CandidateDescriptorID = v1
	}
	if options.CandidateDescriptorID != v1 && options.CandidateDescriptorID != v2 {
		return PoolOptions{}, ErrPoolRuntime
	}
	return options, nil
}

// CheckPoolRuntimeSelection is a read-only preflight for the outer account
// owner, before it opens its own lock or authority. Runtime construction repeats
// the check independently. A missing marker never authorizes orphaned data.
func CheckPoolRuntimeSelection(root, authority string, options PoolOptions) error {
	options, e := ResolvePoolOptions(options)
	if e != nil || !validAccountID(authority) || !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, "\x00\r\n\t") {
		return ErrPoolRuntime
	}
	if e = poolRuntimePath(root); e != nil {
		return e
	}
	exists, e := poolRuntimeExists(root)
	if e != nil {
		return e
	}
	if !exists {
		return nil
	}
	if i, e := os.Lstat(root); e != nil || !i.IsDir() {
		return ErrPoolRuntime
	}
	markerPath := filepath.Join(root, poolRuntimeMarkerName)
	exists, e = poolRuntimeExists(markerPath)
	if e != nil {
		return e
	}
	if !exists {
		entries, e := os.ReadDir(root)
		if e != nil || len(entries) != 0 {
			return ErrPoolRuntime
		}
		return nil
	}
	var marker poolRuntimeMarker
	if readManifestJSON(markerPath, poolRuntimeMarkerLimit, &marker) != nil || !marker.validFor(authority, options.CandidateDescriptorID) {
		return ErrPoolRuntime
	}
	sourcePath := filepath.Join(root, poolRuntimeSourceName)
	exists, e = poolRuntimeExists(sourcePath)
	if e != nil {
		return e
	}
	if exists {
		var source poolshare.Config
		if readManifestJSON(sourcePath, poolRuntimeMarkerLimit, &source) != nil || !marker.validSource(source) {
			return ErrPoolRuntime
		}
	}
	return nil
}

func (m poolRuntimeMarker) validSource(source poolshare.Config) bool {
	p := source.AuthorityPin
	if p.Network != m.Network || p.Genesis != m.Genesis || p.DescriptorID != m.DescriptorID || source != m.source(p) {
		return false
	}
	_, e := poolservice.BindingID(source)
	return e == nil
}
