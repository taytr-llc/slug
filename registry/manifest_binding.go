// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"fmt"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
	"github.com/taytr-llc/slug/prospect"
)

// binding has no caller-selected implementation or exported Verified bypass.
// Open uses it for cheap internal consistency only; trusted reads separately
// establish exact proof/Verified equality through production verification.
func (s *ManifestStore) binding(verified prospect.Verified) (StaticBinding, error) {
	d, err := fighterdescriptor.ResolveCandidate(s.meta.DescriptorID)
	if err != nil {
		return StaticBinding{}, err
	}
	if d != s.meta.Descriptor || !manifestCanonicalHash(verified.WorkHash) {
		return StaticBinding{}, ErrManifestAuthority
	}
	m, err := fightermanifest.GenerateText(d.ManifestVersion, verified.WorkHash)
	if err != nil {
		return StaticBinding{}, err
	}
	if m.DNA != verified.DNA || m.DNA.DNAVersion != d.DNAVersion || m.Version != d.ManifestVersion || m.Generator != d.GeneratorID || m.Tables != d.ManifestTableHash || m.Portrait.Version != d.PortraitTraitVersion {
		return StaticBinding{}, fmt.Errorf("manifest descriptor or verified DNA mismatch")
	}
	manifestDigest, err := fightermanifest.Digest(m)
	if err != nil {
		return StaticBinding{}, err
	}
	// ResolveCandidate pins the complete implementation tuple; Digest checks the
	// exact renderer/manifest version pairing with the SVG byte bound.
	portraitDigest, err := fighterportrait.Digest(d.RendererVersion, m)
	if err != nil {
		return StaticBinding{}, err
	}
	return StaticBinding{StaticBindingVersion1, s.meta.DescriptorID, manifestDigest, portraitDigest}, nil
}
