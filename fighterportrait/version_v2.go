// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterportrait

import "github.com/taytr-llc/slug/fightermanifest"

const (
	Version2       uint32 = 2
	RendererIDV2          = "slug-fighter-portrait-candidate-v2"
	digestDomainV2        = "SLUG fighter portrait SVG\x00v2\x00"
)

func rendererVersionID(version uint32) string {
	switch version {
	case Version1:
		return RendererID
	case Version2:
		return RendererIDV2
	default:
		return ""
	}
}
func supportedVersion(version uint32, m fightermanifest.Manifest) bool {
	switch version {
	case Version1:
		return supported(m)
	case Version2:
		return m.Version == fightermanifest.Version2 && m.Generator == fightermanifest.GeneratorIDV2 && m.Tables == fightermanifest.TableHashV2 && m.DNA.DNAVersion == 1 && m.Portrait.Version == 1
	default:
		return false
	}
}
