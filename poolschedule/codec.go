// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"time"
)

func identifier(s string) bool {
	if len(s) != 64 || s == "0000000000000000000000000000000000000000000000000000000000000000" {
		return false
	}
	raw, e := hex.DecodeString(s)
	return e == nil && hex.EncodeToString(raw) == s
}
func validConfig(c Config) bool {
	return c.Version == Version1 && identifier(c.SchedulerID) && identifier(c.RuntimeID) && identifier(c.SettlementBindingID) && c.DailyUTCMinute <= 1439 && c.RetryMS >= 1000 && c.RetryMS <= 3600000
}
func date(ms int64) string { return time.UnixMilli(ms).UTC().Format("2006-01-02") }
func validDay(s string) bool {
	if len(s) != 10 {
		return false
	}
	t, e := time.Parse("2006-01-02", s)
	return e == nil && t.Year() >= 1 && t.Format("2006-01-02") == s
}
func validReason(s string) bool {
	switch s {
	case "", "payout-pending", "broadcast-unknown", "transaction-unavailable", "payout-conflict", "funding-unavailable", "funding-conflict", "funding-changed", "immature", "awaiting-confirmation", "below-minimum", "destination-unavailable", "daily-batch-complete", "fee-unavailable", "fee-budget", "change-dust", "signing-unavailable", "capacity", "transaction-capacity":
		return true
	}
	return false
}
func validReport(state, reason string) bool {
	return (state == "complete" || state == "pending" || state == "deferred") && validReason(reason)
}
func validError(s string) bool {
	switch s {
	case "busy", "unavailable", "clock", "storage", "closed", "profile", "invalid", "cancelled", "deadline", "other":
		return true
	}
	return false
}
func validState(s State) bool {
	if s.Version != Version1 || !validConfig(s.Config) || s.Revision == 0 || s.LastClockMS < 1 || s.LastClockMS > MaxTimeMS {
		return false
	}
	if s.DailyCursor != "" && (!validDay(s.DailyCursor) || s.DailyCursor > date(s.LastClockMS)) {
		return false
	}
	if s.NextWakeMS < 0 || s.NextWakeMS > MaxTimeMS || (s.NextWakeMS != 0 && s.NextWakeMS <= s.LastClockMS) {
		return false
	}
	a := s.Attempt
	if a == nil {
		return s.Revision == 1 && s.DailyCursor == "" && s.NextWakeMS == 0
	}
	if a.Sequence == 0 || a.Sequence > s.Revision || (a.Kind != "daily" && a.Kind != "recovery") || a.StartedAtMS < 1 || a.StartedAtMS > s.LastClockMS || a.AdmittedDay != date(a.StartedAtMS) {
		return false
	}
	if s.NextWakeMS == 0 && !(a.Kind == "recovery" && a.Result == "running") {
		return false
	}
	if a.Result == "running" {
		return a.FinishedAtMS == 0 && a.ReportState == "" && a.ReportReason == "" && a.ErrorCode == ""
	}
	if a.FinishedAtMS < a.StartedAtMS || a.FinishedAtMS > s.LastClockMS {
		return false
	}
	switch a.Result {
	case "returned":
		return a.ErrorCode == "" && validReport(a.ReportState, a.ReportReason)
	case "failed":
		return a.ReportState == "" && a.ReportReason == "" && validError(a.ErrorCode)
	}
	return false
}

// boundedJSON refuses duplicate keys, arrays and over-budget input before typed
// decoding. Exact canonical re-encoding additionally rejects aliases, field
// reordering, omitted/unknown fields and alternate number spellings.
func boundedJSON(raw []byte, max int, out any) error {
	if len(raw) == 0 || len(raw) > max {
		return ErrInvalid
	}
	for _, b := range raw {
		if b > 127 {
			return ErrInvalid
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	tokens := 0
	var walk func(int) error
	walk = func(depth int) error {
		tokens++
		if tokens > 128 || depth > 4 {
			return ErrInvalid
		}
		t, e := d.Token()
		if e != nil {
			return ErrInvalid
		}
		switch x := t.(type) {
		case json.Delim:
			if x != '{' {
				return ErrInvalid
			}
			seen := map[string]bool{}
			members := 0
			for d.More() {
				members++
				tokens++
				if members > 16 || tokens > 128 {
					return ErrInvalid
				}
				k, e := d.Token()
				if e != nil {
					return ErrInvalid
				}
				key, ok := k.(string)
				if !ok || len(key) > 64 || seen[key] {
					return ErrInvalid
				}
				for _, r := range key {
					if r > 127 {
						return ErrInvalid
					}
				}
				seen[key] = true
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return ErrInvalid
			}
		case string:
			if len(x) > 64 {
				return ErrInvalid
			}
			for _, r := range x {
				if r > 127 {
					return ErrInvalid
				}
			}
		case json.Number:
			if len(x) > 16 {
				return ErrInvalid
			}
		case nil, bool:
		default:
			return ErrInvalid
		}
		return nil
	}
	if walk(0) != nil {
		return ErrInvalid
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrInvalid
	}
	typed := json.NewDecoder(bytes.NewReader(raw))
	typed.DisallowUnknownFields()
	if typed.Decode(out) != nil {
		return ErrInvalid
	}
	canonical, e := json.Marshal(out)
	if e != nil || !bytes.Equal(raw, canonical) {
		return ErrInvalid
	}
	return nil
}
func encodeMarker(c Config) ([]byte, error) {
	if !validConfig(c) {
		return nil, ErrInvalid
	}
	raw, e := json.Marshal(c)
	if e != nil || len(raw)+1 > maxMarkerBytes {
		return nil, ErrInvalid
	}
	return append(raw, '\n'), nil
}
func decodeMarker(raw []byte) (Config, error) {
	var c Config
	if len(raw) == 0 || len(raw) > maxMarkerBytes || raw[len(raw)-1] != '\n' || boundedJSON(raw[:len(raw)-1], maxMarkerBytes-1, &c) != nil || !validConfig(c) {
		return Config{}, ErrInvalid
	}
	return c, nil
}
func encodeState(s State) ([]byte, error) {
	if !validState(s) {
		return nil, ErrInvalid
	}
	raw, e := json.Marshal(s)
	if e != nil || len(raw) > maxStateBytes {
		return nil, ErrInvalid
	}
	return raw, nil
}
func decodeState(raw []byte) (State, error) {
	var s State
	if boundedJSON(raw, maxStateBytes, &s) != nil || !validState(s) {
		return State{}, ErrInvalid
	}
	return s, nil
}
