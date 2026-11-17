// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterbout

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
)

func canonicalJSON(v any, limit int) ([]byte, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	raw := b.Bytes()
	if len(raw) < 2 || len(raw)-1 > limit || raw[len(raw)-1] != '\n' {
		return nil, ErrResult
	}
	return raw[:len(raw)-1], nil
}

// inspectJSON performs byte/depth/container/count checks before decoding into
// slices. All canonical strings are short ASCII without escapes. This rejects
// alternate spellings before they can be mistaken for a new JSON field.
func inspectJSON(raw []byte, limit int) error {
	if len(raw) == 0 || len(raw) > limit || raw[0] != '{' {
		return ErrResult
	}
	depth, quoted := 0, false
	for _, c := range raw {
		if c > 127 || c < 32 || c == '\\' {
			return ErrResult
		}
		if c == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch c {
		case '{', '[':
			depth++
			if depth > MaxJSONDepth {
				return ErrResult
			}
		case '}', ']':
			depth--
			if depth < 0 {
				return ErrResult
			}
		case ' ':
			return ErrResult
		}
	}
	if depth != 0 || quoted {
		return ErrResult
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := inspectValue(d, 1, ""); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrResult
	}
	return nil
}

func inspectValue(d *json.Decoder, depth int, field string) error {
	value, err := d.Token()
	if err != nil {
		return ErrResult
	}
	switch v := value.(type) {
	case json.Delim:
		if depth > MaxJSONDepth {
			return ErrResult
		}
		if v == '{' {
			seen := make(map[string]bool, 16)
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || len(name) > 32 || seen[name] || len(seen) == 16 {
					return ErrResult
				}
				seen[name] = true
				if err := inspectValue(d, depth+1, name); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return ErrResult
			}
			return nil
		}
		if v != '[' {
			return ErrResult
		}
		limit, exact := 0, false
		switch field {
		case "rounds":
			limit = int(Rounds)
		case "ticks":
			limit = int(TicksPerRound)
		case "initial", "final", "scores", "states", "end", "corner_stopped":
			limit, exact = 2, true
		default:
			return ErrResult
		}
		count := 0
		for d.More() {
			if count >= limit {
				return ErrResult
			}
			count++
			if err := inspectValue(d, depth+1, field); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') || count == 0 || exact && count != 2 {
			return ErrResult
		}
	case string:
		if len(v) > 128 {
			return ErrResult
		}
	case json.Number:
		s := string(v)
		parsed, err := strconv.ParseInt(s, 10, 64)
		if err != nil || strconv.FormatInt(parsed, 10) != s ||
			parsed < 0 && field != "form_q" && field != "shoulder_jitter" {
			return ErrResult
		}
	case bool:
		if field != "hit" && field != "significant" && field != "knockdown" && field != "corner_stopped" {
			return ErrResult
		}
	case nil:
		if field != "recovery" {
			return ErrResult
		}
	default:
		return ErrResult
	}
	return nil
}

// MarshalInput returns canonical, compact version1 bytes after regeneration.
func MarshalInput(in Input) ([]byte, error) {
	if _, err := ValidateInput(in); err != nil {
		return nil, err
	}
	raw, err := canonicalJSON(in, MaxInputBytes)
	if err != nil {
		return nil, ErrInput
	}
	return raw, nil
}

// ParseInput accepts only canonical bytes, including corner and field order.
func ParseInput(raw []byte) (Input, error) {
	if inspectJSON(raw, MaxInputBytes) != nil {
		return Input{}, ErrInput
	}
	var in Input
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		return Input{}, ErrInput
	}
	want, err := MarshalInput(in)
	if err != nil || !bytes.Equal(raw, want) {
		return Input{}, ErrInput
	}
	return in, nil
}

func resultShape(r Result) error {
	if r.Version != Version1 || r.EngineID != EngineID || !canonicalHash(r.InputID) ||
		(r.Winner != "" && !canonicalHash(r.Winner)) || len(r.Method) > 32 ||
		len(r.Rounds) == 0 || len(r.Rounds) > int(Rounds) {
		return ErrResult
	}
	for _, round := range r.Rounds {
		if len(round.Ticks) == 0 || len(round.Ticks) > int(TicksPerRound) {
			return ErrResult
		}
		for _, tick := range round.Ticks {
			if len(tick.Strike) > 16 || len(tick.Stoppage) > 32 {
				return ErrResult
			}
		}
	}
	return nil
}

// ValidateResult independently regenerates the complete result from Input.
// The returned transcript owns its arrays and cannot alias caller mutations.
func ValidateResult(ctx context.Context, in Input, supplied Result) (Result, error) {
	if ctx == nil {
		return Result{}, ErrInput
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if resultShape(supplied) != nil {
		return Result{}, ErrResult
	}
	want, err := Simulate(ctx, in)
	if err != nil {
		return Result{}, err
	}
	if !reflect.DeepEqual(want, supplied) {
		return Result{}, ErrResult
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return want, nil
}

// MarshalResult verifies by regeneration before returning canonical bytes.
func MarshalResult(ctx context.Context, in Input, r Result) ([]byte, error) {
	want, err := ValidateResult(ctx, in, r)
	if err != nil {
		return nil, err
	}
	raw, err := canonicalJSON(want, MaxResultBytes)
	if err != nil {
		return nil, ErrResult
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return raw, nil
}

// ParseResult bounds all arrays before decoding and accepts exact bytes only.
func ParseResult(ctx context.Context, in Input, raw []byte) (Result, error) {
	if ctx == nil {
		return Result{}, ErrInput
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if inspectJSON(raw, MaxResultBytes) != nil {
		return Result{}, ErrResult
	}
	var r Result
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil {
		return Result{}, ErrResult
	}
	want, err := ValidateResult(ctx, in, r)
	if err != nil {
		return Result{}, err
	}
	encoded, err := canonicalJSON(want, MaxResultBytes)
	if err != nil || !bytes.Equal(encoded, raw) {
		return Result{}, ErrResult
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return want, nil
}

// Digest hashes the complete regenerated canonical result; it is not a
// signature or a claim about the authority of the supplied seed or fighters.
func Digest(ctx context.Context, in Input, r Result) (string, error) {
	raw, err := MarshalResult(ctx, in, r)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(resultDomain))
	h.Write(raw)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
