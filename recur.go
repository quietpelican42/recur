// Package recur implements RFC 5545 (iCalendar) recurrence rules: parsing
// RRULE/RDATE/EXDATE, and expanding them into concrete occurrences.
//
// Unlike a naive implementation that adds calendar units to a time.Time,
// this package resolves every occurrence through the local wall-clock rules
// RFC 5545 §3.3.5 specifies for DST transitions: a local time that does not
// exist (spring-forward gap) is interpreted using the UTC offset in effect
// before the gap, and a local time that occurs twice (fall-back) resolves to
// its first occurrence. See resolve.go for the mechanism.
package recur

import (
	"fmt"
	"time"
)

// Frequency is the base recurrence frequency (RFC 5545 §3.3.10, the FREQ
// rule part).
type Frequency int

// The seven RFC 5545 frequencies, ordered coarsest first for internal
// comparisons.
const (
	Yearly Frequency = iota
	Monthly
	Weekly
	Daily
	Hourly
	Minutely
	Secondly
)

// String returns the RFC 5545 FREQ token for f.
func (f Frequency) String() string {
	switch f {
	case Yearly:
		return "YEARLY"
	case Monthly:
		return "MONTHLY"
	case Weekly:
		return "WEEKLY"
	case Daily:
		return "DAILY"
	case Hourly:
		return "HOURLY"
	case Minutely:
		return "MINUTELY"
	case Secondly:
		return "SECONDLY"
	default:
		return "INVALID"
	}
}

// Weekday is a day of the week with an optional ordinal, as used in the
// BYDAY rule part (e.g. "2TU" is Weekday{Day: time.Tuesday, N: 2}, the
// second Tuesday of the applicable period; "-1FR", the last Friday, is
// FR.Nth(-1)).
type Weekday struct {
	Day time.Weekday
	N   int // 0 means "every occurrence of Day", no ordinal.
}

// Nth returns w with the ordinal n. n is only meaningful for MONTHLY and
// YEARLY rules; RFC 5545 forbids it elsewhere.
func (w Weekday) Nth(n int) Weekday { return Weekday{Day: w.Day, N: n} }

// The seven unqualified weekdays, usable directly in ByDay or qualified via
// Nth, e.g. FR.Nth(1) for "the first Friday".
var (
	SU = Weekday{Day: time.Sunday}
	MO = Weekday{Day: time.Monday}
	TU = Weekday{Day: time.Tuesday}
	WE = Weekday{Day: time.Wednesday}
	TH = Weekday{Day: time.Thursday}
	FR = Weekday{Day: time.Friday}
	SA = Weekday{Day: time.Saturday}
)

func weekdayName(d time.Weekday) string {
	return [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}[d]
}

// Rule is a fully parsed recurrence: a DTSTART anchor, at most one RRULE
// expansion, and any RDATE/EXDATE additions and exclusions.
//
// The zero value is not usable; construct a Rule with New or Parse.
type Rule struct {
	// DTStart is the anchor instant. Its Location determines the time
	// zone every generated occurrence is expressed in, and it always
	// counts as the first occurrence of the recurrence set (subject to
	// EXDATE and to the BYxxx parts actually matching it, per RFC
	// 5545 §3.8.5.3).
	DTStart time.Time
	// DTStartFloating records whether DTStart was parsed from a floating
	// DATE-TIME (RFC 5545 §3.3.5, FORM #1: no "Z", no TZID). A floating
	// DTStart is represented internally in time.UTC — see Parse's
	// DTSTART case for why — so this flag is the only way to tell
	// "floating, incidentally UTC-backed" apart from "genuinely UTC"
	// when serializing with String. Rules built with New rather than
	// Parse are never floating.
	DTStartFloating bool

	// HasRRule reports whether an expansion rule is present. A Rule with
	// only RDATE/EXDATE entries (no RRULE) is valid: it is just DTStart
	// plus RDate, minus ExDate.
	HasRRule bool

	Freq     Frequency
	Interval int // Always >= 1. Zero on input means 1 (the RFC default).

	// Count and Until are mutually exclusive per RFC 5545. Count == 0
	// and Until.IsZero() both mean "repeat forever".
	Count int
	Until time.Time
	// UntilFloating records whether Until was parsed from a floating
	// (no "Z", no TZID) DATE-TIME, matching DTSTART's own floating
	// form. A floating Until is compared in DTStart's Location.
	UntilFloating bool

	BySecond   []int
	ByMinute   []int
	ByHour     []int
	ByDay      []Weekday
	ByMonthDay []int
	ByYearDay  []int
	ByWeekNo   []int
	ByMonth    []int
	BySetPos   []int

	// WKST is the day the work week starts on. Always resolved by the
	// time a Rule is returned from New or Parse: RFC 5545's default of
	// Monday has already been applied if the caller did not specify one.
	WKST time.Weekday

	// RDate lists additional explicit occurrences, merged into the
	// recurrence set. ExDate lists occurrences to remove; exclusion
	// always wins over inclusion, per RFC 5545 §3.8.5.3.
	RDate  []time.Time
	ExDate []time.Time
}

// Options builds a Rule with New. It mirrors Rule's fields except for WKST,
// which is a pointer here so New can tell "left unset" (defaults to Monday,
// the RFC 5545 default) apart from "explicitly set to Sunday" — time.Sunday
// is zero in time.Weekday, so a plain time.Weekday field could not carry
// that distinction.
type Options struct {
	DTStart       time.Time
	HasRRule      bool
	Freq          Frequency
	Interval      int
	Count         int
	Until         time.Time
	UntilFloating bool
	BySecond      []int
	ByMinute      []int
	ByHour        []int
	ByDay         []Weekday
	ByMonthDay    []int
	ByYearDay     []int
	ByWeekNo      []int
	ByMonth       []int
	BySetPos      []int
	WKST          *time.Weekday
	RDate         []time.Time
	ExDate        []time.Time
}

