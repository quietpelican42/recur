package recur

import (
	"iter"
	"sort"
	"time"
)

// maxConsecutiveEmptyPeriods bounds how many base-frequency periods in a row
// (years/months/weeks/days, or hour/minute/second steps) may produce zero
// candidates before generation gives up. Without this, a rule that can
// never match anything — FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=30, February 30
// never existing — would iterate forever with nothing to yield, and a
// caller with no COUNT/UNTIL bound would never get control back even from
// a bounded Iter(from, to): an empty period never calls yield, so the
// range loop never gets a chance to check "have we passed to" and break.
// The counter resets to zero the moment any candidate is produced, so
// ordinary sparse rules (e.g. BYWEEKNO=53, true only in some years) are
// unaffected.
const maxConsecutiveEmptyPeriods = 100000

// walkRRule yields the RRULE-only occurrences of r (not RDATE, not minus
// EXDATE) in chronological order, honoring COUNT and UNTIL. It is the
// shared core behind Iter, Between, After, Before and All.
func (r *Rule) walkRRule(yield func(time.Time) bool) {
	if !r.HasRRule {
		return
	}
	loc := r.DTStart.Location()
	count := 0
	stop := false
	var prev time.Time
	havePrev := false

	// emit returns false once the caller should stop: yield said so, the
	// UNTIL bound was passed, or COUNT was reached.
	emit := func(t time.Time) bool {
		if t.Before(r.DTStart) {
			// The recurrence set never contains an instant before
			// its own anchor. This only matters for the anchor's
			// own period: e.g. DTSTART 1997-09-30 with
			// BYMONTHDAY=1,-1 would otherwise also emit
			// 1997-09-01, which is before the rule even starts.
			return true
		}
		if havePrev && prev.Equal(t) {
			// Two different calendar-day candidates can resolve
			// to the same instant: RFC 5545 §3.3.10's gap rule,
			// applied to a whole day skipped by a time zone (e.g.
			// Pacific/Apia's 2011-12-30), shifts that day's
			// occurrence forward onto the same instant the very
			// next day already produces. RFC 5545 §3.8.5.3 says
			// duplicate instances are ignored and count once; if
			// this were not deduplicated here, a COUNT-bounded
			// rule would fall one short, having spent a COUNT slot
			// on an instant nothing downstream ever sees as new.
			return true
		}
		if !r.Until.IsZero() && t.After(r.Until) {
			stop = true
			return false
		}
		if !yield(t) {
			stop = true
			return false
		}
		prev, havePrev = t, true
		count++
		if r.Count > 0 && count >= r.Count {
			stop = true
			return false
		}
		return true
	}

	switch r.Freq {
	case Yearly, Monthly, Weekly, Daily:
		times := timeset(r)
		empty := 0
		advance := func(a ymd) ymd {
			switch r.Freq {
			case Yearly:
				return ymd{a.y + r.Interval, a.m, a.d}
			case Monthly:
				total := (int(a.m) - 1) + r.Interval
				return ymd{a.y + total/12, time.Month(total%12 + 1), a.d}
			case Weekly:
				return addDays(a, 7*r.Interval)
			default: // Daily
				return addDays(a, r.Interval)
			}
		}
		anchor := ymd{r.DTStart.Year(), r.DTStart.Month(), r.DTStart.Day()}
		if r.Freq == Weekly {
			anchor = weekStart(anchor, r.WKST)
		}
		for {
			days := periodDays(r.Freq, anchor, r)
			var periodTimes []time.Time
			for _, d := range days {
				for _, tm := range times {
					periodTimes = append(periodTimes, resolveLocal(d.y, d.m, d.d, tm.h, tm.mi, tm.s, loc))
				}
			}
			sort.Slice(periodTimes, func(i, j int) bool { return periodTimes[i].Before(periodTimes[j]) })
			periodTimes = applySetPos(periodTimes, r.BySetPos)
			if len(periodTimes) == 0 {
				empty++
				if empty >= maxConsecutiveEmptyPeriods {
					return
				}
			} else {
				empty = 0
			}
			for _, t := range periodTimes {
				if !emit(t) {
					return
				}
			}
			if stop {
				return
			}
			anchor = advance(anchor)
		}

	case Hourly, Minutely, Secondly:
		unit := stepUnit(r.Freq)
		step := time.Duration(r.Interval) * unit
		instant := r.DTStart
		empty := 0
		for {
			var cands []time.Time
			switch r.Freq {
			case Hourly:
				cands = hourlyStepCandidates(instant, r)
			case Minutely:
				cands = minutelyStepCandidates(instant, r)
			default:
				cands = secondlyStepCandidates(instant, r)
			}
			if len(cands) == 0 {
				empty++
				if empty >= maxConsecutiveEmptyPeriods {
					return
				}
			} else {
				empty = 0
			}
			for _, t := range cands {
				if !emit(t) {
					return
				}
			}
			if stop {
				return
			}
			instant = instant.Add(step)
		}
	}
}

