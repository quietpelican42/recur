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
		// local — 30 minutes after the (60-minute) gap, matching
		// "one hour after 1:30 A.M. EST" in the RFC's own NY example.
		{"Tehran gap, 30-minute offset step", 2015, 3, 22, 0, 30, 0, tehran, "2015-03-22T01:30:00+04:30"},
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

// TestVerifiedPremiseReproductions re-runs, through the public Rule API,
// the two failure modes this package was written to fix (confirmed against
// teambition/rrule-go v1.8.2 before writing any code here):
//
//  1. DAILY, DTSTART 2024-03-08 02:30 America/New_York, COUNT=4: rrule-go
//     emits 2024-03-10 01:30 EST (wall time moving backward through the
//     spring-forward gap). RFC 5545 §3.3.5 requires 03:30 EDT.
//  2. HOURLY, DTSTART 2024-10-06 00:00 Australia/Sydney: rrule-go emits
//     03:00 AEDT twice (the same instant counted as two occurrences) across
//     the spring-forward gap (teambition/rrule-go issue #63).
func TestVerifiedPremiseReproductions(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	syd, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Skipf("Australia/Sydney tzdata not available: %v", err)
	}

	t.Run("DAILY across NY spring-forward", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:20240308T023000\nRRULE:FREQ=DAILY;COUNT=4")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(4)
		want := []time.Time{
			resolveLocal(2024, 3, 8, 2, 30, 0, ny),
			resolveLocal(2024, 3, 9, 2, 30, 0, ny),
			resolveLocal(2024, 3, 10, 2, 30, 0, ny), // nonexistent -> shifted to 03:30 EDT
			resolveLocal(2024, 3, 11, 2, 30, 0, ny),
		}
		if len(got) != len(want) {
			t.Fatalf("got %d occurrences, want %d: %v", len(got), len(want), got)
		}
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i].Format(time.RFC3339), want[i].Format(time.RFC3339))
			}
		}
		if got[2].Format(time.RFC3339) != "2024-03-10T03:30:00-04:00" {
			t.Errorf("occurrence 2 = %s, want 2024-03-10T03:30:00-04:00 (the RFC-correct shift, not rrule-go's 01:30 EST)", got[2].Format(time.RFC3339))
		}
	})

	t.Run("HOURLY across Sydney spring-forward, no duplicate", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=Australia/Sydney:20241006T000000\nRRULE:FREQ=HOURLY;COUNT=6")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(6)
		want := []time.Time{
			resolveLocal(2024, 10, 6, 0, 0, 0, syd),
			resolveLocal(2024, 10, 6, 1, 0, 0, syd),
			// 02:00-02:59 AEST do not exist; the absolute-duration
			// step from 01:00 lands exactly on 03:00 AEDT once,
			// not twice.
			resolveLocal(2024, 10, 6, 3, 0, 0, syd),
			resolveLocal(2024, 10, 6, 4, 0, 0, syd),
			resolveLocal(2024, 10, 6, 5, 0, 0, syd),
			resolveLocal(2024, 10, 6, 6, 0, 0, syd),
		}
		if len(got) != len(want) {
			t.Fatalf("got %d occurrences, want %d: %v", len(got), len(want), got)
		}
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i].Format(time.RFC3339), want[i].Format(time.RFC3339))
			}
		}
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
