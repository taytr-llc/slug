// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymentry"
)

func rawID(s string) []byte { b, _ := hex.DecodeString(s); return b }
func le64(n uint64) []byte  { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, n); return b }
func lp(raw []byte) []byte {
	b := make([]byte, 4, len(raw)+4)
	binary.LittleEndian.PutUint32(b, uint32(len(raw)))
	return append(b, raw...)
}
func hash(domain string, parts ...[]byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func RulesID() string {
	v := struct {
		Version          uint32 `json:"version"`
		Boundary         string `json:"boundary"`
		ConditionTime    string `json:"condition_time"`
		OwnershipBasis   string `json:"ownership_basis"`
		ConditionVersion uint32 `json:"condition_version"`
		ProjectorID      string `json:"projector_id"`
		XP               uint32 `json:"xp"`
		Training         uint32 `json:"training"`
		Participants     uint32 `json:"participants"`
		MembersPerGym    uint32 `json:"members_per_gym"`
		ExclusionOrder   string `json:"exclusion_order"`
		LaterChanges     string `json:"later_changes"`
		SeedAuthority    string `json:"seed_authority"`
	}{1, "committed_before_cutoff", "cutoff", "candidate_assignment", 1, fightercondition.ProjectorID, 0, 0, 2, 5, "retired_then_not_rostered", "immutable", "unavailable"}
	b, _ := json.Marshal(v)
	return hash("SLUG candidate snapshot rules\x00v1\x00", lp(b))
}
func ID(p Pin) (string, error) {
	b, e := MarshalPin(p)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate event store\x00v1\x00", lp(b)), nil
}
func EventForDate(p Pin, date string) (Event, error) {
	p, e := ValidatePin(p)
	if e != nil {
		return Event{}, e
	}
	source, e := gymentry.EventForDate(p.Participants[0], date)
	if e != nil {
		return Event{}, ErrCalendar
	}
	deadline, e := gymentry.IncidentDeadline(source)
	if e != nil {
		return Event{}, ErrCalendar
	}
	id, e := ID(p)
	if e != nil {
		return Event{}, e
	}
	v := Event{Version: 1, SourceEventID: source.EventID, StoreID: id, NamespaceID: source.NamespaceID, CalendarID: p.CalendarID, RulesID: p.RulesID, Saturday: date, CutoffMS: source.CutoffMS, IncidentDeadlineMS: deadline}
	v.EventID = hash("SLUG candidate cutoff event\x00v1\x00", rawID(id), rawID(v.SourceEventID), le64(uint64(v.CutoffMS)), le64(uint64(deadline)))
	return v, nil
}
func SnapshotDigest(p Pin, v SnapshotInput) (string, error) {
	b, e := MarshalSnapshotInput(p, v)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate cutoff input\x00v1\x00", lp(b)), nil
}
func ReceiptDigest(p Pin, v SnapshotReceipt) (string, error) {
	b, e := MarshalSnapshotReceipt(p, v)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate cutoff receipt\x00v1\x00", lp(b)), nil
}
