// Copyright (c) 2026 The SLUG developers. ISC license.

package prospectservice

import (
	"context"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

const Version3 uint32 = 3

// ManifestServerConfig selects a separately pinned candidate authority. Empty
// Clients is valid configuration only for an explicit session constructor.
type ManifestServerConfig struct {
	Version      uint32                        `json:"version"`
	Listen       string                        `json:"listen"`
	AuthorityPin registry.ManifestAuthorityPin `json:"authority_pin"`
	Clients      []Credential                  `json:"clients"`
	MaxJobs      uint64                        `json:"max_jobs"`
	MaxRecords   uint64                        `json:"max_records"`
}

// ManifestClientConfig pins the endpoint's trust policy and the separate
// schema-5 authority namespace. Token is API access, never a signing key.
type ManifestClientConfig struct {
	Version       uint32                        `json:"version"`
	URL           string                        `json:"url"`
	Token         string                        `json:"token"`
	CACertificate string                        `json:"ca_certificate"`
	AuthorityPin  registry.ManifestAuthorityPin `json:"authority_pin"`
}

// ManifestAdmissionResult retains immutable candidate evidence. Parsing this
// DTO alone establishes neither trusted admission nor current ownership.
type ManifestAdmissionResult struct {
	Version            uint32                  `json:"version"`
	ContextSource      string                  `json:"context_source"`
	CurrentNodeChecked bool                    `json:"current_node_checked"`
	Authentication     string                  `json:"authentication"`
	BirthRule          string                  `json:"birth_rule"`
	Inserted           bool                    `json:"inserted"`
	Record             registry.ManifestRecord `json:"record"`
}

// ManifestNode supplies trusted observations; verified is always produced by
// the registry's own proof verification, never supplied by an HTTP client.
type ManifestNode interface {
	Resolve(context.Context, prospectnode.Policy) (prospect.Context, error)
	RecheckContext(context.Context, prospect.Context) error
	RecheckAdmission(context.Context, prospect.Context, prospect.Proof, prospect.Verified) (string, error)
}
