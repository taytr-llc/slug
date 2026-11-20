// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"github.com/taytr-llc/slug/fightercondition"
	"time"
)

const TZifSHA256 = "200d05754f6d83a371cf408d7085125797657b3b0bebeba1e508cffe86a3e5c8"

//go:embed calendar/America_Los_Angeles.tzif
var zoneBytes []byte
var entryZone = func() *time.Location {
	h := sha256.Sum256(zoneBytes)
	if hex.EncodeToString(h[:]) != TZifSHA256 {
		panic("entry calendar pin mismatch")
	}
	z, e := time.LoadLocationFromTZData("America/Los_Angeles", zoneBytes)
	if e != nil {
		panic("invalid entry calendar")
	}
	return z
}()

func hash(domain string, pieces ...[]byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, p := range pieces {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func le32(n uint32) []byte  { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, n); return b }
func le64(n uint64) []byte  { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, n); return b }
func lp(s string) []byte    { return append(le32(uint32(len(s))), []byte(s)...) }
func rawID(s string) []byte { b, _ := hex.DecodeString(s); return b }
func CalendarID() string {
	return hash("SLUG candidate entry calendar\x00v1\x00", le32(1), lp("America/Los_Angeles"), rawID(TZifSHA256), le32(2026), le32(2099), le32(6), le32(12), le32(0))
}
func RulesID() string {
	return hash("SLUG candidate entry rules\x00v1\x00", le32(1), le32(5), le32(1), le64(0), le32(1), le32(1), le32(1), lp(fightercondition.ProjectorID), le32(0), lp("light"), lp("welter"), lp("middle"), lp("light_heavy"), lp("heavy"), lp("intake-only"))
}
func NamespaceID(network uint32, genesis, authority string) (string, error) {
	if !hashID(genesis) || !hashID(authority) {
		return "", ErrInvalid
	}
	return hash("SLUG local candidate entry namespace\x00v1\x00", le32(network), rawID(genesis), rawID(authority)), nil
}
func cutoff(date string) (int64, error) {
	if len(date) != 10 {
		return 0, ErrCalendar
	}
	for i, c := range date {
		if i == 4 || i == 7 {
			if c != '-' {
				return 0, ErrCalendar
			}
		} else if c < '0' || c > '9' {
			return 0, ErrCalendar
		}
	}
	d, e := time.ParseInLocation("2006-01-02", date, entryZone)
	if e != nil || d.Year() < 2026 || d.Year() > 2099 || d.Weekday() != time.Saturday {
		return 0, ErrCalendar
	}
	d = time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, entryZone)
	if d.Format("2006-01-02") != date || d.Hour() != 12 || d.Minute() != 0 || d.Second() != 0 {
		return 0, ErrCalendar
	}
	return d.UnixMilli(), nil
}
func eventForNamespace(namespace, date string) (Event, error) {
	n, e := cutoff(date)
	if e != nil {
		return Event{}, e
	}
	if !hashID(namespace) {
		return Event{}, ErrInvalid
	}
	ev := Event{Version: Version1, NamespaceID: namespace, CalendarID: CalendarID(), RulesID: RulesID(), Saturday: date, CutoffMS: n}
	ev.EventID = hash("SLUG candidate Saturday event\x00v1\x00", rawID(ev.NamespaceID), rawID(ev.CalendarID), rawID(ev.RulesID), []byte(date), le64(uint64(n)))
	return ev, nil
}
func EventForDate(p Pin, date string) (Event, error) {
	p, e := ValidatePin(p)
	if e != nil {
		return Event{}, e
	}
	return eventForNamespace(p.NamespaceID, date)
}
func EntryID(store, event, gym string) (string, error) {
	if !hashID(store) || !hashID(event) || !hashID(gym) {
		return "", ErrInvalid
	}
	return hash("SLUG candidate Saturday entry\x00v1\x00", rawID(store), rawID(event), rawID(gym)), nil
}
func upcomingDate(now int64) (string, error) {
	if !timestamp(now) {
		return "", ErrClock
	}
	d := time.UnixMilli(now).In(entryZone)
	if d.Year() < 2026 {
		d = time.Date(2026, 1, 1, 0, 0, 0, 0, entryZone)
	}
	if d.Year() > 2099 {
		return "", ErrCalendar
	}
	for i := 0; i < 8; i++ {
		if d.Weekday() == time.Saturday {
			date := d.Format("2006-01-02")
			n, e := cutoff(date)
			if e == nil && n > now {
				return date, nil
			}
		}
		d = d.AddDate(0, 0, 1)
	}
	return "", ErrCalendar
}
