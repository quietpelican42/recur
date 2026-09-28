package recur

import (
	"testing"
	"time"
)

// Every example transcribed here is copied verbatim (DTSTART, RRULE, and
// the expected occurrence dates) from RFC 5545 §3.8.5.3, "All examples
// assume the Eastern United States time zone." Where the RFC gives a
// contiguous date range in prose ("September 2-11") the test expands it
// programmatically with mkRange rather than by hand, to avoid transcription
// slips over what is otherwise ~150 individual dates; the sparser examples
// are listed explicitly, matching the RFC text date for date.

func nyLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("America/New_York tzdata not available: %v", err)
	}
	return loc
}

func mk(loc *time.Location, y int, m time.Month, d, h, mi, s int) time.Time {
	return resolveLocal(y, m, d, h, mi, s, loc)
}

// mkRange returns every day from (y1,m1,d1) to (y2,m2,d2) inclusive, at the
// given time of day.
func mkRange(loc *time.Location, y1 int, m1 time.Month, d1 int, y2 int, m2 time.Month, d2 int, h, mi, s int) []time.Time {
	var out []time.Time
	cur := ymd{y1, m1, d1}
	end := ymd{y2, m2, d2}
	for {
		out = append(out, mk(loc, cur.y, cur.m, cur.d, h, mi, s))
		if cur == end {
			break
		}
		cur = addDays(cur, 1)
	}
	return out
}

// mkDates returns one time.Time per (y,m,d) triple in days, all at the same
// time of day.
func mkDates(loc *time.Location, h, mi, s int, days [][3]int) []time.Time {
	out := make([]time.Time, len(days))
	for i, d := range days {
		out[i] = mk(loc, d[0], time.Month(d[1]), d[2], h, mi, s)
	}
	return out
}

func checkRule(t *testing.T, block string, want []time.Time) {
	t.Helper()
	r, err := Parse(block)
	if err != nil {
		t.Fatalf("Parse(%q): %v", block, err)
	}
	var got []time.Time
	if r.Count > 0 {
		got = r.All(r.Count + 5)
	} else {
		got = r.All(len(want) + 5)
	}
	if len(got) != len(want) {
		t.Fatalf("%s\n got %d occurrences, want %d\n got:  %v\n want: %v", block, len(got), len(want), formatTimes(got), formatTimes(want))
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("%s\n occurrence %d = %s, want %s", block, i, got[i].Format(time.RFC3339), want[i].Format(time.RFC3339))
		}
	}
}

func formatTimes(ts []time.Time) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Format(time.RFC3339)
	}
	return out
}

