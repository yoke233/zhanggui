package cron

import (
	"testing"
	"time"
)

func TestParseCron(t *testing.T) {
	cases := []struct {
		expr    string
		wantErr bool
	}{
		{"* * * * *", false},
		{"0 * * * *", false},
		{"*/15 * * * *", false},
		{"0 8 * * *", false},
		{"0 8 * * 1-5", false},
		{"0 */6 * * *", false},
		{"30 2 1,15 * *", false},
		{"bad", true},
		{"* * *", true},
		{"60 * * * *", true},
		{"* 25 * * *", true},
	}

	for _, tc := range cases {
		_, err := parseCron(tc.expr)
		if tc.wantErr && err == nil {
			t.Errorf("parseCron(%q): expected error", tc.expr)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("parseCron(%q): unexpected error: %v", tc.expr, err)
		}
	}
}

func TestCronScheduleMatches(t *testing.T) {
	// "0 8 * * *" — every day at 08:00
	sched, err := parseCron("0 8 * * *")
	if err != nil {
		t.Fatal(err)
	}

	at0800 := time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC)
	at0801 := time.Date(2026, 3, 11, 8, 1, 0, 0, time.UTC)
	at0900 := time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC)

	if !sched.matches(at0800) {
		t.Error("should match 08:00")
	}
	if sched.matches(at0801) {
		t.Error("should not match 08:01")
	}
	if sched.matches(at0900) {
		t.Error("should not match 09:00")
	}
}

func TestCronShouldFire(t *testing.T) {
	// "*/15 * * * *" — every 15 minutes
	sched, err := parseCron("*/15 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	lastFired := time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC)
	now8_10 := time.Date(2026, 3, 11, 8, 10, 0, 0, time.UTC)
	now8_16 := time.Date(2026, 3, 11, 8, 16, 0, 0, time.UTC)

	if sched.shouldFire(lastFired, now8_10) {
		t.Error("should not fire at 08:10 (next is 08:15)")
	}
	if !sched.shouldFire(lastFired, now8_16) {
		t.Error("should fire at 08:16 (passed 08:15)")
	}
}

func TestCronShouldFireNeverFired(t *testing.T) {
	sched, err := parseCron("0 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 11, 8, 0, 30, 0, time.UTC)
	if !sched.shouldFire(time.Time{}, now) {
		t.Error("should fire when never fired and current minute matches")
	}

	now2 := time.Date(2026, 3, 11, 8, 30, 0, 0, time.UTC)
	if sched.shouldFire(time.Time{}, now2) {
		t.Error("should not fire at :30 for '0 * * * *'")
	}
}

func TestNextAfter(t *testing.T) {
	// "0 8 * * *" — every day at 08:00
	sched, err := parseCron("0 8 * * *")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC)
	next := sched.nextAfter(base)
	want := time.Date(2026, 3, 12, 8, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("nextAfter(08:00) = %v, want %v", next, want)
	}

	// Next after 07:59 should be same day 08:00.
	before := time.Date(2026, 3, 11, 7, 59, 0, 0, time.UTC)
	next2 := sched.nextAfter(before)
	want2 := time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC)
	if !next2.Equal(want2) {
		t.Errorf("nextAfter(07:59) = %v, want %v", next2, want2)
	}
}

func TestNextAfterStep(t *testing.T) {
	// "*/15 * * * *" — every 15 minutes
	sched, err := parseCron("*/15 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC)
	next := sched.nextAfter(base)
	want := time.Date(2026, 3, 11, 8, 15, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("nextAfter(08:00) = %v, want %v", next, want)
	}
}

func TestShouldFireLargeGap(t *testing.T) {
	// "0 8 * * *" — daily at 08:00
	// lastFired 3 days ago, now is 08:01 — should fire
	sched, err := parseCron("0 8 * * *")
	if err != nil {
		t.Fatal(err)
	}

	lastFired := time.Date(2026, 3, 8, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, 3, 11, 8, 1, 0, 0, time.UTC)
	if !sched.shouldFire(lastFired, now) {
		t.Error("should fire after 3-day gap")
	}
}
