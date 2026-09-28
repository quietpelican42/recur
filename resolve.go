package recur

import "time"

// resolveLocal converts a wall-clock date and time in loc to the instant RFC
// 5545 §3.3.5 says it denotes.
//
// RFC 5545 §3.3.5 (DATE-TIME, FORM #3): "If, based on the definition of the
// referenced time zone, the local time described occurs more than once
// (when changing from daylight to standard time), the DATE-TIME value
// refers to the first occurrence of the referenced time. ... If the local
// time described does not occur (when changing from standard to daylight
// time), the DATE-TIME value is interpreted using the UTC offset before the
// gap in local times." Its own example: TZID=America/New_York:20070311T023000
// (nonexistent) means "March 11, 2007 at 3:30 A.M. EDT (UTC-04:00), one hour
// after 1:30 A.M. EST (UTC-05:00)".
//
// time.Date alone cannot be used for this: verified against Go 1.27.1,
// time.Date(2024,3,10,2,30,...,America/New_York) (nonexistent) returns
// 01:30-05:00, silently moving the wall clock backward instead of shifting
// it forward past the gap, while the equivalent Australia/Sydney case
// happens to come out right. Go documents no rule either way, and an
// earlier version of this function tried to paper over that by always
// reinterpreting the wall clock with whatever offset was in effect about a
// day earlier — which is wrong for any ORDINARY (non-transitional) time
// that merely happens to fall within a day of some unrelated transition: it
// would incorrectly re-shift already-unambiguous times (caught by the
// reviewer via DTSTART 2024-03-08 02:30 America/New_York, DAILY, COUNT=4:
// occurrence 4, 2024-03-11 02:30, is a perfectly ordinary EDT time with
// nothing before it to resolve, and must not be shifted to 03:30).
//
// The approach here instead locates the specific transition nearest the
// candidate wall-clock time using Location.ZoneBounds (Go 1.19+), and only
// engages RFC 5545's gap/fold rule when the wall clock actually is
// nonexistent or ambiguous.
func resolveLocal(y int, mo time.Month, d, h, mi, s int, loc *time.Location) time.Time {
	// wallUTC's only purpose is arithmetic: treating the wall-clock
	// digits as literal Unix time lets us test "interpret this wall
	// clock using offset X" as wallUTC.Add(-X seconds), for any offset X,
	// without reference to any real instant yet.
	wallUTC := time.Date(y, mo, d, h, mi, s, 0, time.UTC)

	// guess is Go's own best-effort construction. Per the doc comment
	// above, its wall-clock display can be wrong across a transition —
	// but as an instant, it always lands within one DST delta of the
	// true target, which puts it in a zone segment immediately
	// neighboring the transition we actually care about (confirmed
	// against both the New_York and Sydney cases above). We only use it
	// to find that segment, via ZoneBounds, never as the answer itself.
	guess := time.Date(y, mo, d, h, mi, s, 0, loc)
	segStart, segEnd := guess.ZoneBounds()
	_, curOff := guess.Zone()

	// tryOffset reports whether interpreting the wall clock with off
	// produces a self-consistent instant: one where off is genuinely the
	// offset in effect at the result. If so, that instant is a valid
	// reading of the wall clock; if not, off does not apply here at all
	// (the wall clock is on the other side of a transition from off).
	tryOffset := func(off int) (time.Time, bool) {
		c := wallUTC.Add(-time.Duration(off) * time.Second)
		_, actual := c.In(loc).Zone()
		return c, actual == off
	}

	offsets := []int{curOff}
	if !segStart.IsZero() {
		_, before := segStart.Add(-time.Second).Zone()
		offsets = append(offsets, before)
	}
	if !segEnd.IsZero() {
		_, after := segEnd.Zone()
		offsets = append(offsets, after)
	}

	seen := make(map[int]bool, len(offsets))
	var valid []time.Time
	for _, off := range offsets {
		if seen[off] {
			continue
		}
		seen[off] = true
		if c, ok := tryOffset(off); ok {
			valid = append(valid, c)
		}
	}

	switch len(valid) {
	case 1:
		// Ordinary case: exactly one offset among the candidates in
		// this neighborhood produces a genuine instant. This covers
		// every wall-clock time that isn't itself in a gap or fold,
		// including one that merely falls near (but not in) a
		// transition.
		return valid[0].In(loc)
	case 2:
		// Fold: the wall clock is reached twice. RFC 5545 says to use
		// its first (earlier) occurrence.
		if valid[0].Before(valid[1]) {
			return valid[0].In(loc)
		}
		return valid[1].In(loc)
	default:
		// Gap: none of the candidate offsets round-trip, so the wall
		// clock does not exist. RFC 5545 says to use the offset in
		// effect before the gap. A gap is, by definition, a forward
		// jump in local time (DST always adds time relative to
		// standard time, and Pacific/Apia's 2011-12-30 whole-day skip
		// is the same kind of forward jump at a larger scale), so the
		// pre-gap offset is always the smaller of the (at most two,
		// after dedup) distinct offsets found near this wall clock.
		off := offsets[0]
		for _, o := range offsets[1:] {
			if o < off {
				off = o
			}
		}
		c, _ := tryOffset(off)
		return c.In(loc)
	}
}
