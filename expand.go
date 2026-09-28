package recur

import (
	"sort"
	"time"
)

// This file builds, for one period of the rule's base frequency (one
// calendar year for YEARLY, one month for MONTHLY, one WKST-aligned week for
// WEEKLY, one day for DAILY), the set of calendar days RFC 5545 §3.3.10
// says that period contributes, following the BYMONTH, BYWEEKNO, BYYEARDAY,
// BYMONTHDAY, BYDAY evaluation order the RFC specifies. HOURLY/MINUTELY/
// SECONDLY are handled separately in timestep.go, by stepping absolute
// duration rather than calendar units — see that file's doc comment for why.

func resolveMonthDay(dim, v int) (int, bool) {
	if v > 0 {
		if v > dim {
			return 0, false
		}
		return v, true
	}
	day := dim + v + 1
	if day < 1 {
		return 0, false
	}
	return day, true
}

func fromMonthDay(y int, months []int, byMonthDay []int) []ymd {
	var out []ymd
	for _, mo := range months {
		m := time.Month(mo)
		dim := daysInMonth(y, m)
		for _, v := range byMonthDay {
			if day, ok := resolveMonthDay(dim, v); ok {
				out = append(out, ymd{y, m, day})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	return out
}

func fromYearDay(y int, byYearDay []int) []ymd {
	n := yearLength(y)
	var out []ymd
	for _, v := range byYearDay {
		var idx int
		if v > 0 {
			if v > n {
				continue
			}
			idx = v
		} else {
			idx = n + v + 1
			if idx < 1 {
				continue
			}
		}
		out = append(out, dateFromDayOfYear(y, idx))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	return out
}

func fromWeekNo(y int, byWeekNo []int, wkst time.Weekday) []ymd {
	w1 := yearWeek1Start(y, wkst)
	w1Next := yearWeek1Start(y+1, wkst)
	total := daysBetween(w1, w1Next) / 7
	var out []ymd
	for _, v := range byWeekNo {
		var idx int
		if v > 0 {
			idx = v
		} else {
			idx = total + v + 1
		}
		if idx < 1 || idx > total {
			continue
		}
		start := addDays(w1, (idx-1)*7)
		for i := 0; i < 7; i++ {
			out = append(out, addDays(start, i))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	return out
}

func filterMonth(days []ymd, byMonth []int) []ymd {
	if len(byMonth) == 0 {
		return days
	}
	var out []ymd
	for _, d := range days {
		if containsInt(byMonth, int(d.m)) {
			out = append(out, d)
		}
	}
	return out
}

func matchesWeekdaySet(d ymd, byDay []Weekday) bool {
	wd := weekdayOf(d)
	for _, w := range byDay {
		if w.Day == wd {
			return true
		}
	}
	return false
}

func filterWeekday(days []ymd, byDay []Weekday) []ymd {
	var out []ymd
	for _, d := range days {
		if matchesWeekdaySet(d, byDay) {
			out = append(out, d)
		}
	}
	return out
}

// selectByDay applies BYDAY within one scope (days must already be sorted
// and belong to a single scope: a month, or a whole year). Entries with
// N==0 match every occurrence of that weekday in scope; entries with an
// ordinal select only the Nth (1-based, negative counts from the end).
func selectByDay(days []ymd, byDay []Weekday) []ymd {
	seen := make(map[ymd]bool)
	var out []ymd
	add := func(d ymd) {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, w := range byDay {
		if w.N == 0 {
			for _, d := range days {
				if weekdayOf(d) == w.Day {
					add(d)
				}
			}
			continue
		}
		var matches []ymd
		for _, d := range days {
			if weekdayOf(d) == w.Day {
				matches = append(matches, d)
			}
		}
		var idx int
		if w.N > 0 {
			idx = w.N - 1
		} else {
			idx = len(matches) + w.N
		}
		if idx >= 0 && idx < len(matches) {
			add(matches[idx])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	return out
}

// yearlyDays returns the candidate days year y contributes, per RFC 5545
// §3.3.10's evaluation order (BYMONTH, BYWEEKNO, BYYEARDAY, BYMONTHDAY,
// BYDAY) and its expand/limit table.
func yearlyDays(y int, r *Rule) []ymd {
	switch {
	case len(r.ByYearDay) > 0:
		days := fromYearDay(y, r.ByYearDay)
		if len(r.ByDay) > 0 {
			days = filterWeekday(days, r.ByDay)
		}
		return filterMonth(days, r.ByMonth)

	case len(r.ByWeekNo) > 0:
		days := fromWeekNo(y, r.ByWeekNo, r.WKST)
		if len(r.ByDay) > 0 {
			days = filterWeekday(days, r.ByDay)
		}
		return filterMonth(days, r.ByMonth)

	case len(r.ByMonthDay) > 0:
		months := r.ByMonth
		if len(months) == 0 {
			months = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
		}
		days := fromMonthDay(y, months, r.ByMonthDay)
		if len(r.ByDay) > 0 {
			days = filterWeekday(days, r.ByDay)
		}
		return days

	case len(r.ByDay) > 0:
		if len(r.ByMonth) > 0 {
			var out []ymd
			for _, mo := range sortInts(r.ByMonth) {
				out = append(out, selectByDay(allDaysInMonth(y, time.Month(mo)), r.ByDay)...)
			}
			sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
			return out
		}
		return selectByDay(allDaysInYear(y), r.ByDay)

	case len(r.ByMonth) > 0:
		var out []ymd
		for _, mo := range sortInts(r.ByMonth) {
			m := time.Month(mo)
			if validDate(y, m, r.DTStart.Day()) {
				out = append(out, ymd{y, m, r.DTStart.Day()})
			}
		}
		return out

	default:
		if validDate(y, r.DTStart.Month(), r.DTStart.Day()) {
			return []ymd{{y, r.DTStart.Month(), r.DTStart.Day()}}
		}
		return nil
	}
}

// monthlyDays returns the candidate days month (y, mo) contributes.
func monthlyDays(y int, mo time.Month, r *Rule) []ymd {
	switch {
	case len(r.ByMonthDay) > 0:
		days := fromMonthDay(y, []int{int(mo)}, r.ByMonthDay)
		if len(r.ByDay) > 0 {
			days = filterWeekday(days, r.ByDay)
		}
		return days
	case len(r.ByDay) > 0:
		return selectByDay(allDaysInMonth(y, mo), r.ByDay)
	default:
		if validDate(y, mo, r.DTStart.Day()) {
			return []ymd{{y, mo, r.DTStart.Day()}}
		}
		return nil
	}
}

// weeklyDays returns the candidate days for the WKST-aligned week starting
// at ws.
func weeklyDays(ws ymd, r *Rule) []ymd {
	days := make([]ymd, 7)
	for i := range days {
		days[i] = addDays(ws, i)
	}
	days = filterMonth(days, r.ByMonth)
	if len(r.ByDay) > 0 {
		return filterWeekday(days, r.ByDay)
	}
	want := r.DTStart.Weekday()
	var out []ymd
	for _, d := range days {
		if weekdayOf(d) == want {
			out = append(out, d)
		}
	}
	return out
}

// dailyDays returns d itself if it passes every BYxxx limit that applies to
// FREQ=DAILY (BYMONTH, BYMONTHDAY, BYYEARDAY, BYDAY are all pure limits at
// this granularity — see the expand/limit table in RFC 5545 §3.3.10).
func dailyDays(d ymd, r *Rule) []ymd {
	if len(r.ByMonth) > 0 && !containsInt(r.ByMonth, int(d.m)) {
		return nil
	}
	if len(r.ByMonthDay) > 0 {
		dim := daysInMonth(d.y, d.m)
		ok := false
		for _, v := range r.ByMonthDay {
			if day, valid := resolveMonthDay(dim, v); valid && day == d.d {
				ok = true
				break
			}
		}
		if !ok {
			return nil
		}
	}
	if len(r.ByYearDay) > 0 {
		n := yearLength(d.y)
		doy := dayOfYear(d)
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
			return nil
		}
	}
	if len(r.ByDay) > 0 && !matchesWeekdaySet(d, r.ByDay) {
		return nil
	}
	return []ymd{d}
}

// timeOfDay is one (hour, minute, second) combination.
type timeOfDay struct{ h, mi, s int }

// timeset returns the BYHOUR/BYMINUTE/BYSECOND cross product (Expand, for
// FREQ=DAILY and coarser), defaulting each missing part to DTSTART's value,
// sorted ascending so day x time enumeration stays chronological.
func timeset(r *Rule) []timeOfDay {
	hours := r.ByHour
	if len(hours) == 0 {
		hours = []int{r.DTStart.Hour()}
	} else {
		hours = sortInts(hours)
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
	out := make([]timeOfDay, 0, len(hours)*len(mins)*len(secs))
	for _, h := range hours {
		for _, mi := range mins {
			for _, s := range secs {
				out = append(out, timeOfDay{h, mi, s})
			}
		}
	}
	return out
}

// applySetPos selects the BySetPos-indexed elements of a chronologically
// sorted candidate list (1-based; negative counts from the end), per RFC
// 5545 §3.3.10's BYSETPOS description, and returns them still in
// chronological order.
func applySetPos(candidates []time.Time, bySetPos []int) []time.Time {
	if len(bySetPos) == 0 {
		return candidates
	}
	n := len(candidates)
	pick := make(map[int]bool)
	for _, v := range bySetPos {
		var idx int
		if v > 0 {
			idx = v - 1
		} else {
			idx = n + v
		}
		if idx >= 0 && idx < n {
			pick[idx] = true
		}
	}
	var out []time.Time
	for i, c := range candidates {
		if pick[i] {
			out = append(out, c)
		}
	}
	return out
}

// periodDays returns the candidate calendar days for the date-based
// frequencies (YEARLY, MONTHLY, WEEKLY, DAILY only; HOURLY/MINUTELY/
// SECONDLY are handled by timestep.go).
func periodDays(freq Frequency, anchor ymd, r *Rule) []ymd {
	switch freq {
	case Yearly:
		return yearlyDays(anchor.y, r)
	case Monthly:
		return monthlyDays(anchor.y, anchor.m, r)
	case Weekly:
		return weeklyDays(anchor, r)
	case Daily:
		return dailyDays(anchor, r)
	}
	return nil
}
