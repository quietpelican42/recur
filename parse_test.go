package recur

import (
	"errors"
	"testing"
	"time"
)

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"no DTSTART", "RRULE:FREQ=DAILY;COUNT=5"},
		{"no FREQ", "DTSTART:19970902T090000Z\nRRULE:COUNT=5"},
		{"COUNT and UNTIL", "DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;COUNT=5;UNTIL=19980101T000000Z"},
		{"bad FREQ", "DTSTART:19970902T090000Z\nRRULE:FREQ=FORTNIGHTLY;COUNT=5"},
		{"BYWEEKNO with MONTHLY", "DTSTART:19970902T090000Z\nRRULE:FREQ=MONTHLY;BYWEEKNO=3;COUNT=5"},
		{"BYYEARDAY with DAILY", "DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;BYYEARDAY=100;COUNT=5"},
		{"BYMONTHDAY with WEEKLY", "DTSTART:19970902T090000Z\nRRULE:FREQ=WEEKLY;BYMONTHDAY=5;COUNT=5"},
		{"ordinal BYDAY with WEEKLY", "DTSTART:19970902T090000Z\nRRULE:FREQ=WEEKLY;BYDAY=1MO;COUNT=5"},
		{"BYSETPOS alone", "DTSTART:19970902T090000Z\nRRULE:FREQ=MONTHLY;BYSETPOS=1;COUNT=5"},
		{"bad TZID", "DTSTART;TZID=Nowhere/Imaginary:19970902T090000\nRRULE:FREQ=DAILY;COUNT=5"},
		{"unknown rule part", "DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;BYFOO=1;COUNT=5"},
		{"BYMONTH out of range", "DTSTART:19970902T090000Z\nRRULE:FREQ=YEARLY;BYMONTH=13;COUNT=5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.in)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want one", c.in)
			}
			if !errors.Is(err, ErrInvalidRule) && !errors.Is(err, ErrUnsupported) {
				t.Errorf("Parse(%q) error %v is neither ErrInvalidRule nor ErrUnsupported", c.in, err)
			}
		})
	}
}

func TestParseUTCFloatingTZID(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("America/New_York tzdata not available: %v", err)
	}

	t.Run("UTC DTSTART", func(t *testing.T) {
		r, err := Parse("DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;COUNT=1")
		if err != nil {
			t.Fatal(err)
		}
		if r.DTStartFloating {
			t.Error("UTC DTSTART should not be floating")
		}
		if got := r.DTStart.Format(time.RFC3339); got != "1997-09-02T09:00:00Z" {
			t.Errorf("DTStart = %s, want 1997-09-02T09:00:00Z", got)
		}
	})

	t.Run("floating DTSTART", func(t *testing.T) {
		r, err := Parse("DTSTART:19970902T090000\nRRULE:FREQ=DAILY;COUNT=1")
		if err != nil {
			t.Fatal(err)
		}
		if !r.DTStartFloating {
			t.Error("DTSTART with no Z and no TZID should be floating")
		}
	})

	t.Run("TZID DTSTART", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=DAILY;COUNT=1")
		if err != nil {
			t.Fatal(err)
		}
		if r.DTStartFloating {
			t.Error("TZID DTSTART should not be floating")
		}
		want := resolveLocal(1997, 9, 2, 9, 0, 0, ny)
		if !r.DTStart.Equal(want) {
			t.Errorf("DTStart = %s, want %s", r.DTStart, want)
		}
	})
}

func TestStringRoundTrip(t *testing.T) {
	blocks := []string{
		"DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=DAILY;COUNT=10",
		"DTSTART:19970902T090000Z\nRRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE,FR;WKST=SU;COUNT=6",
		"DTSTART;TZID=America/New_York:19970929T090000\nRRULE:FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-2",
		"DTSTART:19970902T090000Z\nRRULE:FREQ=YEARLY;UNTIL=20000131T140000Z;BYMONTH=1;BYDAY=SU,MO,TU,WE,TH,FR,SA",
	}
	for _, b := range blocks {
		t.Run(b, func(t *testing.T) {
			r1, err := Parse(b)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			s := r1.String()
			r2, err := Parse(s)
			if err != nil {
				t.Fatalf("Parse(String()) = %v; String() was %q", err, s)
			}
			want := r1.All(20)
			got := r2.All(20)
			if len(want) != len(got) {
				t.Fatalf("round trip changed occurrence count: %d vs %d (String()=%q)", len(want), len(got), s)
			}
			for i := range want {
				if !want[i].Equal(got[i]) {
					t.Errorf("occurrence %d: %s vs %s (String()=%q)", i, want[i], got[i], s)
				}
			}
		})
	}
}

