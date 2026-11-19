// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterevidence

import (
	"context"
	"errors"
	"sync"

	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

var verifyGate sync.Mutex

// CheckProfile permits callers such as the offline CLI to refuse an
// unsupported build before opening any input files.
func CheckProfile() error {
	if wire.TestPoW {
		return ErrProfile
	}
	return nil
}

// VerifySupplied performs exactly one normal proof verification on success.
// The source pin is mandatory and independent of the claimant's archive.
func VerifySupplied(ctx context.Context, expected gymstable.Pin, raw []byte) (Report, error) {
	return verifySupplied(ctx, expected, raw, prospect.Verify)
}

// The private work argument is a finite test seam, never a public preverified
// bypass. The sole production caller above always supplies prospect.Verify.
func verifySupplied(ctx context.Context, expected gymstable.Pin, raw []byte, work func(prospect.Context, prospect.Proof) (prospect.Verified, error)) (Report, error) {
	if err := CheckProfile(); err != nil {
		return Report{}, err
	}
	if ctx == nil {
		return Report{}, ErrInvalid
	}
	if !verifyGate.TryLock() {
		return Report{}, ErrBusy
	}
	defer verifyGate.Unlock()
	cancelled := func() error {
		if err := ctx.Err(); err != nil {
			return errors.Join(ErrCancelled, err)
		}
		return nil
	}
	if err := cancelled(); err != nil {
		return Report{}, err
	}
	if len(raw) == 0 || len(raw) > MaxSize {
		return Report{}, ErrInvalid
	}
	// Retain the exact input bytes through entered work and final checksum.
	raw = append([]byte(nil), raw...)
	a, err := Parse(raw)
	if err != nil {
		return Report{}, err
	}
	a, err = ValidateSuppliedShape(expected, a)
	if err != nil {
		return Report{}, err
	}
	if err = cancelled(); err != nil {
		return Report{}, err
	}
	v, err := work(a.Admission.Context, a.Admission.Proof)
	// The entered work call is joined, even when its original request was
	// cancelled while Argon was running. No detached worker or partial report.
	if cancelErr := cancelled(); cancelErr != nil {
		return Report{}, cancelErr
	}
	if err != nil || v != a.Admission.Verified || registry.ValidateEligibility(a.Admission.Context, v, a.Admission.Eligibility) != nil {
		return Report{}, ErrInvalid
	}
	report := Report{Version: Version, Status: "consistent_supplied_evidence", FileSHA256: checksum(raw), FileBytes: len(raw),
		SourceStoreID: a.Capture.Status.StoreID, ProspectID: v.ProspectID, HistoryID: a.Capture.HistoryID,
		CapturedAsOfMS: a.Capture.AsOfMS, EventCount: a.Capture.EventCount,
		Checks: Checks{true, true, true, true, true, true}, Unverified: Unverified{true, true, true, true, true}}
	if err = cancelled(); err != nil {
		return Report{}, err
	}
	return report, nil
}
