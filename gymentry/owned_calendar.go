// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

// OwnedUpcomingEvent derives calendar metadata from the exact immutable entry
// pin and supplied time. It does not inspect a store, certify current ownership,
// capture a cutoff, or grant authority to submit an entry.
func OwnedUpcomingEvent(p OwnedPin, atMS int64) (Event, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Event{}, e
	}
	date, e := upcomingDate(atMS)
	if e != nil {
		return Event{}, e
	}
	return OwnedEventForDate(p, date)
}
