// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterdescriptor

// CandidateNameV2Descriptor selects only the reviewed names-v2 bundle.
// It does not change the original default or activate production issuance.
func CandidateNameV2Descriptor() Descriptor {
	return Descriptor{
		Version: 1, DNAVersion: 1, ManifestVersion: 2, GeneratorID: "slug-fighter-manifest-candidate-v2",
		ManifestTableHash:    "b537490e1e602e27de01975aeec36f5fb53ba9a3a35f98143e8ecb7f9e2687c2",
		ManifestContractHash: "0283415807d33b362aaeffbb1570504085cd975bf9f3adc0b497d0c8b79856a7",
		PortraitTraitVersion: 1, RendererVersion: 2, RendererID: "slug-fighter-portrait-candidate-v2",
		RendererContractHash: "6044ce174e9a7a466783985e34b02b34ae2785719ca77b1268f7e6ad3c717677",
	}
}
