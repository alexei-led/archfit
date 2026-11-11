package resolved

import (
	"strconv"
	"time"
)

// secondsPerDay converts a span between two UTC midnights to days; UTC has no
// daylight saving, so the span is always whole days. Unix seconds, not
// time.Duration, which tops out near 292 years and would miscount a far date
// such as 9999-12-31.
const secondsPerDay = 24 * 60 * 60

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
	switch left := (utcDay(expires).Unix() - utcDay(today).Unix()) / secondsPerDay; {
	case left < 0:
		return date + " (expired)"
	case left == 1:
		return date + " (1 day)"
	default:
		return date + " (" + strconv.FormatInt(left, 10) + " days)"
	}
}

// utcDay is 00:00 UTC of the calendar day t falls on in UTC.
func utcDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
