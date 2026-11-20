// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
)

func journalHex(value string, size int, nonzero bool) bool {
	if len(value) != size*2 {
		return false
	}
	raw, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(raw) != value {
		return false
	}
	if nonzero {
		for _, b := range raw {
			if b != 0 {
				return true
			}
		}
		return false
	}
	return true
}
func journalTime(t int64) bool         { return t > 0 && t <= 253402300799999 }
func journalTerminal(v Operation) bool { return v.Stage == "complete" || v.Stage == "abandoned" }
func journalMarkerFor(i Identity, binding string) journalMarker {
	return journalMarker{journalApplication, 1, i.JournalID, i.Pin, binding}
}
func decodeJournalMarker(raw []byte, out *journalMarker) error {
	if len(raw) < 2 || len(raw) > 4096 || raw[len(raw)-1] != '\n' {
		return ErrCorrupt
	}
	return decodeJournal(raw[:len(raw)-1], 4095, out)
}
func sameJournal(a, b any) bool {
	x, e := json.Marshal(a)
	if e != nil {
		return false
	}
	y, e := json.Marshal(b)
	return e == nil && bytes.Equal(x, y)
}
func cloneJournalOperation(v Operation) Operation {
	// This only copies an already bounded, validated retained value. Decoding
	// untrusted state always uses the lexical prewalk before DTO allocation.
	raw, err := json.Marshal(v)
	if err != nil {
		panic("non-JSON carrier operation")
	}
	var out Operation
	if json.Unmarshal(raw, &out) != nil {
		panic("non-roundtripping carrier operation")
	}
	return out
}
func validateJournalState(ctx context.Context, value journalState, i Identity, binding string) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if value.Application != journalApplication || value.Version != 1 || value.JournalID != i.JournalID || value.BindingID != binding || value.Records == nil || len(value.Records) > journalMaxRecords {
		return ErrCorrupt
	}
	if len(value.Records) == 0 {
		if value.ClockHighWaterMS != 0 {
			return ErrCorrupt
		}
	} else if !journalTime(value.ClockHighWaterMS) {
		return ErrCorrupt
	}
	seen := make(map[string]bool, len(value.Records))
	active := 0
	for n, v := range value.Records {
		if err := ctx.Err(); err != nil {
			return err
		}
		if v.Sequence != uint32(n+1) || seen[v.RequestID] || v.UpdatedAtMS > value.ClockHighWaterMS {
			return ErrCorrupt
		}
		if err := ValidateOperation(v, i.Pin, binding); err != nil {
			return ErrCorrupt
		}
		seen[v.RequestID] = true
		if !journalTerminal(v) {
			active++
		}
	}
	if active > 1 {
		return ErrCorrupt
	}
	return nil
}
