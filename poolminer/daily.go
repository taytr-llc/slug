// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

import "context"

// ValidateDailyConfig checks public daily bindings only. Concrete authority and
// session custody remain with the independently selected original owners.
func ValidateDailyConfig(c Config, b Binding) error {
	return validateConfigVersion(c, b, Version2)
}
func DailyConfigID(c Config, b Binding) (string, error) {
	if err := ValidateDailyConfig(c, b); err != nil {
		return "", err
	}
	return configDigestVersion(c, Version2)
}

// CreateDaily creates a fresh daily credential journal. It never adopts V1.
func CreateDaily(ctx context.Context, path string, expected Config, b Binding) (*Store, error) {
	return openVersion(ctx, path, expected, b, true, true)
}
func OpenDaily(ctx context.Context, path string, expected Config, b Binding) (*Store, error) {
	return openVersion(ctx, path, expected, b, false, true)
}
func (s *Store) version() uint32 {
	if s.daily {
		return Version2
	}
	return Version1
}
