package stats

import (
	"errors"
	"fmt"
	"time"
)

// Interval is the bucket width of a time series.
type Interval string

const (
	IntervalHour Interval = "hour"
	IntervalDay  Interval = "day"
)

// Range is a half-open UTC window: [From, To).
type Range struct {
	From time.Time
	To   time.Time
}

const (
	defaultRangeDays = 7
	maxRangeDays     = 400
)

// ParseRange reads the from/to query parameters. Both accept RFC3339
// ("2026-10-04T00:00:00Z") or a bare date ("2026-10-04"), which is taken as
// midnight UTC. Empty values default to the last 7 days.
//
// Everything is UTC: daily buckets and (from Phase 5) the visitor salt both
// rotate at 00:00 UTC, so a site-local time zone would not line up.
func ParseRange(from, to string, now time.Time) (Range, error) {
	now = now.UTC()

	r := Range{
		From: now.AddDate(0, 0, -defaultRangeDays),
		To:   now,
	}

	if from != "" {
		t, err := parseTime(from)
		if err != nil {
			return Range{}, fmt.Errorf("invalid from: %w", err)
		}
		r.From = t
	}
	if to != "" {
		t, err := parseTime(to)
		if err != nil {
			return Range{}, fmt.Errorf("invalid to: %w", err)
		}
		r.To = t
	}

	if !r.To.After(r.From) {
		return Range{}, errors.New("to must be after from")
	}
	if r.To.Sub(r.From) > maxRangeDays*24*time.Hour {
		return Range{}, fmt.Errorf("range must be %d days or less", maxRangeDays)
	}
	return r, nil
}

func parseTime(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, errors.New("want RFC3339 or YYYY-MM-DD")
	}
	return t.UTC(), nil
}

// ParseInterval validates the interval parameter. Empty picks hourly buckets
// for windows up to 2 days and daily buckets beyond that, so the chart never
// renders hundreds of points.
func ParseInterval(v string, r Range) (Interval, error) {
	switch Interval(v) {
	case IntervalHour, IntervalDay:
		return Interval(v), nil
	case "":
		if r.To.Sub(r.From) <= 48*time.Hour {
			return IntervalHour, nil
		}
		return IntervalDay, nil
	default:
		return "", errors.New(`interval must be "hour" or "day"`)
	}
}