func TestRFC5545Examples(t *testing.T) {
	loc := nyLoc(t)

	t.Run("Daily for 10 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=DAILY;COUNT=10",
			mkRange(loc, 1997, 9, 2, 1997, 9, 11, 9, 0, 0))
	})

	t.Run("Daily until December 24 1997", func(t *testing.T) {
		var want []time.Time
		want = append(want, mkRange(loc, 1997, 9, 2, 1997, 12, 23, 9, 0, 0)...)
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=DAILY;UNTIL=19971224T000000Z", want)
	})

	t.Run("Every other day forever (first 10)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=DAILY;INTERVAL=2")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(5)
		want := mkDates(loc, 9, 0, 0, [][3]int{{1997, 9, 2}, {1997, 9, 4}, {1997, 9, 6}, {1997, 9, 8}, {1997, 9, 10}})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Every 10 days 5 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=DAILY;INTERVAL=10;COUNT=5",
			mkDates(loc, 9, 0, 0, [][3]int{{1997, 9, 2}, {1997, 9, 12}, {1997, 9, 22}, {1997, 10, 2}, {1997, 10, 12}}))
	})

	t.Run("Every day in January for 3 years (BYMONTH+BYDAY form)", func(t *testing.T) {
		var want []time.Time
		want = append(want, mkRange(loc, 1998, 1, 1, 1998, 1, 31, 9, 0, 0)...)
		want = append(want, mkRange(loc, 1999, 1, 1, 1999, 1, 31, 9, 0, 0)...)
		want = append(want, mkRange(loc, 2000, 1, 1, 2000, 1, 31, 9, 0, 0)...)
		checkRule(t, "DTSTART;TZID=America/New_York:19980101T090000\nRRULE:FREQ=YEARLY;UNTIL=20000131T140000Z;BYMONTH=1;BYDAY=SU,MO,TU,WE,TH,FR,SA", want)
	})

	t.Run("Every day in January for 3 years (BYMONTH+DAILY form)", func(t *testing.T) {
		var want []time.Time
		want = append(want, mkRange(loc, 1998, 1, 1, 1998, 1, 31, 9, 0, 0)...)
		want = append(want, mkRange(loc, 1999, 1, 1, 1999, 1, 31, 9, 0, 0)...)
		want = append(want, mkRange(loc, 2000, 1, 1, 2000, 1, 31, 9, 0, 0)...)
		checkRule(t, "DTSTART;TZID=America/New_York:19980101T090000\nRRULE:FREQ=DAILY;UNTIL=20000131T140000Z;BYMONTH=1", want)
	})

	t.Run("Weekly for 10 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=WEEKLY;COUNT=10",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 2}, {1997, 9, 9}, {1997, 9, 16}, {1997, 9, 23}, {1997, 9, 30},
				{1997, 10, 7}, {1997, 10, 14}, {1997, 10, 21}, {1997, 10, 28}, {1997, 11, 4},
			}))
	})

	t.Run("Weekly until December 24 1997", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=WEEKLY;UNTIL=19971224T000000Z",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 2}, {1997, 9, 9}, {1997, 9, 16}, {1997, 9, 23}, {1997, 9, 30},
				{1997, 10, 7}, {1997, 10, 14}, {1997, 10, 21},
				{1997, 10, 28}, {1997, 11, 4}, {1997, 11, 11}, {1997, 11, 18}, {1997, 11, 25},
				{1997, 12, 2}, {1997, 12, 9}, {1997, 12, 16}, {1997, 12, 23},
			}))
	})

	t.Run("Every other week forever (first 13)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=WEEKLY;INTERVAL=2;WKST=SU")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(13)
		want := mkDates(loc, 9, 0, 0, [][3]int{
			{1997, 9, 2}, {1997, 9, 16}, {1997, 9, 30}, {1997, 10, 14},
			{1997, 10, 28}, {1997, 11, 11}, {1997, 11, 25}, {1997, 12, 9}, {1997, 12, 23},
			{1998, 1, 6}, {1998, 1, 20}, {1998, 2, 3}, {1998, 2, 17},
		})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Weekly on Tuesday and Thursday for five weeks (UNTIL form)", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=WEEKLY;UNTIL=19971007T000000Z;WKST=SU;BYDAY=TU,TH",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 2}, {1997, 9, 4}, {1997, 9, 9}, {1997, 9, 11}, {1997, 9, 16}, {1997, 9, 18},
				{1997, 9, 23}, {1997, 9, 25}, {1997, 9, 30}, {1997, 10, 2},
			}))
	})

	t.Run("Weekly on Tuesday and Thursday for five weeks (COUNT form)", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=WEEKLY;COUNT=10;WKST=SU;BYDAY=TU,TH",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 2}, {1997, 9, 4}, {1997, 9, 9}, {1997, 9, 11}, {1997, 9, 16}, {1997, 9, 18},
				{1997, 9, 23}, {1997, 9, 25}, {1997, 9, 30}, {1997, 10, 2},
			}))
	})

	t.Run("Every other week on MO WE FR until Dec 24 1997 starting Sep 1", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970901T090000\nRRULE:FREQ=WEEKLY;INTERVAL=2;UNTIL=19971224T000000Z;WKST=SU;BYDAY=MO,WE,FR",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 1}, {1997, 9, 3}, {1997, 9, 5}, {1997, 9, 15}, {1997, 9, 17}, {1997, 9, 19}, {1997, 9, 29},
				{1997, 10, 1}, {1997, 10, 3}, {1997, 10, 13}, {1997, 10, 15}, {1997, 10, 17},
				{1997, 10, 27}, {1997, 10, 29}, {1997, 10, 31},
				{1997, 11, 10}, {1997, 11, 12}, {1997, 11, 14}, {1997, 11, 24}, {1997, 11, 26}, {1997, 11, 28},
				{1997, 12, 8}, {1997, 12, 10}, {1997, 12, 12}, {1997, 12, 22},
			}))
	})

	t.Run("Every other week on Tuesday and Thursday for 8 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=WEEKLY;INTERVAL=2;COUNT=8;WKST=SU;BYDAY=TU,TH",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 2}, {1997, 9, 4}, {1997, 9, 16}, {1997, 9, 18}, {1997, 9, 30},
				{1997, 10, 2}, {1997, 10, 14}, {1997, 10, 16},
			}))
	})

	t.Run("Monthly on the first Friday for 10 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970905T090000\nRRULE:FREQ=MONTHLY;COUNT=10;BYDAY=1FR",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 5}, {1997, 10, 3}, {1997, 11, 7}, {1997, 12, 5},
				{1998, 1, 2}, {1998, 2, 6}, {1998, 3, 6}, {1998, 4, 3}, {1998, 5, 1}, {1998, 6, 5},
			}))
	})

	t.Run("Monthly on the first Friday until December 24 1997", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970905T090000\nRRULE:FREQ=MONTHLY;UNTIL=19971224T000000Z;BYDAY=1FR",
			mkDates(loc, 9, 0, 0, [][3]int{{1997, 9, 5}, {1997, 10, 3}, {1997, 11, 7}, {1997, 12, 5}}))
	})

	t.Run("Every other month first and last Sunday for 10 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970907T090000\nRRULE:FREQ=MONTHLY;INTERVAL=2;COUNT=10;BYDAY=1SU,-1SU",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 7}, {1997, 9, 28}, {1997, 11, 2}, {1997, 11, 30},
				{1998, 1, 4}, {1998, 1, 25}, {1998, 3, 1}, {1998, 3, 29}, {1998, 5, 3}, {1998, 5, 31},
			}))
	})

	t.Run("Monthly second-to-last Monday for 6 months", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970922T090000\nRRULE:FREQ=MONTHLY;COUNT=6;BYDAY=-2MO",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 22}, {1997, 10, 20}, {1997, 11, 17}, {1997, 12, 22}, {1998, 1, 19}, {1998, 2, 16},
			}))
	})

	t.Run("Monthly third-to-last day forever (first 6)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970928T090000\nRRULE:FREQ=MONTHLY;BYMONTHDAY=-3")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(6)
		want := mkDates(loc, 9, 0, 0, [][3]int{
			{1997, 9, 28}, {1997, 10, 29}, {1997, 11, 28}, {1997, 12, 29}, {1998, 1, 29}, {1998, 2, 26},
		})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Monthly on the 2nd and 15th for 10 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=MONTHLY;COUNT=10;BYMONTHDAY=2,15",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 2}, {1997, 9, 15}, {1997, 10, 2}, {1997, 10, 15}, {1997, 11, 2}, {1997, 11, 15},
				{1997, 12, 2}, {1997, 12, 15}, {1998, 1, 2}, {1998, 1, 15},
			}))
	})

	t.Run("Monthly on the first and last day for 10 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970930T090000\nRRULE:FREQ=MONTHLY;COUNT=10;BYMONTHDAY=1,-1",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 30}, {1997, 10, 1}, {1997, 10, 31}, {1997, 11, 1}, {1997, 11, 30}, {1997, 12, 1},
				{1997, 12, 31}, {1998, 1, 1}, {1998, 1, 31}, {1998, 2, 1},
			}))
	})

	t.Run("Every 18 months on 10th thru 15th for 10 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970910T090000\nRRULE:FREQ=MONTHLY;INTERVAL=18;COUNT=10;BYMONTHDAY=10,11,12,13,14,15",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 9, 10}, {1997, 9, 11}, {1997, 9, 12}, {1997, 9, 13}, {1997, 9, 14}, {1997, 9, 15},
				{1999, 3, 10}, {1999, 3, 11}, {1999, 3, 12}, {1999, 3, 13},
			}))
	})

	t.Run("Every Tuesday every other month (first 10)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=MONTHLY;INTERVAL=2;BYDAY=TU")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(10)
		want := mkDates(loc, 9, 0, 0, [][3]int{
			{1997, 9, 2}, {1997, 9, 9}, {1997, 9, 16}, {1997, 9, 23}, {1997, 9, 30},
			{1997, 11, 4}, {1997, 11, 11}, {1997, 11, 18}, {1997, 11, 25},
			{1998, 1, 6},
		})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Yearly in June and July for 10 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970610T090000\nRRULE:FREQ=YEARLY;COUNT=10;BYMONTH=6,7",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 6, 10}, {1997, 7, 10}, {1998, 6, 10}, {1998, 7, 10}, {1999, 6, 10},
				{1999, 7, 10}, {2000, 6, 10}, {2000, 7, 10}, {2001, 6, 10}, {2001, 7, 10},
			}))
	})

	t.Run("Every other year Jan Feb Mar for 10 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970310T090000\nRRULE:FREQ=YEARLY;INTERVAL=2;COUNT=10;BYMONTH=1,2,3",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 3, 10},
				{1999, 1, 10}, {1999, 2, 10}, {1999, 3, 10},
				{2001, 1, 10}, {2001, 2, 10}, {2001, 3, 10},
				{2003, 1, 10}, {2003, 2, 10}, {2003, 3, 10},
			}))
	})

	t.Run("Every third year on the 1st 100th 200th day for 10 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970101T090000\nRRULE:FREQ=YEARLY;INTERVAL=3;COUNT=10;BYYEARDAY=1,100,200",
			mkDates(loc, 9, 0, 0, [][3]int{
				{1997, 1, 1}, {1997, 4, 10}, {1997, 7, 19},
				{2000, 1, 1}, {2000, 4, 9}, {2000, 7, 18},
				{2003, 1, 1}, {2003, 4, 10}, {2003, 7, 19},
				{2006, 1, 1},
			}))
	})

	t.Run("Every 20th Monday of the year forever (first 3)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970519T090000\nRRULE:FREQ=YEARLY;BYDAY=20MO")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(3)
		want := mkDates(loc, 9, 0, 0, [][3]int{{1997, 5, 19}, {1998, 5, 18}, {1999, 5, 17}})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Monday of week number 20 forever (first 3)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970512T090000\nRRULE:FREQ=YEARLY;BYWEEKNO=20;BYDAY=MO")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(3)
		want := mkDates(loc, 9, 0, 0, [][3]int{{1997, 5, 12}, {1998, 5, 11}, {1999, 5, 17}})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Every Thursday in March forever (first 3 years)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970313T090000\nRRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=TH")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(11)
		want := mkDates(loc, 9, 0, 0, [][3]int{
			{1997, 3, 13}, {1997, 3, 20}, {1997, 3, 27},
			{1998, 3, 5}, {1998, 3, 12}, {1998, 3, 19}, {1998, 3, 26},
			{1999, 3, 4}, {1999, 3, 11}, {1999, 3, 18}, {1999, 3, 25},
		})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Every Thursday in June July August forever (first year)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970605T090000\nRRULE:FREQ=YEARLY;BYDAY=TH;BYMONTH=6,7,8")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(13)
		want := mkDates(loc, 9, 0, 0, [][3]int{
			{1997, 6, 5}, {1997, 6, 12}, {1997, 6, 19}, {1997, 6, 26},
			{1997, 7, 3}, {1997, 7, 10}, {1997, 7, 17}, {1997, 7, 24}, {1997, 7, 31},
			{1997, 8, 7}, {1997, 8, 14}, {1997, 8, 21}, {1997, 8, 28},
		})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Every Friday the 13th forever, with EXDATE (first 4)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970902T090000\nEXDATE;TZID=America/New_York:19970902T090000\nRRULE:FREQ=MONTHLY;BYDAY=FR;BYMONTHDAY=13")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(4)
		want := mkDates(loc, 9, 0, 0, [][3]int{{1998, 2, 13}, {1998, 3, 13}, {1998, 11, 13}, {1999, 8, 13}})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("First Saturday following first Sunday of the month forever (first 6)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970913T090000\nRRULE:FREQ=MONTHLY;BYDAY=SA;BYMONTHDAY=7,8,9,10,11,12,13")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(6)
		want := mkDates(loc, 9, 0, 0, [][3]int{
			{1997, 9, 13}, {1997, 10, 11}, {1997, 11, 8}, {1997, 12, 13}, {1998, 1, 10}, {1998, 2, 7},
		})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("US Presidential Election day every 4 years (first 3)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19961105T090000\nRRULE:FREQ=YEARLY;INTERVAL=4;BYMONTH=11;BYDAY=TU;BYMONTHDAY=2,3,4,5,6,7,8")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(3)
		want := mkDates(loc, 9, 0, 0, [][3]int{{1996, 11, 5}, {2000, 11, 7}, {2004, 11, 2}})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Third TU/WE/TH into the month for next 3 months (BYSETPOS)", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970904T090000\nRRULE:FREQ=MONTHLY;COUNT=3;BYDAY=TU,WE,TH;BYSETPOS=3",
			mkDates(loc, 9, 0, 0, [][3]int{{1997, 9, 4}, {1997, 10, 7}, {1997, 11, 6}}))
	})

	t.Run("Second-to-last weekday of the month forever (first 6, BYSETPOS)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970929T090000\nRRULE:FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-2")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(6)
		want := mkDates(loc, 9, 0, 0, [][3]int{
			{1997, 9, 29}, {1997, 10, 30}, {1997, 11, 27}, {1997, 12, 30}, {1998, 1, 29}, {1998, 2, 26},
		})
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Every 3 hours 9AM to 5PM on a specific day", func(t *testing.T) {
		// RFC 5545's own text has an internal inconsistency here: the
		// prose says "9:00 AM to 5:00 PM" and the expected output is
		// 09:00, 12:00, 15:00 EDT, but the UNTIL value it prints,
		// 19970902T170000Z, is 17:00 UTC = 13:00 EDT — which would
		// cut the sequence off after 12:00, before 15:00 is ever
		// reached. 17:00 EDT (matching the prose and the stated
		// output) is 21:00 UTC. This is a known RFC 5545 erratum
		// (Errata ID 3883); we use the corrected UNTIL so the test
		// checks the RFC's stated *output*, and separately assert
		// our engine honors UNTIL as an absolute instant (so a
		// literal 170000Z bound really does cut off at 12:00 EDT,
		// not 15:00) in TestUntilIsAnAbsoluteInstant.
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=HOURLY;INTERVAL=3;UNTIL=19970902T210000Z",
			[]time.Time{mk(loc, 1997, 9, 2, 9, 0, 0), mk(loc, 1997, 9, 2, 12, 0, 0), mk(loc, 1997, 9, 2, 15, 0, 0)})
	})

	t.Run("Every 15 minutes for 6 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=MINUTELY;INTERVAL=15;COUNT=6",
			[]time.Time{
				mk(loc, 1997, 9, 2, 9, 0, 0), mk(loc, 1997, 9, 2, 9, 15, 0), mk(loc, 1997, 9, 2, 9, 30, 0),
				mk(loc, 1997, 9, 2, 9, 45, 0), mk(loc, 1997, 9, 2, 10, 0, 0), mk(loc, 1997, 9, 2, 10, 15, 0),
			})
	})

	t.Run("Every hour and a half for 4 occurrences", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=MINUTELY;INTERVAL=90;COUNT=4",
			[]time.Time{
				mk(loc, 1997, 9, 2, 9, 0, 0), mk(loc, 1997, 9, 2, 10, 30, 0),
				mk(loc, 1997, 9, 2, 12, 0, 0), mk(loc, 1997, 9, 2, 13, 30, 0),
			})
	})

	t.Run("Every 20 minutes 9AM-4:40PM daily (BYHOUR+BYMINUTE form, first 2 days)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=DAILY;BYHOUR=9,10,11,12,13,14,15,16;BYMINUTE=0,20,40")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(48)
		var want []time.Time
		for _, day := range [][3]int{{1997, 9, 2}, {1997, 9, 3}} {
			for _, h := range []int{9, 10, 11, 12, 13, 14, 15, 16} {
				for _, mi := range []int{0, 20, 40} {
					want = append(want, mk(loc, day[0], time.Month(day[1]), day[2], h, mi, 0))
				}
			}
		}
		if len(got) != len(want) {
			t.Fatalf("got %d, want %d", len(got), len(want))
		}
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Every 20 minutes 9AM-4:40PM daily (MINUTELY+BYHOUR form, first 2 days)", func(t *testing.T) {
		r, err := Parse("DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=MINUTELY;INTERVAL=20;BYHOUR=9,10,11,12,13,14,15,16")
		if err != nil {
			t.Fatal(err)
		}
		got := r.All(48)
		var want []time.Time
		for _, day := range [][3]int{{1997, 9, 2}, {1997, 9, 3}} {
			for _, h := range []int{9, 10, 11, 12, 13, 14, 15, 16} {
				for _, mi := range []int{0, 20, 40} {
					want = append(want, mk(loc, day[0], time.Month(day[1]), day[2], h, mi, 0))
				}
			}
		}
		if len(got) != len(want) {
			t.Fatalf("got %d, want %d", len(got), len(want))
		}
		for i := range want {
			if !got[i].Equal(want[i]) {
				t.Errorf("occurrence %d = %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("WKST=MO changes the result", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970805T090000\nRRULE:FREQ=WEEKLY;INTERVAL=2;COUNT=4;BYDAY=TU,SU;WKST=MO",
			mkDates(loc, 9, 0, 0, [][3]int{{1997, 8, 5}, {1997, 8, 10}, {1997, 8, 19}, {1997, 8, 24}}))
	})

	t.Run("WKST=SU yields a different result", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:19970805T090000\nRRULE:FREQ=WEEKLY;INTERVAL=2;COUNT=4;BYDAY=TU,SU;WKST=SU",
			mkDates(loc, 9, 0, 0, [][3]int{{1997, 8, 5}, {1997, 8, 17}, {1997, 8, 19}, {1997, 8, 31}}))
	})

	t.Run("Invalid date (February 30) is ignored", func(t *testing.T) {
		checkRule(t, "DTSTART;TZID=America/New_York:20070115T090000\nRRULE:FREQ=MONTHLY;BYMONTHDAY=15,30;COUNT=5",
			mkDates(loc, 9, 0, 0, [][3]int{{2007, 1, 15}, {2007, 1, 30}, {2007, 2, 15}, {2007, 3, 15}, {2007, 3, 30}}))
	})
}

// TestUntilIsAnAbsoluteInstant checks that this package applies UNTIL as an
// absolute UTC instant (as RFC 5545 §3.3.10 requires when DTSTART carries a
// TZID), independent of the erratum noted above: taken literally, UNTIL=
// 19970902T170000Z (17:00 UTC = 13:00 EDT) must cut the sequence off after
// 12:00 EDT, one step before the RFC prose's intended 15:00.
func TestUntilIsAnAbsoluteInstant(t *testing.T) {
	loc := nyLoc(t)
	checkRule(t, "DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=HOURLY;INTERVAL=3;UNTIL=19970902T170000Z",
		[]time.Time{mk(loc, 1997, 9, 2, 9, 0, 0), mk(loc, 1997, 9, 2, 12, 0, 0)})
}
