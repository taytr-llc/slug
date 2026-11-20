// Copyright (c) 2026 The SLUG developers. ISC license.

// Package careerstate carries trusted, bounded career facts between stores.
// Only the result owner mints application Readers; JSON never grants authority.
package careerstate

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/taytr-llc/slug/fightercareer"
)

const MaxEvents = 256
const MaxEventSize = 2048
const MaxIndexSize = 544 * 1024
const MaxTimestampMS int64 = 8640000000000000

type Mode string

const (
	Current Mode = "current"
	Cutoff  Mode = "cutoff"
)

var (
	ErrInvalid   = errors.New("career_invalid")
	ErrInactive  = errors.New("career_inactive")
	ErrUnfenced  = errors.New("career_unfenced")
	ErrClosed    = errors.New("career_closed")
	ErrPoisoned  = errors.New("career_poisoned")
	ErrCancelled = errors.New("career_cancelled")
	ErrCorrupt   = errors.New("career_corrupt")
)

type Activation struct {
	Version           uint32 `json:"version"`
	PolicyID          string `json:"policy_id"`
	ActivationID      string `json:"activation_id"`
	ResultStoreID     string `json:"result_store_id"`
	ResultCreatedAtMS int64  `json:"result_created_at_ms"`
	SourceCreatedAtMS int64  `json:"source_created_at_ms"`
	ActivatedAtMS     int64  `json:"activated_at_ms"`
}
type Corner struct {
	ProspectID string `json:"prospect_id"`
	AccountID  string `json:"account_id"`
	GymID      string `json:"gym_id"`
}
type Event struct {
	Version            uint32 `json:"version"`
	AwardID            string `json:"award_id"`
	ResultKey          string `json:"result_key"`
	CardID             string `json:"card_id"`
	BoutID             string `json:"bout_id"`
	SourceRecordDigest string `json:"source_record_digest"`
	InputID            string `json:"input_id"`
	ResultDigest       string `json:"result_digest"`
	FinalityDigest     string `json:"finality_digest"`
	EffectiveAtMS      int64  `json:"effective_at_ms"`
	Saturday           string `json:"saturday"`
	CornerA            Corner `json:"corner_a"`
	CornerB            Corner `json:"corner_b"`
	Winner             string `json:"winner"`
	Method             string `json:"method"`
}

// Scope is trusted factory wiring, never caller-supplied authority.
type Scope struct {
	StableStoreIDs [2]string
	CardStoreID    string
}

// State is an owned atomic copy returned only by the concrete result owner.
type State struct {
	Activation Activation
	Scope      Scope
	ClockMS    int64
	Events     []Event
}
type Reader struct {
	activation Activation
	scope      Scope
	read       func(context.Context) (State, error)
}
type Snapshot struct {
	activation Activation
	asOf       int64
	mode       Mode
	ids        []string
	digest     string
	totals     map[string]fightercareer.Totals
}

const policy = `{"version":1,"scope":"two_fixture_standard_career","source":"result_v1_first_finality_after_activation","xp":"D42_standard_1_cap15","base_points":"D41_no_quality","effective_time":"first_finality_observed_at_ms","cutoff":"strict_before","later_conflict":"retain","c1":"zero_professional_record_only"}`

