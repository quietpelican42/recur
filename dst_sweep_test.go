package recur

import (
	"fmt"
	"testing"
	"time"
)

// This file extends the DST coverage in dst_test.go past the transition day
// itself. A reviewer's independent probe found that the first resolveLocal
// (a fixed ~26h lookback for "the offset before the gap") produced correct
// answers exactly ON each transition day, which is all the original
// hand-derived tests checked, while silently corrupting ordinary,
// unambiguous times on the days AROUND a transition. These tests are
// designed specifically to catch that class of bug: they check the days
// before AND after a transition, not just the transition instant.

// transitionFixture describes one DST transition precisely enough to build
// both a "safe hour" 7-day sweep and a same-day gap/fold probe. Values were
// obtained by scanning each zone minute-by-minute around the relevant dates
// (not hand-derived), matching what Go's own tzdata reports.
type transitionFixture struct {
	zone string
	// foldDate is the local calendar date on which affectedHour is
	// nonexistent (gap) or repeated (fold).
	foldDate [3]int
	// safeDate is the local calendar date on which safeHour first shows
	// offsetAfter. Equal to foldDate for every fixture except Tehran's
	// fall-back, where the offset change carries local time back across
	// midnight, so the fold happens on the *evening before* the calendar
	// date a plain morning hour is first affected.
	safeDate     [3]int
	safeHour     int    // an hour of day never touched by the gap/fold (e.g. 10:00)
	offsetBefore string // RFC3339 offset before the transition, e.g. "-05:00"
	offsetAfter  string // RFC3339 offset after the transition
	affectedHour int    // the local hour that is nonexistent (gap) or repeated (fold)
	gap          bool   // true for spring-forward (gap), false for fall-back (fold)
	deltaMinutes int    // size of the offset change, in minutes (always positive)
}

// rfc3339Offset formats an RFC3339 numeric UTC offset the way time.Format
// itself does: a literal all-zero offset always prints as "Z", never
// "+00:00", regardless of whether the Location behind it is named "UTC".
func rfc3339Offset(offset string) string {
	if offset == "+00:00" || offset == "-00:00" {
		return "Z"
	}
	return offset
}

var nySpringForward2024 = transitionFixture{
	zone: "America/New_York", foldDate: [3]int{2024, 3, 10}, safeDate: [3]int{2024, 3, 10}, safeHour: 10,
	offsetBefore: "-05:00", offsetAfter: "-04:00", affectedHour: 2, gap: true, deltaMinutes: 60,
}
var nyFallBack2024 = transitionFixture{
	zone: "America/New_York", foldDate: [3]int{2024, 11, 3}, safeDate: [3]int{2024, 11, 3}, safeHour: 10,
	offsetBefore: "-04:00", offsetAfter: "-05:00", affectedHour: 1, gap: false, deltaMinutes: 60,
}
var londonSpringForward2024 = transitionFixture{
	zone: "Europe/London", foldDate: [3]int{2024, 3, 31}, safeDate: [3]int{2024, 3, 31}, safeHour: 10,
	offsetBefore: "+00:00", offsetAfter: "+01:00", affectedHour: 1, gap: true, deltaMinutes: 60,
}
var londonFallBack2024 = transitionFixture{
	zone: "Europe/London", foldDate: [3]int{2024, 10, 27}, safeDate: [3]int{2024, 10, 27}, safeHour: 10,
	offsetBefore: "+01:00", offsetAfter: "+00:00", affectedHour: 1, gap: false, deltaMinutes: 60,
}
var sydneySpringForward2024 = transitionFixture{
	zone: "Australia/Sydney", foldDate: [3]int{2024, 10, 6}, safeDate: [3]int{2024, 10, 6}, safeHour: 10,
	offsetBefore: "+10:00", offsetAfter: "+11:00", affectedHour: 2, gap: true, deltaMinutes: 60,
}
var sydneyFallBack2024 = transitionFixture{
	zone: "Australia/Sydney", foldDate: [3]int{2024, 4, 7}, safeDate: [3]int{2024, 4, 7}, safeHour: 10,
	offsetBefore: "+11:00", offsetAfter: "+10:00", affectedHour: 2, gap: false, deltaMinutes: 60,
}
var tehranSpringForward2015 = transitionFixture{
	zone: "Asia/Tehran", foldDate: [3]int{2015, 3, 22}, safeDate: [3]int{2015, 3, 22}, safeHour: 10,
	offsetBefore: "+03:30", offsetAfter: "+04:30", affectedHour: 0, gap: true, deltaMinutes: 60,
}

