// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import "time"

// IncidentDeadline is only the pinned calendar threshold. It grants no seed or
// finality authority. A deadline in January 2100 does not admit a 2100 event.
func IncidentDeadline(event Event) (int64, error) {
	if validateEvent(event) != nil {
		return 0, ErrCalendar
	}
	d := time.UnixMilli(event.CutoffMS).In(entryZone).AddDate(0, 0, 7)
	if d.Weekday() != time.Saturday || d.Hour() != 12 || d.Minute() != 0 || d.Second() != 0 || d.Nanosecond() != 0 || d.Year() > 2100 {
		return 0, ErrCalendar
	}
	n := d.UnixMilli()
	if n <= event.CutoffMS || !timestamp(n) {
		return 0, ErrCalendar
	}
	return n, nil
}
