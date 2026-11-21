// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
)

const Version2 uint32 = 2

// validateDailyConfig selects the fresh daily profile. The unchanged V1
// validator remains the only validator used by legacy constructors and IDs.
func validateDailyConfig(c Config) error {
	if c.Version != Version2 || c.MaxRounds == 0 || c.MaxRounds > 16384 || c.MaxBatches == 0 || c.MaxBatches > 32767 {
		return ErrConfig
	}
	// Reuse the exact destination/script grammar without widening V1 limits.
	base := c
	base.Version, base.MaxRounds, base.MaxBatches = Version1, 1, 1
	return validateConfig(base)
}

// DailyConfigBindingID is public configuration correlation, never a physical
// journal identity or spending capability. It rejects all legacy configurations.
func DailyConfigBindingID(c Config) (string, error) {
	if validateDailyConfig(c) != nil {
		return "", ErrConfig
	}
	raw, err := json.Marshal(c)
	if err != nil || len(raw) > 65536 {
		return "", ErrConfig
	}
	h := sha256.New()
	h.Write([]byte("SLUG operator pool settlement config\x00v2\x00"))
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(raw)))
	h.Write(size[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}
