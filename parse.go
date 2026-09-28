package recur

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Parse parses an iCalendar recurrence block (RFC 5545 §3.8.5, §3.3.10): a
// DTSTART line, at most one RRULE line, and any number of RDATE/EXDATE
// lines, separated by "\n" or "\r\n". DTSTART is required — every other
// part of this package needs an anchor instant and time zone to resolve
// occurrences against.
//
// Example:
//
//	DTSTART;TZID=America/New_York:19970902T090000
//	RRULE:FREQ=DAILY;COUNT=10
//
// A bare recurrence value with no DTSTART line ("FREQ=DAILY;COUNT=10") is
// also accepted as a single-line RRULE, but the returned Rule then has a
// zero DTStart and every method that expands occurrences returns
// ErrInvalidRule; set Rule.DTStart (and re-validate by round-tripping
// through New) before using it.
func Parse(s string) (*Rule, error) {
	lines := splitLines(s)
	r := &Rule{WKST: time.Monday}
	var dtstartSeen bool
	var rruleSeen bool

	for _, line := range lines {
		if line == "" {
			continue
		}
		name, params, value, err := splitProperty(line)
		if err != nil {
			return nil, err
		}
		switch strings.ToUpper(name) {
		case "DTSTART":
			// A floating DTSTART (no "Z", no TZID) is represented
			// internally in time.UTC. That is a deliberate choice,
			// not a claim that the value is UTC: RFC 5545 §3.3.5
			// says a floating value means "the same hour, minute
			// and second regardless of time zone", so it must
			// never be shifted by a DST rule at all. Anchoring it
			// to UTC (which has no DST transitions) keeps every
			// wall-clock computation a no-op, and keeps expansion
			// deterministic across machines in different zones —
			// anchoring it to time.Local instead would make the
			// same rule expand differently depending on where it
			// runs.
			t, floating, err := parseDateTime(value, params)
			if err != nil {
				return nil, fmt.Errorf("recur: DTSTART: %w", err)
			}
			r.DTStart = t
			r.DTStartFloating = floating
			dtstartSeen = true
		case "RRULE":
			if rruleSeen {
				return nil, fmt.Errorf("%w: more than one RRULE", ErrInvalidRule)
			}
			rruleSeen = true
			if err := parseRRuleValue(value, r); err != nil {
				return nil, err
			}
			r.HasRRule = true
		case "RDATE":
			ts, err := parseDateTimeList(value, params)
			if err != nil {
				return nil, fmt.Errorf("recur: RDATE: %w", err)
			}
			r.RDate = append(r.RDate, ts...)
		case "EXDATE":
			ts, err := parseDateTimeList(value, params)
			if err != nil {
				return nil, fmt.Errorf("recur: EXDATE: %w", err)
			}
			r.ExDate = append(r.ExDate, ts...)
		default:
			return nil, fmt.Errorf("%w: unsupported property %q", ErrUnsupported, name)
		}
	}

	if !dtstartSeen {
		return nil, fmt.Errorf("%w: DTSTART is required", ErrInvalidRule)
	}
	if r.HasRRule {
		if r.Interval == 0 {
			r.Interval = 1
		}
		if err := r.validate(); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// splitProperty splits "NAME;P1=V1;P2=V2:VALUE" into name, params, value.
func splitProperty(line string) (name string, params map[string]string, value string, err error) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return "", nil, "", fmt.Errorf("%w: no ':' in property line %q", ErrInvalidRule, line)
	}
	head := line[:colon]
	value = line[colon+1:]
	parts := strings.Split(head, ";")
	name = parts[0]
	params = make(map[string]string)
	for _, p := range parts[1:] {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			params[strings.ToUpper(kv[0])] = kv[1]
		}
	}
	return name, params, value, nil
}

// parseDateTime parses one RFC 5545 DATE-TIME value (§3.3.5): floating
// ("19980118T230000"), UTC ("19980119T070000Z"), or local with TZID via the
// params map. It returns the resolved instant and whether the value was
// floating (no "Z", no TZID).
func parseDateTime(value string, params map[string]string) (time.Time, bool, error) {
	value = strings.TrimSpace(value)
	floating := false
	loc := time.UTC
	var format string
	switch {
	case strings.HasSuffix(value, "Z"):
		format = "20060102T150405Z"
	case params["TZID"] != "":
		l, err := time.LoadLocation(params["TZID"])
		if err != nil {
			return time.Time{}, false, fmt.Errorf("%w: TZID %q: %v", ErrUnsupported, params["TZID"], err)
		}
		loc = l
		format = "20060102T150405"
	default:
		floating = true
		format = "20060102T150405"
	}
	t, err := time.ParseInLocation(format, value, loc)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("%w: bad DATE-TIME %q: %v", ErrInvalidRule, value, err)
	}
	if !floating && params["TZID"] != "" {
		t = resolveLocal(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), loc)
	}
	return t, floating, nil
}