func digest(domain string, parts ...[]byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func raw(s string) []byte { b, _ := hex.DecodeString(s); return b }
func u32(n uint32) []byte { var b [4]byte; binary.LittleEndian.PutUint32(b[:], n); return b[:] }
func u64(n int64) []byte  { var b [8]byte; binary.LittleEndian.PutUint64(b[:], uint64(n)); return b[:] }
func lp(b []byte) []byte  { return append(u32(uint32(len(b))), b...) }
func ID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func stamp(n int64) bool { return n > 0 && n <= MaxTimestampMS }
func PolicyID() string   { return digest("SLUG candidate career policy\x00v1\x00", lp([]byte(policy))) }
func ActivationID(a Activation) string {
	return digest("SLUG candidate career activation\x00v1\x00", raw(a.PolicyID), raw(a.ResultStoreID), u64(a.ResultCreatedAtMS), u64(a.SourceCreatedAtMS), u64(a.ActivatedAtMS))
}
func AwardID(a Activation, key string) string {
	return digest("SLUG candidate career award\x00v1\x00", raw(a.ActivationID), raw(key))
}
func FinalityDigest(canonical []byte) string {
	return digest("SLUG candidate career finality\x00v1\x00", lp(canonical))
}
func ValidateActivation(a Activation) error {
	if a.Version != 1 || a.PolicyID != PolicyID() || !ID(a.ResultStoreID) || !stamp(a.SourceCreatedAtMS) || !stamp(a.ResultCreatedAtMS) || !stamp(a.ActivatedAtMS) || a.SourceCreatedAtMS > a.ResultCreatedAtMS || a.ResultCreatedAtMS > a.ActivatedAtMS || a.ActivationID != ActivationID(a) {
		return ErrCorrupt
	}
	return nil
}
func deltas(e Event) ([2]fightercareer.Delta, error) {
	winner := "draw"
	if e.Winner == e.CornerA.ProspectID {
		winner = "a"
	} else if e.Winner == e.CornerB.ProspectID {
		winner = "b"
	} else if e.Winner != "" {
		return [2]fightercareer.Delta{}, ErrCorrupt
	}
	d, err := fightercareer.Award(winner, e.Method, "standard")
	if err != nil {
		return d, ErrCorrupt
	}
	return d, nil
}

// ValidateIndex validates and owns a sorted complete index. It never imports a verdict.
func ValidateIndex(a Activation, events []Event) ([]Event, error) {
	if ValidateActivation(a) != nil || events == nil || len(events) > MaxEvents {
		return nil, ErrCorrupt
	}
	out := append([]Event{}, events...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].EffectiveAtMS == out[j].EffectiveAtMS {
			return out[i].AwardID < out[j].AwardID
		}
		return out[i].EffectiveAtMS < out[j].EffectiveAtMS
	})
	keys := map[string]bool{}
	awards := map[string]bool{}
	for _, e := range out {
		for _, id := range []string{e.AwardID, e.ResultKey, e.CardID, e.BoutID, e.SourceRecordDigest, e.InputID, e.ResultDigest, e.FinalityDigest, e.CornerA.ProspectID, e.CornerA.AccountID, e.CornerA.GymID, e.CornerB.ProspectID, e.CornerB.AccountID, e.CornerB.GymID} {
			if !ID(id) {
				return nil, ErrCorrupt
			}
		}
		date, err := time.Parse("2006-01-02", e.Saturday)
		if err != nil || date.Format("2006-01-02") != e.Saturday || date.Weekday() != time.Saturday || e.Version != 1 || !stamp(e.EffectiveAtMS) || e.EffectiveAtMS < a.ActivatedAtMS || e.AwardID != AwardID(a, e.ResultKey) || keys[e.ResultKey] || awards[e.AwardID] || e.CornerA.ProspectID == e.CornerB.ProspectID || e.CornerA.AccountID == e.CornerB.AccountID || e.CornerA.GymID == e.CornerB.GymID {
			return nil, ErrCorrupt
		}
		if _, err = deltas(e); err != nil {
			return nil, err
		}
		b, _ := json.Marshal(e)
		if len(b) > MaxEventSize {
			return nil, ErrCorrupt
		}
		keys[e.ResultKey] = true
		awards[e.AwardID] = true
	}
	b, _ := json.Marshal(out)
	if len(b) > MaxIndexSize {
		return nil, ErrCorrupt
	}
	return out, nil
}

