# recur

RFC 5545 (iCalendar) recurrence rules for Go: parse an RRULE, expand it into
occurrences, correctly across DST.

```go
r, err := recur.Parse("DTSTART;TZID=America/New_York:20240308T023000\nRRULE:FREQ=DAILY;COUNT=4")
if err != nil {
    log.Fatal(err)
}
for _, t := range r.All(4) {
    fmt.Println(t.Format(time.RFC3339))
}
// 2024-03-08T02:30:00-05:00
// 2024-03-09T02:30:00-05:00
// 2024-03-10T03:30:00-04:00  (shifted past the spring-forward gap, per RFC 5545 §3.3.5)
// 2024-03-11T03:30:00-04:00
```

## Why

A recurrence library that just adds calendar units to a `time.Time` gets DST
wrong twice: a daily rule whose time-of-day falls in a spring-forward gap
moves the wall clock *backward* instead of shifting past the gap, and an
hourly rule stepped by wall-clock hour rather than absolute duration can
land on the same instant twice across a gap, counting it as two occurrences.
RFC 5545 §3.3.5 specifies the correct behavior explicitly: a nonexistent
local time is interpreted using the UTC offset in effect before the gap, and
an ambiguous (fall-back) local time refers to its first occurrence.

Checked incumbents: `teambition/rrule-go` (383 GitHub stars, most starred Go
RRULE library, last release tag 2024) has both bugs — issue #63 documents the
duplicate-hour case, open with two unmerged fix PRs (#64, #68) for over two
years. `stephens2424/rrule` and `malcolmston/rrule` were also checked against
the same two cases (below) and have the same spring-forward bug.

Measured: `go run` against a local checkout of `teambition/rrule-go` v1.8.2,
`DAILY, DTSTART 2024-03-08 02:30 America/New_York, COUNT=4`:

```
2024-03-08T02:30:00-05:00 America/New_York
2024-03-09T02:30:00-05:00 America/New_York
2024-03-10T01:30:00-05:00 America/New_York   <- wall clock moved backward; should be 03:30 EDT
2024-03-11T02:30:00-04:00 America/New_York
```

The same rule through this package:

```
2024-03-08T02:30:00-05:00
2024-03-09T02:30:00-05:00
2024-03-10T03:30:00-04:00
2024-03-11T03:30:00-04:00
```

And `teambition/rrule-go` on `HOURLY, DTSTART 2024-10-06 00:00
Australia/Sydney, COUNT=6` (a spring-forward in the Southern Hemisphere)
emits the same instant twice:

```
2024-10-06T00:00:00+10:00 Australia/Sydney
2024-10-06T01:00:00+10:00 Australia/Sydney
2024-10-06T03:00:00+11:00 Australia/Sydney
2024-10-06T03:00:00+11:00 Australia/Sydney   <- duplicate instant
2024-10-06T04:00:00+11:00 Australia/Sydney
2024-10-06T05:00:00+11:00 Australia/Sydney
```

This package steps HOURLY/MINUTELY/SECONDLY by absolute duration, which
cannot produce a duplicate or a gap by construction (see `TestVerifiedPremiseReproductions`).

## Installation

```sh
go get github.com/__GH_OWNER__/recur
```

## Usage

### Parse and expand a rule

```go
r, err := recur.Parse("DTSTART;TZID=Europe/London:20240101T090000\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR;COUNT=12")
occurrences := r.All(12)
```

### Query a window

```go
for t := range r.Iter(from, to) {
    fmt.Println(t)
}
occurrences := r.Between(from, to)
next, ok := r.After(time.Now(), false)
last, ok := r.Before(time.Now(), false)
```

### RDATE and EXDATE

`Parse` accepts a full iCalendar recurrence block, not just a single RRULE
line:

```go
r, err := recur.Parse(`DTSTART:19970902T090000Z
RRULE:FREQ=DAILY;COUNT=3
RDATE:19971225T090000Z
EXDATE:19970903T090000Z`)
// 1997-09-02, 1997-09-04, 1997-12-25 (09:03 excluded, 12-25 added)
```

### Build a rule without parsing text

```go
r, err := recur.New(recur.Options{
    DTStart:  time.Date(1997, 9, 5, 9, 0, 0, 0, mustLoc("America/New_York")),
    HasRRule: true,
    Freq:     recur.Monthly,
    Count:    10,
    ByDay:    []recur.Weekday{recur.FR.Nth(1)},
})
```

### Serialize back to text

```go
r.String() // "DTSTART;TZID=America/New_York:19970905T090000\nRRULE:FREQ=MONTHLY;COUNT=10;BYDAY=1FR"
```

## Reference

### Parsing and building

| Symbol | Description |
| --- | --- |
| `Parse(s string) (*Rule, error)` | Parses a DTSTART/RRULE/RDATE/EXDATE block. |
| `New(opts Options) (*Rule, error)` | Builds and validates a `Rule` programmatically. |
| `(*Rule) String() string` | Serializes back to the block form `Parse` accepts. |

### Expansion

| Symbol | Description |
| --- | --- |
| `(*Rule) Iter(from, to time.Time) iter.Seq[time.Time]` | Go 1.23 iterator over `[from, to)`; a zero `time.Time` on either side means unbounded. |
| `(*Rule) Between(from, to time.Time) []time.Time` | `Iter` materialized into a slice. |
| `(*Rule) After(t time.Time, inclusive bool) (time.Time, bool)` | First occurrence after (or at, if `inclusive`) `t`. |
| `(*Rule) Before(t time.Time, inclusive bool) (time.Time, bool)` | Last occurrence before (or at, if `inclusive`) `t`. |
| `(*Rule) All(limit int) []time.Time` | First `limit` occurrences from the start. |

### Types

| Symbol | Description |
| --- | --- |
| `Rule` | A parsed/validated recurrence: `DTStart`, `Freq`, `Interval`, `Count`/`Until`, every `ByXxx` field, `WKST`, `RDate`, `ExDate`. |
| `Options` | Input to `New`; mirrors `Rule` except `WKST *time.Weekday` (nil defaults to Monday). |
| `Frequency` | `Yearly`, `Monthly`, `Weekly`, `Daily`, `Hourly`, `Minutely`, `Secondly`. |
| `Weekday` | A `time.Weekday` plus an optional ordinal; `SU`...`SA` are predefined, `.Nth(n)` qualifies one (`FR.Nth(-1)` = last Friday). |
| `ErrInvalidRule` | A syntactically or semantically invalid rule (bad range, COUNT+UNTIL both set). Test with `errors.Is`. |
| `ErrUnsupported` | A rule RFC 5545 forbids in this combination (e.g. BYWEEKNO outside YEARLY), or a feature this package does not implement. Never produces wrong dates instead. |

## How it works

Occurrences are built in two passes, matching RFC 5545 §3.3.10's own
description of the BYxxx evaluation order:

1. For YEARLY/MONTHLY/WEEKLY/DAILY, each period (a calendar year, month, WKST
   week, or day) contributes a set of calendar days, computed by applying
   BYMONTH, BYWEEKNO, BYYEARDAY, BYMONTHDAY, and BYDAY in that order (see
   `expand.go`), then crossed with a BYHOUR×BYMINUTE×BYSECOND time-of-day set
   and, finally, filtered by BYSETPOS.
2. For HOURLY/MINUTELY/SECONDLY, occurrences are generated by adding
   `interval * unit` to the previous instant as an absolute `time.Duration`
   — never by incrementing a wall-clock field — so a DST transition can
   neither duplicate nor skip a step (see `timestep.go`).

Every calendar day produced by pass 1 is resolved to an instant by
`resolveLocal` (`resolve.go`), which reproduces RFC 5545 §3.3.5 exactly.
Pass 2 never calls it: HOURLY/MINUTELY/SECONDLY candidates are always built
by adding a duration to an already-resolved instant, precisely so that a
BYMINUTE/BYSECOND expansion cannot reintroduce gap/fold shifting into
what must stay pure absolute-duration arithmetic (an earlier version of
this package called `resolveLocal` from the HOURLY path too, and a
reviewer's probe caught it silently skipping an instant across Sydney's
October spring-forward — see `timestep.go`'s doc comment).

`resolveLocal` locates the specific zone transition nearest the candidate
using `time.Location`'s `ZoneBounds` (Go 1.19+) rather than assuming one is
nearby: it tries the offset in effect at the candidate, and the offsets
immediately before and after the neighboring transition, keeping whichever
of those round-trip (interpreting the wall clock with the offset reproduces
that same offset at the resulting instant). Exactly one round-tripping
offset is an ordinary, unambiguous time. Two mean an ambiguous (fall-back)
time — resolved to its first (earlier) occurrence. None mean a nonexistent
(spring-forward) time — resolved with the smaller of the two offsets, which
is always the pre-gap one, since DST by definition only ever adds time. This
also handles Pacific/Apia's 2011-12-30, a transition that skips an entire
day rather than an hour, the same way — see `TestApiaSkippedDayRule`.

An earlier version of `resolveLocal` instead always reinterpreted the wall
clock with whatever offset was in effect about a day earlier, which handled
a transition's own day correctly but wrongly re-shifted ordinary,
unambiguous times on the days around it (a reviewer's probe: DTSTART
2024-03-08 02:30 America/New_York, DAILY, COUNT=4 — the 4th occurrence,
2024-03-11 02:30, an ordinary EDT time the day after the transition, came
out as 03:30). `TestDailySweepAroundTransitions` and `TestDailyAtGapOrFoldHour`
in `dst_sweep_test.go` check a week either side of 8 real transitions across
5 zones specifically to catch a regression of that shape again.

## Limitations

- Unsupported RRULE parts or combinations return `ErrUnsupported` from
  `Parse`/`New` rather than silently producing an incorrect recurrence set.
  This includes: BYWEEKNO outside FREQ=YEARLY, BYYEARDAY with
  DAILY/WEEKLY/MONTHLY, BYMONTHDAY with WEEKLY, a numeric BYDAY ordinal
  outside MONTHLY/YEARLY, and BYSETPOS without another BYxxx part — all
  forbidden by RFC 5545 §3.3.10 itself.
- BYSETPOS combined with HOURLY/MINUTELY/SECONDLY is supported only within a
  single step's own expansion (e.g. BYSETPOS over the BYMINUTE/BYSECOND
  combinations inside one HOURLY step); it does not reach across steps.
- A rule that can never produce an occurrence (e.g.
  `FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=30`, since February 30th never exists)
  stops after 100,000 consecutive empty base-frequency periods rather than
  iterating forever — see `maxConsecutiveEmptyPeriods` in `iter.go`. A
  realistic rule never comes close to this.
- `Iter`/`Between`/`After`/`Before`/`All` always walk the recurrence from
  `DTStart`, because `COUNT` counts from there; a query for a window far in
  the future on a rule with many prior occurrences costs proportionally more,
  not just the size of the window.
- No `EXRULE` (deprecated in RFC 5545, dropped in favor of `EXDATE`), no
  `RSCALE` (RFC 7529, non-Gregorian calendars), and no `PERIOD`-typed
  `RDATE`.
- A floating DTSTART/UNTIL (no `Z`, no `TZID`) is represented internally
  anchored to `time.UTC`. This is a deliberate implementation choice — see
  the comment on `Rule.DTStartFloating` — not a claim that a floating value
  means UTC; it keeps floating rules deterministic across machines in
  different zones, since UTC has no DST transitions to accidentally apply.

## Compatibility

Developed and tested on Go 1.27.1, darwin/arm64. CI (`.github/workflows/test.yml`)
runs the same checks on `ubuntu-latest` with the current stable Go release;
that is "run in CI", not independently verified here.

## Development

```sh
go build ./...
go vet ./...
go test -race -count=1 ./...
gofmt -l .   # must print nothing
```

The differential test against `python-dateutil` (`dateutil_diff_test.go`)
needs a local venv that is never committed:

```sh
python3 -m venv /path/to/scratchpad/dateutil-venv
/path/to/scratchpad/dateutil-venv/bin/pip install python-dateutil
RECUR_SCRATCHPAD=/path/to/scratchpad go test -run TestDateutilDifferential -v .
```

It skips loudly if the venv is not present.

## License

MIT. See [LICENSE](LICENSE).
