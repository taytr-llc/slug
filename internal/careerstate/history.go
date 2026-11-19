// Copyright (c) 2026 The SLUG developers. ISC license.
package careerstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/taytr-llc/slug/fightercareer"
)

const MaxHistoryRowSize = 1536

type HistoryRow struct {
	Ordinal     uint32               `json:"ordinal"`
	Event       Event                `json:"event"`
	Earned      fightercareer.Delta  `json:"earned"`
	XPBefore    uint32               `json:"xp_before"`
	XPApplied   uint32               `json:"xp_applied"`
	TotalsAfter fightercareer.Totals `json:"totals_after"`
}

// History is minted only by an installed Reader. It retains one canonical
// display-order buffer and offsets, never the owner's global event graph.
type History struct {
	snapshot    Snapshot
	prefixCount uint32
	rows        []byte
	offsets     []uint32
}

func (r *Reader) CaptureHistory(ctx context.Context, prospectID string, asOfMS int64) (*History, error) {
	if r == nil || r.read == nil {
		return nil, ErrInactive
	}
	if !ID(prospectID) || !stamp(asOfMS) {
		return nil, ErrInvalid
	}
	state, err := r.state(ctx)
	if err != nil {
		return nil, err
	}
	if asOfMS < state.Activation.ActivatedAtMS {
		return nil, ErrInvalid
	}
	events, err := ValidateIndex(state.Activation, state.Events)
	if err != nil {
		return nil, err
	}
	// Validate the whole owned index, even future and unrelated entries.
	for _, e := range events {
		if e.EffectiveAtMS > state.ClockMS {
			return nil, ErrCorrupt
		}
	}
	prefix := events[:0]
	for _, e := range events {
		if e.EffectiveAtMS <= asOfMS {
			prefix = append(prefix, e)
		}
	}
	hash := sha256.New()
	hash.Write([]byte("SLUG candidate career prefix\x00v1\x00"))
	hash.Write(raw(state.Activation.ActivationID))
	hash.Write(u64(asOfMS))
	hash.Write(lp([]byte(Current)))
	hash.Write(u32(1))
	hash.Write(raw(prospectID))
	hash.Write(u32(uint32(len(prefix))))
	rows := make([]HistoryRow, 0, len(prefix))
	total := fightercareer.Totals{}
	for _, e := range prefix {
		b, _ := json.Marshal(e)
		hash.Write(lp(b))
		corner := -1
		if e.CornerA.ProspectID == prospectID {
			corner = 0
		} else if e.CornerB.ProspectID == prospectID {
			corner = 1
		}
		if corner < 0 {
			continue
		}
		d, err := deltas(e)
		if err != nil {
			return nil, ErrCorrupt
		}
		before := total.XP
		total, err = fightercareer.Apply(total, d[corner])
		if err != nil {
			return nil, ErrCorrupt
		}
		rows = append(rows, HistoryRow{uint32(len(rows) + 1), e, d[corner], before, total.XP - before, total})
	}
	h := &History{snapshot: Snapshot{activation: state.Activation, asOf: asOfMS, mode: Current,
		ids: []string{prospectID}, digest: hex.EncodeToString(hash.Sum(nil)), totals: map[string]fightercareer.Totals{prospectID: total}},
		prefixCount: uint32(len(prefix)), rows: []byte{'['}, offsets: make([]uint32, 0, len(rows)+1)}
	for i := len(rows) - 1; i >= 0; i-- {
		if i != len(rows)-1 {
			h.rows = append(h.rows, ',')
		}
		h.offsets = append(h.offsets, uint32(len(h.rows)))
		b, err := json.Marshal(rows[i])
		if err != nil || len(b) > MaxHistoryRowSize {
			return nil, ErrCorrupt
		}
		h.rows = append(h.rows, b...)
	}
	h.offsets = append(h.offsets, uint32(len(h.rows)))
	h.rows = append(h.rows, ']')
	sealed := make([]byte, len(h.rows))
	copy(sealed, h.rows)
	h.rows = sealed
	if ctx.Err() != nil {
		return nil, ErrCancelled
	}
	return h, nil
}

func (h *History) Snapshot() Snapshot {
	if h == nil {
		return Snapshot{}
	}
	return h.snapshot
}
func (h *History) PrefixCount() uint32 {
	if h == nil {
		return 0
	}
	return h.prefixCount
}
func (h *History) EventCount() uint32 {
	if h == nil || len(h.offsets) == 0 {
		return 0
	}
	return uint32(len(h.offsets) - 1)
}

// RetainedBytes accounts for the buffer and its exact four-byte offsets. The
// outer Stable token separately accounts for all fixed owner/header metadata.
func (h *History) RetainedBytes() int {
	if h == nil {
		return 0
	}
	return len(h.rows) + 4*len(h.offsets)
}
func (h *History) Rows(after, limit uint32) ([]HistoryRow, error) {
	if h == nil || len(h.offsets) == 0 || after > h.EventCount() || limit < 1 || limit > 8 {
		return nil, ErrInvalid
	}
	end := min(h.EventCount(), after+limit)
	out := make([]HistoryRow, 0, end-after)
	for i := after; i < end; i++ {
		start, stop := h.offsets[i], h.offsets[i+1]
		if i+1 < h.EventCount() {
			stop--
		} // skip the separator before the next row
		var row HistoryRow
		if json.Unmarshal(h.rows[start:stop], &row) != nil {
			return nil, ErrCorrupt
		}
		out = append(out, row)
	}
	return out, nil
}