func TestRDateExDate(t *testing.T) {
	block := "DTSTART:19970902T090000Z\n" +
		"RRULE:FREQ=DAILY;COUNT=3\n" +
		"RDATE:19971225T090000Z\n" +
		"EXDATE:19970903T090000Z"
	r, err := Parse(block)
	if err != nil {
		t.Fatal(err)
	}
	got := r.All(10)
	want := []string{"1997-09-02T09:00:00Z", "1997-09-04T09:00:00Z", "1997-12-25T09:00:00Z"}
	if len(got) != len(want) {
		t.Fatalf("got %d occurrences %v, want %d", len(got), formatTimes(got), len(want))
	}
	for i, w := range want {
		if got[i].Format(time.RFC3339) != w {
			t.Errorf("occurrence %d = %s, want %s", i, got[i].Format(time.RFC3339), w)
		}
	}
}

func TestNoRRuleJustRDate(t *testing.T) {
	r, err := Parse("DTSTART:19970902T090000Z\nRDATE:19971001T090000Z,19971101T090000Z")
	if err != nil {
		t.Fatal(err)
	}
	if r.HasRRule {
		t.Fatal("HasRRule should be false")
	}
	got := r.All(10)
	want := []string{"1997-09-02T09:00:00Z", "1997-10-01T09:00:00Z", "1997-11-01T09:00:00Z"}
	if len(got) != len(want) {
		t.Fatalf("got %d, want %d: %v", len(got), len(want), formatTimes(got))
	}
	for i, w := range want {
		if got[i].Format(time.RFC3339) != w {
			t.Errorf("occurrence %d = %s, want %s", i, got[i].Format(time.RFC3339), w)
		}
	}
}

func TestBetweenAfterBefore(t *testing.T) {
	r, err := Parse("DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;COUNT=10")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(1997, 9, 4, 0, 0, 0, 0, time.UTC)
	to := time.Date(1997, 9, 7, 0, 0, 0, 0, time.UTC)
	between := r.Between(from, to)
	wantBetween := []string{"1997-09-04T09:00:00Z", "1997-09-05T09:00:00Z", "1997-09-06T09:00:00Z"}
	if len(between) != len(wantBetween) {
		t.Fatalf("Between: got %d, want %d: %v", len(between), len(wantBetween), formatTimes(between))
	}
	for i, w := range wantBetween {
		if between[i].Format(time.RFC3339) != w {
			t.Errorf("Between[%d] = %s, want %s", i, between[i].Format(time.RFC3339), w)
		}
	}

	after, ok := r.After(time.Date(1997, 9, 5, 9, 0, 0, 0, time.UTC), false)
	if !ok || after.Format(time.RFC3339) != "1997-09-06T09:00:00Z" {
		t.Errorf("After = %v, %v, want 1997-09-06T09:00:00Z, true", after, ok)
	}
	afterInc, ok := r.After(time.Date(1997, 9, 5, 9, 0, 0, 0, time.UTC), true)
	if !ok || afterInc.Format(time.RFC3339) != "1997-09-05T09:00:00Z" {
		t.Errorf("After(inclusive) = %v, %v, want 1997-09-05T09:00:00Z, true", afterInc, ok)
	}

	before, ok := r.Before(time.Date(1997, 9, 5, 9, 0, 0, 0, time.UTC), false)
	if !ok || before.Format(time.RFC3339) != "1997-09-04T09:00:00Z" {
		t.Errorf("Before = %v, %v, want 1997-09-04T09:00:00Z, true", before, ok)
	}
	beforeInc, ok := r.Before(time.Date(1997, 9, 5, 9, 0, 0, 0, time.UTC), true)
	if !ok || beforeInc.Format(time.RFC3339) != "1997-09-05T09:00:00Z" {
		t.Errorf("Before(inclusive) = %v, %v, want 1997-09-05T09:00:00Z, true", beforeInc, ok)
	}

	// After the last occurrence: nothing.
	if _, ok := r.After(time.Date(1998, 1, 1, 0, 0, 0, 0, time.UTC), false); ok {
		t.Error("After past the end of a COUNT-bounded rule should return false")
	}
}

func TestIterEarlyBreak(t *testing.T) {
	r, err := Parse("DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for range r.Iter(time.Time{}, time.Time{}) {
		n++
		if n == 5 {
			break
		}
	}
	if n != 5 {
		t.Errorf("expected to be able to break after 5 occurrences of an unbounded rule, got %d", n)
	}
}

func TestImpossibleRuleTerminates(t *testing.T) {
	// February 30th never exists; generation must give up rather than
	// loop forever, per iter.go's maxConsecutiveEmptyPeriods.
	r, err := Parse("DTSTART:19970902T090000Z\nRRULE:FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=30")
	if err != nil {
		t.Fatal(err)
	}
	got := r.All(5)
	if len(got) != 0 {
		t.Errorf("got %d occurrences of an impossible rule, want 0", len(got))
	}
}

func TestUnsupportedNeverProducesWrongDates(t *testing.T) {
	// BYSETPOS requires another BYxxx rule part per RFC 5545 §3.3.10;
	// this must be rejected at Parse time, not silently ignored.
	_, err := Parse("DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;BYSETPOS=2;COUNT=5")
	if !errors.Is(err, ErrInvalidRule) {
		t.Errorf("Parse = %v, want ErrInvalidRule", err)
	}
}
