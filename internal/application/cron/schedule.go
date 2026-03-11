package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronSchedule represents a parsed cron expression.
// Supports standard 5-field format: minute hour day-of-month month day-of-week.
// Supports: numbers, ranges (1-5), steps (*/15), lists (1,3,5), wildcards (*).
type cronSchedule struct {
	minutes  fieldMatcher
	hours    fieldMatcher
	days     fieldMatcher
	months   fieldMatcher
	weekdays fieldMatcher
	raw      string
}

// parseCron parses a standard 5-field cron expression.
func parseCron(expr string) (cronSchedule, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return cronSchedule{}, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(parts), expr)
	}

	minutes, err := parseField(parts[0], 0, 59)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("cron minute: %w", err)
	}
	hours, err := parseField(parts[1], 0, 23)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("cron hour: %w", err)
	}
	days, err := parseField(parts[2], 1, 31)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("cron day: %w", err)
	}
	months, err := parseField(parts[3], 1, 12)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("cron month: %w", err)
	}
	weekdays, err := parseField(parts[4], 0, 6)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("cron weekday: %w", err)
	}

	return cronSchedule{
		minutes:  minutes,
		hours:    hours,
		days:     days,
		months:   months,
		weekdays: weekdays,
		raw:      expr,
	}, nil
}

// shouldFire returns true if the schedule should fire between lastFired and now.
func (s cronSchedule) shouldFire(lastFired, now time.Time) bool {
	if lastFired.IsZero() {
		// Never fired before; fire if current minute matches.
		return s.matches(now)
	}

	// Walk forward from lastFired in 1-minute increments, check if any minute
	// between (lastFired, now] matches. For efficiency, cap at 1440 iterations (24h).
	check := lastFired.Truncate(time.Minute).Add(time.Minute)
	nowTrunc := now.Truncate(time.Minute)
	maxIter := 1440
	for i := 0; check.Before(nowTrunc) || check.Equal(nowTrunc); i++ {
		if i >= maxIter {
			break
		}
		if s.matches(check) {
			return true
		}
		check = check.Add(time.Minute)
	}
	return false
}

// matches checks if a specific time matches all cron fields.
func (s cronSchedule) matches(t time.Time) bool {
	return s.minutes.matches(t.Minute()) &&
		s.hours.matches(t.Hour()) &&
		s.days.matches(t.Day()) &&
		s.months.matches(int(t.Month())) &&
		s.weekdays.matches(int(t.Weekday()))
}

// fieldMatcher matches a cron field value.
type fieldMatcher struct {
	values map[int]struct{} // nil means wildcard (match all)
}

func (f fieldMatcher) matches(v int) bool {
	if f.values == nil {
		return true // wildcard
	}
	_, ok := f.values[v]
	return ok
}

// parseField parses a single cron field (e.g., "*/15", "1-5", "1,3,5", "*").
func parseField(field string, min, max int) (fieldMatcher, error) {
	if field == "*" {
		return fieldMatcher{}, nil // wildcard
	}

	values := make(map[int]struct{})

	// Handle comma-separated list: "1,3,5"
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)

		// Handle step: "*/15" or "1-30/5"
		step := 1
		if idx := strings.Index(part, "/"); idx >= 0 {
			s, err := strconv.Atoi(part[idx+1:])
			if err != nil || s <= 0 {
				return fieldMatcher{}, fmt.Errorf("invalid step in %q", field)
			}
			step = s
			part = part[:idx]
		}

		// Handle range: "1-5" or "*"
		if part == "*" {
			for i := min; i <= max; i += step {
				values[i] = struct{}{}
			}
			continue
		}

		if idx := strings.Index(part, "-"); idx >= 0 {
			lo, err := strconv.Atoi(part[:idx])
			if err != nil {
				return fieldMatcher{}, fmt.Errorf("invalid range start in %q", field)
			}
			hi, err := strconv.Atoi(part[idx+1:])
			if err != nil {
				return fieldMatcher{}, fmt.Errorf("invalid range end in %q", field)
			}
			if lo < min || hi > max || lo > hi {
				return fieldMatcher{}, fmt.Errorf("range %d-%d out of bounds [%d,%d]", lo, hi, min, max)
			}
			for i := lo; i <= hi; i += step {
				values[i] = struct{}{}
			}
			continue
		}

		// Single value.
		v, err := strconv.Atoi(part)
		if err != nil {
			return fieldMatcher{}, fmt.Errorf("invalid value %q", part)
		}
		if v < min || v > max {
			return fieldMatcher{}, fmt.Errorf("value %d out of bounds [%d,%d]", v, min, max)
		}
		for i := v; i <= max; i += step {
			values[i] = struct{}{}
			if step == 1 {
				break // single value, not a step range
			}
		}
	}

	if len(values) == 0 {
		return fieldMatcher{}, fmt.Errorf("empty field %q", field)
	}
	return fieldMatcher{values: values}, nil
}
