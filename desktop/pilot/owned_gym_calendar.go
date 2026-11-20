// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"time"
)

const ownedCalendarReserve = 8 * time.Second

// Ordinary verification may use only the time before the real boundary's
// reserved observation window. Clamp before converting the bounded ms value.
func ownedCalendarWorkBudget(event gymentry.Event, nowMS int64, maximum time.Duration) time.Duration {
	left := event.CutoffMS - nowMS - ownedCalendarReserve.Milliseconds()
	if left <= 0 || maximum <= 0 {
		return 0
	}
	if left >= maximum.Milliseconds() {
		return maximum
	}
	return time.Duration(left) * time.Millisecond
}

func (c *OwnedGymChild) walletRefreshAllowed() bool {
	event, e := gymentry.OwnedUpcomingEvent(c.scope.EntryPin, c.league.calendarTime().UnixMilli())
	if e != nil {
		return false
	}
	return ownedCalendarWorkBudget(event, c.league.calendarTime().UnixMilli(), 6*time.Second) > 5*time.Second
}

// The calendar hook has two bounded candidates. Date arithmetic chooses only
// calendar dates; all cutoff instants, DST rules and event IDs come from the
// pinned concrete entry/event packages. It never subtracts seven days in ms.
func ownedCalendarDates(p gymevent.OwnedPin, upcoming gymentry.Event) ([]string, error) {
	if _, e := gymevent.ValidateOwnedPin(p); e != nil {
		return nil, e
	}
	expected, e := gymentry.OwnedEventForDate(p.Participants[0], upcoming.Saturday)
	if e != nil || expected != upcoming {
		return nil, gymevent.ErrInvalid
	}
	day, e := time.Parse("2006-01-02", upcoming.Saturday)
	if e != nil {
		return nil, e
	}
	previous := day.AddDate(0, 0, -7).Format("2006-01-02")
	dates := []string{upcoming.Saturday}
	if _, e = gymevent.OwnedEventForDate(p, previous); e == nil {
		dates = append(dates, previous)
	}
	return dates, nil
}

// The continuation hint is true only after an error-free incomplete replay
// slice. It is scheduling metadata, never a ready head or disclosure authority.
func (c *OwnedGymChild) refreshOwnedEvents(ctx context.Context) bool {
	work, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	date, retained, ready, e := c.league.resumeCutoff(work)
	if retained {
		if e == nil && ready {
			_, _ = c.captureOwnedEvent(work, date)
		}
		return e == nil && !ready
	}
	if e != nil {
		return false
	}
	// Calendar selection uses only the original immutable pin and the league's
	// clock. Entry.Upcoming also requires a complete current ownership Head,
	// which is deliberately unavailable between bounded replay steps.
	upcoming, e := gymentry.OwnedUpcomingEvent(c.scope.EntryPin, c.league.calendarTime().UnixMilli())
	if e != nil {
		return false
	}
	dates, e := ownedCalendarDates(c.scope.EventPin, upcoming)
	if e != nil {
		return false
	}
	// A near-noon observation must begin before ordinary catch-up consumes the
	// remaining bracket. The core independently samples and checks both sides.
	now := c.league.calendarTime().UnixMilli()
	if upcoming.CutoffMS > now && upcoming.CutoffMS-now <= ownedCalendarReserve.Milliseconds() {
		ready, e := c.league.advanceCutoff(work, dates[0])
		if e == nil && ready {
			_, _ = c.captureOwnedEvent(work, dates[0])
		}
		return e == nil && !ready
	}
	// Current replay is bounded and may legitimately remain incomplete. It can
	// never advance instead of a retained historical replay from the first branch.
	budget := ownedCalendarWorkBudget(upcoming, now, 20*time.Second)
	if budget <= 0 {
		return false
	}
	ordinary, stop := context.WithTimeout(work, budget)
	progress, e := c.runtime.RefreshOwnership(ordinary, 8)
	stop()
	if e != nil {
		return false
	}
	if !progress.Ready {
		return true
	}
	if len(dates) < 2 {
		return false
	}
	// The previous calendar event can only resume its originally retained bracket;
	// CaptureBoundary refuses creation after noon, including after process loss.
	ready, e = c.league.advanceCutoff(work, dates[1])
	if e == nil && ready {
		_, _ = c.captureOwnedEvent(work, dates[1])
	}
	return e == nil && !ready
}

func (l *OwnedLeague) calendarTime() time.Time {
	if l.calendarNow != nil {
		return l.calendarNow()
	}
	return time.Now()
}

// Seal the concrete result prefix before entering Event/Entry/Stable gates.
// A cutoff is an observation under the original View, not a result award or
// publishing capability. Missing result authority never becomes zero career.
func (c *OwnedGymChild) captureOwnedEvent(ctx context.Context, date string) (gymevent.OwnedCaptureResult, error) {
	if c == nil || c.league == nil || c.league.results == nil || c.events == nil || c.entries == nil {
		return gymevent.OwnedCaptureResult{}, ErrPoolRuntime
	}
	if e := c.league.results.FenceCareer(ctx, c.entries, c.session(), date); e != nil {
		return gymevent.OwnedCaptureResult{}, e
	}
	return c.events.Capture(ctx, c.entries, c.session(), date)
}