// tehranFallBack2015's transition instant is 2015-09-21T19:30:00Z; unlike
// every other fixture here it lands on the local calendar date BEFORE the
// nominal transition date (the fold hour, 23:00-23:59, is on 2015-09-21, not
// 2015-09-22) because the offset change carries local time back across
// midnight. safeHour=10 on the morning of 2015-09-22 already sees the new
// offset.
var tehranFallBack2015 = transitionFixture{
	zone: "Asia/Tehran", foldDate: [3]int{2015, 9, 21}, safeDate: [3]int{2015, 9, 22}, safeHour: 10,
	offsetBefore: "+04:30", offsetAfter: "+03:30", affectedHour: 23, gap: false, deltaMinutes: 60,
}

func allTransitions() []transitionFixture {
	return []transitionFixture{
		nySpringForward2024, nyFallBack2024,
		londonSpringForward2024, londonFallBack2024,
		sydneySpringForward2024, sydneyFallBack2024,
		tehranSpringForward2015, tehranFallBack2015,
	}
}

// TestDailySweepAroundTransitions checks a DAILY rule at a safe (never
// ambiguous) hour for 3 days before through 3 days after each transition in
// each direction, in 4 zones. Every occurrence must show exactly
// safeHour:00:00 local, with offsetBefore on the days up to and including
// the transition's eve and offsetAfter from the transition day on — this is
// the case the ~26h-lookback bug got wrong, since several of these days
// fall within 26h of the transition without being ambiguous at all.
func TestDailySweepAroundTransitions(t *testing.T) {
	for _, tr := range allTransitions() {
		tr := tr
		t.Run(fmt.Sprintf("%s %04d-%02d-%02d", tr.zone, tr.safeDate[0], tr.safeDate[1], tr.safeDate[2]), func(t *testing.T) {
			if _, err := time.LoadLocation(tr.zone); err != nil {
				t.Skipf("%s tzdata not available: %v", tr.zone, err)
			}
			start := ymd{tr.safeDate[0], time.Month(tr.safeDate[1]), tr.safeDate[2]}
			start = addDays(start, -3)
			block := fmt.Sprintf("DTSTART;TZID=%s:%04d%02d%02dT%02d0000\nRRULE:FREQ=DAILY;COUNT=7",
				tr.zone, start.y, start.m, start.d, tr.safeHour)
			r, err := Parse(block)
			if err != nil {
				t.Fatal(err)
			}
			got := r.All(7)
			if len(got) != 7 {
				t.Fatalf("got %d occurrences, want 7: %v", len(got), formatTimes(got))
			}
			transitionDay := ymd{tr.safeDate[0], time.Month(tr.safeDate[1]), tr.safeDate[2]}
			d := start
			for i, occ := range got {
				wantOffset := tr.offsetBefore
				if !d.less(transitionDay) {
					wantOffset = tr.offsetAfter
				}
				wantPrefix := fmt.Sprintf("%04d-%02d-%02dT%02d:00:00", d.y, d.m, d.d, tr.safeHour)
				want := wantPrefix + rfc3339Offset(wantOffset)
				if occ.Format(time.RFC3339) != want {
					t.Errorf("day %d (%04d-%02d-%02d) = %s, want %s", i, d.y, d.m, d.d, occ.Format(time.RFC3339), want)
				}
				d = addDays(d, 1)
			}
		})
	}
}