// New validates opts and returns the Rule it describes. Most callers parse
// RRULE text with Parse instead; New is for building a Rule programmatically.
func New(opts Options) (*Rule, error) {
	if opts.DTStart.IsZero() {
		return nil, fmt.Errorf("recur: DTStart is required")
	}
	r := &Rule{
		DTStart:       opts.DTStart,
		HasRRule:      opts.HasRRule,
		Freq:          opts.Freq,
		Interval:      opts.Interval,
		Count:         opts.Count,
		Until:         opts.Until,
		UntilFloating: opts.UntilFloating,
		BySecond:      opts.BySecond,
		ByMinute:      opts.ByMinute,
		ByHour:        opts.ByHour,
		ByDay:         opts.ByDay,
		ByMonthDay:    opts.ByMonthDay,
		ByYearDay:     opts.ByYearDay,
		ByWeekNo:      opts.ByWeekNo,
		ByMonth:       opts.ByMonth,
		BySetPos:      opts.BySetPos,
		WKST:          time.Monday,
		RDate:         opts.RDate,
		ExDate:        opts.ExDate,
	}
	if opts.WKST != nil {
		r.WKST = *opts.WKST
	}
	if r.Interval == 0 {
		r.Interval = 1
	}
	if err := r.validate(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Rule) validate() error {
	if !r.HasRRule {
		return nil
	}
	if r.Freq < Yearly || r.Freq > Secondly {
		return fmt.Errorf("%w: invalid FREQ", ErrInvalidRule)
	}
	if r.Interval < 1 {
		return fmt.Errorf("%w: INTERVAL must be >= 1", ErrInvalidRule)
	}
	if r.Count > 0 && !r.Until.IsZero() {
		return fmt.Errorf("%w: COUNT and UNTIL are mutually exclusive", ErrInvalidRule)
	}
	if len(r.ByWeekNo) > 0 && r.Freq != Yearly {
		return fmt.Errorf("%w: BYWEEKNO is only valid with FREQ=YEARLY", ErrUnsupported)
	}
	if len(r.ByYearDay) > 0 && (r.Freq == Daily || r.Freq == Weekly || r.Freq == Monthly) {
		return fmt.Errorf("%w: BYYEARDAY is not valid with FREQ=%s", ErrUnsupported, r.Freq)
	}
	if len(r.ByMonthDay) > 0 && r.Freq == Weekly {
		return fmt.Errorf("%w: BYMONTHDAY is not valid with FREQ=WEEKLY", ErrUnsupported)
	}
	for _, wd := range r.ByDay {
		if wd.N != 0 && r.Freq != Monthly && r.Freq != Yearly {
			return fmt.Errorf("%w: BYDAY ordinals are only valid with FREQ=MONTHLY or FREQ=YEARLY", ErrUnsupported)
		}
		if wd.N != 0 && r.Freq == Yearly && len(r.ByWeekNo) > 0 {
			return fmt.Errorf("%w: BYDAY ordinals are not valid with FREQ=YEARLY and BYWEEKNO", ErrUnsupported)
		}
	}
	if len(r.BySetPos) > 0 {
		if len(r.BySecond) == 0 && len(r.ByMinute) == 0 && len(r.ByHour) == 0 &&
			len(r.ByDay) == 0 && len(r.ByMonthDay) == 0 && len(r.ByYearDay) == 0 &&
			len(r.ByWeekNo) == 0 && len(r.ByMonth) == 0 {
			return fmt.Errorf("%w: BYSETPOS requires another BYxxx rule part", ErrInvalidRule)
		}
	}
	for _, v := range r.BySecond {
		if v < 0 || v > 60 {
			return fmt.Errorf("%w: BYSECOND=%d out of range 0-60", ErrInvalidRule, v)
		}
	}
	for _, v := range r.ByMinute {
		if v < 0 || v > 59 {
			return fmt.Errorf("%w: BYMINUTE=%d out of range 0-59", ErrInvalidRule, v)
		}
	}
	for _, v := range r.ByHour {
		if v < 0 || v > 23 {
			return fmt.Errorf("%w: BYHOUR=%d out of range 0-23", ErrInvalidRule, v)
		}
	}
	for _, v := range r.ByMonth {
		if v < 1 || v > 12 {
			return fmt.Errorf("%w: BYMONTH=%d out of range 1-12", ErrInvalidRule, v)
		}
	}
	for _, v := range r.ByMonthDay {
		if v == 0 || v < -31 || v > 31 {
			return fmt.Errorf("%w: BYMONTHDAY=%d out of range 1-31 or -1..-31", ErrInvalidRule, v)
		}
	}
	for _, v := range r.ByYearDay {
		if v == 0 || v < -366 || v > 366 {
			return fmt.Errorf("%w: BYYEARDAY=%d out of range 1-366 or -1..-366", ErrInvalidRule, v)
		}
	}
	for _, v := range r.ByWeekNo {
		if v == 0 || v < -53 || v > 53 {
			return fmt.Errorf("%w: BYWEEKNO=%d out of range 1-53 or -1..-53", ErrInvalidRule, v)
		}
	}
	return nil
}
