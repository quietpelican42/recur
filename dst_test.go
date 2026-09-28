package recur

import (
	"testing"
	"time"
)

// TestResolveLocalCheck hand-derives every expected instant from RFC 5545
// §3.3.5's rule ("a nonexistent local time is interpreted using the UTC
// offset before the gap; an ambiguous local time refers to its first
// occurrence") rather than from this package's own output, across five
// zones with different transition shapes: a one-hour spring-forward and
// fall-back (America/New_York, the RFC's own example zone), a Southern
// Hemisphere spring-forward with the opposite sign (Australia/Sydney), a UK
// one-hour gap (Europe/London), a historical non-integer-hour transition
// (Asia/Tehran, +03:30 to +04:30 in 2015 — Iran still observed DST that
// year; it stopped in 2022), and a whole calendar day skipped entirely
// (Pacific/Apia, which jumped from UTC-11 to UTC+14 on 2011-12-30 to move
// to the other side of the International Date Line).
func TestResolveLocalCheck(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	syd, _ := time.LoadLocation("Australia/Sydney")
	apia, _ := time.LoadLocation("Pacific/Apia")
	london, _ := time.LoadLocation("Europe/London")
	tehran, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		t.Skipf("Asia/Tehran tzdata not available: %v", err)
	}

	cases := []struct {
		name        string
		y           int
		mo          time.Month
		d, h, mi, s int
		loc         *time.Location
		wantRFC3339 string
	}{
		{"NY gap RFC example", 2007, 3, 11, 2, 30, 0, ny, "2007-03-11T03:30:00-04:00"},
		{"NY gap verified premise", 2024, 3, 10, 2, 30, 0, ny, "2024-03-10T03:30:00-04:00"},
		{"NY fold RFC example first occurrence", 2007, 11, 4, 1, 30, 0, ny, "2007-11-04T01:30:00-04:00"},
		{"NY ordinary time unaffected", 2024, 6, 15, 14, 0, 0, ny, "2024-06-15T14:00:00-04:00"},
		{"Sydney gap", 2024, 10, 6, 2, 30, 0, syd, "2024-10-06T03:30:00+11:00"},
		{"Sydney ordinary", 2024, 10, 6, 0, 0, 0, syd, "2024-10-06T00:00:00+10:00"},
		{"London gap", 2024, 3, 31, 1, 30, 0, london, "2024-03-31T02:30:00+01:00"},
		{"London fold first occurrence", 2024, 10, 27, 1, 30, 0, london, "2024-10-27T01:30:00+01:00"},
		// Tehran jumped from 00:00 +03:30 straight to 01:00 +04:30 on
		// 2015-03-22, so 00:00-00:59 local do not exist that day.
		// Interpreting 00:30 with the pre-gap offset (+03:30) gives
		// UTC instant 2015-03-21T21:00:00Z, which is 01:30 +04:30
		// local — one hour after 00:30, the width of the gap, exactly
		// matching "one hour after 1:30 A.M. EST" in the RFC's own NY
		// example.
		{"Tehran gap, offset changes by a full hour (+03:30 to +04:30)", 2015, 3, 22, 0, 30, 0, tehran, "2015-03-22T01:30:00+04:30"},
		{"Apia skipped day", 2011, 12, 30, 12, 0, 0, apia, "2011-12-31T12:00:00+14:00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveLocal(c.y, c.mo, c.d, c.h, c.mi, c.s, c.loc)
			if got.Format(time.RFC3339) != c.wantRFC3339 {
				t.Errorf("resolveLocal(%d-%02d-%02d %02d:%02d:%02d, %v) = %s, want %s",
					c.y, c.mo, c.d, c.h, c.mi, c.s, c.loc, got.Format(time.RFC3339), c.wantRFC3339)
			}
		})
	}
}

// checkRFC3339Sequence asserts got matches want exactly, formatting both as
// RFC3339 for readable diffs. want is always written as independent literal
// strings, deliberately never computed by calling resolveLocal or any other
// function under test — a expectation built the same way as the code being
// checked cannot catch a shared bug. This is not a hypothetical: an earlier
// version of TestVerifiedPremiseReproductions built its "want" slice with
// resolveLocal itself, so it kept passing across a resolveLocal regression
// that a reviewer's independent probe caught (see below).
func checkRFC3339Sequence(t *testing.T, got []time.Time, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d occurrences, want %d\n got:  %v\n want: %v", len(got), len(want), formatTimes(got), want)
	}
	for i, w := range want {
		if got[i].Format(time.RFC3339) != w {
			t.Errorf("occurrence %d = %s, want %s", i, got[i].Format(time.RFC3339), w)
		}
	}
}