func parseDateTimeList(value string, params map[string]string) ([]time.Time, error) {
	var out []time.Time
	for _, v := range strings.Split(value, ",") {
		t, _, err := parseDateTime(v, params)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

var freqNames = map[string]Frequency{
	"YEARLY": Yearly, "MONTHLY": Monthly, "WEEKLY": Weekly, "DAILY": Daily,
	"HOURLY": Hourly, "MINUTELY": Minutely, "SECONDLY": Secondly,
}

var weekdayNames = map[string]time.Weekday{
	"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday,
}

// parseRRuleValue parses a RECUR value (RFC 5545 §3.3.10), the part after
// "RRULE:", into r. r.DTStart must already be set: it supplies the
// Location that a floating or missing-TZID UNTIL is resolved in.
func parseRRuleValue(value string, r *Rule) error {
	var freqSeen bool
	var untilSeen, countSeen bool
	for _, part := range strings.Split(value, ";") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("%w: malformed rule part %q", ErrInvalidRule, part)
		}
		key, val := strings.ToUpper(kv[0]), kv[1]
		switch key {
		case "FREQ":
			f, ok := freqNames[strings.ToUpper(val)]
			if !ok {
				return fmt.Errorf("%w: unknown FREQ %q", ErrInvalidRule, val)
			}
			r.Freq = f
			freqSeen = true
		case "INTERVAL":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return fmt.Errorf("%w: bad INTERVAL %q", ErrInvalidRule, val)
			}
			r.Interval = n
		case "COUNT":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return fmt.Errorf("%w: bad COUNT %q", ErrInvalidRule, val)
			}
			r.Count = n
			countSeen = true
		case "UNTIL":
			t, floating, err := parseDateTime(val, nil)
			if err != nil {
				return fmt.Errorf("recur: UNTIL: %w", err)
			}
			if floating {
				// RFC 5545: UNTIL must share DTSTART's form. A
				// floating UNTIL is interpreted in DTStart's
				// Location.
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, r.DTStart.Location())
			}
			r.Until = t
			r.UntilFloating = floating
			untilSeen = true
		case "BYSECOND":
			ns, err := intList(val)
			if err != nil {
				return err
			}
			r.BySecond = ns
		case "BYMINUTE":
			ns, err := intList(val)
			if err != nil {
				return err
			}
			r.ByMinute = ns
		case "BYHOUR":
			ns, err := intList(val)
			if err != nil {
				return err
			}
			r.ByHour = ns
		case "BYMONTH":
			ns, err := intList(val)
			if err != nil {
				return err
			}
			r.ByMonth = ns
		case "BYMONTHDAY":
			ns, err := intList(val)
			if err != nil {
				return err
			}
			r.ByMonthDay = ns
		case "BYYEARDAY":
			ns, err := intList(val)
			if err != nil {
				return err
			}
			r.ByYearDay = ns
		case "BYWEEKNO":
			ns, err := intList(val)
			if err != nil {
				return err
			}
			r.ByWeekNo = ns
		case "BYSETPOS":
			ns, err := intList(val)
			if err != nil {
				return err
			}
			r.BySetPos = ns
		case "BYDAY":
			wds, err := weekdayList(val)
			if err != nil {
				return err
			}
			r.ByDay = wds
		case "WKST":
			wd, ok := weekdayNames[strings.ToUpper(val)]
			if !ok {
				return fmt.Errorf("%w: bad WKST %q", ErrInvalidRule, val)
			}
			r.WKST = wd
		default:
			return fmt.Errorf("%w: unknown rule part %q", ErrUnsupported, key)
		}
	}
	if !freqSeen {
		return fmt.Errorf("%w: FREQ is required", ErrInvalidRule)
	}
	if untilSeen && countSeen {
		return fmt.Errorf("%w: COUNT and UNTIL are mutually exclusive", ErrInvalidRule)
	}
	return nil
}

func intList(s string) ([]int, error) {
	var out []int
	for _, v := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("%w: bad integer %q", ErrInvalidRule, v)
		}
		out = append(out, n)
	}
	return out, nil
}

