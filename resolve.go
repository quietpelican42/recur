package recur

import "time"

// resolveLocal converts a wall-clock date and time in loc to the instant RFC
// 5545 §3.3.5 says it denotes, handling the two DST discontinuities that
// time.Date alone does not resolve consistently (verified against Go
// 1.27.1: time.Date(2024,3,10,2,30,...,America/New_York) returns
// 01:30-05:00, silently moving the wall clock backward, while the
// equivalent Sydney case happens to come out right — the stdlib gives no
// documented guarantee either way).
//
// RFC 5545 §3.3.5 (DATE-TIME, FORM #3): "If, based on the definition of the
// referenced time zone, the local time described occurs more than once
// (when changing from daylight to standard time), the DATE-TIME value
// refers to the first occurrence of the referenced time. ... If the local
// time described does not occur (when changing from standard to daylight
// time), the DATE-TIME value is interpreted using the UTC offset before the
// gap in local times." Its own example: TZID=America/New_York:20070311T023000
// (nonexistent) means "March 11, 2007 at 3:30 A.M. EDT (UTC-04:00), one hour
// after 1:30 A.M. EST (UTC-05:00)" — the wall clock is shifted forward by
// the width of the gap, not rolled back into the previous offset.
//
// Both cases reduce to the same rule: interpret the wall-clock components
// using the UTC offset that was in effect immediately before whatever
// transition is nearest that wall-clock time, ignoring what offset is
// actually in effect at the resulting instant. For an ordinary time (no
// transition within a day either side) that offset is just the correct one
// and this is a no-op.
func resolveLocal(y int, mo time.Month, d, h, mi, s int, loc *time.Location) time.Time {
	// pivot is a reference point only: the wall-clock numbers themselves
	// interpreted as if they were already a Unix time. It carries no
	// meaning beyond letting us add/subtract a candidate offset in
	// seconds and compare.
	pivot := time.Date(y, mo, d, h, mi, s, 0, time.UTC).Unix()

	// offsetBefore is the zone offset in effect a bit over a day before
	// this wall-clock instant. DST transitions are hours wide and, in
	// every zone in tzdata, at least many days apart, so this reliably
	// lands before any transition that could make (y,mo,d,h,mi,s)
	// ambiguous or nonexistent — including a whole day being skipped, as
	// in Pacific/Apia's 2011-12-30.
	const probe = 26 * 3600
	_, offsetBefore := time.Unix(pivot-probe, 0).In(loc).Zone()

	// Interpreting the wall clock with offsetBefore: UTC = local - offset.
	candidate := time.Unix(pivot-int64(offsetBefore), 0)
	return candidate.In(loc)
}
