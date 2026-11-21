// Copyright (c) 2026 The SLUG developers. ISC license.
package poolingress

// ValidateDailyConfig selects only the explicit public Config2 routing catalog.
func ValidateDailyConfig(c Config) error { _, _, err := validateSelectedConfig(c, 2); return err }

// NewDaily owns the same narrow TLS lifetime and exact private upstreams as
// New, with only pool/v2 replacing pool/v1. Admission/ownership routes stay exact.
func NewDaily(c Config, certificatePEM, privateKeyPEM []byte) (*Server, error) {
	return newSelected(c, certificatePEM, privateKeyPEM, 2)
}
