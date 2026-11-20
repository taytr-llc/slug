// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/gymcard"
)

func rawID(s string) []byte { b, _ := hex.DecodeString(s); return b }
func le32(n uint32) []byte  { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, n); return b }
func lp(b []byte) []byte    { return append(le32(uint32(len(b))), b...) }
func hash(domain string, parts ...[]byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, b := range parts {
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func RulesID() string {
	id, _ := json.Marshal(gymcard.RulesID())
	return hash("SLUG candidate bout rules\x00v1\x00", lp([]byte(fmt.Sprintf(`{"version":1,"scope":"two_fixture_booked_candidate_bout","record_basis":"candidate_zero_record_no_results_authority","card_rules_id":%s,"engine_version":1,"engine_id":"slug-fighter-bout-candidate-v1","mode":"standard","seed_kind":"explicit","rounds":3,"as_of":"event_cutoff","seed":"booked_bout_seed","pro_xp":0,"training_skill":0,"new_run_requires_usable_card":true,"saved_replay_immutable":true}`, id))))
}
func CandidatePin(source gymcard.Pin) (Pin, error) {
	p, e := gymcard.ValidatePin(source)
	if e != nil {
		return Pin{}, ErrInvalid
	}
	id, e := gymcard.ID(p)
	if e != nil {
		return Pin{}, ErrInvalid
	}
	return Pin{1, id, p, RulesID()}, nil
}
func ID(p Pin) (string, error) {
	b, e := MarshalPin(p)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate bout store\x00v1\x00", lp(b)), nil
}
func recordDigest(v Record) string {
	b, _ := json.Marshal(v)
	return hash("SLUG candidate bout record\x00v1\x00", lp(b))
}
func RecordDigest(ctx context.Context, p Pin, v Record) (string, error) {
	v, e := ValidateRecord(ctx, p, v)
	if e != nil {
		return "", e
	}
	return recordDigest(v), nil
}
func sourceDigest(v any) string {
	b, _ := json.Marshal(v)
	return hash("SLUG candidate booked bout source\x00v1\x00", lp(b))
}
func inputDigest(v any) string {
	b, _ := json.Marshal(v)
	return hash("SLUG fighter bout input\x00v1\x00", b)
}
func resultDigest(v any) string {
	b, _ := json.Marshal(v)
	return hash("SLUG fighter bout result\x00v1\x00", b)
}
