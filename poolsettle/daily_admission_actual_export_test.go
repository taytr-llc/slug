//go:build slugdailyactual && !slugtestpow && (linux || (darwin && cgo))

package poolsettle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// DailyAdmissionActualEvidence is bounded public evidence from the original
// fully audited owner. It grants no operation, raw DB or signing capability.
type DailyAdmissionActualEvidence struct {
	BodySHA256 string   `json:"body_sha256"`
	BodyBytes  int      `json:"body_bytes"`
	Credits    []Credit `json:"credits"`
}

func DailyAdmissionActualRead(ctx context.Context, s *Store) (DailyAdmissionActualEvidence, error) {
	if s == nil || s.daily == nil {
		return DailyAdmissionActualEvidence{}, ErrInvalid
	}
	end, err := s.begin(ctx)
	if err != nil {
		return DailyAdmissionActualEvidence{}, err
	}
	defer end()
	v, err := s.auditDaily(ctx)
	if err != nil {
		return DailyAdmissionActualEvidence{}, err
	}
	if v.head.Credits > 2 || v.head.Batches != 1 || v.head.Events > 3 {
		return DailyAdmissionActualEvidence{}, ErrCapacity
	}
	raw, err := s.db.Get(dailyNumber("b/", 1), nil)
	if err != nil || len(raw) == 0 || len(raw) > dailyBatchLimit {
		return DailyAdmissionActualEvidence{}, ErrCorrupt
	}
	h := sha256.Sum256(raw)
	out := DailyAdmissionActualEvidence{BodySHA256: hex.EncodeToString(h[:]), BodyBytes: len(raw), Credits: []Credit{}}
	for seq := uint64(1); seq <= v.head.Credits; seq++ {
		if err = ctx.Err(); err != nil {
			return DailyAdmissionActualEvidence{}, err
		}
		raw, e := s.db.Get(dailyNumber("c/", seq), nil)
		var credit Credit
		if e != nil || dailyDecode(raw, dailyCreditLimit, &credit) != nil {
			return DailyAdmissionActualEvidence{}, ErrCorrupt
		}
		out.Credits = append(out.Credits, clone(credit))
	}
	if err = ctx.Err(); err != nil {
		return DailyAdmissionActualEvidence{}, err
	}
	return out, nil
}
