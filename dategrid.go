package recur

import "time"

// ymd is a plain calendar date, used internally while building candidate
// sets before they are resolved to instants via resolveLocal.
type ymd struct {
	y int
	m time.Month
	d int
}

func (a ymd) less(b ymd) bool {
	if a.y != b.y {
		return a.y < b.y
	}
	if a.m != b.m {
		return a.m < b.m
	}
	return a.d < b.d
}

func daysInMonth(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func isLeap(y int) bool {
	return (y%4 == 0 && y%100 != 0) || y%400 == 0
}

func yearLength(y int) int {
	if isLeap(y) {
		return 366
	}
	return 365
}

// dateFromDayOfYear returns the ymd for the n-th day of year y (1-based).
func dateFromDayOfYear(y, n int) ymd {
	t := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n-1)
	return ymd{t.Year(), t.Month(), t.Day()}
}

// dayOfYear returns the 1-based ordinal of d within its year.
func dayOfYear(d ymd) int {
	jan1 := time.Date(d.y, 1, 1, 0, 0, 0, 0, time.UTC)
	this := time.Date(d.y, d.m, d.d, 0, 0, 0, 0, time.UTC)
	return int(this.Sub(jan1).Hours()/24) + 1
}

func weekdayOf(d ymd) time.Weekday {
	return time.Date(d.y, d.m, d.d, 0, 0, 0, 0, time.UTC).Weekday()
}

func addDays(d ymd, n int) ymd {
	t := time.Date(d.y, d.m, d.d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n)
	return ymd{t.Year(), t.Month(), t.Day()}
}

func daysBetween(a, b ymd) int {
	ta := time.Date(a.y, a.m, a.d, 0, 0, 0, 0, time.UTC)
	tb := time.Date(b.y, b.m, b.d, 0, 0, 0, 0, time.UTC)
	return int(tb.Sub(ta).Hours() / 24)
}

func allDaysInYear(y int) []ymd {
	n := yearLength(y)
	out := make([]ymd, n)
	d := ymd{y, time.January, 1}
	for i := 0; i < n; i++ {
		out[i] = d
		d = addDays(d, 1)
	}
	return out
}

func allDaysInMonth(y int, m time.Month) []ymd {
	n := daysInMonth(y, m)
	out := make([]ymd, n)
	for i := 0; i < n; i++ {
		out[i] = ymd{y, m, i + 1}
	}
	return out
}

// weekStart returns the wkst-aligned start of the week containing d.
func weekStart(d ymd, wkst time.Weekday) ymd {
	wd := weekdayOf(d)
	back := (int(wd) - int(wkst) + 7) % 7
	return addDays(d, -back)
}

// yearWeek1Start returns the wkst-aligned start of week 1 of year y, per
// RFC 5545's rule (borrowed from ISO 8601): week 1 is the WKST-aligned week
// containing at least 4 days of year y.
func yearWeek1Start(y int, wkst time.Weekday) ymd {
	jan1 := ymd{y, time.January, 1}
	ws := weekStart(jan1, wkst)
	daysOfNewYearInWS := 7 - daysBetween(ws, jan1)
	if daysOfNewYearInWS >= 4 {
		return ws
	}
	return addDays(ws, 7)
}

func validDate(y int, m time.Month, d int) bool {
	if m < time.January || m > time.December || d < 1 {
		return false
	}
	return d <= daysInMonth(y, m)
}

func sortInts(s []int) []int {
	out := append([]int(nil), s...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