// fullSeq yields the Rule's complete occurrence set: the RRULE expansion
// and RDate, merged in chronological order, with ExDate removed. Instants
// are compared with time.Time.Equal (so the same instant expressed with a
// different offset still dedupes), and exact (Equal) duplicates between
// RRULE and RDate collapse to one occurrence, per RFC 5545 §3.8.5.3.
func (r *Rule) fullSeq(yield func(time.Time) bool) {
	rdate := append([]time.Time(nil), r.RDate...)
	if !r.HasRRule {
		// With no RRULE, DTStart is not just an anchor: it is the
		// event, so it belongs in the occurrence set the same way an
		// explicit RDATE would (subject to ExDate, like any other
		// occurrence). When there is an RRULE, DTStart is left to
		// walkRRule instead: it appears only if it actually satisfies
		// the rule's BYxxx parts, matching every RFC 5545 §3.8.5.3
		// example (none of which has a DTSTART that mismatches its
		// own rule) and this package's differential tests against
		// python-dateutil, which take the same view.
		rdate = append(rdate, r.DTStart)
	}
	sort.Slice(rdate, func(i, j int) bool { return rdate[i].Before(rdate[j]) })

	excluded := func(t time.Time) bool {
		for _, ex := range r.ExDate {
			if ex.Equal(t) {
				return true
			}
		}
		return false
	}

	next, stopPull := iter.Pull(r.walkRRule)
	defer stopPull()
	rt, rok := next()
	ri := 0

	var lastEmitted time.Time
	haveLast := false
	emit := func(t time.Time) bool {
		if haveLast && lastEmitted.Equal(t) {
			return true
		}
		if excluded(t) {
			return true
		}
		lastEmitted = t
		haveLast = true
		return yield(t)
	}

	for rok || ri < len(rdate) {
		switch {
		case rok && ri < len(rdate):
			if rt.Before(rdate[ri]) {
				if !emit(rt) {
					return
				}
				rt, rok = next()
			} else {
				if !emit(rdate[ri]) {
					return
				}
				ri++
			}
		case rok:
			if !emit(rt) {
				return
			}
			rt, rok = next()
		default:
			if !emit(rdate[ri]) {
				return
			}
			ri++
		}
	}
}

// Iter returns the Rule's occurrences t with from <= t < to. A zero from
// means "from the beginning of the recurrence" and a zero to means "no
// upper bound" — the caller must eventually stop ranging (e.g. with break)
// over an unbounded rule.
func (r *Rule) Iter(from, to time.Time) iter.Seq[time.Time] {
	return func(yield func(time.Time) bool) {
		for t := range r.fullSeq {
			if !from.IsZero() && t.Before(from) {
				continue
			}
			if !to.IsZero() && !t.Before(to) {
				return
			}
			if !yield(t) {
				return
			}
		}
	}
}

// Between returns, in chronological order, every occurrence t with
// from <= t < to.
func (r *Rule) Between(from, to time.Time) []time.Time {
	var out []time.Time
	for t := range r.Iter(from, to) {
		out = append(out, t)
	}
	return out
}

// After returns the first occurrence strictly after t, or the zero Time and
// false if there is none (COUNT/UNTIL exhausted, or the whole rule has no
// RRULE and no RDate after t). If inclusive is true, an occurrence exactly
// equal to t also qualifies.
func (r *Rule) After(t time.Time, inclusive bool) (time.Time, bool) {
	for got := range r.fullSeq {
		if got.After(t) || (inclusive && got.Equal(t)) {
			return got, true
		}
	}
	return time.Time{}, false
}

// Before returns the last occurrence strictly before t, or the zero Time
// and false if there is none. If inclusive is true, an occurrence exactly
// equal to t also qualifies. Before must walk the whole recurrence set up
// to t, since occurrences are only produced in forward chronological order.
func (r *Rule) Before(t time.Time, inclusive bool) (time.Time, bool) {
	var best time.Time
	found := false
	for got := range r.fullSeq {
		if got.After(t) {
			break
		}
		if got.Equal(t) && !inclusive {
			continue
		}
		best = got
		found = true
	}
	if !found {
		return time.Time{}, false
	}
	return best, true
}

// All returns the first limit occurrences of the recurrence set, or fewer
// if COUNT/UNTIL/RDate exhaust it first. limit must be positive; All exists
// specifically to put a bound on rules that would otherwise repeat forever.
func (r *Rule) All(limit int) []time.Time {
	if limit <= 0 {
		return nil
	}
	out := make([]time.Time, 0, limit)
	for t := range r.fullSeq {
		out = append(out, t)
		if len(out) >= limit {
			break
		}
	}
	return out
}
