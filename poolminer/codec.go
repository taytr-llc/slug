// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

import (
	"bytes"
	"encoding/json"
	"io"
)

func validTime(n int64) bool { return n >= 1 && n <= MaxTimeMS }
func validState(s State) bool {
	if s.Version != Version1 || !validConfig(s.Config) || s.Revision == 0 || !validTime(s.ClockMS) || s.Credentials == nil || s.History == nil || len(s.Credentials) > 128 || len(s.History) > maxHistory || uint64(len(s.History)) > uint64(s.Revision)-1 {
		return false
	}
	gyms := map[string]bool{}
	for _, en := range s.Config.Enrollments {
		gyms[en.GymID] = true
	}
	pairs := map[TokenUse]bool{}
	hashes, reuses := map[string]bool{}, map[string]bool{}
	for _, x := range s.History {
		if !identifier(x.TokenHash) || !identifier(x.ReuseHash) || hashes[x.TokenHash] || reuses[x.ReuseHash] {
			return false
		}
		pairs[x] = true
		hashes[x.TokenHash] = true
		reuses[x.ReuseHash] = true
	}
	last := ""
	used := map[TokenUse]bool{}
	total := uint64(0)
	for _, c := range s.Credentials {
		order := c.GymID + ":" + c.Audience
		pair := TokenUse{c.TokenHash, c.ReuseHash}
		if !gyms[c.GymID] || !audience(c.Audience) || order <= last || c.Generation == 0 || c.Generation > maxHistory || !validTime(c.IssuedAtMS) || c.IssuedAtMS > s.ClockMS || !validTime(c.ExpiresAtMS) || c.ExpiresAtMS <= c.IssuedAtMS || c.ExpiresAtMS-c.IssuedAtMS > MaxLifetimeMS || c.RevokedAtMS != 0 && (c.RevokedAtMS < c.IssuedAtMS || c.RevokedAtMS > s.ClockMS) || !pairs[pair] || used[pair] {
			return false
		}
		total += uint64(c.Generation)
		used[pair] = true
		last = order
	}
	return total == uint64(len(s.History))
}
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
		if depth > 8 || tokens > 65536 {
			return ErrInvalid
		}
		t, e := d.Token()
		if e != nil {
			return ErrInvalid
		}
		switch x := t.(type) {
		case json.Delim:
			if x != '{' && x != '[' {
				return ErrInvalid
			}
			seen := map[string]bool{}
			count := 0
			for d.More() {
				count++
				if x == '{' {
					if count > 16 {
						return ErrInvalid
					}
					tokens++
					k, e := d.Token()
					s, ok := k.(string)
					if e != nil || !ok || len(s) > 64 || seen[s] {
						return ErrInvalid
					}
					seen[s] = true
					for _, b := range s {
						if b > 127 {
							return ErrInvalid
						}
					}
				} else if count > 4096 {
					return ErrInvalid
				}
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || x == '{' && end != json.Delim('}') || x == '[' && end != json.Delim(']') {
				return ErrInvalid
			}
		case string:
			if len(x) > 128 {
				return ErrInvalid
			}
			for _, b := range x {
				if b > 127 {
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
	if len(raw) < 2 || len(raw) > maxMarkerBytes || raw[len(raw)-1] != '\n' || boundedJSON(raw[:len(raw)-1], maxMarkerBytes-1, &c) != nil || !validConfig(c) {
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
		return nil, ErrCapacity
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
