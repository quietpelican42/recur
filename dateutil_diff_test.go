package recur

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This file differentially tests this package against python-dateutil's
// rrule for many randomly generated rules, all anchored in UTC.
//
// dateutil is not the DST oracle for this package: this package's whole
// reason to exist is that a naive "add calendar units to a time.Time"
// implementation gets DST wrong, and dateutil.rrule works the same naive
// way — it operates on the DTSTART's tzinfo without RFC 5545 §3.3.5's
// gap/fold resolution, so it reproduces the same class of bug this package
// fixes (see resolve.go and TestVerifiedPremiseReproductions). Comparing
// against it is only meaningful with no time zone in play, where "add a
// calendar unit" and "resolve through DST" agree by construction. DST
// correctness is instead verified against RFC 5545 §3.3.5's stated rule by
// hand in dst_test.go.
//
// The oracle lives in a local venv under the scratchpad
// (dateutil-venv/bin/python + dateutil_expand.py), never committed to this
// repository (see BUILD_COMMON.md's third-party rule) and never installed
// system-wide. If either is missing, the test skips loudly with the command
// to set it up.

func dateutilOracle(t *testing.T) (python string, script string) {
	t.Helper()
	scratchpad := os.Getenv("RECUR_SCRATCHPAD")
	if scratchpad == "" {
		t.Skip("RECUR_SCRATCHPAD not set: point it at a directory containing " +
			"dateutil-venv/ (python3 -m venv dateutil-venv && dateutil-venv/bin/pip install python-dateutil) " +
			"and dateutil_expand.py to run the python-dateutil differential test")
	}
	python = filepath.Join(scratchpad, "dateutil-venv", "bin", "python")
	script = filepath.Join(scratchpad, "dateutil_expand.py")
	if _, err := os.Stat(python); err != nil {
		t.Skipf("python-dateutil oracle not set up (expected venv at %s): %v\n"+
			"set up with: python3 -m venv %s/dateutil-venv && %s/dateutil-venv/bin/pip install python-dateutil",
			python, err, scratchpad, scratchpad)
	}
	if _, err := os.Stat(script); err != nil {
		t.Skipf("dateutil_expand.py not found at %s: %v", script, err)
	}
	return python, script
}

func runOracle(t *testing.T, python, script, block string, count int) ([]time.Time, error) {
	t.Helper()
	cmd := exec.Command(python, script, fmt.Sprintf("%d", count))
	cmd.Stdin = strings.NewReader(block)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("dateutil_expand.py: %v: %s", err, stderr.String())
	}
	var out []time.Time
	sc := bufio.NewScanner(&stdout)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, line)
		if err != nil {
			return nil, fmt.Errorf("bad oracle output line %q: %w", line, err)
		}
		out = append(out, ts)
	}
	return out, nil
}

// randomRule generates a syntactically valid, UTC-anchored RRULE that both
// this package and dateutil should accept, using a fixed-seed RNG so the
// suite is reproducible across runs.
func randomRule(rng *rand.Rand) (dtstart time.Time, ruleValue string, count int) {
	freqs := []string{"DAILY", "WEEKLY", "MONTHLY", "YEARLY", "HOURLY", "MINUTELY"}
	freq := freqs[rng.Intn(len(freqs))]

	y := 2015 + rng.Intn(10)
	mo := 1 + rng.Intn(12)
	d := 1 + rng.Intn(28)
	h := rng.Intn(24)
	mi := rng.Intn(60)
	s := rng.Intn(60)
	dtstart = time.Date(y, time.Month(mo), d, h, mi, s, 0, time.UTC)

	count = 5 + rng.Intn(15)
	parts := []string{"FREQ=" + freq}
	if rng.Intn(3) == 0 {
		parts = append(parts, fmt.Sprintf("INTERVAL=%d", 1+rng.Intn(3)))
	}

	weekdays := []string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"}
	switch freq {
	case "WEEKLY":
		if rng.Intn(2) == 0 {
			n := 1 + rng.Intn(3)
			chosen := make(map[string]bool)
			var picks []string
			for len(picks) < n {
				w := weekdays[rng.Intn(len(weekdays))]
				if !chosen[w] {
					chosen[w] = true
					picks = append(picks, w)
				}
			}
			parts = append(parts, "BYDAY="+strings.Join(picks, ","))
		}
	case "MONTHLY":
		switch rng.Intn(3) {
		case 0:
			parts = append(parts, fmt.Sprintf("BYMONTHDAY=%d", 1+rng.Intn(28)))
		case 1:
			ord := 1 + rng.Intn(4)
			if rng.Intn(2) == 0 {
				ord = -ord
			}
			parts = append(parts, fmt.Sprintf("BYDAY=%d%s", ord, weekdays[rng.Intn(7)]))
		}
		if rng.Intn(4) == 0 {
			parts = append(parts, "BYSETPOS=-1", "BYDAY=MO,TU,WE,TH,FR")
		}
	case "YEARLY":
		if rng.Intn(2) == 0 {
			parts = append(parts, fmt.Sprintf("BYMONTH=%d", 1+rng.Intn(12)))
		}
		if rng.Intn(2) == 0 {
			parts = append(parts, "BYDAY="+weekdays[rng.Intn(7)])
		}
	case "HOURLY", "MINUTELY":
		if rng.Intn(2) == 0 {
			n := 1 + rng.Intn(3)
			var picks []string
			for i := 0; i < n; i++ {
				picks = append(picks, fmt.Sprintf("%d", rng.Intn(24)))
			}
			parts = append(parts, "BYHOUR="+strings.Join(picks, ","))
		}
	}

	return dtstart, strings.Join(parts, ";"), count
}

func TestDateutilDifferential(t *testing.T) {
	python, script := dateutilOracle(t)

	rng := rand.New(rand.NewSource(20260928))
	const trials = 60
	agree, disagree, skipped := 0, 0, 0

	for i := 0; i < trials; i++ {
		dtstart, ruleValue, count := randomRule(rng)
		block := fmt.Sprintf("DTSTART:%sZ\nRRULE:%s;COUNT=%d", dtstart.Format("20060102T150405"), ruleValue, count)

		ours, err := Parse(block)
		if err != nil {
			t.Fatalf("trial %d: Parse(%q): %v", i, block, err)
		}
		gotOurs := ours.All(count + 2)

		theirs, err := runOracle(t, python, script, block, count+2)
		if err != nil {
			skipped++
			t.Logf("trial %d: %q: oracle error, skipping this case: %v", i, block, err)
			continue
		}

		if len(gotOurs) != len(theirs) {
			disagree++
			t.Errorf("trial %d %q\n  recur:    %d occurrences %v\n  dateutil: %d occurrences %v",
				i, block, len(gotOurs), formatTimes(gotOurs), len(theirs), formatTimes(theirs))
			continue
		}
		mismatch := false
		for j := range gotOurs {
			if !gotOurs[j].Equal(theirs[j]) {
				mismatch = true
				break
			}
		}
		if mismatch {
			disagree++
			t.Errorf("trial %d %q\n  recur:    %v\n  dateutil: %v", i, block, formatTimes(gotOurs), formatTimes(theirs))
		} else {
			agree++
		}
	}

	t.Logf("dateutil differential: %d/%d agreed, %d disagreed, %d skipped (oracle errors)", agree, trials, disagree, skipped)
}
