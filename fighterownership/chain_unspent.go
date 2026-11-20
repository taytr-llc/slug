// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
)

// chainAtoms converts the node's bounded JSON coin amount without float64
// rounding. Scientific notation is accepted, but fractional atoms are not.
func chainAtoms(raw []byte) (int64, bool) {
	if len(raw) == 0 || len(raw) > 32 || raw[0] < '0' || raw[0] > '9' || !json.Valid(raw) {
		return 0, false
	}
	s := string(raw)
	exponent := 0
	if at := strings.IndexAny(s, "eE"); at >= 0 {
		n, err := strconv.ParseInt(s[at+1:], 10, 8)
		if err != nil || n < -18 || n > 18 {
			return 0, false
		}
		exponent, s = int(n), s[:at]
	}
	if at := strings.IndexByte(s, '.'); at >= 0 {
		exponent -= len(s) - at - 1
		s = s[:at] + s[at+1:]
	}
	n, err := strconv.ParseUint(s, 10, 63)
	if err != nil {
		return 0, false
	}
	for scale := 8 + exponent; scale != 0; {
		if scale > 0 {
			if n > math.MaxInt64/10 {
				return 0, false
			}
			n *= 10
			scale--
		} else {
			if n%10 != 0 {
				return 0, false
			}
			n /= 10
			scale++
		}
	}
	return int64(n), true
}

// chainOutputAbsent is private: an arbitrary caller's unknown outpoint is not
// proof of a spent escrow. PreparedRecovery adds the canonical commit authority.
var chainOutputAbsent = errors.New("owned node output currently absent")

// CheckUnspent freshly checks an exact confirmed output, including mempool
// spends, within one unchanged node observation. It does not reserve funds or
// establish ownership. The caller must also retain and check its original
// replay head; a later spend can always make a subsequent action unavailable.
func (s *OwnedChainSource) CheckUnspent(ctx context.Context, txid string, index uint32, atoms int64, script []byte) error {
	if !chainHex(txid, 32, false) || atoms <= 0 || len(script) == 0 || len(script) > 10000 {
		return ErrInvalid
	}
	wantScript := hex.EncodeToString(script)
	op, done, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	if err = s.identity(op); err != nil {
		return err
	}
	before, err := s.observe(op)
	if err != nil {
		return err
	}
	raw, err := s.call(op, "gettxout", []any{txid, index, true}, 65536)
	if err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		// Even absence is an original-node observation, not an RPC error or a
		// shortcut around restart/ABA/network checks.
		after, e := s.observe(op)
		if e != nil {
			return e
		}
		if after != before {
			return ErrStale
		}
		if e = s.identity(op); e != nil {
			return e
		}
		if e = op.Err(); e != nil {
			return e
		}
		return errors.Join(ErrUnavailable, chainOutputAbsent)
	}
	fields, err := chainObject(raw, 65536)
	if err != nil || len(fields) != 5 {
		return ErrUnavailable
	}
	var best string
	var confirmations int64
	var coinbase bool
	value, exact := chainAtoms(fields["value"])
	if json.Unmarshal(fields["bestblock"], &best) != nil || best != before.Hash ||
		json.Unmarshal(fields["confirmations"], &confirmations) != nil || confirmations <= 0 || confirmations > int64(before.Height)+1 ||
		json.Unmarshal(fields["coinbase"], &coinbase) != nil || bytes.Equal(fields["coinbase"], []byte("null")) ||
		!exact || value != atoms {
		return ErrUnavailable
	}
	pubkey, err := chainObject(fields["scriptPubKey"], 64000)
	if err != nil {
		return ErrUnavailable
	}
	for key := range pubkey {
		if key != "asm" && key != "hex" && key != "reqSigs" && key != "type" && key != "address" && key != "addresses" {
			return ErrUnavailable
		}
	}
	var actual, asm, kind string
	if json.Unmarshal(pubkey["hex"], &actual) != nil || actual != wantScript ||
		json.Unmarshal(pubkey["asm"], &asm) != nil || len(asm) > 40000 || bytes.Equal(pubkey["asm"], []byte("null")) ||
		json.Unmarshal(pubkey["type"], &kind) != nil || len(kind) == 0 || len(kind) > 64 {
		return ErrUnavailable
	}
	if v, ok := pubkey["reqSigs"]; ok {
		var count uint32
		if json.Unmarshal(v, &count) != nil || count > 20 || bytes.Equal(v, []byte("null")) {
			return ErrUnavailable
		}
	}
	if v, ok := pubkey["address"]; ok {
		var address string
		if json.Unmarshal(v, &address) != nil || len(address) > 128 || bytes.Equal(v, []byte("null")) {
			return ErrUnavailable
		}
	}
	if v, ok := pubkey["addresses"]; ok {
		var addresses []string
		if json.Unmarshal(v, &addresses) != nil || len(addresses) > 20 || bytes.Equal(v, []byte("null")) {
			return ErrUnavailable
		}
		for _, address := range addresses {
			if len(address) == 0 || len(address) > 128 {
				return ErrUnavailable
			}
		}
	}
	after, err := s.observe(op)
	if err != nil {
		return err
	}
	if after != before {
		return ErrStale
	}
	if err = s.identity(op); err != nil {
		return err
	}
	return op.Err()
}