func weekdayList(s string) ([]Weekday, error) {
	var out []Weekday
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if len(v) < 2 {
			return nil, fmt.Errorf("%w: bad BYDAY entry %q", ErrInvalidRule, v)
		}
		code := v[len(v)-2:]
		wd, ok := weekdayNames[strings.ToUpper(code)]
		if !ok {
			return nil, fmt.Errorf("%w: bad BYDAY weekday %q", ErrInvalidRule, code)
		}
		n := 0
		if rest := v[:len(v)-2]; rest != "" {
			parsed, err := strconv.Atoi(rest)
			if err != nil {
				return nil, fmt.Errorf("%w: bad BYDAY ordinal %q", ErrInvalidRule, v)
			}
			n = parsed
		}
		out = append(out, Weekday{Day: wd, N: n})
	}
	return out, nil
}

// String serializes r back to an iCalendar recurrence block: DTSTART, then
// RRULE if present, then RDATE and EXDATE.
func (r *Rule) String() string {
	var b strings.Builder
	writeDT := func(prop string, t time.Time, floating bool) {
		b.WriteString(prop)
		switch {
		case floating:
			fmt.Fprintf(&b, ":%s\n", t.Format("20060102T150405"))
		case t.Location() == time.UTC:
			fmt.Fprintf(&b, ":%sZ\n", t.Format("20060102T150405"))
		default:
			fmt.Fprintf(&b, ";TZID=%s:%s\n", t.Location().String(), t.Format("20060102T150405"))
		}
	}
	writeDT("DTSTART", r.DTStart, r.DTStartFloating)
	if r.HasRRule {
		b.WriteString("RRULE:")
		b.WriteString(r.ruleString())
		b.WriteString("\n")
	}
	if len(r.RDate) > 0 {
		writeList("RDATE", r.RDate, &b)
	}
	if len(r.ExDate) > 0 {
		writeList("EXDATE", r.ExDate, &b)
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeList(prop string, ts []time.Time, b *strings.Builder) {
	sorted := append([]time.Time(nil), ts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Before(sorted[j]) })
	b.WriteString(prop)
	b.WriteString(":")
	for i, t := range sorted {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(t.UTC().Format("20060102T150405") + "Z")
	}
	b.WriteString("\n")
}

// ruleString returns just the RECUR value (no "RRULE:" prefix), matching
// what RFC 5545 §3.3.10 defines and what parseRRuleValue accepts back.
func (r *Rule) ruleString() string {
	var parts []string
	parts = append(parts, "FREQ="+r.Freq.String())
	if r.Interval > 1 {
		parts = append(parts, fmt.Sprintf("INTERVAL=%d", r.Interval))
	}
	if len(r.ByMonth) > 0 {
		parts = append(parts, "BYMONTH="+joinInts(r.ByMonth))
	}
	if len(r.ByWeekNo) > 0 {
		parts = append(parts, "BYWEEKNO="+joinInts(r.ByWeekNo))
	}
	if len(r.ByYearDay) > 0 {
		parts = append(parts, "BYYEARDAY="+joinInts(r.ByYearDay))
	}
	if len(r.ByMonthDay) > 0 {
		parts = append(parts, "BYMONTHDAY="+joinInts(r.ByMonthDay))
	}
	if len(r.ByDay) > 0 {
		items := make([]string, len(r.ByDay))
		for i, w := range r.ByDay {
			if w.N != 0 {
				items[i] = fmt.Sprintf("%d%s", w.N, weekdayName(w.Day))
			} else {
				items[i] = weekdayName(w.Day)
			}
		}
		parts = append(parts, "BYDAY="+strings.Join(items, ","))
	}
	if len(r.ByHour) > 0 {
		parts = append(parts, "BYHOUR="+joinInts(r.ByHour))
	}
	if len(r.ByMinute) > 0 {
		parts = append(parts, "BYMINUTE="+joinInts(r.ByMinute))
	}
	if len(r.BySecond) > 0 {
		parts = append(parts, "BYSECOND="+joinInts(r.BySecond))
	}
	if len(r.BySetPos) > 0 {
		parts = append(parts, "BYSETPOS="+joinInts(r.BySetPos))
	}
	if r.WKST != time.Monday {
		parts = append(parts, "WKST="+weekdayName(r.WKST))
	}
	if r.Count > 0 {
		parts = append(parts, fmt.Sprintf("COUNT=%d", r.Count))
	} else if !r.Until.IsZero() {
		if r.UntilFloating {
			parts = append(parts, "UNTIL="+r.Until.Format("20060102T150405"))
		} else {
			parts = append(parts, "UNTIL="+r.Until.UTC().Format("20060102T150405")+"Z")
		}
	}
	return strings.Join(parts, ";")
}

func joinInts(ns []int) string {
	ss := make([]string, len(ns))
	for i, n := range ns {
		ss[i] = strconv.Itoa(n)
	}
	return strings.Join(ss, ",")
}
