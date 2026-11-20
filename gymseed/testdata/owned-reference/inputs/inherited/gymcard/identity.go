// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
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
	return hash("SLUG candidate card rules\x00v1\x00", lp([]byte(`{"version":1,"scope":"two_fixture_candidate","record_basis":"candidate_zero_record_no_results_authority","classes":["light","welter","middle","light_heavy","heavy"],"band_min_numerator":1,"band_min_denominator":2,"band_max_numerator":2,"band_max_denominator":1,"widen_to_five":false,"condition_version":1,"rating":"D65","cutoff_eligibility":true,"max_entrants":10,"max_bouts":5}`)))
}
func CandidatePin(source gymseed.Pin) (Pin, error) {
	p, e := gymseed.ValidatePin(source)
	if e != nil {
		return Pin{}, ErrInvalid
	}
	id, e := gymseed.ID(p)
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
	return hash("SLUG candidate card store\x00v1\x00", lp(b)), nil
}
func CardID(p Pin, ev gymevent.Event) (string, error) {
	if validatePin(p) != nil {
		return "", ErrInvalid
	}
	if _, e := gymevent.ValidateEvent(p.SourcePin.SourcePin, ev); e != nil {
		return "", ErrInvalid
	}
	id, _ := ID(p)
	return hash("SLUG candidate card event\x00v1\x00", rawID(id), rawID(ev.EventID)), nil
}
func RecordDigest(p Pin, v Record) (string, error) {
	b, e := MarshalRecord(p, v)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate card record\x00v1\x00", lp(b)), nil
}
func recordDigest(v Record) string {
	b, _ := json.Marshal(v)
	return hash("SLUG candidate card record\x00v1\x00", lp(b))
}

// Internal byte recipe; the complete Pin/Event were already validated.
func cardID(p Pin, ev gymevent.Event) string {
	b, _ := json.Marshal(p)
	store := hash("SLUG candidate card store\x00v1\x00", lp(b))
	return hash("SLUG candidate card event\x00v1\x00", rawID(store), rawID(ev.EventID))
}