// TestVerifiedPremiseReproductions re-runs, through the public Rule API,
// the failure modes this package was written to fix (confirmed against
// teambition/rrule-go v1.8.2 before writing any code here), plus the two
// regressions a reviewer's independent probe found in the first
// implementation of resolveLocal (a fixed ~26h lookback window, which wrongly
// re-applied the pre-transition offset to any candidate merely close in time
// to some transition, whether or not that candidate was itself ambiguous):
//
//  1. DAILY, DTSTART 2024-03-08 02:30 America/New_York, COUNT=4: rrule-go
//     emits 2024-03-10 01:30 EST (wall time moving backward through the
//     spring-forward gap) instead of the RFC 5545 §3.3.5-correct 03:30 EDT.
//     Regression: the naive lookback fix also wrongly shifted 2024-03-11
//     02:30 — an ordinary, unambiguous EDT time the day after the
//     transition — to 03:30 EDT.
//  2. DAILY, DTSTART 2024-11-02 01:30 America/New_York, COUNT=3, crossing
//     the fall-back: 11-03 01:30 must be the fold's first (EDT) occurrence,
//     and 11-04 01:30 — the day after the fold, ordinary and unambiguous —
//     must come back to plain 01:30 EST, not drift to 00:30 EST.
//  3. HOURLY, DTSTART 2024-10-06 00:00 Australia/Sydney, COUNT=5/6: rrule-go
//     emits 03:00 AEDT twice (the same instant counted as two occurrences)
//     across the spring-forward gap (teambition/rrule-go issue #63).
//     Regression: an HOURLY path that re-derives each candidate from wall-
//     clock components via resolveLocal (instead of pure Add arithmetic)
//     can skip 03:00 AEDT entirely rather than duplicate it.
func TestVerifiedPremiseReproductions(t *testing.T) {
	if _, err := time.LoadLocation("America/New_York"); err != nil {
		t.Skipf("America/New_York tzdata not available: %v", err)
	}
	if _, err := time.LoadLocation("Australia/Sydney"); err != nil {
		t.Skipf("Australia/Sydney tzdata not available: %v", err)
	}

	t.Run("DAILY across NY spring-forward, day after is not re-shifted", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:20240308T023000\nRRULE:FREQ=DAILY;COUNT=4")
		if err != nil {
			t.Fatal(err)
		}
		checkRFC3339Sequence(t, r.All(4), []string{
			"2024-03-08T02:30:00-05:00",
			"2024-03-09T02:30:00-05:00",
			"2024-03-10T03:30:00-04:00", // nonexistent -> shifted past the gap
			"2024-03-11T02:30:00-04:00", // ordinary EDT day after: NOT 03:30
		})
	})

	t.Run("DAILY across NY fall-back, day after is not drifted", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:20241102T013000\nRRULE:FREQ=DAILY;COUNT=3")
		if err != nil {
			t.Fatal(err)
		}
		checkRFC3339Sequence(t, r.All(3), []string{
			"2024-11-02T01:30:00-04:00",
			"2024-11-03T01:30:00-04:00", // ambiguous -> first (EDT) occurrence
			"2024-11-04T01:30:00-05:00", // ordinary EST day after: NOT 00:30
		})
	})

	t.Run("HOURLY across Sydney spring-forward, no duplicate and no skip", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=Australia/Sydney:20241006T000000\nRRULE:FREQ=HOURLY;COUNT=6")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(6)
		checkRFC3339Sequence(t, got, []string{
			"2024-10-06T00:00:00+10:00",
			"2024-10-06T01:00:00+10:00",
			// 02:00-02:59 AEST do not exist; the absolute-duration
			// step from 01:00 lands exactly on 03:00 AEDT once, not
			// twice, and 03:00 AEDT must actually appear (a
			// resolveLocal-based HOURLY path regressed this to
			// skipping straight to 04:00).
			"2024-10-06T03:00:00+11:00",
			"2024-10-06T04:00:00+11:00",
			"2024-10-06T05:00:00+11:00",
			"2024-10-06T06:00:00+11:00",
		})
		seen := make(map[time.Time]bool)
		for _, g := range got {
			if seen[g] {
				t.Errorf("duplicate occurrence %s", g.Format(time.RFC3339))
			}
			seen[g] = true
		}
	})
}

// TestApiaSkippedDayRule exercises Pacific/Apia's 2011-12-30 skip (UTC-11 to
// UTC+14, moving Samoa to the other side of the date line) through a DAILY
// rule rather than a single resolveLocal call: the day simply does not
// appear as an occurrence, since RFC 5545 §3.3.10 says an instance with no
// valid local time is skipped, and no other day is affected.
func TestApiaSkippedDayRule(t *testing.T) {
	if _, err := time.LoadLocation("Pacific/Apia"); err != nil {
		t.Skipf("Pacific/Apia tzdata not available: %v", err)
	}
	r, err := Parse("DTSTART;TZID=Pacific/Apia:20111228T120000\nRRULE:FREQ=DAILY;COUNT=4")
	if err != nil {
		t.Fatal(err)
	}
	got := r.All(4)
	wantDays := []string{"2011-12-28", "2011-12-29", "2011-12-31", "2012-01-01"}
	if len(got) != len(wantDays) {
		t.Fatalf("got %d occurrences, want %d: %v", len(got), len(wantDays), got)
	}
	for i, w := range wantDays {
		if got[i].Format("2006-01-02") != w {
			t.Errorf("occurrence %d = %s, want date %s", i, got[i].Format(time.RFC3339), w)
		}
	}
}
