package resolved

import (
	"strconv"
	"time"
)

// hoursPerDay converts a span between two UTC midnights to days; UTC has no
// daylight saving, so the span is always whole days.
const hoursPerDay = 24

// FormatExpires writes the date of an exception as design section 5 shows
// it: the ISO date and the days left, "2026-11-19 (9 days)". Days are counted
// between UTC calendar days. Expires is the last day the exception holds, so
// that day shows "(0 days)" and the day after "(expired)". A zero expires is an
// exception without a date and gives "".
func FormatExpires(expires, today time.Time) string {
	if expires.IsZero() {
		return ""
	}
	date := expires.UTC().Format(time.DateOnly)
	switch left := int(utcDay(expires).Sub(utcDay(today)).Hours()) / hoursPerDay; {
	case left < 0:
		return date + " (expired)"
	case left == 1:
		return date + " (1 day)"
	default:
		return date + " (" + strconv.Itoa(left) + " days)"
	}
}

// utcDay is 00:00 UTC of the calendar day t falls on in UTC.
func utcDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