// TestDailyAtGapOrFoldHour checks a DAILY rule anchored exactly at the
// affected hour of each transition, for the same before/after window, for
// every zone-direction fixture. Only the transition day itself may deviate
// from a plain, unshifted "affectedHour:30:00" at offsetBefore (gap: shifts
// forward by the gap width, at offsetAfter) / offsetAfter (fold: the fold's
// first occurrence, at offsetBefore) — every other day must be completely
// ordinary.
func TestDailyAtGapOrFoldHour(t *testing.T) {
	for _, tr := range allTransitions() {
		tr := tr
		t.Run(fmt.Sprintf("%s %04d-%02d-%02d", tr.zone, tr.foldDate[0], tr.foldDate[1], tr.foldDate[2]), func(t *testing.T) {
			if _, err := time.LoadLocation(tr.zone); err != nil {
				t.Skipf("%s tzdata not available: %v", tr.zone, err)
			}
			anchorDay := ymd{tr.foldDate[0], time.Month(tr.foldDate[1]), tr.foldDate[2]}
			// Anchor the rule on the transition day itself, at
			// :30 past the affected hour, then run it both
			// forward and backward 3 days so the fixture only
			// needs one transition-day definition.
			start := addDays(anchorDay, -3)
			block := fmt.Sprintf("DTSTART;TZID=%s:%04d%02d%02dT%02d3000\nRRULE:FREQ=DAILY;COUNT=7",
				tr.zone, start.y, start.m, start.d, tr.affectedHour)
			r, err := Parse(block)
			if err != nil {
				t.Fatal(err)
			}
			got := r.All(7)
			if len(got) != 7 {
				t.Fatalf("got %d occurrences, want 7: %v", len(got), formatTimes(got))
			}
			d := start
			for i, occ := range got {
				var want string
				switch {
				case d.less(anchorDay):
					want = fmt.Sprintf("%04d-%02d-%02dT%02d:30:00%s", d.y, d.m, d.d, tr.affectedHour, rfc3339Offset(tr.offsetBefore))
				case d == anchorDay && tr.gap:
					// Shift forward by the gap width, at the post-transition offset.
					shifted := time.Date(d.y, d.m, d.d, tr.affectedHour, 30, 0, 0, time.UTC).Add(time.Duration(tr.deltaMinutes) * time.Minute)
					want = fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:00%s", shifted.Year(), shifted.Month(), shifted.Day(), shifted.Hour(), shifted.Minute(), rfc3339Offset(tr.offsetAfter))
				case d == anchorDay && !tr.gap:
					// Fold's first occurrence, at the pre-transition offset.
					want = fmt.Sprintf("%04d-%02d-%02dT%02d:30:00%s", d.y, d.m, d.d, tr.affectedHour, rfc3339Offset(tr.offsetBefore))
				default: // after the transition day
					want = fmt.Sprintf("%04d-%02d-%02dT%02d:30:00%s", d.y, d.m, d.d, tr.affectedHour, rfc3339Offset(tr.offsetAfter))
				}
				if occ.Format(time.RFC3339) != want {
					t.Errorf("day %d (%04d-%02d-%02d) = %s, want %s", i, d.y, d.m, d.d, occ.Format(time.RFC3339), want)
				}
				d = addDays(d, 1)
			}
		})
	}
}

// TestSubDailyEvenSpacingAcrossTransitions checks that HOURLY and MINUTELY
// rules produce strictly increasing, evenly spaced UTC instants (no gap, no
// duplicate) when stepped across every transition fixture, in both
// directions, in every zone. This is the direct regression check for
// timestep.go re-deriving candidates via resolveLocal instead of pure
// Add arithmetic (root cause of the reviewer's Sydney HOURLY report).
func TestSubDailyEvenSpacingAcrossTransitions(t *testing.T) {
	for _, tr := range allTransitions() {
		tr := tr
		for _, freq := range []string{"HOURLY", "MINUTELY"} {
			freq := freq
			t.Run(fmt.Sprintf("%s %s %04d-%02d-%02d", freq, tr.zone, tr.foldDate[0], tr.foldDate[1], tr.foldDate[2]), func(t *testing.T) {
				if _, err := time.LoadLocation(tr.zone); err != nil {
					t.Skipf("%s tzdata not available: %v", tr.zone, err)
				}
				start := addDays(ymd{tr.foldDate[0], time.Month(tr.foldDate[1]), tr.foldDate[2]}, -1)
				var step time.Duration
				var count int
				switch freq {
				case "HOURLY":
					step, count = time.Hour, 48
				case "MINUTELY":
					step, count = time.Minute, 240
				}
				block := fmt.Sprintf("DTSTART;TZID=%s:%04d%02d%02dT000000\nRRULE:FREQ=%s;COUNT=%d",
					tr.zone, start.y, start.m, start.d, freq, count)
				r, err := Parse(block)
				if err != nil {
					t.Fatal(err)
				}
				got := r.All(count)
				if len(got) != count {
					t.Fatalf("got %d occurrences, want %d", len(got), count)
				}
				for i := 1; i < len(got); i++ {
					diff := got[i].Sub(got[i-1])
					if diff != step {
						t.Errorf("gap/duplicate between occurrence %d (%s) and %d (%s): delta = %s, want %s",
							i-1, got[i-1].Format(time.RFC3339), i, got[i].Format(time.RFC3339), diff, step)
					}
				}
			})
		}
	}
}
