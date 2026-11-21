// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

const maxDailyBindingBody = 128 << 10

// DailySettlementBinding requires the original writable daily Store. It is
// configuration correlation, not a database identity or payment capability.
func DailySettlementBinding(ctx context.Context, original *poolsettle.Store) (string, error) {
	if wire.TestPoW {
		return "", ErrProfile
	}
	if ctx == nil || original == nil {
		return "", ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	config, source, err := original.DailySchedulerBinding(work)
	if err != nil {
		return "", err
	}
	id, err := dailyBinding(config, source)
	if err != nil {
		return "", err
	}
	if err := work.Err(); err != nil {
		return "", err
	}
	return id, nil
}

// Only DailySettlementBinding calls this with the original audited owner.
// Bounded pure component validation is not a substitute for that ownership.
func dailyBinding(config poolsettle.Config, source poolshare.Config) (string, error) {
	if wire.TestPoW {
		return "", ErrProfile
	}
	if _, err := poolsettle.DailyConfigBindingID(config); err != nil {
		return "", ErrInvalid
	}
	if _, err := poolshare.DailyConfigID(source); err != nil {
		return "", ErrInvalid
	}
	c, err := json.Marshal(config)
	if err != nil || len(c) == 0 || len(c) > 65536 {
		return "", ErrInvalid
	}
	s, err := json.Marshal(source)
	if err != nil || len(s) == 0 || len(s) > 8192 {
		return "", ErrInvalid
	}
	// Both component sizes are bounded before addition and body allocation.
	size := len(c) + len(s) + 21
	if size > maxDailyBindingBody || uint64(size) > uint64(^uint32(0)) {
		return "", ErrInvalid
	}
	body := make([]byte, 0, size)
	body = append(body, []byte(`{"config":`)...)
	body = append(body, c...)
	body = append(body, []byte(`,"source":`)...)
	body = append(body, s...)
	body = append(body, '}')
	if len(body) != size {
		return "", ErrInvalid
	}
	h := sha256.New()
	h.Write([]byte("SLUG pool settlement binding\x00v2\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(size))
	h.Write(n[:])
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CreateDaily selects a fresh daily schedule and the original concrete daily
// Store. It never imports or rewrites a V1 scheduler root.
func CreateDaily(ctx context.Context, path string, expected Config, original *poolsettle.Store) (*Scheduler, error) {
	return openSelected(ctx, path, expected, original, true, Version2)
}

func OpenDaily(ctx context.Context, path string, expected Config, original *poolsettle.Store) (*Scheduler, error) {
	return openSelected(ctx, path, expected, original, false, Version2)
}

func (s *Scheduler) selectedVersion() uint32 {
	if s.daily {
		return Version2
	}
	return Version1
}

func (s *Scheduler) settlementBinding(ctx context.Context) (string, error) {
	if s.daily {
		return DailySettlementBinding(ctx, s.store)
	}
	return SettlementBinding(ctx, s.store)
}
