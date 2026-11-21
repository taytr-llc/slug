// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

// Package prospectservice transports D68 jobs and immutable birth receipts
// between an explicitly trusted local authority and gym clients.
package prospectservice

import "github.com/taytr-llc/slug/registry"

const (
	Version1            uint32 = 1
	Version2            uint32 = 2
	MaxRequestSize             = 65536
	MaxResponseSize            = 73728
	MaxServerConfigSize        = 65536
	MaxClientConfigSize        = 65536
)

// Credential grants API access to one public gym key. TokenSHA256 is SHA256
// of the decoded 32-byte bearer token, never of its hexadecimal text.
type Credential struct {
	PublicKey   string `json:"public_key"`
	TokenSHA256 string `json:"token_sha256"`
}

// ServerConfig bounds a private authority's listener, clients and durable
// storage. Capacities are operator resource limits, not product roster caps.
type ServerConfig struct {
	Version    uint32       `json:"version"`
	Listen     string       `json:"listen"`
	Clients    []Credential `json:"clients"`
	MaxJobs    uint64       `json:"max_jobs"`
	MaxRecords uint64       `json:"max_records"`
}

// ClientConfig contains an API credential, not a signing or node-RPC key.
// A nonempty CA certificate replaces system roots with the supplied PEM set.
type ClientConfig struct {
	Version       uint32 `json:"version"`
	URL           string `json:"url"`
	Token         string `json:"token"`
	CACertificate string `json:"ca_certificate"`
}

// AdmissionResult distinguishes a new node observation from recovery of the
// original receipt. Historical recovery never renews eligibility or age.
type AdmissionResult struct {
	Version            uint32          `json:"version"`
	ContextSource      string          `json:"context_source"`
	CurrentNodeChecked bool            `json:"current_node_checked"`
	Authentication     string          `json:"authentication"`
	BirthRule          string          `json:"birth_rule"`
	Inserted           bool            `json:"inserted"`
	Record             registry.Record `json:"record"`
}

// APIError contains only stable public codes. An uncertain outcome requires
// replaying the identical proof and authorization to recover the first receipt.
type APIError struct {
	Code           string `json:"code"`
	OutcomeUnknown bool   `json:"outcome_unknown"`
}

type ErrorResponse struct {
	Version uint32   `json:"version"`
	Error   APIError `json:"error"`
}
