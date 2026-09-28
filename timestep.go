package recur

import "time"

// HOURLY, MINUTELY and SECONDLY step by absolute duration — instant.Add(n *
// interval * unit) — rather than by adding to the wall-clock hour/minute/
// second field. RFC 5545 does not spell this out as explicitly as it does
// the DST resolution rules in §3.3.5, but it is the only reading consistent
// with "an interval of an hour" (§3.3.10's definition of HOURLY): a real
// hour is 3600 real seconds, not "increment the local hour field by one,
// whatever that means across a transition". Adding to wall-clock fields
// instead is exactly the bug this package's design doc (BUILD_COMMON.md /
// the task brief) reports against teambition/rrule-go: an HOURLY rule
// crossing Sydney's October 2024 spring-forward emits 03:00 AEDT twice, the
// same instant counted as two occurrences. Absolute-duration stepping
// cannot produce a duplicate or a gap: each step is a distinct instant by
// construction.
//
// BYSECOND/BYMINUTE/BYHOUR still expand or limit within each step's period,
// per the table in RFC 5545 §3.3.10 (BYMINUTE and BYSECOND expand a step
// that is coarser than themselves; BYMONTH/BYYEARDAY/BYMONTHDAY/BYDAY/
// BYHOUR/BYMINUTE/BYSECOND that are the same granularity as, or coarser
// than, FREQ act as limits on the stepped instant).

func passesDateLimits(y int, mo time.Month, d int, r *Rule) bool {
	dd := ymd{y, mo, d}
	if len(r.ByMonth) > 0 && !containsInt(r.ByMonth, int(mo)) {
		return false
	}
	if len(r.ByMonthDay) > 0 {
		dim := daysInMonth(y, mo)
		ok := false
		for _, v := range r.ByMonthDay {
			if day, valid := resolveMonthDay(dim, v); valid && day == d {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(r.ByYearDay) > 0 {
		n := yearLength(y)
		doy := dayOfYear(dd)
		ok := false
		for _, v := range r.ByYearDay {
			idx := v
			if v < 0 {
				idx = n + v + 1
			}
			if idx == doy {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(r.ByDay) > 0 && !matchesWeekdaySet(dd, r.ByDay) {
		return false
	}
	return true
}

// hourlyStepCandidates returns the occurrences the hour-period starting at
// instant contributes.
func hourlyStepCandidates(instant time.Time, r *Rule) []time.Time {
	loc := instant.Location()
	y, mo, d := instant.Date()
	h := instant.Hour()
	if !passesDateLimits(y, mo, d, r) {
		return nil
	}
	if len(r.ByHour) > 0 && !containsInt(r.ByHour, h) {
		return nil
	}
	mins := r.ByMinute
	if len(mins) == 0 {
		mins = []int{r.DTStart.Minute()}
	} else {
		mins = sortInts(mins)
	}
	secs := r.BySecond
	if len(secs) == 0 {
		secs = []int{r.DTStart.Second()}
	} else {
		secs = sortInts(secs)
	}
	var out []time.Time
	for _, mi := range mins {
		for _, s := range secs {
			out = append(out, resolveLocal(y, mo, d, h, mi, s, loc))
		}
	}
	return applySetPos(out, r.BySetPos)
}

// minutelyStepCandidates returns the occurrences the minute-period starting
// at instant contributes.
func minutelyStepCandidates(instant time.Time, r *Rule) []time.Time {
	loc := instant.Location()
	y, mo, d := instant.Date()
	h, mi, _ := instant.Clock()
	if !passesDateLimits(y, mo, d, r) {
		return nil
	}
	if len(r.ByHour) > 0 && !containsInt(r.ByHour, h) {
		return nil
	}
	if len(r.ByMinute) > 0 && !containsInt(r.ByMinute, mi) {
		return nil
	}
	secs := r.BySecond
	if len(secs) == 0 {
		secs = []int{r.DTStart.Second()}
	} else {
		secs = sortInts(secs)
	}
	var out []time.Time
	for _, s := range secs {
		out = append(out, resolveLocal(y, mo, d, h, mi, s, loc))
	}
	return applySetPos(out, r.BySetPos)
}

// secondlyStepCandidates returns the (at most one) occurrence the
// second-period starting at instant contributes.
func secondlyStepCandidates(instant time.Time, r *Rule) []time.Time {
	loc := instant.Location()
	y, mo, d := instant.Date()
	h, mi, s := instant.Clock()
	if !passesDateLimits(y, mo, d, r) {
		return nil
	}
	if len(r.ByHour) > 0 && !containsInt(r.ByHour, h) {
		return nil
	}
	if len(r.ByMinute) > 0 && !containsInt(r.ByMinute, mi) {
		return nil
	}
	if len(r.BySecond) > 0 && !containsInt(r.BySecond, s) {
		return nil
	}
	out := []time.Time{resolveLocal(y, mo, d, h, mi, s, loc)}
	return applySetPos(out, r.BySetPos)
}

// stepUnit returns the absolute duration of one FREQ unit for the
// sub-daily frequencies.
func stepUnit(freq Frequency) time.Duration {
	switch freq {
	case Hourly:
		return time.Hour
	case Minutely:
		return time.Minute
	case Secondly:
		return time.Second
	}
	return 0
}
