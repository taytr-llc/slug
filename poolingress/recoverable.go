// Copyright (c) 2026 The SLUG developers. ISC license.
package poolingress

// The recoverable operator wrapper selects this route catalog explicitly. The
// underlying TLS/upstream Config2 bytes and all limits remain unchanged.
func ValidateRecoverableDailyConfig(c Config) error { return ValidateDailyConfig(c) }
func NewRecoverableDaily(c Config, certificatePEM, privateKeyPEM []byte) (*Server, error) {
	s, e := newSelected(c, certificatePEM, privateKeyPEM, 2)
	if e != nil {
		return nil, e
	}
	s.protocol = 3
	return s, nil
}