// NewReader is trusted internal wiring. It is not an external data constructor.
func validScope(s Scope) bool {
	return ID(s.CardStoreID) && ID(s.StableStoreIDs[0]) && ID(s.StableStoreIDs[1]) && s.StableStoreIDs[0] != s.StableStoreIDs[1]
}
func NewReader(a Activation, scope Scope, read func(context.Context) (State, error)) (*Reader, error) {
	if read == nil || ValidateActivation(a) != nil || !validScope(scope) {
		return nil, ErrInvalid
	}
	return &Reader{activation: a, scope: scope, read: read}, nil
}
func (r *Reader) state(ctx context.Context) (State, error) {
	if r == nil || r.read == nil {
		return State{}, ErrInactive
	}
	if ctx == nil {
		return State{}, ErrInvalid
	}
	if ctx.Err() != nil {
		return State{}, ErrCancelled
	}
	s, e := r.read(ctx)
	if e != nil {
		return State{}, e
	}
	if s.Activation != r.activation || s.Scope != r.scope || ValidateActivation(s.Activation) != nil || !stamp(s.ClockMS) || s.ClockMS < s.Activation.ActivatedAtMS || !ID(s.Scope.CardStoreID) || !ID(s.Scope.StableStoreIDs[0]) || !ID(s.Scope.StableStoreIDs[1]) || s.Scope.StableStoreIDs[0] == s.Scope.StableStoreIDs[1] {
		return State{}, ErrCorrupt
	}
	return s, nil
}
func (r *Reader) Activation() (Activation, error) {
	s, e := r.state(context.Background())
	return s.Activation, e
}
func (r *Reader) Scope() (Scope, error) { s, e := r.state(context.Background()); return s.Scope, e }
func (r *Reader) Snapshot(ctx context.Context, ids []string, asOf int64, mode Mode) (Snapshot, error) {
	if r == nil || r.read == nil {
		return Snapshot{}, ErrInactive
	}
	if !stamp(asOf) || (mode != Current && mode != Cutoff) || len(ids) < 1 || len(ids) > 10 {
		return Snapshot{}, ErrInvalid
	}
	for i, id := range ids {
		if !ID(id) || (i > 0 && ids[i-1] >= id) {
			return Snapshot{}, ErrInvalid
		}
	}
	state, err := r.state(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if asOf < state.Activation.ActivatedAtMS {
		return Snapshot{}, ErrInvalid
	}
	if mode == Cutoff && state.ClockMS < asOf {
		return Snapshot{}, ErrUnfenced
	}
	events, err := ValidateIndex(state.Activation, state.Events)
	if err != nil {
		return Snapshot{}, err
	}
	out := Snapshot{activation: state.Activation, asOf: asOf, mode: mode, ids: append([]string(nil), ids...), totals: map[string]fightercareer.Totals{}}
	for _, id := range ids {
		out.totals[id] = fightercareer.Totals{}
	}
	prefix := []Event{}
	for _, e := range events {
		if e.EffectiveAtMS > state.ClockMS {
			return Snapshot{}, ErrCorrupt
		}
		if e.EffectiveAtMS > asOf || mode == Cutoff && e.EffectiveAtMS == asOf {
			continue
		}
		prefix = append(prefix, e)
		d, _ := deltas(e)
		for i, c := range []Corner{e.CornerA, e.CornerB} {
			if prior, ok := out.totals[c.ProspectID]; ok {
				next, e := fightercareer.Apply(prior, d[i])
				if e != nil {
					return Snapshot{}, ErrCorrupt
				}
				out.totals[c.ProspectID] = next
			}
		}
	}
	h := sha256.New()
	h.Write([]byte("SLUG candidate career prefix\x00v1\x00"))
	h.Write(raw(state.Activation.ActivationID))
	h.Write(u64(asOf))
	h.Write(lp([]byte(mode)))
	h.Write(u32(uint32(len(ids))))
	for _, id := range ids {
		h.Write(raw(id))
	}
	h.Write(u32(uint32(len(prefix))))
	for _, e := range prefix {
		b, _ := json.Marshal(e)
		h.Write(lp(b))
	}
	out.digest = hex.EncodeToString(h.Sum(nil))
	if ctx.Err() != nil {
		return Snapshot{}, ErrCancelled
	}
	return out, nil
}
func (s Snapshot) Activation() Activation                        { return s.activation }
func (s Snapshot) Digest() string                                { return s.digest }
func (s Snapshot) AsOfMS() int64                                 { return s.asOf }
func (s Snapshot) Totals(id string) (fightercareer.Totals, bool) { v, ok := s.totals[id]; return v, ok }

func (s Snapshot) Mode() Mode            { return s.mode }
func (s Snapshot) ProspectIDs() []string { return append([]string(nil), s.ids...) }

// ValidateTransition forbids removal or alteration of any previously effective
// event. The result owner validates the proposed complete index before Sync.
func ValidateTransition(a Activation, previous, proposed []Event) ([]Event, error) {
	old, e := ValidateIndex(a, previous)
	if e != nil {
		return nil, e
	}
	next, e := ValidateIndex(a, proposed)
	if e != nil {
		return nil, e
	}
	byID := make(map[string]Event, len(next))
	for _, v := range next {
		byID[v.AwardID] = v
	}
	for _, v := range old {
		if got, ok := byID[v.AwardID]; !ok || got != v {
			return nil, ErrCorrupt
		}
	}
	return next, nil
}
